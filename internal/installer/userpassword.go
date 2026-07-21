// internal/installer/userpassword.go

package installer

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// MinLoginPasswordLen is the minimum length for the admin
// user's login password. UI screens import this for their
// hint copy; enforcement lives in NewLoginPassword so the
// two can never disagree.
const MinLoginPasswordLen = 16

// LoginPassword is a validated login password. The only way
// to obtain a non-zero LoginPassword is NewLoginPassword,
// so any value that reaches a privileged function has
// already passed validation. The zero value is unvalidated
// and is rejected by SetUserPassword.
type LoginPassword struct {
	v string
}

// NewLoginPassword validates s and returns it as a
// LoginPassword. Policy: at least MinLoginPasswordLen
// characters and no newline (chpasswd's stdin protocol is
// line-based, so an embedded newline would be interpreted
// as a second, malformed change request).
//
// This is the ONLY enforcement point for password quality.
// chpasswd runs as root here, and root-invoked password
// changes bypass PAM's quality rules (confirmed on the
// target image: a 5-character password was accepted with
// exit code 0). There is no PAM backstop.
func NewLoginPassword(s string) (LoginPassword, error) {
	if s == "" {
		return LoginPassword{},
			errors.New("password is empty")
	}
	if strings.ContainsAny(s, "\n") {
		return LoginPassword{},
			errors.New("password has newline")
	}
	if len(s) < MinLoginPasswordLen {
		return LoginPassword{}, fmt.Errorf(
			"password must be at least %d characters",
			MinLoginPasswordLen)
	}
	return LoginPassword{v: s}, nil
}

// String implements fmt.Stringer so that formatting a
// LoginPassword with %v or %s can never leak the password
// into a log line or error message.
func (LoginPassword) String() string { return "[redacted]" }

// GoString implements fmt.GoStringer so %#v is covered too.
func (LoginPassword) GoString() string { return "[redacted]" }

// SetUserPassword changes the given user's login password.
// As root it pipes to chpasswd's stdin directly (the password
// never appears in argv, which would leak via /proc/*/cmdline
// or `ps`); from the unprivileged TUI it requests the helper's
// typed set-user-password operation, which re-validates with
// this same package's constructor and runs the identical
// chpasswd path root-side.
//
// The password arrives as a LoginPassword, so validation has
// already happened at construction; the zero-value check below
// closes the one remaining hole (a zero LoginPassword was
// never validated).
func SetUserPassword(
	username string, password LoginPassword,
) error {
	if username == "" {
		return errors.New("username is empty")
	}
	if strings.ContainsAny(username, ":\n") {
		return errors.New("username has invalid chars")
	}
	if password.v == "" {
		return errors.New(
			"password was not validated " +
				"(zero LoginPassword)")
	}

	if os.Geteuid() != 0 {
		return helper.Call(helper.VerbSetUserPassword,
			helper.SetUserPasswordParams{
				User:     username,
				Password: password.v,
			}, nil)
	}

	out, err := system.SudoRunStdin(
		username+":"+password.v+"\n", "chpasswd")
	if err != nil {
		if out == "" {
			return fmt.Errorf("chpasswd: %w", err)
		}
		return fmt.Errorf("chpasswd: %s", out)
	}
	return nil
}
