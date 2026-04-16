package welcome

import (
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
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
	tabLndHub                           //
	tabLndHubAccount                    // LndHub account detail
	tabLndHubCreate                     // LndHub create account flow
	tabOpenChannel                      // Channel open flow
	tabCloseChannel                     // Channel close flow
	tabOnChainTx                        // on-chain transaction detail
	tabUtxoDetail                       // UTXO detail with label edit
	tabChannelHistory                   // channel history view
	tabSyncthingInstall                 // Syncthing install flow
	tabLndHubInstall                    // LndHub install flow
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
	Screen  Screen // L16: owns all state for this tab's content (nil = legacy path)
}

type feeTier struct {
	Target   int     // block target: 1, 3, 6, 25
	SatPerVB float64 // fee rate in sat/vB
	Label    string  // "~1 blk", "~3 blk", etc.
}

type svcActionDoneMsg struct{}
type tickMsg time.Time
type latestVersionMsg string

type lndhubAccountCreatedMsg struct {
	account *installer.LndHubAccount
	label   string
	err     error
}
type lndhubDeactivatedMsg struct {
	login   string
	balance string
	err     error
}
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
	ChanID        uint64
	ChannelPoint  string
	PeerAlias     string
	RemotePubkey  string
	Capacity      int64
	LocalBalance  int64
	RemoteBalance int64
	Active        bool
	Private       bool
	Initiator     bool
	Pending       bool
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
	if cfg.LndHubInstalled {
		names = append(names, "lndhub")
	}
	if cfg.LndHubInstalled && cfg.P2PMode == "hybrid" {
		names = append(names, "lndhub-proxy")
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
		fetchLatestVersion(),
		tickEvery(m.pollInterval()))
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchLatestVersion() tea.Cmd {
	return func() tea.Msg {
		return latestVersionMsg(
			installer.CheckLatestVersion())
	}
}

func pairSyncthingDeviceCmd(
	deviceID string,
) tea.Cmd {
	return func() tea.Msg {
		err := installer.PairSyncthingDevice(deviceID)
		return syncthingPairedMsg{
			deviceID: deviceID, err: err}
	}
}

func removeSyncthingDeviceCmd(
	deviceID string,
) tea.Cmd {
	return func() tea.Msg {
		err := installer.UnpairSyncthingDevice(deviceID)
		return syncthingRemovedMsg{
			deviceID: deviceID, err: err}
	}
}

func openChannelCmd(
	client *lndrpc.Client, pubkey, host string,
	amount int64, private bool, taproot bool,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return channelOpenResultMsg{
				err: fmt.Errorf("LND not connected")}
		}
		if host != "" {
			if err := client.ConnectPeer(
				pubkey, host); err != nil {
				logger.TUI(
					"Peer connect warning: %v", err)
			}
		}
		if err := client.WaitForPeer(
			pubkey, 60*time.Second); err != nil {
			return channelOpenResultMsg{
				err: fmt.Errorf(
					"could not connect: %v", err)}
		}
		result, err := client.OpenChannel(
			pubkey, amount, private, taproot)
		if err != nil {
			return channelOpenResultMsg{err: err}
		}
		return channelOpenResultMsg{
			txid: result.FundingTxID}
	}
}

func closeChannelCmd(
	client *lndrpc.Client,
	chanPoint string,
	force bool,
	satPerVbyte uint64,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return closeChannelMsg{
				err: fmt.Errorf("LND not connected")}
		}
		result, err := client.CloseChannel(
			chanPoint, force, satPerVbyte)
		if err != nil {
			return closeChannelMsg{err: err}
		}
		return closeChannelMsg{
			txid: result.ClosingTxid}
	}
}

func fetchClosedChannelsCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return closedChannelsMsg{
				err: fmt.Errorf("LND not connected")}
		}
		channels, err := client.ListClosedChannels()
		return closedChannelsMsg{
			channels: channels, err: err}
	}
}

func getNewAddressCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return newAddressMsg{
				err: fmt.Errorf("LND not connected")}
		}
		addr, err := client.GetNewAddress()
		if err != nil {
			return newAddressMsg{err: err}
		}
		return newAddressMsg{address: addr.Address}
	}
}

func createInvoiceCmd(
	client *lndrpc.Client, amount int64, memo string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return invoiceCreatedMsg{
				err: fmt.Errorf("LND not connected")}
		}
		inv, err := client.AddInvoice(amount, memo)
		if err != nil {
			return invoiceCreatedMsg{err: err}
		}
		return invoiceCreatedMsg{
			payReq:      inv.PaymentRequest,
			paymentHash: inv.PaymentHash,
			amountSats:  inv.AmountSats,
		}
	}
}

func waitForInvoiceCmd(
	client *lndrpc.Client, paymentHash string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return invoiceSettledMsg{
				err: fmt.Errorf("LND not connected")}
		}
		hashBytes, err := hex.DecodeString(paymentHash)
		if err != nil {
			return invoiceSettledMsg{err: err}
		}
		inv, err := client.WaitForInvoiceSettlement(
			hashBytes, 3600*time.Second)
		if err != nil {
			return invoiceSettledMsg{err: err}
		}
		return invoiceSettledMsg{
			settled: inv.Settled,
			expired: inv.IsExpired,
		}
	}
}

func decodePayReqCmd(
	client *lndrpc.Client, payReq string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return payReqDecodedMsg{
				err: fmt.Errorf("LND not connected")}
		}
		decoded, err := client.DecodePayReq(payReq)
		if err != nil {
			return payReqDecodedMsg{err: err}
		}
		return payReqDecodedMsg{decoded: decoded}
	}
}

func sendPaymentCmd(
	client *lndrpc.Client, payReq string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return sendPaymentResultMsg{
				err: fmt.Errorf("LND not connected")}
		}
		result, err := client.SendPayment(payReq)
		if err != nil {
			return sendPaymentResultMsg{err: err}
		}
		return sendPaymentResultMsg{result: result}
	}
}

func fetchPaymentHistoryCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return paymentHistoryMsg{
				err: fmt.Errorf("LND not connected")}
		}
		invoices, invErr := client.ListInvoices(50)
		if invErr != nil {
			logger.TUI("ListInvoices: %v", invErr)
		}
		payments, payErr := client.ListPayments(50)
		if payErr != nil {
			logger.TUI("ListPayments: %v", payErr)
		}
		var all []lndrpc.PaymentEntry
		all = append(all, invoices...)
		all = append(all, payments...)
		sort.Slice(all, func(i, j int) bool {
			return all[i].CreationDate >
				all[j].CreationDate
		})
		var rpcErr error
		switch {
		case invErr != nil && payErr != nil:
			rpcErr = fmt.Errorf(
				"invoices and payments: %v", invErr)
		case invErr != nil:
			rpcErr = fmt.Errorf("invoices: %v", invErr)
		case payErr != nil:
			rpcErr = fmt.Errorf("payments: %v", payErr)
		}
		return paymentHistoryMsg{
			entries: all, err: rpcErr}
	}
}

func listUnspentCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return utxoListMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		utxos, err := client.ListUnspent(0, 999999)
		return utxoListMsg{utxos: utxos, err: err}
	}
}

func sendCoinsCmd(
	client *lndrpc.Client, addr string,
	amount int64, feeRate int64, sendAll bool,
	outpoints []string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return sendCoinsResultMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		result, err := client.SendCoins(
			addr, amount, feeRate, sendAll, outpoints)
		if err != nil {
			return sendCoinsResultMsg{err: err}
		}
		return sendCoinsResultMsg{txid: result.Txid}
	}
}

func fetchFeeTiersCmd(
	cfg *config.AppConfig,
) tea.Cmd {
	return func() tea.Msg {
		return fetchFeeTiers(cfg)
	}
}

func estimateTxFeeCmd(
	client *lndrpc.Client, addr string,
	amount int64, targetConf int32,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return feeEstimateMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		est, err := client.EstimateFee(
			addr, amount, targetConf)
		if err != nil {
			return feeEstimateMsg{err: err}
		}
		return feeEstimateMsg{feeSats: est.FeeSats}
	}
}

func fetchOnChainTxCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return onChainTxMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		txs, err := client.GetTransactions()
		return onChainTxMsg{txs: txs, err: err}
	}
}
