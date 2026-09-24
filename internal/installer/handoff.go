package installer

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
	"golang.org/x/term"
)

// HandoffToAdminConsole opens the owner TUI through a separate pseudo-terminal.
// runuser supplies the identity, login environment and PAM session. The original
// terminal retains its owner, and the original SSH login retains its identity.
// Installation is already complete; failure leaves connection instructions.
func HandoffToAdminConsole() {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		printConnectInstructions()
		return
	}
	cmd := exec.Command("/usr/sbin/runuser", "--pty", "--login", paths.AdminUser,
		"--command", paths.BinaryPath)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		logger.Install("handoff: owner console ended with error: %v", err)
		printConnectInstructions()
	}
}

// printConnectInstructions also handles unattended runs and unavailable terminals.
func printConnectInstructions() {
	ip := system.PublicIPv4()
	target := paths.AdminUser + "@<your-server-ip>"
	if ip != "" {
		target = paths.AdminUser + "@" + ip
	}
	fmt.Printf("\n  Install complete.\n\n  Connect to your node:\n\n    ssh %s\n\n", target)
}
