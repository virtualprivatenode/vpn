// internal/installer/sshstep.go

package installer

// The install-path SSH hardening step observes effective state, writes the
// current project drop-in, validates the merged configuration, and restarts
// sshd. Prior rlvpn drop-ins are lifecycle conflicts and never reach this step;
// v0.7.0 does not delete or migrate them.
//
// Directive election (ruling xvi(a)): the new drop-in writes
// PasswordAuthentication EXPLICITLY with the value observed
// seconds before, in the same process. Explicit-from-observed is
// STRONGER than the script's omission: our 00- file owns the
// setting from here on, so later provider/cloud-init drift
// cannot silently re-enable password auth. Only if the
// observation itself fails does the step degrade to the script's
// omission semantics, with a logged warning — preflight already
// proved sshd observable, so this failing mid-install means the
// environment regressed, and asserting nothing beats asserting a
// guess.

import (
	"github.com/virtualprivatenode/vpn/internal/host"

	"github.com/virtualprivatenode/vpn/internal/logger"
)

// installSSHHardening is the ssh.harden step.
func installSSHHardening() error {
	// 1. Observe — seconds before the write, same process.
	passwordAuth := ""
	obs, err := ObserveSSHState()
	if err != nil {
		logger.Install(
			"WARNING: sshd observation failed at the SSH step "+
				"(%v) — writing the drop-in WITHOUT a "+
				"PasswordAuthentication directive (script "+
				"omission semantics); password auth state is "+
				"whatever the rest of the sshd config elects", err)
	} else {
		if obs.PasswordAuth {
			passwordAuth = "yes"
		} else {
			passwordAuth = "no"
		}
	}

	if err := host.ApplySSHHardening(passwordAuth); err != nil {
		return err
	}
	if passwordAuth == "" {
		logger.Install("SSH hardening applied (root login " +
			"disabled; PasswordAuthentication directive omitted — " +
			"degraded mode)")
	} else {
		logger.Install("SSH hardening applied (root login "+
			"disabled; PasswordAuthentication %s, from observation)",
			passwordAuth)
	}
	return nil
}
