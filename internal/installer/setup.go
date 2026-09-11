package installer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/host"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/loginpassword"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

const (
	bitcoinVersion   = "29.3"
	lndVersion       = "0.21.2-beta"
	syncthingVersion = "2.1.1"
	bitcoinUser      = "bitcoin"
	lndUser          = "lnd"
	syncthingUser    = "syncthing"
	backupGroup      = "vpn-lnd-backup"
)

var appVersion = "dev"

func SetVersion(v string)         { appVersion = v }
func LndVersionStr() string       { return lndVersion }
func SyncthingVersionStr() string { return syncthingVersion }

// ── Main install flow ────────────────────────────────────
//
// `vpn install` — explicit dispatch only (IA-1-8's fix): this
// runs because the operator asked, never because the binary
// sniffed box state and decided for itself. The old
// NeedsInstall() config/service-probe is deleted with the old
// implicit flow.
//
// The step model, resume planner, and step runner live in
// engine.go; the ledger in ledger.go; the interactive front-end
// (wizard screens + step renderer) in wizard.go. Front-ends are
// thin: they render what the runner reports and make no skip or
// record decisions of their own, so the TUI and the unattended
// runner cannot diverge.

// InstallOptions carries the `vpn install` command line.
type InstallOptions struct {
	// Network from --testnet4 or --signet ("" = mainnet for a pristine host,
	// or keep the interrupted lifecycle's recorded answer).
	Network string
	// Unattended runs with no TUI and no prompts (ruling iv/vii:
	// keys auto-copied from enumeration, password randomly
	// generated and printed once — the image path's fallback).
	Unattended bool
	// UntilBake runs only PhaseBake steps (image build
	// pipeline, ruling iv). Requires Unattended. The run ends
	// without terminal ledger status, without handoff, without the
	// verification banner — first-boot steps are still owed.
	UntilBake bool
	// AllowConsoleOnly permits an unattended install to
	// complete with NO SSH way in: no enumerable keys AND
	// password auth observed off means the printed password
	// works only at the provider console. Without this flag
	// such a run REFUSES — completing it would strand the box
	// by automation. The flag names the consequence so the
	// consent is auditable wherever the command line is
	// recorded.
	AllowConsoleOnly bool
}

// installStartupDependencies keeps the lifecycle authorization boundary
// testable against isolated paths. The production constructor below supplies
// the same paths, lock, classifier, preflight, and initializer used before this
// seam was introduced.
type installStartupDependencies struct {
	runtimeDir          string
	installLock         string
	fs                  lifecycleFS
	lookup              identityLookup
	acquireRunLock      func(string, string) (*os.File, error)
	classifyLifecycle   func(lifecycleFS, identityLookup) (lifecycleState, error)
	runPreflight        func() (SSHObservation, error)
	initializeLifecycle func(lifecycleFS, identityLookup, installContext) (*installLedger, error)
}

func productionInstallStartupDependencies() installStartupDependencies {
	return installStartupDependencies{
		runtimeDir:          paths.RuntimeDir,
		installLock:         paths.InstallLock,
		fs:                  productionLifecycleFS(),
		lookup:              productionIdentityLookup(),
		acquireRunLock:      acquireRunLock,
		classifyLifecycle:   classifyLifecycleState,
		runPreflight:        RunPreflight,
		initializeLifecycle: initializeLifecycle,
	}
}

var newInstallStartupDependencies = productionInstallStartupDependencies

// RunInstall is the `sudo vpn install` entry point.
func RunInstall(opts InstallOptions) error {
	if os.Geteuid() != 0 {
		return errors.New(
			"the installer must run as root — run: sudo vpn install")
	}
	if opts.UntilBake && !opts.Unattended {
		return errors.New(
			"--until=bake requires --unattended (image build path)")
	}

	// Non-interactive package operations for the whole run
	// (absorbed from the retired bootstrap): debconf prompts
	// suppressed, needrestart auto-restarts services instead of
	// showing its dialog mid-upgrade.
	os.Setenv("DEBIAN_FRONTEND", "noninteractive")
	os.Setenv("NEEDRESTART_MODE", "a")
	deps := newInstallStartupDependencies()

	// The transient stable lock is acquired before classification. It is not
	// durable lifecycle state and remains the same inode while the ledger is
	// atomically replaced.
	runLock, err := deps.acquireRunLock(deps.runtimeDir, deps.installLock)
	if err != nil {
		return err
	}
	defer runLock.Close()

	fs := deps.fs
	lookup := deps.lookup
	lifecycle, err := deps.classifyLifecycle(fs, lookup)
	if err != nil {
		return err
	}
	if lifecycle.Disposition == lifecycleCompleted {
		fmt.Println("\n  Virtual Private Node is already installed; no changes were made.")
		return nil
	}
	if lifecycle.Disposition != lifecyclePristine &&
		!opts.Unattended && passwordPending() {
		return errors.New(
			"an interrupted unattended install still owes password delivery — resume with: sudo vpn install --unattended")
	}

	// Preflight is read-only and precedes every durable initialization.
	obs, err := deps.runPreflight()
	if err != nil {
		return err
	}

	ledger := lifecycle.Ledger
	if lifecycle.Disposition == lifecyclePristine {
		network := opts.Network
		if network == "" {
			network = "mainnet"
		}
		ledger, err = deps.initializeLifecycle(fs, lookup, installContext{
			Network: network, InitialP2PMode: "tor",
		})
		if err != nil {
			return err
		}
	} else if lifecycle.Disposition == lifecycleBootstrap {
		ctx := *lifecycle.BootstrapContext
		if opts.Network != "" && opts.Network != ctx.Network {
			return fmt.Errorf(
				"--%s conflicts with the interrupted install network %q — drop the flag to resume the recorded lifecycle",
				opts.Network, ctx.Network)
		}
		ledger, err = resumeLifecycleBootstrap(fs, lookup, ctx)
		if err != nil {
			return err
		}
	} else if opts.Network != "" && opts.Network != ledger.Context.Network {
		return fmt.Errorf(
			"--%s conflicts with the interrupted install network %q — drop the flag to resume the recorded lifecycle",
			opts.Network, ledger.Context.Network)
	}
	if lifecycle.Disposition == lifecycleCompletionPending && opts.UntilBake {
		return errors.New(
			"base installation is awaiting finalization — resume without --until=bake")
	}

	// General configuration is reconstructed from immutable lifecycle context
	// and fresh observations. It is not installation-completion authority.
	cfg := config.Default()
	cfg.Network = ledger.Context.Network
	cfg.P2PMode = ledger.Context.InitialP2PMode
	dec := &InstallDecisions{Obs: obs}
	if ledger.Context.DbCacheMB != nil {
		dec.DbCacheMB = *ledger.Context.DbCacheMB
		cfg.DbCache = dec.DbCacheMB
	}

	allSteps := buildInstallSteps(cfg, dec)
	if err := validateBaseInstallSteps(allSteps); err != nil {
		return err
	}
	steps := allSteps
	if opts.UntilBake {
		steps = FilterPhase(steps, PhaseBake)
	}

	persistDbCache := func(v int) error {
		if err := ledger.setDbCache(v); err != nil {
			return err
		}
		if err := ledger.save(paths.InstallStateFile); err != nil {
			return fmt.Errorf("record db cache decision: %w", err)
		}
		dec.DbCacheMB = v
		cfg.DbCache = v
		return nil
	}
	completeInteractive := func() error {
		if err := prepareInstallCompletion(cfg, dec, ledger, len(allSteps)); err != nil {
			return err
		}
		return publishTerminalLedger(ledger)
	}

	var res RunResult
	openConsole := false
	if opts.Unattended && lifecycle.Disposition != lifecycleCompletionPending {
		if err := fillUnattendedDecisions(
			dec, opts.AllowConsoleOnly); err != nil {
			return err
		}
		if ledger.Context.DbCacheMB == nil {
			if err := persistDbCache(dec.DbCacheMB); err != nil {
				return err
			}
		} else {
			dec.DbCacheMB = *ledger.Context.DbCacheMB
			cfg.DbCache = dec.DbCacheMB
		}
	} else if opts.Unattended && passwordPending() {
		if err := fillGeneratedPassword(dec); err != nil {
			return err
		}
	}

	if lifecycle.Disposition == lifecycleCompletionPending {
		res = RunResult{Outcome: RunComplete, Total: len(allSteps)}
	} else if opts.Unattended {
		fmt.Printf("\n  Virtual Private Node — unattended install\n\n")
		res, err = RunInstallUnattended(
			steps, appVersion, ledger, paths.InstallStateFile)
	} else {
		res, openConsole, err = runInstallWizard(
			cfg, steps, dec, appVersion, ledger,
			persistDbCache, completeInteractive)
	}
	if err != nil {
		return err
	}
	if lifecycle.Disposition == lifecycleCompletionPending && !opts.Unattended {
		if err := completeInteractive(); err != nil {
			return err
		}
	}
	switch res.Outcome {
	case RunFailed:
		// The runner already logged the failure line; the
		// error surfaces to stderr via main.
		return fmt.Errorf("%s: %w", res.StepName, res.Err)
	case RunInterrupted:
		// The log trail must not just stop (commit-5 addendum):
		// record how far the run got, and that a re-run resumes.
		logger.Install(
			"install INTERRUPTED at step %d/%d: %s — "+
				"run again to resume", res.StepNum, res.Total,
			res.StepName)
		return fmt.Errorf(
			"install interrupted at step %d/%d (%s) — "+
				"run again to resume", res.StepNum, res.Total,
			res.StepName)
	}

	if opts.UntilBake {
		// The bake slice completing is not the install
		// completing: first-boot steps (identity, hardware fit,
		// SSH hardening) are still owed on the deployed box.
		logger.Install(
			"bake phase complete (%d steps) — install NOT marked "+
				"complete; first-boot steps pending", res.Total)
		fmt.Printf("\n  Bake phase complete (%d steps).\n", res.Total)
		return nil
	}

	if opts.Unattended {
		if err := prepareInstallCompletion(
			cfg, dec, ledger, len(allSteps)); err != nil {
			return err
		}
		if needsPasswordReapply(dec.GeneratedPassword,
			dec.PasswordApplied, passwordPending()) {
			// Fail-then-resume (live-run finding): an earlier
			// unattended pass applied a generated password and
			// died before the end-of-run print, so nobody has
			// ever seen a working credential — and this pass
			// ledger-skipped the identity step. Re-apply THIS
			// pass's generated password now, so the line printed
			// below is one that works.
			if err := host.SetLoginPassword(dec.Password); err != nil {
				return fmt.Errorf(
					"re-apply admin password: %w", err)
			}
			dec.PasswordApplied = true
			logger.Install("admin password re-applied at " +
				"completion — an earlier pass applied one that " +
				"was never displayed")
		}
	}

	if dec.GeneratedPassword != "" && dec.PasswordApplied {
		// Unattended fallback only (ruling vii), and only when
		// this pass actually applied it (a ledger-skip with no
		// pending marker means an older, already-shown password
		// stands). Printed once, never logged.
		if err := printGeneratedPassword(dec.GeneratedPassword); err != nil {
			return err
		}
		if err := clearPasswordPendingMarkerStrict(); err != nil {
			return err
		}
	}
	if opts.Unattended {
		if err := publishTerminalLedger(ledger); err != nil {
			return err
		}
	}

	if opts.Unattended {
		// The console-only end state (reachable only with
		// --allow-console-only) has no working ssh line to
		// print — say what IS true instead. Same condition the
		// strand guard evaluated, minus the consent flag.
		if strandsBox(len(dec.Keys), dec.Obs.PasswordAuth, false) {
			printConsoleOnlyInstructions()
		} else {
			printConnectInstructions()
		}
		return nil
	}
	// The done screen offered a real choice (live-run fix):
	// Enter opens the node TUI here via the identity drop;
	// ctrl+c means exit, so exit — just leave the connect
	// command behind. The handoff degrades to printed
	// instructions, never to an error; the install is already
	// recorded either way.
	if openConsole {
		HandoffToAdminConsole()
	} else {
		printConnectInstructions()
	}
	return nil
}

func validateBaseInstallSteps(steps []InstallStep) error {
	if len(steps) != len(baseInstallStepKeys) {
		return fmt.Errorf("base install has %d steps, ledger schema requires %d",
			len(steps), len(baseInstallStepKeys))
	}
	for i, step := range steps {
		if step.Key != baseInstallStepKeys[i] {
			return fmt.Errorf("base install step %d is %q, ledger schema requires %q",
				i+1, step.Key, baseInstallStepKeys[i])
		}
	}
	return nil
}

func prepareInstallCompletion(
	cfg *config.AppConfig, dec *InstallDecisions,
	ledger *installLedger, stepCount int,
) error {
	if !ledger.allBaseStepsDone() {
		return errors.New("cannot finalize installation before every base step is recorded")
	}
	logger.Install("all %d install steps complete", stepCount)
	if ledger.Context.DbCacheMB == nil {
		return errors.New("cannot finalize installation without recorded db cache")
	}
	cfg.DbCache = *ledger.Context.DbCacheMB
	dec.DbCacheMB = cfg.DbCache
	if ledger.done("identity.access") {
		if !AdminLoginObserved() {
			if err := ensureKeyVerificationPending(); err != nil {
				return fmt.Errorf("arm SSH login verification: %w", err)
			}
		} else if err := clearKeyVerificationPendingAt(
			paths.KeyVerificationMarker, 0); err != nil {
			return fmt.Errorf("clear SSH login verification: %w", err)
		}
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("write %s: %w", config.DefaultPath, err)
	}
	if err := finalizeOwnership(); err != nil {
		return err
	}
	return nil
}

func publishTerminalLedger(ledger *installLedger) error {
	if passwordPending() {
		return errors.New(
			"cannot mark installation complete while password delivery is pending")
	}
	if err := ledger.markComplete(); err != nil {
		return err
	}
	if err := ledger.save(paths.InstallStateFile); err != nil {
		return fmt.Errorf("publish terminal install ledger: %w", err)
	}
	return nil
}

func printGeneratedPassword(password string) error {
	_, err := fmt.Fprintf(os.Stdout,
		"\n  Login password for %q (SAVE IT — it will not be shown again):\n\n    %s\n",
		paths.AdminUser, password)
	if err != nil {
		return fmt.Errorf("display generated login password: %w", err)
	}
	return nil
}

// finalizeOwnership hands only the operator-facing log to vpn. The system
// configuration and its parent remain root:vpn and are never made writable by
// the TUI identity.
func finalizeOwnership() error {
	owner := paths.AdminUser + ":" + paths.AdminUser
	for _, p := range []string{paths.LogFile} {
		if err := system.SudoRun("chown", owner, p); err != nil {
			return fmt.Errorf("chown %s to %s: %w", p, owner, err)
		}
	}
	return nil
}

// fillUnattendedDecisions supplies the wizard answers for
// --unattended: every enumerated (non-decoy) key is copied — the
// spiritual successor of the script's cascade, from enumeration
// instead of guessing — and the password is randomly generated
// (ruling vii: random survives ONLY here) and printed at the
// end. dbcache takes the hardware recommendation.
//
// Zero-key strand guard: the interactive wizard refuses zero
// selected keys when observed password auth is off; this is the
// unattended equivalent. A box with no enumerable keys AND
// password auth off would finish with no SSH way in at all —
// the printed password works only at the provider console. That
// outcome must be asked for by name (--allow-console-only),
// never reached by default: a warning scrolling past in an
// unattended run is not consent.
func fillUnattendedDecisions(
	dec *InstallDecisions, allowConsoleOnly bool,
) error {
	dec.Keys = DedupeKeys(EnumerateKeySources())
	if strandsBox(len(dec.Keys), dec.Obs.PasswordAuth,
		allowConsoleOnly) {
		return errors.New(
			"refusing: no SSH keys found anywhere on this box " +
				"and password login is disabled in sshd — " +
				"completing would leave no SSH way in (the " +
				"generated password works only at a console). " +
				"Add a key, enable password auth, or re-run " +
				"with --allow-console-only if console-only " +
				"access is really what you want")
	}
	if err := fillGeneratedPassword(dec); err != nil {
		return err
	}
	dec.DbCacheMB = RecommendDbCache(DetectHardware().RAMMB)
	return nil
}

func fillGeneratedPassword(dec *InstallDecisions) error {
	gen, err := generateAdminPassword()
	if err != nil {
		return err
	}
	pw, err := loginpassword.New(gen)
	if err != nil {
		return err
	}
	dec.Password = pw
	dec.GeneratedPassword = gen
	return nil
}

// strandsBox is the zero-key strand condition: no keys to
// write, password auth off, and no explicit console-only
// consent. Pure — unit-tested.
func strandsBox(
	keyCount int, passwordAuth, allowConsoleOnly bool,
) bool {
	return keyCount == 0 && !passwordAuth && !allowConsoleOnly
}

// ── Absorbed bootstrap steps (script Phase 1) ────────────

// installBasePackages is the SINGLE clearnet apt operation
// (IA-2-L disclosure: op count unchanged from the script; ufw
// joined its package list per ruling xvi(b) so the firewall can
// come up immediately after).
func installBasePackages() error {
	if err := system.SudoRun("apt-get", "update", "-qq"); err != nil {
		return err
	}
	return system.SudoRun("apt-get", "install", "-y", "-qq",
		"sudo", "gnupg", "tor", "torsocks", "wget", "ufw")
}

// upgradeBasePackages brings the base image current (fresh VPS
// images are often weeks old with unpatched CVEs). Runs AFTER
// the firewall step per ruling xvi(b): default-deny now covers
// the longest pre-Tor phase instead of following it. confdef +
// confold keep existing config files on conflict — the safe
// default on a fresh image.
func upgradeBasePackages() error {
	return system.SudoRun("apt-get", "upgrade", "-y", "-qq",
		"-o", "Dpkg::Options::=--force-confdef",
		"-o", "Dpkg::Options::=--force-confold")
}

// prepareHost absorbs the script's host fixes: hostname
// resolution (prevents sudo delays) and NTP clock sync (Bitcoin
// Core and LND depend on accurate time for block timestamps,
// HTLC timeouts, and macaroon expiry; systemd-timesyncd uses the
// Debian pool, UTC).
func prepareHost() error {
	if name, err := os.Hostname(); err == nil && name != "" {
		if err := system.RunSilent(
			"getent", "hosts", name); err != nil {
			hosts, readErr := os.ReadFile("/etc/hosts")
			if readErr == nil {
				content := string(hosts)
				if !strings.HasSuffix(content, "\n") {
					content += "\n"
				}
				content += "127.0.0.1 " + name + "\n"
				if err := system.SudoWriteFile("/etc/hosts",
					[]byte(content), 0644); err != nil {
					return fmt.Errorf(
						"fix hostname resolution: %w", err)
				}
				logger.Install("hostname resolution fixed (%s)", name)
			}
		}
	}
	// Best-effort, like the script's `|| true`: a box without
	// timedatectl still installs; the clock-sync gap is logged.
	if err := system.SudoRunSilent(
		"timedatectl", "set-ntp", "true"); err != nil {
		logger.Install(
			"WARNING: could not enable NTP sync (%v)", err)
	}
	return nil
}

// buildInstallSteps returns the initial-install step list. Every
// step carries a stable Key (the ledger identity — versionless),
// a Kind (gates re-run every pass), a Group where steps hand
// ephemeral material to each other (see engine.go), and a Phase
// (bake vs first-boot, ruling iv — assignments provisional until
// the image-track session ratifies the map; identity/SSH steps
// are first-boot per rulings vii/viii: they apply observed,
// per-box state that an image build box cannot know).
//
// Order (ruling xvi(b)): the firewall step sits immediately
// after the single clearnet apt op — default-deny lands before
// the base upgrade, the longest pre-Tor phase. Outbound stays
// default-allow so Tor can bootstrap behind it; established SSH
// sessions are unaffected by `ufw enable`.
func buildInstallSteps(
	cfg *config.AppConfig, dec *InstallDecisions,
) []InstallStep {
	// Pipeline working directories — created in each pipeline's
	// first step, captured by closures, cleaned up in the final
	// step. Random paths via os.MkdirTemp prevent symlink
	// attacks. NOTE these closures are exactly why the btc/lnd
	// triplets are resume-atomic Groups: the workdir path lives
	// only in this process, so a resumed process can never
	// re-enter a pipeline midway.
	var btcWork, lndWork string

	return []InstallStep{
		{Key: "binary.install",
			Name: "Installing the vpn binary",
			Fn:   installSelfBinary},
		{Key: "apt.base",
			Name: "Installing base packages",
			Fn:   installBasePackages},
		{Key: "firewall", Name: "Configuring firewall",
			Fn: func() error { return configureInitialFirewall(cfg) }},
		{Key: "base.upgrade",
			Name: "Upgrading base packages",
			Fn:   upgradeBasePackages},
		{Key: "host.prep",
			Name: "Configuring hostname and clock sync",
			Fn:   prepareHost},
		{Key: "identity.access", Phase: PhaseFirstBoot,
			Name: "Creating the admin user (" +
				paths.AdminUser + ")",
			Fn: func() error {
				return applyIdentityAccess(dec)
			}},
		{Key: "service-identities.v1",
			Name: "Creating dedicated service identities",
			Fn: func() error {
				return createBaseServiceIdentities()
			}},
		{Key: "ipv6.disable", Name: "Disabling IPv6",
			Fn: disableIPv6},
		{Key: "tor.configure", Name: "Configuring Tor",
			Fn: func() error {
				if err := RebuildTorConfig(cfg); err != nil {
					return err
				}
				return enableAndRestartTor()
			}},
		// HARD GATE (IA-2-K): no Tor-dependent network step below —
		// apt over the socks5h proxy, every DownloadRequireTor —
		// runs unless Tor routing is verified. See torgate.go.
		// StepGate: re-verified on EVERY pass including resumes —
		// no download step can execute in a pass whose Tor routing
		// was not verified in that same pass. The torsocks-present
		// assertion re-homed here from preflight (ruling xvi(c))
		// also lives inside this step: post-Tor-install,
		// pre-first-download.
		{Key: "tor.gate", Name: "Verifying Tor routing",
			Kind: StepGate, Fn: verifyTorRouting},
		{Key: "apt.torproxy", Name: "Configuring apt for Tor",
			Fn: func() error {
				if err := configureAptTor(); err != nil {
					return err
				}
				return ensureGPG()
			}},
		{Key: "btc.download", Group: "btc",
			Name: "Downloading Bitcoin Core " + bitcoinVersion,
			Fn: func() error {
				var err error
				btcWork, err = os.MkdirTemp("", "vpn-btc-")
				if err != nil {
					return fmt.Errorf("create work dir: %w", err)
				}
				return downloadBitcoin(bitcoinVersion, btcWork)
			}},
		{Key: "btc.verify", Group: "btc",
			Name: "Verifying Bitcoin Core",
			Fn: func() error {
				if err := verifyBitcoinCoreSigs(
					btcWork, 2); err != nil {
					return err
				}
				return verifyBitcoin(btcWork)
			}},
		{Key: "btc.install", Group: "btc",
			Name: "Installing Bitcoin Core",
			Fn: func() error {
				if err := extractAndInstallBitcoin(
					bitcoinVersion, btcWork); err != nil {
					return err
				}
				os.RemoveAll(btcWork)
				if err := writeBitcoinConfig(cfg); err != nil {
					return err
				}
				return writeBitcoindService(bitcoinUser)
			}},
		{Key: "btc.start", Name: "Starting Bitcoin Core",
			Fn: func() error { return startBitcoind(cfg) }},
		{Key: "security", Name: "Configuring security",
			Fn: func() error {
				if err := installUnattendedUpgrades(); err != nil {
					return err
				}
				if err := configureUnattendedUpgrades(); err != nil {
					return err
				}
				if err := installFail2ban(); err != nil {
					return err
				}
				return configureFail2ban()
			}},

		// ── LND (Tor-only, non-interactive) ─────────
		{Key: "lnd.download", Group: "lnd",
			Name: "Downloading LND",
			Fn: func() error {
				var err error
				lndWork, err = os.MkdirTemp("", "vpn-lnd-")
				if err != nil {
					return fmt.Errorf("create work dir: %w", err)
				}
				return downloadLND(lndVersion, lndWork)
			}},
		{Key: "lnd.verify", Group: "lnd",
			Name: "Verifying LND",
			Fn: func() error {
				if err := verifyLNDSig(
					lndWork, lndVersion); err != nil {
					return err
				}
				return verifyLND(lndWork)
			}},
		{Key: "lnd.install", Group: "lnd",
			Name: "Installing LND",
			Fn: func() error {
				if err := extractAndInstallLND(
					lndVersion, lndWork); err != nil {
					return err
				}
				os.RemoveAll(lndWork)
				return writeLNDServiceFromConfig(cfg, lndUser)
			}},
		{Key: "tor.lnd", Name: "Configuring Tor for LND",
			Fn: func() error {
				if err := RebuildTorConfig(cfg); err != nil {
					return err
				}
				return enableAndRestartTor()
			}},
		{Key: "lnd.configure",
			Name: "Finalizing LND onion configuration",
			Fn: func() error {
				// Re-read the onion after the dedicated LND Tor
				// restart and rewrite lnd.conf with the preserved,
				// LND-only bitcoind credential. Missing or invalid
				// onion state fails closed inside writeLNDConfig.
				return writeLNDConfig(cfg, "")
			}},
		{Key: "lnd.start", Name: "Starting LND", Fn: startLND},
		{Key: "lnd.tls-san", Kind: StepGate,
			Name: "Verifying LND TLS onion certificate",
			Fn:   verifyLNDTLSOnionSAN},
		// LND owns its TLS certificate lifecycle. At startup it
		// replaces an expired certificate, and tlsautorefresh
		// also replaces one whose configured SAN inputs changed.
		// No TUI operation necessarily requested that startup.
		// This watch re-stages the TUI's copy within seconds of any
		// rewrite. After lnd.start so a migration pass arms it
		// on the certificate LND is actually serving.
		{Key: "lnd.certwatch",
			Name: "Watching the LND TLS certificate",
			Fn:   installLNDCertWatch},
		// The initial drop-in write + stale-drop-in deletion,
		// with the ruling-xv binding order inside (observe →
		// write new → delete old → validate → restart). Late in
		// the list, matching the script's placement: everything
		// the box needs to be reachable already ran.
		{Key: "ssh.harden", Phase: PhaseFirstBoot,
			Name: "Hardening SSH",
			Fn: func() error {
				return installSSHHardening()
			}},
		// The runtime privilege boundary. Three steps, all
		// first-boot (they need the admin user and group to
		// exist): journal read access for the admin user, the
		// root helper's socket-activated units, and the staging
		// board snapshot. After these — and with
		// identity.access granting no sudo — the end state
		// holds: the admin user has no root privilege at all,
		// and every privileged operation it can request is one
		// of the helper's fixed, typed, journal-audited verbs.
		{Key: "journal.access", Phase: PhaseFirstBoot,
			Name: "Granting journal read access",
			Fn:   setupJournalAccess},
		{Key: "helper.enable", Phase: PhaseFirstBoot,
			Name: "Enabling the root helper socket",
			Fn:   installHelperUnits},
		{Key: "state.stage", Phase: PhaseFirstBoot,
			Name: "Staging node facts for the TUI",
			Fn:   StageBoardAll},
		// Formerly a post-TUI special case that warned but
		// completed anyway (IA-1-16). As a real step it
		// inherits the ledger, the completion gate, failure
		// logging, and resume — the special case is dead.
		{Key: "shellenv", Name: "Configuring shell environment",
			Fn: func() error {
				return setupShellEnvironment(cfg)
			}},
	}
}

// UpgradeP2PToHybridSteps returns the one-way post-install transition from
// Tor-only to hybrid (clearnet+Tor) P2P. The root helper has already required
// authoritative mode=tor and supplies a validated hybrid desired view. These
// steps touch only LND's project-owned config, the two P2P-owned UFW rules,
// and LND itself; the helper owns persistence after every postcondition passes.
func UpgradeP2PToHybridSteps(
	cfg *config.AppConfig, publicIPv4 string,
) []InstallStep {
	// Note: we deliberately do NOT manually delete
	// the TLS cert here. LND has tlsautorefresh=1 in
	// its config, so when we rewrite lnd.conf with the
	// new tlsextraip line and restart LND, LND detects
	// the parameter change and replaces the cert and key
	// itself as part of startup. This
	// avoids the race where our gRPC client tries to
	// read the cert during the window between manual
	// deletion and LND's regeneration.
	steps := []InstallStep{
		{Name: "Checking active firewall",
			Fn: requireActiveUFW},
		{Name: "Updating LND config",
			Fn: func() error {
				return writeLNDConfig(cfg, publicIPv4)
			}},
		{Name: "Adding hybrid P2P firewall rules",
			Fn: allowHybridP2PFirewallRules},
		{Name: "Restarting LND",
			Fn: func() error {
				if err := system.SudoRun(
					"systemctl", "restart", "lnd"); err != nil {
					return err
				}
				if !system.IsServiceActive("lnd") {
					return fmt.Errorf("LND is not active after P2P restart")
				}
				return nil
			}},
		{Name: "Verifying LND TLS IP certificate",
			Fn: func() error {
				return verifyLNDTLSIPSAN(publicIPv4)
			}},
	}

	return steps
}

// ── Syncthing installation ───────────────────────────────

// SyncthingInstallSteps returns the install step list and a
// generated password. The root helper supplies a view with
// SyncthingEnabled=true so the canonical Tor template includes the add-on;
// UFW receives only Syncthing's owned rule. The helper stages the password and
// owns desired-state publication.
func SyncthingInstallSteps(
	cfg *config.AppConfig,
) ([]InstallStep, string, error) {
	passBytes := make([]byte, 12)
	if _, err := randRead(passBytes); err != nil {
		return nil, "", fmt.Errorf(
			"generate password: %w", err)
	}
	syncPassword := hexEncode(passBytes)

	var syncWork string
	steps := []InstallStep{
		{Name: "Downloading Syncthing " + syncthingVersion,
			Fn: func() error {
				var err error
				syncWork, err = os.MkdirTemp("", "vpn-sync-")
				if err != nil {
					return fmt.Errorf("create work dir: %w", err)
				}
				return downloadSyncthing(
					syncthingVersion, syncWork)
			}},
		{Name: "Verifying Syncthing",
			Fn: func() error {
				if err := verifySyncthingSig(syncWork); err != nil {
					return err
				}
				return verifySyncthingChecksum(syncWork)
			}},
		{Name: "Installing Syncthing",
			Fn: func() error {
				if err := extractAndInstallSyncthing(
					syncthingVersion, syncWork); err != nil {
					return err
				}
				os.RemoveAll(syncWork)
				return nil
			}},
		{Name: "Creating Syncthing directories",
			Fn: func() error {
				if err := createSystemGroup(backupGroup); err != nil {
					return err
				}
				if err := createSystemUser(syncthingUser,
					paths.SyncthingDataDir); err != nil {
					return err
				}
				return createSyncthingDirs()
			}},
		{Name: "Creating Syncthing service",
			Fn: writeSyncthingService},
		{Name: "Configuring Syncthing authentication",
			Fn: func() error {
				return configureSyncthingAuth(syncPassword)
			}},
		{Name: "Adding Syncthing firewall rule",
			Fn: allowSyncthingFirewallRule},
		{Name: "Reloading Tor configuration",
			Fn: func() error {
				return configureAndReloadTorForSyncthing(cfg)
			}},
		{Name: "Starting Syncthing", Fn: startSyncthing},
		{Name: "Registering backup folder",
			Fn: registerBackupFolder},
		{Name: "Setting up channel backup watcher",
			Fn: func() error {
				return setupChannelBackupWatcher(cfg)
			}},
	}
	return steps, syncPassword, nil
}

// ── Self-update ──────────────────────────────────────────

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// SelfUpdateSteps returns the install steps for updating
// the vpn binary to newVersion. Steps are idempotent —
// no rollback needed on failure. No config save on
// success (binary replaced, takes effect on next SSH login).
func SelfUpdateSteps(newVersion string) []InstallStep {
	baseURL := fmt.Sprintf(
		"https://github.com/virtualprivatenode/vpn/releases/download/v%s",
		newVersion)
	tarball := fmt.Sprintf("vpn-%s-amd64.tar.gz",
		newVersion)

	var workDir string

	return []InstallStep{
		{Name: "Downloading v" + newVersion,
			Fn: func() error {
				var err error
				workDir, err = os.MkdirTemp("",
					"vpn-update-")
				if err != nil {
					return fmt.Errorf(
						"create work dir: %w", err)
				}
				if err := system.DownloadRequireTor(
					baseURL+"/"+tarball,
					filepath.Join(workDir, tarball)); err != nil {
					return err
				}
				if err := system.DownloadRequireTor(
					baseURL+"/SHA256SUMS",
					filepath.Join(workDir,
						"SHA256SUMS")); err != nil {
					return err
				}
				return system.DownloadRequireTor(
					baseURL+"/SHA256SUMS.asc",
					filepath.Join(workDir,
						"SHA256SUMS.asc"))
			}},
		{Name: "Verifying signature",
			Fn: func() error {
				return verifySelfUpdate(workDir)
			}},
		{Name: "Verifying checksum",
			Fn: func() error {
				cmd := exec.Command("sha256sum",
					"--ignore-missing", "--check",
					"SHA256SUMS")
				cmd.Dir = workDir
				output, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf(
						"checksum failed: %s",
						string(output))
				}
				return nil
			}},
		{Name: "Installing new binary",
			Fn: func() error {
				if err := system.Run("tar", "-xzf",
					filepath.Join(workDir, tarball),
					"-C", workDir); err != nil {
					return err
				}
				if err := system.SudoRun("install",
					"-m", "755",
					filepath.Join(workDir, "vpn"),
					"/usr/local/bin/vpn"); err != nil {
					return err
				}
				os.RemoveAll(workDir)
				return nil
			}},
	}
}

const versionCacheMaxAge = 24 * time.Hour

func CheckLatestVersion() string {
	if cached := readVersionCache(); cached != "" {
		return cached
	}

	if _, err := exec.LookPath("torsocks"); err != nil {
		return ""
	}
	output, err := system.RunContext(10*time.Second,
		"torsocks", "curl", "-sL",
		"https://api.github.com/repos/virtualprivatenode/vpn/releases/latest")
	if err != nil {
		return ""
	}

	var release githubRelease
	if err := json.Unmarshal([]byte(output), &release); err != nil {
		return ""
	}

	version := strings.TrimPrefix(release.TagName, "v")
	if version != "" {
		writeVersionCache(version)
	}
	return version
}

func readVersionCache() string {
	info, err := os.Stat(paths.VersionCacheFile)
	if err != nil {
		return ""
	}
	if time.Since(info.ModTime()) > versionCacheMaxAge {
		return ""
	}
	data, err := os.ReadFile(paths.VersionCacheFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeVersionCache(version string) {
	existing := readVersionCache()
	if existing == version {
		return
	}
	os.MkdirAll(paths.VersionCacheDir, 0750)
	os.WriteFile(paths.VersionCacheFile,
		[]byte(version), 0600)
}

func GetVersion() string {
	return appVersion
}

// ── Helpers ──────────────────────────────────────────────

// setupShellEnvironment writes the admin user's cli wrappers.
// Both run with NO privilege: they are the recovery path a
// zero-sudo box leans on when the TUI itself misbehaves,
// so they must work exactly as the admin user.
//
//   - bitcoin-cli authenticates with the node's own RPC
//     credential: the staged password is fed on stdin
//     (-stdinrpcpass), never on the command line, where it
//     would be visible in /proc/*/cmdline. The wrapper cannot
//     read bitcoin.conf (root-owned) and does not need to —
//     connection details are passed explicitly.
//   - lncli reads the staged certificate and macaroon copies.
//     The wallet-create ceremony passes its own flags
//     (cert-only — no macaroon exists yet); this wrapper is
//     for the day-to-day case.
func setupShellEnvironment(cfg *config.AppConfig) error {
	bashrc := paths.AdminBashrc
	data, _ := os.ReadFile(bashrc)
	existing := string(data)
	net, err := cfg.NetworkConfig()
	if err != nil {
		return err
	}

	var content string

	// bitcoin-cli wrapper
	if !strings.Contains(existing, "bitcoin-cli()") {
		btcNetFlag := bitcoinCLINetworkFlag(net)
		content += fmt.Sprintf(`
# -- Virtual Private Node --
# RPC password comes from the staged credential file on stdin;
# commands that themselves read stdin should be run with
# explicit flags instead of this wrapper.
bitcoin-cli() {
    /usr/local/bin/bitcoin-cli \
        -rpcconnect=127.0.0.1 \
        -rpcport=%d \
        -rpcuser=%s \
        -stdinrpcpass \%s
        "$@" < %s
}
export -f bitcoin-cli
`, net.RPCPort, BitcoindRPCUser, btcNetFlag,
			paths.StateBitcoindRPCPass)
	}

	// lncli wrapper — always set up now that LND is part of
	// the initial install
	if cfg.HasLND() &&
		!strings.Contains(existing, "lncli()") {
		lndNetFlag := lncliNetworkFlag(net)
		content += fmt.Sprintf(`
lncli() {
    /usr/local/bin/lncli \
        --rpcserver=%s \%s
        --macaroonpath=%s \
        --tlscertpath=%s \
        "$@"
}
export -f lncli
`, paths.LNDGRPCEndpoint, lndNetFlag,
			paths.StateLNDMacaroon, paths.StateLNDTLSCert)
	}

	if content == "" {
		return nil
	}

	f, err := os.OpenFile(bashrc,
		os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func bitcoinCLINetworkFlag(net *config.NetworkConfig) string {
	if net.BitcoinCLIFlag == "" {
		return ""
	}
	return "\n        " + net.BitcoinCLIFlag + " \\"
}

func lncliNetworkFlag(net *config.NetworkConfig) string {
	if net.Name == config.NetworkMainnet {
		return ""
	}
	return fmt.Sprintf("\n        --network=%s \\", net.LNDNetwork)
}

func randRead(b []byte) (int, error) {
	return randReadImpl(b)
}

func hexEncode(b []byte) string {
	return hexEncodeImpl(b)
}
