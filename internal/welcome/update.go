package welcome

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Focus helpers ────────────────────────────────────────

func (m *Model) focusSidebar() {
	m.nav.Focus()
	m.tabFocused = false
	m.contentFocused = false
}

func (m *Model) focusTabBar() {
	m.nav.Blur()
	m.tabFocused = true
	m.contentFocused = false
}

func (m *Model) focusContent() {
	m.nav.Blur()
	m.tabFocused = false
	m.contentFocused = true
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.ResumeMsg:
		m.fetchInFlight = true
		if m.cfg.HasLND() && m.cfg.WalletExists() &&
			m.lndClient != nil {
			return m, tea.Sequence(
				fetchStatus(m.cfg, m.lndClient),
				fetchPaymentHistoryCmd(m.lndClient))
		}
		return m, fetchStatus(m.cfg, m.lndClient)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.PasteMsg:
		return m.handlePaste(msg)
	case svcActionDoneMsg:
		return m, fetchStatus(m.cfg, m.lndClient)
	case statusMsg:
		m.fetchInFlight = false
		m.status = &msg
		if msg.walletDetected && !m.cfg.WalletCreated {
			m.cfg.WalletCreated = true
			m.saveCfg()
		}
		m.rebuildChannelTable()
		return m, nil
	case latestVersionMsg:
		m.latestVersion = string(msg)
		return m, nil
	case lndhubAccountCreatedMsg:
		if msg.err != nil {
			logger.TUI(
				"Warning: LndHub create failed: %v",
				msg.err)
			m.subview = svLndHubManage
			return m, nil
		}
		if msg.account != nil {
			m.lastAccount = msg.account
			m.cfg.LndHubAccounts = append(
				m.cfg.LndHubAccounts,
				config.LndHubAccount{
					Label: m.hubNameInput.Value(),
					Login: msg.account.Login,
					CreatedAt: time.Now().
						Format("2006-01-02"),
					Active: true,
				})
			m.saveCfg()
			m.addonBtnIdx = 0
			m.subview = svLndHubCreateAccount
		}
		return m, nil
	case lndhubDeactivatedMsg:
		if m.hubCursor < len(m.cfg.LndHubAccounts) {
			acct := &m.cfg.LndHubAccounts[m.hubCursor]
			if msg.err != nil {
				logger.TUI(
					"Warning: deactivate failed: %v",
					msg.err)
			} else {
				acct.Active = false
				acct.DeactivatedAt = time.Now().
					Format("2006-01-02")
				acct.BalanceOnDeactivate = msg.balance
				m.saveCfg()
			}
		}
		m.subview = svLndHubManage
		return m, nil
	case syncthingPairedMsg:
		if msg.err != nil {
			m.syncPairError = msg.err.Error()
			m.syncPairSuccess = false
		} else {
			m.syncPairError = ""
			m.syncPairSuccess = true
			m.cfg.SyncthingDevices = append(
				m.cfg.SyncthingDevices,
				config.SyncthingDevice{
					Name: "Device " + string(
						rune('0'+len(
							m.cfg.SyncthingDevices)+1)),
					DeviceID: syncthingIDValue(
						m.syncDeviceInput),
					PairedAt: time.Now().
						Format("2006-01-02"),
				})
			m.saveCfg()
		}
		return m, nil
	case channelOpenResultMsg:
		m.chanOpenInFlight = false
		if msg.err != nil {
			m.chanOpenError = msg.err.Error()
		} else {
			m.chanOpenTxid = msg.txid
			m.chanOpenError = ""
		}
		m.subview = svChannelOpenResult
		return m, nil
	case newAddressMsg:
		if msg.err == nil {
			m.chanFundAddress = msg.address
			m.onChainAddress = msg.address
			if m.subview == svOnChainReceive {
				m.ocRecvAddress = msg.address
			}
		}
		return m, nil
	case invoiceCreatedMsg:
		if msg.err != nil {
			m.recvError = msg.err.Error()
			return m, nil
		}
		m.recvPayReq = msg.payReq
		m.recvPaymentHash = msg.paymentHash
		m.recvAmountSats = msg.amountSats
		m.subview = svReceiveWaiting
		return m, waitForInvoiceCmd(
			m.lndClient, msg.paymentHash)
	case invoiceSettledMsg:
		if msg.err != nil {
			return m, nil
		}
		if msg.settled {
			m.recvSettled = true
			m.subview = svReceivePaid
		} else if msg.expired {
			m.recvExpired = true
			m.subview = svReceiveExpired
		}
		return m, nil
	case payReqDecodedMsg:
		if msg.err != nil {
			m.sendError = msg.err.Error()
			return m, nil
		}
		if msg.decoded.IsExpired {
			m.sendError = "This invoice has expired"
			return m, nil
		}
		m.sendDecodedValid = true
		m.sendDecodedAmt = msg.decoded.AmountSats
		m.sendDecodedDesc = msg.decoded.Description
		m.sendDecodedDest = msg.decoded.Destination
		m.subview = svSendConfirm
		return m, nil
	case sendPaymentResultMsg:
		m.sendInFlight = false
		if msg.err != nil {
			m.sendError = msg.err.Error()
		} else if msg.result.Status == "SUCCEEDED" {
			m.sendPreimage = msg.result.Preimage
			m.sendFeeSats = msg.result.FeeSats
			m.sendRouteHops = msg.result.Hops
			m.sendError = ""
		} else {
			m.sendError = msg.result.Error
		}
		m.subview = svSendResult
		return m, nil
	case paymentHistoryMsg:
		if msg.err == nil {
			m.payHistory = msg.entries
			m.rebuildTxTable()
		}
		return m, nil
	case utxoListMsg:
		if msg.err == nil {
			m.utxos = msg.utxos
			// Prune selections beyond new UTXO range
			for idx := range m.utxoSelected {
				if idx >= len(m.utxos) {
					delete(m.utxoSelected, idx)
				}
			}
			m.recalcSelectedTotal()
		}
		return m, nil
	case onChainTxMsg:
		if msg.err == nil {
			m.onChainTxs = msg.txs
		}
		return m, nil
	case sendCoinsResultMsg:
		if msg.err != nil {
			m.onChainSendError = msg.err.Error()
		} else {
			m.onChainSendTxid = msg.txid
			m.onChainSendError = ""
			m.clearUtxoSelection()
		}
		m.subview = svOnChainResult
		return m, nil
	case closeChannelMsg:
		m.closeInFlight = false
		if msg.err != nil {
			m.closeError = msg.err.Error()
		} else {
			m.closeTxid = msg.txid
			m.closeError = ""
		}
		m.subview = svCloseResult
		return m, nil
	case closedChannelsMsg:
		if msg.err == nil {
			m.buildChannelHistory(msg.channels)
		}
		return m, nil
	case labelTxMsg:
		m.utxoLabelEditing = false
		if msg.err == nil {
			return m, fetchOnChainTxCmd(m.lndClient)
		}
		return m, nil
	case feeTiersMsg:
		if msg.err == nil {
			m.ocFeeTiers = msg.tiers
			if isCloseSubview(m.subview) {
				m.closeFeeTiers = msg.tiers
			}
		} else {
			m.onChainSendError = msg.err.Error()
		}
		return m, nil
	case feeEstimateMsg:
		if msg.err == nil {
			m.ocConfirmFee = msg.feeSats
		} else {
			m.onChainSendError = msg.err.Error()
		}
		return m, nil
	case tickMsg:
		if m.fetchInFlight {
			return m, tickEvery(m.pollInterval())
		}
		m.fetchInFlight = true
		return m, tea.Batch(
			fetchStatus(m.cfg, m.lndClient),
			tickEvery(m.pollInterval()))
	}
	return m, nil
}

// ── Key dispatch (tab-first) ─────────────────────────────

func (m Model) handleKey(
	msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+z" {
		return m, tea.Suspend
	}

	// 1. Confirm dialogs
	if m.svcConfirm != "" {
		return m.handleSvcConfirmKey(key)
	}
	if m.sysConfirm != "" {
		return m.handleSysConfirmKey(key)
	}
	if m.updateConfirm {
		return m.handleUpdateConfirmKey(key)
	}

	// 2. Fullscreen views
	if m.subview == svQR || m.subview == svFullURL ||
		m.subview == svSyncthingDeviceQR {
		return m.handleGenericSubviewKey(key)
	}

	// 3. Tab bar focused
	if m.tabFocused {
		return m.handleTabBarKey(key)
	}

	// 4. Sidebar focused
	if m.nav.Focused {
		return m.handleSidebarKey(key)
	}

	// 5. Content focused — dispatch by active tab
	tabs := m.effectiveTabs()
	if m.activeTab > 0 && m.activeTab < len(tabs) {
		tab := tabs[m.activeTab]
		return m.handleTabContentKey(tab, key, msg)
	}

	// 6. Section home
	return m.handleSectionHomeKey(key, msg)
}

// ── Tab content dispatch ─────────────────────────────────

func (m Model) handleTabContentKey(
	tab openTab, key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch tab.Kind {
	case tabChannelHistory:
		return m.handleChannelHistoryKey(key)
	case tabChannel:
		if isCloseSubview(m.subview) {
			return m.handleCloseFlowKey(key, msg)
		}
		return m.handleChannelDetailKey(key)
	case tabPayment:
		return m.handlePaymentDetailKey(key)
	case tabOnChainTx:
		return m.handleOnChainTxDetailKey(key)
	case tabUtxoDetail:
		return m.handleUtxoDetailKey(key, msg)
	case tabOpenChannel:
		return m.handleOpenChannelTabKey(key, msg)
	case tabSend:
		return m.handleSendTabKey(key, msg)
	case tabReceive:
		return m.handleReceiveTabKey(key, msg)
	case tabOnChain:
		return m.handleOnChainTabKey(key, msg)
	case tabOCReceive:
		return m.handleOCReceiveTabKey(key)
	case tabPairing:
		return m.handlePairingTabKey(key)
	case tabSyncthing:
		return m.handleSyncthingTabKey(key, msg)
	case tabLndHub:
		return m.handleLndHubTabKey(key, msg)
	}
	return m.handleSectionHomeKey(key, msg)
}

func (m Model) handleChannelHistoryKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.chanHistoryCursor > 0 {
			m.chanHistoryCursor--
		} else if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
	case "down", "j":
		if m.chanHistoryCursor <
			len(m.chanHistory)-1 {
			m.chanHistoryCursor++
		}
	case "backspace":
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

func (m Model) handleOCReceiveTabKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.ocRecvBtnIdx > 0 {
			m.ocRecvBtnIdx--
			return m, nil
		}
		m.focusSidebar()
		return m, nil
	case "right", "l":
		if m.ocRecvBtnIdx < 1 {
			m.ocRecvBtnIdx++
		}
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "backspace":
		m.ocRecvAddress = ""
		m.ocRecvError = ""
		m.subview = svNone
		return m.closeTab(m.activeTab)
	case "enter":
		if m.ocRecvAddress != "" {
			if m.ocRecvBtnIdx == 0 {
				m.urlTarget = m.ocRecvAddress
				m.qrLabel = "On-Chain Address"
				m.urlReturnTo = svOnChainReceive
				m.subview = svQR
				return m, nil
			}
			if m.ocRecvBtnIdx == 1 {
				m.ocRecvAddress = ""
				return m,
					getNewAddressCmd(m.lndClient)
			}
		}
		return m, nil
	}
	return m, nil
}

// ── View-only tab handlers ───────────────────────────────

func (m Model) handleChannelDetailKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.contentFocus == 1 {
			m.contentFocus = 0
			return m, nil
		}
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			return m, nil
		}
	case "down", "j":
		if m.contentFocus == 0 {
			m.contentFocus = 1
			return m, nil
		}
	case "enter":
		if m.contentFocus == 1 {
			return m.startCloseFlow()
		}
	case "backspace":
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

func (m Model) startCloseFlow() (
	tea.Model, tea.Cmd,
) {
	if m.status == nil ||
		m.chanCursor >= len(m.status.channels) {
		return m, nil
	}
	ch := m.status.channels[m.chanCursor]
	if ch.Pending {
		return m, nil
	}

	m.closeChanPoint = ch.ChannelPoint
	m.closePeerAlias = ch.PeerAlias
	m.closeCapacity = ch.Capacity
	m.closeLocalBal = ch.LocalBalance
	m.closeRemoteBal = ch.RemoteBalance
	m.closeForce = false
	m.closeFeeIdx = 0
	m.closeEstFee = 0
	m.closeTxid = ""
	m.closeError = ""
	m.closeBtnIdx = 0
	m.closeInFlight = false

	m.subview = svCloseType
	return m, fetchFeeTiersCmd(m.cfg)
}

func (m Model) handleCloseFlowKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svCloseType:
		return m.handleCloseTypeKey(key)
	case svCloseConfirm:
		return m.handleCloseConfirmKey(key)
	case svClosing:
		if key == "q" || key == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	case svCloseResult:
		return m.handleCloseResultKey(key)
	}
	return m, nil
}

func (m Model) handleCloseTypeKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.closeBtnIdx > 0 {
			m.closeBtnIdx--
		} else if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
	case "down", "j":
		if m.closeBtnIdx < 1 {
			m.closeBtnIdx++
		}
	case "backspace":
		m.subview = svNone
		m.contentFocus = 0
		return m, nil
	case "enter":
		m.closeForce = m.closeBtnIdx == 1
		m.closeError = ""
		m.subview = svCloseConfirm
		return m, nil
	}
	return m, nil
}

func (m Model) handleCloseConfirmKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
	case "backspace":
		m.subview = svCloseType
		m.closeError = ""
		return m, nil
	case "y":
		if m.closeInFlight {
			return m, nil
		}
		m.closeInFlight = true
		m.closeError = ""
		m.subview = svClosing

		var feeRate uint64
		if !m.closeForce &&
			m.closeFeeIdx < 4 {
			tier := m.closeFeeTiers[m.closeFeeIdx]
			if tier.SatPerVB > 0 {
				feeRate = uint64(tier.SatPerVB)
			}
		}

		return m, closeChannelCmd(
			m.lndClient,
			m.closeChanPoint,
			m.closeForce,
			feeRate)
	}
	return m, nil
}

func (m Model) handleCloseResultKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter", "backspace":
		m.subview = svNone
		m.closeError = ""
		m.closeTxid = ""
		m.closeInFlight = false
		m.contentFocus = 0
		m.nav.SetActive(secChannels)
		cm, cmd := m.closeTab(m.activeTab)
		return cm, tea.Batch(cmd,
			fetchStatus(m.cfg, m.lndClient))
	}
	return m, nil
}

func (m Model) handlePaymentDetailKey(
	key string,
) (tea.Model, tea.Cmd) {
	return m.handleViewOnlyKey(key, svNone)
}

func (m Model) handleOnChainTxDetailKey(
	key string,
) (tea.Model, tea.Cmd) {
	return m.handleViewOnlyKey(key, svNone)
}

func (m Model) handleUtxoDetailKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	if m.utxoLabelEditing {
		switch key {
		case "enter":
			if m.utxoCursor < len(m.utxos) {
				txid :=
					m.utxos[m.utxoCursor].Txid
				label := m.utxoLabelInput.Value()
				m.utxoLabelEditing = false
				return m, labelTxCmd(
					m.lndClient, txid, label)
			}
			m.utxoLabelEditing = false
			return m, nil
		case "escape":
			m.utxoLabelEditing = false
			return m, nil
		case "q":
			// Don't quit while editing
			return m, nil
		default:
			var cmd tea.Cmd
			m.utxoLabelInput, cmd =
				m.utxoLabelInput.Update(msg)
			return m, cmd
		}
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.contentFocus == 1 {
			m.contentFocus = 0
			return m, nil
		}
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
	case "down", "j":
		if m.contentFocus == 0 {
			m.contentFocus = 1
			return m, nil
		}
	case "enter":
		if m.contentFocus == 1 {
			// Start label editing
			current := ""
			if m.utxoCursor < len(m.utxos) {
				current = m.utxoTxLabel(
					m.utxos[m.utxoCursor].Txid)
			}
			m.utxoLabelInput =
				newUtxoLabelInput(current)
			m.utxoLabelEditing = true
			return m, nil
		}
	case "backspace":
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

func labelTxCmd(
	client *lndrpc.Client, txid, label string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return labelTxMsg{
				err: fmt.Errorf("LND not connected")}
		}
		err := client.LabelTransaction(
			txid, label, true)
		return labelTxMsg{err: err}
	}
}

// ── Coin control helpers ─────────────────────────────────

func (m *Model) toggleUtxoSelection(idx int) {
	if idx < 0 || idx >= len(m.utxos) {
		return
	}
	if m.utxoSelected[idx] {
		delete(m.utxoSelected, idx)
	} else {
		m.utxoSelected[idx] = true
	}
	m.recalcSelectedTotal()
}

func (m *Model) recalcSelectedTotal() {
	m.utxoSelectedTotal = 0
	m.utxoOutpoints = nil
	for idx := range m.utxoSelected {
		if idx < len(m.utxos) {
			m.utxoSelectedTotal +=
				m.utxos[idx].AmountSats
			m.utxoOutpoints = append(
				m.utxoOutpoints,
				fmt.Sprintf("%s:%d",
					m.utxos[idx].Txid,
					m.utxos[idx].Vout))
		}
	}
}

func (m *Model) clearUtxoSelection() {
	m.utxoSelected = make(map[int]bool)
	m.utxoSelectedTotal = 0
	m.utxoOutpoints = nil
}

// ── Flow tab handlers ────────────────────────────────────

func (m Model) handleOpenChannelTabKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svChannelOpen:
		return m.handleChannelOpenKey(key)
	case svChannelCustomPeer:
		return m.handleChannelCustomPeerKey(key, msg)
	case svChannelAmountSelect:
		return m.handleChannelAmountKey(key, msg)
	case svChannelOpenConfirm:
		return m.handleChannelConfirmKey(key)
	case svChannelOpening:
		return m.handleChannelOpeningKey(key)
	case svChannelOpenResult:
		return m.handleChannelResultKey(key)
	case svChannelFundWallet:
		return m.handleChannelFundKey(key)
	default:
		return m.closeTab(m.activeTab)
	}
}

func (m Model) handleSendTabKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svSend:
		return m.handleSendKey(key, msg)
	case svSendConfirm:
		return m.handleSendConfirmKey(key)
	case svSendInFlight:
		return m.handleSendInFlightKey(key)
	case svSendResult:
		return m.handleSendResultKey(key)
	default:
		return m.closeTab(m.activeTab)
	}
}

func (m Model) handleReceiveTabKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svReceive:
		return m.handleReceiveKey(key, msg)
	case svReceiveWaiting:
		return m.handleReceiveWaitingKey(key)
	case svReceivePaid:
		return m.handleReceivePaidKey(key)
	case svReceiveExpired:
		return m.handleReceiveExpiredKey(key)
	default:
		return m.closeTab(m.activeTab)
	}
}

func (m Model) handleOnChainTabKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svOnChain:
		return m.handleOnChainKey(key)
	case svOnChainResult:
		return m.handleOnChainKey(key)
	case svOnChainSend:
		return m.handleOCSendKey(key, msg)
	case svOCSendConfirm:
		return m.handleOCSendConfirmKey(key)
	case svOCSendBroadcast:
		if key == "q" || key == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	default:
		return m.closeTab(m.activeTab)
	}
}

func (m Model) handlePairingTabKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.pairingButtonIdx > 0 {
			m.pairingButtonIdx--
			return m, nil
		}
		m.focusSidebar()
		return m, nil
	case "right", "l":
		maxBtn := 1
		if m.cfg.P2PMode == "hybrid" {
			maxBtn = 2
		}
		if m.pairingButtonIdx < maxBtn {
			m.pairingButtonIdx++
		}
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "backspace":
		return m.closeTab(m.activeTab)
	case "enter":
		return m.handlePairingEnter()
	}
	return m, nil
}

func (m Model) handlePairingContentKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		if m.pairingButtonIdx > 0 {
			m.pairingButtonIdx--
		}
	case "right", "l":
		if m.pairingButtonIdx < 3 {
			m.pairingButtonIdx++
		}
	case "enter":
		return m.handlePairingEnter()
	}
	return m, nil
}

func (m Model) handleSyncthingTabKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svSyncthingDetail:
		return m.handleSyncDetailKey(key)
	case svSyncthingPairInput:
		return m.handleSyncthingPairInputKey(key, msg)
	case svSyncthingWebUI:
		return m.handleSyncWebUIKey(key)
	case svSyncthingDeviceDetail:
		return m.handleSyncDeviceDetailKey(key)
	case svSyncthingDeviceQR:
		return m.handleGenericSubviewKey(key)
	default:
		return m.closeTab(m.activeTab)
	}
}

func (m Model) handleSyncDeviceDetailKey(
	key string,
) (tea.Model, tea.Cmd) {
	return m.handleViewOnlyKey(key, svSyncthingDetail)
}

func (m Model) handleLndHubTabKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svLndHubManage:
		return m.handleLndhubManageKey(key)
	case svLndHubCreateName:
		return m.handleLndHubCreateNameKey(key, msg)
	case svLndHubCreateAccount:
		return m.handleLndHubCreatedKey(key)
	case svLndHubAccountDetail:
		return m.handleLndHubAccountDetailKey(key)
	case svLndHubDeactivateConfirm:
		return m.handleLndHubDeactivateKey(key)
	default:
		return m.closeTab(m.activeTab)
	}
}

// New handler for LndHub created screen (2 buttons)
func (m Model) handleLndHubCreatedKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.addonBtnIdx > 0 {
			m.addonBtnIdx--
			return m, nil
		}
		m.focusSidebar()
		return m, nil
	case "right", "l":
		if m.addonBtnIdx < 1 {
			m.addonBtnIdx++
		}
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "backspace":
		m.addonBtnIdx = 0
		m.subview = svLndHubManage
		return m, nil
	case "enter":
		if m.addonBtnIdx == 0 {
			// Show QR
			if m.lastAccount != nil {
				hubOnion := readOnion(
					paths.TorLndHubHostname)
				if hubOnion != "" {
					qrData := fmt.Sprintf(
						"lndhub://%s:%s@%s:%s",
						m.lastAccount.Login,
						m.lastAccount.Password,
						hubOnion,
						paths.LndHubExternalPort)
					m.urlTarget = qrData
					m.qrLabel = "LndHub — " +
						m.hubNameInput.Value()
					m.urlReturnTo =
						svLndHubCreateAccount
					m.subview = svQR
					return m, nil
				}
			}
		} else {
			// Done
			m.addonBtnIdx = 0
			m.subview = svLndHubManage
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleLndHubAccountDetailKey(
	key string,
) (tea.Model, tea.Cmd) {
	return m.handleViewOnlyKey(key, svLndHubManage)
}

func (m Model) handleLndHubDeactivateKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "y":
		if m.hubCursor < len(m.cfg.LndHubAccounts) {
			login :=
				m.cfg.LndHubAccounts[m.hubCursor].Login
			return m, deactivateLndHubAccountCmd(login)
		}
	case "n", "backspace":
		m.subview = svLndHubManage
	}
	return m, nil
}

// ── Section home key dispatch ────────────────────────────

func (m Model) handleSectionHomeKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		m.focusSidebar()
		m.contentFocus = 0
		return m, nil
	}

	sec := m.nav.ActiveSection()
	switch sec {
	case secChannels:
		return m.handleChannelsHomeKey(key)
	case secWallet:
		return m.handleWalletHomeKey(key)
	case secOnChain:
		return m.handleOnChainHomeKey(key)
	case secAddons:
		return m.handleAddonsHomeKey(key)
	case secSystem:
		return m.handleSystemHomeKey(key)
	}
	return m, nil
}

// ── Channels home ────────────────────────────────────────

func (m Model) handleChannelsHomeKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		if m.contentFocus == 1 && m.btnIdx > 0 {
			m.btnIdx--
			return m, nil
		}
		m.focusSidebar()
		m.contentFocus = 0
		return m, nil
	case "up", "k":
		if m.contentFocus == 1 {
			m.contentFocus = 0
			if m.status != nil &&
				len(m.status.channels) > 0 {
				m.chanCursor =
					len(m.status.channels) - 1
			}
		} else if m.contentFocus == 0 {
			if m.chanCursor > 0 {
				m.chanCursor--
			} else if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = 1
			}
		}
	case "down", "j":
		if m.contentFocus == 0 {
			if m.status != nil &&
				m.chanCursor <
					len(m.status.channels)-1 {
				m.chanCursor++
			} else {
				m.contentFocus = 1
				m.btnIdx = 0
			}
		}
	case "right", "l":
		if m.contentFocus == 1 && m.btnIdx < 1 {
			m.btnIdx++
		}
	case "enter":
		if m.contentFocus == 0 {
			if m.status != nil &&
				len(m.status.channels) > 0 &&
				m.chanCursor <
					len(m.status.channels) &&
				!m.status.channels[m.chanCursor].
					Pending {
				ch := m.status.channels[m.chanCursor]
				label := ch.PeerAlias
				if label == "" {
					label =
						ch.RemotePubkey[:12] + ".."
				}
				if len(label) > 17 {
					label = label[:17] + "..."
				}
				m.findOrOpenTab(tabChannel, label,
					m.chanCursor, secChannels)
			}
		} else if m.contentFocus == 1 {
			switch m.btnIdx {
			case 0:
				return m.startChannelOpenCmd()
			case 1:
				m.chanHistoryCursor = 0
				m.openFlowTab(tabChannelHistory,
					"History", secChannels)
				return m,
					fetchClosedChannelsCmd(
						m.lndClient)
			}
		}
	}
	return m, nil
}

// ── Wallet home ──────────────────────────────────────────

func (m Model) handleWalletHomeKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		if m.contentFocus == 1 {
			if m.btnIdx > 0 {
				m.btnIdx--
				return m, nil
			}
		}
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.contentFocus == 0 {
			if m.payHistoryCursor > 0 {
				m.payHistoryCursor--
			} else {
				m.contentFocus = 1
				m.btnIdx = 0
			}
		} else if m.contentFocus == 1 {
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = 1
				return m, nil
			}
		}
	case "down", "j":
		if m.contentFocus == 1 {
			m.contentFocus = 0
			m.payHistoryCursor = 0
		} else if m.contentFocus == 0 {
			if m.payHistoryCursor <
				len(m.payHistory)-1 {
				m.payHistoryCursor++
			}
		}
	case "right", "l":
		if m.contentFocus == 1 {
			if m.btnIdx < 2 {
				m.btnIdx++
			}
		}
	case "enter":
		if m.contentFocus == 0 &&
			len(m.payHistory) > 0 {
			entry :=
				m.payHistory[m.payHistoryCursor]
			label := entry.Memo
			if label == "" {
				if entry.IsIncoming {
					label = "↓ " + formatSats(
						entry.AmountSats)
				} else {
					label = "↑ " + formatSats(
						entry.AmountSats)
				}
			}
			if len(label) > 14 {
				label = label[:12] + ".."
			}
			m.findOrOpenTab(tabPayment, label,
				m.payHistoryCursor, secWallet)
		} else if m.contentFocus == 1 {
			switch m.btnIdx {
			case 0:
				if m.cfg.HasLND() &&
					m.cfg.WalletExists() {
					m.resetSendState()
					cw := min(m.width,
						theme.ContentWidth+20) -
						m.nav.Width - 5
					if cw > 58 {
						cw = 58
					}
					if cw < 20 {
						cw = 20
					}
					m.sendInput.SetWidth(cw)
					m.subview = svSend
					m.openFlowTab(tabSend,
						"⚡ Send", secWallet)
				}
			case 1:
				if m.cfg.HasLND() &&
					m.cfg.WalletExists() {
					m.resetReceiveState()
					m.subview = svReceive
					m.openFlowTab(tabReceive,
						"⚡ Receive", secWallet)
				}
			case 2:
				m.pairingButtonIdx = 0
				m.subview = svWalletPairing
				m.openFlowTab(tabPairing,
					"⚡ Zeus — LND REST",
					secWallet)
			}
		}
	}
	return m, nil
}

func (m *Model) buildChannelHistory(
	closed []lndrpc.ClosedChannel,
) {
	var entries []channelHistoryEntry

	// Active and inactive channels
	if m.status != nil {
		for _, ch := range m.status.channels {
			if ch.Pending {
				entries = append(entries,
					channelHistoryEntry{
						PeerAlias:    ch.PeerAlias,
						RemotePubkey: ch.RemotePubkey,
						Capacity:     ch.Capacity,
						LocalBalance: ch.LocalBalance,
						Status:       "pending open",
						CloseType:    "—",
						Active:       false,
					})
				continue
			}
			status := "active"
			if !ch.Active {
				status = "inactive"
			}
			entries = append(entries,
				channelHistoryEntry{
					PeerAlias:    ch.PeerAlias,
					RemotePubkey: ch.RemotePubkey,
					Capacity:     ch.Capacity,
					LocalBalance: ch.LocalBalance,
					Status:       status,
					CloseType:    "—",
					Active:       ch.Active,
				})
		}

		// Waiting close channels (close tx broadcast,
		// not yet confirmed)
		for _, wc := range m.status.waitingCloseChannels {
			entries = append(entries,
				channelHistoryEntry{
					PeerAlias:    wc.PeerAlias,
					RemotePubkey: wc.RemotePubkey,
					Capacity:     wc.Capacity,
					LocalBalance: wc.LocalBalance,
					LimboBalance: wc.LimboBalance,
					Status:       "waiting close",
					CloseType:    "closing",
					ClosingTxid:  wc.ClosingTxid,
					Active:       false,
				})
		}

		// Pending force close channels
		for _, fc := range m.status.pendingForceCloseChannels {
			entries = append(entries,
				channelHistoryEntry{
					PeerAlias:       fc.PeerAlias,
					RemotePubkey:    fc.RemotePubkey,
					Capacity:        fc.Capacity,
					LocalBalance:    fc.LocalBalance,
					LimboBalance:    fc.LimboBalance,
					Status:          "force close",
					CloseType:       "force",
					ClosingTxid:     fc.ClosingTxid,
					BlocksRemaining: fc.BlocksRemaining,
					Active:          false,
				})
		}
	}

	// Closed channels
	for _, ch := range closed {
		closeLabel := ch.CloseType
		switch closeLabel {
		case "cooperative":
			closeLabel = "coop"
		case "force":
			closeLabel = "force"
		case "breach":
			closeLabel = "breach"
		case "canceled":
			closeLabel = "canceled"
		case "abandoned":
			closeLabel = "abandoned"
		}

		entries = append(entries,
			channelHistoryEntry{
				PeerAlias:    ch.PeerAlias,
				RemotePubkey: ch.RemotePubkey,
				Capacity:     ch.Capacity,
				Status:       "closed",
				CloseType:    closeLabel,
				ClosingTxid:  ch.ClosingTxid,
				SettledBal:   ch.SettledBal,
				CloseHeight:  ch.CloseHeight,
			})
	}

	m.chanHistory = entries
}

// ── On-Chain home ────────────────────────────────────────

func (m Model) handleOnChainHomeKey(
	key string,
) (tea.Model, tea.Cmd) {
	if m.subview == svOnChainResult {
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter", "backspace":
			m.subview = svNone
			m.onChainSendTxid = ""
			m.onChainSendError = ""
			return m, tea.Sequence(
				listUnspentCmd(m.lndClient),
				fetchOnChainTxCmd(m.lndClient),
				fetchStatus(m.cfg, m.lndClient))
		}
		return m, nil
	}

	if isOnChainSendSubview(m.subview) {
		m.openFlowTab(tabOnChain,
			"Send", secOnChain)
		return m, nil
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.onChainTxFocus == 0 &&
			m.onChainBtnIdx > 0 {
			m.onChainBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "up", "k":
		switch m.onChainTxFocus {
		case 2:
			if m.onChainTxCursor > 0 {
				m.onChainTxCursor--
			} else {
				m.onChainTxFocus = 1
				if len(m.utxos) > 0 {
					m.utxoCursor =
						len(m.utxos) - 1
				}
			}
		case 1:
			if m.utxoCursor > 0 {
				m.utxoCursor--
			} else {
				m.onChainTxFocus = 0
				m.onChainBtnIdx = 0
			}
		case 0:
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = 1
				return m, nil
			}
		}
	case "down", "j":
		switch m.onChainTxFocus {
		case 0:
			if len(m.utxos) > 0 {
				m.onChainTxFocus = 1
				m.utxoCursor = 0
			} else if len(m.onChainTxs) > 0 {
				m.onChainTxFocus = 2
				m.onChainTxCursor = 0
			}
		case 1:
			if m.utxoCursor < len(m.utxos)-1 {
				m.utxoCursor++
			} else if len(m.onChainTxs) > 0 {
				m.onChainTxFocus = 2
				m.onChainTxCursor = 0
			}
		case 2:
			if m.onChainTxCursor < len(m.onChainTxs)-1 {
				m.onChainTxCursor++
			}
		}
	case "right", "l":
		if m.onChainTxFocus == 0 &&
			m.onChainBtnIdx < 1 {
			m.onChainBtnIdx++
		}
	case "space":
		if m.onChainTxFocus == 1 &&
			m.utxoCursor < len(m.utxos) {
			m.toggleUtxoSelection(m.utxoCursor)
		}
		return m, nil
	case "enter":
		if m.onChainTxFocus == 0 {
			switch m.onChainBtnIdx {
			case 0:
				m.ocRecvAddress = ""
				m.ocRecvBtnIdx = 0
				m.ocRecvError = ""
				m.subview = svOnChainReceive
				m.openFlowTab(tabOCReceive,
					"⛓ Receive", secOnChain)
				return m,
					getNewAddressCmd(m.lndClient)
			case 1:
				m.resetOnChainSendState()
				if len(m.utxoSelected) > 0 {
					m.ocSendAmtInput.SetValue(
						fmt.Sprintf("%d",
							m.utxoSelectedTotal))
				}
				m.subview = svOnChainSend
				m.openFlowTab(tabOnChain,
					"⛓ Send", secOnChain)
				return m, nil
			}
		} else if m.onChainTxFocus == 1 &&
			m.utxoCursor < len(m.utxos) {
			u := m.utxos[m.utxoCursor]
			label := u.Address
			if len(label) > 14 {
				label = label[:12] + ".."
			}
			m.utxoLabelEditing = false
			m.contentFocus = 0
			m.findOrOpenTab(tabUtxoDetail, label,
				m.utxoCursor, secOnChain)
		} else if m.onChainTxFocus == 2 &&
			m.onChainTxCursor < len(m.onChainTxs) {
			tx := m.onChainTxs[m.onChainTxCursor]
			label := tx.Label
			if len(label) > 14 {
				label = label[:12] + ".."
			}
			m.findOrOpenTab(tabOnChainTx, label,
				m.onChainTxCursor, secOnChain)
		}
	}
	return m, nil
}

func isOnChainSendSubview(sv wSubview) bool {
	switch sv {
	case svOnChainSend, svOCSendConfirm,
		svOCSendBroadcast:
		return true
	}
	return false
}

// ── Addons home ──────────────────────────────────────────

func (m Model) handleAddonsHomeKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.btnIdx > 0 {
			m.btnIdx--
		} else if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = 1
			return m, nil
		}
	case "down", "j":
		if m.btnIdx < 1 {
			m.btnIdx++
		}
	case "enter":
		switch m.btnIdx {
		case 0:
			if m.cfg.SyncthingInstalled {
				m.subview = svSyncthingDetail
				m.addonBtnIdx = 0
				m.addonFocus = 0
				m.openFlowTab(tabSyncthing,
					"Syncthing", secAddons)
			} else if m.cfg.HasLND() &&
				m.cfg.WalletExists() {
				m.shellAction = svSyncthingInstall
				return m, tea.Quit
			}
		case 1:
			if m.cfg.LndHubInstalled {
				m.hubCursor = 0
				m.subview = svLndHubManage
				m.addonBtnIdx = 0
				m.addonFocus = 0
				m.openFlowTab(tabLndHub,
					"LndHub", secAddons)
			} else if m.cfg.HasLND() &&
				m.cfg.WalletExists() {
				m.shellAction = svLndHubInstall
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

// ── System home ──────────────────────────────────────────

func (m Model) handleSystemHomeKey(
	key string,
) (tea.Model, tea.Cmd) {
	hasUpdate := m.latestVersion != "" &&
		m.latestVersion != m.version

	maxBtn := 1
	if m.status != nil && m.status.rebootRequired {
		maxBtn = 2
	}

	switch key {
	case "left", "h":
		if m.contentFocus == 1 && m.btnIdx > 0 {
			m.btnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "up", "k":
		if m.contentFocus == 1 {
			m.contentFocus = 0
			if m.svcCursor >= m.svcCount() {
				m.svcCursor = m.svcCount() - 1
			}
			if m.svcCursor < 0 {
				m.svcCursor = 0
			}
		} else if m.contentFocus == 0 {
			if m.svcCursor > 0 {
				m.svcCursor--
			} else if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = 1
				return m, nil
			}
		}
	case "down", "j":
		if m.contentFocus == 0 {
			if m.svcCursor < m.svcCount()-1 {
				m.svcCursor++
			} else {
				m.contentFocus = 1
				m.btnIdx = 0
			}
		}
	case "right", "l":
		if m.contentFocus == 1 {
			if m.btnIdx < maxBtn {
				m.btnIdx++
			}
		}
	case "r":
		if m.contentFocus == 0 {
			m.svcConfirm = "Restart"
		}
	case "s":
		if m.contentFocus == 0 {
			m.svcConfirm = "Stop"
		}
	case "a":
		if m.contentFocus == 0 {
			m.svcConfirm = "Start"
		}
	case "enter":
		if m.contentFocus == 1 {
			switch m.btnIdx {
			case 0:
				m.sysConfirm = "Update packages"
			case 1:
				if hasUpdate {
					m.updateConfirm = true
				}
			case 2:
				m.sysConfirm = "Reboot"
			}
		}
	}
	return m, nil
}

// ── On-chain overview keys (used by on-chain TAB) ────────

func (m Model) handleOnChainKey(
	key string,
) (tea.Model, tea.Cmd) {
	if m.subview == svOnChainResult {
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter", "backspace":
			m.subview = svOnChain
			m.onChainSendTxid = ""
			m.onChainSendError = ""
			return m, tea.Sequence(
				listUnspentCmd(m.lndClient),
				fetchOnChainTxCmd(m.lndClient),
				fetchStatus(m.cfg, m.lndClient))
		}
		return m, nil
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.onChainTxFocus == 0 &&
			m.onChainBtnIdx > 0 {
			m.onChainBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "backspace":
		return m.closeTab(m.activeTab)
	case "up", "k":
		switch m.onChainTxFocus {
		case 2:
			if m.onChainTxCursor > 0 {
				m.onChainTxCursor--
			} else {
				m.onChainTxFocus = 1
				if len(m.utxos) > 0 {
					m.utxoCursor =
						len(m.utxos) - 1
				}
			}
		case 1:
			if m.utxoCursor > 0 {
				m.utxoCursor--
			} else {
				m.onChainTxFocus = 0
				m.onChainBtnIdx = 0
			}
		case 0:
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = m.findFlowTab()
				return m, nil
			}
		}
	case "down", "j":
		switch m.onChainTxFocus {
		case 0:
			if len(m.utxos) > 0 {
				m.onChainTxFocus = 1
				m.utxoCursor = 0
			} else if len(m.onChainTxs) > 0 {
				m.onChainTxFocus = 2
				m.onChainTxCursor = 0
			}
		case 1:
			if m.utxoCursor < len(m.utxos)-1 {
				m.utxoCursor++
			} else if len(m.onChainTxs) > 0 {
				m.onChainTxFocus = 2
				m.onChainTxCursor = 0
			}
		case 2:
			if m.onChainTxCursor < len(m.onChainTxs)-1 {
				m.onChainTxCursor++
			}
		}
	case "right", "l":
		if m.onChainTxFocus == 0 &&
			m.onChainBtnIdx < 1 {
			m.onChainBtnIdx++
		}
	case "space":
		if m.onChainTxFocus == 1 &&
			m.utxoCursor < len(m.utxos) {
			m.toggleUtxoSelection(m.utxoCursor)
		}
		return m, nil
	case "enter":
		if m.onChainTxFocus == 0 {
			switch m.onChainBtnIdx {
			case 0:
				m.ocRecvAddress = ""
				m.ocRecvBtnIdx = 0
				m.ocRecvError = ""
				m.subview = svOnChainReceive
				m.openFlowTab(tabOCReceive,
					"⛓ Receive", secOnChain)
				return m,
					getNewAddressCmd(m.lndClient)
			case 1:
				m.resetOnChainSendState()
				if len(m.utxoSelected) > 0 {
					m.ocSendAmtInput.SetValue(
						fmt.Sprintf("%d",
							m.utxoSelectedTotal))
				}
				m.subview = svOnChainSend
				return m, nil
			}
		} else if m.onChainTxFocus == 1 &&
			m.utxoCursor < len(m.utxos) {
			u := m.utxos[m.utxoCursor]
			label := u.Address
			if len(label) > 14 {
				label = label[:12] + ".."
			}
			m.utxoLabelEditing = false
			m.contentFocus = 0
			m.findOrOpenTab(tabUtxoDetail, label,
				m.utxoCursor, secOnChain)
		} else if m.onChainTxFocus == 2 &&
			m.onChainTxCursor < len(m.onChainTxs) {
			tx := m.onChainTxs[m.onChainTxCursor]
			label := tx.Label
			if len(label) > 14 {
				label = label[:12] + ".."
			}
			m.findOrOpenTab(tabOnChainTx, label,
				m.onChainTxCursor, secOnChain)
		}
	}
	return m, nil
}

// ── Syncthing detail keys (with addonFocus) ──────────────

func (m Model) handleSyncDetailKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.addonFocus == 0 && m.addonBtnIdx > 0 {
			m.addonBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "right", "l":
		if m.addonFocus == 0 && m.addonBtnIdx < 2 {
			m.addonBtnIdx++
		}
	case "up", "k":
		if m.addonFocus == 1 {
			if m.syncCursor > 0 {
				m.syncCursor--
			} else {
				m.addonFocus = 0
				m.addonBtnIdx = 0
			}
		} else if m.addonFocus == 0 {
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = m.findFlowTab()
				return m, nil
			}
		}
	case "down", "j":
		if m.addonFocus == 0 {
			if len(m.cfg.SyncthingDevices) > 0 {
				m.addonFocus = 1
				m.syncCursor = 0
			}
		} else if m.addonFocus == 1 {
			if m.syncCursor <
				len(m.cfg.SyncthingDevices)-1 {
				m.syncCursor++
			}
		}
	case "enter":
		if m.addonFocus == 0 {
			switch m.addonBtnIdx {
			case 0:
				m.subview = svSyncthingPairInput
				m.syncPairError = ""
				m.syncPairSuccess = false
			case 1:
				m.subview = svSyncthingDeviceQR
			case 2:
				m.subview = svSyncthingWebUI
				m.addonBtnIdx = 0
			}
		} else if m.addonFocus == 1 {
			if m.syncCursor <
				len(m.cfg.SyncthingDevices) {
				m.subview = svSyncthingDeviceDetail
			}
		}
	case "backspace":
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

// ── LndHub manage keys (with addonFocus) ─────────────────

func (m Model) handleLndhubManageKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.addonFocus == 0 && m.addonBtnIdx > 0 {
			m.addonBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "right", "l":
		if m.addonFocus == 0 && m.addonBtnIdx < 2 {
			m.addonBtnIdx++
		}
	case "up", "k":
		if m.addonFocus == 1 {
			if m.hubCursor > 0 {
				m.hubCursor--
			} else {
				m.addonFocus = 0
				m.addonBtnIdx = 0
			}
		} else if m.addonFocus == 0 {
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = m.findFlowTab()
				return m, nil
			}
		}
	case "down", "j":
		if m.addonFocus == 0 {
			if len(m.cfg.LndHubAccounts) > 0 {
				m.addonFocus = 1
				m.hubCursor = 0
			}
		} else if m.addonFocus == 1 {
			if m.hubCursor <
				len(m.cfg.LndHubAccounts)-1 {
				m.hubCursor++
			}
		}
	case "enter":
		if m.addonFocus == 0 {
			switch m.addonBtnIdx {
			case 0:
				m.hubNameInput = newHubNameInput()
				m.subview = svLndHubCreateName
			case 1:
				if m.hubCursor <
					len(m.cfg.LndHubAccounts) {
					m.subview = svLndHubAccountDetail
				}
			case 2:
				if m.hubCursor <
					len(m.cfg.LndHubAccounts) &&
					m.cfg.LndHubAccounts[m.hubCursor].
						Active {
					m.subview =
						svLndHubDeactivateConfirm
				}
			}
		} else if m.addonFocus == 1 {
			if m.hubCursor <
				len(m.cfg.LndHubAccounts) {
				m.subview = svLndHubAccountDetail
			}
		}
	case "backspace":
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

// ── Sidebar keys ─────────────────────────────────────────

func (m Model) handleSidebarKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.nav.Cursor == 0 && m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			if m.activeTab < 1 {
				m.activeTab = 1
			}
			return m, nil
		}
		m.nav.MoveUp()
		return m, nil
	case "down", "j":
		m.nav.MoveDown()
		return m, nil
	case "enter", "right", "l":
		sec := m.nav.Activate()
		m.focusContent()
		m.activeTab = 0
		m.subview = svNone
		m.btnIdx = 0
		m.contentFocus = 0
		m.tabFocused = false
		m.tabCursorX = 0
		m.ensureContentCursor()
		return m.previewSection(sec)
	}
	return m, nil
}

func (m *Model) ensureContentCursor() {
	sec := m.nav.ActiveSection()
	switch sec {
	case secChannels:
		if m.status != nil &&
			len(m.status.channels) > 0 {
			if m.chanCursor >=
				len(m.status.channels) {
				m.chanCursor = 0
			}
		}
	case secWallet:
	case secOnChain:
	case secAddons:
	case secSystem:
	}
}

func (m Model) previewSection(
	sec int,
) (tea.Model, tea.Cmd) {
	switch sec {
	case secWallet:
		m.rebuildTxTable()
		return m,
			fetchPaymentHistoryCmd(m.lndClient)
	case secOnChain:
		return m, tea.Batch(
			listUnspentCmd(m.lndClient),
			fetchOnChainTxCmd(m.lndClient))
	case secChannels:
		m.rebuildChannelTable()
	}
	return m, nil
}

// ── Tab bar keys ─────────────────────────────────────────

func (m Model) handleTabBarKey(
	key string,
) (tea.Model, tea.Cmd) {
	tabs := m.effectiveTabs()

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "down", "j":
		m.focusContent()
		m.contentFocus = 0
		if m.activeTab == 0 {
			m.subview = svNone
		}
		m.ensureContentCursor()
		return m, nil

	case "left", "h":
		if m.tabCursorX == 1 {
			m.tabCursorX = 0
			return m, nil
		}
		if m.activeTab > 1 {
			m.activeTab--
			m.tabCursorX = 0
			if m.activeTab-1 < m.tabScrollOffset {
				m.tabScrollOffset = m.activeTab - 1
				if m.tabScrollOffset < 0 {
					m.tabScrollOffset = 0
				}
			}
			return m, nil
		}
		m.focusSidebar()
		m.contentFocus = 0
		m.activeTab = 0
		m.tabScrollOffset = 0
		return m, nil

	case "right", "l":
		if m.activeTab > 0 &&
			m.activeTab < len(tabs) {
			tab := tabs[m.activeTab]
			if tab.Kind != tabMain &&
				m.tabCursorX == 0 {
				m.tabCursorX = 1
				return m, nil
			}
		}
		if m.activeTab < len(tabs)-1 {
			m.activeTab++
			m.tabCursorX = 0
			return m, nil
		}

	case "enter":
		if m.tabCursorX == 1 && m.activeTab > 0 {
			return m.closeTab(m.activeTab)
		}
		m.focusContent()
		m.contentFocus = 0
		tab := tabs[m.activeTab]
		if tab.Kind == tabMain {
			m.subview = svNone
		}
		m.ensureContentCursor()
		return m, nil

	case "backspace":
		m.focusSidebar()
		m.activeTab = 0
		return m, nil
	}
	return m, nil
}

func (m Model) closeTab(
	tabIdx int,
) (tea.Model, tea.Cmd) {
	tabs := m.effectiveTabs()
	if tabIdx <= 0 || tabIdx >= len(tabs) {
		return m, nil
	}

	closingTab := tabs[tabIdx]

	switch closingTab.Kind {
	case tabOpenChannel:
		m.subview = svNone
		m.chanOpenError = ""
		m.chanOpenTxid = ""
		m.chanOpenInFlight = false
	case tabChannelHistory:
		m.subview = svNone
	case tabUtxoDetail:
		m.subview = svNone
		m.utxoLabelEditing = false
	case tabSend:
		m.resetSendState()
		m.subview = svNone
	case tabReceive:
		m.resetReceiveState()
		m.subview = svNone
	case tabOnChain:
		m.resetOnChainSendState()
		m.subview = svNone
	case tabOCReceive:
		m.ocRecvAddress = ""
		m.ocRecvError = ""
		m.subview = svNone
	case tabPairing:
		m.subview = svNone
	case tabSyncthing:
		m.subview = svNone
		m.addonFocus = 0
	case tabLndHub:
		m.subview = svNone
		m.addonFocus = 0
	}

	var newTabs []openTab
	for _, t := range m.tabs {
		if t.Kind == closingTab.Kind &&
			t.Index == closingTab.Index &&
			t.Section == closingTab.Section {
			continue
		}
		newTabs = append(newTabs, t)
	}
	m.tabs = newTabs

	if m.activeTab >= tabIdx {
		m.activeTab--
		if m.activeTab < 0 {
			m.activeTab = 0
		}
	}
	m.tabCursorX = 0

	m.focusContent()
	m.contentFocus = 0
	m.activeTab = 0
	m.ensureContentCursor()

	if m.tabScrollOffset >
		len(m.effectiveTabs())-2 {
		m.tabScrollOffset =
			len(m.effectiveTabs()) - 2
		if m.tabScrollOffset < 0 {
			m.tabScrollOffset = 0
		}
	}

	return m, nil
}

// ── Confirm keys ─────────────────────────────────────────

func (m Model) handleSvcConfirmKey(
	key string,
) (tea.Model, tea.Cmd) {
	action := m.svcConfirm
	m.svcConfirm = ""
	if key == "y" {
		svc := m.svcName(m.svcCursor)
		if svc != "" {
			return m, runSvcActionCmd(action, svc)
		}
	}
	return m, nil
}

func (m Model) handleSysConfirmKey(
	key string,
) (tea.Model, tea.Cmd) {
	action := m.sysConfirm
	m.sysConfirm = ""
	if key == "y" {
		if action == "Reboot" {
			return m, runRebootCmd()
		}
		return m, runUpdatePackagesCmd()
	}
	return m, nil
}

func (m Model) handleUpdateConfirmKey(
	key string,
) (tea.Model, tea.Cmd) {
	m.updateConfirm = false
	if key == "y" {
		m.shellAction = svSelfUpdate
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleGenericSubviewKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if m.urlReturnTo != svNone {
			m.subview = m.urlReturnTo
			m.urlReturnTo = svNone
			return m, nil
		}
		m.subview = svNone
		m.btnIdx = 0
		return m, nil
	}
	return m, nil
}

// ── View-only tab handler (shared) ──────────────────────

func (m Model) handleViewOnlyKey(
	key string, backSubview wSubview,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
	case "backspace":
		if backSubview != svNone {
			m.subview = backSubview
			return m, nil
		}
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

// ── findOrOpenTab (shared) ──────────────────────────────

func (m *Model) findOrOpenTab(
	kind tabKind, label string,
	index, section int,
) {
	tabs := m.effectiveTabs()
	for i, t := range tabs {
		if t.Kind == kind && t.Index == index {
			m.activeTab = i
			m.focusTabBar()
			m.tabCursorX = 0
			return
		}
	}
	m.tabs = append(m.tabs, openTab{
		Kind: kind, Label: label,
		Index: index, Section: section,
	})
	m.activeTab = len(m.effectiveTabs()) - 1
	m.focusTabBar()
	m.tabCursorX = 0
}

// ── Channel open entry ───────────────────────────────────

func (m Model) startChannelOpenCmd() (
	tea.Model, tea.Cmd,
) {
	if m.lndClient == nil || !m.cfg.HasLND() ||
		!m.cfg.WalletExists() {
		return m, nil
	}
	if m.status != nil &&
		m.status.lndBalance == "0" {
		m.subview = svChannelFundWallet
		m.openFlowTab(tabOpenChannel,
			"Open Channel", secChannels)
		return m, getNewAddressCmd(m.lndClient)
	}
	m.chanPeerList = curatedPeers()
	m.chanOpenPeerIdx = 0
	m.chanOpenError = ""
	m.subview = svChannelOpen
	m.openFlowTab(tabOpenChannel,
		"Open Channel", secChannels)
	return m, nil
}

func (m *Model) openFlowTab(
	kind tabKind, label string, section int,
) {
	tabs := m.effectiveTabs()
	for i, t := range tabs {
		if t.Kind == kind && t.Section == section {
			m.activeTab = i
			m.focusContent()
			m.contentFocus = 0
			return
		}
	}
	m.tabs = append(m.tabs, openTab{
		Kind:    kind,
		Label:   label,
		Section: section,
	})
	m.activeTab = len(m.effectiveTabs()) - 1
	m.focusContent()
	m.contentFocus = 0
}

func (m Model) findFlowTab() int {
	tabs := m.effectiveTabs()
	var kind tabKind
	switch {
	case m.subview == svSend ||
		m.subview == svSendConfirm ||
		m.subview == svSendInFlight ||
		m.subview == svSendResult:
		kind = tabSend
	case m.subview == svReceive ||
		m.subview == svReceiveWaiting ||
		m.subview == svReceivePaid ||
		m.subview == svReceiveExpired:
		kind = tabReceive
	case m.subview == svWalletPairing:
		kind = tabPairing
	case m.subview == svOnChainReceive:
		kind = tabOCReceive
	case isOnChainSubview(m.subview):
		kind = tabOnChain
	case isChannelSubview(m.subview):
		kind = tabOpenChannel
	case m.subview == svSyncthingDetail ||
		m.subview == svSyncthingPairInput ||
		m.subview == svSyncthingDeviceDetail ||
		m.subview == svSyncthingWebUI ||
		m.subview == svSyncthingDeviceQR:
		kind = tabSyncthing
	case m.subview == svLndHubManage ||
		m.subview == svLndHubCreateName ||
		m.subview == svLndHubCreateAccount ||
		m.subview == svLndHubAccountDetail ||
		m.subview == svLndHubDeactivateConfirm:
		kind = tabLndHub
	default:
		return m.activeTab
	}
	for i, t := range tabs {
		if t.Kind == kind {
			return i
		}
	}
	return m.activeTab
}

func isChannelSubview(sv wSubview) bool {
	switch sv {
	case svChannelOpen, svChannelCustomPeer,
		svChannelAmountSelect, svChannelOpenConfirm,
		svChannelOpening, svChannelOpenResult,
		svChannelFundWallet:
		return true
	}
	return false
}

func isCloseSubview(sv wSubview) bool {
	switch sv {
	case svCloseType, svCloseConfirm,
		svClosing, svCloseResult:
		return true
	}
	return false
}

func isOnChainSubview(sv wSubview) bool {
	switch sv {
	case svOnChain, svOnChainResult,
		svOnChainSend, svOCSendConfirm,
		svOCSendBroadcast,
		svOnChainReceive:
		return true
	}
	return false
}

// ── Shell commands ───────────────────────────────────────

func showMacaroonCmd(cfg *config.AppConfig) tea.Cmd {
	mac := readMacaroonHex(cfg)
	if mac == "" {
		return nil
	}
	tmpFile, err := os.CreateTemp("", "rlvpn-macaroon-")
	if err != nil {
		return nil
	}
	tmpPath := tmpFile.Name()
	_, _ = tmpFile.WriteString(mac)
	_ = tmpFile.Close()
	c := exec.Command("bash", "-c",
		"clear && echo && cat "+tmpPath+
			" && echo && echo && echo "+
			"'  Press Enter...' && read && rm -f "+
			tmpPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		_ = os.Remove(tmpPath)
		return svcActionDoneMsg{}
	})
}

func runSvcActionCmd(action, svc string) tea.Cmd {
	var verb string
	switch action {
	case "Restart":
		verb = "restart"
	case "Stop":
		verb = "stop"
	case "Start":
		verb = "start"
	default:
		return nil
	}
	c := exec.Command("sudo", "systemctl", verb, svc)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return svcActionDoneMsg{}
	})
}

func runUpdatePackagesCmd() tea.Cmd {
	c := exec.Command("bash", "-c",
		"sudo apt-get update && "+
			"sudo apt-get upgrade -y")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return svcActionDoneMsg{}
	})
}

func runRebootCmd() tea.Cmd {
	c := exec.Command("sudo", "reboot")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return svcActionDoneMsg{}
	})
}

// ── Paste handling ───────────────────────────────────────

func (m Model) handlePaste(
	msg tea.PasteMsg,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svSend:
		var cmd tea.Cmd
		m.sendInput, cmd = m.sendInput.Update(msg)
		return m, cmd
	case svReceive:
		var cmd tea.Cmd
		if m.recvAmountInput.Focused() {
			m.recvAmountInput, cmd =
				m.recvAmountInput.Update(msg)
		} else {
			m.recvMemoInput, cmd =
				m.recvMemoInput.Update(msg)
		}
		return m, cmd
	case svChannelCustomPeer:
		var cmd tea.Cmd
		if m.chanPubkeyInput.Focused() {
			m.chanPubkeyInput, cmd =
				m.chanPubkeyInput.Update(msg)
		} else {
			m.chanHostInput, cmd =
				m.chanHostInput.Update(msg)
		}
		return m, cmd
	case svChannelAmountSelect:
		if m.chanAmountPreset ==
			len(amountPresets)-1 {
			var cmd tea.Cmd
			m.chanAmountInput, cmd =
				m.chanAmountInput.Update(msg)
			return m, cmd
		}
	case svLndHubCreateName:
		var cmd tea.Cmd
		m.hubNameInput, cmd =
			m.hubNameInput.Update(msg)
		return m, cmd
	case svSyncthingPairInput:
		var cmd tea.Cmd
		m.syncDeviceInput, cmd =
			m.syncDeviceInput.Update(msg)
		return m, cmd
	case svOnChainSend:
		var cmd tea.Cmd
		if m.ocSendStep == 0 {
			m.ocSendAddrInput, cmd =
				m.ocSendAddrInput.Update(msg)
		} else if m.ocSendStep == 1 && !m.ocSendAll {
			m.ocSendAmtInput, cmd =
				m.ocSendAmtInput.Update(msg)
		} else if m.ocSendStep == 4 {
			m.ocCustomFeeInput, cmd =
				m.ocCustomFeeInput.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

// Unused import guard
var _ = theme.Value
var _ = strings.TrimSpace
var _ = logger.TUI
