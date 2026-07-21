// internal/helper/board.go

package helper

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/virtualprivatenode/vpn/internal/paths"
)

// ── The staging board ────────────────────────────────────
//
// /etc/vpn/state holds one file per privileged FACT the TUI
// needs to display or use: onion hostnames, the staged bitcoind
// RPC password, copies of LND's TLS certificate and admin
// macaroon, Syncthing's API key and device ID, and the observed
// SSH password-auth state. Root writes them; the admin user
// (group vpn) reads them; nothing else can see them.
//
// The contract that keeps the board trustworthy:
//
//   - every file is (re)written by whatever operation changes
//     the fact it carries — at install, and afterwards by the
//     helper operation that performs the change. The full
//     write-map lives in internal/helperd/matrix.go and is
//     enforced by unit tests there.
//   - readers NEVER guess. A missing or unreadable file means
//     the feature renders as unavailable with a logged reason,
//     not a stale or default value. Staleness must surface as
//     visible breakage.
//   - nothing wallet-critical lives here. The seed, wallet.db,
//     and the auto-unlock password file are NOT board facts;
//     the admin macaroon copy is a revocable credential the
//     admin user needs to operate the node (it is the same
//     authority every wallet screen already exercises).

// ReadBoard reads one staging-board file. The error is written
// for surfacing to the operator: it names the file and points
// at the helper's journal, because a missing board file means a
// staging step failed or was skipped — a real defect to report,
// not a condition to paper over.
func ReadBoard(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf(
			"staged fact %s unavailable (%v) — a privileged "+
				"operation should have staged it; check "+
				"journalctl -u vpn-helperd and the install log",
			filepath.Base(path), err)
	}
	return data, nil
}

// ReadBoardString reads a board file as a trimmed string
// (hostname files and the API key are single lines).
func ReadBoardString(path string) (string, error) {
	data, err := ReadBoard(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// ── Root-side writer ─────────────────────────────────────

// vpnGID resolves the admin group's numeric gid. The group may
// legitimately not exist yet during the bake phase of an image
// build (the admin user is created at first boot) — the caller
// falls back to root-only modes and the first-boot staging step
// re-applies ownership.
func vpnGID() (int, bool) {
	g, err := user.LookupGroup(paths.AdminUser)
	if err != nil {
		return 0, false
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, false
	}
	return gid, true
}

// EnsureBoardDir creates the board directory with root:vpn
// 0750 (root:root 0700 before the admin group exists).
// Idempotent — re-applies ownership and mode so a bake-phase
// directory is corrected at first boot. As belt and braces on
// top of the root-owned-ancestors guarantee, it refuses to
// operate on anything that is not a real directory owned by
// root.
func EnsureBoardDir() error {
	if err := os.MkdirAll(paths.VarLibVPN, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", paths.VarLibVPN, err)
	}
	if err := os.MkdirAll(paths.StateDir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", paths.StateDir, err)
	}
	fi, err := os.Lstat(paths.StateDir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", paths.StateDir, err)
	}
	if !fi.Mode().IsDir() {
		return fmt.Errorf(
			"%s is not a directory — refusing to stage",
			paths.StateDir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Uid != 0 {
		return fmt.Errorf(
			"%s is not owned by root — refusing to stage",
			paths.StateDir)
	}
	gid, ok := vpnGID()
	if !ok {
		return os.Chmod(paths.StateDir, 0o700)
	}
	if err := os.Chown(paths.StateDir, 0, gid); err != nil {
		return fmt.Errorf("chown %s: %w", paths.StateDir, err)
	}
	return os.Chmod(paths.StateDir, 0o750)
}

// WriteBoard writes one staging-board file atomically: same-dir
// temp, explicit chmod (the open mode is filtered by umask),
// chown root:vpn, then rename onto the final path — a reader
// never sees a partial file or a loose mode. Runs as root only.
func WriteBoard(path string, data []byte) error {
	if err := EnsureBoardDir(); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(paths.StateDir,
		"."+filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("stage %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op after successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("stage %s: write: %w", path, err)
	}
	if err := tmp.Chmod(0o640); err != nil {
		tmp.Close()
		return fmt.Errorf("stage %s: chmod: %w", path, err)
	}
	if gid, ok := vpnGID(); ok {
		if err := tmp.Chown(0, gid); err != nil {
			tmp.Close()
			return fmt.Errorf("stage %s: chown: %w", path, err)
		}
	} else if err := tmp.Chmod(0o600); err != nil {
		// No admin group yet (bake phase): keep it root-only;
		// the first-boot staging step re-owns every board file.
		tmp.Close()
		return fmt.Errorf("stage %s: chmod: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("stage %s: sync: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("stage %s: close: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("stage %s: rename: %w", path, err)
	}
	return nil
}

// RemoveBoard removes a board file (used when the fact it
// carries ceases to exist, e.g. credentials for a component
// that was removed). A missing file is success.
func RemoveBoard(path string) error {
	if err := os.Remove(path); err != nil &&
		!os.IsNotExist(err) {
		return fmt.Errorf("remove staged fact %s: %w", path, err)
	}
	return nil
}

// FixBoardOwnership re-applies root:vpn 0640 to every existing
// board file and 0750 to the directory. The first-boot staging
// step calls this so files written during an image bake (before
// the admin group existed) become readable once the group does.
func FixBoardOwnership() error {
	if err := EnsureBoardDir(); err != nil {
		return err
	}
	gid, ok := vpnGID()
	if !ok {
		return fmt.Errorf(
			"admin group %q does not exist", paths.AdminUser)
	}
	entries, err := os.ReadDir(paths.StateDir)
	if err != nil {
		return fmt.Errorf("list %s: %w", paths.StateDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(paths.StateDir, e.Name())
		if err := os.Chown(p, 0, gid); err != nil {
			return fmt.Errorf("chown %s: %w", p, err)
		}
		if err := os.Chmod(p, 0o640); err != nil {
			return fmt.Errorf("chmod %s: %w", p, err)
		}
	}
	return nil
}
