package host

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/sshkeys"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// CreateOperatorAccess creates the operator account and installs confirmed keys
// during an admitted base installation. It grants no sudo access. Password
// application and delivery remain separate so installer can record partial work.
func CreateOperatorAccess(keys []sshkeys.Key) error {
	if os.Geteuid() != 0 {
		return errors.New("initial operator access requires root")
	}
	for _, key := range keys {
		if _, err := sshkeys.Parse(key.RawLine); err != nil {
			return fmt.Errorf("invalid confirmed SSH key: %w", err)
		}
	}
	if err := ensureOperatorAccount(user.Lookup, system.SudoRun); err != nil {
		return err
	}

	// Preserve the installer's removal of this fixed sudoers path. Lifecycle
	// admission refuses a pre-existing unmarked grant; this operation does not
	// inventory or establish ownership of independent host sudo policy.
	if err := os.Remove(paths.AdminSudoers); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old sudoers rule %s: %w", paths.AdminSudoers, err)
	}
	if len(keys) == 0 {
		logger.Install("admin access: no SSH keys configured (password login)")
		return nil
	}

	sshDir := paths.AdminHome + "/.ssh"
	if err := system.SudoRun("mkdir", "-p", sshDir); err != nil {
		return fmt.Errorf("mkdir %s: %w", sshDir, err)
	}
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key.RawLine)
		b.WriteString("\n")
	}
	if err := system.SudoWriteFile(paths.AuthorizedKeysFile, []byte(b.String()), 0600); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	if err := system.SudoRun("chown", "-R", paths.AdminUser+":"+paths.AdminUser, sshDir); err != nil {
		return err
	}
	if err := system.SudoRun("chmod", "700", sshDir); err != nil {
		return err
	}
	logger.Install("admin access: %d key(s) written for %s", len(keys), paths.AdminUser)
	return nil
}

// Keep lookup and creation together: failure to observe an account is not proof
// of absence. The narrow dependencies let refusal tests detect creation attempts.
func ensureOperatorAccount(lookup func(string) (*user.User, error), run func(string, ...string) error) error {
	if _, err := lookup(paths.AdminUser); err == nil {
		return nil
	} else {
		var unknown user.UnknownUserError
		if !errors.As(err, &unknown) {
			return fmt.Errorf("look up operator account %s: %w", paths.AdminUser, err)
		}
	}
	if err := run("adduser", "--disabled-password", "--gecos", "Virtual Private Node", paths.AdminUser); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	return nil
}

// ConfigureOperatorAutoLaunch opens the node TUI for interactive SSH logins.
// Installer calls it after recording successful password application.
func ConfigureOperatorAutoLaunch() error {
	profile := `# Virtual Private Node — auto-launch
if [ -n "$SSH_CONNECTION" ] && [ -t 0 ]; then
    ` + paths.BinaryPath + `
fi

# Source .bashrc after the TUI exits (cli wrappers live there)
[ -f ~/.bashrc ] && source ~/.bashrc
`
	if err := system.SudoWriteFile(paths.AdminBashProfile, []byte(profile), 0644); err != nil {
		return fmt.Errorf("write .bash_profile: %w", err)
	}
	return system.SudoRun("chown", paths.AdminUser+":"+paths.AdminUser, paths.AdminBashProfile)
}

// SetOperatorLogOwnership gives the operator its application log. System
// configuration and its parent remain root-owned and are never made writable.
func SetOperatorLogOwnership() error {
	owner := paths.AdminUser + ":" + paths.AdminUser
	if err := system.SudoRun("chown", owner, paths.LogFile); err != nil {
		return fmt.Errorf("chown %s to %s: %w", paths.LogFile, owner, err)
	}
	return nil
}
