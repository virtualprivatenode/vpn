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

	if err := system.RunRoot("chown",
		username+":tty", tty); err != nil {
		return "", nil, err
	}
	restore := func() {
		if err := system.RunRoot("chown", fmt.Sprintf(
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

// printConsoleOnlyInstructions replaces the ssh hint when the
// completed install left no SSH way in — no keys copied and
// password login over SSH observed off, the end state consented
// to by name with --allow-console-only. An ssh line here would
// be affirmatively misleading: it cannot work until the
// operator adds a key or enables password login. This is a
// statement of the reached end state, not a warning — the
// consent already happened, on the command line.
func printConsoleOnlyInstructions() {
	fmt.Printf("\n  Install complete."+
		"\n\n  This box has no SSH way in: no SSH keys were"+
		"\n  installed and password login over SSH is disabled."+
		"\n  Log in as %q at your provider console with the"+
		"\n  login password, then add an SSH key or enable"+
		"\n  password login from the node TUI (System)."+
		"\n\n", paths.AdminUser)
}
