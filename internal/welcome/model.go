package welcome

import (
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"charm.land/bubbles/v2/textinput"
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
	svWalletInfo
	svChannelList
	svChannelDetail
	svChannelOpen
	svChannelAmountSelect
	svChannelCustomPeer
	svChannelOpenConfirm
	svChannelOpening
	svChannelOpenResult
	svChannelFundWallet
	svZeusPairing
	svWalletPairing
	svOnChain
	svSend
	svSendConfirm
	svSendInFlight
	svSendResult
	svReceive
	svReceiveWaiting
	svReceivePaid
	svReceiveExpired
	svPaymentDetail
	svQR
	svFullURL
	svSyncthingDetail
	svSyncthingPairInput
	svSyncthingDeviceDetail
	svSyncthingWebUI
	svSyncthingDeviceQR
	svLndHubManage
	svLndHubCreateName
	svLndHubCreateAccount
	svLndHubAccountDetail
	svLndHubDeactivateConfirm
	// Shell actions
	svWalletCreate
	svLNDInstall
	svSyncthingInstall
	svLndHubInstall
	svOnChainResult
	svSelfUpdate
	svP2PUpgrade
	// On-chain send flow (single screen)
	svOnChainSend
	svOCSendConfirm
	svOCSendBroadcast
	// On-chain receive flow
	svOnChainReceive
	// Channel close flow
	svCloseType
	svCloseConfirm
	svClosing
	svCloseResult
)

// Tab types for the top tab bar
type tabKind int

const (
	tabMain           tabKind = iota // Main view for current section
	tabChannel                       // Channel detail
	tabPayment                       // Payment detail
	tabSend                          // ⚡ Send payment flow
	tabReceive                       // ⚡ Receive payment flow
	tabPairing                       // Pairing screen
	tabOnChain                       // ⛓ On-chain send flow
	tabOCReceive                     // ⛓ On-chain receive flow
	tabSyncthing                     //
	tabLndHub                        //
	tabOpenChannel                   // Channel open flow
	tabOnChainTx                     // on-chain transaction detail
	tabUtxoDetail                    // UTXO detail with label edit
	tabChannelHistory                // channel history view
)

type openTab struct {
	Kind    tabKind
	Label   string
	Index   int // channel index, payment index, etc.
	Section int // which section opened this tab
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
	err     error
}
type lndhubDeactivatedMsg struct {
	balance string
	err     error
}
type syncthingPairedMsg struct{ err error }
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

type paymentType int

const (
	payInvoice paymentType = iota
	payKeysend
)

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
	cfgStore  *config.Store
	lndClient *lndrpc.Client
	version   string
	subview   wSubview
	width     int
	height    int

	shellAction   wSubview
	status        *statusMsg
	latestVersion string
	updateConfirm bool
	fetchInFlight bool

	// Button index for content pane buttons
	btnIdx      int
	addonBtnIdx int
	addonFocus  int // 0=buttons, 1=list (syncthing/lndhub)

	// System
	svcCursor  int
	svcConfirm string
	sysConfirm string

	// Addons
	lastAccount          *installer.LndHubAccount
	hubCursor            int
	hubDeactivateBalance string
	syncDeviceLabel      string
	syncPairError        string
	syncPairSuccess      bool
	syncCursor           int
	showSecrets          bool

	// Pairing
	pairingButtonIdx int
	urlTarget        string
	qrMode           string
	qrLabel          string
	urlReturnTo      wSubview

	// Channels
	chanCursor       int
	chanOpenPeerIdx  int
	chanOpenAmount   int64
	chanOpenPrivate  bool
	chanOpenPubkey   string
	chanOpenHost     string
	chanOpenAlias    string
	chanOpenInFlight bool
	chanOpenTxid     string
	chanOpenError    string
	chanPeerList     []peerOption
	chanAmountPreset int
	chanFundAddress  string

	// Channel close
	closeForce         bool
	closeChanPoint     string
	closePeerAlias     string
	closeCapacity      int64
	closeLocalBal      int64
	closeRemoteBal     int64
	closeFeeTiers      [4]feeTier
	closeFeeInput      textinput.Model
	closeFeeIdx        int
	closeEstFee        int64
	closeTxid          string
	closeError         string
	closeBtnIdx        int
	closeConfirmBtnIdx int
	closeInFlight      bool

	// Channel history
	chanHistory       []channelHistoryEntry
	chanHistoryCursor int

	// Navigation
	nav            NavSidebar
	contentFocused bool
	sectionFocus   [numSections]int // per-section focus zone

	// Tab bar
	tabs            []openTab
	activeTab       int
	tabFocused      bool
	tabCursorX      int
	tabScrollOffset int

	// Text inputs
	sendInput       textinput.Model
	recvAmountInput textinput.Model
	recvMemoInput   textinput.Model
	chanPubkeyInput textinput.Model
	chanHostInput   textinput.Model
	chanAmountInput textinput.Model
	hubNameInput    textinput.Model
	syncDeviceInput textinput.Model

	// Receive state
	recvButtonIdx   int
	recvPayReq      string
	recvPaymentHash string
	recvAmountSats  int64
	recvSettled     bool
	recvExpired     bool
	recvError       string

	// On-chain state
	onChainAddress   string
	onChainBtnIdx    int
	onChainFocus     int // 0=buttons, 1=utxo table
	ocSendBtnIdx     int // 0=Clear, 1=Create Transaction
	onChainSendAddr  string
	onChainSendAmt   string
	onChainSendFee   int64
	onChainSendTxid  string
	onChainSendError string
	utxos            []lndrpc.UTXO
	utxoCursor       int
	// UTXO pencil icon + label edit popup
	utxoPencilFocused bool            // true when ✎ icon is focused
	utxoLabelEditing  bool            // true when label popup is open
	utxoLabelInput    textinput.Model // label edit field in popup
	utxoLabelOnBtn    bool            // true when on button row
	utxoLabelBtnIdx   int             // 0=Save, 1=Cancel
	// Coin control: UTXO selection
	utxoSelected      map[int]bool // keyed by UTXO index
	utxoSelectedTotal int64        // running sat total
	utxoOutpoints     []string     // "txid:vout" for SendCoins

	// On-chain receive state
	ocRecvAddress string
	ocRecvError   string

	// On-chain send flow
	ocSendAddrInput  textinput.Model
	ocSendAmtInput   textinput.Model
	ocSendLabelInput textinput.Model
	ocCustomFeeInput textinput.Model
	ocSendAll        bool
	ocMaxFocused     bool  // true when Max button (not amount input) is focused on step 1
	ocSendStep       int   // 0=addr, 1=amount, 2=label, 3=fee rate, 4=buttons
	ocConfirmFee     int64 // precise fee from LND
	ocConfirmBtnIdx  int   // 0=Go Back, 1=Confirm & Broadcast
	ocSendAddrVal    string
	ocSendAmtVal     int64
	ocSendFeeRate    int64
	ocSendLabelVal   string
	sendFeeTiers     [4]feeTier

	// On-chain transaction history
	onChainTxs      []lndrpc.OnChainTx
	onChainTxCursor int

	// Send state
	sendDecodedValid bool
	sendDecodedDesc  string
	sendDecodedAmt   int64
	sendDecodedDest  string
	sendDecodedExp   string
	sendInFlight     bool
	sendError        string
	sendPreimage     string
	sendRouteHops    []lndrpc.RouteHop
	sendFeeSats      int64
	sendType         paymentType

	// Payment history
	payHistory       []lndrpc.PaymentEntry
	payHistoryCursor int
}

func NewModel(
	cfg *config.AppConfig, version string,
) Model {
	theme.Init(cfg.Theme != "light")
	var client *lndrpc.Client
	if cfg.HasLND() && cfg.WalletExists() {
		client = lndrpc.New(cfg.Network)
	}
	return Model{
		cfg: cfg, lndClient: client, version: version,
		subview: svNone, fetchInFlight: true,
		nav:          NewNavSidebar(),
		utxoSelected: make(map[int]bool),
	}
}

func NewTestModel(
	cfg *config.AppConfig, version string,
	store *config.Store,
) Model {
	m := Model{
		cfg: cfg, version: version,
		subview: svNone, fetchInFlight: true,
		nav: NewNavSidebar(),
	}
	m.cfgStore = store
	m.utxoSelected = make(map[int]bool)
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
		logger.TUI(
			"ERROR: failed to save config: %v", err)
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

func (m Model) pollInterval() time.Duration {
	if m.status == nil {
		return 3 * time.Second
	}
	if !m.status.lndResponding && m.cfg.HasLND() &&
		m.cfg.WalletExists() {
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
		p := tea.NewProgram(m)
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
			if err := installer.AppendLNCLIToShell(
				cfg); err != nil {
				logger.TUI(
					"Warning: lncli wrapper: %v",
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
			installer.RunSelfUpdate(
				cfg, final.latestVersion)
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

func createLndHubAccountCmd(
	adminToken string,
) tea.Cmd {
	return func() tea.Msg {
		account, err := installer.CreateLndHubAccount(
			adminToken)
		return lndhubAccountCreatedMsg{
			account: account, err: err}
	}
}

func deactivateLndHubAccountCmd(
	login string,
) tea.Cmd {
	return func() tea.Msg {
		balance, _ := installer.GetUserBalance(login)
		err := installer.DeactivateUser(login)
		return lndhubDeactivatedMsg{
			balance: balance, err: err}
	}
}

func pairSyncthingDeviceCmd(
	deviceID string,
) tea.Cmd {
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
			pubkey, amount, private)
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
		invoices, _ := client.ListInvoices(50)
		payments, _ := client.ListPayments(50)
		var all []lndrpc.PaymentEntry
		all = append(all, invoices...)
		all = append(all, payments...)
		sort.Slice(all, func(i, j int) bool {
			return all[i].CreationDate >
				all[j].CreationDate
		})
		return paymentHistoryMsg{entries: all}
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
