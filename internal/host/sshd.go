// Package host owns privileged operations on the local node.
package host

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/sshkeys"
	"github.com/virtualprivatenode/vpn/internal/system"
)

func buildHardeningDropIn(passwordAuth string) string {
	base := `# Virtual Private Node — SSH hardening
# Managed by the vpn TUI. Do not edit by hand.
PermitRootLogin no
PubkeyAuthentication yes
ChallengeResponseAuthentication no
KbdInteractiveAuthentication no
X11Forwarding no
`
	if passwordAuth == "" {
		return base
	}
	return base + "PasswordAuthentication " + passwordAuth + "\n"
}

// ApplySSHHardening preserves the installer's observed yes/no state. Empty omits
// the directive when observation failed. Runtime changes use RebuildSSHHardeningConfig.
func ApplySSHHardening(passwordAuth string) error {
	if os.Geteuid() != 0 {
		return errors.New("SSH configuration requires the root helper")
	}
	return applySSHHardening(passwordAuth, sshdOps{
		read:  func() ([]byte, error) { return os.ReadFile(paths.SSHDDropIn) },
		write: func(b []byte) error { return system.SudoWriteFile(paths.SSHDDropIn, b, 0644) },
		remove: func() error {
			err := os.Remove(paths.SSHDDropIn)
			if os.IsNotExist(err) {
				return nil
			}
			return err
		},
		validate: func() (string, error) { return system.SudoRunCombinedOutput("sshd", "-t") },
		restart:  restartSSHD,
	})
}

type sshdOps struct {
	read     func() ([]byte, error)
	write    func([]byte) error
	remove   func() error
	validate func() (string, error)
	restart  func() error
}

// Both installation and runtime changes validate before restarting and restore
// the previous file on validation failure. Restart failure leaves state uncertain.
func applySSHHardening(passwordAuth string, ops sshdOps) error {
	if passwordAuth != "" && passwordAuth != "yes" && passwordAuth != "no" {
		return errors.New("invalid password authentication setting")
	}
	prev, err := ops.read()
	existed := !os.IsNotExist(err)
	if err != nil && existed {
		return fmt.Errorf("read current sshd drop-in: %w", err)
	}
	if err = ops.write([]byte(buildHardeningDropIn(passwordAuth))); err != nil {
		return fmt.Errorf("write sshd drop-in: %w", err)
	}
	if out, err := ops.validate(); err != nil {
		var restoreErr error
		if existed {
			restoreErr = ops.write(prev)
		} else {
			restoreErr = ops.remove()
		}
		if restoreErr != nil {
			return fmt.Errorf("sshd rejected the config (%s); restoring %s failed (%v); sshd was not restarted; inspect the file before restarting", strings.TrimSpace(out), paths.SSHDDropIn, restoreErr)
		}
		return fmt.Errorf("sshd rejected the config; previous drop-in restored; sshd not restarted: %s", strings.TrimSpace(out))
	}
	return ops.restart()
}

// The helper checks fresh, structurally valid keys while holding the same lock
// as application key edits. This cannot prove that a key actually permits login.
func RebuildSSHHardeningConfig(disabled bool) error {
	if os.Geteuid() != 0 {
		return errors.New("SSH configuration requires the root helper")
	}
	store := sshkeys.Store{Path: paths.AuthorizedKeysFile}
	return rebuildSSHConfig(disabled, store, ApplySSHHardening, EffectiveSSHPasswordAuth)
}

func rebuildSSHConfig(disabled bool, store sshkeys.Store, apply func(string) error, observe func() (bool, error)) error {
	change := func() error {
		auth := "yes"
		if disabled {
			data, err := store.Read()
			if err != nil {
				return err
			}
			if len(sshkeys.Keys(data)) == 0 {
				return errors.New("refusing to disable password authentication with no supported SSH keys; add and test a key first")
			}
			auth = "no"
		}
		if err := apply(auth); err != nil {
			return err
		}
		enabled, err := observe()
		if err != nil {
			return fmt.Errorf("verify effective SSH password authentication: %w", err)
		}
		if enabled == disabled {
			return errors.New("effective SSH password authentication does not match the requested setting")
		}
		return nil
	}
	if disabled {
		return store.WithLock(change)
	}
	return change()
}

// EffectiveSSHPasswordAuth samples sshd's resolved configuration for the operator
// at localhost. Address-specific Match rules can differ for an actual connection;
// this observation is not proof that a remote password login will succeed.
func EffectiveSSHPasswordAuth() (bool, error) {
	if os.Geteuid() != 0 {
		return false, errors.New("SSH configuration requires the root helper")
	}
	out, err := system.SudoRunOutput("sshd", "-T", "-C", "user="+paths.AdminUser+",host=localhost,addr=127.0.0.1")
	if err != nil {
		return false, fmt.Errorf("query effective sshd config: %w", err)
	}
	return parsePasswordAuth(out)
}

func parsePasswordAuth(output string) (bool, error) {
	found, enabled := false, false
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.EqualFold(fields[0], "passwordauthentication") {
			continue
		}
		if found || len(fields) != 2 || (!strings.EqualFold(fields[1], "yes") && !strings.EqualFold(fields[1], "no")) {
			return false, errors.New("invalid passwordauthentication in sshd output")
		}
		found = true
		enabled = strings.EqualFold(fields[1], "yes")
	}
	if !found {
		return false, errors.New("passwordauthentication not present in sshd output")
	}
	return enabled, nil
}

func restartSSHD() error {
	if err := system.SudoRun("systemctl", "restart", "sshd"); err == nil {
		return nil
	}
	return system.SudoRun("systemctl", "restart", "ssh")
}
