// internal/welcome/view.go

package welcome

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	switch m.subview {
	case svWalletInfo:
		return m.viewWalletInfo()
	case svZeus:
		return m.viewZeus()
	case svSyncthingDetail:
		return m.viewSyncthingDetail()
	case svSyncthingPairInput:
		return m.viewSyncthingPairInput()
	case svSyncthingDeviceDetail:
		return m.viewSyncthingDeviceDetail()
	case svSyncthingWebUI:
		return m.viewSyncthingWebUI()
	case svSyncthingDeviceQR:
		return m.viewSyncthingDeviceQR()
	case svChannelDetail:
		return m.viewChannelDetail()
	case svChannelOpen:
		return m.viewChannelOpen()
	case svChannelAmountSelect:
		return m.viewChannelAmountSelect()
	case svChannelCustomPeer:
		return m.viewChannelCustomPeer()
	case svChannelOpenConfirm:
		return m.viewChannelOpenConfirm()
	case svChannelOpening:
		return m.viewChannelOpening()
	case svChannelOpenResult:
		return m.viewChannelOpenResult()
	case svChannelFundWallet:
		return m.viewChannelFundWallet()
	case svLndHubManage:
		return m.viewLndHubManage()
	case svLndHubCreateName:
		return m.viewLndHubCreateName()
	case svLndHubCreateAccount:
		return m.viewLndHubNewAccount()
	case svLndHubAccountDetail:
		return m.viewLndHubAccountDetail()
	case svLndHubDeactivateConfirm:
		return m.viewLndHubDeactivateConfirm()
	case svQR:
		return m.viewQR()
	case svFullURL:
		return m.viewFullURL()
	case svReceive:
		return m.viewReceive()
	case svReceiveWaiting:
		return m.viewReceiveWaiting()
	case svReceivePaid:
		return m.viewReceivePaid()
	case svReceiveExpired:
		return m.viewReceiveExpired()
	case svSend:
		return m.viewSend()
	case svSendConfirm:
		return m.viewSendConfirm()
	case svSendInFlight:
		return m.viewSendInFlight()
	case svSendResult:
		return m.viewSendResult()
	case svPaymentHistory:
		return m.viewPaymentHistory()
	case svPaymentDetail:
		return m.viewPaymentDetail()
	}

	bw := min(m.width-4, theme.ContentWidth)
	var content string
	switch m.activeTab {
	case tabDashboard:
		content = m.viewDashboard(bw)
	case tabLightning:
		content = m.viewLightningTab(bw)
	case tabPairing:
		content = m.viewPairing(bw)
	case tabAddons:
		content = m.viewAddons(bw)
	case tabSettings:
		content = m.viewSettings(bw)
	}

	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(fmt.Sprintf(" Virtual Private Node v%s ", m.version))
	tabs := m.viewTabs(bw)
	footer := m.viewFooter()
	body := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", tabs, "", content)
	full := lipgloss.JoinVertical(lipgloss.Center, body, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewTabs(tw int) string {
	tabs := []struct {
		n string
		t wTab
	}{
		{"Dashboard", tabDashboard},
		{"Lightning", tabLightning},
		{"Pairing", tabPairing},
		{"Add-ons", tabAddons},
		{"Settings", tabSettings},
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
	if m.cardActive {
		if m.dashCard == cardServices {
			return theme.Footer.Render(
				"  ↑↓ select • [r]estart [s]top [a]start [l]ogs • backspace back • q quit  ")
		}
		if m.dashCard == cardSystem {
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
			"  ↑↓←→ navigate • enter select • tab switch • q quit  ")
	case tabLightning:
		if m.lightningFocus == 0 {
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
	case tabSettings:
		if m.updateConfirm {
			return theme.Footer.Render(
				"  y confirm • any key cancel  ")
		}
		return theme.Footer.Render(
			"  enter update • tab switch • q quit  ")
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

// viewLightningTab renders the Lightning tab with two cards:
// Channels (left) and Wallet (right).
func (m Model) viewLightningTab(bw int) string {
	halfW := (bw - 2) / 2
	cardH := theme.BoxHeight

	channelsCard := m.lightningChannelsCard(halfW, cardH)
	walletCard := m.lightningWalletCard(halfW, cardH)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		channelsCard, "  ", walletCard)
}

func (m Model) lightningChannelsCard(w, h int) string {
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
	lines = append(lines, theme.Grayed.Render("  Install LND from Dashboard"))
	border := theme.NormalBorder
	if m.lightningFocus == 0 {
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
	if m.lightningFocus == 0 {
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
		if m.lightningFocus == 0 {
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
		if m.lightningFocus == 0 {
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
		if m.lightningFocus == 0 && m.chanCursor == i {
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
	var totalCap, totalLocal, totalRemote int64
	for _, ch := range m.status.channels {
		totalCap += ch.Capacity
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
	if m.lightningFocus == 0 {
		border = theme.SelectedBorder
	}
	return border.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

func (m Model) lightningWalletCard(w, h int) string {
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡ Wallet"))
	lines = append(lines, "")

	if !m.cfg.HasLND() {
		lines = append(lines, theme.Grayed.Render("  Install LND from Dashboard"))
		border := theme.NormalBorder
		if m.lightningFocus == 1 {
			border = theme.GrayedBorder
		}
		return border.Width(w).Padding(1, 2).
			Render(padLines(lines, h))
	}

	if !m.cfg.WalletExists() {
		lines = append(lines, theme.Grayed.Render("  Create wallet from Dashboard"))
		border := theme.NormalBorder
		if m.lightningFocus == 1 {
			border = theme.GrayedBorder
		}
		return border.Width(w).Padding(1, 2).
			Render(padLines(lines, h))
	}

	if m.status == nil || !m.status.lndResponding {
		lines = append(lines, "  "+theme.Dim.Render("Waiting for LND..."))
		border := theme.NormalBorder
		if m.lightningFocus == 1 {
			border = theme.SelectedBorder
		}
		return border.Width(w).Padding(1, 2).
			Render(padLines(lines, h))
	}

	// Balance
	balance := "0"
	if m.status.lndBalance != "" {
		balance = m.status.lndBalance
	}
	lines = append(lines, "  "+theme.Header.Render("Balance"))
	lines = append(lines, "  "+theme.Value.Render(
		formatSats(parseBalance(balance))+" sats"))
	lines = append(lines, "")

	// Node info summary
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

	// Sync status
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
	if m.lightningFocus == 1 {
		border = theme.SelectedBorder
	}
	return border.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

// viewWalletInfo shows detailed wallet/node info (was svLightning).
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

// parseBalance converts a balance string to int64 for formatting.
func parseBalance(s string) int64 {
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	return n
}
