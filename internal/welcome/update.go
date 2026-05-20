package welcome

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

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

// rememberTabPosition saves the current activeTab into
// sectionFocus for the current section, so the next
// "up from sidebar" or focusTabBarMsg-from-home-screen
// restores it. Guards against saving 0, which would
// clobber existing memory with a value the restore
// logic treats as "no memory."
//
// Call this from any code path that *intentionally*
// moves the cursor onto a detail tab and wants that
// position remembered: section-exit, tab-bar→sidebar
// boundary, openTabMsg, closeTab.
func (m *Model) rememberTabPosition() {
	if m.activeTab <= 0 {
		return
	}
	sec := m.nav.ActiveSection()
	if sec < 0 || sec >= numSections {
		return
	}
	m.sectionFocus[sec] = m.activeTab
}

// activateTab delivers a tabActivatedMsg to the active
// tab's screen, giving it a chance to refresh stale data.
// Returns the screen's cmd (typically a fetch) or nil if
// no screen is mounted or the active tab is the section
// home. Only called when the user "commits" to viewing a
// detail tab — not during tab bar browsing or sidebar
// navigation.
func (m *Model) activateTab() tea.Cmd {
	tabs := m.effectiveTabs()
	if m.activeTab <= 0 ||
		m.activeTab >= len(tabs) {
		return nil
	}
	tab := tabs[m.activeTab]
	if tab.Screen == nil {
		return nil
	}
	m.screenCtx.HasTabs = m.hasDetailTabs()
	m.screenCtx.ContentFocused = m.contentFocused
	newScreen, cmd :=
		tab.Screen.HandleMsg(tabActivatedMsg{})
	m.setTabScreen(m.activeTab, newScreen)
	return cmd
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
		// Two cases:
		//  1. Detail screen emitted this — m.activeTab
		//     is already > 0, preserve it (the user
		//     should land on the tab they came from).
		//  2. Home screen emitted this — m.activeTab
		//     is 0, restore from sectionFocus or fall
		//     back to tab 1. Without this, the tab
		//     bar focuses on the home tab and shows
		//     no visible detail-tab cursor.
		if m.activeTab == 0 {
			sec := m.nav.ActiveSection()
			tabs := m.effectiveTabs()
			remembered := m.sectionFocus[sec]
			if remembered >= 1 &&
				remembered < len(tabs) {
				m.activeTab = remembered
			} else if len(tabs) > 1 {
				m.activeTab = 1
			}
		}
		return m, nil
	case focusParentMsg:
		// Navigate to the active tab's parent. If
		// the parent is 0 (section home), focus the
		// section home. Otherwise find the open tab
		// whose kind matches the parent and focus it.
		tabs := m.effectiveTabs()
		if m.activeTab > 0 &&
			m.activeTab < len(tabs) {
			parent := tabs[m.activeTab].Parent
			if parent != 0 {
				for i, t := range tabs {
					if t.Kind == parent {
						m.activeTab = i
						m.focusContent()
						return m, m.activateTab()
					}
				}
			}
		}
		// Parent is section home or not found —
		// fall back to section home.
		m.activeTab = 0
		m.focusContent()
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
	case openTabMsg:
		// Dedup by kind + index if Index is set
		if msg.Index != 0 {
			tabs := m.effectiveTabs()
			for i, t := range tabs {
				if t.Kind == msg.Kind &&
					t.Index == msg.Index {
					m.activeTab = i
					m.rememberTabPosition()
					if msg.FocusTabBar {
						m.focusTabBar()
						m.tabCursorX = 0
						return m, nil
					}
					m.focusContent()
					return m, m.activateTab()
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
					m.rememberTabPosition()
					if msg.Replace &&
						msg.Screen != nil {
						m.setTabScreen(i, msg.Screen)
					}
					if msg.FocusTabBar {
						m.focusTabBar()
						m.tabCursorX = 0
					} else {
						m.focusContent()
					}
					if msg.Replace &&
						msg.Screen != nil {
						return m, msg.Screen.Init()
					}
					if !msg.FocusTabBar {
						return m, m.activateTab()
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
			Parent:  msg.Parent,
			Screen:  msg.Screen,
		})
		m.activeTab = len(m.effectiveTabs()) - 1
		m.rememberTabPosition()
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
		m.routeToSectionScreen(secSystem, msg)
		return m, fetchStatus(m.cfg, m.lndClient)
	case pkgUpdateDoneMsg:
		m.routeToSectionScreen(secSystem, msg)
		return m, fetchStatus(m.cfg, m.lndClient)
	case statusMsg:
		m.fetchInFlight = false
		m.status = &msg
		m.screenCtx.Status = m.status
		// walletDetected can only fire when lndClient
		// is non-nil (see status.go). lndClient stays
		// nil until walletCreatedMsg runs — so during
		// the wallet-create flow this branch is dead,
		// and the in-place tab transform in
		// walletCreatedMsg is the only path that moves
		// the user off the wallet-create screen. See
		// the invariant comment in NewModel.
		if msg.walletDetected && !m.cfg.WalletCreated {
			m.cfg.WalletCreated = true
			m.saveCfg()
		}
		return m, nil
	case latestVersionMsg:
		m.latestVersion = string(msg)
		m.screenCtx.LatestVersion = string(msg)
		return m, nil
	case syncthingPairedMsg:
		if msg.err == nil {
			m.cfg.AddSyncthingDevice(msg.deviceID)
			m.saveCfg()
		}
		// Route to screen for step/error state
		return m.dispatchToTab(tabSyncthingPair, msg)
	case syncthingRemovedMsg:
		if msg.err == nil {
			if m.cfg.RemoveSyncthingDevice(msg.deviceID) {
				m.saveCfg()
			}
		}
		// Route to screen — screen emits closeTab
		// on success, sets error on failure
		return m.dispatchToTab(tabSyncthingDevice, msg)
	case channelOpenResultMsg:
		return m.dispatchToTab(tabOpenChannel, msg)
	case newAddressMsg:
		return m.dispatchToTab(tabOCReceive, msg)
	case invoiceCreatedMsg:
		return m.dispatchToTab(tabReceive, msg)
	case invoiceSettledMsg:
		return m.dispatchToTab(tabReceive, msg)
	case payReqDecodedMsg:
		return m.dispatchToTab(tabSend, msg)
	case sendPaymentResultMsg:
		return m.dispatchToTab(tabSend, msg)
	case paymentHistoryMsg:
		// Route to wallet home screen
		if cmd, ok := m.routeToSectionScreen(
			secWallet, msg); ok {
			return m, cmd
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
		return m.dispatchToTab(tabOnChain, msg)
	case closeChannelMsg:
		// Broadcast to all channel detail tabs — only
		// the one with an active close flow (embedded
		// ChannelCloseScreen) will consume the message.
		// Broadcast is needed because multiple detail
		// tabs can be open simultaneously, and
		// dispatchToTab's first-match would miss the
		// active close flow if it's not on the first
		// detail tab.
		tabs := m.effectiveTabs()
		var cmds []tea.Cmd
		for i, tab := range tabs {
			if tab.Kind == tabChannel &&
				tab.Screen != nil {
				m.screenCtx.HasTabs = m.hasDetailTabs()
				m.screenCtx.ContentFocused =
					m.contentFocused
				newScreen, cmd :=
					tab.Screen.HandleMsg(msg)
				m.setTabScreen(i, newScreen)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
		return m, tea.Batch(cmds...)
	case closedChannelsMsg:
		// Route to history screen so it gets the data
		return m.dispatchToTab(tabChannelHistory, msg)
	case labelTxMsg:
		// Route to on-chain home screen
		if cmd, ok := m.routeToSectionScreen(
			secOnChain, msg); ok {
			return m, cmd
		}
		if msg.err == nil {
			return m, fetchOnChainTxCmd(m.lndClient)
		}
		return m, nil
	case feeTiersMsg:
		if msg.err != nil {
			return m, nil
		}
		m.ocCtx.SendFeeTiers = msg.tiers
		// Fan out to every screen that may want fee
		// tiers. Each routeToScreen call returns an
		// updated Model and a possibly-nil cmd; thread
		// the model through and batch the cmds so none
		// get dropped.
		//
		// The m = rm threading looks redundant because
		// routeToScreen's screen-writeback goes through
		// the m.tabs slice (shared backing array) and
		// its screenCtx mutations go through a pointer,
		// so today the discarded returns would still
		// land in shared memory. The threading is
		// defensive: the day someone adds a non-pointer
		// Model mutation to routeToScreen, this loop
		// stays correct without needing a second look.
		// Contrast with routeToSectionScreen, which
		// writes to the m.sectionScreens array and was
		// converted to a pointer receiver for the same
		// reason.
		var cmds []tea.Cmd
		for _, kind := range []tabKind{
			tabChannel, tabOnChain,
		} {
			rm, cmd, ok := m.routeToScreen(kind, msg)
			if !ok {
				continue
			}
			m = rm
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	case feeEstimateMsg:
		return m.dispatchToTab(tabOnChain, msg)
	case installStepDoneMsg:
		// Route to whichever install flow tab is open.
		// Only one install runs at a time, so first
		// match wins.
		return m.dispatchToFirstTab([]tabKind{
			tabSyncthingInstall,
			tabP2PUpgrade, tabSelfUpdate,
		}, msg)
	case autoUnlockSetupDoneMsg:
		return m.dispatchToTab(tabAutoUnlock, msg)
	case autoUnlockDisableDoneMsg:
		return m.dispatchToTab(tabAutoUnlock, msg)
	case sshKeysListMsg:
		return m.dispatchToTab(tabSSHKeys, msg)
	case sshKeyAddMsg:
		return m.dispatchToTab(tabSSHKeyAdd, msg)
	case sshKeyRemoveMsg:
		return m.dispatchToTab(tabSSHKeyDetail, msg)
	case sshPwAuthDoneMsg:
		// Cfg was mutated by SetSSHPasswordAuth before
		// the msg was returned; persist before routing.
		m.saveCfg()
		return m.dispatchToTab(tabSSHPasswordAuth, msg)
	case changePwDoneMsg:
		return m.dispatchToTab(tabSSHChangePassword, msg)
	case walletLNDReadyMsg:
		return m.dispatchToTab(tabWalletCreate, msg)
	case walletExecDoneMsg:
		return m.dispatchToTab(tabWalletCreate, msg)
	case walletCreatedMsg:
		// Wallet was successfully created. Persist
		// the flag, create the lndClient (it didn't
		// exist before this point because NewModel
		// only constructs it when the wallet already
		// exists), and transform the wallet creation
		// tab in place into an AutoUnlockScreen so
		// the user goes straight from "I SAVED MY
		// SEED" into auto-unlock setup.
		m.cfg.WalletCreated = true
		m.saveCfg()
		if m.lndClient == nil && m.cfg.HasLND() {
			m.lndClient = lndrpc.New(m.cfg.Network)
			m.screenCtx.LndClient = m.lndClient
		}
		// Find the wallet creation tab and transform
		// it. We mutate m.tabs directly because the
		// effectiveTabs() view is computed on demand.
		for i := range m.tabs {
			if m.tabs[i].Kind == tabWalletCreate {
				newScreen :=
					NewAutoUnlockScreen(m.screenCtx)
				m.tabs[i].Kind = tabAutoUnlock
				m.tabs[i].Label = "Auto-Unlock"
				m.tabs[i].Screen = newScreen
				return m, tea.Batch(
					fetchStatus(m.cfg, m.lndClient),
					newScreen.Init(),
				)
			}
		}
		// Tab not found (shouldn't happen, but be
		// defensive). Just refresh status.
		return m, fetchStatus(m.cfg, m.lndClient)
	case tickMsg:
		if m.fetchInFlight {
			return m, tickEveryCmd(m.pollInterval())
		}
		m.fetchInFlight = true
		return m, tea.Batch(
			fetchStatus(m.cfg, m.lndClient),
			tickEveryCmd(m.pollInterval()))
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
			// Restore last tab position for this
			// section, falling back to tab 1 if no
			// memory yet or if the saved index is
			// out of range.
			sec := m.nav.ActiveSection()
			tabs := m.effectiveTabs()
			remembered := m.sectionFocus[sec]
			if remembered >= 1 &&
				remembered < len(tabs) {
				m.activeTab = remembered
			} else if m.activeTab < 1 {
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
		// Save the current section's tab position
		// before Activate switches us away. After
		// Activate, m.nav.ActiveSection() will return
		// the new section, so we have to capture the
		// old section's index here.
		m.rememberTabPosition()
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
	case secChannels:
		return m,
			fetchStatus(m.cfg, m.lndClient)
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
		return m, m.activateTab()

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
		// Save current position before resetting so
		// the next "up from sidebar" restores it.
		m.rememberTabPosition()
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
		return m, nil

	case "enter":
		if m.tabCursorX == 1 && m.activeTab > 0 {
			return m.closeTab(m.activeTab)
		}
		// Enter on tab label: focus content
		m.focusContent()
		if m.activeTab == 0 {
			m.subview = svNone
		}
		return m, m.activateTab()

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

	// Build a set of tabs to remove. Always includes
	// the closing tab itself; if the closing tab is a
	// parent, also includes all tabs in the same
	// section whose Parent matches the closing tab's
	// kind. Cascade is silent — no confirmation.
	//
	// Async results that arrive after a child is
	// cascade-closed will land in routeToScreen,
	// find no matching tab, and be dropped silently.
	// This is safe for the existing flows because
	// state-changing async messages
	// (syncthingPairedMsg, syncthingRemovedMsg)
	// update m.cfg directly in their handlers, not
	// only in the screen handler — so the side
	// effect persists even if the tab is gone.
	// Future flows that change state only via screen
	// handlers would break this assumption.
	shouldRemove := func(t openTab) bool {
		if t.Section != closingTab.Section {
			return false
		}
		if t.Kind == closingTab.Kind &&
			t.Index == closingTab.Index {
			return true
		}
		// Cascade: remove children whose Parent is
		// the closing tab's kind.
		if t.Parent == closingTab.Kind {
			return true
		}
		return false
	}

	var newTabs []openTab
	for _, t := range m.tabs {
		if shouldRemove(t) {
			continue
		}
		newTabs = append(newTabs, t)
	}
	m.tabs = newTabs

	m.tabCursorX = 0

	// Close-to-neighbor: land on whatever tab now
	// occupies the closed parent's index. If that
	// index is past the new end, clamp to the last
	// tab. If no detail tabs remain, fall back to
	// the section home (index 0).
	//
	// This single rule replaces the previous
	// addon-parent special case. Closing a Syncthing
	// device with no siblings now lands on Syncthing
	// manage (which is at the same neighbor index by
	// virtue of being adjacent in the tab list);
	// closing it with siblings lands on the next
	// sibling instead — which is what a Sparrow /
	// browser user expects.
	newTabCount := len(m.effectiveTabs())
	if newTabCount > 1 {
		landing := tabIdx
		if landing >= newTabCount {
			landing = newTabCount - 1
		}
		m.activeTab = landing
		m.focusTabBar()
	} else {
		// No detail tabs left — fall back to section
		// home. Focus content since there's no tab bar.
		m.activeTab = 0
		m.focusContent()
	}

	// Save the resolved landing index into
	// sectionFocus so the next "up from sidebar"
	// lands here too. This intentionally bypasses
	// rememberTabPosition because we want to *clear*
	// memory when no detail tabs remain (write 0),
	// not preserve stale memory the way the helper's
	// guard does.
	sec := closingTab.Section
	if sec >= 0 && sec < numSections {
		m.sectionFocus[sec] = m.activeTab
	}

	// Scroll correctness: keep tabScrollOffset in
	// agreement with where m.activeTab now points.
	// The renderTabBar pass compensates for stale
	// offsets on the fly, so skipping this would not
	// be user-visible today — but the model field
	// would drift from rendered state, which will
	// burn any future tab-system test that asserts
	// on tabScrollOffset directly. Mirror the two
	// invariants the "left" key handler maintains:
	//
	//   1. If no detail tabs remain, reset to 0.
	//   2. Otherwise, if the active tab is now to
	//      the left of the current offset, pull the
	//      offset backward to make it visible.
	//
	// The original upper-bound clamp (don't let the
	// offset point past the new end) is also kept.
	if m.activeTab == 0 {
		m.tabScrollOffset = 0
	} else if m.activeTab-1 < m.tabScrollOffset {
		m.tabScrollOffset = m.activeTab - 1
		if m.tabScrollOffset < 0 {
			m.tabScrollOffset = 0
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
//
// screenCtx.HasTabs and screenCtx.ContentFocused are
// both refreshed before dispatch so HandleMsg sees the
// same view of focus state that HandleKey would. Without
// this, a screen that branches on ContentFocused inside
// HandleMsg (e.g. to decide whether to consume a paste)
// would see whatever the last key event left the flag
// as — silently wrong when the async message arrives
// during sidebar or tab-bar focus.
func (m Model) routeToScreen(
	kind tabKind, msg tea.Msg,
) (Model, tea.Cmd, bool) {
	tabs := m.effectiveTabs()
	for i, tab := range tabs {
		if tab.Kind == kind && tab.Screen != nil {
			m.screenCtx.HasTabs = m.hasDetailTabs()
			m.screenCtx.ContentFocused = m.contentFocused
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
// tab kind. Returns (cmd, true) if routed, or
// (nil, false) if no screen is mounted.
//
// Pointer receiver is load-bearing: m.sectionScreens is
// a fixed-size array, not a slice, so a value receiver
// would write the new screen into a copy and discard it
// on return. This bit us historically when a caller
// forgot to capture the Model return; making the
// receiver a pointer eliminates the class of bug.
// routeToScreen (above) can stay a value receiver
// because it mutates m.tabs, which is a slice and
// shares its backing array across copies.
func (m *Model) routeToSectionScreen(
	sec int, msg tea.Msg,
) (tea.Cmd, bool) {
	if sec < 0 || sec >= numSections ||
		m.sectionScreens[sec] == nil {
		return nil, false
	}
	m.screenCtx.HasTabs = m.hasDetailTabs()
	m.screenCtx.ContentFocused = m.contentFocused
	newScreen, cmd :=
		m.sectionScreens[sec].HandleMsg(msg)
	m.sectionScreens[sec] = newScreen
	return cmd, true
}

// dispatchToTab routes msg to the screen on the tab of the
// given kind and returns the updated model + cmd in the
// shape every Update-switch arm needs. If no matching tab
// is open, returns (m, nil) — the msg is dropped.
//
// This is the boilerplate collapsed: any async message
// whose only job is "deliver to the screen that started
// the work" uses this. Pre-routing state mutations do NOT
// go here — those stay inline in Update so ordering stays
// visible. See go-style-review.md Q4 for the pattern that
// covers cases with both routing and state mutation.
func (m Model) dispatchToTab(
	kind tabKind, msg tea.Msg,
) (Model, tea.Cmd) {
	if rm, cmd, ok := m.routeToScreen(kind, msg); ok {
		return rm, cmd
	}
	return m, nil
}

// dispatchToFirstTab routes msg to the first tab whose
// kind appears in kinds. Returns (m, nil) if none match.
//
// Used when a single async message class can arrive for
// any of several mutually-exclusive tabs (e.g.
// installStepDoneMsg can come from a Syncthing install,
// a P2P upgrade, or a self-update —
// but only one flow runs at a time). Order in kinds is
// the match priority if more than one were somehow open.
func (m Model) dispatchToFirstTab(
	kinds []tabKind, msg tea.Msg,
) (Model, tea.Cmd) {
	for _, k := range kinds {
		if rm, cmd, ok := m.routeToScreen(k, msg); ok {
			return rm, cmd
		}
	}
	return m, nil
}
