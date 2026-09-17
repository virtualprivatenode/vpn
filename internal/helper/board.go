// internal/helper/board.go

package helper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ── The staging board ────────────────────────────────────
//
// /var/lib/vpn/state holds one file per privileged FACT the TUI
// needs to use: the staged bitcoind RPC password, copies of LND's TLS
// certificate and admin macaroon, Syncthing's API key, and the generated
// Syncthing web password. Root writes them; the admin user
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
//   - nothing wallet-authoritative lives here. The seed, LND databases,
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
