package welcome

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/lndrpc"
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

// contentFocus returns the focus zone for the active
// section. Each section remembers its own zone
// independently (0=buttons/top, 1+=content zones).
func (m *Model) contentFocus() int {
	sec := m.nav.ActiveSection()
	if sec >= 0 && sec < numSections {
		return m.sectionFocus[sec]
	}
	return 0
}

// setContentFocus sets the focus zone for the active
// section.
func (m *Model) setContentFocus(v int) {
	sec := m.nav.ActiveSection()
	if sec >= 0 && sec < numSections {
		m.sectionFocus[sec] = v
	}
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
		// L16: route paste to active tab's screen.
		// Paste is user-directed (not async), so the
		// active tab is always the correct target.
		tabs := m.effectiveTabs()
		if m.activeTab > 0 &&
			m.activeTab < len(tabs) &&
			tabs[m.activeTab].Screen != nil {
			s := tabs[m.activeTab].Screen
			m.screenCtx.HasTabs = m.hasDetailTabs()
			m.screenCtx.ContentFocused = true
			newScreen, cmd := s.HandleMsg(msg)
			m.setTabScreen(m.activeTab, newScreen)
			return m, cmd
		}
		return m.handlePaste(msg)

	// ── L16 screen-to-Model messages ────────────────
	case closeTabMsg:
		return m.closeTab(m.activeTab)
	case focusSidebarMsg:
		m.focusSidebar()
		return m, nil
	case focusTabBarMsg:
		m.focusTabBar()
		m.tabCursorX = 0
		m.activeTab = m.findFlowTab()
		return m, nil
	case showQRMsg:
		m.urlTarget = msg.URL
		m.qrLabel = msg.Label
		m.subview = svQR
		return m, nil
	case showFullURLMsg:
		m.urlTarget = msg.URL
		m.subview = svFullURL
		return m, nil
	case refreshStatusMsg:
		return m, fetchStatus(m.cfg, m.lndClient)
	case clearUtxoSelectionMsg:
		m.clearUtxoSelection()
		m.syncOcCtxSelection()
		return m, nil
	case openTabMsg:
		// Dedup by kind + index if Index is set
		if msg.Index != 0 {
			tabs := m.effectiveTabs()
			for i, t := range tabs {
				if t.Kind == msg.Kind &&
					t.Index == msg.Index {
					m.activeTab = i
					if msg.FocusTabBar {
						m.focusTabBar()
						m.tabCursorX = 0
					} else {
						m.focusContent()
						m.setContentFocus(0)
					}
					return m, nil
				}
			}
		}
		m.tabs = append(m.tabs, openTab{
			Kind:    msg.Kind,
			Label:   msg.Label,
			Index:   msg.Index,
			Section: m.nav.ActiveSection(),
			Screen:  msg.Screen,
		})
		m.activeTab = len(m.effectiveTabs()) - 1
		if msg.FocusTabBar {
			m.focusTabBar()
			m.tabCursorX = 0
		} else {
			m.focusContent()
			m.setContentFocus(0)
		}
		if msg.Screen != nil {
			return m, msg.Screen.Init()
		}
		return m, nil

	case svcActionDoneMsg:
		return m, fetchStatus(m.cfg, m.lndClient)
	case statusMsg:
		m.fetchInFlight = false
		m.status = &msg
		m.screenCtx.Status = m.status
		if msg.walletDetected && !m.cfg.WalletCreated {
			m.cfg.WalletCreated = true
			m.saveCfg()
		}
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
		m.subview = svLndHubAccountDetail
		return m, nil
	case syncthingPairedMsg:
		if msg.err == nil {
			m.cfg.SyncthingDevices = append(
				m.cfg.SyncthingDevices,
				config.SyncthingDevice{
					Name: fmt.Sprintf("Device %d",
						len(m.cfg.SyncthingDevices)+1),
					DeviceID: msg.deviceID,
					PairedAt: time.Now().
						Format("2006-01-02"),
				})
			m.saveCfg()
		}
		// Route to screen for step/error state
		if rm, cmd, ok := m.routeToScreen(
			tabSyncthingPair, msg); ok {
			return rm, cmd
		}
		return m, nil
	case syncthingRemovedMsg:
		if msg.err == nil {
			// Remove device from config by ID
			for i, d := range m.cfg.SyncthingDevices {
				if d.DeviceID == msg.deviceID {
					m.cfg.SyncthingDevices = append(
						m.cfg.SyncthingDevices[:i],
						m.cfg.SyncthingDevices[i+1:]...)
					m.saveCfg()
					break
				}
			}
		}
		// Route to screen — screen emits closeTab
		// on success, sets error on failure
		if rm, cmd, ok := m.routeToScreen(
			tabSyncthingDevice, msg); ok {
			return rm, cmd
		}
		return m, nil
	case channelOpenResultMsg:
		// L16: route to channel open screen
		if rm, cmd, ok := m.routeToScreen(
			tabOpenChannel, msg); ok {
			return rm, cmd
		}
		return m, nil
	case newAddressMsg:
		if msg.err == nil {
			m.onChainAddress = msg.address
		}
		// Route to OCReceiveScreen if open
		if rm, cmd, ok := m.routeToScreen(
			tabOCReceive, msg); ok {
			return rm, cmd
		}
		return m, nil
	case invoiceCreatedMsg:
		rm, cmd, _ := m.routeToScreen(
			tabReceive, msg)
		return rm, cmd
	case invoiceSettledMsg:
		rm, cmd, _ := m.routeToScreen(
			tabReceive, msg)
		return rm, cmd
	case payReqDecodedMsg:
		rm, cmd, _ := m.routeToScreen(
			tabSend, msg)
		return rm, cmd
	case sendPaymentResultMsg:
		rm, cmd, _ := m.routeToScreen(
			tabSend, msg)
		return rm, cmd
	case paymentHistoryMsg:
		if msg.err == nil {
			m.payHistory = msg.entries
		}
		return m, nil
	case utxoListMsg:
		if msg.err == nil {
			m.utxos = msg.utxos
			m.ocCtx.Utxos = msg.utxos
			// Prune selections beyond new UTXO range
			for idx := range m.utxoSelected {
				if idx >= len(m.utxos) {
					delete(m.utxoSelected, idx)
				}
			}
			m.recalcSelectedTotal()
			m.syncOcCtxSelection()
		}
		return m, nil
	case onChainTxMsg:
		if msg.err == nil {
			m.onChainTxs = msg.txs
			m.ocCtx.OnChainTxs = msg.txs
		}
		return m, nil
	case sendCoinsResultMsg:
		rm, cmd, _ := m.routeToScreen(
			tabOnChain, msg)
		return rm, cmd
	case closeChannelMsg:
		rm, cmd, _ := m.routeToScreen(
			tabCloseChannel, msg)
		return rm, cmd
	case closedChannelsMsg:
		if msg.err == nil {
			m.buildChannelHistory(msg.channels)
		}
		return m, nil
	case labelTxMsg:
		if msg.err == nil {
			m.closeLabelPopup()
			return m, fetchOnChainTxCmd(m.lndClient)
		}
		return m, nil
	case feeTiersMsg:
		if msg.err == nil {
			m.sendFeeTiers = msg.tiers
			m.ocCtx.SendFeeTiers = msg.tiers
			// Route to close screen
			m.routeToScreen(
				tabCloseChannel, msg)
			// Route to channel detail screen
			m.routeToScreen(
				tabChannel, msg)
			// Route to on-chain send screen
			if rm, cmd, ok := m.routeToScreen(
				tabOnChain, msg); ok {
				return rm, cmd
			}
		}
		return m, nil
	case feeEstimateMsg:
		rm, cmd, _ := m.routeToScreen(
			tabOnChain, msg)
		return rm, cmd
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
	if m.subview == svQR || m.subview == svFullURL {
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
	// L16 new path: delegate to screen component
	if tab.Screen != nil {
		m.screenCtx.HasTabs = m.hasDetailTabs()
		m.screenCtx.ContentFocused = true
		newScreen, cmd := tab.Screen.HandleKey(key, msg)
		m.setTabScreen(m.activeTab, newScreen)
		return m, cmd
	}

	// Legacy path: existing switch on tab.Kind / m.subview
	switch tab.Kind {
	case tabLndHub:
		return m.handleLndHubTabKey(key, msg)
	case tabLndHubAccount:
		return m.handleLndHubAccountTabKey(key)
	}
	return m.handleSectionHomeKey(key, msg)
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
	m.syncOcCtxSelection()
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

// syncOcCtxSelection copies current UTXO selection state
// to the OnChainContext so screens see current data.
func (m *Model) syncOcCtxSelection() {
	m.ocCtx.UtxoSelected = m.utxoSelected
	m.ocCtx.UtxoSelectedTotal = m.utxoSelectedTotal
	m.ocCtx.UtxoOutpoints = m.utxoOutpoints
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
	case svLndHubCreateQR:
		return m.handleLndHubCreateQRKey(key)
	default:
		m.restoreTabSubview(tabLndHub)
		return m.handleLndhubManageKey(key)
	}
}

// Handler for LndHub created screen (Show QR + Done)
func (m Model) handleLndHubCreatedKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left":
		if m.addonBtnIdx > 0 {
			m.addonBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "right":
		if m.addonBtnIdx < 1 {
			m.addonBtnIdx++
		}
		return m, nil
	case "up":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "enter":
		switch m.addonBtnIdx {
		case 0: // Show QR
			m.subview = svLndHubCreateQR
		case 1: // Done
			m.addonBtnIdx = 0
			m.subview = svLndHubManage
		}
		return m, nil
	case "backspace":
		m.addonBtnIdx = 0
		m.subview = svLndHubManage
		return m, nil
	}
	return m, nil
}

// Handler for LndHub create QR subview
func (m Model) handleLndHubCreateQRKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "enter", "backspace":
		m.subview = svLndHubCreateAccount
		return m, nil
	case "left":
		m.focusSidebar()
		return m, nil
	case "up":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
	}
	return m, nil
}

// ── LndHub account detail tab ───────────────────────────

func (m Model) handleLndHubAccountTabKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svLndHubDeactivateConfirm:
		return m.handleLndHubDeactivateKey(key)
	default:
		m.subview = svLndHubAccountDetail
		return m.handleLndHubAccountDetailKey(key)
	}
}

func (m Model) handleLndHubAccountDetailKey(
	key string,
) (tea.Model, tea.Cmd) {
	// Check if account is active (has button)
	hasButton := false
	if m.hubCursor < len(m.cfg.LndHubAccounts) {
		hasButton =
			m.cfg.LndHubAccounts[m.hubCursor].Active
	}

	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left":
		m.focusSidebar()
		return m, nil
	case "up":
		if m.hasDetailTabs() {
			m.setContentFocus(0)
			m.focusTabBar()
			m.tabCursorX = 0
			return m, nil
		}
	case "down", "tab":
		return m, nil
	case "enter":
		if hasButton && m.contentFocus() == 1 {
			m.hubDeactivateBtnIdx = 0
			m.subview = svLndHubDeactivateConfirm
		} else if !hasButton {
			// Deactivated account: enter returns
			// (same as channel close result)
			return m.closeTab(m.activeTab)
		}
	case "backspace":
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

func (m Model) handleLndHubDeactivateKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left":
		if m.hubDeactivateBtnIdx > 0 {
			m.hubDeactivateBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "right":
		if m.hubDeactivateBtnIdx < 1 {
			m.hubDeactivateBtnIdx++
		}
		return m, nil
	case "up":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
	case "enter":
		switch m.hubDeactivateBtnIdx {
		case 0: // Go Back
			m.subview = svLndHubAccountDetail
			m.setContentFocus(1)
			return m, nil
		case 1: // Deactivate
			if m.hubCursor <
				len(m.cfg.LndHubAccounts) {
				login :=
					m.cfg.LndHubAccounts[m.hubCursor].Login
				return m, deactivateLndHubAccountCmd(login)
			}
		}
	case "backspace":
		m.subview = svLndHubAccountDetail
		m.setContentFocus(1)
		return m, nil
	}
	return m, nil
}

// ── Section home key dispatch ────────────────────────────

func (m Model) handleSectionHomeKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	// UTXO label popup intercepts ALL keys
	// before any other handling.
	if m.utxoLabelEditing {
		return m.handleUtxoLabelPopupKey(key, msg)
	}

	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "backspace":
		m.focusSidebar()
		return m, nil
	}

	sec := m.nav.ActiveSection()
	switch sec {
	case secChannels:
		return m.handleChannelsHomeKey(key)
	case secWallet:
		return m.handleWalletHomeKey(key)
	case secOnChain:
		return m.handleOnChainContentKey(key, false, msg)
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
	case "left":
		if m.contentFocus() == 0 && m.btnIdx > 0 {
			m.btnIdx--
			return m, nil
		}
		m.focusSidebar()
		return m, nil
	case "up":
		if m.contentFocus() == 1 {
			if m.chanCursor > 0 {
				m.chanCursor--
			} else {
				m.setContentFocus(0)
				m.btnIdx = 0
			}
		} else if m.contentFocus() == 0 {
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = 1
			}
		}
	case "down", "tab":
		if m.contentFocus() == 0 {
			if m.status != nil &&
				len(m.status.channels) > 0 {
				m.setContentFocus(1)
				m.chanCursor = 0
			}
		} else if m.contentFocus() == 1 {
			if m.status != nil &&
				m.chanCursor <
					len(m.status.channels)-1 {
				m.chanCursor++
			}
		}
	case "right":
		if m.contentFocus() == 0 && m.btnIdx < 1 {
			m.btnIdx++
		}
	case "enter":
		if m.contentFocus() == 1 {
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
				screen := NewChannelDetailScreen(
					m.screenCtx, ch,
					m.sendFeeTiers)
				m.findOrOpenTabWithScreen(
					tabChannel, label,
					m.chanCursor, secChannels,
					screen)
			}
		} else if m.contentFocus() == 0 {
			switch m.btnIdx {
			case 0:
				return m.startChannelOpenCmd()
			case 1:
				screen := NewChannelHistoryScreen(
					m.screenCtx,
					m.chanHistory)
				cmd := m.openFlowTabWithScreen(
					tabChannelHistory,
					"History",
					secChannels,
					screen)
				return m, tea.Batch(cmd,
					fetchClosedChannelsCmd(
						m.lndClient))
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
	case "left":
		if m.contentFocus() == 0 {
			if m.btnIdx > 0 {
				m.btnIdx--
				return m, nil
			}
		}
		m.focusSidebar()
		return m, nil
	case "up":
		if m.contentFocus() == 1 {
			if m.payHistoryCursor > 0 {
				m.payHistoryCursor--
			} else {
				m.setContentFocus(0)
				m.btnIdx = 0
			}
		} else if m.contentFocus() == 0 {
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = 1
				return m, nil
			}
		}
	case "down", "tab":
		if m.contentFocus() == 0 {
			m.setContentFocus(1)
			m.payHistoryCursor = 0
		} else if m.contentFocus() == 1 {
			if m.payHistoryCursor <
				len(m.payHistory)-1 {
				m.payHistoryCursor++
			}
		}
	case "right":
		if m.contentFocus() == 0 {
			if m.btnIdx < 2 {
				m.btnIdx++
			}
		}
	case "enter":
		if m.contentFocus() == 1 &&
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
			m.findOrOpenTabWithScreen(
				tabPayment, label,
				m.payHistoryCursor, secWallet,
				NewPaymentDetailScreen(
					m.screenCtx, entry))
		} else if m.contentFocus() == 0 {
			switch m.btnIdx {
			case 0:
				if m.cfg.HasLND() &&
					m.cfg.WalletExists() {
					screen := NewSendScreen(
						m.screenCtx)
					cmd := m.openFlowTabWithScreen(
						tabSend,
						"⚡ Send",
						secWallet,
						screen)
					return m, cmd
				}
			case 1:
				if m.cfg.HasLND() &&
					m.cfg.WalletExists() {
					screen := NewReceiveScreen(
						m.screenCtx)
					cmd := m.openFlowTabWithScreen(
						tabReceive,
						"⚡ Receive",
						secWallet,
						screen)
					return m, cmd
				}
			case 2:
				screen := NewPairingScreen(
					m.screenCtx)
				cmd := m.openFlowTabWithScreen(
					tabPairing,
					"⚡ Zeus — LND REST",
					secWallet,
					screen)
				return m, cmd
			}
		}
	}
	return m, nil
}

func (m *Model) buildChannelHistory(
	closed []lndrpc.ClosedChannel,
) {
	var channels []channelInfo
	var waiting []lndrpc.WaitingCloseChannel
	var pending []lndrpc.PendingForceCloseChannel
	if m.status != nil {
		channels = m.status.channels
		waiting = m.status.waitingCloseChannels
		pending = m.status.pendingForceCloseChannels
	}
	m.chanHistory = buildChannelHistoryEntries(
		channels, waiting, pending, closed)
}

func buildChannelHistoryEntries(
	channels []channelInfo,
	waiting []lndrpc.WaitingCloseChannel,
	pending []lndrpc.PendingForceCloseChannel,
	closed []lndrpc.ClosedChannel,
) []channelHistoryEntry {
	var entries []channelHistoryEntry

	// Active and inactive channels
	for _, ch := range channels {
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
	for _, wc := range waiting {
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
	for _, fc := range pending {
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

	return entries
}

// ── On-Chain content (merged home + tab handler) ────────

func (m Model) handleOnChainContentKey(
	key string, fromTab bool, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	// ── UTXO label popup handling ───────────────
	if m.utxoLabelEditing {
		return m.handleUtxoLabelPopupKey(key, msg)
	}

	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if fromTab {
			return m.closeTab(m.activeTab)
		}
	case "up":
		switch m.contentFocus() {
		case 2:
			if m.onChainTxCursor > 0 {
				m.onChainTxCursor--
			} else {
				m.setContentFocus(1)
				if len(m.utxos) > 0 {
					m.utxoCursor =
						len(m.utxos) - 1
				}
			}
		case 1:
			if m.utxoCursor > 0 {
				m.utxoCursor--
				m.utxoPencilFocused = false
			} else {
				m.setContentFocus(0)
				m.onChainBtnIdx = 0
				m.utxoPencilFocused = false
			}
		case 0:
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				if fromTab {
					m.activeTab = m.findFlowTab()
				} else {
					m.activeTab = 1
				}
				return m, nil
			}
		}
	case "down", "tab":
		switch m.contentFocus() {
		case 0:
			if len(m.utxos) > 0 {
				m.setContentFocus(1)
				m.utxoCursor = 0
				m.utxoPencilFocused = false
			} else if len(m.onChainTxs) > 0 {
				m.setContentFocus(2)
				m.onChainTxCursor = 0
			}
		case 1:
			if m.utxoCursor < len(m.utxos)-1 {
				m.utxoCursor++
				m.utxoPencilFocused = false
			} else if len(m.onChainTxs) > 0 {
				m.setContentFocus(2)
				m.onChainTxCursor = 0
				m.utxoPencilFocused = false
			}
		case 2:
			if m.onChainTxCursor < len(m.onChainTxs)-1 {
				m.onChainTxCursor++
			}
		}
	case "right":
		if m.contentFocus() == 0 &&
			m.onChainBtnIdx < 1 {
			m.onChainBtnIdx++
		}
		// UTXO row: right arrow focuses pencil icon
		if m.contentFocus() == 1 &&
			m.utxoCursor < len(m.utxos) &&
			!m.utxoPencilFocused {
			m.utxoPencilFocused = true
			return m, nil
		}
	case "left":
		// UTXO row: left arrow unfocuses pencil
		if m.contentFocus() == 1 &&
			m.utxoPencilFocused {
			m.utxoPencilFocused = false
			return m, nil
		}
		if m.contentFocus() == 0 &&
			m.onChainBtnIdx > 0 {
			m.onChainBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "space":
		if m.contentFocus() == 1 &&
			m.utxoCursor < len(m.utxos) {
			m.toggleUtxoSelection(m.utxoCursor)
		}
		return m, nil
	case "enter":
		if m.contentFocus() == 0 {
			switch m.onChainBtnIdx {
			case 0:
				screen := NewOCReceiveScreen(
					m.screenCtx)
				cmd := m.openFlowTabWithScreen(
					tabOCReceive,
					"⛓ Receive",
					secOnChain,
					screen)
				return m, cmd
			case 1:
				screen := NewOnChainSendScreen(
					m.screenCtx, m.ocCtx)
				// Pre-fill amount from UTXO selection
				if len(m.utxoSelected) > 0 {
					screen.amtInput.SetValue(
						fmt.Sprintf("%d",
							m.utxoSelectedTotal))
				}
				// Pre-fill fee from cached tiers
				if m.sendFeeTiers[0].SatPerVB > 0 {
					screen.feeInput.SetValue(
						fmt.Sprintf("%.0f",
							m.sendFeeTiers[0].SatPerVB))
				}
				cmd := m.openFlowTabWithScreen(
					tabOnChain,
					"⛓ Send",
					secOnChain,
					screen)
				return m, cmd
			}
		} else if m.contentFocus() == 1 &&
			m.utxoCursor < len(m.utxos) {
			if m.utxoPencilFocused {
				// Open label edit popup
				m.openLabelPopup()
				return m, nil
			}
			// Open view-only detail tab
			u := m.utxos[m.utxoCursor]
			label := u.Address
			if len(label) > 14 {
				label = label[:12] + ".."
			}
			m.setContentFocus(0)
			m.findOrOpenTabWithScreen(
				tabUtxoDetail, label,
				m.utxoCursor, secOnChain,
				NewUtxoDetailScreen(
					m.screenCtx, u,
					m.utxoDate(u.Txid),
					m.utxoTxLabel(u.Txid)))
		} else if m.contentFocus() == 2 &&
			m.onChainTxCursor < len(m.onChainTxs) {
			tx := m.onChainTxs[m.onChainTxCursor]
			label := tx.Label
			if len(label) > 14 {
				label = label[:12] + ".."
			}
			var pfc []lndrpc.PendingForceCloseChannel
			if m.status != nil {
				pfc = m.status.
					pendingForceCloseChannels
			}
			m.findOrOpenTabWithScreen(
				tabOnChainTx, label,
				m.onChainTxCursor, secOnChain,
				NewOnChainTxScreen(
					m.screenCtx, tx, pfc))
		}
	}
	return m, nil
}

// ── UTXO label popup keys ───────────────────────────────

func (m Model) handleUtxoLabelPopupKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit

	case "up":
		if m.utxoLabelOnBtn {
			m.utxoLabelOnBtn = false
			m.utxoLabelInput.Focus()
			return m, nil
		}
		return m, nil

	case "down", "tab":
		if !m.utxoLabelOnBtn {
			m.utxoLabelOnBtn = true
			m.utxoLabelBtnIdx = 0
			m.utxoLabelInput.Blur()
			return m, nil
		}
		return m, nil

	case "left":
		if m.utxoLabelOnBtn {
			if m.utxoLabelBtnIdx > 0 {
				m.utxoLabelBtnIdx--
			}
			return m, nil
		}
		// In label field — cursor left
		var cmd tea.Cmd
		m.utxoLabelInput, cmd =
			m.utxoLabelInput.Update(tea.Msg(msg))
		return m, cmd

	case "right":
		if m.utxoLabelOnBtn {
			if m.utxoLabelBtnIdx < 1 {
				m.utxoLabelBtnIdx++
			}
			return m, nil
		}
		// In label field — cursor right
		var cmd tea.Cmd
		m.utxoLabelInput, cmd =
			m.utxoLabelInput.Update(tea.Msg(msg))
		return m, cmd

	case "enter":
		if m.utxoLabelOnBtn {
			switch m.utxoLabelBtnIdx {
			case 0: // Save
				if m.utxoCursor < len(m.utxos) {
					txid :=
						m.utxos[m.utxoCursor].Txid
					label :=
						m.utxoLabelInput.Value()
					return m, labelTxCmd(
						m.lndClient, txid, label)
				}
				m.closeLabelPopup()
				return m, nil
			case 1: // Cancel
				m.closeLabelPopup()
				return m, nil
			}
		}
		// On label field — move to buttons
		m.utxoLabelOnBtn = true
		m.utxoLabelBtnIdx = 0
		m.utxoLabelInput.Blur()
		return m, nil

	case "backspace":
		// Always delete in label field
		if !m.utxoLabelOnBtn {
			var cmd tea.Cmd
			m.utxoLabelInput, cmd =
				m.utxoLabelInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		return m, nil

	default:
		// Pass all other keys to label field
		if !m.utxoLabelOnBtn {
			var cmd tea.Cmd
			m.utxoLabelInput, cmd =
				m.utxoLabelInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
	}
	return m, nil
}

// restoreTabSubview restores the entry subview for a
// flow tab whose subview was cleared during navigation
// (e.g. sidebar switch). Returns true if restoration
// was needed.
func (m *Model) restoreTabSubview(kind tabKind) bool {
	switch kind {
	case tabLndHub:
		if m.subview != svLndHubManage &&
			m.subview != svLndHubCreateName &&
			m.subview != svLndHubCreateAccount &&
			m.subview != svLndHubCreateQR {
			m.subview = svLndHubManage
			return true
		}
	case tabLndHubAccount:
		if m.subview != svLndHubAccountDetail &&
			m.subview != svLndHubDeactivateConfirm {
			m.subview = svLndHubAccountDetail
			return true
		}
	}
	return false
}

// ── Addons home ──────────────────────────────────────────

func (m Model) handleAddonsHomeKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "left":
		m.focusSidebar()
		return m, nil
	case "up":
		if m.btnIdx > 0 {
			m.btnIdx--
		} else if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = 1
			return m, nil
		}
	case "down", "tab":
		if m.btnIdx < 1 {
			m.btnIdx++
		}
	case "enter":
		switch m.btnIdx {
		case 0:
			if m.cfg.SyncthingInstalled {
				screen := NewSyncthingDetailScreen(
					m.screenCtx)
				cmd := m.openFlowTabWithScreen(
					tabSyncthing,
					"Syncthing",
					secAddons,
					screen)
				return m, cmd
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
				m.setContentFocus(0)
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
	case "left":
		if m.contentFocus() == 0 && m.btnIdx > 0 {
			m.btnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "up":
		if m.contentFocus() == 1 {
			if m.svcCursor > 0 {
				m.svcCursor--
			} else {
				m.setContentFocus(0)
				m.btnIdx = 0
			}
		} else if m.contentFocus() == 0 {
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = 1
				return m, nil
			}
		}
	case "down", "tab":
		if m.contentFocus() == 0 {
			if m.svcCount() > 0 {
				m.setContentFocus(1)
				m.svcCursor = 0
			}
		} else if m.contentFocus() == 1 {
			if m.svcCursor < m.svcCount()-1 {
				m.svcCursor++
			}
		}
	case "right":
		if m.contentFocus() == 0 {
			if m.btnIdx < maxBtn {
				m.btnIdx++
			}
		}
	case "r":
		if m.contentFocus() == 1 {
			m.svcConfirm = "Restart"
		}
	case "s":
		if m.contentFocus() == 1 {
			m.svcConfirm = "Stop"
		}
	case "a":
		if m.contentFocus() == 1 {
			m.svcConfirm = "Start"
		}
	case "l":
		if m.contentFocus() == 1 {
			svc := m.svcName(m.svcCursor)
			c := exec.Command("bash", "-c",
				"clear && sudo journalctl -u "+svc+
					" -n 100 --no-pager"+
					" && echo && echo "+
					"'  Press Enter to return...'"+
					" && read")
			return m, tea.ExecProcess(c,
				func(err error) tea.Msg {
					return svcActionDoneMsg{}
				})
		}
	case "enter":
		if m.contentFocus() == 0 {
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

// ── LndHub manage keys ──────────────────────────────────

func (m Model) handleLndhubManageKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left":
		if m.contentFocus() == 0 && m.addonBtnIdx > 0 {
			m.addonBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "right":
		// Single button — no right navigation
	case "up":
		if m.contentFocus() == 1 {
			if m.hubCursor > 0 {
				m.hubCursor--
			} else {
				m.setContentFocus(0)
				m.addonBtnIdx = 0
			}
		} else if m.contentFocus() == 0 {
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = m.findFlowTab()
				return m, nil
			}
		}
	case "down", "tab":
		if m.contentFocus() == 0 {
			if len(m.cfg.LndHubAccounts) > 0 {
				m.setContentFocus(1)
				m.hubCursor = 0
			}
		} else if m.contentFocus() == 1 {
			if m.hubCursor <
				len(m.cfg.LndHubAccounts)-1 {
				m.hubCursor++
			}
		}
	case "enter":
		if m.contentFocus() == 0 {
			// Create
			m.hubNameInput = newHubNameInput()
			m.subview = svLndHubCreateName
		} else if m.contentFocus() == 1 {
			if m.hubCursor <
				len(m.cfg.LndHubAccounts) {
				acct := m.cfg.LndHubAccounts[m.hubCursor]
				label := acct.Label
				if len(label) > 17 {
					label = label[:17] + "..."
				}
				m.findOrOpenTab(tabLndHubAccount,
					label, m.hubCursor, secAddons)
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
	case "ctrl+c":
		return m, tea.Quit
	case "up":
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
	case "down", "tab":
		m.nav.MoveDown()
		return m, nil
	case "enter", "right":
		// Theme toggle — don't activate a section,
		// just toggle the theme and stay on the icon.
		if m.nav.IsOnThemeToggle() {
			mode := theme.Toggle()
			m.cfg.Theme = mode
			m.saveCfg()
			m.nav.UpdateThemeLabel()
			return m, nil
		}
		sec := m.nav.Activate()
		m.focusContent()
		m.activeTab = 0
		m.btnIdx = 0
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
		return m,
			fetchPaymentHistoryCmd(m.lndClient)
	case secOnChain:
		return m, tea.Batch(
			listUnspentCmd(m.lndClient),
			fetchOnChainTxCmd(m.lndClient))
	}
	return m, nil
}

// ── Tab bar keys ─────────────────────────────────────────

func (m Model) handleTabBarKey(
	key string,
) (tea.Model, tea.Cmd) {
	tabs := m.effectiveTabs()

	switch key {
	case "ctrl+c":
		return m, tea.Quit

	case "down", "tab":
		// Don't enter content on view-only tabs
		if m.activeTab > 0 &&
			m.activeTab < len(tabs) {
			switch tabs[m.activeTab].Kind {
			case tabLndHubAccount:
				m.subview = svLndHubAccountDetail
				m.focusContent()
				m.setContentFocus(1)
				return m, nil
			case tabOnChain, tabSend, tabReceive,
				tabOpenChannel, tabCloseChannel,
				tabChannel, tabPayment, tabOnChainTx,
				tabUtxoDetail, tabChannelHistory,
				tabSyncthing, tabLndHub,
				tabSyncthingDevice,
				tabSyncthingWebUI, tabSyncthingPair:
				if tabs[m.activeTab].Screen == nil {
					m.restoreTabSubview(
						tabs[m.activeTab].Kind)
				}
				m.focusContent()
				return m, nil
			}
		}
		m.focusContent()
		if m.activeTab == 0 {
			m.subview = svNone
		}
		m.ensureContentCursor()
		return m, nil

	case "left":
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
		m.activeTab = 0
		m.tabScrollOffset = 0
		return m, nil

	case "right":
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
		// Don't enter content on view-only tabs
		if m.activeTab > 0 &&
			m.activeTab < len(tabs) {
			switch tabs[m.activeTab].Kind {
			case tabLndHubAccount:
				m.subview = svLndHubAccountDetail
				m.focusContent()
				m.setContentFocus(1)
				return m, nil
			case tabOnChain, tabSend, tabReceive,
				tabOpenChannel, tabCloseChannel,
				tabChannel, tabPayment, tabOnChainTx,
				tabUtxoDetail, tabChannelHistory,
				tabSyncthing, tabLndHub,
				tabSyncthingDevice,
				tabSyncthingWebUI, tabSyncthingPair:
				if tabs[m.activeTab].Screen == nil {
					m.restoreTabSubview(
						tabs[m.activeTab].Kind)
				}
				m.focusContent()
				return m, nil
			}
		}
		m.focusContent()
		if m.activeTab == 0 {
			m.subview = svNone
		}
		m.ensureContentCursor()
		return m, nil

	case "backspace":
		if m.activeTab > 0 {
			return m.closeTab(m.activeTab)
		}
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
	case tabCloseChannel:
		m.subview = svNone
	case tabChannel:
		m.subview = svNone
	case tabPayment:
		m.subview = svNone
	case tabOnChainTx:
		m.subview = svNone
	case tabChannelHistory:
		m.subview = svNone
	case tabUtxoDetail:
		m.subview = svNone
		m.closeLabelPopup()
	case tabSend:
		m.subview = svNone
	case tabReceive:
		m.subview = svNone
	case tabOnChain:
		m.subview = svNone
	case tabOCReceive:
		m.subview = svNone
	case tabPairing:
		m.subview = svNone
	case tabSyncthing:
		m.subview = svNone
	case tabSyncthingDevice:
		m.subview = svNone
	case tabSyncthingWebUI:
		m.subview = svNone
	case tabSyncthingPair:
		m.subview = svNone
	case tabLndHub:
		m.subview = svNone
	case tabLndHubAccount:
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

	m.focusContent()
	m.setContentFocus(0)

	// Addon detail tabs: return to parent manage tab.
	// All other tabs: return to section home (tab 0).
	parentKind := tabKind(-1)
	switch closingTab.Kind {
	case tabSyncthingDevice, tabSyncthingWebUI,
		tabSyncthingPair:
		parentKind = tabSyncthing
	case tabLndHubAccount:
		parentKind = tabLndHub
	}
	m.activeTab = 0
	if parentKind >= 0 {
		for i, t := range m.effectiveTabs() {
			if t.Kind == parentKind {
				m.activeTab = i
				break
			}
		}
	}

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
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.subview = svNone
		return m, nil
	}
	return m, nil
}

// ── View-only tab handler (shared) ──────────────────────

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

// findOrOpenTabWithScreen opens a detail tab backed by
// a Screen. If a tab of the same kind and index already
// exists, it reuses it (keeping existing screen state).
// When a new tab is created, the screen's Init() is
// called and the resulting command is returned.
func (m *Model) findOrOpenTabWithScreen(
	kind tabKind, label string,
	index, section int,
	screen Screen,
) tea.Cmd {
	tabs := m.effectiveTabs()
	for i, t := range tabs {
		if t.Kind == kind && t.Index == index {
			m.activeTab = i
			m.focusTabBar()
			m.tabCursorX = 0
			return nil
		}
	}
	m.tabs = append(m.tabs, openTab{
		Kind:    kind,
		Label:   label,
		Index:   index,
		Section: section,
		Screen:  screen,
	})
	m.activeTab = len(m.effectiveTabs()) - 1
	m.focusTabBar()
	m.tabCursorX = 0
	return screen.Init()
}

// ── Channel open entry ───────────────────────────────────

func (m Model) startChannelOpenCmd() (
	tea.Model, tea.Cmd,
) {
	if m.lndClient == nil || !m.cfg.HasLND() ||
		!m.cfg.WalletExists() {
		return m, nil
	}
	// Zero balance: redirect to on-chain receive
	if m.status != nil &&
		m.status.lndBalance == "0" {
		screen := NewOCReceiveScreen(m.screenCtx)
		cmd := m.openFlowTabWithScreen(
			tabOCReceive,
			"Receive", secOnChain,
			screen)
		return m, cmd
	}
	// Normal path: create screen
	screen := NewChannelOpenScreen(m.screenCtx)
	cmd := m.openFlowTabWithScreen(
		tabOpenChannel, "Open Channel",
		secChannels, screen)
	return m, cmd
}

func (m *Model) openFlowTab(
	kind tabKind, label string, section int,
) {
	tabs := m.effectiveTabs()
	for i, t := range tabs {
		if t.Kind == kind && t.Section == section {
			m.activeTab = i
			m.focusContent()
			m.setContentFocus(0)
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
	m.setContentFocus(0)
}

// openFlowTabWithScreen opens a flow tab backed by a
// Screen component. If a tab of the same kind already
// exists in the section, it reuses it (and keeps the
// existing screen's state). When a new tab is created,
// the screen's Init() is called and the resulting
// command is returned.
func (m *Model) openFlowTabWithScreen(
	kind tabKind, label string, section int,
	screen Screen,
) tea.Cmd {
	tabs := m.effectiveTabs()
	for i, t := range tabs {
		if t.Kind == kind && t.Section == section {
			m.activeTab = i
			m.focusContent()
			m.setContentFocus(0)
			return nil
		}
	}
	m.tabs = append(m.tabs, openTab{
		Kind:    kind,
		Label:   label,
		Section: section,
		Screen:  screen,
	})
	m.activeTab = len(m.effectiveTabs()) - 1
	m.focusContent()
	m.setContentFocus(0)
	return screen.Init()
}

func (m Model) findFlowTab() int {
	tabs := m.effectiveTabs()
	var kind tabKind
	switch {
	case isOnChainSubview(m.subview):
		kind = tabOnChain
	case m.subview == svLndHubManage ||
		m.subview == svLndHubCreateName ||
		m.subview == svLndHubCreateAccount ||
		m.subview == svLndHubCreateQR:
		kind = tabLndHub
	case m.subview == svLndHubAccountDetail ||
		m.subview == svLndHubDeactivateConfirm:
		kind = tabLndHubAccount
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

func isOnChainSubview(sv wSubview) bool {
	switch sv {
	case svOnChain, svOnChainResult,
		svOnChainSend, svOCSendConfirm,
		svOCSendBroadcast:
		return true
	}
	return false
}

// ── L16 screen dispatch helpers ──────────────────────────

// setTabScreen updates the Screen on the active tab.
// Works on the underlying m.tabs slice (effectiveTabs is
// a derived view).
func (m *Model) setTabScreen(
	effectiveIdx int, s Screen,
) {
	tabs := m.effectiveTabs()
	if effectiveIdx <= 0 || effectiveIdx >= len(tabs) {
		return
	}
	target := tabs[effectiveIdx]
	for i := range m.tabs {
		if m.tabs[i].Kind == target.Kind &&
			m.tabs[i].Index == target.Index &&
			m.tabs[i].Section == target.Section {
			m.tabs[i].Screen = s
			return
		}
	}
}

// routeToScreen delivers a message to the screen on the
// tab matching the given kind. Routes by tab kind, not
// by which tab is active — an invoiceCreatedMsg must
// reach the receive screen even if the user switched to
// a different tab while the async operation was in
// flight. Returns (model, cmd, true) if routed, or
// (model, nil, false) if no matching screen exists.
func (m Model) routeToScreen(
	kind tabKind, msg tea.Msg,
) (Model, tea.Cmd, bool) {
	tabs := m.effectiveTabs()
	for i, tab := range tabs {
		if tab.Kind == kind && tab.Screen != nil {
			m.screenCtx.HasTabs = m.hasDetailTabs()
			newScreen, cmd := tab.Screen.HandleMsg(msg)
			m.setTabScreen(i, newScreen)
			return m, cmd, true
		}
	}
	return m, nil, false
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

func showInvoiceCmd(invoice string) tea.Cmd {
	if invoice == "" {
		return nil
	}
	tmpFile, err := os.CreateTemp("", "rlvpn-invoice-")
	if err != nil {
		return nil
	}
	tmpPath := tmpFile.Name()
	_, _ = tmpFile.WriteString(invoice)
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
	case svLndHubCreateName:
		var cmd tea.Cmd
		m.hubNameInput, cmd =
			m.hubNameInput.Update(msg)
		return m, cmd
	}

	// Tab-based paste routing (no subview set)
	tabs := m.effectiveTabs()
	idx := m.activeTab
	if idx > 0 && idx < len(tabs) {
		tab := tabs[idx]
		switch tab.Kind {
		case tabUtxoDetail:
			// Legacy — keep for safety
			return m, nil
		}
	}

	// UTXO label popup paste
	if m.utxoLabelEditing && !m.utxoLabelOnBtn {
		var cmd tea.Cmd
		m.utxoLabelInput, cmd =
			m.utxoLabelInput.Update(msg)
		return m, cmd
	}

	return m, nil
}
