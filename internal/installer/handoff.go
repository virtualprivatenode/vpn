// internal/installer/handoff.go

package installer

// The in-session root→admin identity drop (ruling xvi, handoff).
// At install completion the wizard flows straight into the node
// console: the root install process launches the TUI as the vpn
// user ON THE SAME TTY via a su-class mechanism, so the operator
// is never told to disconnect and come back.
//
// Mechanism notes for the live-run red-team (env scrub, tty
// ownership — flagged in ruling xvi):
//
//   - `su - vpn -c <binary>` provides the uid/gid/groups switch,
//     a PAM session, and a LOGIN environment — su's `-` clears
//     the caller's environment except TERM, which is exactly the
//     scrub we want (root's env never leaks into the admin
//     session; TERM survives for the TUI).
//   - su does NOT reassign tty ownership (login(1) does; su
//     doesn't). The inherited stdio fds work regardless, but a
//     fresh open of /dev/tty by the TUI stack would not — so the
//     handoff chowns the tty to the admin user for the session
//     and restores the original owner afterward.
//   - If any part fails at runtime the handoff degrades to
//     printing the connect instruction — the box is fully
//     installed at this point; only the convenience is lost. The
//     DESIGN fallback if the mechanism fails red-team scrutiny is
//     the watched-handoff screen specified in ruling xvi (swap
//     costs nothing; not built unless the live run demands it).
//
// This file also owns the first-run verification evidence: the
// TUI's "key verification pending" banner clears only when sshd's
// journal shows a real accepted login for the admin user — the
// in-session console is deliberately NOT evidence that SSH access
// works.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// HandoffToAdminConsole drops from root to the admin user on the
// current tty and runs the TUI there. Returns after the operator
// leaves the TUI. Never returns an error that should fail the
// install — the install is already complete; failures degrade to
// the printed connect line.
func HandoffToAdminConsole() {
	tty, restore, err := grantTTY(paths.AdminUser)
	if err != nil {
		logger.Install(
			"handoff: tty ownership not transferable (%v) — "+
				"printing connect instructions instead", err)
		printConnectInstructions()
		return
	}
	defer restore()

	logger.Install("handoff: dropping to %s on %s",
		paths.AdminUser, tty)
	cmd := exec.Command("su", "-", paths.AdminUser,
		"-c", paths.BinaryPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.Install("handoff: su returned error: %v", err)
		printConnectInstructions()
	}
}

// grantTTY chowns the controlling terminal to the given user for
// the handoff session and returns a restore function that puts
// the original owner back.
func grantTTY(username string) (string, func(), error) {
	tty, err := os.Readlink("/proc/self/fd/0")
	if err != nil || !strings.HasPrefix(tty, "/dev/") {
		return "", nil, fmt.Errorf(
			"stdin is not a tty (%q, %v)", tty, err)
	}
	st, err := os.Stat(tty)
	if err != nil {
		return "", nil, err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return "", nil, fmt.Errorf("no stat for %s", tty)
	}
	origUID, origGID := int(sys.Uid), int(sys.Gid)

	if err := system.SudoRun("chown",
		username+":tty", tty); err != nil {
		return "", nil, err
	}
	restore := func() {
		if err := system.SudoRun("chown", fmt.Sprintf(
			"%d:%d", origUID, origGID), tty); err != nil {
			logger.Install(
				"handoff: restoring tty owner failed: %v", err)
		}
	}
	return tty, restore, nil
}

// printConnectInstructions is the non-interactive completion
// message (unattended runs, or a degraded interactive handoff).
func printConnectInstructions() {
	ip := system.PublicIPv4()
	target := paths.AdminUser + "@<your-server-ip>"
	if ip != "" {
		target = paths.AdminUser + "@" + ip
	}
	fmt.Printf("\n  Install complete."+
		"\n\n  Connect to your node:\n\n    ssh %s\n\n", target)
}

// AdminLoginObserved reports whether sshd's journal shows an
// accepted login for the admin user. Evidence for clearing the
// first-run verification banner: the admin user exists only
// since this install, so ANY accepted login is a verified,
// independent way in — no time window needed. Errors (journal
// unreadable, -g unsupported) report false: the banner stays,
// which only nags, never locks out.
func AdminLoginObserved() bool {
	out, err := system.SudoRunOutput("journalctl",
		"-u", "ssh", "--no-pager", "-o", "cat",
		"-g", "Accepted")
	if err != nil {
		return false
	}
	return journalShowsAdminLogin(out)
}

// journalShowsAdminLogin scans journal lines for sshd's accepted-
// login record for the admin user ("Accepted publickey for vpn
// from …" / "Accepted password for vpn from …"). Pure —
// unit-tested.
func journalShowsAdminLogin(journal string) bool {
	needle := " for " + paths.AdminUser + " from "
	for _, line := range strings.Split(journal, "\n") {
		if strings.Contains(line, "Accepted ") &&
			strings.Contains(line, needle) {
			return true
		}
	}
	return false
}
