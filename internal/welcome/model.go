package welcome

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/theme"
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
	Index int // channel index, payment index, etc.
	// Section is the sticky owner of this tab — set
	// at construction from m.nav.ActiveSection() and
	// must never be mutated afterward. effectiveTabs,
	// closeTab's cascade guard, and the sectionFocus
	// restore logic all depend on it remaining stable.
	// The only in-place tab transformation in the
	// codebase (walletCreatedMsg's wallet-create →
	// auto-unlock swap in update.go) explicitly
	// preserves this field for that reason.
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

type syncthingPairedMsg struct {
	deviceID string
	err      error
}
type syncthingRemovedMsg struct {
	deviceID string
	err      error
}
type channelOpenResultMsg struct {
	txid string
	err  error
}
type newAddressMsg struct {
	address string
	err     error
}
type invoiceCreatedMsg struct {
	payReq      string
	paymentHash string
	amountSats  int64
	err         error
}
type invoiceSettledMsg struct {
	settled bool
	expired bool
	err     error
}
type payReqDecodedMsg struct {
	decoded *lndrpc.DecodedPayReq
	err     error
}
type sendPaymentResultMsg struct {
	result *lndrpc.SendPaymentResult
	err    error
}
type paymentHistoryMsg struct {
	entries []lndrpc.PaymentEntry
	err     error
}

type utxoListMsg struct {
	utxos []lndrpc.UTXO
	err   error
}

type sendCoinsResultMsg struct {
	txid string
	err  error
}

type feeTiersMsg struct {
	tiers [4]feeTier
	err   error
}

type feeEstimateMsg struct {
	feeSats int64
	err     error
}

type onChainTxMsg struct {
	txs []lndrpc.OnChainTx
	err error
}

type closeChannelMsg struct {
	txid string
	err  error
}

type closedChannelsMsg struct {
	channels []lndrpc.ClosedChannel
	err      error
}

type labelTxMsg struct {
	err error
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
	lndAlias                     string
	lndURIs                      []string
	lndVersion                   string
	lndPeers                     int
	lndChannels                  int
	lndBalance                   string
	lndSyncedChain               bool
	lndSyncedGraph               bool
	lndResponding                bool
	publicIP                     string
	channels                     []channelInfo
	pendingOpen                  int
	pendingForceClose            int
	pendingForceCloseChannels    []lndrpc.PendingForceCloseChannel
	waitingCloseChannels         []lndrpc.WaitingCloseChannel
}

type Model struct {
	cfg       *config.AppConfig
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
	// Invariant: tabs in non-active sections are
	// never added, removed, or reordered. The only
	// in-place mutation is the wallet-create →
	// auto-unlock transformation in walletCreatedMsg,
	// which preserves both the index and the Section
	// field, so the saved index stays valid. If that
	// invariant ever changes, this field needs a
	// validate-on-restore pass to detect stale
	// indices.
	sectionFocus [numSections]int
}

func NewModel(
	cfg *config.AppConfig, version string,
) Model {
	theme.Init(cfg.Theme != "light")
	// Invariant — load-bearing for the wallet-create
	// flow: lndClient stays nil until a wallet exists.
	// The walletCreatedMsg handler in update.go is the
	// only code path that creates lndClient post-launch,
	// and it runs in the same Update tick that flips
	// cfg.WalletCreated to true. Together these prevent
	// statusMsg from racing walletCreatedMsg: see the
	// walletDetected branch in status.go (gated on
	// lndClient != nil) and the statusMsg handler in
	// update.go (gated on !cfg.WalletExists via the
	// same guard in the fetcher). If a future change
	// ever needs lndClient earlier — e.g. to read a
	// macaroon before wallet creation — the walletExec
	// → walletCreatedMsg → tab transform sequence needs
	// to be re-audited for the two handlers interleaving.
	var client *lndrpc.Client
	if cfg.HasLND() && cfg.WalletExists() {
		client = lndrpc.New(cfg.Network)
	}
	m := Model{
		cfg: cfg, lndClient: client, version: version,
		subview: svNone, fetchInFlight: true,
		nav: NewNavSidebar(),
	}
	m.screenCtx = &ScreenContext{
		Cfg:       cfg,
		LndClient: client,
		Version:   version,
	}
	m.ocCtx = &OnChainContext{
		UtxoSelected: make(map[int]bool),
	}
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
	if cfg.SyncthingInstalled {
		names = append(names, "syncthing")
	}
	return names
}

func (m Model) saveCfg() {
	if err := config.Save(m.cfg); err != nil {
		logger.TUI(
			"ERROR: failed to save config: %v", err)
	}
}

func (m Model) pollInterval() time.Duration {
	if m.status == nil {
		return 3 * time.Second
	}
	if !m.status.lndResponding && m.cfg.HasLND() &&
		m.cfg.WalletExists() {
		return 5 * time.Second
	}
	return 60 * time.Second
}

func Show(cfg *config.AppConfig, version string) {
	m := NewModel(cfg, version)
	p := tea.NewProgram(m)
	result, _ := p.Run()
	final := result.(Model)

	if final.lndClient != nil {
		final.lndClient.Close()
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchStatus(m.cfg, m.lndClient),
		fetchLatestVersionCmd(),
		tickEveryCmd(m.pollInterval()))
}
