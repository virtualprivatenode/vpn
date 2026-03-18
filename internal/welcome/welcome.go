// internal/welcome/welcome.go

package welcome

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/logger"
)

type wTab int

const (
	tabDashboard wTab = iota
	tabLightning
	tabPairing
	tabAddons
	tabSettings
)

type wSubview int

const (
	svNone wSubview = iota
	svWalletInfo
	svZeus
	svSyncthingDetail
	svSyncthingPairInput
	svSyncthingDeviceDetail
	svSyncthingWebUI
	svSyncthingDeviceQR
	svChannelDetail
	svChannelOpen
	svChannelAmountSelect
	svChannelCustomPeer
	svChannelOpenConfirm
	svChannelOpening
	svChannelOpenResult
	svChannelFundWallet
	svLndHubManage
	svLndHubCreateName
	svLndHubCreateAccount
	svLndHubAccountDetail
	svLndHubDeactivateConfirm
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

type channelOpenResultMsg struct {
	txid string
	err  error
}

type newAddressMsg struct {
	address string
	err     error
}

type channelInfo struct {
	ChanID        uint64
	PeerAlias     string
	RemotePubkey  string
	Capacity      int64
	LocalBalance  int64
	RemoteBalance int64
	Active        bool
	Private       bool
	Initiator     bool
}

type peerOption struct {
	Pubkey      string
	Host        string
	Alias       string
	TorOnly     bool
	Curated     bool
	MinChanSize int64
	Note        string
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
	channels                     []channelInfo
	pendingOpen                  int
	pendingForceClose            int
}

type Model struct {
	cfg                  *config.AppConfig
	cfgStore             *config.Store
	lndClient            *lndrpc.Client
	version              string
	activeTab            wTab
	subview              wSubview
	dashCard             cardPos
	cardActive           bool
	lightningFocus       int // 0=channels, 1=wallet
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
	chanCursor           int
	chanScrollOffset     int
	chanOpenPeerIdx      int
	chanOpenAmount       int64
	chanOpenPrivate      bool
	chanOpenPubkey       string
	chanOpenHost         string
	chanOpenAlias        string
	chanOpenInFlight     bool
	chanOpenTxid         string
	chanOpenError        string
	chanCustomPubkey     string
	chanCustomHost       string
	chanCustomInputField int
	chanCustomAmountStr  string
	chanFundAddress      string
	chanPeerList         []peerOption
	chanAmountPreset     int
}

func NewModel(cfg *config.AppConfig, version string) Model {
	var client *lndrpc.Client
	if cfg.HasLND() && cfg.WalletExists() {
		client = lndrpc.New(cfg.Network)
	}
	return Model{
		cfg: cfg, lndClient: client, version: version,
		activeTab: tabDashboard, subview: svNone,
		dashCard: cardServices, fetchInFlight: true,
	}
}

func NewTestModel(cfg *config.AppConfig, version string, store *config.Store) Model {
	m := Model{
		cfg: cfg, version: version,
		activeTab: tabDashboard, subview: svNone,
		dashCard: cardServices, fetchInFlight: true,
	}
	m.cfgStore = store
	return m
}

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

func (m Model) svcCount() int {
	return len(serviceNames(m.cfg))
}

func (m Model) svcName(i int) string {
	names := serviceNames(m.cfg)
	if i < len(names) {
		return names[i]
	}
	return ""
}

// pollInterval returns the status polling interval based on current state.
// Faster polling during startup, slower once everything is stable.
func (m Model) pollInterval() time.Duration {
	if m.status == nil {
		return 3 * time.Second
	}
	if !m.status.lndResponding && m.cfg.HasLND() && m.cfg.WalletExists() {
		return 5 * time.Second
	}
	if !m.status.btcSynced {
		return 15 * time.Second
	}
	return 60 * time.Second
}

func Show(cfg *config.AppConfig, version string) {
	for {
		m := NewModel(cfg, version)
		p := tea.NewProgram(m, tea.WithAltScreen())
		result, _ := p.Run()
		final := result.(Model)

		if final.lndClient != nil {
			final.lndClient.Close()
		}

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
				logger.TUI("Warning: failed to add lncli wrapper: %v", err)
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
		fetchStatus(m.cfg, m.lndClient),
		fetchLatestVersion(),
		tickEvery(m.pollInterval()),
	)
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchLatestVersion() tea.Cmd {
	return func() tea.Msg {
		return latestVersionMsg(installer.CheckLatestVersion())
	}
}

func createLndHubAccountCmd(adminToken string) tea.Cmd {
	return func() tea.Msg {
		account, err := installer.CreateLndHubAccount(adminToken)
		return lndhubAccountCreatedMsg{account: account, err: err}
	}
}

func deactivateLndHubAccountCmd(login string) tea.Cmd {
	return func() tea.Msg {
		balance, _ := installer.GetUserBalance(login)
		err := installer.DeactivateUser(login)
		return lndhubDeactivatedMsg{balance: balance, err: err}
	}
}

func pairSyncthingDeviceCmd(deviceID string) tea.Cmd {
	return func() tea.Msg {
		err := installer.PairSyncthingDevice(deviceID)
		return syncthingPairedMsg{err: err}
	}
}

func openChannelCmd(
	client *lndrpc.Client, pubkey, host string,
	amount int64, private bool,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return channelOpenResultMsg{err: fmt.Errorf("LND not connected")}
		}
		if host != "" {
			if err := client.ConnectPeer(pubkey, host); err != nil {
				logger.TUI("Peer connect warning: %v", err)
			}
		}
		if err := client.WaitForPeer(pubkey, 60*time.Second); err != nil {
			return channelOpenResultMsg{
				err: fmt.Errorf("could not connect to peer: %v", err)}
		}
		result, err := client.OpenChannel(pubkey, amount, private)
		if err != nil {
			return channelOpenResultMsg{err: err}
		}
		return channelOpenResultMsg{txid: result.FundingTxID}
	}
}

func getNewAddressCmd(client *lndrpc.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return newAddressMsg{err: fmt.Errorf("LND not connected")}
		}
		addr, err := client.GetNewAddress()
		if err != nil {
			return newAddressMsg{err: err}
		}
		return newAddressMsg{address: addr.Address}
	}
}
