package welcome

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) View() tea.View {
	var content string

	if m.width == 0 {
		content = "Loading..."
	} else {
		switch m.subview {
		case svQR:
			content = m.viewQR()
		case svFullURL:
			content = m.viewFullURL()
		case svSyncthingDeviceQR:
			content = m.viewSyncthingDeviceQR()
		case svWalletInfo:
			content = m.viewWalletInfo()
		default:
			content = m.viewMain()
		}
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "Virtual Private Node"
	v.ReportFocus = true
	return v
}

func (m Model) viewMain() string {
	totalW := 82
	totalH := 34

	insideW := totalW - 2
	insideH := totalH - 2

	sidebarW := m.nav.Width
	if sidebarW < 10 {
		sidebarW = 10
	}

	contentW := insideW - sidebarW - 1
	if contentW < 30 {
		contentW = 30
		sidebarW = insideW - 1 - contentW
	}

	tabBarRows := 2
	hasTabContent := m.hasDetailTabs()
	bodyH := insideH - tabBarRows

	// ── Sidebar block heights ─────────────────────
	sepRows := numSections - 1
	sideUsable := bodyH - sepRows

	if sideUsable < numSections {
		sideUsable = numSections
	}

	blockBase := sideUsable / numSections
	blockRem := sideUsable % numSections

	var blockHeights [numSections]int
	for i := 0; i < numSections; i++ {
		blockHeights[i] = blockBase
		if i < blockRem {
			blockHeights[i]++
		}
	}

	blocks := m.nav.BlockRows(sidebarW, blockHeights)

	// ── Tab bar (full width) ──────────────────────
	var tabBar string
	if hasTabContent {
		tabBar = m.renderTabBar(insideW)
		tabBarVW := lipgloss.Width(tabBar)
		if tabBarVW < insideW {
			tabBar += strings.Repeat(" ",
				insideW-tabBarVW)
		} else if tabBarVW > insideW {
			tabBar = tabBar[:insideW]
		}
	} else {
		tabBar = strings.Repeat(" ", insideW)
	}

	// ── Content body ──────────────────────────────
	contentBodyH := bodyH

	rawContent := m.renderActiveTabContent(
		contentW, contentBodyH)

	contentLines := strings.Split(rawContent, "\n")
	for len(contentLines) < contentBodyH {
		contentLines = append(contentLines,
			strings.Repeat(" ", contentW))
	}
	if len(contentLines) > contentBodyH {
		contentLines = contentLines[:contentBodyH]
	}

	border := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	// ── Build frame ───────────────────────────────
	var output []string

	output = append(output, border.Render(
		"╭"+strings.Repeat("─", insideW)+"╮"))

	output = append(output,
		border.Render("│")+
			tabBar+
			border.Render("│"))

	output = append(output, border.Render(
		"├"+strings.Repeat("─", sidebarW)+
			"┬"+strings.Repeat("─", contentW)+
			"┤"))

	// Build sidebar rows (blocks + separators)
	var sideRows []string
	var sideSeps []bool
	for si := 0; si < numSections; si++ {
		for _, row := range blocks[si] {
			sideRows = append(sideRows, row)
			sideSeps = append(sideSeps, false)
		}
		if si < numSections-1 {
			sep := strings.Repeat("─", sidebarW)
			sideRows = append(sideRows, sep)
			sideSeps = append(sideSeps, true)
		}
	}

	// Build content rows
	var contentRows []string
	for _, line := range contentLines {
		contentRows = append(contentRows,
			clampLine(line, contentW))
	}

	for len(sideRows) < bodyH {
		sideRows = append(sideRows,
			strings.Repeat(" ", sidebarW))
		sideSeps = append(sideSeps, false)
	}
	for len(contentRows) < bodyH {
		contentRows = append(contentRows,
			strings.Repeat(" ", contentW))
	}

	for r := 0; r < bodyH; r++ {
		isSideSep := r < len(sideSeps) && sideSeps[r]

		leftEdge := "│"
		middle := "│"
		rightEdge := "│"

		if isSideSep {
			leftEdge = "├"
			middle = "┤"
		}

		sideCell := sideRows[r]
		contentCell := contentRows[r]

		if isSideSep {
			output = append(output,
				border.Render(leftEdge)+
					border.Render(sideCell)+
					border.Render(middle)+
					contentCell+
					border.Render(rightEdge))
		} else {
			output = append(output,
				border.Render(leftEdge)+
					sideCell+
					border.Render(middle)+
					contentCell+
					border.Render(rightEdge))
		}
	}

	output = append(output, border.Render(
		"╰"+strings.Repeat("─", sidebarW)+
			"┴"+strings.Repeat("─", contentW)+"╯"))

	frame := strings.Join(output, "\n")

	helpStr := m.renderHelpBar(totalW)
	helpLine := centerInWidth(helpStr, totalW)

	fullContent := frame + "\n" + helpLine

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		fullContent,
	)
}

func (m Model) hasDetailTabs() bool {
	sec := m.nav.ActiveSection()
	for _, t := range m.tabs {
		if t.Section == sec {
			return true
		}
	}
	return false
}

func (m Model) renderTabBar(maxW int) string {
	tabs := m.effectiveTabs()

	if len(tabs) <= 1 {
		return ""
	}

	type tabRender struct {
		str   string
		width int
	}
	var allTabs []tabRender

	for i := 1; i < len(tabs); i++ {
		tab := tabs[i]
		isCursor := m.tabFocused && m.activeTab == i

		label := tab.Label
		if len(label) > 14 {
			label = label[:12] + ".."
		}

		var s string
		if isCursor && m.tabCursorX == 1 {
			s = navItemStyle.Render(" "+label+" ") +
				navActiveStyle.Render("✕") + " "
		} else if isCursor && m.tabCursorX == 0 {
			s = navActiveStyle.Render(" "+label+" ") +
				"✕ "
		} else {
			s = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Render(" " + label + " ✕ ")
		}

		allTabs = append(allTabs, tabRender{
			str: s, width: lipgloss.Width(s),
		})
	}

	arrowW := 2
	availW := maxW

	offset := m.tabScrollOffset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(allTabs) {
		offset = len(allTabs) - 1
	}
	if offset < 0 {
		offset = 0
	}

	activeIdx := m.activeTab - 1
	if activeIdx < 0 {
		activeIdx = 0
	}
	if activeIdx < offset {
		offset = activeIdx
	}

	needLeftArrow := offset > 0
	usedW := 0
	if needLeftArrow {
		usedW = arrowW
	}

	endIdx := offset
	for endIdx < len(allTabs) {
		w := allTabs[endIdx].width
		rightArrowW := 0
		if endIdx+1 < len(allTabs) {
			rightArrowW = arrowW
		}
		if usedW+w+rightArrowW > availW {
			break
		}
		usedW += w
		endIdx++
	}

	if activeIdx >= endIdx {
		endIdx = activeIdx + 1
		usedW = 0
		offset = endIdx - 1
		for offset > 0 {
			w := allTabs[offset-1].width
			if usedW+w+allTabs[offset].width >
				availW-arrowW*2 {
				break
			}
			usedW += allTabs[offset].width
			offset--
		}
		needLeftArrow = offset > 0
		usedW = 0
		if needLeftArrow {
			usedW = arrowW
		}
		endIdx = offset
		for endIdx < len(allTabs) {
			w := allTabs[endIdx].width
			rightArrowW := 0
			if endIdx+1 < len(allTabs) {
				rightArrowW = arrowW
			}
			if usedW+w+rightArrowW > availW {
				break
			}
			usedW += w
			endIdx++
		}
	}

	needRightArrow := endIdx < len(allTabs)

	var parts []string
	if needLeftArrow {
		parts = append(parts,
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Render("◀ "))
	}

	for i := offset; i < endIdx; i++ {
		parts = append(parts, allTabs[i].str)
	}

	if needRightArrow {
		parts = append(parts,
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Render(" ▶"))
	}

	return strings.Join(parts, "")
}

func (m Model) effectiveTabs() []openTab {
	sec := m.nav.ActiveSection()
	mainLabel := "Channels"
	switch sec {
	case secWallet:
		mainLabel = "Wallet"
	case secOnChain:
		mainLabel = "On-Chain"
	case secAddons:
		mainLabel = "Add-ons"
	case secSystem:
		mainLabel = "System"
	}

	tabs := []openTab{
		{Kind: tabMain, Label: mainLabel, Section: sec},
	}
	for _, t := range m.tabs {
		if t.Section == sec {
			tabs = append(tabs, t)
		}
	}
	return tabs
}

func (m Model) renderActiveTabContent(
	w, h int,
) string {
	tabs := m.effectiveTabs()
	idx := m.activeTab
	if idx < 0 || idx >= len(tabs) {
		idx = 0
	}

	tab := tabs[idx]

	switch tab.Kind {
	case tabMain:
		return m.renderContent(w, h)
	case tabChannel:
		if m.status != nil &&
			tab.Index < len(m.status.channels) {
			saved := m.chanCursor
			m.chanCursor = tab.Index
			result := m.channelDetailPane(w)
			m.chanCursor = saved
			return result
		}
		return theme.Dim.Render(" Channel not found")
	case tabPayment:
		if tab.Index < len(m.payHistory) {
			saved := m.payHistoryCursor
			m.payHistoryCursor = tab.Index
			result := m.paymentDetailContent(w)
			m.payHistoryCursor = saved
			return result
		}
		return theme.Dim.Render(" Payment not found")
	case tabSend:
		switch m.subview {
		case svSendConfirm:
			return m.walletSendConfirmPane(w)
		case svSendInFlight:
			return m.walletSendInFlightPane(w)
		case svSendResult:
			return m.walletSendResultPane(w)
		default:
			return m.walletSendPane(w)
		}
	case tabReceive:
		switch m.subview {
		case svReceiveWaiting:
			return m.walletReceiveWaitingPane(w)
		case svReceivePaid:
			return m.walletReceivePaidPane(w)
		case svReceiveExpired:
			return m.walletReceiveExpiredPane(w)
		default:
			return m.walletReceivePane(w)
		}
	case tabPairing:
		return m.pairingContent(w, h)
	case tabOnChain:
		switch m.subview {
		case svOnChainSendAddr:
			return m.onChainSendAddrPane(w)
		case svOnChainSendAmount:
			return m.onChainSendAmountPane(w)
		case svOnChainSendConfirm:
			return m.onChainSendConfirmPane(w)
		case svOnChainSendBroadcast:
			return m.onChainSendBroadcastPane(w)
		case svOnChainResult:
			return m.onChainResultContent(w)
		default:
			return m.onChainSendAddrPane(w)
		}
	case tabOnChainTx:
		if tab.Index < len(m.onChainTxs) {
			return m.onChainTxDetailPane(
				m.onChainTxs[tab.Index], w)
		}
		return theme.Dim.Render(
			" Transaction not found")
	case tabOCReceive:
		return m.onChainReceivePane(w)
	case tabOpenChannel:
		return m.channelOpenContent(w)
	case tabSyncthing:
		return m.renderSyncthingTabContent(w, h)
	case tabLndHub:
		return m.renderLndHubTabContent(w, h)
	}
	return m.renderContent(w, h)
}

func (m Model) renderContent(w, h int) string {
	sec := m.nav.ActiveSection()

	switch sec {
	case secChannels:
		return m.renderChannelsContent(w, h)
	case secWallet:
		return m.renderWalletContent(w, h)
	case secOnChain:
		return m.renderOnChainContent(w, h)
	case secAddons:
		return m.renderAddonsContent(w, h)
	case secSystem:
		return m.renderSystemContent(w, h)
	}
	return ""
}

func (m Model) renderChannelsContent(w, h int) string {
	return m.channelsOverview(w, h)
}

func (m Model) renderWalletContent(w, h int) string {
	switch m.subview {
	case svSend:
		return m.walletSendPane(w)
	case svSendConfirm:
		return m.walletSendConfirmPane(w)
	case svSendInFlight:
		return m.walletSendInFlightPane(w)
	case svSendResult:
		return m.walletSendResultPane(w)
	case svReceive:
		return m.walletReceivePane(w)
	case svReceiveWaiting:
		return m.walletReceiveWaitingPane(w)
	case svReceivePaid:
		return m.walletReceivePaidPane(w)
	case svReceiveExpired:
		return m.walletReceiveExpiredPane(w)
	case svWalletPairing:
		return m.pairingContent(w, h)
	case svPaymentDetail:
		return m.paymentDetailContent(w)
	}
	return m.walletOverview(w, h)
}

func (m Model) renderOnChainContent(w, h int) string {
	switch m.subview {
	case svOnChainReceive:
		return m.onChainReceivePane(w)
	case svOnChainResult:
		return m.onChainResultContent(w)
	case svOnChainSendAddr:
		return m.onChainSendAddrPane(w)
	case svOnChainSendAmount:
		return m.onChainSendAmountPane(w)
	case svOnChainSendConfirm:
		return m.onChainSendConfirmPane(w)
	case svOnChainSendBroadcast:
		return m.onChainSendBroadcastPane(w)
	}
	return m.onChainOverview(w, h)
}

func (m Model) renderAddonsContent(w, h int) string {
	switch m.subview {
	case svSyncthingDetail:
		return m.syncthingDetailContent(w, h)
	case svSyncthingPairInput:
		return m.syncthingPairContent(w)
	case svSyncthingWebUI:
		return m.syncthingWebUIContent(w)
	case svSyncthingDeviceDetail:
		return m.syncthingDeviceDetailContent(w)
	case svLndHubManage:
		return m.lndhubManageContent(w, h)
	case svLndHubCreateName:
		return m.lndhubCreateNameContent(w)
	case svLndHubCreateAccount:
		return m.lndhubCreatedContent(w)
	case svLndHubAccountDetail:
		return m.lndhubAccountDetailContent(w)
	case svLndHubDeactivateConfirm:
		return m.lndhubDeactivateContent(w)
	}
	return m.addonsOverview(w, h)
}

func (m Model) renderSystemContent(w, h int) string {
	return m.systemOverview(w, h)
}

func (m Model) viewFullURL() string {
	title := theme.Header.Render(
		"Full URL — Copy and paste into Tor Browser")
	hint := theme.Dim.Render(
		"Select and copy. Press backspace to go back.")
	content := lipgloss.JoinVertical(lipgloss.Left,
		"", title, "", hint, "", m.urlTarget, "")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, content)
}

func (m Model) viewWalletInfo() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	lines = append(lines,
		theme.Lightning.Render("Wallet & Node Info"), "")

	if m.cfg.WalletExists() {
		lines = append(lines,
			"  "+theme.Label.Render("Wallet: ")+
				theme.Success.Render("created"))
		if m.cfg.AutoUnlock {
			lines = append(lines,
				"  "+theme.Label.Render("Auto-unlock: ")+
					theme.Success.Render("enabled"))
		}
		lines = append(lines,
			"  "+theme.Label.Render("P2P Mode: ")+
				theme.Value.Render(
					p2pModeLabel(m.cfg.P2PMode)))
		if m.status != nil && m.status.lndResponding {
			if m.status.lndBalance != "" {
				lines = append(lines,
					"  "+theme.Label.Render("Balance: ")+
						theme.Value.Render(
							m.status.lndBalance+" sats"))
			}
			if m.status.lndPubkey != "" {
				lines = append(lines, "",
					"  "+theme.Label.Render("Pubkey:"),
					"  "+theme.Mono.Render(
						m.status.lndPubkey))
			}
		} else {
			lines = append(lines, "",
				"  "+theme.Dim.Render(
					"Waiting for LND..."))
		}
	} else {
		lines = append(lines,
			"  "+theme.Warning.Render(
				"Wallet not created"))
	}

	content := strings.Join(lines, "\n")
	box := theme.Box.Width(bw).Padding(1, 2).
		Render(content)
	footer := theme.Footer.Render(
		"  backspace back  q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

// ── Helpers ──────────────────────────────────────────────

func clampLine(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s + strings.Repeat(" ",
			w-lipgloss.Width(s))
	}
	r := []rune(s)
	if len(r) > w {
		r = r[:w-1]
		return string(r) + "…"
	}
	return string(r)
}

func centerInWidth(s string, w int) string {
	vis := lipgloss.Width(s)
	if vis >= w {
		return s
	}
	left := (w - vis) / 2
	right := w - vis - left
	return strings.Repeat(" ", left) + s +
		strings.Repeat(" ", right)
}

func padLines(lines []string, target int) string {
	for len(lines) < target {
		lines = append(lines, "")
	}
	if len(lines) > target {
		lines = lines[:target]
	}
	return strings.Join(lines, "\n")
}

func parseBalance(s string) int64 {
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	return n
}
