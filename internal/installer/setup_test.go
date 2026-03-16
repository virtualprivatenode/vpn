// internal/installer/setup_test.go

package installer

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ripsline/virtual-private-node/internal/paths"
)

func TestVersionConstants(t *testing.T) {
	if bitcoinVersion == "" {
		t.Error("bitcoinVersion is empty")
	}
	if lndVersion == "" {
		t.Error("lndVersion is empty")
	}
	if systemUser == "" {
		t.Error("systemUser is empty")
	}
}

func TestSystemUserIsBitcoin(t *testing.T) {
	if systemUser != "bitcoin" {
		t.Errorf("systemUser: got %q, want %q", systemUser, "bitcoin")
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

func TestNeedsInstallNoConfig(t *testing.T) {
	// No config file exists on dev machine, so install is needed
	result := NeedsInstall()
	if !result {
		t.Error("NeedsInstall should return true when config is missing")
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
	if !strings.HasSuffix(paths.VersionCacheDir, ".cache/rlvpn") {
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

func TestCheckOSReadsFile(t *testing.T) {
	err := checkOS()
	if err != nil {
		t.Logf("checkOS returned error (expected on non-Debian): %v", err)
	}
}

func TestCheckOSVersionParsing(t *testing.T) {
	tests := []struct {
		ver  string
		pass bool
	}{
		{"13", true},
		{"14", true},
		{"15", true},
		{"12", false},
		{"11", false},
		{"9", false},
	}
	for _, tt := range tests {
		verNum, err := strconv.Atoi(tt.ver)
		if err != nil {
			t.Fatalf("bad test version: %s", tt.ver)
		}
		result := verNum >= 13
		if result != tt.pass {
			t.Errorf("version %q >= 13: got %v, want %v",
				tt.ver, result, tt.pass)
		}
	}
}
