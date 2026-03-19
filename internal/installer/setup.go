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

// ── Install progress TUI ─────────────────────────────────

type stepStatus int

const (
	stepPending stepStatus = iota
	stepRunning
	stepDone
	stepFailed
)

type installStep struct {
	name   string
	fn     func() error
	status stepStatus
	err    error
}

type stepDoneMsg struct {
	index int
	err   error
}

type installModel struct {
	steps         []installStep
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
		return stepDoneMsg{index: i, err: m.steps[i].fn()}
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
				m.steps[msg.index].status = stepFailed
				m.steps[msg.index].err = msg.err
				m.failed = true
				m.done = true
				return m, nil
			}
			m.steps[msg.index].status = stepDone
			next := msg.index + 1
			if next < len(m.steps) {
				m.current = next
				m.steps[next].status = stepRunning
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
	title := theme.ProgTitle.Width(bw).Align(lipgloss.Center).
		Render(fmt.Sprintf(" Virtual Private Node v%s ", m.version))
	var lines []string
	for i, s := range m.steps {
		var sty lipgloss.Style
		var ind string
		switch s.status {
		case stepDone:
			sty, ind = theme.ProgDone, "[done]"
		case stepRunning:
			sty, ind = theme.ProgRunning, "[....]"
		case stepFailed:
			sty, ind = theme.ProgFail, "[FAIL]"
		default:
			sty, ind = theme.ProgPending, "[wait]"
		}
		lines = append(lines, sty.Render(fmt.Sprintf("  %s [%d/%d] %s",
			ind, i+1, len(m.steps), s.name)))
		if s.status == stepFailed && s.err != nil {
			lines = append(lines, theme.ProgFail.Render(
				fmt.Sprintf("      Error: %v", s.err)))
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
		"", title, "", box, "", footer)
	content := lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func RunInstallTUI(steps []installStep, version string) error {
	if len(steps) == 0 {
		return nil
	}
	steps[0].status = stepRunning
	m := installModel{steps: steps, version: version}
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return err
	}
	final := result.(installModel)
	if final.failed {
		for _, s := range final.steps {
			if s.status == stepFailed {
				return fmt.Errorf("%s: %w", s.name, s.err)
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

func buildSteps(cfg *config.AppConfig) []installStep {
	return []installStep{
		{name: "Creating system user",
			fn: func() error { return createSystemUser(systemUser) }},
		{name: "Creating directories",
			fn: func() error { return createBitcoinDirs(systemUser) }},
		{name: "Disabling IPv6", fn: disableIPv6},
		{name: "Checking Tor + torsocks", fn: installTor},
		{name: "Configuring Tor",
			fn: func() error { return RebuildTorConfig(cfg) }},
		{name: "Adding user to debian-tor group",
			fn: func() error { return addUserToTorGroup(systemUser) }},
		{name: "Starting Tor", fn: restartTor},
		{name: "Verifying Tor routing", fn: logTorStatus},
		{name: "Configuring apt for Tor", fn: configureAptTor},
		{name: "Installing GPG", fn: ensureGPG},
		{name: "Configuring firewall",
			fn: func() error { return configureFirewall(cfg) }},
		{name: "Importing Bitcoin Core signing keys",
			fn: importBitcoinCoreKeys},
		{name: "Downloading Bitcoin Core " + bitcoinVersion,
			fn: func() error { return downloadBitcoin(bitcoinVersion) }},
		{name: "Verifying Bitcoin Core signatures (2/5)",
			fn: func() error { return verifyBitcoinCoreSigs(2) }},
		{name: "Verifying Bitcoin Core checksum", fn: verifyBitcoin},
		{name: "Installing Bitcoin Core",
			fn: func() error {
				return extractAndInstallBitcoin(bitcoinVersion)
			}},
		{name: "Configuring Bitcoin Core",
			fn: func() error { return writeBitcoinConfig(cfg) }},
		{name: "Creating bitcoind service",
			fn: func() error {
				return writeBitcoindService(systemUser)
			}},
		{name: "Starting Bitcoin Core", fn: startBitcoind},
		{name: "Installing unattended-upgrades",
			fn: installUnattendedUpgrades},
		{name: "Configuring auto-security-updates",
			fn: configureUnattendedUpgrades},
		{name: "Installing fail2ban", fn: installFail2ban},
		{name: "Configuring fail2ban", fn: configureFail2ban},
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
		theme.Value.Render("  * Have pen and paper ready") + "\n" +
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

// ── LND installation ─────────────────────────────────────

func RunLNDInstall(cfg *config.AppConfig) error {
	confirmMsg := theme.Header.Render("Install LND "+lndVersion) + "\n\n" +
		theme.Value.Render("This will:") + "\n\n" +
		theme.Value.Render("  * Download and verify LND v"+lndVersion) + "\n" +
		theme.Value.Render("  * Configure LND for "+cfg.Network) + "\n" +
		theme.Value.Render("  * Create Tor hidden services for LND") + "\n" +
		theme.Value.Render("  * Restart Tor") + "\n\n" +
		theme.Dim.Render("Enter to proceed -- backspace to cancel")
	if !ShowConfirmBox(confirmMsg) {
		return nil
	}

	p2pMode := "tor"
	p2pMsg := theme.Header.Render("LND P2P Mode") + "\n\n" +
		theme.Value.Render("  [1] Tor only -- Maximum privacy") + "\n" +
		theme.Value.Render("  [2] Hybrid  -- Tor + clearnet, better routing") + "\n\n" +
		theme.Dim.Render("Press 1 or 2 -- backspace to cancel")
	p2pChoice := showChoiceBox(p2pMsg, []string{"1", "2"})
	if p2pChoice == "" {
		return nil
	}
	if p2pChoice == "2" {
		p2pMode = "hybrid"
	}

	publicIPv4 := ""
	if p2pMode == "hybrid" {
		publicIPv4 = system.PublicIPv4()
		if publicIPv4 == "" {
			p2pMode = "tor"
		}
	}

	cfg.P2PMode = p2pMode
	cfg.LNDInstalled = true
	cfg.Components = "bitcoin+lnd"

	steps := []installStep{
		{name: "Importing LND signing key", fn: importLNDKey},
		{name: "Downloading LND " + lndVersion,
			fn: func() error { return downloadLND(lndVersion) }},
		{name: "Verifying LND signature",
			fn: func() error { return verifyLNDSig(lndVersion) }},
		{name: "Verifying LND checksum", fn: verifyLND},
		{name: "Installing LND",
			fn: func() error { return extractAndInstallLND(lndVersion) }},
		{name: "Creating LND directories",
			fn: func() error { return createLNDDirs(systemUser) }},
		{name: "Configuring LND",
			fn: func() error { return writeLNDConfig(cfg, publicIPv4) }},
		{name: "Creating LND service",
			fn: func() error {
				return writeLNDServiceInitial(systemUser)
			}},
		{name: "Configuring firewall",
			fn: func() error { return configureFirewall(cfg) }},
		{name: "Rebuilding Tor config",
			fn: func() error { return RebuildTorConfig(cfg) }},
		{name: "Restarting Tor", fn: restartTor},
		{name: "Starting LND", fn: startLND},
	}
	if err := RunInstallTUI(steps, appVersion); err != nil {
		cfg.LNDInstalled = false
		cfg.Components = "bitcoin"
		RebuildTorConfig(cfg)
		restartTor()
		return err
	}
	return config.Save(cfg)
}

func RunP2PModeUpgrade(cfg *config.AppConfig) error {
	if cfg.P2PMode == "hybrid" {
		return nil
	}

	publicIPv4 := system.PublicIPv4()
	if publicIPv4 == "" {
		ShowInfoBox(
			theme.Header.Render("Cannot Detect Public IP") + "\n\n" +
				theme.Value.Render("Could not determine your server's public IP.") + "\n" +
				theme.Value.Render("Hybrid mode requires a public IPv4 address.") + "\n\n" +
				theme.Dim.Render("Press Enter to return..."))
		return nil
	}

	confirmMsg := theme.Header.Render("Upgrade to Hybrid P2P Mode") + "\n\n" +
		theme.Value.Render("This will:") + "\n\n" +
		theme.Value.Render("  * Expose your server IP to the Lightning Network") + "\n" +
		theme.Value.Render("  * Open ports 9735 and 8080 in the firewall") + "\n" +
		theme.Value.Render("  * Allow Zeus to connect over clearnet") + "\n" +
		theme.Value.Render("  * Regenerate LND TLS certificate") + "\n" +
		theme.Value.Render("  * Restart LND") + "\n\n"

	if cfg.LndHubInstalled {
		confirmMsg += theme.Value.Render("  * Install TLS proxy for LndHub clearnet") + "\n" +
			theme.Value.Render("  * Open port 3000 for encrypted LndHub access") + "\n\n"
	}

	confirmMsg += theme.Warning.Render("Your IP: "+publicIPv4) + "\n" +
		theme.Warning.Render("This cannot be undone -- your IP will be public.") + "\n\n" +
		theme.Dim.Render("Enter to proceed -- backspace to cancel")
	if !ShowConfirmBox(confirmMsg) {
		return nil
	}

	cfg.P2PMode = "hybrid"

	steps := []installStep{
		{name: "Removing old TLS certificate", fn: func() error {
			system.SudoRunSilent("rm", "-f",
				paths.LNDTLSCert, paths.LNDTLSKey)
			return nil
		}},
		{name: "Updating LND config", fn: func() error {
			return writeLNDConfig(cfg, publicIPv4)
		}},
		{name: "Updating firewall", fn: func() error {
			return configureFirewall(cfg)
		}},
		{name: "Restarting LND", fn: func() error {
			return system.SudoRun("systemctl", "restart", "lnd")
		}},
	}

	if cfg.LndHubInstalled {
		steps = append(steps,
			installStep{
				name: "Generating TLS certificate for LndHub proxy",
				fn: func() error {
					return generateProxyCert(publicIPv4)
				}},
			installStep{
				name: "Creating LndHub proxy service",
				fn:   writeLndHubProxyService},
			installStep{
				name: "Starting LndHub TLS proxy",
				fn:   startLndHubProxy},
		)
	}

	if err := RunInstallTUI(steps, appVersion); err != nil {
		cfg.P2PMode = "tor"
		return err
	}
	return config.Save(cfg)
}

// ── Syncthing installation ───────────────────────────────

func RunSyncthingInstall(cfg *config.AppConfig) error {
	confirmMsg := theme.Header.Render("Install Syncthing") + "\n\n" +
		theme.Value.Render("This will:") + "\n\n" +
		theme.Value.Render("  * Install Syncthing from official repository") + "\n" +
		theme.Value.Render("  * Open port 22000 for sync connections") + "\n" +
		theme.Value.Render("  * Create Tor hidden service for web UI") + "\n" +
		theme.Value.Render("  * Auto-configure LND channel backup sync") + "\n" +
		theme.Value.Render("  * Restart Tor") + "\n\n" +
		theme.Value.Render("After install, pair your local Syncthing") + "\n" +
		theme.Value.Render("from the Syncthing details screen.") + "\n\n" +
		theme.Dim.Render("Enter to proceed -- backspace to cancel")
	if !ShowConfirmBox(confirmMsg) {
		return nil
	}

	passBytes := make([]byte, 12)
	if _, err := randRead(passBytes); err != nil {
		return fmt.Errorf("generate password: %w", err)
	}
	syncPassword := hexEncode(passBytes)

	cfg.SyncthingInstalled = true

	steps := []installStep{
		{name: "Adding Syncthing repository",
			fn: installSyncthingRepo},
		{name: "Installing Syncthing",
			fn: installSyncthingPackage},
		{name: "Creating Syncthing directories",
			fn: createSyncthingDirs},
		{name: "Creating Syncthing service",
			fn: writeSyncthingService},
		{name: "Configuring Syncthing authentication",
			fn: func() error {
				return configureSyncthingAuth(syncPassword)
			}},
		{name: "Configuring firewall",
			fn: func() error { return configureFirewall(cfg) }},
		{name: "Rebuilding Tor config",
			fn: func() error { return RebuildTorConfig(cfg) }},
		{name: "Restarting Tor", fn: restartTor},
		{name: "Starting Syncthing", fn: startSyncthing},
		{name: "Registering backup folder",
			fn: registerBackupFolder},
		{name: "Setting up channel backup watcher",
			fn: func() error {
				return setupChannelBackupWatcher(cfg)
			}},
	}
	if err := RunInstallTUI(steps, appVersion); err != nil {
		cfg.SyncthingInstalled = false
		RebuildTorConfig(cfg)
		restartTor()
		return err
	}
	cfg.SyncthingPassword = syncPassword
	return config.Save(cfg)
}

// ── LndHub installation ──────────────────────────────────

func RunLndHubInstall(cfg *config.AppConfig) error {
	confirmMsg := theme.Header.Render("Install LndHub.go") + "\n\n" +
		theme.Value.Render("This will:") + "\n\n" +
		theme.Value.Render("  * Install Go toolchain (for building from source)") + "\n" +
		theme.Value.Render("  * Install PostgreSQL database") + "\n" +
		theme.Value.Render("  * Clone and build LndHub.go v"+lndhubVersion) + "\n" +
		theme.Value.Render("  * Bake restricted LND macaroon") + "\n" +
		theme.Value.Render("  * Create Tor hidden service") + "\n" +
		theme.Value.Render("  * Create accounts for family/friends from TUI") + "\n\n" +
		theme.Dim.Render("Enter to proceed -- backspace to cancel")
	if !ShowConfirmBox(confirmMsg) {
		return nil
	}

	dbPassBytes := make([]byte, 16)
	if _, err := randRead(dbPassBytes); err != nil {
		return fmt.Errorf("generate db password: %w", err)
	}
	dbPassword := hexEncode(dbPassBytes)

	jwtBytes := make([]byte, 32)
	if _, err := randRead(jwtBytes); err != nil {
		return fmt.Errorf("generate jwt secret: %w", err)
	}
	jwtSecret := hexEncode(jwtBytes)

	adminBytes := make([]byte, 24)
	if _, err := randRead(adminBytes); err != nil {
		return fmt.Errorf("generate admin token: %w", err)
	}
	adminToken := hexEncode(adminBytes)

	cfg.LndHubInstalled = true

	publicIPv4 := ""
	if cfg.P2PMode == "hybrid" {
		publicIPv4 = system.PublicIPv4()
	}

	steps := []installStep{
		{name: "Installing Go toolchain",
			fn: installGoToolchain},
		{name: "Installing PostgreSQL",
			fn: installPostgreSQL},
		{name: "Creating database",
			fn: func() error {
				return createLndHubDatabase(dbPassword)
			}},
		{name: "Cloning lndhub.go v" + lndhubVersion,
			fn: cloneLndHub},
		{name: "Building lndhub (from source)",
			fn: buildLndHub},
		{name: "Installing binary",
			fn: installLndHubBinary},
		{name: "Baking LND macaroon",
			fn: func() error { return bakeLndHubMacaroon(cfg) }},
		{name: "Creating directories",
			fn: createLndHubDirs},
		{name: "Writing configuration", fn: func() error {
			return writeLndHubConfig(
				cfg, dbPassword, jwtSecret, adminToken)
		}},
		{name: "Creating service", fn: writeLndHubService},
		{name: "Configuring firewall",
			fn: func() error { return configureFirewall(cfg) }},
		{name: "Rebuilding Tor config",
			fn: func() error { return RebuildTorConfig(cfg) }},
		{name: "Restarting Tor", fn: restartTor},
		{name: "Starting LndHub", fn: startLndHub},
	}

	if cfg.P2PMode == "hybrid" && publicIPv4 != "" {
		steps = append(steps,
			installStep{
				name: "Generating TLS certificate for LndHub proxy",
				fn: func() error {
					return generateProxyCert(publicIPv4)
				}},
			installStep{
				name: "Creating LndHub proxy service",
				fn:   writeLndHubProxyService},
			installStep{
				name: "Starting LndHub TLS proxy",
				fn:   startLndHubProxy},
		)
	}

	if err := RunInstallTUI(steps, appVersion); err != nil {
		cfg.LndHubInstalled = false
		RebuildTorConfig(cfg)
		restartTor()
		return err
	}

	cfg.LndHubAdminToken = adminToken
	cfg.LndHubDBPassword = dbPassword
	return config.Save(cfg)
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

func RunSelfUpdate(cfg *config.AppConfig, newVersion string) error {
	confirmMsg := theme.Header.Render("Update Virtual Private Node") + "\n\n" +
		theme.Value.Render("Current: v"+appVersion) + "\n" +
		theme.Value.Render("Latest:  v"+newVersion) + "\n\n" +
		theme.Value.Render("This will download and verify the new binary.") + "\n" +
		theme.Value.Render("The update takes effect on next SSH login.") + "\n\n" +
		theme.Dim.Render("Enter to proceed -- backspace to cancel")
	if !ShowConfirmBox(confirmMsg) {
		return nil
	}

	baseURL := fmt.Sprintf(
		"https://github.com/ripsline/virtual-private-node/releases/download/v%s",
		newVersion)
	expectedFP := "AFA0EBACDC9A4C4AA7B0154AC97CE10F170BA5FE"
	tarball := fmt.Sprintf("rlvpn-%s-amd64.tar.gz", newVersion)

	steps := []installStep{
		{name: "Downloading v" + newVersion, fn: func() error {
			return system.DownloadRequireTor(
				baseURL+"/"+tarball, "/tmp/"+tarball)
		}},
		{name: "Downloading checksums", fn: func() error {
			if err := system.DownloadRequireTor(
				baseURL+"/SHA256SUMS",
				"/tmp/rlvpn-SHA256SUMS"); err != nil {
				return err
			}
			return system.DownloadRequireTor(
				baseURL+"/SHA256SUMS.asc",
				"/tmp/rlvpn-SHA256SUMS.asc")
		}},
		{name: "Importing release key", fn: func() error {
			keyFile := "/tmp/rlvpn-release-key.asc"
			keyURL := fmt.Sprintf(
				"https://keys.openpgp.org/vks/v1/by-fingerprint/%s",
				expectedFP)
			if err := system.DownloadRequireTor(keyURL, keyFile); err != nil {
				return fmt.Errorf(
					"could not download signing key: %w", err)
			}
			defer os.Remove(keyFile)
			if _, err := system.RunCombinedOutput("gpg",
				"--batch", "--import", keyFile); err != nil {
				return fmt.Errorf(
					"could not import signing key: %w", err)
			}
			output, err := system.RunCombinedOutput("gpg",
				"--batch", "--with-colons",
				"--list-keys", expectedFP)
			if err != nil || !strings.Contains(output, expectedFP) {
				return fmt.Errorf("release key fingerprint mismatch")
			}
			return nil
		}},
		{name: "Verifying signature", fn: func() error {
			output, err := system.RunCombinedOutput("gpg",
				"--batch", "--verify",
				"/tmp/rlvpn-SHA256SUMS.asc",
				"/tmp/rlvpn-SHA256SUMS")
			if err != nil {
				return fmt.Errorf(
					"signature verification failed: %s", output)
			}
			return nil
		}},
		{name: "Verifying checksum", fn: func() error {
			cmd := exec.Command("sha256sum", "--ignore-missing",
				"--check", "rlvpn-SHA256SUMS")
			cmd.Dir = "/tmp"
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("checksum failed: %s",
					string(output))
			}
			return nil
		}},
		{name: "Installing new binary", fn: func() error {
			if err := system.Run("tar", "-xzf",
				"/tmp/"+tarball, "-C", "/tmp"); err != nil {
				return err
			}
			if err := system.SudoRun("install", "-m", "755",
				"/tmp/rlvpn", "/usr/local/bin/rlvpn"); err != nil {
				return err
			}
			os.Remove("/tmp/" + tarball)
			os.Remove("/tmp/rlvpn-SHA256SUMS")
			os.Remove("/tmp/rlvpn-SHA256SUMS.asc")
			os.Remove("/tmp/rlvpn")
			return nil
		}},
	}

	return RunInstallTUI(steps, appVersion)
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
	data, err := os.ReadFile(bashrc)
	if err == nil && strings.Contains(string(data), "bitcoin-cli()") {
		return nil
	}

	net := cfg.NetworkConfig()
	btcNetFlag := ""
	if net.Name == "testnet4" {
		btcNetFlag = "\n        -testnet4 \\"
	}

	content := fmt.Sprintf(`
# -- Virtual Private Node --
bitcoin-cli() {
    sudo -u bitcoin /usr/local/bin/bitcoin-cli \
        -datadir=/var/lib/bitcoin \
        -conf=/etc/bitcoin/bitcoin.conf \%s
        "$@"
}
export -f bitcoin-cli
`, btcNetFlag)

	f, err := os.OpenFile(bashrc,
		os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func AppendLNCLIToShell(cfg *config.AppConfig) error {
	bashrc := paths.AdminBashrc
	data, err := os.ReadFile(bashrc)
	if err == nil && strings.Contains(string(data), "lncli()") {
		return nil
	}

	net := cfg.NetworkConfig()
	lndNetFlag := ""
	if net.Name != "mainnet" {
		lndNetFlag = fmt.Sprintf(
			"\n        --network=%s \\", net.LNCLINetwork)
	}
	content := fmt.Sprintf(`
lncli() {
    sudo -u bitcoin /usr/local/bin/lncli \
        --lnddir=/var/lib/lnd \%s
        --macaroonpath=/var/lib/lnd/data/chain/bitcoin/%s/admin.macaroon \
        --tlscertpath=/var/lib/lnd/tls.cert \
        "$@"
}
export -f lncli
`, lndNetFlag, net.LNCLINetwork)

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
