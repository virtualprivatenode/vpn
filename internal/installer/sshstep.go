// internal/installer/sshstep.go

package installer

// The install-path SSH hardening step (ruling xvi(a) + ruling
// xv's binding order). What the retired bootstrap script did with
// a heredoc, done with the observation discipline the sprint
// built:
//
//	observe → write new drop-in → delete old drop-in →
//	validate → restart
//
// The order is BINDING. On a migrated box, a TUI-disabled
// PasswordAuthentication lives in the OLD drop-in
// (00-rlvpn-hardening.conf) — deleting it before the observed
// value is captured into the new file would re-manufacture the
// day-one cfg lie in reverse (ruling xv). And because the old
// name sorts before the new one (r < v, first-match-wins), the
// brief window where both files exist is safe precisely because
// the new file carries the observed — that is, identical —
// value.
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
	"fmt"
	"os"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// installSSHHardening is the ssh.harden step.
func installSSHHardening(cfg *config.AppConfig) error {
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
		// Reality wins over any carried-over claim: on a
		// migrated box the two agree by construction (the old
		// drop-in still stands at observation time); if they
		// ever disagree, the cfg was the lie — correct it and
		// say so.
		disabled := !obs.PasswordAuth
		if cfg.SSHPasswordAuthDisabled != disabled {
			logger.Install(
				"config said ssh_password_auth_disabled=%v but "+
					"sshd's effective state is %v — config corrected "+
					"from observation",
				cfg.SSHPasswordAuthDisabled, disabled)
			cfg.SSHPasswordAuthDisabled = disabled
		}
	}

	// Capture both files' prior state so a failed validation
	// restores the box byte-for-byte: the new-name drop-in
	// (usually absent) and the old-name drop-in (present only
	// on migrated boxes).
	prevNew, prevNewExists, err := readDropIn(paths.SSHDDropIn)
	if err != nil {
		return fmt.Errorf("read current sshd drop-in: %w", err)
	}
	prevOld, prevOldExists, err := readDropIn(paths.OldSSHDDropIn)
	if err != nil {
		return fmt.Errorf("read old sshd drop-in: %w", err)
	}

	// 2. Write the new drop-in.
	content := buildHardeningDropIn(passwordAuth)
	if err := system.SudoWriteFile(
		paths.SSHDDropIn, []byte(content), 0644); err != nil {
		return fmt.Errorf("write sshd drop-in: %w", err)
	}

	// 3. Delete the old-name drop-in (the rename's ONLY
	// old-artifact removal — ruling xv).
	if prevOldExists {
		if err := system.SudoRun(
			"rm", "-f", paths.OldSSHDDropIn); err != nil {
			return fmt.Errorf(
				"remove old drop-in %s: %w",
				paths.OldSSHDDropIn, err)
		}
		logger.Install("removed stale drop-in %s (rename)",
			paths.OldSSHDDropIn)
	}

	// 4. Validate the merged config BEFORE any restart; restore
	// both files on rejection, so sshd keeps running its
	// current, valid config.
	if out, err := system.SudoRunCombinedOutput(
		"sshd", "-t"); err != nil {
		detail := strings.TrimSpace(out)
		restoreErr := restoreDropIns(
			prevNew, prevNewExists, prevOld, prevOldExists)
		if restoreErr != nil {
			return fmt.Errorf(
				"sshd rejected the new config (%s) and restoring "+
					"the previous drop-ins also failed (%v) — sshd "+
					"was NOT restarted and keeps running its current "+
					"config; inspect %s before restarting sshd",
				detail, restoreErr, paths.SSHDDropIn)
		}
		return fmt.Errorf(
			"sshd rejected the new config, previous drop-ins "+
				"restored, sshd not restarted: %s", detail)
	}

	// 5. Restart.
	if err := restartSSHD(); err != nil {
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

// readDropIn reads a drop-in, distinguishing absent (fine) from
// unreadable (abort — an unreadable prior state could not be
// restored after a failed validation).
func readDropIn(path string) ([]byte, bool, error) {
	data, err := system.SudoReadFile(path)
	if err != nil {
		if os.IsNotExist(err) ||
			strings.Contains(err.Error(), "No such file") {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

// restoreDropIns puts both drop-in files back to their captured
// pre-step state.
func restoreDropIns(
	prevNew []byte, newExisted bool,
	prevOld []byte, oldExisted bool,
) error {
	var firstErr error
	if newExisted {
		if err := system.SudoWriteFile(
			paths.SSHDDropIn, prevNew, 0644); err != nil {
			firstErr = err
		}
	} else {
		if err := system.SudoRun(
			"rm", "-f", paths.SSHDDropIn); err != nil &&
			firstErr == nil {
			firstErr = err
		}
	}
	if oldExisted {
		if err := system.SudoWriteFile(
			paths.OldSSHDDropIn, prevOld, 0644); err != nil &&
			firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
