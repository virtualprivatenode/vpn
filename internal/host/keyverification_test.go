package host

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestKeyVerificationMarkerLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending")
	uid := os.Geteuid()
	if err := ensureKeyVerificationPendingAt(path, uid); err != nil {
		t.Fatal(err)
	}
	if err := ensureKeyVerificationPendingAt(path, uid); err != nil {
		t.Fatalf("idempotent ensure: %v", err)
	}
	pending, err := keyVerificationPendingAt(path, uid)
	if err != nil || !pending {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %04o, want 0600", info.Mode().Perm())
	}
	if err := clearKeyVerificationPendingAt(path, uid); err != nil {
		t.Fatal(err)
	}
	pending, err = keyVerificationPendingAt(path, uid)
	if err != nil || pending {
		t.Fatalf("after clear pending=%v err=%v", pending, err)
	}
}

func TestKeyVerificationMarkerRefusesUnsafeObjects(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "pending")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ensureKeyVerificationPendingAt(link, os.Geteuid()); err == nil {
		t.Fatal("accepted symlink marker")
	}
	if _, err := keyVerificationPendingAt(link, os.Geteuid()); err == nil {
		t.Fatal("read symlink marker as pending")
	}
	if err := clearKeyVerificationPendingAt(link, os.Geteuid()); err == nil {
		t.Fatal("removed symlink marker")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "keep" {
		t.Fatal("symlink target changed")
	}
}

func TestVerificationRequiresSafeMarkerAndSuccessfulEvidence(t *testing.T) {
	for _, mode := range []string{"absent", "pending", "verified", "unavailable", "unsafe", "clear-failure"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pending")
			uid := os.Geteuid()
			if mode != "absent" {
				if err := ensureKeyVerificationPendingAt(path, uid); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "unsafe" {
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			unavailable := errors.New("journal unavailable")
			pending, verified, err := verifyAdminLoginAt(path, uid, func() (bool, error) {
				calls++
				if mode == "unavailable" {
					return false, unavailable
				}
				if mode == "clear-failure" {
					// A changed marker must not be removed even with successful evidence.
					if err := os.Chmod(path, 0644); err != nil {
						t.Fatal(err)
					}
				}
				return mode == "verified" || mode == "clear-failure", nil
			})
			wantError := mode == "unavailable" || mode == "unsafe" || mode == "clear-failure"
			wantPending := mode == "pending" || mode == "unavailable" || mode == "clear-failure"
			if (err != nil) != wantError || pending != wantPending || verified != (mode == "verified") {
				t.Fatalf("pending=%v verified=%v err=%v", pending, verified, err)
			}
			if mode == "unavailable" && !errors.Is(err, unavailable) {
				t.Fatalf("evidence error lost: %v", err)
			}
			wantCalls := 1
			if mode == "absent" || mode == "unsafe" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("journal reads=%d, want %d", calls, wantCalls)
			}
			_, statErr := os.Lstat(path)
			if (mode == "absent" || mode == "verified") != os.IsNotExist(statErr) {
				t.Fatalf("marker changed without authority: %v", statErr)
			}
		})
	}
}
