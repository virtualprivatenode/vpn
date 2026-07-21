// internal/helperd/verbs.go

package helperd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
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
// Deadlines bound how long one verb may hold the serialized
// queue. They are sized to the operation's legitimate worst
// case — bitcoind alone is allowed 10 minutes to stop, a
// package upgrade can take most of half an hour over Tor — so
// a wedged operation fails loudly instead of blocking an
// urgent service restart forever.

type verbDef struct {
	deadline time.Duration
	handler  func(ctx *verbCtx, params json.RawMessage) (any, error)
}

var verbs = map[string]verbDef{
	helper.VerbServiceAction:        {14 * time.Minute, verbServiceAction},
	helper.VerbReboot:               {1 * time.Minute, verbReboot},
	helper.VerbDirSize:              {2 * time.Minute, verbDirSize},
	helper.VerbSetUserPassword:      {1 * time.Minute, verbSetUserPassword},
	helper.VerbStageWalletPassword:  {6 * time.Minute, verbStageWalletPassword},
	helper.VerbRemoveWalletPassword: {6 * time.Minute, verbRemoveWalletPassword},
	helper.VerbStageLNDCredentials:  {3 * time.Minute, verbStageLNDCredentials},
	helper.VerbRebuildSSHConfig:     {3 * time.Minute, verbRebuildSSHConfig},
	helper.VerbRebuildTorConfig:     {4 * time.Minute, verbRebuildTorConfig},
	helper.VerbPackageUpdate:        {30 * time.Minute, verbPackageUpdate},
	helper.VerbSelfUpdate:           {15 * time.Minute, verbSelfUpdate},
	helper.VerbSetP2PMode:           {8 * time.Minute, verbSetP2PMode},
	helper.VerbSyncthingInstall:     {30 * time.Minute, verbSyncthingInstall},
}

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

// loadConfig reads the node's config root-side. Verbs use it
// for facts the client must not be able to misstate (network,
// installed components); the client remains the owner of
// PERSISTING config changes after a verb succeeds.
func loadConfig() (*config.AppConfig, error) {
	cfg, err := config.Load()
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
	return nil, installer.SetupAutoUnlock(p.Password)
}

func verbRemoveWalletPassword(_ *verbCtx, _ json.RawMessage) (any, error) {
	return nil, installer.DisableAutoUnlock()
}

// ── Credential staging ───────────────────────────────────

func verbStageLNDCredentials(_ *verbCtx, _ json.RawMessage) (any, error) {
	return nil, restage(helper.VerbStageLNDCredentials)
}

// ── Config writers (templates live on this side) ─────────

func verbRebuildSSHConfig(_ *verbCtx, params json.RawMessage) (any, error) {
	var p helper.RebuildSSHConfigParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	// The zero-auth lockout guard and the validate-and-restore
	// sequence live inside RebuildSSHHardeningConfig — at the
	// write boundary, so every caller inherits them.
	view := &config.AppConfig{
		SSHPasswordAuthDisabled: p.PasswordAuthDisabled,
	}
	if err := installer.RebuildSSHHardeningConfig(view); err != nil {
		return nil, err
	}
	return nil, restage(helper.VerbRebuildSSHConfig)
}

func verbRebuildTorConfig(_ *verbCtx, params json.RawMessage) (any, error) {
	var p helper.RebuildTorConfigParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	// The toggles select which fixed template blocks are
	// present; everything else about the torrc comes from the
	// root-side template and the node's own config.
	cfg.LNDInstalled = p.LND
	cfg.SyncthingInstalled = p.Syncthing
	if err := installer.RebuildTorConfig(cfg); err != nil {
		return nil, err
	}
	if err := installer.RestartTor(); err != nil {
		return nil, err
	}
	return nil, restage(helper.VerbRebuildTorConfig)
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

func verbSetP2PMode(ctx *verbCtx, params json.RawMessage) (any, error) {
	var p helper.SetP2PModeParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	if p.Mode != "tor" && p.Mode != "hybrid" {
		return nil, fmt.Errorf("unknown P2P mode %q", p.Mode)
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	publicIP := ""
	if p.Mode == "hybrid" {
		// Derived here, from the kernel routing table — the
		// client cannot supply an address for LND to advertise.
		publicIP = system.PublicIPv4()
		if publicIP == "" {
			return nil, errors.New(
				"could not determine this box's public IPv4 " +
					"address — hybrid mode needs one")
		}
	}
	cfg.P2PMode = p.Mode
	steps := installer.P2PUpgradeSteps(cfg, publicIP)
	if err := runSteps(ctx, steps); err != nil {
		return nil, err
	}
	// The mode switch makes LND regenerate its TLS certificate
	// (the cert's contents change), so the staged copy is now
	// stale — re-stage it, reported as one more step.
	if err := restage(helper.VerbSetP2PMode); err != nil {
		return nil, err
	}
	ctx.emitStep(len(steps))
	return nil, nil
}

func verbSyncthingInstall(ctx *verbCtx, _ json.RawMessage) (any, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	cfg.SyncthingInstalled = true
	steps, password, err := installer.SyncthingInstallSteps(cfg)
	if err != nil {
		return nil, err
	}
	if err := runSteps(ctx, steps); err != nil {
		return nil, err
	}
	// Stage the facts the admin user needs from the new
	// component: API key, device ID, and the new onion
	// hostname — reported as one more step.
	if err := restage(helper.VerbSyncthingInstall); err != nil {
		return nil, err
	}
	ctx.emitStep(len(steps))
	return helper.SyncthingInstallResult{Password: password}, nil
}
