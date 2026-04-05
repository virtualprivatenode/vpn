package welcome

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/lndrpc"
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
		// Route paste to active tab's screen.
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
		// Route paste to section home screen
		// (e.g. UTXO label popup on on-chain home).
		sec := m.nav.ActiveSection()
		if sec >= 0 && sec < numSections &&
			m.sectionScreens[sec] != nil {
			m.screenCtx.HasTabs = m.hasDetailTabs()
			m.screenCtx.ContentFocused = true
			newScreen, cmd :=
				m.sectionScreens[sec].HandleMsg(msg)
			m.sectionScreens[sec] = newScreen
			return m, cmd
		}
		return m, nil

	// ── L16 screen-to-Model messages ────────────────
	case closeTabMsg:
		return m.closeTab(m.activeTab)
	case focusSidebarMsg:
		m.focusSidebar()
		return m, nil
	case focusTabBarMsg:
		m.focusTabBar()
		m.tabCursorX = 0
		m.activeTab = 1
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
		return m, nil
	case shellActionMsg:
		m.shellAction = msg.action
		return m, tea.Quit
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
					}
					return m, nil
				}
			}
		}
		// Dedup flow tabs by kind + section
		if msg.Index == 0 {
			sec := m.nav.ActiveSection()
			tabs := m.effectiveTabs()
			for i, t := range tabs {
				if t.Kind == msg.Kind &&
					t.Section == sec {
					m.activeTab = i
					if msg.FocusTabBar {
						m.focusTabBar()
						m.tabCursorX = 0
					} else {
						m.focusContent()
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
		m.screenCtx.LatestVersion = string(msg)
		return m, nil
	case lndhubAccountCreatedMsg:
		if msg.err == nil && msg.account != nil {
			m.cfg.LndHubAccounts = append(
				m.cfg.LndHubAccounts,
				config.LndHubAccount{
					Label: msg.label,
					Login: msg.account.Login,
					CreatedAt: time.Now().
						Format("2006-01-02"),
					Active: true,
				})
			m.saveCfg()
		}
		// Route to screen for step/error state
		if rm, cmd, ok := m.routeToScreen(
			tabLndHubCreate, msg); ok {
			return rm, cmd
		}
		return m, nil
	case lndhubDeactivatedMsg:
		if msg.err == nil {
			// Find account by login and deactivate
			for i := range m.cfg.LndHubAccounts {
				if m.cfg.LndHubAccounts[i].Login ==
					msg.login {
					m.cfg.LndHubAccounts[i].Active = false
					m.cfg.LndHubAccounts[i].DeactivatedAt =
						time.Now().Format("2006-01-02")
					m.cfg.LndHubAccounts[i].
						BalanceOnDeactivate = msg.balance
					m.saveCfg()
					break
				}
			}
		}
		// Route to screen for state transition
		if rm, cmd, ok := m.routeToScreen(
			tabLndHubAccount, msg); ok {
			return rm, cmd
		}
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
		// Route to wallet home screen
		if rm, cmd, ok := m.routeToSectionScreen(
			secWallet, msg); ok {
			return rm, cmd
		}
		return m, nil
	case utxoListMsg:
		if msg.err == nil {
			m.ocCtx.Utxos = msg.utxos
			// Prune selections beyond new UTXO range
			for idx := range m.ocCtx.UtxoSelected {
				if idx >= len(m.ocCtx.Utxos) {
					delete(m.ocCtx.UtxoSelected, idx)
				}
			}
			m.recalcSelectedTotal()
		}
		return m, nil
	case onChainTxMsg:
		if msg.err == nil {
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
		// Route to history screen so it gets the data
		if rm, cmd, ok := m.routeToScreen(
			tabChannelHistory, msg); ok {
			return rm, cmd
		}
		return m, nil
	case labelTxMsg:
		// Route to on-chain home screen
		if rm, cmd, ok := m.routeToSectionScreen(
			secOnChain, msg); ok {
			return rm, cmd
		}
		if msg.err == nil {
			return m, fetchOnChainTxCmd(m.lndClient)
		}
		return m, nil
	case feeTiersMsg:
		if msg.err == nil {
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

	// 1. Fullscreen views
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
		if tab.Screen != nil {
			m.screenCtx.HasTabs = m.hasDetailTabs()
			m.screenCtx.ContentFocused = true
			newScreen, cmd :=
				tab.Screen.HandleKey(key, msg)
			m.setTabScreen(m.activeTab, newScreen)
			return m, cmd
		}
		return m, nil
	}

	// 6. Section home — all sections are screen-backed
	sec := m.nav.ActiveSection()
	if sec >= 0 && sec < numSections &&
		m.sectionScreens[sec] != nil {
		m.screenCtx.HasTabs = m.hasDetailTabs()
		m.screenCtx.ContentFocused = true
		newScreen, cmd :=
			m.sectionScreens[sec].HandleKey(key, msg)
		m.sectionScreens[sec] = newScreen
		return m, cmd
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
	if idx < 0 || idx >= len(m.ocCtx.Utxos) {
		return
	}
	if m.ocCtx.UtxoSelected[idx] {
		delete(m.ocCtx.UtxoSelected, idx)
	} else {
		m.ocCtx.UtxoSelected[idx] = true
	}
	m.recalcSelectedTotal()
}

func (m *Model) recalcSelectedTotal() {
	m.ocCtx.UtxoSelectedTotal = 0
	m.ocCtx.UtxoOutpoints = nil
	for idx := range m.ocCtx.UtxoSelected {
		if idx < len(m.ocCtx.Utxos) {
			m.ocCtx.UtxoSelectedTotal +=
				m.ocCtx.Utxos[idx].AmountSats
			m.ocCtx.UtxoOutpoints = append(
				m.ocCtx.UtxoOutpoints,
				fmt.Sprintf("%s:%d",
					m.ocCtx.Utxos[idx].Txid,
					m.ocCtx.Utxos[idx].Vout))
		}
	}
}

func (m *Model) clearUtxoSelection() {
	m.ocCtx.UtxoSelected = make(map[int]bool)
	m.ocCtx.UtxoSelectedTotal = 0
	m.ocCtx.UtxoOutpoints = nil
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
		// Theme toggle — only responds to Enter.
		// Right arrow is ignored on the toggle.
		if m.nav.IsOnThemeToggle() {
			if key == "enter" {
				mode := theme.Toggle()
				m.cfg.Theme = mode
				m.saveCfg()
				m.nav.UpdateThemeLabel()
			}
			return m, nil
		}
		sec := m.nav.Activate()
		m.focusContent()
		m.activeTab = 0
		m.tabFocused = false
		m.tabCursorX = 0
		return m.previewSection(sec)
	}
	return m, nil
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
		m.focusContent()
		if m.activeTab == 0 {
			m.subview = svNone
		}
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
		// Enter on tab label: focus content
		m.focusContent()
		if m.activeTab == 0 {
			m.subview = svNone
		}
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

	// Screens own all subview state; just clear the
	// Model-level subview flag on any tab close.
	m.subview = svNone

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

	// Addon detail tabs: return to parent manage tab.
	// All other tabs: return to section home (tab 0).
	parentKind := tabKind(-1)
	switch closingTab.Kind {
	case tabSyncthingDevice, tabSyncthingWebUI,
		tabSyncthingPair:
		parentKind = tabSyncthing
	case tabLndHubAccount:
		parentKind = tabLndHub
	case tabLndHubCreate:
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

// routeToSectionScreen delivers a message to the section
// home screen at the given index. Same pattern as
// routeToScreen but keyed on section index instead of
// tab kind. Returns (model, cmd, true) if routed, or
// (model, nil, false) if no screen is mounted.
func (m Model) routeToSectionScreen(
	sec int, msg tea.Msg,
) (Model, tea.Cmd, bool) {
	if sec < 0 || sec >= numSections ||
		m.sectionScreens[sec] == nil {
		return m, nil, false
	}
	m.screenCtx.HasTabs = m.hasDetailTabs()
	newScreen, cmd :=
		m.sectionScreens[sec].HandleMsg(msg)
	m.sectionScreens[sec] = newScreen
	return m, cmd, true
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
