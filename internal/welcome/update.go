package welcome

import (
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/logger"
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
			logger.TUI("Warning: LndHub create failed: %v",
				msg.err)
			m.subview = svLndHubManage
			return m, nil
		}
		if msg.account != nil {
			m.lastAccount = msg.account
			m.cfg.LndHubAccounts = append(
				m.cfg.LndHubAccounts,
				config.LndHubAccount{
					Label:     m.hubNameInput.Value(),
					Login:     msg.account.Login,
					CreatedAt: time.Now().Format("2006-01-02"),
					Active:    true,
				})
			m.saveCfg()
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
		}
		m.subview = svOnChainResult
		return m, nil
	case feeTiersMsg:
		if msg.err == nil {
			m.ocFeeTiers = msg.tiers
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

// ── Key dispatch (restructured: tab-first) ───────────────

func (m Model) handleKey(
	msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+z" {
		return m, tea.Suspend
	}

	// ── 1. Confirm dialogs (modal, highest priority)
	if m.svcConfirm != "" {
		return m.handleSvcConfirmKey(key)
	}
	if m.sysConfirm != "" {
		return m.handleSysConfirmKey(key)
	}
	if m.updateConfirm {
		return m.handleUpdateConfirmKey(key)
	}

	// ── 2. Fullscreen views (QR, URL)
	if m.subview == svQR || m.subview == svFullURL ||
		m.subview == svSyncthingDeviceQR {
		return m.handleGenericSubviewKey(key)
	}

	// ── 3. Tab bar focused
	if m.tabFocused {
		return m.handleTabBarKey(key)
	}

	// ── 4. Sidebar focused
	if m.nav.Focused {
		return m.handleSidebarKey(key)
	}

	// ── 5. Content focused — dispatch by active tab
	tabs := m.effectiveTabs()
	if m.activeTab > 0 && m.activeTab < len(tabs) {
		tab := tabs[m.activeTab]
		return m.handleTabContentKey(tab, key, msg)
	}

	// ── 6. Section home (activeTab == 0)
	return m.handleSectionHomeKey(key, msg)
}

// ── Tab content dispatch ─────────────────────────────────
// Routes to the correct handler based on tab kind.
// Flow tabs dispatch internally by subview.
// View-only tabs get a simple handler.

func (m Model) handleTabContentKey(
	tab openTab, key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch tab.Kind {
	// View-only tabs
	case tabChannel:
		return m.handleChannelDetailKey(key)
	case tabPayment:
		return m.handlePaymentDetailKey(key)
	case tabOnChainTx:
		return m.handleOnChainTxDetailKey(key)

	// Flow tabs
	case tabOpenChannel:
		return m.handleOpenChannelTabKey(key, msg)
	case tabSend:
		return m.handleSendTabKey(key, msg)
	case tabReceive:
		return m.handleReceiveTabKey(key, msg)
	case tabOnChain:
		return m.handleOnChainTabKey(key, msg)
	case tabPairing:
		return m.handlePairingTabKey(key)
	case tabSyncthing:
		return m.handleSyncthingTabKey(key, msg)
	case tabLndHub:
		return m.handleLndHubTabKey(key, msg)
	}

	// Fallback: section home
	return m.handleSectionHomeKey(key, msg)
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
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			return m, nil
		}
	case "backspace":
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

func (m Model) handlePaymentDetailKey(
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
			return m, nil
		}
	case "backspace":
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

func (m Model) handleOnChainTxDetailKey(
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
			return m, nil
		}
	case "backspace":
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

// ── Flow tab handlers ────────────────────────────────────
// Each flow tab owns its subview dispatch internally.
// Backspace on step 1 closes the tab.

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
		// Tab exists but subview is svNone —
		// shouldn't happen, close tab
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
	case svOnChainSendAddr:
		return m.handleOCSendAddrKey(key, msg)
	case svOnChainSendAmount:
		return m.handleOCSendAmountKey(key, msg)
	case svOnChainSendConfirm:
		return m.handleOCSendConfirmKey(key)
	case svOnChainSendBroadcast:
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
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		m.subview = svSyncthingDetail
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
	}
	return m, nil
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
		if key == "enter" || key == "backspace" {
			m.subview = svLndHubManage
		}
		return m, nil
	case svLndHubAccountDetail:
		return m.handleLndHubAccountDetailKey(key)
	case svLndHubDeactivateConfirm:
		return m.handleLndHubDeactivateKey(key)
	default:
		return m.closeTab(m.activeTab)
	}
}

func (m Model) handleLndHubAccountDetailKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		m.subview = svLndHubManage
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
	}
	return m, nil
}

func (m Model) handleLndHubDeactivateKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "y":
		if m.hubCursor < len(m.cfg.LndHubAccounts) {
			login := m.cfg.LndHubAccounts[m.hubCursor].Login
			return m, deactivateLndHubAccountCmd(login)
		}
	case "n", "backspace":
		m.subview = svLndHubManage
	}
	return m, nil
}

// ── Section home key dispatch ────────────────────────────
// Only reached when activeTab == 0 (no tab selected)

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
			}
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
					label = ch.RemotePubkey[:12] + ".."
				}
				if len(label) > 17 {
					label = label[:17] + "..."
				}
				newTab := openTab{
					Kind:    tabChannel,
					Label:   label,
					Index:   m.chanCursor,
					Section: secChannels,
				}
				found := false
				tabs := m.effectiveTabs()
				for i, t := range tabs {
					if t.Kind == tabChannel &&
						t.Index == m.chanCursor {
						m.activeTab = i
						found = true
						break
					}
				}
				if !found {
					m.tabs = append(m.tabs, newTab)
					m.activeTab =
						len(m.effectiveTabs()) - 1
				}
				m.focusTabBar()
				m.tabCursorX = 0
			}
		} else if m.contentFocus == 1 {
			return m.startChannelOpenCmd()
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
			if m.btnIdx < 3 {
				m.btnIdx++
			}
		}
	case "enter":
		if m.contentFocus == 0 &&
			len(m.payHistory) > 0 {
			entry := m.payHistory[m.payHistoryCursor]
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
			newTab := openTab{
				Kind:    tabPayment,
				Label:   label,
				Index:   m.payHistoryCursor,
				Section: secWallet,
			}
			found := false
			tabs := m.effectiveTabs()
			for i, t := range tabs {
				if t.Kind == tabPayment &&
					t.Index == m.payHistoryCursor {
					m.activeTab = i
					found = true
					break
				}
			}
			if !found {
				m.tabs = append(m.tabs, newTab)
				m.activeTab =
					len(m.effectiveTabs()) - 1
			}
			m.focusTabBar()
			m.tabCursorX = 0
		} else if m.contentFocus == 1 {
			switch m.btnIdx {
			case 0: // Send
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
						"Send", secWallet)
				}
			case 1: // Receive
				if m.cfg.HasLND() &&
					m.cfg.WalletExists() {
					m.resetReceiveState()
					m.subview = svReceive
					m.openFlowTab(tabReceive,
						"Receive", secWallet)
				}
			case 2: // On-Chain
				m.subview = svOnChain
				m.onChainTxFocus = 0
				m.onChainBtnIdx = 0
				m.openFlowTab(tabOnChain,
					"On-Chain", secWallet)
				return m, tea.Batch(
					listUnspentCmd(m.lndClient),
					fetchOnChainTxCmd(m.lndClient))
			case 3: // Pairing
				m.pairingButtonIdx = 0
				m.subview = svWalletPairing
				m.openFlowTab(tabPairing,
					"Pairing", secWallet)
			}
		}
	}
	return m, nil
}

// ── Addons home ──────────────────────────────────────────

func (m Model) handleAddonsHomeKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		if m.btnIdx > 0 {
			m.btnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = 1
			return m, nil
		}
	case "right", "l":
		if m.btnIdx < 1 {
			m.btnIdx++
		}
	case "enter":
		switch m.btnIdx {
		case 0:
			if m.cfg.SyncthingInstalled {
				m.subview = svSyncthingDetail
				m.addonBtnIdx = 0
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
	maxBtn := 1
	if m.status != nil && m.status.rebootRequired {
		maxBtn = 2
	}

	switch key {
	case "left", "h":
		if m.btnIdx > 0 {
			m.btnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "up", "k":
		if m.svcCursor > 0 {
			m.svcCursor--
		} else if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = 1
			return m, nil
		}
	case "down", "j":
		if m.svcCursor < m.svcCount()-1 {
			m.svcCursor++
		}
	case "right", "l":
		if m.btnIdx < maxBtn {
			m.btnIdx++
		}
	case "r":
		m.svcConfirm = "Restart"
	case "s":
		m.svcConfirm = "Stop"
	case "a":
		m.svcConfirm = "Start"
	case "enter":
		switch m.btnIdx {
		case 0:
			if m.latestVersion != "" &&
				m.latestVersion != m.version {
				m.updateConfirm = true
			}
		case 1:
			m.sysConfirm = "Update packages"
		case 2:
			m.sysConfirm = "Reboot"
		}
	}
	return m, nil
}

// ── On-chain overview keys ───────────────────────────────

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
		// Close on-chain tab
		return m.closeTab(m.activeTab)
	case "up", "k":
		switch m.onChainTxFocus {
		case 2:
			if m.utxoCursor > 0 {
				m.utxoCursor--
			} else {
				m.onChainTxFocus = 1
				if len(m.onChainTxs) > 0 {
					m.onChainTxCursor =
						len(m.onChainTxs) - 1
				}
			}
		case 1:
			if m.onChainTxCursor > 0 {
				m.onChainTxCursor--
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
			if len(m.onChainTxs) > 0 {
				m.onChainTxFocus = 1
				m.onChainTxCursor = 0
			} else if len(m.utxos) > 0 {
				m.onChainTxFocus = 2
				m.utxoCursor = 0
			}
		case 1:
			if m.onChainTxCursor <
				len(m.onChainTxs)-1 {
				m.onChainTxCursor++
			} else if len(m.utxos) > 0 {
				m.onChainTxFocus = 2
				m.utxoCursor = 0
			}
		case 2:
			if m.utxoCursor < len(m.utxos)-1 {
				m.utxoCursor++
			}
		}
	case "right", "l":
		if m.onChainTxFocus == 0 &&
			m.onChainBtnIdx < 2 {
			m.onChainBtnIdx++
		}
	case "enter":
		if m.onChainTxFocus == 0 {
			switch m.onChainBtnIdx {
			case 0:
				return m, getNewAddressCmd(m.lndClient)
			case 1:
				return m, tea.Batch(
					listUnspentCmd(m.lndClient),
					fetchOnChainTxCmd(m.lndClient))
			case 2:
				m.resetOnChainSendState()
				m.subview = svOnChainSendAddr
				return m, nil
			}
		} else if m.onChainTxFocus == 1 &&
			m.onChainTxCursor < len(m.onChainTxs) {
			tx := m.onChainTxs[m.onChainTxCursor]
			label := tx.Label
			if len(label) > 14 {
				label = label[:12] + ".."
			}
			newTab := openTab{
				Kind:    tabOnChainTx,
				Label:   label,
				Index:   m.onChainTxCursor,
				Section: secWallet,
			}
			found := false
			tabs := m.effectiveTabs()
			for i, t := range tabs {
				if t.Kind == tabOnChainTx &&
					t.Index == m.onChainTxCursor {
					m.activeTab = i
					found = true
					break
				}
			}
			if !found {
				m.tabs = append(m.tabs, newTab)
				m.activeTab =
					len(m.effectiveTabs()) - 1
			}
			m.focusTabBar()
			m.tabCursorX = 0
		}
	}
	return m, nil
}

// ── On-chain send flow keys ──────────────────────────────

func (m Model) handleOCSendAddrKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
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
		return m, nil
	case "backspace":
		if m.ocSendAddrInput.Value() != "" {
			var cmd tea.Cmd
			m.ocSendAddrInput, cmd =
				m.ocSendAddrInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		m.subview = svOnChain
		m.onChainSendError = ""
		return m, nil
	case "enter":
		addr := strings.TrimSpace(
			m.ocSendAddrInput.Value())
		if addr == "" {
			m.onChainSendError = "Enter an address"
			return m, nil
		}
		if !isValidOnChainAddr(addr, m.cfg.Network) {
			m.onChainSendError = "Invalid address"
			return m, nil
		}
		m.ocSendAddrVal = addr
		m.onChainSendError = ""
		m.subview = svOnChainSendAmount
		m.ocSendStep = 0
		m.ocSendAll = false
		return m, fetchFeeTiersCmd(m.cfg)
	default:
		var cmd tea.Cmd
		m.ocSendAddrInput, cmd =
			m.ocSendAddrInput.Update(tea.Msg(msg))
		return m, cmd
	}
}

func (m Model) handleOCSendAmountKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.ocSendStep == 1 && m.ocSelectedTier > 0 {
			m.ocSelectedTier--
			return m, nil
		}
		m.focusSidebar()
		return m, nil
	case "backspace":
		if m.ocSendStep == 0 && !m.ocSendAll {
			if m.ocSendAmtInput.Value() != "" {
				var cmd tea.Cmd
				m.ocSendAmtInput, cmd =
					m.ocSendAmtInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		if m.ocSendStep == 2 {
			if m.ocCustomFeeInput.Value() != "" {
				var cmd tea.Cmd
				m.ocCustomFeeInput, cmd =
					m.ocCustomFeeInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		m.subview = svOnChainSendAddr
		m.onChainSendError = ""
		return m, nil
	case "tab":
		m.ocSendAll = !m.ocSendAll
		if m.ocSendAll {
			m.ocSendAmtInput.Blur()
		} else {
			m.ocSendAmtInput.Focus()
		}
		return m, nil
	case "up", "k":
		if m.ocSendStep == 1 {
			m.ocSendStep = 0
			if !m.ocSendAll {
				m.ocSendAmtInput.Focus()
			}
		} else if m.ocSendStep == 2 {
			m.ocSendStep = 1
			m.ocCustomFeeInput.Blur()
		} else if m.ocSendStep == 0 {
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = m.findFlowTab()
				return m, nil
			}
		}
		return m, nil
	case "down", "j":
		if m.ocSendStep == 0 {
			m.ocSendStep = 1
			m.ocSendAmtInput.Blur()
		} else if m.ocSendStep == 1 &&
			m.ocSelectedTier == 4 {
			m.ocSendStep = 2
			m.ocCustomFeeInput.Focus()
		}
		return m, nil
	case "right", "l":
		if m.ocSendStep == 1 && m.ocSelectedTier < 4 {
			m.ocSelectedTier++
		}
		return m, nil
	case "enter":
		var amountSats int64
		if m.ocSendAll {
			amountSats = 0
		} else {
			val := strings.TrimSpace(
				m.ocSendAmtInput.Value())
			val = strings.ReplaceAll(val, ",", "")
			if val == "" {
				m.onChainSendError = "Enter an amount"
				return m, nil
			}
			for _, c := range val {
				if c < '0' || c > '9' {
					m.onChainSendError =
						"Invalid number"
					return m, nil
				}
			}
			var n int64
			for _, c := range val {
				n = n*10 + int64(c-'0')
			}
			if n < 546 {
				m.onChainSendError =
					"Minimum 546 sats (dust limit)"
				return m, nil
			}
			amountSats = n
		}

		var feeRate int64
		if m.ocSelectedTier < 4 {
			tier := m.ocFeeTiers[m.ocSelectedTier]
			if tier.SatPerVB <= 0 {
				m.onChainSendError =
					"Fee estimate not available"
				return m, nil
			}
			feeRate = int64(tier.SatPerVB)
			if feeRate < 1 {
				feeRate = 1
			}
		} else {
			feeVal := strings.TrimSpace(
				m.ocCustomFeeInput.Value())
			if feeVal == "" {
				m.onChainSendError =
					"Enter a custom fee rate"
				return m, nil
			}
			var n int64
			for _, c := range feeVal {
				if c < '0' || c > '9' {
					m.onChainSendError =
						"Invalid fee rate"
					return m, nil
				}
				n = n*10 + int64(c-'0')
			}
			if n < 1 {
				m.onChainSendError =
					"Minimum 1 sat/vB"
				return m, nil
			}
			feeRate = n
		}

		m.ocSendAmtVal = amountSats
		m.ocSendFeeRate = feeRate
		m.onChainSendError = ""
		m.ocConfirmFee = 0
		m.subview = svOnChainSendConfirm

		if !m.ocSendAll && m.ocSendAddrVal != "" {
			target := int32(1)
			if m.ocSelectedTier < 4 {
				target = int32(
					m.ocFeeTiers[m.ocSelectedTier].
						Target)
			}
			return m, estimateTxFeeCmd(
				m.lndClient, m.ocSendAddrVal,
				amountSats, target)
		}
		return m, nil
	default:
		if m.ocSendStep == 0 && !m.ocSendAll {
			var cmd tea.Cmd
			m.ocSendAmtInput, cmd =
				m.ocSendAmtInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		if m.ocSendStep == 2 {
			var cmd tea.Cmd
			m.ocCustomFeeInput, cmd =
				m.ocCustomFeeInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) handleOCSendConfirmKey(
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
		return m, nil
	case "backspace":
		m.subview = svOnChainSendAmount
		m.onChainSendError = ""
		return m, nil
	case "y":
		m.onChainSendError = ""
		m.subview = svOnChainSendBroadcast
		return m, sendCoinsCmd(
			m.lndClient,
			m.ocSendAddrVal,
			m.ocSendAmtVal,
			m.ocSendFeeRate,
			m.ocSendAll)
	}
	return m, nil
}

func (m *Model) resetOnChainSendState() {
	m.ocSendAddrInput = newOnChainAddrInput()
	m.ocSendAmtInput = newOnChainAmtInput()
	m.ocCustomFeeInput = newCustomFeeInput()
	m.ocSendAll = false
	m.ocSendStep = 0
	m.ocFeeTiers = [4]feeTier{}
	m.ocSelectedTier = 0
	m.ocConfirmFee = 0
	m.ocSendAddrVal = ""
	m.ocSendAmtVal = 0
	m.ocSendFeeRate = 0
	m.onChainSendError = ""
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
			// At top section, jump to tab bar
			m.focusTabBar()
			m.tabCursorX = 0
			if m.activeTab < 1 {
				m.activeTab = 1
			}
			return m, nil
		}
		m.nav.MoveUp()
		sec := m.nav.Activate()
		m.btnIdx = 0
		m.subview = svNone
		m.activeTab = 0
		m.tabFocused = false
		m.tabCursorX = 0
		return m.previewSection(sec)
	case "down", "j":
		m.nav.MoveDown()
		sec := m.nav.Activate()
		m.btnIdx = 0
		m.subview = svNone
		m.activeTab = 0
		m.tabFocused = false
		m.tabCursorX = 0
		return m.previewSection(sec)
	case "enter", "right", "l":
		m.focusContent()
		m.activeTab = 0
		m.subview = svNone
		m.btnIdx = 0
		m.contentFocus = 0
		m.ensureContentCursor()
		return m, nil
	}
	return m, nil
}

func (m *Model) ensureContentCursor() {
	sec := m.nav.ActiveSection()
	switch sec {
	case secChannels:
		if m.status != nil &&
			len(m.status.channels) > 0 {
			if m.chanCursor >= len(m.status.channels) {
				m.chanCursor = 0
			}
		}
	case secWallet:
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
		return m, fetchPaymentHistoryCmd(m.lndClient)
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
		// Past first tab → sidebar
		m.focusSidebar()
		m.contentFocus = 0
		m.activeTab = 0
		m.tabScrollOffset = 0
		return m, nil

	case "right", "l":
		if m.activeTab > 0 && m.activeTab < len(tabs) {
			tab := tabs[m.activeTab]
			if tab.Kind != tabMain && m.tabCursorX == 0 {
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

	// Clean up flow state
	switch closingTab.Kind {
	case tabOpenChannel:
		m.subview = svNone
		m.chanOpenError = ""
		m.chanOpenTxid = ""
		m.chanOpenInFlight = false
	case tabSend:
		m.resetSendState()
		m.subview = svNone
	case tabReceive:
		m.resetReceiveState()
		m.subview = svNone
	case tabOnChain:
		m.resetOnChainSendState()
		m.subview = svNone
	case tabPairing:
		m.subview = svNone
	case tabSyncthing:
		m.subview = svNone
	case tabLndHub:
		m.subview = svNone
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

	// Return to section home
	m.focusContent()
	m.contentFocus = 0
	m.activeTab = 0
	m.ensureContentCursor()

	if m.tabScrollOffset > len(m.effectiveTabs())-2 {
		m.tabScrollOffset = len(m.effectiveTabs()) - 2
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

// ── Channel open entry ───────────────────────────────────

func (m Model) startChannelOpenCmd() (tea.Model, tea.Cmd) {
	if m.lndClient == nil || !m.cfg.HasLND() ||
		!m.cfg.WalletExists() {
		return m, nil
	}
	if m.status != nil && m.status.lndBalance == "0" {
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

// ── Subview classifiers (still used by findFlowTab) ──────

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

func isOnChainSubview(sv wSubview) bool {
	switch sv {
	case svOnChain, svOnChainResult,
		svOnChainSendAddr, svOnChainSendAmount,
		svOnChainSendConfirm, svOnChainSendBroadcast:
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
		if m.chanAmountPreset == len(amountPresets)-1 {
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
	case svOnChainSendAddr:
		var cmd tea.Cmd
		m.ocSendAddrInput, cmd =
			m.ocSendAddrInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// Unused import guard
var _ = theme.Value
var _ = strings.TrimSpace
var _ = logger.TUI
