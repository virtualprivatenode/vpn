package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/installer"
	"github.com/virtualprivatenode/vpn/internal/lndrpc"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/syncthing"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

type wSubview int

// ── TUI layout constants ────────────────────────────────
// Change these to resize the entire TUI frame. All widths
// and heights derive from these values.
const (
	tuiWidth  = 82
	tuiHeight = 34
)

const (
	svNone wSubview = iota
	svQR
	svFullURL
)

// Tab types for the top tab bar
type tabKind int

const (
	tabMain              tabKind = iota // Main view for current section
	tabChannel                          // Channel detail
	tabPayment                          // Payment detail
	tabSend                             // ⚡ Send payment flow
	tabReceive                          // ⚡ Receive payment flow
	tabPairing                          // Pairing screen
	tabOnChain                          // ⛓ On-chain send flow
	tabOCReceive                        // ⛓ On-chain receive flow
	tabSyncthing                        //
	tabSyncthingDevice                  // Syncthing device detail
	tabSyncthingWebUI                   // Syncthing Web UI
	tabSyncthingPair                    // Syncthing pair device flow
	tabOpenChannel                      // Channel open flow
	tabOnChainTx                        // on-chain transaction detail
	tabUtxoDetail                       // UTXO detail with label edit
	tabChannelHistory                   // channel history view
	tabSyncthingInstall                 // Syncthing install flow
	tabP2PUpgrade                       // P2P mode upgrade flow
	tabSelfUpdate                       // Self-update flow
	tabAutoUnlock                       // Auto-unlock configuration flow
	tabWalletCreate                     // Wallet creation flow
	tabNodeInfo                         // Receive channel / node info screen
	tabSSHKeys                          // SSH key management
	tabSSHKeyDetail                     // SSH key detail (per-key)
	tabSSHKeyAdd                        // SSH key add flow
	tabSSHPasswordAuth                  // SSH password auth toggle
	tabSSHChangePassword                // change login password
)

type openTab struct {
	Kind  tabKind
	Label string
	Index int    // row identity for other detail tabs
	Key   string // funding outpoint for channel detail tabs
	// Section remains the tab's owner across wallet-to-auto-unlock transitions.
	Section int
	// Parent declares which tab kind owns this tab.
	// Zero means "section home is the parent" (top-
	// level detail tabs opened from home screens).
	// Non-zero means this tab is a child of another
	// detail tab (e.g. tabSyncthingDevice's Parent is
	// tabSyncthing). Used by closeTab for cascade-
	// close and by focusParentMsg for backspace
	// navigation. No grandchild tabs exist — depth
	// is at most two levels.
	Parent tabKind
	Screen Screen // L16: owns all state for this tab's content (nil = legacy path)
}

type feeTier struct {
	Target   int     // block target: 1, 3, 6, 25
	SatPerVB float64 // fee rate in sat/vB
	Label    string  // "~1 blk", "~3 blk", etc.
}

type svcActionDoneMsg struct{}
type pkgUpdateDoneMsg struct{}
type tickMsg time.Time
type latestVersionMsg string

// tabActivatedMsg is delivered to a screen's HandleMsg
// when the user navigates to (or lands on) the screen's
// tab. Screens opt in by handling it — those that don't
// care silently ignore it via the default fall-through.
// Used to refresh stale data without replacing the screen
// or its in-progress state.
type tabActivatedMsg struct{}

// nodeAddressesMsg carries the live-read display facts (onion
// hostnames, the Syncthing device ID) fetched at screen entry
// through the helper's read-node-addresses operation. tab
// names the screen that asked, so Update can route the answer
// back to it. No screen caches these beyond its own lifetime —
// the facts have no board copy, and re-entering a screen asks
// again.
type nodeAddressesMsg struct {
	tab   tabKind
	addrs helper.NodeAddressesResult
	err   error
}

type walletStateMsg struct {
	owner    *ScreenContext
	revision uint64
	state    helper.WalletStateResult
	err      error
}

type keyVerificationStateMsg struct {
	state helper.KeyVerificationStateResult
	err   error
}

type syncthingPairedMsg struct {
	owner   *SyncthingPairScreen
	attempt uint64
	result  app.SyncthingResult
}
type syncthingRemovedMsg struct {
	owner   *SyncthingDeviceScreen
	attempt uint64
	result  app.SyncthingResult
}
type syncthingDevicesMsg struct {
	owner    *ScreenContext
	revision uint64
	devices  []syncthing.Device
	err      error
}
type syncthingCloseMsg struct {
	owner   Screen
	attempt uint64
}
type channelOpenResultMsg struct {
	attempt *channelOpenAttempt
	result  app.ChannelOpenResult
}
type newAddressMsg struct {
	owner   *OCReceiveScreen
	attempt uint64
	address app.OnChainReceiveAddress
	err     error
}
type receiveAddressQRMsg struct {
	owner   *OCReceiveScreen
	attempt uint64
}
type invoiceCreatedMsg struct {
	attempt *invoiceAttempt
	invoice app.LightningInvoice
	err     error
}
type invoiceCheckMsg struct {
	attempt *invoiceAttempt
}
type invoiceStatusMsg struct {
	attempt *invoiceAttempt
	state   app.InvoiceState
	err     error
}
type payReqDecodedMsg struct {
	attempt *paymentAttempt
	payment app.PreparedPayment
	err     error
}
type sendPaymentResultMsg struct {
	attempt *paymentAttempt
	result  *lndrpc.SendPaymentResult
	err     error
}
type paymentHistoryMsg struct {
	entries []lndrpc.PaymentEntry
	err     error
}

type utxoListMsg struct {
	utxos []lndrpc.UTXO
	err   error
}

type onChainSendAttempt struct{ prepared app.PreparedOnChainSend }

type sendCoinsResultMsg struct {
	attempt *onChainSendAttempt
	result  app.OnChainSendResult
}

type feeTiersMsg struct {
	tiers [4]feeTier
	err   error
}

type onChainTxMsg struct {
	owner    *OnChainContext
	revision uint64
	txs      []lndrpc.OnChainTx
	err      error
}

type channelCloseResultMsg struct {
	attempt *channelCloseAttempt
	result  app.ChannelCloseResult
}

type closedChannelsMsg struct {
	channels []lndrpc.ClosedChannel
	err      error
}

type labelTxMsg struct {
	owner   *OnChainHomeScreen
	attempt uint64
	result  app.TransactionLabelResult
}

type channelInfo struct {
	ChanID         uint64
	ChannelPoint   string
	PeerAlias      string
	RemotePubkey   string
	Capacity       int64
	LocalBalance   int64
	RemoteBalance  int64
	Active         bool
	Private        bool
	Initiator      bool
	Pending        bool
	CommitmentType string
}

type channelHistoryEntry struct {
	PeerAlias       string
	RemotePubkey    string
	Capacity        int64
	LocalBalance    int64
	Status          string // "active", "inactive", "pending open", etc.
	CloseType       string // "coop", "force", "breach", "—"
	ClosingTxid     string
	SettledBal      int64
	CloseHeight     int32
	BlocksRemaining int32
	LimboBalance    int64
	Active          bool
}

type peerOption struct {
	Pubkey      string
	Host        string
	Alias       string
	TorOnly     bool
	Taproot     bool
	MinChanSize int64
}

type statusMsg struct {
	services                     map[string]bool
	diskTotal, diskUsed, diskPct string
	ramTotal, ramUsed, ramPct    string
	btcSize, lndSize             string
	btcBlocks, btcHeaders        int
	btcProgress                  float64
	btcSynced, btcResponding     bool
	rebootRequired               bool
	lndPubkey                    string
	lndAlias                     string
	lndURIs                      []string
	lndVersion                   string
	lndPeers                     int
	lndChannels                  int
	lndBalance                   string
	lndSyncedChain               bool
	lndSyncedGraph               bool
	lndResponding                bool
	lndWalletState               lndrpc.WalletState
	publicIP                     string
	channels                     []channelInfo
	pendingOpen                  int
	pendingForceClose            int
	pendingForceCloseChannels    []lndrpc.PendingForceCloseChannel
	waitingCloseChannels         []lndrpc.WaitingCloseChannel
}

type Model struct {
	cfg       *config.AppConfig
	prefs     *config.Preferences
	state     *RuntimeState
	lndClient *lndrpc.Client
	version   string
	subview   wSubview
	width     int
	height    int

	// L16: shared context for screen components
	screenCtx *ScreenContext
	ocCtx     *OnChainContext

	// L16: section home screens (nil = legacy path)
	sectionScreens [numSections]Screen

	status        *statusMsg
	latestVersion string
	fetchInFlight bool

	// QR fullscreen (Model-owned overlay)
	urlTarget string
	qrLabel   string

	// Navigation
	nav            NavSidebar
	contentFocused bool

	// Tab bar
	tabs            []openTab
	activeTab       int
	tabFocused      bool
	tabCursorX      int
	tabScrollOffset int

	// Per-section tab memory. sectionFocus[s] holds
	// the user's last activeTab index within section
	// s, so that returning to s and pressing up from
	// the sidebar restores their previous position
	// instead of jumping to the leftmost detail tab.
	// Zero means "no memory yet, fall back to tab 1".
	//
	sectionFocus [numSections]int
}

func NewModel(
	cfg *config.AppConfig, prefs *config.Preferences,
	state *RuntimeState, version string,
) Model {
	theme.Init(prefs.Theme != "light")
	// Creation owns credential staging before client publication. Startup can
	// initialize an existing wallet independently of that interactive workflow.
	var client *lndrpc.Client
	if cfg.HasLND() && state.WalletKnown && state.WalletExists {
		client = lndrpc.New()
	}
	m := Model{
		cfg: cfg, prefs: prefs, state: state,
		lndClient: client, version: version,
		subview: svNone, fetchInFlight: true,
		nav: NewNavSidebar(),
	}
	m.screenCtx = &ScreenContext{
		Syncthing:       app.NewSyncthing(),
		HelperWorkflows: app.NewHelperWorkflows(installer.SyncthingVersionStr()),
		Cfg:             cfg,
		State:           state,
		LndClient:       client,
		Version:         version,
	}
	m.ocCtx = &OnChainContext{}
	m.sectionScreens[secChannels] =
		NewChannelsHomeScreen(m.screenCtx)
	m.sectionScreens[secWallet] =
		NewWalletHomeScreen(m.screenCtx)
	m.sectionScreens[secOnChain] =
		NewOnChainHomeScreen(m.screenCtx, m.ocCtx)
	m.sectionScreens[secAddons] =
		NewAddonsHomeScreen(m.screenCtx)
	m.sectionScreens[secSystem] =
		NewSystemHomeScreen(m.screenCtx)
	return m
}

func serviceNames(cfg *config.AppConfig) []string {
	names := []string{"tor", "bitcoind"}
	if cfg.HasLND() {
		names = append(names, "lnd")
	}
	if cfg.SyncthingEnabled {
		names = append(names, "syncthing")
	}
	return names
}

func (m Model) savePreferences() {
	if err := config.SavePreferences(m.prefs); err != nil {
		logger.TUI(
			"ERROR: failed to save TUI preferences: %v", err)
	}
}

func (m Model) pollInterval() time.Duration {
	if m.status == nil {
		return 3 * time.Second
	}
	if !m.state.WalletKnown || !m.state.KeyVerificationKnown {
		return 5 * time.Second
	}
	if !m.status.lndResponding && m.cfg.HasLND() &&
		m.state.WalletKnown && m.state.WalletExists {
		return 5 * time.Second
	}
	return 60 * time.Second
}

func Show(
	cfg *config.AppConfig, prefs *config.Preferences, version string,
) {
	state := observeRuntimeState(cfg)
	m := NewModel(cfg, prefs, state, version)
	// Bubble Tea does not cancel or join commands on exit. The workflow owner
	// releases helper readers even when Run fails or provides no final model.
	defer func() {
		if m.screenCtx.AutoUnlock != nil {
			m.screenCtx.AutoUnlock.Close()
		}
		if m.screenCtx.LoginPasswords != nil {
			m.screenCtx.LoginPasswords.Close()
		}
		if m.screenCtx.WalletCreation != nil {
			m.screenCtx.WalletCreation.Close()
		}
		m.screenCtx.HelperWorkflows.Close()
		m.screenCtx.Syncthing.Close()
		if m.screenCtx.LndClient != nil {
			m.screenCtx.LndClient.Close()
		}
	}()
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		logger.TUI("terminal session: %v", err)
	}
}

func observeRuntimeState(cfg *config.AppConfig) *RuntimeState {
	state := &RuntimeState{}
	var wallet helper.WalletStateResult
	if err := helper.Call(
		helper.VerbReadWalletState, nil, &wallet); err != nil {
		logger.TUI("read live wallet state: %v", err)
	} else {
		state.WalletExists = wallet.WalletExists
		state.WalletKnown = true
	}
	var verification helper.KeyVerificationStateResult
	if err := helper.Call(
		helper.VerbReadKeyVerificationState,
		nil, &verification); err != nil {
		logger.TUI("read key-verification state: %v", err)
	} else {
		state.KeyVerificationPending = verification.Pending
		state.KeyVerificationKnown = true
	}
	if enabled, err := app.NewSSHAccess().PasswordAuth(); err != nil {
		logger.TUI("read live SSH password authentication: %v", err)
	} else {
		state.SSHPasswordAuthDisabled = !enabled
		state.SSHPasswordAuthKnown = true
	}
	if cfg.SyncthingEnabled {
		runtime := app.NewSyncthing()
		state.SyncthingDevices, state.SyncthingDevicesErr = runtime.ListDevices()
		runtime.Close()
		state.SyncthingDevicesKnown = state.SyncthingDevicesErr == nil
	}
	return state
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchStatus(m.cfg, m.state, m.lndClient),
		fetchWalletStateCmd(m.screenCtx),
		fetchKeyVerificationStateCmd(),
		fetchLatestVersionCmd(),
		tickEveryCmd(m.pollInterval()))
}
