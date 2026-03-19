package welcome

import (
	"fmt"
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
		case svWalletInfo:
			content = m.viewWalletInfo()
		case svZeus:
			content = m.viewZeus()
		case svSyncthingDetail:
			content = m.viewSyncthingDetail()
		case svSyncthingPairInput:
			content = m.viewSyncthingPairInput()
		case svSyncthingDeviceDetail:
			content = m.viewSyncthingDeviceDetail()
		case svSyncthingWebUI:
			content = m.viewSyncthingWebUI()
		case svSyncthingDeviceQR:
			content = m.viewSyncthingDeviceQR()
		case svChannelDetail:
			content = m.viewChannelDetail()
		case svChannelOpen:
			content = m.viewChannelOpen()
		case svChannelAmountSelect:
			content = m.viewChannelAmountSelect()
		case svChannelCustomPeer:
			content = m.viewChannelCustomPeer()
		case svChannelOpenConfirm:
			content = m.viewChannelOpenConfirm()
		case svChannelOpening:
			content = m.viewChannelOpening()
		case svChannelOpenResult:
			content = m.viewChannelOpenResult()
		case svChannelFundWallet:
			content = m.viewChannelFundWallet()
		case svLndHubManage:
			content = m.viewLndHubManage()
		case svLndHubCreateName:
			content = m.viewLndHubCreateName()
		case svLndHubCreateAccount:
			content = m.viewLndHubNewAccount()
		case svLndHubAccountDetail:
			content = m.viewLndHubAccountDetail()
		case svLndHubDeactivateConfirm:
			content = m.viewLndHubDeactivateConfirm()
		case svQR:
			content = m.viewQR()
		case svFullURL:
			content = m.viewFullURL()
		case svReceive:
			content = m.viewReceive()
		case svReceiveWaiting:
			content = m.viewReceiveWaiting()
		case svReceivePaid:
			content = m.viewReceivePaid()
		case svReceiveExpired:
			content = m.viewReceiveExpired()
		case svSend:
			content = m.viewSend()
		case svSendConfirm:
			content = m.viewSendConfirm()
		case svSendInFlight:
			content = m.viewSendInFlight()
		case svSendResult:
			content = m.viewSendResult()
		case svPaymentHistory:
			content = m.viewPaymentHistory()
		case svPaymentDetail:
			content = m.viewPaymentDetail()
		default:
			bw := min(m.width-4, theme.ContentWidth)
			var tabContent string
			switch m.activeTab {
			case tabDashboard:
				tabContent = m.viewDashboard(bw)
			case tabWallet:
				tabContent = m.viewWalletTab(bw)
			case tabPairing:
				tabContent = m.viewPairing(bw)
			case tabAddons:
				tabContent = m.viewAddons(bw)
			case tabSystem:
				tabContent = m.viewSystem(bw)
			}

			title := theme.Title.Width(bw).Align(lipgloss.Center).
				Render(fmt.Sprintf(" Virtual Private Node v%s ", m.version))
			tabs := m.viewTabs(bw)
			footer := m.viewFooter()
			body := lipgloss.JoinVertical(lipgloss.Center,
				"", title, "", tabs, "", tabContent)
			content = lipgloss.JoinVertical(lipgloss.Center, body, "", footer)
			content = lipgloss.Place(m.width, m.height,
				lipgloss.Center, lipgloss.Center, content)
		}
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) viewTabs(tw int) string {
	tabs := []struct {
		n string
		t wTab
	}{
		{"Dashboard", tabDashboard},
		{"Wallet", tabWallet},
		{"Pairing", tabPairing},
		{"Add-ons", tabAddons},
		{"System", tabSystem},
	}
	w := tw / len(tabs)
	var out []string
	for _, t := range tabs {
		if t.t == m.activeTab {
			out = append(out, theme.ActiveTab.Width(w).
				Align(lipgloss.Center).Render(t.n))
		} else {
			out = append(out, theme.InactiveTab.Width(w).
				Align(lipgloss.Center).Render(t.n))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, out...)
}

func (m Model) viewFooter() string {
	// System tab card-active footers
	if m.cardActive && m.activeTab == tabSystem {
		if m.sysCard == cardServices {
			return theme.Footer.Render(
				"  ↑↓ select • [r]estart [s]top [a]start [l]ogs • backspace back • q quit  ")
		}
		if m.sysCard == cardSysStats {
			if m.status != nil && m.status.rebootRequired {
				return theme.Footer.Render(
					"  [u]pdate • [r]eboot • backspace back • q quit  ")
			}
			return theme.Footer.Render(
				"  [u]pdate system • backspace back • q quit  ")
		}
	}
	switch m.activeTab {
	case tabDashboard:
		return theme.Footer.Render(
			"  tab switch • q quit  ")
	case tabWallet:
		if m.walletFocus == 0 {
			if m.status != nil && len(m.status.channels) > 0 {
				return theme.Footer.Render(
					"  ←→ card • ↑↓ select • enter details • o open channel • tab switch • q quit  ")
			}
			return theme.Footer.Render(
				"  ←→ card • enter open channel • tab switch • q quit  ")
		}
		return theme.Footer.Render(
			"  ←→ card • s send • r receive • v history • enter details • tab switch • q quit  ")
	case tabPairing:
		return theme.Footer.Render(
			"  enter open • tab switch • q quit  ")
	case tabAddons:
		return theme.Footer.Render(
			"  ←→ select • enter install/view • tab switch • q quit  ")
	case tabSystem:
		if m.updateConfirm {
			return theme.Footer.Render(
				"  y confirm • any key cancel  ")
		}
		return theme.Footer.Render(
			"  ↑↓←→ navigate • enter select • tab switch • q quit  ")
	}
	return ""
}

func (m Model) viewFullURL() string {
	title := theme.Header.Render("Full URL — Copy and paste into Tor Browser")
	hint := theme.Dim.Render("Select and copy. Press backspace to go back.")
	content := lipgloss.JoinVertical(lipgloss.Left,
		"", title, "", hint, "", m.urlTarget, "")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, content)
}

func (m Model) viewWalletTab(bw int) string {
	halfW := (bw - 2) / 2
	cardH := theme.BoxHeight

	channelsCard := m.walletChannelsCard(halfW, cardH)
	walletCard := m.walletInfoCard(halfW, cardH)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		channelsCard, "  ", walletCard)
}

func (m Model) walletChannelsCard(w, h int) string {
	if !m.cfg.HasLND() {
		return m.channelsNotInstalledCard(w, h)
	}
	if !m.cfg.WalletExists() {
		return m.channelsNoWalletCard(w, h)
	}
	return m.channelsListCard(w, h)
}

func (m Model) channelsNotInstalledCard(w, h int) string {
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡ Channels"))
	lines = append(lines, "")
	lines = append(lines, theme.Grayed.Render("  Install LND from System tab"))
	border := theme.NormalBorder
	if m.walletFocus == 0 {
		border = theme.GrayedBorder
	}
	return border.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

func (m Model) channelsNoWalletCard(w, h int) string {
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡ Channels"))
	lines = append(lines, "")
	lines = append(lines, theme.Grayed.Render("  Create LND wallet first"))
	border := theme.NormalBorder
	if m.walletFocus == 0 {
		border = theme.GrayedBorder
	}
	return border.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

func (m Model) channelsListCard(w, h int) string {
	var lines []string

	activeCount := 0
	inactiveCount := 0
	if m.status != nil {
		for _, ch := range m.status.channels {
			if ch.Active {
				activeCount++
			} else {
				inactiveCount++
			}
		}
	}

	headerText := "⚡ Channels"
	if m.status != nil && len(m.status.channels) > 0 {
		headerText = fmt.Sprintf("⚡ Channels (%d active", activeCount)
		if inactiveCount > 0 {
			headerText += fmt.Sprintf(", %d offline", inactiveCount)
		}
		if m.status.pendingOpen > 0 {
			headerText += fmt.Sprintf(", %d pending", m.status.pendingOpen)
		}
		headerText += ")"
	}
	lines = append(lines, theme.Lightning.Render(headerText))
	lines = append(lines, "")

	if m.status == nil || !m.status.lndResponding {
		lines = append(lines, "  "+theme.Dim.Render("Waiting for LND..."))
		border := theme.NormalBorder
		if m.walletFocus == 0 {
			border = theme.SelectedBorder
		}
		return border.Width(w).Padding(1, 2).
			Render(padLines(lines, h))
	}

	if len(m.status.channels) == 0 {
		lines = append(lines, "  "+theme.Dim.Render(
			"No channels yet."))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Action.Render(
			"▸ [o] Open Channel"))
		border := theme.NormalBorder
		if m.walletFocus == 0 {
			border = theme.SelectedBorder
		}
		return border.Width(w).Padding(1, 2).
			Render(padLines(lines, h))
	}

	visibleCount := h - 10
	if visibleCount < 3 {
		visibleCount = 3
	}

	if m.chanScrollOffset > 0 {
		lines = append(lines, "  "+theme.Dim.Render("  ↑ more"))
	}

	viewEnd := m.chanScrollOffset + visibleCount
	if viewEnd > len(m.status.channels) {
		viewEnd = len(m.status.channels)
	}

	barWidth := w - 36
	if barWidth < 10 {
		barWidth = 10
	}

	for i := m.chanScrollOffset; i < viewEnd; i++ {
		ch := m.status.channels[i]
		prefix := "  "
		nameStyle := theme.Value
		if m.walletFocus == 0 && m.chanCursor == i {
			prefix = "▸ "
			nameStyle = theme.Action
		}
		dot := theme.RedDot.Render("○")
		if ch.Active {
			dot = theme.GreenDot.Render("●")
		}
		name := ch.PeerAlias
		if name == "" {
			if len(ch.RemotePubkey) > 12 {
				name = ch.RemotePubkey[:12] + "..."
			} else {
				name = ch.RemotePubkey
			}
		}
		if len(name) > 14 {
			name = name[:14]
		}
		name = fmt.Sprintf("%-14s", name)

		bar := renderBalanceBar(ch.LocalBalance, ch.RemoteBalance,
			ch.Capacity, barWidth)
		lines = append(lines, fmt.Sprintf("%s%s %s %s",
			prefix, dot, nameStyle.Render(name), bar))
	}

	if viewEnd < len(m.status.channels) {
		lines = append(lines, "  "+theme.Dim.Render("  ↓ more"))
	}

	lines = append(lines, "")
	var totalLocal, totalRemote int64
	for _, ch := range m.status.channels {
		totalLocal += ch.LocalBalance
		totalRemote += ch.RemoteBalance
	}
	lines = append(lines, "  "+theme.Label.Render("Send: ")+
		theme.Value.Render(formatSats(totalLocal)))
	lines = append(lines, "  "+theme.Label.Render("Recv: ")+
		theme.Value.Render(formatSats(totalRemote)))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Action.Render("[o] Open channel"))

	border := theme.NormalBorder
	if m.walletFocus == 0 {
		border = theme.SelectedBorder
	}
	return border.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

func (m Model) walletInfoCard(w, h int) string {
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡ Wallet"))
	lines = append(lines, "")

	if !m.cfg.HasLND() {
		lines = append(lines, theme.Grayed.Render("  Install LND from System tab"))
		border := theme.NormalBorder
		if m.walletFocus == 1 {
			border = theme.GrayedBorder
		}
		return border.Width(w).Padding(1, 2).
			Render(padLines(lines, h))
	}

	if !m.cfg.WalletExists() {
		lines = append(lines, theme.Grayed.Render("  Create wallet from System tab"))
		border := theme.NormalBorder
		if m.walletFocus == 1 {
			border = theme.GrayedBorder
		}
		return border.Width(w).Padding(1, 2).
			Render(padLines(lines, h))
	}

	if m.status == nil || !m.status.lndResponding {
		lines = append(lines, "  "+theme.Dim.Render("Waiting for LND..."))
		border := theme.NormalBorder
		if m.walletFocus == 1 {
			border = theme.SelectedBorder
		}
		return border.Width(w).Padding(1, 2).
			Render(padLines(lines, h))
	}

	balance := "0"
	if m.status.lndBalance != "" {
		balance = m.status.lndBalance
	}
	lines = append(lines, "  "+theme.Header.Render("Balance"))
	lines = append(lines, "  "+theme.Value.Render(
		formatSats(parseBalance(balance))+" sats"))
	lines = append(lines, "")

	if m.status.lndPubkey != "" {
		pubkeyShort := m.status.lndPubkey
		if len(pubkeyShort) > 20 {
			pubkeyShort = pubkeyShort[:20] + "..."
		}
		lines = append(lines, "  "+theme.Label.Render("Pubkey: ")+
			theme.Dim.Render(pubkeyShort))
	}
	lines = append(lines, "  "+theme.Label.Render("P2P: ")+
		theme.Value.Render(p2pModeLabel(m.cfg.P2PMode)))
	if m.cfg.AutoUnlock {
		lines = append(lines, "  "+theme.Label.Render("Auto-unlock: ")+
			theme.GreenDot.Render("● ")+"enabled")
	}
	lines = append(lines, "")

	if m.status.lndSyncedChain {
		lines = append(lines, "  "+theme.GreenDot.Render("●")+
			" Chain synced")
	} else {
		lines = append(lines, "  "+theme.RedDot.Render("○")+
			" Chain syncing...")
	}
	if m.status.lndSyncedGraph {
		lines = append(lines, "  "+theme.GreenDot.Render("●")+
			" Graph synced")
	} else {
		lines = append(lines, "  "+theme.RedDot.Render("○")+
			" Graph syncing...")
	}

	lines = append(lines, "")
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Action.Render("[s] Send"))
	lines = append(lines, "  "+theme.Action.Render("[r] Receive"))
	lines = append(lines, "  "+theme.Action.Render("[v] History"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Action.Render("enter details ▸"))

	border := theme.NormalBorder
	if m.walletFocus == 1 {
		border = theme.SelectedBorder
	}
	return border.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

func (m Model) viewWalletInfo() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡️ Wallet & Node Info"))
	lines = append(lines, "")

	if m.cfg.WalletExists() {
		lines = append(lines, "  "+theme.Label.Render("Wallet: ")+
			theme.Success.Render("created"))
		if m.cfg.AutoUnlock {
			lines = append(lines, "  "+theme.Label.Render("Auto-unlock: ")+
				theme.Success.Render("enabled"))
		}
		lines = append(lines, "  "+theme.Label.Render("P2P Mode: ")+
			theme.Value.Render(p2pModeLabel(m.cfg.P2PMode)))

		if m.status != nil && m.status.lndResponding {
			if m.status.lndBalance != "" {
				lines = append(lines, "  "+theme.Label.Render("Balance: ")+
					theme.Value.Render(m.status.lndBalance+" sats"))
			}
			if m.status.lndChannels > 0 {
				lines = append(lines, "  "+theme.Label.Render("Channels: ")+
					theme.Value.Render(fmt.Sprintf("%d", m.status.lndChannels)))
			}
			if m.status.lndPubkey != "" {
				lines = append(lines, "")
				lines = append(lines, "  "+theme.Label.Render("Pubkey:"))
				lines = append(lines, "  "+
					theme.Mono.Render(m.status.lndPubkey))
			}
		} else {
			lines = append(lines, "")
			lines = append(lines, "  "+
				theme.Dim.Render("Waiting for LND..."))
		}

		if m.cfg.P2PMode == "tor" {
			lines = append(lines, "")
			lines = append(lines, "  "+theme.Action.Render(
				"[p] upgrade to clearnet+tor"))
		}
	} else {
		lines = append(lines, "  "+
			theme.Warning.Render("Wallet not created"))
	}

	content := strings.Join(lines, "\n")
	box := theme.Box.Width(bw).Padding(1, 2).Render(content)
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡️ Wallet & Node Info ")
	var footer string
	if m.cfg.P2PMode == "tor" {
		footer = theme.Footer.Render(
			"  p upgrade P2P • backspace back • q quit  ")
	} else {
		footer = theme.Footer.Render(
			"  backspace back • q quit  ")
	}
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
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
