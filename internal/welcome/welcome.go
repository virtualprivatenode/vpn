package welcome

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

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
	// Wallet — send and receive
	svReceive
	svReceiveWaiting
	svReceivePaid
	svReceiveExpired
	svSend
	svSendConfirm
	svSendInFlight
	svSendResult
	svPaymentHistory
	svPaymentDetail
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
	// Receive
	recvAmountStr   string
	recvMemo        string
	recvPayReq      string
	recvPaymentHash string
	recvAmountSats  int64
	recvSettled     bool
	recvExpired     bool
	recvInputField  int // 0=amount, 1=memo
	recvError       string

	// Send
	sendPayReqInput  string
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
			payReq:      inv.PaymentRequest,
			paymentHash: inv.PaymentHash,
			amountSats:  inv.AmountSats,
		}
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
