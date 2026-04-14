// internal/installer/userpassword.go

package installer

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// SetUserPassword changes the given user's login password
// via `sudo chpasswd`. The password is piped to chpasswd's
// stdin so it never appears in argv (which would leak via
// /proc/*/cmdline or `ps`).
//
// PAM rules apply — if the password is too short, weak,
// or otherwise rejected by /etc/pam.d/common-password,
// chpasswd will fail and we surface its stderr verbatim.
func SetUserPassword(username, newPassword string) error {
	if username == "" {
		return errors.New("username is empty")
	}
	if newPassword == "" {
		return errors.New("password is empty")
	}
	if strings.ContainsAny(username, ":\n") {
		return errors.New("username has invalid chars")
	}
	if strings.ContainsAny(newPassword, "\n") {
		return errors.New("password has newline")
	}

	cmd := exec.Command("sudo", "chpasswd")
	cmd.Stdin = strings.NewReader(
		username + ":" + newPassword + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("chpasswd: %w", err)
		}
		return fmt.Errorf("chpasswd: %s", msg)
	}
	return nil
}
