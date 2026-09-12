package host

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestRootAutoUnlockProtectedFileReplacementAndRemoval(t *testing.T) {
	requireAutoUnlockRoot(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "wallet_password")
	// The repository's sandboxed root runner can map only UID/GID 0. The
	// disposable Debian gate exercises the same primitive with the real lnd
	// identity and proves lnd:lnd ownership on the canonical path.
	const uid, gid = 0, 0
	if err := replaceExactFile(path, []byte("secret"), 0o400, uid, gid); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if info.Mode().Perm() != 0o400 || stat.Uid != uid || stat.Gid != gid {
		t.Fatalf("metadata mode=%04o owner=%d:%d",
			info.Mode().Perm(), stat.Uid, stat.Gid)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "secret" {
		t.Fatalf("content=%q err=%v", data, err)
	}
	if err := removeExactFile(path, 0o400, uid, gid); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("protected file still exists: %v", err)
	}
}

func TestRootAutoUnlockPasswordPublicationLeavesNoStage(t *testing.T) {
	requireAutoUnlockRoot(t)
	dir := t.TempDir()
	canonical := filepath.Join(dir, "wallet_password")
	stage := filepath.Join(dir, ".vpn-wallet-password.stage")
	if err := publishWalletPasswordAt(
		canonical, stage, []byte("first secret"), 0, 0,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatalf("stage remains after publication: %v", err)
	}
	if err := publishWalletPasswordAt(
		canonical, stage, []byte("replacement"), 0, 0,
	); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(canonical)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("canonical content=%q err=%v", data, err)
	}
	info, err := os.Lstat(canonical)
	if err != nil || info.Mode().Perm() != 0o400 {
		t.Fatalf("canonical metadata=%v err=%v", info, err)
	}
}

func TestRootAutoUnlockStageClassificationAndCleanup(t *testing.T) {
	requireAutoUnlockRoot(t)
	dir := t.TempDir()
	stage := filepath.Join(dir, ".vpn-wallet-password.stage")
	if err := os.WriteFile(stage, []byte("interrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stage, 0o600); err != nil {
		t.Fatal(err)
	}
	exists, err := passwordStageExistsAt(stage, 0, 0)
	if err != nil || !exists {
		t.Fatalf("root write phase not recognized: exists=%v err=%v", exists, err)
	}
	if err := removeWalletPasswordStageAt(stage, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatalf("interrupted stage remains: %v", err)
	}
	if err := os.Symlink("target", stage); err != nil {
		t.Fatal(err)
	}
	if _, err := passwordStageExistsAt(stage, 0, 0); err == nil {
		t.Fatal("symlink stage was accepted")
	}
}

func TestRootAutoUnlockRefusesSymlinkDirectoryBoundary(t *testing.T) {
	requireAutoUnlockRoot(t)
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateExactDir(realDir, 0o755, 0, 0); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(root, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	if err := validateExactDir(linkDir, 0o755, 0, 0); err == nil {
		t.Fatal("symlink directory boundary was accepted")
	}
}

func TestRootAutoUnlockProtectedFileRefusesUnsafeDestination(t *testing.T) {
	requireAutoUnlockRoot(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("do not replace"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o400); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := readExactFile(target, 0o400, 0, 0); err != nil || !exists {
		t.Fatalf("invalid positive file fixture: exists=%v, err=%v", exists, err)
	}
	link := filepath.Join(dir, "wallet_password")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := replaceExactFile(link, []byte("secret"), 0o400, 0, 0); err == nil {
		t.Fatal("symlink destination was accepted")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "do not replace" {
		t.Fatalf("symlink target changed: %q, %v", data, err)
	}
}

func requireAutoUnlockRoot(t *testing.T) {
	t.Helper()
	required := os.Getenv("VPN_REQUIRE_ROOT_TESTS")
	if required != "" && required != "1" {
		t.Fatal("VPN_REQUIRE_ROOT_TESTS must be unset or 1")
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		return
	}
	if required == "1" {
		t.Fatal("auto-unlock filesystem tests require Linux root")
	}
	t.Skip("auto-unlock filesystem tests require Linux root")
}
