// internal/installer/sshd.go

package installer

import (
	"errors"
	"fmt"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
)

// BuildSSHHardeningConfig generates the complete contents
// of /etc/ssh/sshd_config.d/99-rlvpn-hardening.conf from
// AppConfig. Pure function — no side effects.
//
// Bootstrap writes the same static lines minus the
// PasswordAuthentication directive (it's intentionally
// silent so the user isn't locked out before logging in
// via TUI). Once the TUI's SSH Password Auth screen runs,
// this function takes over and the drop-in becomes the
// authoritative source for password auth state.
func BuildSSHHardeningConfig(cfg *config.AppConfig) string {
	passwordAuth := "yes"
	if cfg.SSHPasswordAuthDisabled {
		passwordAuth = "no"
	}

	return fmt.Sprintf(`# Virtual Private Node — SSH hardening
# Managed by the rlvpn TUI. Do not edit by hand.
PermitRootLogin no
PubkeyAuthentication yes
ChallengeResponseAuthentication no
KbdInteractiveAuthentication no
X11Forwarding no
PasswordAuthentication %s
`, passwordAuth)
}

// RebuildSSHHardeningConfig writes the drop-in to disk
// and restarts sshd. Idempotent.
func RebuildSSHHardeningConfig(cfg *config.AppConfig) error {
	content := BuildSSHHardeningConfig(cfg)
	if err := system.SudoWriteFile(
		paths.SSHDDropIn, []byte(content), 0644); err != nil {
		return fmt.Errorf(
			"write sshd drop-in: %w", err)
	}
	return restartSSHD()
}

// SetSSHPasswordAuth flips the password-auth flag in cfg
// and rebuilds the drop-in. Refuses to disable password
// auth when no SSH keys exist — that would leave the
// system with zero auth methods.
func SetSSHPasswordAuth(
	cfg *config.AppConfig, disabled bool,
) error {
	if disabled {
		keys, err := ListAuthorizedKeys()
		if err != nil {
			return fmt.Errorf(
				"check authorized_keys: %w", err)
		}
		if len(keys) == 0 {
			return errors.New(
				"cannot disable password auth with " +
					"no SSH keys configured — add a key " +
					"first")
		}
	}
	cfg.SSHPasswordAuthDisabled = disabled
	return RebuildSSHHardeningConfig(cfg)
}

func restartSSHD() error {
	// Try the systemd service named "sshd" first; some
	// distros use "ssh" as the unit name.
	if err := system.SudoRun(
		"systemctl", "restart", "sshd"); err == nil {
		return nil
	}
	return system.SudoRun("systemctl", "restart", "ssh")
}
