package welcome

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/logger"
)

type wSubview int

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
	// On-chain send flow
	svOnChainSendAddr
	svOnChainSendAmount
	svOnChainSendConfirm
	svOnChainSendBroadcast
)

// Tab types for the top tab bar
type tabKind int

const (
	tabMain    tabKind = iota // Main view for current section
	tabChannel                // Channel detail
	tabPayment                // Payment detail
	tabSend                   // Send payment flow
	tabReceive                // Receive payment flow
	tabPairing                // Pairing screen
	tabOnChain                // On-chain screen
	tabSyncthing
	tabLndHub
	tabOpenChannel // Channel open flow
	tabOnChainTx   // on-chain transaction detail
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

type paymentType int

const (
	payInvoice paymentType = iota
	payKeysend
)

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
	Pending       bool
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
	chanScrollOffset int
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

	// Navigation
	nav            NavSidebar
	contentFocused bool
	contentFocus   int // 0=primary area, 1=buttons

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

	// Tables
	channelTable table.Model
	txTable      table.Model

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
	onChainSendAddr  string
	onChainSendAmt   string
	onChainSendFee   int64
	onChainSendTxid  string
	onChainSendError string
	utxos            []lndrpc.UTXO
	utxoCursor       int

	// On-chain send flow
	ocSendAddrInput  textinput.Model
	ocSendAmtInput   textinput.Model
	ocCustomFeeInput textinput.Model
	ocSendAll        bool
	ocSendStep       int // 0=amount field, 1=fee tiers
	ocFeeTiers       [4]feeTier
	ocSelectedTier   int   // 0-3, or 4=custom
	ocConfirmFee     int64 // precise fee from LND
	ocSendAddrVal    string
	ocSendAmtVal     int64
	ocSendFeeRate    int64

	// On-chain transaction history
	onChainTxs      []lndrpc.OnChainTx
	onChainTxCursor int
	onChainTxFocus  int // 0=buttons, 1=tx table, 2=utxo table

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

func newChannelTable() table.Model {
	cols := []table.Column{
		{Title: "", Width: 2},
		{Title: "Name", Width: 16},
		{Title: "Local", Width: 10},
		{Title: "Remote", Width: 10},
		{Title: "Capacity", Width: 10},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithRows([]table.Row{}),
		table.WithHeight(10),
		table.WithFocused(false))
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).Bold(false)
	t.SetStyles(s)
	return t
}

func newTxTable() table.Model {
	cols := []table.Column{
		{Title: "", Width: 2},
		{Title: "Dir", Width: 6},
		{Title: "Amount", Width: 14},
		{Title: "Memo", Width: 20},
		{Title: "Date", Width: 10},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithRows([]table.Row{}),
		table.WithHeight(10),
		table.WithFocused(false))
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).Bold(false)
	t.SetStyles(s)
	return t
}

func NewModel(cfg *config.AppConfig, version string) Model {
	var client *lndrpc.Client
	if cfg.HasLND() && cfg.WalletExists() {
		client = lndrpc.New(cfg.Network)
	}
	return Model{
		cfg: cfg, lndClient: client, version: version,
		subview: svNone, fetchInFlight: true,
		nav:          NewNavSidebar(),
		channelTable: newChannelTable(),
		txTable:      newTxTable(),
	}
}

func NewTestModel(cfg *config.AppConfig, version string, store *config.Store) Model {
	m := Model{
		cfg: cfg, version: version,
		subview: svNone, fetchInFlight: true,
		nav:          NewNavSidebar(),
		channelTable: newChannelTable(),
		txTable:      newTxTable(),
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

func (m Model) svcCount() int { return len(serviceNames(m.cfg)) }

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
			if err := installer.AppendLNCLIToShell(cfg); err != nil {
				logger.TUI("Warning: lncli wrapper: %v", err)
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
		tickEvery(m.pollInterval()))
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

func openChannelCmd(client *lndrpc.Client, pubkey, host string, amount int64, private bool) tea.Cmd {
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
			return channelOpenResultMsg{err: fmt.Errorf("could not connect: %v", err)}
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

func createInvoiceCmd(client *lndrpc.Client, amount int64, memo string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return invoiceCreatedMsg{err: fmt.Errorf("LND not connected")}
		}
		inv, err := client.AddInvoice(amount, memo)
		if err != nil {
			return invoiceCreatedMsg{err: err}
		}
		return invoiceCreatedMsg{
			payReq: inv.PaymentRequest, paymentHash: inv.PaymentHash,
			amountSats: inv.AmountSats}
	}
}

func waitForInvoiceCmd(client *lndrpc.Client, paymentHash string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return invoiceSettledMsg{err: fmt.Errorf("LND not connected")}
		}
		hashBytes, err := hexDecodeBytes(paymentHash)
		if err != nil {
			return invoiceSettledMsg{err: err}
		}
		inv, err := client.WaitForInvoiceSettlement(hashBytes, 3600*time.Second)
		if err != nil {
			return invoiceSettledMsg{err: err}
		}
		return invoiceSettledMsg{settled: inv.Settled, expired: inv.IsExpired}
	}
}

func decodePayReqCmd(client *lndrpc.Client, payReq string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return payReqDecodedMsg{err: fmt.Errorf("LND not connected")}
		}
		decoded, err := client.DecodePayReq(payReq)
		if err != nil {
			return payReqDecodedMsg{err: err}
		}
		return payReqDecodedMsg{decoded: decoded}
	}
}

func sendPaymentCmd(client *lndrpc.Client, payReq string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return sendPaymentResultMsg{err: fmt.Errorf("LND not connected")}
		}
		result, err := client.SendPayment(payReq)
		if err != nil {
			return sendPaymentResultMsg{err: err}
		}
		return sendPaymentResultMsg{result: result}
	}
}

func fetchPaymentHistoryCmd(client *lndrpc.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return paymentHistoryMsg{err: fmt.Errorf("LND not connected")}
		}
		invoices, _ := client.ListInvoices(50)
		payments, _ := client.ListPayments(50)
		var all []lndrpc.PaymentEntry
		all = append(all, invoices...)
		all = append(all, payments...)
		for i := 0; i < len(all); i++ {
			for j := i + 1; j < len(all); j++ {
				if all[j].CreationDate > all[i].CreationDate {
					all[i], all[j] = all[j], all[i]
				}
			}
		}
		return paymentHistoryMsg{entries: all}
	}
}

func listUnspentCmd(client *lndrpc.Client) tea.Cmd {
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
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return sendCoinsResultMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		result, err := client.SendCoins(
			addr, amount, feeRate, sendAll)
		if err != nil {
			return sendCoinsResultMsg{err: err}
		}
		return sendCoinsResultMsg{txid: result.Txid}
	}
}

func fetchFeeTiersCmd(cfg *config.AppConfig) tea.Cmd {
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

func fetchOnChainTxCmd(client *lndrpc.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return onChainTxMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		txs, err := client.GetTransactions()
		return onChainTxMsg{txs: txs, err: err}
	}
}

func hexDecodeBytes(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd length hex")
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(b); i++ {
		high := hexVal(s[i*2])
		low := hexVal(s[i*2+1])
		if high < 0 || low < 0 {
			return nil, fmt.Errorf("invalid hex at %d", i*2)
		}
		b[i] = byte(high<<4 | low)
	}
	return b, nil
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c - 'a' + 10)
	case c >= 'A' && c <= 'F':
		return int(c - 'A' + 10)
	default:
		return -1
	}
}

func (m *Model) rebuildChannelTable() {
	if m.status == nil {
		m.channelTable.SetRows([]table.Row{})
		return
	}
	var rows []table.Row
	for _, ch := range m.status.channels {
		dot := "●"
		if !ch.Active {
			dot = "○"
		}
		if ch.Pending {
			dot = "◌"
		}
		name := ch.PeerAlias
		if name == "" && len(ch.RemotePubkey) > 12 {
			name = ch.RemotePubkey[:12] + ".."
		}
		rows = append(rows, table.Row{
			dot, name,
			formatSatsCompact(ch.LocalBalance),
			formatSatsCompact(ch.RemoteBalance),
			formatSatsCompact(ch.Capacity)})
	}
	m.channelTable.SetRows(rows)
}

func (m *Model) rebuildTxTable() {
	var rows []table.Row
	for _, entry := range m.payHistory {
		dot := "●"
		if entry.Status == "FAILED" {
			dot = "✗"
		} else if entry.Status == "EXPIRED" {
			dot = "○"
		} else if entry.Status == "IN_FLIGHT" || entry.Status == "OPEN" {
			dot = "◌"
		}
		dir := "↑ sent"
		if entry.IsIncoming {
			dir = "↓ recv"
		}
		amt := formatSats(entry.AmountSats) + " sat"
		memo := entry.Memo
		if len(memo) > 18 {
			memo = memo[:18] + ".."
		}
		if memo == "" {
			memo = "—"
		}
		ts := formatTimestamp(entry.CreationDate)
		rows = append(rows, table.Row{dot, dir, amt, memo, ts})
	}
	m.txTable.SetRows(rows)
}
