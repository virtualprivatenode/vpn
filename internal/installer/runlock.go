// internal/installer/runlock.go

package installer

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// acquireRunLock takes an exclusive advisory lock on the
// install ledger, so two concurrent `sudo vpn install` runs
// cannot interleave step execution and ledger writes. The
// second run refuses immediately with a clear message.
//
// flock, not a pid file, deliberately: the lock belongs to the
// open file description, so it evaporates the instant the
// process dies — by any death, including SIGKILL — and there is
// no stale-lock state to clean up. The caller must keep the
// returned *os.File referenced until the install returns
// (dropping it lets the runtime close the file and release the
// lock mid-install), and must NOT delete the file on release —
// unlinking a lock file creates the classic two-holders race.
// The ledger is a persistent file anyway.
func acquireRunLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open install lock %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()),
		syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf(
				"another `vpn install` is already running " +
					"(the install ledger is locked) — wait for it " +
					"to finish")
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return f, nil
}
