// internal/helperd/verbs.go

package helperd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/host"
	"github.com/virtualprivatenode/vpn/internal/installer"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// ── The verb menu ────────────────────────────────────────
//
// Each verb: a typed parameter struct (decoded strictly),
// validation against closed sets, the operation itself, a
// postcondition where the outcome is checkable, and re-staging
// of every board fact the operation invalidated (the freshness
// matrix in matrix.go drives that last part — handlers call
// restage(verb) rather than remembering individual files).
//
// Deadlines bound the connection's socket I/O for one verb.
// They are sized to the operation's legitimate worst case —
// bitcoind alone is allowed 10 minutes to stop, a package
// upgrade can take most of half an hour over Tor. A deadline
// alone cannot cut short a wedged SUBPROCESS (nothing does
// socket I/O while one runs); the bounds there live with the
// subprocesses themselves — download timeouts and retry caps
// (system.doDownload), apt's Acquire timeout configuration,
// and systemd's per-unit stop timeouts.

type verbDef struct {
	deadline time.Duration
	handler  func(ctx *verbCtx, params json.RawMessage) (any, error)
}

var verbs = map[string]verbDef{
	// LND uses its upstream readiness notification and permits up to 20
	// minutes for an ordinary start. Keep the helper connection above the
	// longest supported service start plus its graceful-stop allowance.
	helper.VerbServiceAction:   {30 * time.Minute, verbServiceAction},
	helper.VerbReboot:          {1 * time.Minute, verbReboot},
	helper.VerbDirSize:         {2 * time.Minute, verbDirSize},
	helper.VerbSetUserPassword: {1 * time.Minute, verbSetUserPassword},
	// LND permits a graceful stop to take up to five minutes. A failed
	// transition can require a second stop/start recovery, so the helper
	// connection must outlive both bounded systemd transactions.
	helper.VerbStageWalletPassword:  {20 * time.Minute, verbStageWalletPassword},
	helper.VerbRemoveWalletPassword: {20 * time.Minute, verbRemoveWalletPassword},
	helper.VerbStageLNDCredentials:  {3 * time.Minute, verbStageLNDCredentials},
	helper.VerbStageLNDMacaroon:     {1 * time.Minute, verbStageLNDMacaroon},
	helper.VerbRebuildSSHConfig:     {3 * time.Minute, verbRebuildSSHConfig},
	helper.VerbPackageUpdate:        {30 * time.Minute, verbPackageUpdate},
	helper.VerbSelfUpdate:           {15 * time.Minute, verbSelfUpdate},
	helper.VerbUpgradeP2PToHybrid:   {8 * time.Minute, verbUpgradeP2PToHybrid},
	helper.VerbSyncthingInstall:     {30 * time.Minute, verbSyncthingInstall},
	helper.VerbReadNodeAddresses:    {1 * time.Minute, verbReadNodeAddresses},
	helper.VerbReadSSHAuth:          {1 * time.Minute, verbReadSSHAuth},
	helper.VerbReadWalletState:      {1 * time.Minute, verbReadWalletState},
	helper.VerbReadKeyVerificationState: {
		1 * time.Minute, verbReadKeyVerificationState,
	},
	helper.VerbVerifyAdminLogin: {1 * time.Minute, verbVerifyAdminLogin},
}

var (
	loadSystemConfig             = config.Load
	saveSystemConfig             = config.Save
	setupAutoUnlock              = installer.SetupAutoUnlock
	disableAutoUnlock            = installer.DisableAutoUnlock
	publicIPv4                   = system.PublicIPv4
	p2pUpgradeSteps              = installer.UpgradeP2PToHybridSteps
	syncthingInstallSteps        = installer.SyncthingInstallSteps
	syncthingResiduePresent      = installer.SyncthingResiduePresent
	verifySyncthingPrerequisites = installer.VerifySyncthingInstallPrerequisites
	walletExists                 = installer.WalletExists
	keyVerificationPending       = installer.KeyVerificationPending
	verifyAdminLogin             = installer.VerifyAdminLogin
	stageSyncthingWebPassword    = func(password string) error {
		return helper.WriteBoard(paths.StateSyncthingWebPassword,
			[]byte(password+"\n"))
	}
	restageFacts = restage
)

// decode unmarshals params strictly: unknown fields are an
// error, so a client/server drift surfaces as a loud refusal
// instead of a silently ignored option.
func decode(params json.RawMessage, into any) error {
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("invalid parameters: %v", err)
	}
	return nil
}

func rejectParams(params json.RawMessage) error {
	trimmed := bytes.TrimSpace(params)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	return errors.New("this operation accepts no parameters")
}

// loadConfig reads the node's config root-side. Verbs use it
// for facts the client must not be able to misstate. Typed mutating verbs also
// publish their own desired-state change before returning success.
func loadConfig() (*config.AppConfig, error) {
	cfg, err := loadSystemConfig()
	if err != nil {
		return nil, fmt.Errorf(
			"read node config: %v — the node may not be "+
				"installed", err)
	}
	return cfg, nil
}

// ── Service control ──────────────────────────────────────

var allowedUnits = map[string]bool{
	"tor": true, "bitcoind": true, "lnd": true, "syncthing": true,
}
var allowedActions = map[string]bool{
	"start": true, "stop": true, "restart": true,
}

func verbServiceAction(_ *verbCtx, params json.RawMessage) (any, error) {
	var p helper.ServiceActionParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	if !allowedUnits[p.Unit] {
		return nil, fmt.Errorf("unit %q is not managed here", p.Unit)
	}
	if !allowedActions[p.Action] {
		return nil, fmt.Errorf("action %q is not supported", p.Action)
	}
	if err := system.SudoRun("systemctl", p.Action, p.Unit); err != nil {
		return nil, err
	}
	// Postcondition: rc=0 means systemctl ran, not that the
	// world changed. Ask systemd what state the unit is
	// actually in now.
	active := system.IsServiceActive(p.Unit)
	switch p.Action {
	case "stop":
		if active {
			return nil, fmt.Errorf(
				"%s is still active after stop", p.Unit)
		}
	default:
		if !active {
			return nil, fmt.Errorf(
				"%s is not active after %s — check: journalctl "+
					"-u %s", p.Unit, p.Action, p.Unit)
		}
	}
	// LND replaces an expired TLS certificate during startup;
	// tlsautorefresh also replaces it when configured SAN inputs
	// changed. A start or restart of
	// the lnd unit can invalidate the staged certificate copy
	// the TUI reads. Re-stage it so the board keeps
	// matching the certificate LND actually serves. The
	// freshness matrix carries this as service-action's entry;
	// the unit condition lives here because whether the
	// invalidation happened depends on a verb parameter, which
	// the verb-keyed table cannot express alone.
	if p.Unit == "lnd" && p.Action != "stop" {
		if err := restageFacts(helper.VerbServiceAction); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func verbReboot(ctx *verbCtx, _ json.RawMessage) (any, error) {
	// Answer first, reboot after: the client gets its
	// terminator before the box goes down.
	ctx.afterEnd = func() {
		if err := system.SudoRun("systemctl", "reboot"); err != nil {
			auditErr("reboot",
				`{"event":"error","detail":"reboot failed: %v"}`, err)
		}
	}
	return nil, nil
}

// ── Sizes ────────────────────────────────────────────────

func verbDirSize(_ *verbCtx, params json.RawMessage) (any, error) {
	var p helper.DirSizeParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	// Closed enum, not a path. "lnd" is the only entry: the
	// bitcoin data dir's size comes from bitcoind's own RPC
	// (size_on_disk) with no privileged call at all.
	if p.Which != "lnd" {
		return nil, fmt.Errorf("unknown directory %q", p.Which)
	}
	size := system.DirSize(paths.LNDDataDir)
	if size == "N/A" {
		return nil, errors.New("could not measure the LND data dir")
	}
	return helper.DirSizeResult{Size: size}, nil
}

// ── Passwords ────────────────────────────────────────────

func verbSetUserPassword(_ *verbCtx, params json.RawMessage) (any, error) {
	var p helper.SetUserPasswordParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	if p.User != paths.AdminUser {
		return nil, fmt.Errorf(
			"only the %q user's password is managed here",
			paths.AdminUser)
	}
	// Same rule as the client's input screen, enforced again
	// at the boundary — the two share one constructor, so they
	// cannot disagree.
	pw, err := installer.NewLoginPassword(p.Password)
	if err != nil {
		return nil, err
	}
	if err := installer.SetUserPassword(p.User, pw); err != nil {
		return nil, err
	}
	// An operator-chosen password supersedes any generated one
	// that was never displayed (the unattended-install marker).
	installer.ClearPasswordPendingMarker()
	return nil, nil
}

// validateWalletPassword bounds the auto-unlock payload. LND
// enforces its own minimum at wallet creation; here we only
// refuse shapes that would corrupt the password file protocol.
func validateWalletPassword(pw string) error {
	if pw == "" {
		return errors.New("wallet password is empty")
	}
	if len(pw) > 512 {
		return errors.New("wallet password is implausibly long")
	}
	if strings.ContainsAny(pw, "\n\r") {
		return errors.New("wallet password has a line break")
	}
	return nil
}

func verbStageWalletPassword(_ *verbCtx, params json.RawMessage) (any, error) {
	var p helper.StageWalletPasswordParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	if err := validateWalletPassword(p.Password); err != nil {
		return nil, err
	}
	return setupAutoUnlock(p.Password)
}

func verbRemoveWalletPassword(_ *verbCtx, _ json.RawMessage) (any, error) {
	return disableAutoUnlock()
}

// ── Credential staging ───────────────────────────────────

func verbStageLNDCredentials(_ *verbCtx, _ json.RawMessage) (any, error) {
	return nil, restageFacts(helper.VerbStageLNDCredentials)
}

// Wallet creation mints only the admin macaroon. Keep that transition narrow;
// the broader credentials verb remains the repair path for an LND client that
// cannot know whether its certificate, macaroon, or both became stale.
func verbStageLNDMacaroon(_ *verbCtx, _ json.RawMessage) (any, error) {
	return nil, restageFacts(helper.VerbStageLNDMacaroon)
}

// ── Read-only verbs ──────────────────────────────────────
//
// Live reads for display facts the TUI needs at human
// cadence (screen entry). These facts can change with no
// operation of ours involved — a root login can destroy and
// recreate hidden-service directories, an sshd drop-in can be
// edited by hand or by provider tooling — and a fact with no
// failure moment cannot be repaired at one. So no copy is
// kept anywhere: the answer is read from its source at the
// moment of the question, and what the screen shows is what
// is true right now. Cost per read: one socket round-trip, a
// handful of times per session. Both verbs take no parameters
// and change nothing; they get the same peer-credential check
// and journal record as every other verb.

// readHostname reads a Tor hidden-service hostname file. A
// missing or empty file means that service is not configured
// on this box — reported as an empty field, which screens
// render as unavailable.
func readHostname(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func verbReadNodeAddresses(_ *verbCtx, _ json.RawMessage) (any, error) {
	return helper.NodeAddressesResult{
		BitcoinP2POnion: readHostname(
			paths.TorBitcoinP2P + "/hostname"),
		LNDGRPCOnion: readHostname(
			paths.TorLNDGRPC + "/hostname"),
		LNDRESTOnion:   readHostname(paths.TorLNDRESTHostname),
		SyncthingOnion: readHostname(paths.TorSyncthingHostname),
		// Root-side read: parses Syncthing's own config, so it
		// answers correctly even when the daemon is stopped.
		SyncthingDeviceID: installer.GetSyncthingDeviceID(),
	}, nil
}

func verbReadSSHAuth(_ *verbCtx, _ json.RawMessage) (any, error) {
	// Sample the operator configuration at localhost. Errors remain unknown;
	// source-specific Match rules may differ on a real connection.
	enabled, err := host.EffectiveSSHPasswordAuth()
	if err != nil {
		return nil, err
	}
	return helper.SSHAuthResult{PasswordAuthEnabled: enabled}, nil
}

func verbReadWalletState(_ *verbCtx, _ json.RawMessage) (any, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	wallet, err := walletExists(cfg.Network)
	if err != nil {
		return nil, err
	}
	return helper.WalletStateResult{WalletExists: wallet}, nil
}

func verbReadKeyVerificationState(
	_ *verbCtx, _ json.RawMessage,
) (any, error) {
	pending, err := keyVerificationPending()
	if err != nil {
		return nil, err
	}
	return helper.KeyVerificationStateResult{Pending: pending}, nil
}

// This is a closed security-workflow mutation, not a caller-directed config
// write: current sshd journal evidence is the only authority allowed to clear
// the root-private marker.
func verbVerifyAdminLogin(_ *verbCtx, _ json.RawMessage) (any, error) {
	pending, verified, err := verifyAdminLogin()
	if err != nil {
		return nil, err
	}
	return helper.VerifyAdminLoginResult{Pending: pending, Verified: verified}, nil
}

// ── Config writers (templates live on this side) ─────────

func verbRebuildSSHConfig(_ *verbCtx, params json.RawMessage) (any, error) {
	var p helper.RebuildSSHConfigParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	// The fresh key guard, validate/restore sequence, and effective-state
	// verification live at the privileged write boundary.
	if err := host.RebuildSSHHardeningConfig(
		p.PasswordAuthDisabled); err != nil {
		return nil, err
	}
	// Nothing to re-stage: the effective password-auth answer
	// is not copied anywhere — readers ask the read-ssh-auth
	// verb, which queries sshd live.
	return nil, nil
}

// ── Streaming verbs ──────────────────────────────────────

// runSteps executes installer steps sequentially, emitting one
// progress event per completed step. The client renders these
// in the same step widget the installer uses.
func runSteps(ctx *verbCtx, steps []installer.InstallStep) error {
	for i := range steps {
		if err := steps[i].Fn(); err != nil {
			return fmt.Errorf("%s: %v", steps[i].Name, err)
		}
		ctx.emitStep(i)
	}
	return nil
}

func verbPackageUpdate(ctx *verbCtx, _ json.RawMessage) (any, error) {
	steps := installer.PackageUpdateSteps()
	if err := runSteps(ctx, steps); err != nil {
		return nil, err
	}
	// Postcondition: no packages left half-configured.
	if out, err := system.RunOutput("dpkg", "--audit"); err != nil ||
		strings.TrimSpace(out) != "" {
		return nil, fmt.Errorf(
			"packages left in an inconsistent state after "+
				"upgrade (dpkg --audit: %v %s)", err,
			strings.TrimSpace(out))
	}
	return nil, nil
}

func verbSelfUpdate(ctx *verbCtx, params json.RawMessage) (any, error) {
	var p helper.SelfUpdateParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	// Same-major gate, enforced HERE and not only in the
	// client's rendering: a gate that lives only in UI copy is
	// copy. A major release carries changes that need the
	// operator to read its release notes first. (The strict
	// version-shape validation lives inside SameMajor — the
	// one choke point before the version reaches a URL.)
	same, err := helper.SameMajor(ctx.version, p.Version)
	if err != nil {
		return nil, err
	}
	if !same {
		return nil, fmt.Errorf(
			"v%s is a major release — it is not installed "+
				"through self-update; see its release notes",
			p.Version)
	}
	// Download, GPG-verify, checksum-verify, and install — all
	// on this side of the boundary. Nothing the unprivileged
	// user staged is trusted anywhere in this path.
	if err := runSteps(ctx,
		installer.SelfUpdateSteps(p.Version)); err != nil {
		return nil, err
	}
	// Exit after answering: the next activation of the helper
	// runs the NEW binary, so helper and TUI can never disagree
	// about versions for longer than one connection.
	ctx.exitAfterEnd = true
	return nil, nil
}

func verbUpgradeP2PToHybrid(
	ctx *verbCtx, params json.RawMessage,
) (any, error) {
	if err := rejectParams(params); err != nil {
		return nil, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.P2PMode != "tor" {
		return nil, fmt.Errorf(
			"hybrid P2P upgrade requires authoritative mode tor; current mode is %q",
			cfg.P2PMode)
	}
	// Derived here, from the kernel routing table — the client cannot supply
	// an address for LND to advertise.
	publicIP := publicIPv4()
	if publicIP == "" {
		return nil, errors.New(
			"could not determine this box's public IPv4 " +
				"address — hybrid mode needs one")
	}
	cfg.P2PMode = "hybrid"
	steps := p2pUpgradeSteps(cfg, publicIP)
	if err := runSteps(ctx, steps); err != nil {
		return nil, err
	}
	// The mode switch makes LND regenerate its TLS certificate
	// (the cert's contents change), so the staged copy is now
	// stale — re-stage it, reported as one more step.
	if err := restageFacts(helper.VerbUpgradeP2PToHybrid); err != nil {
		return nil, err
	}
	ctx.emitStep(len(steps))
	if err := saveSystemConfig(cfg); err != nil {
		return nil, fmt.Errorf("publish P2P setting: %w", err)
	}
	ctx.emitStep(len(steps) + 1)
	return nil, nil
}

func verbSyncthingInstall(ctx *verbCtx, _ json.RawMessage) (any, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.SyncthingEnabled {
		return nil, errors.New("Syncthing is already enabled")
	}
	residue, err := syncthingResiduePresent()
	if err != nil {
		return nil, err
	}
	if residue {
		return nil, errors.New("Syncthing add-on residue exists while desired enablement is false — refusing to modify it; ADDON-001 recovery is not implemented")
	}
	if err := verifySyncthingPrerequisites(cfg); err != nil {
		return nil, err
	}
	cfg.SyncthingEnabled = true
	steps, password, err := syncthingInstallSteps(cfg)
	if err != nil {
		return nil, err
	}
	if err := runSteps(ctx, steps); err != nil {
		return nil, err
	}
	// Stage the two credentials the admin user needs from the new component:
	// the API key and generated Web password. The device ID and onion hostname
	// remain live reads; the Tor step above already proved the onion exists.
	if err := restageFacts(helper.VerbSyncthingInstall); err != nil {
		return nil, err
	}
	if err := stageSyncthingWebPassword(password); err != nil {
		return nil, fmt.Errorf("stage Syncthing Web password: %w", err)
	}
	ctx.emitStep(len(steps))
	if err := saveSystemConfig(cfg); err != nil {
		return nil, fmt.Errorf("publish Syncthing setting: %w", err)
	}
	ctx.emitStep(len(steps) + 1)
	return nil, nil
}
