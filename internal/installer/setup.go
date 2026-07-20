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
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

const (
	bitcoinVersion   = "29.3"
	lndVersion       = "0.20.0-beta"
	syncthingVersion = "2.1.1"
	systemUser       = "bitcoin"
)

var appVersion = "dev"

func SetVersion(v string)   { appVersion = v }
func LndVersionStr() string { return lndVersion }

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
	// Network from --testnet4 ("" = mainnet, or keep a
	// pre-existing config's answer).
	Network string
	// Unattended runs with no TUI and no prompts (ruling iv/vii:
	// keys auto-copied from enumeration, password randomly
	// generated and printed once — the image path's fallback).
	Unattended bool
	// UntilBake runs only PhaseBake steps (image build
	// pipeline, ruling iv). Requires Unattended. The run ends
	// WITHOUT InstallComplete, without handoff, without the
	// verification banner — first-boot steps are still owed.
	UntilBake bool
}

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

	// Preflight (principle 3): assert the environment before the
	// first mutation; refuses with a full report on any failure.
	// Returns the sshd observation (ruling xvi(b)) consumed by
	// the firewall rules, the wizard copy, and the config seed.
	obs, err := RunPreflight()
	if err != nil {
		return err
	}

	// A pre-existing loadable config is the operator's prior
	// answers (migration requirement 1, ruling xv): a migrated
	// box pre-creates /etc/vpn/config.json by copy. Values it
	// carries are never clobbered; only ABSENT fields are
	// seeded. install_complete:true with an empty ledger is NOT
	// refused — explicit dispatch means the operator asked.
	cfg := config.Default()
	preExisting := false
	if pre, loadErr := config.Load(); loadErr == nil {
		cfg = pre
		preExisting = true
	} else if !os.IsNotExist(loadErr) {
		// An unloadable (present but corrupt) config is a
		// fail-stop, same direction as the TUI path (IA-1-C1):
		// silently installing over the operator's carried
		// answers would destroy them.
		return fmt.Errorf(
			"cannot read %s: %v — fix or remove the file, "+
				"then re-run", config.DefaultPath, loadErr)
	}

	if opts.Network != "" {
		if preExisting && cfg.Network != opts.Network {
			return fmt.Errorf(
				"--%s conflicts with network %q already set in %s "+
					"— drop the flag to keep the existing answer",
				opts.Network, cfg.Network, config.DefaultPath)
		}
		cfg.Network = opts.Network
	}

	// Seed from observation ONLY where the config never
	// answered (on a migrated box the two agree anyway: the
	// observation runs while the old drop-in still stands).
	if !config.RawFieldPresent("ssh_password_auth_disabled") {
		cfg.SSHPasswordAuthDisabled = !obs.PasswordAuth
	}
	// Observed ports are a recorded observation, not an
	// operator answer — a fresh observation always wins.
	cfg.SSHPorts = obs.Ports

	// LND is installed during initial setup (Tor-only default).
	// These fields are set IN MEMORY before the steps are built
	// because steps read them (Tor config and firewall rules
	// include LND hidden services and ports) — but they are
	// PERSISTED only on a complete run, below. A failed or
	// interrupted run must not record intent as fact (IA-1-9).
	cfg.P2PMode = "tor"
	cfg.LNDInstalled = true
	cfg.Components = "bitcoin+lnd"

	// The ledger lives in the config dir; the dir must exist
	// before the first step completes. Root-owned during the
	// install; ownership of the dir and config.json passes to
	// the admin user at completion (migration requirement 2).
	if err := os.MkdirAll(paths.ConfigDir, 0750); err != nil {
		return fmt.Errorf("create %s: %w", paths.ConfigDir, err)
	}

	dec := &InstallDecisions{Obs: obs}
	steps := buildInstallSteps(cfg, dec)
	if opts.UntilBake {
		steps = FilterPhase(steps, PhaseBake)
	}

	// completeInstall writes the durable completion record.
	// Every step verified complete this pass — only then may
	// the record say so: InstallComplete is DERIVED from
	// per-step results, never from a front-end returning
	// (IA-1-9); the intent fields set above reach disk only
	// through here. On the interactive path this runs the
	// moment the last step verifies, BEFORE the done screen
	// waits for the operator (live-run finding: the old
	// after-the-TUI order left completion unpersisted while
	// the done screen sat unattended). Idempotent.
	completeInstall := func() error {
		logger.Install(
			"all %d install steps complete", len(steps))
		cfg.InstallComplete = true
		cfg.InstallVersion = appVersion
		if dec.DbCacheMB > 0 {
			cfg.DbCache = dec.DbCacheMB
		}
		// First-run verification banner: armed when the ledger
		// shows this install lifecycle set up the admin user AND
		// the journal shows no admin login yet. Keyed on the
		// LEDGER, not on this pass (live-run finding: a run that
		// failed after the identity step and then resumed to
		// completion lost the banner, because the completing
		// pass ledger-skipped identity). The journal check keeps
		// the original guarantee too: a re-run after a verified
		// login never re-arms a stale banner.
		if loadLedger(paths.InstallStateFile).
			done("identity.access") && !AdminLoginObserved() {
			cfg.KeyVerificationPending = true
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf(
				"write %s: %w", config.DefaultPath, err)
		}
		finalizeOwnership()
		return nil
	}

	var res RunResult
	openConsole := false
	if opts.Unattended {
		if err := fillUnattendedDecisions(dec); err != nil {
			return err
		}
		fmt.Printf("\n  Virtual Private Node — unattended install\n\n")
		res, err = RunInstallUnattended(
			steps, appVersion, paths.InstallStateFile)
	} else {
		res, openConsole, err = runInstallWizard(
			cfg, steps, dec, appVersion, completeInstall)
	}
	if err != nil {
		return err
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
		// The unattended runner has no wait-for-input gap; the
		// record is written here, right after the last step.
		if err := completeInstall(); err != nil {
			return err
		}
	}

	if dec.GeneratedPassword != "" && dec.PasswordApplied {
		// Unattended fallback only (ruling vii), and only when
		// the identity step actually applied it this pass (a
		// ledger-skip means an older password stands). Printed
		// once, never logged.
		fmt.Printf("\n  Login password for %q (SAVE IT — it will "+
			"not be shown again):\n\n    %s\n",
			paths.AdminUser, dec.GeneratedPassword)
	}

	if opts.Unattended {
		printConnectInstructions()
		return nil
	}
	// The done screen offered a real choice (live-run fix):
	// Enter opens the node console here via the identity drop;
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

// finalizeOwnership hands the admin user the files it owns in
// the post-install world: the config dir + config.json
// (migration requirement 2 — a migrated box pre-created them
// root-owned via cp) and the log file (the TUI appends to it).
// The install ledger deliberately stays root-owned (ruling xiv).
// Failures are logged, not fatal: the install is complete; a
// wrong owner surfaces as a TUI config error the operator can
// fix, and the log names the fix.
func finalizeOwnership() {
	owner := paths.AdminUser + ":" + paths.AdminUser
	for _, p := range []string{
		paths.ConfigDir, paths.ConfigFile, paths.LogFile,
	} {
		if err := system.SudoRun("chown", owner, p); err != nil {
			logger.Install(
				"WARNING: chown %s to %s failed (%v) — fix with: "+
					"chown %s %s", p, owner, err, owner, p)
		}
	}
}

// fillUnattendedDecisions supplies the wizard answers for
// --unattended: every enumerated (non-decoy) key is copied — the
// spiritual successor of the script's cascade, from enumeration
// instead of guessing — and the password is randomly generated
// (ruling vii: random survives ONLY here) and printed at the
// end. dbcache takes the hardware recommendation.
func fillUnattendedDecisions(dec *InstallDecisions) error {
	dec.Keys = DedupeKeys(EnumerateKeySources())
	gen, err := generateAdminPassword()
	if err != nil {
		return err
	}
	pw, err := NewLoginPassword(gen)
	if err != nil {
		return err
	}
	dec.Password = pw
	dec.GeneratedPassword = gen
	dec.DbCacheMB = RecommendDbCache(DetectHardware().RAMMB)
	return nil
}

// ── Absorbed bootstrap steps (script Phase 1) ────────────

// installBasePackages is the SINGLE clearnet apt operation
// (IA-2-L disclosure: op count unchanged from the script; ufw
// joined its package list per ruling xvi(b) so the firewall can
// come up immediately after). On a migrated box the old
// install's apt Tor proxy is still configured, so even this op
// routes through Tor there.
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
			Fn: func() error { return configureFirewall(cfg) }},
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
		{Key: "user.create",
			Name: "Creating system user and directories",
			Fn: func() error {
				if err := createSystemUser(systemUser); err != nil {
					return err
				}
				return createBitcoinDirs(systemUser)
			}},
		{Key: "ipv6.disable", Name: "Disabling IPv6",
			Fn: disableIPv6},
		{Key: "tor.configure", Name: "Configuring Tor",
			Fn: func() error {
				if err := RebuildTorConfig(cfg); err != nil {
					return err
				}
				if err := addUserToTorGroup(systemUser); err != nil {
					return err
				}
				return restartTor()
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
				return writeBitcoindService(systemUser)
			}},
		{Key: "btc.start", Name: "Starting Bitcoin Core",
			Fn: startBitcoind},
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
				if err := createLNDDirs(systemUser); err != nil {
					return err
				}
				if err := writeLNDConfig(cfg, ""); err != nil {
					return err
				}
				return writeLNDServiceInitial(systemUser)
			}},
		{Key: "tor.lnd", Name: "Configuring Tor for LND",
			Fn: func() error {
				if err := RebuildTorConfig(cfg); err != nil {
					return err
				}
				return restartTor()
			}},
		{Key: "lnd.start", Name: "Starting LND", Fn: startLND},
		// The initial drop-in write + stale-drop-in deletion,
		// with the ruling-xv binding order inside (observe →
		// write new → delete old → validate → restart). Late in
		// the list, matching the script's placement: everything
		// the box needs to be reachable already ran.
		{Key: "ssh.harden", Phase: PhaseFirstBoot,
			Name: "Hardening SSH",
			Fn: func() error {
				return installSSHHardening(cfg)
			}},
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

// P2PUpgradeSteps returns the install steps for upgrading
// from Tor-only to hybrid (clearnet+Tor) P2P mode. The
// caller must set cfg.P2PMode = "hybrid" before running
// steps (so firewall and LND config include clearnet
// listeners), and must save config after steps complete.
// On failure the caller reverts cfg.P2PMode = "tor".
func P2PUpgradeSteps(
	cfg *config.AppConfig, publicIPv4 string,
) []InstallStep {
	// Note: we deliberately do NOT manually delete
	// the TLS cert here. LND has tlsautorefresh=1 in
	// its config, so when we rewrite lnd.conf with the
	// new tlsextraip line and restart LND, LND detects
	// the parameter change and regenerates the cert
	// itself, atomically, as part of its startup. This
	// avoids the race where our gRPC client tries to
	// read the cert during the window between manual
	// deletion and LND's regeneration.
	steps := []InstallStep{
		{Name: "Updating LND config",
			Fn: func() error {
				return writeLNDConfig(cfg, publicIPv4)
			}},
		{Name: "Updating firewall",
			Fn: func() error {
				return configureFirewall(cfg)
			}},
		{Name: "Restarting LND",
			Fn: func() error {
				return system.SudoRun(
					"systemctl", "restart", "lnd")
			}},
	}

	return steps
}

// ── Syncthing installation ───────────────────────────────

// SyncthingInstallSteps returns the install step list and a
// generated password. The caller is responsible for setting
// cfg.SyncthingInstalled = true before running steps (so Tor
// and firewall configs include Syncthing), and for saving
// cfg.SyncthingPassword after steps complete successfully.
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
			Fn: createSyncthingDirs},
		{Name: "Creating Syncthing service",
			Fn: writeSyncthingService},
		{Name: "Configuring Syncthing authentication",
			Fn: func() error {
				return configureSyncthingAuth(syncPassword)
			}},
		{Name: "Configuring firewall",
			Fn: func() error {
				return configureFirewall(cfg)
			}},
		{Name: "Rebuilding Tor config",
			Fn: func() error {
				return RebuildTorConfig(cfg)
			}},
		{Name: "Restarting Tor", Fn: restartTor},
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

func readFileOrDefault(path, def string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	return string(data)
}

func setupShellEnvironment(cfg *config.AppConfig) error {
	bashrc := paths.AdminBashrc
	data, _ := os.ReadFile(bashrc)
	existing := string(data)

	var content string

	// bitcoin-cli wrapper
	if !strings.Contains(existing, "bitcoin-cli()") {
		net := cfg.NetworkConfig()
		btcNetFlag := ""
		if net.Name == "testnet4" {
			btcNetFlag = "\n        -testnet4 \\"
		}
		content += fmt.Sprintf(`
# -- Virtual Private Node --
bitcoin-cli() {
    sudo -u bitcoin /usr/local/bin/bitcoin-cli \
        -datadir=/var/lib/bitcoin \
        -conf=/etc/bitcoin/bitcoin.conf \%s
        "$@"
}
export -f bitcoin-cli
`, btcNetFlag)
	}

	// lncli wrapper — always set up now that LND is part of
	// the initial install
	if cfg.HasLND() &&
		!strings.Contains(existing, "lncli()") {
		net := cfg.NetworkConfig()
		lndNetFlag := ""
		if net.Name != "mainnet" {
			lndNetFlag = fmt.Sprintf(
				"\n        --network=%s \\", net.LNCLINetwork)
		}
		content += fmt.Sprintf(`
lncli() {
    sudo -u bitcoin /usr/local/bin/lncli \
        --lnddir=/var/lib/lnd \%s
        --macaroonpath=/var/lib/lnd/data/chain/bitcoin/%s/admin.macaroon \
        --tlscertpath=/var/lib/lnd/tls.cert \
        "$@"
}
export -f lncli
`, lndNetFlag, net.LNCLINetwork)
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

func randRead(b []byte) (int, error) {
	return randReadImpl(b)
}

func hexEncode(b []byte) string {
	return hexEncodeImpl(b)
}
