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
//     files under /var/lib/vpn/state that carry deliberately staged
//     credentials and fixed facts to the
//     admin user without any privileged code running on the
//     read path (board.go).
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
	VerbStageLNDMacaroon     = "stage-lnd-macaroon"
	VerbRebuildSSHConfig     = "rebuild-ssh-config"
	// VerifyAdminLogin checks live journal evidence and may clear the
	// root-private pending marker. It accepts no caller-chosen state.
	VerbVerifyAdminLogin = "verify-admin-login"

	// Read-only verbs: no parameters, no mutation, a typed
	// result. They exist for display facts the TUI needs
	// at human cadence (screen entry) whose truth lives in
	// root-readable places and can change outside any
	// operation of ours — keeping a copy of such a fact
	// would risk rendering it confidently wrong, so no copy
	// exists and the screens ask here instead.
	VerbReadNodeAddresses        = "read-node-addresses"
	VerbReadSSHAuth              = "read-ssh-auth"
	VerbReadWalletState          = "read-wallet-state"
	VerbReadKeyVerificationState = "read-key-verification-state"

	// Streaming verbs: step progress events precede the
	// terminator, and feed the TUI's step renderer.
	VerbPackageUpdate      = "package-update"
	VerbSelfUpdate         = "self-update"
	VerbUpgradeP2PToHybrid = "upgrade-p2p-to-hybrid"
	VerbSyncthingInstall   = "syncthing-install"
)

// Request is the single line a client writes after connecting.
type Request struct {
	Verb   string          `json:"verb"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Event is one response line from the helper. Two kinds:
//
//   - "step": progress from a streaming verb. Index is the
//     0-based position in the verb's fixed step list. The
//     helper emits a step event on COMPLETION only — a step
//     that fails produces no step event; the failure arrives
//     as the error terminator. Err is reserved wire surface
//     for a per-step error report: the helper never sets it
//     today, and clients treat a non-empty Err as fatal if
//     one ever appears.
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

// SetUserPasswordParams changes only the vpn operator password. The helper
// enforces the same loginpassword validation as installation and the TUI.
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

// AutoUnlockOutcome is the classified end state of an auto-unlock change.
// Helper transport success means the root side finished observing and
// classifying the node; the outcome says whether the requested setting was
// applied, safely rolled back, or needs operator repair. Keeping these states
// structured prevents the TUI from parsing privileged error text.
type AutoUnlockOutcome string

const (
	AutoUnlockEnabled              AutoUnlockOutcome = "enabled"
	AutoUnlockDisabled             AutoUnlockOutcome = "disabled"
	AutoUnlockVerificationFailed   AutoUnlockOutcome = "verification_failed"
	AutoUnlockVerificationTimedOut AutoUnlockOutcome = "verification_timed_out"
	AutoUnlockStillEnabled         AutoUnlockOutcome = "still_enabled"
	AutoUnlockRepairRequired       AutoUnlockOutcome = "repair_required"
)

// AutoUnlockResult carries no credential material. FailedStep is a bounded,
// operator-facing stage name for logs and the repair-required screen; Detail
// is used only for a safely classified retry state.
type AutoUnlockResult struct {
	Outcome    AutoUnlockOutcome `json:"outcome"`
	FailedStep string            `json:"failed_step,omitempty"`
	Detail     string            `json:"detail,omitempty"`
}

// RebuildSSHConfigParams: rewrite the SSH hardening drop-in.
// The template lives on the root side; the only caller-chosen
// value is the password-auth flag.
type RebuildSSHConfigParams struct {
	PasswordAuthDisabled bool `json:"password_auth_disabled"`
}

// SelfUpdateParams: update the vpn binary to a release version.
// Validated root-side: strict version shape, and same-major
// only (a cross-major release requires reading its release
// notes; the helper refuses it no matter what a client asks).
type SelfUpdateParams struct {
	Version string `json:"version"`
}

// NodeAddressesResult is the read-node-addresses answer: the
// node's Tor hidden-service hostnames and its Syncthing device
// ID, read from their sources at the moment of the request. An
// empty field means that service is not configured on this box
// (its hostname file does not exist) — the screen renders the
// feature unavailable rather than showing an address nobody
// can reach.
type NodeAddressesResult struct {
	BitcoinP2POnion   string `json:"bitcoin_p2p_onion"`
	LNDGRPCOnion      string `json:"lnd_grpc_onion"`
	LNDRESTOnion      string `json:"lnd_rest_onion"`
	SyncthingOnion    string `json:"syncthing_onion"`
	SyncthingDeviceID string `json:"syncthing_device_id"`
}

// SSHAuthResult is the read-ssh-auth answer: whether sshd's
// EFFECTIVE configuration permits password authentication for
// the admin user, asked of sshd itself at request time. This
// answer gates removing the last SSH key, so callers treat a
// verb error as "unavailable" and refuse the risky action.
type SSHAuthResult struct {
	PasswordAuthEnabled bool `json:"password_auth_enabled"`
}

// WalletStateResult is the independent live wallet observation. It is not
// coupled to any private workflow marker, so a marker read failure cannot make
// an otherwise readable wallet appear unknown.
type WalletStateResult struct {
	WalletExists bool `json:"wallet_exists"`
}

// KeyVerificationStateResult is the independent root-private workflow-marker
// observation. Wallet availability never depends on this read succeeding.
type KeyVerificationStateResult struct {
	Pending bool `json:"pending"`
}

// VerifyAdminLoginResult reports the private marker state after the helper
// checked current sshd journal evidence and, when verified, cleared it.
type VerifyAdminLoginResult struct {
	Pending  bool `json:"pending"`
	Verified bool `json:"verified"`
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

// UpgradeP2PToHybridStepNames mirrors the server's one-way P2P transition.
func UpgradeP2PToHybridStepNames() []string {
	return []string{
		"Checking active firewall",
		"Updating LND config",
		"Adding hybrid P2P firewall rules",
		"Restarting LND",
		"Verifying LND TLS IP certificate",
		"Restaging LND TLS certificate",
		"Publishing node configuration",
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
		"Adding Syncthing firewall rule",
		"Reloading Tor configuration",
		"Starting Syncthing",
		"Registering backup folder",
		"Setting up channel backup watcher",
		"Staging Syncthing facts",
		"Publishing node configuration",
	}
}
