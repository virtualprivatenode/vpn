// internal/installer/runlock_test.go

package installer

import (
	"path/filepath"
	"strings"
	"testing"
)

// Two acquisitions of the same lock path must conflict — flock
// is per open file description, so a second open in the same
// process contends exactly like a second process would — and
// the refusal message must say what is happening in operator
// language.
func TestRunLockExcludesSecondAcquire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")

	first, err := acquireRunLock(path)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.Close()

	if _, err := acquireRunLock(path); err == nil {
		t.Fatal("second acquire succeeded while first is held")
	} else if !strings.Contains(err.Error(), "already running") {
		t.Errorf("refusal message %q lacks the diagnosis", err)
	}

	// Releasing the first makes the lock available again.
	first.Close()
	third, err := acquireRunLock(path)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	third.Close()
}
