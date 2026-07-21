// internal/helperd/matrix.go

package helperd

import (
	"fmt"

	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/installer"
	"github.com/virtualprivatenode/vpn/internal/paths"
)

// ── The freshness matrix ─────────────────────────────────
//
// The staging board's one failure mode is staleness: an
// operation changes reality and the board file carrying the old
// fact survives. The defense is this table. It maps each verb
// to the board facts that verb invalidates, and restage(verb)
// — called by every handler as part of its postcondition — is
// driven BY the table, so the mapping is the mechanism, not
// documentation that can drift from it.
//
// Reading the matrix: a verb not listed here invalidates no
// staged fact (a service restart changes no credential; a
// package upgrade changes no onion address). The unit tests in
// matrix_test.go walk every cell: every listed fact has a
// stager, every stager is reachable from a verb or the
// installer's staging step, and the sets match this table
// exactly.

// stagers maps each board file to the function that refreshes
// it from current reality. All run as root.
var stagers = map[string]func() error{
	paths.StateOnionBitcoinP2P: func() error {
		return installer.StageOnionHostname(
			paths.TorBitcoinP2P+"/hostname",
			paths.StateOnionBitcoinP2P)
	},
	paths.StateOnionLNDGRPC: func() error {
		return installer.StageOnionHostname(
			paths.TorLNDGRPC+"/hostname",
			paths.StateOnionLNDGRPC)
	},
	paths.StateOnionLNDREST: func() error {
		return installer.StageOnionHostname(
			paths.TorLNDRESTHostname,
			paths.StateOnionLNDREST)
	},
	paths.StateOnionSyncthing: func() error {
		return installer.StageOnionHostname(
			paths.TorSyncthingHostname,
			paths.StateOnionSyncthing)
	},
	paths.StateLNDTLSCert:      installer.StageLNDTLSCert,
	paths.StateLNDMacaroon:     installer.StageLNDMacaroon,
	paths.StateSyncthingAPIKey: installer.StageSyncthingAPIKey,
	paths.StateSyncthingDevID:  installer.StageSyncthingDeviceID,
	paths.StateSSHPasswordAuth: installer.StageSSHAuthFact,
}

// freshnessMatrix: verb → board facts the verb invalidates and
// therefore re-stages on success.
var freshnessMatrix = map[string][]string{
	// Rebuilding torrc restarts Tor; hidden-service dirs are
	// (re)created and hostnames may newly exist. Onion KEYS
	// persist, so existing addresses do not change — but a
	// toggled-on service's hostname appears here for the first
	// time, and re-staging the unchanged ones is free.
	helper.VerbRebuildTorConfig: {
		paths.StateOnionBitcoinP2P,
		paths.StateOnionLNDGRPC,
		paths.StateOnionLNDREST,
		paths.StateOnionSyncthing,
	},
	// Wallet creation mints fresh macaroons; the staging verb
	// exists exactly for that moment (and re-copies the cert,
	// which is free).
	helper.VerbStageLNDCredentials: {
		paths.StateLNDTLSCert,
		paths.StateLNDMacaroon,
	},
	// A P2P mode change alters the cert's contents (LND
	// regenerates it via tlsautorefresh), so the staged copy
	// is stale the moment LND restarts.
	helper.VerbSetP2PMode: {
		paths.StateLNDTLSCert,
		paths.StateLNDMacaroon,
	},
	// A fresh Syncthing install generates a new identity and
	// API key, and its Tor rebuild creates the web-UI onion.
	helper.VerbSyncthingInstall: {
		paths.StateSyncthingAPIKey,
		paths.StateSyncthingDevID,
		paths.StateOnionSyncthing,
	},
	// The SSH drop-in write changes effective password-auth
	// state; the staged observation must follow it.
	helper.VerbRebuildSSHConfig: {
		paths.StateSSHPasswordAuth,
	},
}

// restage refreshes every fact the given verb invalidates.
// Failures here fail the VERB: an operation that succeeded but
// left a stale board would be exactly the silent divergence the
// board's contract forbids — better to surface it now, with the
// journal naming the file.
func restage(verb string) error {
	for _, file := range freshnessMatrix[verb] {
		st, ok := stagers[file]
		if !ok {
			return fmt.Errorf(
				"no stager for %s (defect: matrix and stagers "+
					"disagree)", file)
		}
		if err := st(); err != nil {
			return fmt.Errorf("restage %s: %w", file, err)
		}
	}
	return nil
}
