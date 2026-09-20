package host

import (
	"errors"
	"fmt"
	"os/user"

	"github.com/virtualprivatenode/vpn/internal/system"
)

const (
	lndUser       = "lnd"
	syncthingUser = "syncthing"
	backupGroup   = "vpn-lnd-backup"
)

// CreateSystemUser creates a non-login service identity when it is absent.
// Existing identities are left unchanged; callers own lifecycle admission.
func CreateSystemUser(username, home string) error {
	if _, err := user.Lookup(username); err == nil {
		return nil
	} else {
		var unknown user.UnknownUserError
		if !errors.As(err, &unknown) {
			return fmt.Errorf("look up system user %s: %w", username, err)
		}
	}
	return system.SudoRun("adduser",
		"--system", "--group",
		"--home", home,
		"--shell", "/usr/sbin/nologin",
		username)
}

func createSystemGroup(name string) error {
	if _, err := user.LookupGroup(name); err == nil {
		return nil
	} else {
		var unknown user.UnknownGroupError
		if !errors.As(err, &unknown) {
			return fmt.Errorf("look up system group %s: %w", name, err)
		}
	}
	return system.SudoRun("groupadd", "--system", name)
}
