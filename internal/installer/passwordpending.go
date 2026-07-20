// internal/installer/passwordpending.go

package installer

// The password-pending marker closes a fail-then-resume hole on
// the unattended path (live-run finding): the identity step
// APPLIES the generated admin password early in the run, but the
// password is PRINTED only at the very end of a completed run. A
// run that fails between the two leaves the box holding a
// recovery password nobody has ever seen — and the resuming pass
// ledger-skips the identity step, so without outside help it
// would complete without ever showing a working credential.
//
// The marker is that outside help: a root-owned flag file whose
// presence means "a generated admin password is applied on this
// box but was never displayed". A completing unattended pass
// that finds it re-applies its own freshly generated password
// and prints that one instead. The operator setting a password
// of their own from the node console clears it too — at that
// point they hold a credential they chose. The file carries no
// secret; plaintext passwords exist only in process memory.

import (
	"os"

	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

const passwordPendingNote = `An unattended install applied a generated admin login password
that has not been displayed yet. A completed unattended run
resolves this automatically; setting a new password from the
node console also clears it.
`

// markPasswordPending records that a generated password was
// applied but not yet displayed. Failure is returned, not
// swallowed: this marker is what guarantees the credential can
// still reach the operator if the run dies before the print.
func markPasswordPending() error {
	return system.SudoWriteFile(
		paths.PasswordPendingMarker,
		[]byte(passwordPendingNote), 0600)
}

// passwordPending reports whether the marker is present.
func passwordPending() bool {
	_, err := os.Stat(paths.PasswordPendingMarker)
	return err == nil
}

// ClearPasswordPendingMarker removes the marker. Best-effort by
// design: it runs at moments when the operator HAS a working
// credential (it was just printed, or they just chose one), so a
// failed removal must not fail that operation — the cost of a
// stale marker is one redundant password re-apply and re-print
// on a later completing run.
func ClearPasswordPendingMarker() {
	if err := os.Remove(paths.PasswordPendingMarker); err != nil &&
		!os.IsNotExist(err) {
		logger.Install(
			"WARNING: could not remove %s (%v) — a later install "+
				"re-run may re-apply and print a fresh password",
			paths.PasswordPendingMarker, err)
	}
}

// needsPasswordReapply decides whether a completing unattended
// pass must re-apply its generated password so the printed
// credential is one that works. True only when this pass
// generated a password (unattended), the identity step did NOT
// apply it (ledger-skipped on a resume), and the marker says an
// earlier pass left one undisplayed. A re-run after a completed
// install has no marker, so it re-applies and prints nothing —
// the password the operator saved (or later changed) stands.
// Pure — unit-tested.
func needsPasswordReapply(
	generated string, appliedThisPass, pendingOnDisk bool,
) bool {
	return generated != "" && !appliedThisPass && pendingOnDisk
}
