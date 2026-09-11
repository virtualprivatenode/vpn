package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/lndrpc"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ── First-run verification banner (ruling xvi) ───────────
//
// While the root-private verification marker exists, the layout shows a
// banner asking the operator to verify SSH access from a SECOND
// terminal. It clears only on journal evidence of a real sshd
// login for the admin user — the in-session handoff console is
// deliberately not evidence. The check rides the status tick and
// costs one privileged journal read per poll while pending.

type adminLoginVerifiedMsg struct {
	pending bool
	err     error
}

func checkAdminLoginCmd() tea.Cmd {
	return func() tea.Msg {
		var result helper.VerifyAdminLoginResult
		err := helper.Call(helper.VerbVerifyAdminLogin, nil, &result)
		return adminLoginVerifiedMsg{pending: result.Pending, err: err}
	}
}

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
		if m.cfg.HasLND() && m.state.WalletKnown &&
			m.state.WalletExists &&
			m.lndClient != nil {
			return m, tea.Batch(
				fetchStatus(m.cfg, m.state, m.lndClient),
				fetchPaymentHistoryCmd(m.lndClient),
				fetchWalletStateCmd(m.screenCtx),
				fetchKeyVerificationStateCmd())
		}
		return m, tea.Batch(
			fetchStatus(m.cfg, m.state, m.lndClient),
			fetchWalletStateCmd(m.screenCtx),
			fetchKeyVerificationStateCmd())
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
	case closeSSHScreenMsg:
		if msg.screen == nil || sshAccessBusy(msg.screen) {
			return m, nil
		}
		return m.closeScreenTab(msg.screen)
	case closeTabMsg:
		return m.closeTab(m.activeTab)
	case closeOnChainSendMsg:
		if msg.screen == nil || msg.screen.step != ocStepResult {
			return m, nil
		}
		return m.closeScreenTab(msg.screen)
	case closeChannelOpenMsg:
		if msg.screen == nil || msg.screen.step != coStepResult {
			return m, nil
		}
		return m.closeScreenTab(msg.screen)
	case closeChannelDoneMsg:
		if msg.screen == nil || msg.screen.step != closeStepResult {
			return m, nil
		}
		for _, tab := range m.tabs {
			if detail, ok := tab.Screen.(*ChannelDetailScreen); ok && detail.closeScreen == msg.screen {
				return m.closeScreenTab(detail)
			}
		}
		return m, nil
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
		return m, fetchStatus(m.cfg, m.state, m.lndClient)
	case openTabMsg:
		if msg.Kind == tabWalletCreate {
			// One wallet belongs to this node, even when opened from another section.
			for _, tab := range m.tabs {
				if tab.Kind != tabWalletCreate {
					continue
				}
				m.nav.ActiveItem, m.nav.Cursor = tab.Section, tab.Section
				for i, visible := range m.effectiveTabs() {
					if visible.Screen == tab.Screen {
						m.activeTab = i
						m.rememberTabPosition()
						m.focusContent()
						return m, m.activateTab()
					}
				}
			}
		}
		if msg.Kind == tabSyncthingDevice {
			detail, ok := msg.Screen.(*SyncthingDeviceScreen)
			if !ok || msg.Key == "" || detail.device.DeviceID != msg.Key || m.nav.ActiveSection() != secAddons {
				return m, nil
			}
			for i, tab := range m.effectiveTabs() {
				if tab.Kind == tabSyncthingDevice && tab.Key == msg.Key {
					m.activeTab = i
					m.rememberTabPosition()
					m.focusContent()
					return m, m.activateTab()
				}
			}
		}

		if msg.Kind == tabSSHKeyDetail {
			detail, ok := msg.Screen.(*SSHKeyDetailScreen)
			if !ok || msg.Key == "" || detail.keyInfo.Fingerprint != msg.Key || m.nav.ActiveSection() != secSystem {
				return m, nil
			}
			for i, tab := range m.effectiveTabs() {
				if tab.Kind == tabSSHKeyDetail && tab.Key == msg.Key {
					m.activeTab = i
					m.rememberTabPosition()
					m.focusContent()
					return m, m.activateTab()
				}
			}
		}

		if msg.Kind == tabChannel {
			detail, ok := msg.Screen.(*ChannelDetailScreen)
			if !ok || msg.Key == "" || detail.channel.ChannelPoint != msg.Key || m.nav.ActiveSection() != secChannels {
				return m, nil
			}
			for i, tab := range m.effectiveTabs() {
				if tab.Kind == tabChannel && tab.Key == msg.Key {
					m.activeTab = i
					m.rememberTabPosition()
					m.focusContent()
					return m, m.activateTab()
				}
			}
		}
		// Dedup by kind + index if Index is set
		if msg.Kind != tabChannel && msg.Kind != tabSSHKeyDetail && msg.Kind != tabSyncthingDevice && msg.Index != 0 {
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
		if msg.Kind != tabChannel && msg.Kind != tabSSHKeyDetail && msg.Kind != tabSyncthingDevice && msg.Index == 0 {
			sec := m.nav.ActiveSection()
			tabs := m.effectiveTabs()
			for i, t := range tabs {
				if t.Kind == msg.Kind &&
					t.Section == sec {
					m.activeTab = i
					m.rememberTabPosition()
					if msg.Replace && (walletCreationBusy(t.Screen) || onChainSendBusy(t.Screen) || channelOpenBusy(t.Screen) || m.sshTabBusy(t) || m.helperTabBusy(t) || m.syncthingTabBusy(t)) {
						return m, nil
					}
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
			Key:     msg.Key,
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
		return m, fetchStatus(m.cfg, m.state, m.lndClient)
	case pkgUpdateDoneMsg:
		m.routeToSectionScreen(secSystem, msg)
		return m, fetchStatus(m.cfg, m.state, m.lndClient)
	case statusMsg:
		m.fetchInFlight = false
		m.status = &msg
		m.screenCtx.Status = m.status
		return m, nil
	case latestVersionMsg:
		m.latestVersion = string(msg)
		m.screenCtx.LatestVersion = string(msg)
		return m, nil
	case nodeAddressesMsg:
		// Live-read answer — deliver to the screen that asked
		// (it recorded which tab kind it lives on).
		return m.dispatchToTab(msg.tab, msg)
	case walletStateMsg:
		if msg.owner != m.screenCtx || msg.revision != m.screenCtx.walletRevision || walletCreationBusy(m.screenCtx.walletCreationOwner) {
			return m, nil
		}
		if msg.err != nil {
			m.state.WalletKnown = false
			logger.TUI("read live wallet state: %v", msg.err)
			return m, nil
		}
		m.state.WalletExists = msg.state.WalletExists
		m.state.WalletKnown = true
		if m.state.WalletExists && m.lndClient == nil &&
			m.cfg.HasLND() && m.screenCtx.walletCreationOwner == nil {
			m.lndClient = lndrpc.New()
			m.screenCtx.LndClient = m.lndClient
			return m, fetchStatus(m.cfg, m.state, m.lndClient)
		}
		return m, nil
	case keyVerificationStateMsg:
		if msg.err != nil {
			m.state.KeyVerificationKnown = false
			logger.TUI("read key-verification state: %v", msg.err)
			return m, nil
		}
		m.state.KeyVerificationPending = msg.state.Pending
		m.state.KeyVerificationKnown = true
		return m, nil
	case syncthingWebPasswordMsg:
		return m.dispatchToTab(tabSyncthingWebUI, msg)
	case syncthingPairedMsg:
		if msg.owner == nil || msg.owner.attempt != msg.attempt || msg.owner.step != syncPairStepPairing {
			return m, nil
		}
		rm, cmd := m.routeSyncthingResult(msg.owner, msg)
		return rm, tea.Batch(cmd, fetchSyncthingDevicesCmd(m.screenCtx))
	case syncthingRemovedMsg:
		if msg.owner == nil || msg.owner.attempt != msg.attempt || msg.owner.step != syncDeviceStepRemoving {
			return m, nil
		}
		rm, cmd := m.routeSyncthingResult(msg.owner, msg)
		return rm, tea.Batch(cmd, fetchSyncthingDevicesCmd(m.screenCtx))
	case syncthingCloseMsg:
		switch owner := msg.owner.(type) {
		case *SyncthingPairScreen:
			if owner == nil || owner.attempt != msg.attempt || syncthingBusy(owner) {
				return m, nil
			}
		case *SyncthingDeviceScreen:
			if owner == nil || owner.attempt != msg.attempt || syncthingBusy(owner) {
				return m, nil
			}
		default:
			return m, nil
		}
		return m.closeScreenTab(msg.owner)
	case syncthingDevicesMsg:
		if msg.owner != m.screenCtx || msg.revision != m.screenCtx.syncthingRevision {
			return m, nil
		}
		for _, tab := range m.tabs {
			if detail, ok := tab.Screen.(*SyncthingDetailScreen); ok {
				detail.HandleMsg(msg)
			}
		}
		m.state.SyncthingDevices = msg.devices
		m.state.SyncthingDevicesErr = msg.err
		m.state.SyncthingDevicesKnown = msg.err == nil
		return m, nil
	case channelOpenResultMsg:
		return m.dispatchToTab(tabOpenChannel, msg)
	case coUtxoListMsg:
		return m.dispatchToTab(tabOpenChannel, msg)
	case coTxListMsg:
		return m.dispatchToTab(tabOpenChannel, msg)
	case newAddressMsg:
		return m.dispatchToTab(tabOCReceive, msg)
	case invoiceCreatedMsg:
		return m.dispatchToTab(tabReceive, msg)
	case invoiceCheckMsg, invoiceStatusMsg:
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
		}
		return m, nil
	case onChainTxMsg:
		if msg.err == nil {
			m.ocCtx.OnChainTxs = msg.txs
		}
		return m, nil
	case sendCoinsResultMsg:
		return m.dispatchToTab(tabOnChain, msg)
	case channelCloseResultMsg, channelCloseFeesMsg:
		var cmds []tea.Cmd
		for i, tab := range m.tabs {
			if tab.Kind != tabChannel || tab.Screen == nil {
				continue
			}
			m.screenCtx.HasTabs = true
			m.screenCtx.ContentFocused = m.contentFocused && tab.Section == m.nav.ActiveSection()
			screen, cmd := tab.Screen.HandleMsg(msg)
			m.tabs[i].Screen = screen
			if cmd != nil {
				cmds = append(cmds, cmd)
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
		// Keep each screen update and batch the resulting commands.
		var cmds []tea.Cmd
		for _, kind := range []tabKind{
			tabOnChain, tabOpenChannel,
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
	case helperProgressMsg:
		for _, tab := range m.tabs {
			if progress := helperProgress(tab.Screen); progress != nil && progress.operation == msg.operation {
				_, cmd := progress.HandleMsg(msg)
				return m, cmd
			}
		}
		return m, nil
	case closeHelperProgressMsg:
		if msg.screen == nil || !msg.screen.done {
			return m, nil
		}
		for _, tab := range m.tabs {
			if helperProgress(tab.Screen) == msg.screen {
				return m.closeScreenTab(tab.Screen)
			}
		}
		return m, nil
	case autoUnlockSetupDoneMsg:
		return m.dispatchToTab(tabAutoUnlock, msg)
	case autoUnlockDisableDoneMsg:
		return m.dispatchToTab(tabAutoUnlock, msg)
	case refreshSSHKeysMsg:
		var cmds []tea.Cmd
		for _, tab := range m.tabs {
			if screen, ok := tab.Screen.(*SSHKeysScreen); ok {
				cmds = append(cmds, screen.refresh())
			}
		}
		return m, tea.Batch(cmds...)
	case sshKeysListMsg:
		return m.routeSSHResult(msg.owner, msg)
	case sshKeyAddMsg:
		return m.routeSSHResult(msg.owner, msg)
	case sshKeyRemoveMsg:
		return m.routeSSHResult(msg.owner, msg)
	case sshPwAuthDoneMsg:
		return m.routeSSHResult(msg.owner, msg)
	case refreshSSHAuthMsg:
		for _, tab := range m.tabs {
			if screen, ok := tab.Screen.(*SSHPasswordAuthScreen); ok && sshAccessBusy(screen) {
				return m, nil
			}
		}
		m.screenCtx.sshAuthRevision++
		revision, ctx, access := m.screenCtx.sshAuthRevision, m.screenCtx, m.screenCtx.sshAccess()
		return m, func() tea.Msg {
			enabled, err := access.PasswordAuth()
			return sshPwAuthStateMsg{owner: ctx, revision: revision, disabled: !enabled, err: err}
		}
	case sshPwAuthStateMsg:
		if msg.owner != m.screenCtx || msg.revision != m.screenCtx.sshAuthRevision {
			return m, nil
		}
		m.state.SSHPasswordAuthKnown = msg.err == nil
		if msg.err == nil {
			m.state.SSHPasswordAuthDisabled = msg.disabled
		}
		return m, nil
	case changePwDoneMsg:
		return m.routeSSHResult(msg.owner, msg)
	case closeLoginPasswordMsg:
		if msg.owner == nil || msg.owner.attempt != msg.attempt || sshAccessBusy(msg.owner) {
			return m, nil
		}
		return m.closeScreenTab(msg.owner)
	case walletLNDReadyMsg:
		return m.routeWalletCreation(msg.owner, msg)
	case walletExecDoneMsg:
		return m.routeWalletCreation(msg.owner, msg)
	case walletFinalizedMsg:
		return m.finishWalletCreation(msg)
	case closeWalletCreateMsg:
		if msg.owner == nil || msg.owner.attempt != msg.attempt || walletCreationBusy(msg.owner) ||
			(msg.attempt != nil && msg.revision != msg.attempt.finalization) {
			return m, nil
		}
		return m.closeScreenTab(msg.owner)
	case continueWalletCreateMsg:
		if msg.owner == nil || msg.owner.attempt != msg.attempt || msg.owner.step != walletResult || !msg.owner.canContinue() {
			return m, nil
		}
		return m.continueWalletCreation(msg.owner)
	case adminLoginVerifiedMsg:
		if msg.err != nil {
			logger.TUI("verify admin login: %v", msg.err)
		} else {
			m.state.KeyVerificationPending = msg.pending
			m.state.KeyVerificationKnown = true
		}
		return m, nil
	case tickMsg:
		if m.fetchInFlight {
			return m, tickEveryCmd(m.pollInterval())
		}
		m.fetchInFlight = true
		cmds := []tea.Cmd{
			fetchStatus(m.cfg, m.state, m.lndClient),
			fetchWalletStateCmd(m.screenCtx),
			fetchKeyVerificationStateCmd(),
			tickEveryCmd(m.pollInterval()),
		}
		if m.state.KeyVerificationKnown &&
			m.state.KeyVerificationPending {
			cmds = append(cmds, checkAdminLoginCmd())
		}
		return m, tea.Batch(cmds...)
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
				m.prefs.Theme = mode
				m.savePreferences()
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
			fetchStatus(m.cfg, m.state, m.lndClient)
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

// Done belongs to its originating screen even if navigation changes before delivery.
func (m Model) closeScreenTab(screen Screen) (tea.Model, tea.Cmd) {
	for i, tab := range m.tabs {
		if tab.Screen != screen {
			continue
		}
		if tab.Section == m.nav.ActiveSection() {
			for index, visible := range m.effectiveTabs() {
				if visible.Screen == screen {
					return m.closeTab(index)
				}
			}
		} else {
			m.releaseWalletCreation(tab.Screen)
			m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
			m.sectionFocus[tab.Section] = 0
			return m, nil
		}
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
	// Keep submitted operations reachable until their bounded calls return.
	if walletCreationBusy(closingTab.Screen) || onChainSendBusy(closingTab.Screen) || channelOpenBusy(closingTab.Screen) || channelCloseBusy(closingTab.Screen) || m.sshTabBusy(closingTab) || m.helperTabBusy(closingTab) || m.syncthingTabBusy(closingTab) {
		return m, nil
	}

	// Screens own all subview state; just clear the
	// Model-level subview flag on any tab close.
	m.subview = svNone

	// Closing a parent also removes its child tabs in the same section.
	// Future asynchronous results will no longer reach the removed screens.
	shouldRemove := func(t openTab) bool {
		if t.Section != closingTab.Section {
			return false
		}
		if t.Kind == closingTab.Kind &&
			t.Index == closingTab.Index && t.Key == closingTab.Key {
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
			m.releaseWalletCreation(t.Screen)
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
			m.tabs[i].Key == target.Key &&
			m.tabs[i].Section == target.Section {
			m.tabs[i].Screen = s
			return
		}
	}
}

// Payment, invoice, on-chain send and channel-open results reach their open tabs across sections.
// Other workflows retain visible-section routing until their lifecycle is reviewed.
func (m Model) routeToScreen(kind tabKind, msg tea.Msg) (Model, tea.Cmd, bool) {
	section := m.nav.ActiveSection()
	for i, tab := range m.tabs {
		if tab.Section != section && kind != tabSend && kind != tabReceive && kind != tabOnChain && kind != tabOpenChannel {
			continue
		}
		if tab.Kind == kind && tab.Screen != nil {
			m.screenCtx.HasTabs = true
			m.screenCtx.ContentFocused = m.contentFocused && tab.Section == section
			newScreen, cmd := tab.Screen.HandleMsg(msg)
			m.tabs[i].Screen = newScreen
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

// Parent close or replacement must not discard a submitted SSH operation.
func (m Model) sshTabBusy(tab openTab) bool {
	if sshAccessBusy(tab.Screen) {
		return true
	}
	for _, child := range m.tabs {
		if child.Section == tab.Section && child.Parent == tab.Kind && sshAccessBusy(child.Screen) {
			return true
		}
	}
	return false
}

func syncthingBusy(screen Screen) bool {
	switch s := screen.(type) {
	case *SyncthingPairScreen:
		return s.step == syncPairStepPairing
	case *SyncthingDeviceScreen:
		return s.step == syncDeviceStepRemoving
	}
	return false
}
func (m Model) syncthingTabBusy(tab openTab) bool {
	if syncthingBusy(tab.Screen) {
		return true
	}
	for _, child := range m.tabs {
		if child.Section == tab.Section && child.Parent == tab.Kind && syncthingBusy(child.Screen) {
			return true
		}
	}
	return false
}
func (m Model) routeSyncthingResult(owner Screen, msg tea.Msg) (tea.Model, tea.Cmd) {
	for i, tab := range m.tabs {
		if tab.Screen == owner {
			screen, cmd := owner.HandleMsg(msg)
			m.tabs[i].Screen = screen
			return m, cmd
		}
	}
	return m, nil
}
