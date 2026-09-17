package host

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"testing/synctest"
)

func requireStagingRoot(t *testing.T) {
	t.Helper()
	required := os.Getenv("VPN_REQUIRE_ROOT_TESTS")
	if required != "" && required != "1" {
		t.Fatal("VPN_REQUIRE_ROOT_TESTS must be unset or 1")
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		return
	}
	if required == "1" {
		t.Fatal("staging filesystem tests require Linux root")
	}
	t.Skip("staging filesystem tests require Linux root")
}

func assertBoardMetadata(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	st := info.Sys().(*syscall.Stat_t)
	if info.Mode().Perm() != mode || st.Uid != 0 || st.Gid != 0 {
		t.Fatalf("%s mode=%04o owner=%d:%d", path, info.Mode().Perm(), st.Uid, st.Gid)
	}
}

func TestBoardBakePublicationAndFirstBootOwnership(t *testing.T) {
	requireStagingRoot(t)
	b := stagingBoard{dir: filepath.Join(t.TempDir(), "state")}
	if err := b.write("credential", []byte("baked"), os.Rename); err != nil {
		t.Fatal(err)
	}
	assertBoardMetadata(t, b.dir, 0o700)
	path := filepath.Join(b.dir, "credential")
	assertBoardMetadata(t, path, 0o600)
	if err := b.fixOwnership(); err == nil {
		t.Fatal("ownership repair succeeded without admin group")
	}
	// The container may map only root. Real vpn-group access is a Debian gate.
	b.haveGroup = true
	if err := b.fixOwnership(); err != nil {
		t.Fatal(err)
	}
	assertBoardMetadata(t, b.dir, 0o750)
	assertBoardMetadata(t, path, 0o640)
	if err := b.write("credential", []byte("current"), os.Rename); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "current" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	assertBoardMetadata(t, path, 0o640)
}

func TestTLSBoardPublicationFailurePreservesPreviousCredential(t *testing.T) {
	requireStagingRoot(t)
	b := stagingBoard{dir: filepath.Join(t.TempDir(), "state"), haveGroup: true}
	if err := b.write("cert", []byte("previous"), os.Rename); err != nil {
		t.Fatal(err)
	}
	cert := stagingCertificate(t)
	failure := errors.New("injected rename failure")
	synctest.Test(t, func(t *testing.T) {
		err := stageLNDTLSCert("source", func(string) ([]byte, error) { return cert, nil }, func(data []byte) error {
			return b.write("cert", data, func(tmp, dst string) error {
				assertBoardMetadata(t, tmp, 0o640)
				previous, err := os.ReadFile(dst)
				if err != nil || string(previous) != "previous" {
					t.Fatal("old credential changed before publication")
				}
				return failure
			})
		})
		if !errors.Is(err, failure) {
			t.Fatalf("publication failure lost: %v", err)
		}
	})
	data, err := os.ReadFile(filepath.Join(b.dir, "cert"))
	if err != nil || string(data) != "previous" {
		t.Fatal("publication failure damaged previous credential")
	}
	entries, err := os.ReadDir(b.dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary credential leaked: entries=%v err=%v", entries, err)
	}
}

func TestBoardRefusesSymlinkDirectoryAndOwnershipRepair(t *testing.T) {
	requireStagingRoot(t)
	dir := t.TempDir()
	outside := filepath.Join(dir, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	b := stagingBoard{dir: filepath.Join(dir, "state"), haveGroup: true}
	if err := os.Symlink(outside, b.dir); err != nil {
		t.Fatal(err)
	}
	if err := b.write("credential", []byte("secret"), os.Rename); err == nil {
		t.Fatal("followed board symlink")
	}
	if err := os.Remove(b.dir); err != nil {
		t.Fatal(err)
	}
	if err := b.ensureDir(); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "private")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(b.dir, "credential")); err != nil {
		t.Fatal(err)
	}
	if err := b.fixOwnership(); err == nil {
		t.Fatal("followed credential symlink during ownership repair")
	}
	assertBoardMetadata(t, secret, 0o600)
}
