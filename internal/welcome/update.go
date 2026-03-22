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
	case sendCoinsResultMsg:
		if msg.err != nil {
			m.onChainSendError = msg.err.Error()
		} else {
			m.onChainSendTxid = msg.txid
			m.onChainSendError = ""
		}
		m.subview = svOnChainResult
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

// ── Key dispatch ─────────────────────────────────────────

func (m Model) handleKey(
	msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+z" {
		return m, tea.Suspend
	}

	// Text input subviews
	if m.subview == svLndHubCreateName {
		return m.handleLndHubCreateNameKey(key, msg)
	}
	if m.subview == svSyncthingPairInput {
		return m.handleSyncthingPairInputKey(key, msg)
	}

	// Flow subviews: up goes to tab bar
	if isChannelSubview(m.subview) ||
		isWalletSubview(m.subview) {
		if key == "up" || key == "k" {
			if m.hasDetailTabs() {
				m.tabFocused = true
				m.contentFocused = false
				m.tabCursorX = 0
				// Find the tab for current flow
				m.activeTab = m.findFlowTab()
				return m, nil
			}
		}
	}

	// Channel open flow
	if isChannelSubview(m.subview) {
		return m.handleChannelsKey(key, msg)
	}
	// Wallet send/receive flow
	if isWalletSubview(m.subview) {
		return m.handleWalletKey(key, msg)
	}
	// QR/URL fullscreen
	if m.subview == svQR || m.subview == svFullURL {
		return m.handleGenericSubviewKey(key)
	}
	// Addon subviews
	if isAddonSubview(m.subview) {
		return m.handleAddonSubviewKey(key)
	}

	// Confirms
	if m.svcConfirm != "" {
		return m.handleSvcConfirmKey(key)
	}
	if m.sysConfirm != "" {
		return m.handleSysConfirmKey(key)
	}
	if m.updateConfirm {
		return m.handleUpdateConfirmKey(key)
	}

	// Generic subview back
	if m.subview != svNone {
		return m.handleGenericSubviewKey(key)
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	}

	if m.tabFocused {
		return m.handleTabBarKey(key)
	}
	if m.nav.Focused {
		return m.handleSidebarKey(key)
	}
	return m.handleContentKey(key)
}

func (m Model) handleOnChainKey(
	key string,
) (tea.Model, tea.Cmd) {
	if m.subview == svOnChainResult {
		switch key {
		case "enter", "backspace", "esc":
			m.subview = svOnChain
			m.onChainSendTxid = ""
			m.onChainSendError = ""
			return m, tea.Sequence(
				listUnspentCmd(m.lndClient),
				fetchStatus(m.cfg, m.lndClient))
		}
		return m, nil
	}

	switch key {
	case "backspace", "esc":
		m.subview = svNone
		m.contentFocus = 1
	case "up", "k":
		if m.onChainFocus == 1 && m.utxoCursor > 0 {
			m.utxoCursor--
		} else if m.onChainFocus == 1 {
			m.onChainFocus = 0
			m.onChainBtnIdx = 0
		}
	case "down", "j":
		if m.onChainFocus == 0 {
			m.onChainFocus = 1
			m.utxoCursor = 0
		} else if m.onChainFocus == 1 &&
			m.utxoCursor < len(m.utxos)-1 {
			m.utxoCursor++
		}
	case "left", "h":
		if m.onChainFocus == 0 && m.onChainBtnIdx > 0 {
			m.onChainBtnIdx--
		}
	case "right", "l":
		if m.onChainFocus == 0 && m.onChainBtnIdx < 1 {
			m.onChainBtnIdx++
		}
	case "enter":
		if m.onChainFocus == 0 {
			switch m.onChainBtnIdx {
			case 0: // New Address
				return m, getNewAddressCmd(m.lndClient)
			case 1: // Refresh UTXOs
				return m, listUnspentCmd(m.lndClient)
			}
		}
	}
	return m, nil
}

// ── Sidebar keys ─────────────────────────────────────────

func (m Model) handleSidebarKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
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
		m.nav.Blur()
		m.contentFocused = true
		m.btnIdx = 0
		m.contentFocus = 0
		// Ensure cursor is visible in content
		m.ensureContentCursor()
		return m, nil
	}
	return m, nil
}

// ensureContentCursor makes sure that when entering the
// content pane, the cursor lands on a visible element.
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
		// Cursor on tx table is fine
	case secAddons:
		// Cursor on buttons
	case secSystem:
		// Cursor on services list
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
	case "down", "j":
		// From any tab, go back to main content
		m.activeTab = 0
		m.tabFocused = false
		m.contentFocused = true
		m.contentFocus = 0
		m.subview = svNone
		m.nav.Blur()
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
			// Adjust scroll if needed
			if m.activeTab-1 < m.tabScrollOffset {
				m.tabScrollOffset = m.activeTab - 1
				if m.tabScrollOffset < 0 {
					m.tabScrollOffset = 0
				}
			}
			return m, nil
		}
		// On first detail tab or beyond, go to sidebar
		m.tabFocused = false
		m.contentFocused = false
		m.contentFocus = 0
		m.activeTab = 0
		m.tabScrollOffset = 0
		m.nav.Focus()
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
			// Scroll will auto-adjust in renderTabBar
			return m, nil
		}

	case "enter":
		if m.tabCursorX == 1 && m.activeTab > 0 {
			return m.closeTab(m.activeTab)
		}
		tab := tabs[m.activeTab]
		// Flow tabs (send/receive/pairing/onchain)
		// and main tab: enter focuses content
		if tab.Kind == tabMain ||
			tab.Kind == tabSend ||
			tab.Kind == tabReceive ||
			tab.Kind == tabPairing ||
			tab.Kind == tabOnChain {
			m.tabFocused = false
			m.contentFocused = true
			m.contentFocus = 0
			m.nav.Blur()
			m.ensureContentCursor()
		}
		// Channel/payment detail tabs are view-only
		return m, nil

	case "backspace":
		// Back to sidebar
		m.tabFocused = false
		m.contentFocused = false
		m.activeTab = 0
		m.nav.Focus()
		return m, nil

	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) closeTab(
	tabIdx int,
) (tea.Model, tea.Cmd) {
	tabs := m.effectiveTabs()
	if tabIdx <= 0 || tabIdx >= len(tabs) {
		// Reset scroll offset
		if m.tabScrollOffset > len(m.effectiveTabs())-2 {
			m.tabScrollOffset = len(m.effectiveTabs()) - 2
			if m.tabScrollOffset < 0 {
				m.tabScrollOffset = 0
			}
		}

		return m, nil
	}

	closingTab := tabs[tabIdx]
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

	// If no more detail tabs, leave tab bar
	if !m.hasDetailTabs() {
		m.tabFocused = false
		m.activeTab = 0
		// Focus content (not lost in tab bar)
		m.contentFocused = true
		m.contentFocus = 0
		m.nav.Blur()
		m.ensureContentCursor()
	}

	return m, nil
}

// ── Content keys ─────────────────────────────────────────

func (m Model) handleContentKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "backspace":
		if m.subview != svNone {
			m.subview = svNone
			m.btnIdx = 0
			m.contentFocus = 0
			return m, nil
		}
		// Back to sidebar — always lands on yellow item
		m.contentFocused = false
		m.contentFocus = 0
		m.tabFocused = false
		m.nav.Focus()
		return m, nil
	case "esc":
		if m.subview != svNone {
			m.subview = svNone
			m.btnIdx = 0
			m.contentFocus = 0
			return m, nil
		}
	case "left", "h":
		// If in a context where left should go to sidebar
		sec := m.nav.ActiveSection()
		if sec == secChannels && m.contentFocus == 0 {
			m.contentFocused = false
			m.contentFocus = 0
			m.nav.Focus()
			return m, nil
		}
		if sec == secWallet && m.contentFocus == 0 {
			m.contentFocused = false
			m.contentFocus = 0
			m.nav.Focus()
			return m, nil
		}
		if sec == secAddons && m.btnIdx == 0 {
			m.contentFocused = false
			m.nav.Focus()
			return m, nil
		}
		if sec == secSystem && m.btnIdx == 0 {
			m.contentFocused = false
			m.nav.Focus()
			return m, nil
		}
	case "q", "ctrl+c":
		return m, tea.Quit
	}

	sec := m.nav.ActiveSection()

	switch sec {
	case secChannels:
		return m.handleChannelsContentKey(key)
	case secWallet:
		return m.handleWalletContentKey(key)
	case secAddons:
		return m.handleAddonsContentKey(key)
	case secSystem:
		return m.handleSystemContentKey(key)
	}
	return m, nil
}

// ── Channels content ─────────────────────────────────────

func (m Model) handleChannelsContentKey(
	key string,
) (tea.Model, tea.Cmd) {
	if m.subview == svOnChain ||
		m.subview == svOnChainResult {
		return m.handleOnChainKey(key)
	}

	switch key {
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
				// At top channel → tab bar
				m.tabFocused = true
				m.contentFocused = false
				m.tabCursorX = 0
				// Start on first detail tab
				m.activeTab = 1
			}
			// If no tabs, stay at top channel
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
				m.tabFocused = true
				m.contentFocused = false
				m.tabCursorX = 0
			}
		} else if m.contentFocus == 1 {
			return m.startChannelOpenCmd()
		}
	}
	return m, nil
}

// ── Wallet content ───────────────────────────────────────

func (m Model) handleWalletContentKey(
	key string,
) (tea.Model, tea.Cmd) {
	if m.subview == svPaymentDetail {
		if key == "backspace" || key == "esc" {
			m.subview = svNone
			m.contentFocus = 0
		}
		return m, nil
	}
	if m.subview == svOnChain {
		if key == "backspace" || key == "esc" {
			m.subview = svNone
			m.contentFocus = 1
		}
		return m, nil
	}

	// contentFocus: 0 = tx table, 1 = buttons row (above)
	switch key {
	case "up", "k":
		if m.contentFocus == 0 {
			if m.payHistoryCursor > 0 {
				m.payHistoryCursor--
			} else {
				// At top of table → buttons row
				m.contentFocus = 1
				m.btnIdx = 0
			}
		}
		// On buttons, up does nothing (top of content)
	case "down", "j":
		if m.contentFocus == 1 {
			// From buttons → table
			m.contentFocus = 0
			m.payHistoryCursor = 0
		} else if m.contentFocus == 0 {
			if m.payHistoryCursor <
				len(m.payHistory)-1 {
				m.payHistoryCursor++
			}
		}
	case "left", "h":
		if m.contentFocus == 1 {
			if m.btnIdx > 0 {
				m.btnIdx--
			} else {
				// Back to sidebar from buttons
				m.contentFocused = false
				m.nav.Focus()
				return m, nil
			}
		} else {
			m.contentFocused = false
			m.nav.Focus()
			return m, nil
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
			// Open as detail tab (view-only)
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
			// Check if already open
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
			m.tabFocused = true
			m.contentFocused = false
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
				m.openFlowTab(tabOnChain,
					"On-Chain", secWallet)
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

// ── Add-ons content ──────────────────────────────────────

func (m Model) handleAddonsContentKey(
	key string,
) (tea.Model, tea.Cmd) {
	if m.subview != svNone {
		return m.handleAddonSubviewKey(key)
	}

	switch key {
	case "left", "h":
		if m.btnIdx > 0 {
			m.btnIdx--
		}
		// leftmost handled by handleContentKey
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

func (m Model) handleAddonSubviewKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svSyncthingDetail:
		return m.handleSyncDetailKey(key)
	case svSyncthingWebUI:
		return m.handleSyncWebUIKey(key)
	case svSyncthingDeviceDetail:
		if key == "backspace" || key == "esc" {
			m.subview = svSyncthingDetail
		}
		return m, nil
	case svLndHubManage:
		return m.handleLndhubManageKey(key)
	case svLndHubCreateAccount:
		if key == "enter" || key == "backspace" {
			m.subview = svLndHubManage
		}
		return m, nil
	case svLndHubAccountDetail:
		if key == "backspace" || key == "esc" {
			m.subview = svLndHubManage
		}
		return m, nil
	case svLndHubDeactivateConfirm:
		if key == "y" {
			if m.hubCursor <
				len(m.cfg.LndHubAccounts) {
				login := m.cfg.LndHubAccounts[m.hubCursor].Login
				return m, deactivateLndHubAccountCmd(login)
			}
		}
		if key == "n" || key == "esc" {
			m.subview = svLndHubManage
		}
		return m, nil
	}

	if key == "backspace" || key == "esc" {
		m.subview = svNone
		m.btnIdx = 0
	}
	return m, nil
}

func (m Model) handleSyncDetailKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "left":
		if m.addonBtnIdx > 0 {
			m.addonBtnIdx--
		}
	case "right", "l":
		if m.addonBtnIdx < 2 {
			m.addonBtnIdx++
		}
	case "up", "k":
		if m.syncCursor > 0 {
			m.syncCursor--
		}
	case "down", "j":
		if m.syncCursor <
			len(m.cfg.SyncthingDevices)-1 {
			m.syncCursor++
		}
	case "enter":
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
	case "backspace":
		m.subview = svNone
		m.btnIdx = 0
	}
	return m, nil
}

func (m Model) handleSyncWebUIKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "left":
		if m.addonBtnIdx > 0 {
			m.addonBtnIdx--
		}
	case "right", "l":
		if m.addonBtnIdx < 1 {
			m.addonBtnIdx++
		}
	case "enter":
		switch m.addonBtnIdx {
		case 0:
			syncOnion := readOnion(
				"TorSyncthingHostname placeholder")
			if syncOnion != "" {
				m.urlTarget = "http://" + syncOnion +
					":8384"
				m.urlReturnTo = svSyncthingWebUI
				m.subview = svFullURL
			}
		case 1:
			m.showSecrets = !m.showSecrets
		}
	case "backspace":
		m.subview = svSyncthingDetail
		m.addonBtnIdx = 0
	}
	return m, nil
}

func (m Model) handleLndhubManageKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.hubCursor > 0 {
			m.hubCursor--
		}
	case "down", "j":
		if m.hubCursor <
			len(m.cfg.LndHubAccounts)-1 {
			m.hubCursor++
		}
	case "left":
		if m.addonBtnIdx > 0 {
			m.addonBtnIdx--
		}
	case "right", "l":
		if m.addonBtnIdx < 2 {
			m.addonBtnIdx++
		}
	case "enter":
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
				m.subview = svLndHubDeactivateConfirm
			}
		}
	case "backspace":
		m.subview = svNone
		m.btnIdx = 0
	}
	return m, nil
}

// ── System content ───────────────────────────────────────

func (m Model) handleSystemContentKey(
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
		}
		// leftmost handled by handleContentKey
	case "right", "l":
		if m.btnIdx < maxBtn {
			m.btnIdx++
		}
	case "up", "k":
		if m.svcCursor > 0 {
			m.svcCursor--
		}
	case "down", "j":
		if m.svcCursor < m.svcCount()-1 {
			m.svcCursor++
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
	case "backspace", "esc":
		m.subview = svNone
		m.btnIdx = 0
		return m, nil
	}
	return m, nil
}

func (m Model) handleLndHubCreateNameKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.subview = svLndHubManage
		return m, nil
	case "enter":
		name := m.hubNameInput.Value()
		if name != "" {
			return m, createLndHubAccountCmd(
				m.cfg.LndHubAdminToken)
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.hubNameInput, cmd =
			m.hubNameInput.Update(tea.Msg(msg))
		return m, cmd
	}
}

func (m Model) handleSyncthingPairInputKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.syncPairError = ""
		m.syncPairSuccess = false
		m.subview = svSyncthingDetail
		return m, nil
	case "enter":
		if m.syncPairSuccess {
			m.syncDeviceInput = newSyncthingIDInput()
			m.syncPairSuccess = false
			m.subview = svSyncthingDetail
			return m, nil
		}
		deviceID := syncthingIDValue(m.syncDeviceInput)
		if deviceID != "" {
			parts := strings.Split(deviceID, "-")
			if len(parts) != 8 {
				m.syncPairError = "Invalid Device ID format. Expected 8 groups separated by hyphens."
				return m, nil
			}
			for _, p := range parts {
				if len(p) != 7 {
					m.syncPairError = "Invalid Device ID format. Each group should be 7 characters."
					return m, nil
				}
			}
			m.syncPairError = ""
			return m, pairSyncthingDeviceCmd(deviceID)
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.syncDeviceInput, cmd =
			m.syncDeviceInput.Update(tea.Msg(msg))
		return m, cmd
	}
}

// ── Subview classifiers ──────────────────────────────────

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

func isAddonSubview(sv wSubview) bool {
	switch sv {
	case svSyncthingDetail, svSyncthingDeviceDetail,
		svSyncthingWebUI, svSyncthingDeviceQR,
		svSyncthingPairInput,
		svLndHubManage, svLndHubCreateAccount,
		svLndHubCreateName,
		svLndHubAccountDetail,
		svLndHubDeactivateConfirm:
		return true
	}
	return false
}

// ── Channel open entry ───────────────────────────────────

func (m Model) startChannelOpenCmd() (tea.Model, tea.Cmd) {
	if m.lndClient == nil || !m.cfg.HasLND() ||
		!m.cfg.WalletExists() {
		return m, nil
	}
	if m.status != nil && m.status.lndBalance == "0" {
		m.subview = svChannelFundWallet
		return m, getNewAddressCmd(m.lndClient)
	}
	m.chanPeerList = curatedPeers()
	m.chanOpenPeerIdx = 0
	m.chanOpenError = ""
	m.subview = svChannelOpen
	return m, nil
}

// openFlowTab opens a flow tab (Send, Receive, etc.)
// or switches to it if already open. Focuses the tab
// and enters content.
func (m *Model) openFlowTab(
	kind tabKind, label string, section int,
) {
	// Check if already open
	tabs := m.effectiveTabs()
	for i, t := range tabs {
		if t.Kind == kind && t.Section == section {
			m.activeTab = i
			m.tabFocused = false
			m.contentFocused = true
			m.contentFocus = 0
			m.nav.Blur()
			return
		}
	}
	// Open new tab
	m.tabs = append(m.tabs, openTab{
		Kind:    kind,
		Label:   label,
		Section: section,
	})
	m.activeTab = len(m.effectiveTabs()) - 1
	m.tabFocused = false
	m.contentFocused = true
	m.contentFocus = 0
	m.nav.Blur()
}

// findFlowTab returns the tab index for the current
// flow subview, or the last activeTab if not found.
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
	case m.subview == svOnChain ||
		m.subview == svOnChainResult:
		kind = tabOnChain
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
	}
	return m, nil
}

// Unused import guard
var _ = theme.Value
var _ = strings.TrimSpace
var _ = logger.TUI
