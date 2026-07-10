package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
	"github.com/ripsline/virtual-private-node/internal/theme"
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

func NeedsInstall() bool {
	cfg, err := config.Load()
	if err != nil {
		if system.IsServiceActive("bitcoind") {
			return false
		}
		return true
	}
	return !cfg.InstallComplete
}

// ── Install step types ──────────────────────────────────
// Exported for use by welcome.InstallProgressScreen.
// The step functions themselves stay unexported — callers
// use the builder functions below to get step slices.

type StepStatus int

const (
	StepPending StepStatus = iota
	StepRunning
	StepDone
	StepFailed
)

type InstallStep struct {
	Name   string
	Fn     func() error
	Status StepStatus
	Err    error
}

type stepDoneMsg struct {
	index int
	err   error
}

type installModel struct {
	steps         []InstallStep
	current       int
	done, failed  bool
	version       string
	width, height int
}

func (m installModel) Init() tea.Cmd { return m.runStep(0) }

func (m installModel) runStep(i int) tea.Cmd {
	return func() tea.Msg {
		if i >= len(m.steps) {
			return stepDoneMsg{index: i}
		}
		return stepDoneMsg{index: i, err: m.steps[i].Fn()}
	}
}

func (m installModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		if msg.String() == "enter" && m.done {
			return m, tea.Quit
		}
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case stepDoneMsg:
		if msg.index < len(m.steps) {
			if msg.err != nil {
				m.steps[msg.index].Status = StepFailed
				m.steps[msg.index].Err = msg.err
				m.failed = true
				m.done = true
				return m, nil
			}
			m.steps[msg.index].Status = StepDone
			next := msg.index + 1
			if next < len(m.steps) {
				m.current = next
				m.steps[next].Status = StepRunning
				return m, m.runStep(next)
			}
			m.done = true
		}
	}
	return m, nil
}

func (m installModel) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	for i, s := range m.steps {
		var sty lipgloss.Style
		var ind string
		switch s.Status {
		case StepDone:
			sty, ind = theme.ProgDone, "[done]"
		case StepRunning:
			sty, ind = theme.ProgRunning, "[....]"
		case StepFailed:
			sty, ind = theme.ProgFail, "[FAIL]"
		default:
			sty, ind = theme.ProgPending, "[wait]"
		}
		lines = append(lines, sty.Render(fmt.Sprintf("  %s [%d/%d] %s",
			ind, i+1, len(m.steps), s.Name)))
		if s.Status == StepFailed && s.Err != nil {
			lines = append(lines, theme.ProgFail.Render(
				fmt.Sprintf("      Error: %v", s.Err)))
		}
	}
	box := theme.ProgBox.Width(bw).Render(strings.Join(lines, "\n"))
	var footer string
	if m.done && !m.failed {
		footer = theme.Success.Render(
			"  Complete -- press Enter to continue  ")
	} else if m.failed {
		footer = theme.ProgFail.Render(
			"  Failed. Press ctrl+c to exit.  ")
	} else {
		footer = theme.Dim.Render("  Installing... please wait  ")
	}
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", box, "", footer)
	content := lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func RunInstallTUI(steps []InstallStep, version string) error {
	if len(steps) == 0 {
		return nil
	}
	steps[0].Status = StepRunning
	m := installModel{steps: steps, version: version}
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return err
	}
	final := result.(installModel)
	if final.failed {
		for _, s := range final.steps {
			if s.Status == StepFailed {
				return fmt.Errorf("%s: %w", s.Name, s.Err)
			}
		}
	}
	return nil
}

// ── Main install flow ────────────────────────────────────

func Run() error {
	if err := checkOS(); err != nil {
		return err
	}

	cfg := config.Default()
	if preCfg, err := config.Load(); err == nil {
		cfg = preCfg
	}

	// LND is now installed during initial setup (Tor-only default).
	// Set config fields before buildSteps so Tor config and firewall
	// rules include LND hidden services and ports.
	cfg.P2PMode = "tor"
	cfg.LNDInstalled = true
	cfg.Components = "bitcoin+lnd"

	steps := buildSteps(cfg)

	if err := RunInstallTUI(steps, appVersion); err != nil {
		return err
	}
	if err := setupShellEnvironment(cfg); err != nil {
		logger.Install("Warning: shell setup failed: %v", err)
		fmt.Printf("  Warning: shell setup failed: %v\n", err)
	}
	cfg.InstallComplete = true
	cfg.InstallVersion = appVersion
	return config.Save(cfg)
}

func buildSteps(cfg *config.AppConfig) []InstallStep {
	// Pipeline working directories — created in each pipeline's
	// first step, captured by closures, cleaned up in the final
	// step. Random paths via os.MkdirTemp prevent symlink attacks.
	var btcWork, lndWork string

	return []InstallStep{
		{Name: "Creating system user and directories",
			Fn: func() error {
				if err := createSystemUser(systemUser); err != nil {
					return err
				}
				return createBitcoinDirs(systemUser)
			}},
		{Name: "Disabling IPv6", Fn: disableIPv6},
		{Name: "Installing Tor",
			Fn: func() error {
				if err := installTor(); err != nil {
					return err
				}
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
		{Name: "Verifying Tor routing", Fn: verifyTorRouting},
		{Name: "Configuring apt for Tor",
			Fn: func() error {
				if err := configureAptTor(); err != nil {
					return err
				}
				return ensureGPG()
			}},
		{Name: "Configuring firewall",
			Fn: func() error { return configureFirewall(cfg) }},
		{Name: "Downloading Bitcoin Core " + bitcoinVersion,
			Fn: func() error {
				var err error
				btcWork, err = os.MkdirTemp("", "rlvpn-btc-")
				if err != nil {
					return fmt.Errorf("create work dir: %w", err)
				}
				return downloadBitcoin(bitcoinVersion, btcWork)
			}},
		{Name: "Verifying Bitcoin Core",
			Fn: func() error {
				if err := verifyBitcoinCoreSigs(
					btcWork, 2); err != nil {
					return err
				}
				return verifyBitcoin(btcWork)
			}},
		{Name: "Installing Bitcoin Core",
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
		{Name: "Starting Bitcoin Core", Fn: startBitcoind},
		{Name: "Configuring security",
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
		{Name: "Downloading LND",
			Fn: func() error {
				var err error
				lndWork, err = os.MkdirTemp("", "rlvpn-lnd-")
				if err != nil {
					return fmt.Errorf("create work dir: %w", err)
				}
				return downloadLND(lndVersion, lndWork)
			}},
		{Name: "Verifying LND",
			Fn: func() error {
				if err := verifyLNDSig(
					lndWork, lndVersion); err != nil {
					return err
				}
				return verifyLND(lndWork)
			}},
		{Name: "Installing LND",
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
		{Name: "Configuring Tor for LND",
			Fn: func() error {
				if err := RebuildTorConfig(cfg); err != nil {
					return err
				}
				return restartTor()
			}},
		{Name: "Starting LND", Fn: startLND},
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
				syncWork, err = os.MkdirTemp("", "rlvpn-sync-")
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
// the rlvpn binary to newVersion. Steps are idempotent —
// no rollback needed on failure. No config save on
// success (binary replaced, takes effect on next SSH login).
func SelfUpdateSteps(newVersion string) []InstallStep {
	baseURL := fmt.Sprintf(
		"https://github.com/ripsline/virtual-private-node/releases/download/v%s",
		newVersion)
	tarball := fmt.Sprintf("rlvpn-%s-amd64.tar.gz",
		newVersion)

	var workDir string

	return []InstallStep{
		{Name: "Downloading v" + newVersion,
			Fn: func() error {
				var err error
				workDir, err = os.MkdirTemp("",
					"rlvpn-update-")
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
					filepath.Join(workDir, "rlvpn"),
					"/usr/local/bin/rlvpn"); err != nil {
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
		"https://api.github.com/repos/ripsline/virtual-private-node/releases/latest")
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
