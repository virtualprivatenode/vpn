package installer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

const (
	bitcoinVersion = "29.3"
	lndVersion     = "0.20.0-beta"
	systemUser     = "bitcoin"
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

// ── Info and Confirm boxes ───────────────────────────────

type infoBoxModel struct {
	content       string
	width, height int
}

func (m infoBoxModel) Init() tea.Cmd { return nil }
func (m infoBoxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		if msg.String() == "enter" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m infoBoxModel) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}
	box := theme.Box.Padding(1, 3).
		Width(min(m.width-8, 70)).Render(m.content)
	content := lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, box)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
func ShowInfoBox(content string) {
	p := tea.NewProgram(infoBoxModel{content: content})
	p.Run()
}

type confirmBoxModel struct {
	content       string
	confirmed     bool
	width, height int
}

func (m confirmBoxModel) Init() tea.Cmd { return nil }
func (m confirmBoxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.confirmed = true
			return m, tea.Quit
		case "backspace", "ctrl+c", "q":
			m.confirmed = false
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m confirmBoxModel) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}
	box := theme.Box.Padding(1, 3).
		Width(min(m.width-8, 70)).Render(m.content)
	content := lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, box)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
func ShowConfirmBox(content string) bool {
	m := confirmBoxModel{content: content}
	p := tea.NewProgram(m)
	result, _ := p.Run()
	return result.(confirmBoxModel).confirmed
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
				if err := restartTor(); err != nil {
					return err
				}
				return logTorStatus()
			}},
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
				if err := importBitcoinCoreKeys(); err != nil {
					return err
				}
				return downloadBitcoin(bitcoinVersion)
			}},
		{Name: "Verifying Bitcoin Core",
			Fn: func() error {
				if err := verifyBitcoinCoreSigs(2); err != nil {
					return err
				}
				return verifyBitcoin()
			}},
		{Name: "Installing Bitcoin Core",
			Fn: func() error {
				if err := extractAndInstallBitcoin(bitcoinVersion); err != nil {
					return err
				}
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
				if err := importLNDKey(); err != nil {
					return err
				}
				return downloadLND(lndVersion)
			}},
		{Name: "Verifying LND",
			Fn: func() error {
				if err := verifyLNDSig(lndVersion); err != nil {
					return err
				}
				return verifyLND()
			}},
		{Name: "Installing LND",
			Fn: func() error {
				if err := extractAndInstallLND(lndVersion); err != nil {
					return err
				}
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

// ── Wallet creation ──────────────────────────────────────

func RunWalletCreation(cfg *config.AppConfig) error {
	net := cfg.NetworkConfig()
	info := theme.Header.Render("Create Your LND Wallet") + "\n\n" +
		theme.Warning.Render("IMPORTANT: Read before proceeding") + "\n\n" +
		theme.Value.Render("  LND will display your 24-word seed phrase") + "\n" +
		theme.Value.Render("  in the terminal. It is shown ONCE and") + "\n" +
		theme.Value.Render("  cannot be displayed again.") + "\n\n" +
		theme.Value.Render("  Before proceeding:") + "\n" +
		theme.Value.Render("  * Make sure you are in a private area") + "\n" +
		theme.Value.Render("  * Have pen and paper ready") + "\n\n" +
		theme.Value.Render("  LND will ask you to:") + "\n" +
		theme.Value.Render("  1. Enter a wallet password (min 8 characters)") + "\n" +
		theme.Value.Render("  2. Confirm the password") + "\n" +
		theme.Value.Render("  3. 'n' to create a new seed phrase") + "\n" +
		theme.Value.Render("  4. Optionally set a cipher seed Passphrase") + "\n" +
		theme.Value.Render("     (press Enter to skip, most people should skip)") + "\n" +
		theme.Value.Render("  5. WRITE DOWN your 24-word seed phrase") + "\n\n" +
		theme.Warning.Render("Your seed is the ONLY way to recover funds.") + "\n" +
		theme.Warning.Render("No one can help you if you lose it.") + "\n\n" +
		theme.Dim.Render("Enter to proceed -- backspace to cancel")
	if !ShowConfirmBox(info) {
		return nil
	}

	fmt.Print("\033[2J\033[H")
	fmt.Println("\n  ===================================================")
	fmt.Println("    LND Wallet Creation")
	fmt.Println("  ===================================================")
	fmt.Println("  Waiting for LND...")
	if err := waitForLND(); err != nil {
		return err
	}
	fmt.Println("  LND is ready")
	fmt.Println()

	cmd := exec.Command("sudo", "-u", systemUser, "lncli",
		"--lnddir=/var/lib/lnd", "--network="+net.LNCLINetwork,
		"create")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("lncli create: %w", err)
	}

	fmt.Println()
	fmt.Println("  ===================================================")
	fmt.Println("  Your 24-word seed is displayed above.")
	fmt.Println("  Write it down NOW. It will not be shown again.")
	fmt.Println()
	fmt.Println("  Store it safely:")
	fmt.Println("  * Write on paper and store securely")
	fmt.Println("  * Never share it with anyone")
	fmt.Println()
	fmt.Println("  ===================================================")
	fmt.Println()
	fmt.Print("  Type 'I SAVED MY SEED' to continue: ")

	reader := bufio.NewReader(os.Stdin)
	for {
		confirmation, _ := reader.ReadString('\n')
		confirmation = strings.TrimSpace(confirmation)
		if confirmation == "I SAVED MY SEED" {
			break
		}
		fmt.Print("  Please type exactly: I SAVED MY SEED: ")
	}

	fmt.Println()
	fmt.Println("  Seed confirmed.")
	fmt.Println()

	unlockMsg := theme.Header.Render("Auto-Unlock Configuration") + "\n\n" +
		theme.Value.Render("Enter your WALLET PASSWORD in the next screen.") + "\n" +
		theme.Value.Render("This is the password you entered at the") + "\n" +
		theme.Value.Render("very start of wallet creation (step 1).") + "\n\n" +
		theme.Warning.Render(" This is NOT your seed phrase") + "\n" +
		theme.Warning.Render(" This is NOT a cipher seed Passphrase") + "\n\n" +
		theme.Value.Render("Your wallet password will be stored so LND") + "\n" +
		theme.Value.Render("starts automatically after reboot.") + "\n\n" +
		theme.Dim.Render("Press Enter to continue...")
	ShowInfoBox(unlockMsg)

	fmt.Print("\033[2J\033[H")
	fmt.Println("\n  ===================================================")
	fmt.Println("    Auto-Unlock -- Enter Wallet Password")
	fmt.Println("  ===================================================")
	fmt.Println()
	fmt.Println("  Enter the SAME password you used at the")
	fmt.Println("  start of wallet creation (step 1).")
	fmt.Println()
	fmt.Println("  NOT your seed phrase")
	fmt.Println("  NOT a cipher seed Passphrase")
	fmt.Println()

	var matched bool
	for attempt := 0; attempt < 3; attempt++ {
		fmt.Print("  Enter your wallet password: ")
		pw1 := readPassword()
		fmt.Println()

		if pw1 == "" {
			fmt.Println("  Password cannot be empty.")
			if attempt < 2 {
				fmt.Println("  Try again.")
			}
			continue
		}

		fmt.Print("  Confirm your wallet password: ")
		pw2 := readPassword()
		fmt.Println()

		if pw1 != pw2 {
			fmt.Println("  Passwords do not match.")
			if attempt < 2 {
				fmt.Println("  Try again.")
			}
			continue
		}

		if err := setupAutoUnlock(pw1); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		} else {
			fmt.Println("  Auto-unlock configured")
		}
		cfg.AutoUnlock = true
		matched = true
		break
	}

	if !matched {
		fmt.Println("  Skipping auto-unlock. " +
			"You will need to unlock LND manually after reboot.")
		fmt.Println("    Run: lncli unlock")
	}
	cfg.WalletCreated = true
	config.Save(cfg)
	fmt.Print("\033[2J\033[H")
	return nil
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
	steps := []InstallStep{
		{Name: "Removing old TLS certificate",
			Fn: func() error {
				system.SudoRunSilent("rm", "-f",
					paths.LNDTLSCert, paths.LNDTLSKey)
				return nil
			}},
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

	if cfg.LndHubInstalled {
		steps = append(steps,
			InstallStep{
				Name: "Generating TLS certificate for LndHub proxy",
				Fn: func() error {
					return generateProxyCert(publicIPv4)
				}},
			InstallStep{
				Name: "Creating LndHub proxy service",
				Fn:   writeLndHubProxyService},
			InstallStep{
				Name: "Starting LndHub TLS proxy",
				Fn:   startLndHubProxy},
		)
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

	steps := []InstallStep{
		{Name: "Adding Syncthing repository",
			Fn: installSyncthingRepo},
		{Name: "Installing Syncthing",
			Fn: installSyncthingPackage},
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

// ── LndHub installation ──────────────────────────────────

// LndHubInstallSteps returns the install step list, the
// generated admin token, and the DB password. The caller
// is responsible for setting cfg.LndHubInstalled = true
// before running steps, and for saving cfg.LndHubAdminToken
// and cfg.LndHubDBPassword after steps complete.
func LndHubInstallSteps(
	cfg *config.AppConfig,
) ([]InstallStep, string, string, error) {
	dbPassBytes := make([]byte, 16)
	if _, err := randRead(dbPassBytes); err != nil {
		return nil, "", "", fmt.Errorf(
			"generate db password: %w", err)
	}
	dbPassword := hexEncode(dbPassBytes)

	jwtBytes := make([]byte, 32)
	if _, err := randRead(jwtBytes); err != nil {
		return nil, "", "", fmt.Errorf(
			"generate jwt secret: %w", err)
	}
	jwtSecret := hexEncode(jwtBytes)

	adminBytes := make([]byte, 24)
	if _, err := randRead(adminBytes); err != nil {
		return nil, "", "", fmt.Errorf(
			"generate admin token: %w", err)
	}
	adminToken := hexEncode(adminBytes)

	publicIPv4 := ""
	if cfg.P2PMode == "hybrid" {
		publicIPv4 = system.PublicIPv4()
	}

	steps := []InstallStep{
		{Name: "Installing Go toolchain",
			Fn: installGoToolchain},
		{Name: "Installing PostgreSQL",
			Fn: installPostgreSQL},
		{Name: "Creating database",
			Fn: func() error {
				return createLndHubDatabase(dbPassword)
			}},
		{Name: "Cloning lndhub.go v" + lndhubVersion,
			Fn: cloneLndHub},
		{Name: "Building lndhub (from source)",
			Fn: buildLndHub},
		{Name: "Installing binary",
			Fn: installLndHubBinary},
		{Name: "Baking LND macaroon",
			Fn: func() error {
				return bakeLndHubMacaroon(cfg)
			}},
		{Name: "Creating directories",
			Fn: createLndHubDirs},
		{Name: "Writing configuration",
			Fn: func() error {
				return writeLndHubConfig(
					cfg, dbPassword, jwtSecret, adminToken)
			}},
		{Name: "Creating service",
			Fn: writeLndHubService},
		{Name: "Configuring firewall",
			Fn: func() error {
				return configureFirewall(cfg)
			}},
		{Name: "Rebuilding Tor config",
			Fn: func() error {
				return RebuildTorConfig(cfg)
			}},
		{Name: "Restarting Tor", Fn: restartTor},
		{Name: "Starting LndHub", Fn: startLndHub},
	}

	if cfg.P2PMode == "hybrid" && publicIPv4 != "" {
		steps = append(steps,
			InstallStep{
				Name: "Generating TLS certificate for LndHub proxy",
				Fn: func() error {
					return generateProxyCert(publicIPv4)
				}},
			InstallStep{
				Name: "Creating LndHub proxy service",
				Fn:   writeLndHubProxyService},
			InstallStep{
				Name: "Starting LndHub TLS proxy",
				Fn:   startLndHubProxy},
		)
	}

	return steps, adminToken, dbPassword, nil
}

// ── Choice box ───────────────────────────────────────────

type choiceBoxModel struct {
	content       string
	choices       []string
	result        string
	width, height int
}

func (m choiceBoxModel) Init() tea.Cmd { return nil }
func (m choiceBoxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "backspace", "ctrl+c":
			return m, tea.Quit
		default:
			for _, c := range m.choices {
				if msg.String() == c {
					m.result = c
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}
func (m choiceBoxModel) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}
	box := theme.Box.Padding(1, 3).
		Width(min(m.width-8, 70)).Render(m.content)
	content := lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, box)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
func showChoiceBox(content string, choices []string) string {
	m := choiceBoxModel{content: content, choices: choices}
	p := tea.NewProgram(m)
	result, _ := p.Run()
	return result.(choiceBoxModel).result
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
	expectedFP := "AFA0EBACDC9A4C4AA7B0154AC97CE10F170BA5FE"
	tarball := fmt.Sprintf("rlvpn-%s-amd64.tar.gz",
		newVersion)

	return []InstallStep{
		{Name: "Downloading v" + newVersion,
			Fn: func() error {
				return system.DownloadRequireTor(
					baseURL+"/"+tarball,
					"/tmp/"+tarball)
			}},
		{Name: "Downloading checksums",
			Fn: func() error {
				if err := system.DownloadRequireTor(
					baseURL+"/SHA256SUMS",
					"/tmp/rlvpn-SHA256SUMS"); err != nil {
					return err
				}
				return system.DownloadRequireTor(
					baseURL+"/SHA256SUMS.asc",
					"/tmp/rlvpn-SHA256SUMS.asc")
			}},
		{Name: "Importing release key",
			Fn: func() error {
				// Skip download if key is already imported
				// (e.g. from a previous update).
				output, err := system.RunCombinedOutput(
					"gpg", "--batch", "--with-colons",
					"--list-keys", expectedFP)
				if err == nil &&
					strings.Contains(output, expectedFP) {
					return nil
				}

				keyFile := "/tmp/rlvpn-release-key.asc"
				keyURL := fmt.Sprintf(
					"https://keys.openpgp.org/vks/v1/by-fingerprint/%s",
					expectedFP)
				if err := system.DownloadRequireTor(
					keyURL, keyFile); err != nil {
					return fmt.Errorf(
						"could not download signing key: %w",
						err)
				}
				defer os.Remove(keyFile)
				if _, err := system.RunCombinedOutput(
					"gpg", "--batch", "--import",
					keyFile); err != nil {
					return fmt.Errorf(
						"could not import signing key: %w",
						err)
				}
				output, err = system.RunCombinedOutput(
					"gpg", "--batch", "--with-colons",
					"--list-keys", expectedFP)
				if err != nil ||
					!strings.Contains(output, expectedFP) {
					return fmt.Errorf(
						"release key fingerprint mismatch")
				}
				return nil
			}},
		{Name: "Verifying signature",
			Fn: func() error {
				output, err := system.RunCombinedOutput(
					"gpg", "--batch", "--verify",
					"/tmp/rlvpn-SHA256SUMS.asc",
					"/tmp/rlvpn-SHA256SUMS")
				if err != nil {
					return fmt.Errorf(
						"signature verification failed: %s",
						output)
				}
				return nil
			}},
		{Name: "Verifying checksum",
			Fn: func() error {
				cmd := exec.Command("sha256sum",
					"--ignore-missing", "--check",
					"rlvpn-SHA256SUMS")
				cmd.Dir = "/tmp"
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
					"/tmp/"+tarball, "-C",
					"/tmp"); err != nil {
					return err
				}
				if err := system.SudoRun("install",
					"-m", "755", "/tmp/rlvpn",
					"/usr/local/bin/rlvpn"); err != nil {
					return err
				}
				os.Remove("/tmp/" + tarball)
				os.Remove("/tmp/rlvpn-SHA256SUMS")
				os.Remove("/tmp/rlvpn-SHA256SUMS.asc")
				os.Remove("/tmp/rlvpn")
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

func readPassword() string {
	for attempts := 0; attempts < 3; attempts++ {
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Printf("\n  Error reading password: %v\n", err)
			if attempts < 2 {
				fmt.Print("  Try again: ")
			}
			continue
		}
		if len(pw) == 0 {
			fmt.Println("\n  Password cannot be empty.")
			if attempts < 2 {
				fmt.Print("  Try again: ")
			}
			continue
		}
		return string(pw)
	}
	fmt.Println("\n  Failed to read password after 3 attempts.")
	return ""
}

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
