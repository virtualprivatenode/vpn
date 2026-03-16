// internal/welcome/welcome.go

package welcome

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/logger"
)

// ── Enums ────────────────────────────────────────────────

type wTab int

const (
	tabDashboard wTab = iota
	tabPairing
	tabAddons
	tabSettings
)

type wSubview int

const (
	svNone wSubview = iota
	svLightning
	svZeus
	svSyncthingDetail
	svSyncthingPairInput
	svSyncthingDeviceDetail
	svSyncthingWebUI
	svLndHubManage
	svLndHubCreateName
	svLndHubCreateAccount
	svLndHubAccountDetail
	svLndHubDeactivateConfirm
	svSyncthingDeviceQR
	svQR
	svFullURL
	svWalletCreate
	svLNDInstall
	svSyncthingInstall
	svLndHubInstall
	svSelfUpdate
	svP2PUpgrade
)

type cardPos int

const (
	cardServices cardPos = iota
	cardSystem
	cardBitcoin
	cardLightning
)

// ── Messages ─────────────────────────────────────────────

type svcActionDoneMsg struct{}
type tickMsg time.Time
type latestVersionMsg string

type lndhubAccountCreatedMsg struct {
	account *installer.LndHubAccount
	err     error
}

type lndhubDeactivatedMsg struct {
	balance string
	err     error
}

type syncthingPairedMsg struct {
	err error
}

type statusMsg struct {
	services                     map[string]bool
	walletDetected               bool
	diskTotal, diskUsed, diskPct string
	ramTotal, ramUsed, ramPct    string
	btcSize, lndSize             string
	btcBlocks, btcHeaders        int
	btcProgress                  float64
	btcSynced, btcResponding     bool
	rebootRequired               bool
	lndPubkey                    string
	lndChannels                  int
	lndBalance                   string
	lndSyncedChain               bool
	lndSyncedGraph               bool
	lndResponding                bool
	publicIP                     string
}

// ── Model ────────────────────────────────────────────────

type Model struct {
	cfg                  *config.AppConfig
	cfgStore             *config.Store
	version              string
	activeTab            wTab
	subview              wSubview
	dashCard             cardPos
	cardActive           bool
	svcCursor            int
	svcConfirm           string
	sysConfirm           string
	addonFocus           int
	urlTarget            string
	qrMode               string
	qrLabel              string
	urlReturnTo          wSubview
	width                int
	height               int
	shellAction          wSubview
	status               *statusMsg
	settingsFocus        int
	latestVersion        string
	updateConfirm        bool
	fetchInFlight        bool
	lastAccount          *installer.LndHubAccount
	hubCursor            int
	hubNameInput         string
	hubDeactivateBalance string
	syncDeviceInput      string
	syncDeviceLabel      string
	syncPairError        string
	syncPairSuccess      bool
	syncCursor           int
	showSecrets          bool
}

func NewModel(cfg *config.AppConfig, version string) Model {
	return Model{
		cfg: cfg, version: version,
		activeTab: tabDashboard, subview: svNone,
		dashCard:      cardServices,
		fetchInFlight: true,
	}
}

func NewTestModel(
	cfg *config.AppConfig, version string, store *config.Store,
) Model {
	m := NewModel(cfg, version)
	m.cfgStore = store
	return m
}

// serviceNames returns the list of managed service names based on config state.
// Used by dashboard, status polling, and service management.
func serviceNames(cfg *config.AppConfig) []string {
	names := []string{"tor", "bitcoind"}
	if cfg.HasLND() {
		names = append(names, "lnd")
	}
	if cfg.SyncthingInstalled {
		names = append(names, "syncthing")
	}
	if cfg.LndHubInstalled {
		names = append(names, "lndhub")
	}
	if cfg.LndHubInstalled && cfg.P2PMode == "hybrid" {
		names = append(names, "lndhub-proxy")
	}
	return names
}

func (m Model) saveCfg() {
	if err := config.SaveTo(m.cfgStore, m.cfg); err != nil {
		logger.TUI("ERROR: failed to save config: %v", err)
	}
}

func Show(cfg *config.AppConfig, version string) {
	for {
		m := NewModel(cfg, version)
		p := tea.NewProgram(m, tea.WithAltScreen())
		result, _ := p.Run()
		final := result.(Model)

		switch final.shellAction {
		case svLndHubInstall:
			installer.RunLndHubInstall(cfg)
			if u, e := config.Load(); e == nil {
				cfg = u
			}
			continue
		case svWalletCreate:
			installer.RunWalletCreation(cfg)
			if u, e := config.Load(); e == nil {
				cfg = u
			}
			continue
		case svLNDInstall:
			installer.RunLNDInstall(cfg)
			if u, e := config.Load(); e == nil {
				cfg = u
			}
			if err := installer.AppendLNCLIToShell(cfg); err != nil {
				logger.TUI(
					"Warning: failed to add lncli wrapper: %v",
					err)
			}
			continue
		case svSyncthingInstall:
			installer.RunSyncthingInstall(cfg)
			if u, e := config.Load(); e == nil {
				cfg = u
			}
			continue
		case svSelfUpdate:
			installer.RunSelfUpdate(cfg, final.latestVersion)
			continue
		case svP2PUpgrade:
			installer.RunP2PModeUpgrade(cfg)
			if u, e := config.Load(); e == nil {
				cfg = u
			}
			continue
		default:
			return
		}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchStatus(m.cfg),
		fetchLatestVersion(),
		tickEvery(5*time.Second),
	)
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchLatestVersion() tea.Cmd {
	return func() tea.Msg {
		v := installer.CheckLatestVersion()
		return latestVersionMsg(v)
	}
}

func createLndHubAccountCmd(adminToken string) tea.Cmd {
	return func() tea.Msg {
		account, err := installer.CreateLndHubAccount(adminToken)
		return lndhubAccountCreatedMsg{
			account: account, err: err}
	}
}

func deactivateLndHubAccountCmd(login string) tea.Cmd {
	return func() tea.Msg {
		balance, _ := installer.GetUserBalance(login)
		err := installer.DeactivateUser(login)
		return lndhubDeactivatedMsg{
			balance: balance, err: err}
	}
}

func pairSyncthingDeviceCmd(deviceID string) tea.Cmd {
	return func() tea.Msg {
		err := installer.PairSyncthingDevice(deviceID)
		return syncthingPairedMsg{err: err}
	}
}
