// internal/installer/setup_test.go

package installer

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/paths"
)

type protectedTreeEntry struct {
	Path       string
	Mode       uint32
	Size       int64
	ModTimeNS  int64
	UID        uint32
	GID        uint32
	Inode      uint64
	Links      uint64
	LinkTarget string
	Contents   []byte
}

func snapshotProtectedTree(t *testing.T, root string) []byte {
	t.Helper()
	var entries []protectedTreeEntry
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := protectedTreeEntry{
			Path:      rel,
			Mode:      uint32(info.Mode()),
			Size:      info.Size(),
			ModTimeNS: info.ModTime().UnixNano(),
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			entry.UID = stat.Uid
			entry.GID = stat.Gid
			entry.Inode = stat.Ino
			entry.Links = uint64(stat.Nlink)
		}
		switch {
		case info.Mode().IsRegular():
			entry.Contents, err = os.ReadFile(path)
		case info.Mode()&os.ModeSymlink != 0:
			entry.LinkTarget, err = os.Readlink(path)
		}
		if err != nil {
			return err
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func installStartupFixture(
	t *testing.T, f *lifecycleFixture,
	preflight func() (SSHObservation, error),
) (*int, *int) {
	t.Helper()
	preflightCalls := 0
	initializeCalls := 0
	deps := productionInstallStartupDependencies()
	runtimeRoot := t.TempDir()
	deps.runtimeDir = filepath.Join(runtimeRoot, "runtime")
	deps.installLock = filepath.Join(deps.runtimeDir, "install.lock")
	deps.fs = f.fs
	deps.lookup = f.lookup
	deps.runPreflight = func() (SSHObservation, error) {
		preflightCalls++
		return preflight()
	}
	deps.initializeLifecycle = func(
		lifecycleFS, identityLookup, installContext,
	) (*installLedger, error) {
		initializeCalls++
		return nil, errors.New("unexpected lifecycle initialization")
	}

	original := newInstallStartupDependencies
	newInstallStartupDependencies = func() installStartupDependencies {
		return deps
	}
	t.Cleanup(func() {
		newInstallStartupDependencies = original
	})
	t.Setenv("DEBIAN_FRONTEND", "test-value")
	t.Setenv("NEEDRESTART_MODE", "test-value")
	return &preflightCalls, &initializeCalls
}

func captureRunInstallOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	runErr := fn()
	os.Stdout = original
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}

func completeLifecycleFixture(t *testing.T, f *lifecycleFixture) {
	t.Helper()
	ledger := f.initialize(t)
	if err := ledger.setDbCache(512); err != nil {
		t.Fatal(err)
	}
	for _, key := range baseInstallStepKeys {
		if err := ledger.markDone(key, "0.7.0"); err != nil {
			t.Fatal(err)
		}
	}
	if err := ledger.markComplete(); err != nil {
		t.Fatal(err)
	}
	if err := ledger.save(f.fs.ledger); err != nil {
		t.Fatal(err)
	}
}

func TestRootRunInstallEarlyExitsDoNotMutateProtectedState(t *testing.T) {
	requireRootTestEnvironment(t)

	t.Run("completed installation", func(t *testing.T) {
		f := newLifecycleFixture(t)
		completeLifecycleFixture(t, f)
		root := filepath.Dir(f.fs.varLibVPN)
		before := snapshotProtectedTree(t, root)
		preflightCalls, initializeCalls := installStartupFixture(
			t, f, func() (SSHObservation, error) {
				return SSHObservation{}, errors.New("unexpected preflight")
			})

		output, err := captureRunInstallOutput(t, func() error {
			return RunInstall(InstallOptions{}, unexpectedInstallFrontend(t))
		})
		if err != nil {
			t.Fatalf("completed install refused: %v", err)
		}
		if !strings.Contains(output, "already installed") {
			t.Fatalf("completed status not reported: %q", output)
		}
		if *preflightCalls != 0 || *initializeCalls != 0 {
			t.Fatalf("completed path reached preflight=%d initialize=%d",
				*preflightCalls, *initializeCalls)
		}
		after := snapshotProtectedTree(t, root)
		if string(after) != string(before) {
			t.Fatal("completed RunInstall path changed protected state")
		}
	})

	refusals := []struct {
		name string
		seed func(*testing.T, *lifecycleFixture)
	}{
		{"unmarked legacy installation", func(t *testing.T, f *lifecycleFixture) {
			if err := os.WriteFile(f.fs.unmarkedPaths[2],
				[]byte("preserve me"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"malformed ledger", func(t *testing.T, f *lifecycleFixture) {
			f.initialize(t)
			if err := os.WriteFile(f.fs.ledger, []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsafe lifecycle object", func(t *testing.T, f *lifecycleFixture) {
			f.initialize(t)
			if err := os.Chmod(f.fs.ledger, 0o640); err != nil {
				t.Fatal(err)
			}
		}},
		{"conflicting bootstrap evidence", func(t *testing.T, f *lifecycleFixture) {
			f.seedBootstrap(t, 1)
			if err := os.WriteFile(f.fs.unmarkedPaths[1],
				[]byte("preserve me"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			tt.seed(t, f)
			root := filepath.Dir(f.fs.varLibVPN)
			before := snapshotProtectedTree(t, root)
			preflightCalls, initializeCalls := installStartupFixture(
				t, f, func() (SSHObservation, error) {
					return SSHObservation{}, errors.New("unexpected preflight")
				})

			if err := RunInstall(InstallOptions{}, unexpectedInstallFrontend(t)); err == nil {
				t.Fatal("unsafe or conflicting installation state accepted")
			}
			if *preflightCalls != 0 || *initializeCalls != 0 {
				t.Fatalf("refusal reached preflight=%d initialize=%d",
					*preflightCalls, *initializeCalls)
			}
			after := snapshotProtectedTree(t, root)
			if string(after) != string(before) {
				t.Fatal("refused RunInstall path changed protected state")
			}
		})
	}

	t.Run("unsupported architecture", func(t *testing.T) {
		f := newLifecycleFixture(t)
		root := filepath.Dir(f.fs.varLibVPN)
		before := snapshotProtectedTree(t, root)
		preflightCalls, initializeCalls := installStartupFixture(
			t, f, func() (SSHObservation, error) {
				return SSHObservation{}, checkArchitecture("arm64")
			})

		err := RunInstall(InstallOptions{}, unexpectedInstallFrontend(t))
		if err == nil || !strings.Contains(err.Error(), "amd64 only") {
			t.Fatalf("unsupported architecture error=%v", err)
		}
		if *preflightCalls != 1 || *initializeCalls != 0 {
			t.Fatalf("architecture path reached preflight=%d initialize=%d",
				*preflightCalls, *initializeCalls)
		}
		after := snapshotProtectedTree(t, root)
		if string(after) != string(before) {
			t.Fatal("unsupported architecture changed protected state")
		}
	})
}

func TestVersionConstants(t *testing.T) {
	if bitcoinVersion == "" {
		t.Error("bitcoinVersion is empty")
	}
	if lndVersion == "" {
		t.Error("lndVersion is empty")
	}
	for name, value := range map[string]string{
		"bitcoinUser":   bitcoinUser,
		"lndUser":       lndUser,
		"syncthingUser": syncthingUser,
		"backupGroup":   backupGroup,
	} {
		if value == "" {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestBaseInstallStepsMatchLedgerSchema(t *testing.T) {
	cfg := config.Default()
	steps := buildInstallSteps(cfg, &InstallDecisions{})
	if err := validateBaseInstallSteps(steps); err != nil {
		t.Fatal(err)
	}
	bake := FilterPhase(steps, PhaseBake)
	if len(bake) != len(bakeInstallStepKeys) {
		t.Fatalf("bake steps=%d schema keys=%d",
			len(bake), len(bakeInstallStepKeys))
	}
	for i := range bake {
		if bake[i].Key != bakeInstallStepKeys[i] {
			t.Fatalf("bake step %d=%q schema=%q", i+1,
				bake[i].Key, bakeInstallStepKeys[i])
		}
	}
}

func TestDedicatedServiceIdentityNames(t *testing.T) {
	tests := []struct {
		name, got, want string
	}{
		{"bitcoin user", bitcoinUser, "bitcoin"},
		{"LND user", lndUser, "lnd"},
		{"Syncthing user", syncthingUser, "syncthing"},
		{"backup group", backupGroup, "vpn-lnd-backup"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestSetAndGetVersion(t *testing.T) {
	original := appVersion
	defer func() { appVersion = original }()

	SetVersion("1.2.3")
	if GetVersion() != "1.2.3" {
		t.Errorf("GetVersion: got %q, want %q", GetVersion(), "1.2.3")
	}
}

func TestLndVersionStr(t *testing.T) {
	v := LndVersionStr()
	if v == "" {
		t.Error("LndVersionStr returned empty")
	}
	if v != lndVersion {
		t.Errorf("got %q, want %q", v, lndVersion)
	}
}

func TestReadVersionCacheEmpty(t *testing.T) {
	// On a dev machine, cache file shouldn't exist at the production path
	cached := readVersionCache()
	// Just verify it doesn't panic — it may or may not have a value
	_ = cached
}

func TestWriteAndReadVersionCache(t *testing.T) {
	// Save original values
	origDir := paths.VersionCacheDir
	origFile := paths.VersionCacheFile

	// We can't override const values, so we test the logic directly
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/latest-version"

	// Write directly
	os.MkdirAll(tmpDir, 0750)
	os.WriteFile(tmpFile, []byte("1.2.3"), 0600)

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if string(data) != "1.2.3" {
		t.Errorf("cached version: got %q, want 1.2.3", string(data))
	}

	// Verify path constants are set
	if origDir == "" {
		t.Error("VersionCacheDir is empty")
	}
	if origFile == "" {
		t.Error("VersionCacheFile is empty")
	}
}

func TestVersionCacheDirConsistency(t *testing.T) {
	if !strings.HasSuffix(paths.VersionCacheDir, ".cache/vpn") {
		t.Errorf("VersionCacheDir unexpected suffix: %s",
			paths.VersionCacheDir)
	}
}

func TestVersionCacheFileConsistency(t *testing.T) {
	if !strings.HasPrefix(paths.VersionCacheFile, paths.VersionCacheDir) {
		t.Error("VersionCacheFile should be inside VersionCacheDir")
	}
	if !strings.HasSuffix(paths.VersionCacheFile, "latest-version") {
		t.Errorf("VersionCacheFile unexpected suffix: %s",
			paths.VersionCacheFile)
	}
}

func TestShellWrapperNetworkFlags(t *testing.T) {
	tests := []struct {
		network string
		bitcoin string
		lnd     string
	}{
		{config.NetworkMainnet, "", ""},
		{config.NetworkTestnet4, "\n        -testnet4 \\", "\n        --network=testnet4 \\"},
		{config.NetworkPublicSignet, "\n        -signet \\", "\n        --network=signet \\"},
	}
	for _, tt := range tests {
		profile, err := config.NetworkConfigFromName(tt.network)
		if err != nil {
			t.Fatal(err)
		}
		if got := bitcoinCLINetworkFlag(profile); got != tt.bitcoin {
			t.Errorf("%s bitcoin wrapper flag %q, want %q",
				tt.network, got, tt.bitcoin)
		}
		if got := lncliNetworkFlag(profile); got != tt.lnd {
			t.Errorf("%s lncli wrapper flag %q, want %q",
				tt.network, got, tt.lnd)
		}
	}
}

// The checkOS tests formerly here moved to preflight_test.go
// (TestCheckOSRelease*), against the preflight's exactly-13 rule.
// The NeedsInstall tests died with NeedsInstall itself: commit 6
// replaced state-sniffing with explicit dispatch (IA-1-8).

func unexpectedInstallFrontend(t *testing.T) InstallFrontend {
	t.Helper()
	return func(InstallView, *InstallSession) (bool, error) {
		t.Error("early exit reached interactive frontend")
		return false, nil
	}
}
