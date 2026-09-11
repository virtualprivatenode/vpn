package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/virtualprivatenode/vpn/internal/loginpassword"
	"github.com/virtualprivatenode/vpn/internal/paths"
)

// SetLoginPassword is shared by installation and the root helper. Its bound is
// independent of the requesting TUI: disconnecting does not cancel an accepted
// change. An interrupted process may already have changed the password.
func SetLoginPassword(password loginpassword.Password) error {
	if os.Geteuid() != 0 {
		return errors.New("login password changes require the root helper")
	}
	return runLoginPassword(password, 45*time.Second, func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "/usr/sbin/chpasswd")
	})
}

func runLoginPassword(password loginpassword.Password, timeout time.Duration, command func(context.Context) *exec.Cmd) error {
	if password.Text() == "" {
		return errors.New("password was not validated")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := command(ctx)
	cmd.Stdin = strings.NewReader(paths.AdminUser + ":" + password.Text() + "\n")
	// Do not return tool output: PAM modules may include sensitive input in it.
	// WaitDelay also bounds pipes retained by a descendant after the tool exits.
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("login password change was not confirmed: %w", ctx.Err())
		}
		return fmt.Errorf("login password change was not confirmed: %w", err)
	}
	return nil
}

// ClearPasswordPendingMarker acknowledges an operator-chosen password. This is
// best effort after a successful change; installer completion uses strict cleanup.
func ClearPasswordPendingMarker() error {
	if os.Geteuid() != 0 {
		return errors.New("password delivery state requires the root helper")
	}
	err := os.Remove(paths.PasswordPendingMarker)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
