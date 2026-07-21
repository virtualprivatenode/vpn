// internal/helper/wire.go

// Package helper is the unprivileged side of the node's
// privilege boundary. It holds three things:
//
//   - the wire protocol shared by the root helper (vpn helperd)
//     and its clients: one JSON request line per connection,
//     answered by zero or more progress events and exactly one
//     ok/error terminator (wire.go);
//   - the client the TUI uses to request privileged operations
//     over the helper's unix socket (client.go);
//   - the staging-board reader and writer: the root-written
//     files under /etc/vpn/state that carry privileged facts
//     (onion hostnames, staged credentials) to the admin user
//     without any privileged code running on the read path
//     (board.go).
//
// The package deliberately does NOT import the installer: the
// installer imports this package to write board files, and the
// helper daemon (internal/helperd) imports both.
package helper

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Verb names. The helper serves this fixed menu and nothing
// else — there is no verb that runs a caller-supplied command,
// and no verb that returns a caller-chosen file. Every verb
// validates its parameters against a closed set of accepted
// values before touching the system.
const (
	// Simple verbs: one request, one terminator.
	VerbServiceAction        = "service-action"
	VerbReboot               = "reboot"
	VerbDirSize              = "dir-size"
	VerbSetUserPassword      = "set-user-password"
	VerbStageWalletPassword  = "stage-wallet-password"
	VerbRemoveWalletPassword = "remove-wallet-password"
	VerbStageLNDCredentials  = "stage-lnd-credentials"
	VerbRebuildSSHConfig     = "rebuild-ssh-config"
	VerbRebuildTorConfig     = "rebuild-tor-config"

	// Streaming verbs: step progress events precede the
	// terminator, and feed the TUI's step renderer.
	VerbPackageUpdate    = "package-update"
	VerbSelfUpdate       = "self-update"
	VerbSetP2PMode       = "set-p2p-mode"
	VerbSyncthingInstall = "syncthing-install"
)

// Request is the single line a client writes after connecting.
type Request struct {
	Verb   string          `json:"verb"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Event is one response line from the helper. Two kinds:
//
//   - "step": progress from a streaming verb. Index is the
//     0-based position in the verb's fixed step list; Err is
//     non-empty if that step failed (a failed step is always
//     followed by an error terminator).
//   - "end": the terminator. Exactly one per connection. OK
//     with an optional Result payload, or an Error message.
type Event struct {
	Event string `json:"event"` // "step" or "end"

	// step fields
	Index int    `json:"index,omitempty"`
	Err   string `json:"err,omitempty"`

	// end fields
	OK     bool            `json:"ok,omitempty"`
	Error  string          `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

// ── Typed parameters ─────────────────────────────────────
//
// Every value that crosses the privilege boundary is a field in
// one of these structs, and the helper validates each against a
// closed set (see internal/helperd). The structs live here so
// client and server marshal/unmarshal the same shape.

// ServiceActionParams: systemctl <action> <unit>. Both fields
// are validated against closed sets on the root side.
type ServiceActionParams struct {
	Unit   string `json:"unit"`
	Action string `json:"action"`
}

// DirSizeParams: which data directory to measure. "lnd" is the
// only accepted value — the bitcoin data dir's size comes from
// bitcoind's own getblockchaininfo (size_on_disk) over RPC,
// with no privileged operation at all.
type DirSizeParams struct {
	Which string `json:"which"`
}

// DirSizeResult carries the measured size back, already
// human-formatted ("12G").
type DirSizeResult struct {
	Size string `json:"size"`
}

// SetUserPasswordParams: change a login password. User must be
// the admin user — no other account's password is manageable
// through the helper. The password is re-validated root-side
// against the same 16-character rule the client enforces.
type SetUserPasswordParams struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// StageWalletPasswordParams: enable LND auto-unlock. The
// password the operator typed is written root-side to LND's
// wallet_password file (which stays owned by the service user,
// never readable by the admin user) and the LND service is
// rewritten to use it.
type StageWalletPasswordParams struct {
	Password string `json:"password"`
}

// RebuildSSHConfigParams: rewrite the SSH hardening drop-in.
// The template lives on the root side; the only caller-chosen
// value is the password-auth flag.
type RebuildSSHConfigParams struct {
	PasswordAuthDisabled bool `json:"password_auth_disabled"`
}

// RebuildTorConfigParams: rewrite torrc from the root-side
// template and restart Tor. The flags select which of the
// template's fixed hidden-service blocks are present — no
// caller-supplied torrc content, ever.
type RebuildTorConfigParams struct {
	LND       bool `json:"lnd"`
	Syncthing bool `json:"syncthing"`
}

// SetP2PModeParams: switch LND between tor-only and hybrid
// P2P. For "hybrid" the helper derives the box's public IPv4
// itself (kernel routing table) — the client cannot supply an
// address.
type SetP2PModeParams struct {
	Mode string `json:"mode"` // "tor" or "hybrid"
}

// SelfUpdateParams: update the vpn binary to a release version.
// Validated root-side: strict version shape, and same-major
// only (a cross-major release requires reading its release
// notes; the helper refuses it no matter what a client asks).
type SelfUpdateParams struct {
	Version string `json:"version"`
}

// SyncthingInstallResult returns the generated web-UI password
// for the operator's config.
type SyncthingInstallResult struct {
	Password string `json:"password"`
}

// ── Version gate (shared by both sides) ──────────────────

// releaseVersion is the accepted shape for a release version.
// Anchored and strict: this is the single choke point between
// "string from the network" and "string in a release URL".
var releaseVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// SameMajor reports whether two release versions share a major
// version. It errors when either does not parse — callers
// refuse rather than guess. Both the TUI (which renders a
// cross-major release as "see the release notes", with no
// update action) and the helper (which refuses to install one)
// use this same function, so the two halves of the gate cannot
// drift.
func SameMajor(current, target string) (bool, error) {
	if !releaseVersion.MatchString(current) {
		return false, fmt.Errorf(
			"running version %q is not a release build — "+
				"self-update requires one", current)
	}
	if !releaseVersion.MatchString(target) {
		return false, fmt.Errorf(
			"%q is not a valid release version", target)
	}
	return strings.SplitN(current, ".", 2)[0] ==
		strings.SplitN(target, ".", 2)[0], nil
}

// ── Streaming step lists ─────────────────────────────────
//
// A streaming verb's step names are fixed per verb and known to
// both sides: the client renders the full list up front, the
// server reports completion by index. A unit test in the helper
// daemon asserts each server-side step list matches these — the
// two can never silently drift past a test run.

// SelfUpdateStepNames mirrors the server's self-update steps.
func SelfUpdateStepNames(version string) []string {
	return []string{
		"Downloading v" + version,
		"Verifying signature",
		"Verifying checksum",
		"Installing new binary",
	}
}

// PackageUpdateStepNames mirrors the server's package-update
// steps.
func PackageUpdateStepNames() []string {
	return []string{
		"Refreshing package lists",
		"Upgrading packages",
	}
}

// SetP2PModeStepNames mirrors the server's P2P-mode steps.
func SetP2PModeStepNames() []string {
	return []string{
		"Updating LND config",
		"Updating firewall",
		"Restarting LND",
		"Restaging LND credentials",
	}
}

// SyncthingInstallStepNames mirrors the server's Syncthing
// install steps.
func SyncthingInstallStepNames(version string) []string {
	return []string{
		"Downloading Syncthing " + version,
		"Verifying Syncthing",
		"Installing Syncthing",
		"Creating Syncthing directories",
		"Creating Syncthing service",
		"Configuring Syncthing authentication",
		"Configuring firewall",
		"Rebuilding Tor config",
		"Restarting Tor",
		"Starting Syncthing",
		"Registering backup folder",
		"Setting up channel backup watcher",
		"Staging Syncthing facts",
	}
}
