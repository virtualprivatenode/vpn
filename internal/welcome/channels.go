// internal/welcome/channels.go

package welcome

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

const channelBarWidth = 30

func (m Model) viewChannels(bw int) string {
	cardW := bw - 2
	cardH := theme.BoxHeight

	if !m.cfg.HasLND() {
		return m.channelsNotInstalled(cardW, cardH)
	}
	if !m.cfg.WalletExists() {
		return m.channelsNoWallet(cardW, cardH)
	}
	return m.channelsList(cardW, cardH)
}

func (m Model) channelsNotInstalled(w, h int) string {
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡ Channels"))
	lines = append(lines, "")
	lines = append(lines, theme.Grayed.Render("  Install LND from Dashboard"))
	return theme.NormalBorder.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

func (m Model) channelsNoWallet(w, h int) string {
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡ Channels"))
	lines = append(lines, "")
	lines = append(lines, theme.Grayed.Render("  Create LND wallet first"))
	return theme.NormalBorder.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

func (m Model) channelsList(w, h int) string {
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
		return theme.NormalBorder.Width(w).Padding(1, 2).
			Render(padLines(lines, h))
	}

	if len(m.status.channels) == 0 {
		lines = append(lines, "  "+theme.Dim.Render(
			"No channels yet. Open a channel to start using Lightning."))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Action.Render("▸ [Open Channel]"))
		return theme.NormalBorder.Width(w).Padding(1, 2).
			Render(padLines(lines, h))
	}

	visibleCount := m.channelVisibleCount()

	if m.chanScrollOffset > 0 {
		lines = append(lines, "  "+theme.Dim.Render("  ↑ more"))
	}

	viewEnd := m.chanScrollOffset + visibleCount
	if viewEnd > len(m.status.channels) {
		viewEnd = len(m.status.channels)
	}

	for i := m.chanScrollOffset; i < viewEnd; i++ {
		ch := m.status.channels[i]

		prefix := "  "
		nameStyle := theme.Value
		if m.chanCursor == i {
			prefix = "▸ "
			nameStyle = theme.Action
		}

		dot := theme.RedDot.Render("○")
		if ch.Active {
			dot = theme.GreenDot.Render("●")
		}

		name := ch.PeerAlias
		if name == "" {
			if len(ch.RemotePubkey) > 16 {
				name = ch.RemotePubkey[:16] + "..."
			} else {
				name = ch.RemotePubkey
			}
		}
		if len(name) > 20 {
			name = name[:20]
		}
		name = fmt.Sprintf("%-20s", name)

		bar := renderBalanceBar(ch.LocalBalance, ch.RemoteBalance,
			ch.Capacity, channelBarWidth)

		localPct := 0
		if ch.Capacity > 0 {
			localPct = int(ch.LocalBalance * 100 / ch.Capacity)
		}
		balText := fmt.Sprintf("%s / %s  %d%%",
			formatSats(ch.LocalBalance),
			formatSats(ch.RemoteBalance),
			localPct)

		lines = append(lines, fmt.Sprintf("%s%s %s  %s  %s",
			prefix, dot, nameStyle.Render(name), bar,
			theme.Dim.Render(balText)))
	}

	if viewEnd < len(m.status.channels) {
		lines = append(lines, "  "+theme.Dim.Render("  ↓ more"))
	}

	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"─────────────────────────────────────────────"))

	var totalCap, totalLocal, totalRemote int64
	for _, ch := range m.status.channels {
		totalCap += ch.Capacity
		totalLocal += ch.LocalBalance
		totalRemote += ch.RemoteBalance
	}

	lines = append(lines, "  "+theme.Label.Render("Total capacity: ")+
		theme.Value.Render(formatSats(totalCap)+" sats"))
	lines = append(lines, "  "+theme.Label.Render("Can send:       ")+
		theme.Value.Render(formatSats(totalLocal)+" sats"))
	lines = append(lines, "  "+theme.Label.Render("Can receive:    ")+
		theme.Value.Render(formatSats(totalRemote)+" sats"))

	if m.status.pendingOpen > 0 || m.status.pendingForceClose > 0 {
		lines = append(lines, "")
		if m.status.pendingOpen > 0 {
			lines = append(lines, "  "+theme.Dim.Render(
				fmt.Sprintf("  %d channel(s) opening", m.status.pendingOpen)))
		}
		if m.status.pendingForceClose > 0 {
			lines = append(lines, "  "+theme.Warning.Render(
				fmt.Sprintf("  %d channel(s) force closing",
					m.status.pendingForceClose)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, "  "+theme.Action.Render("[o] Open new channel"))

	return theme.SelectedBorder.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

func (m Model) viewChannelDetail() string {
	bw := min(m.width-4, theme.ContentWidth)

	if m.status == nil || m.chanCursor >= len(m.status.channels) {
		return lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			theme.Warn.Render("Channel not found"))
	}

	ch := m.status.channels[m.chanCursor]

	name := ch.PeerAlias
	if name == "" {
		if len(ch.RemotePubkey) > 20 {
			name = ch.RemotePubkey[:20] + "..."
		} else {
			name = ch.RemotePubkey
		}
	}

	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡ "+name))
	lines = append(lines, "")

	bar := renderBalanceBar(ch.LocalBalance, ch.RemoteBalance, ch.Capacity, 40)
	lines = append(lines, "  "+bar)
	lines = append(lines, "")

	localPct := 0
	if ch.Capacity > 0 {
		localPct = int(ch.LocalBalance * 100 / ch.Capacity)
	}
	remotePct := 100 - localPct

	lines = append(lines, "  "+theme.Label.Render("Capacity:     ")+
		theme.Value.Render(formatSats(ch.Capacity)+" sats"))
	lines = append(lines, "  "+theme.Label.Render("Local:        ")+
		theme.Value.Render(fmt.Sprintf("%s sats (%d%%)",
			formatSats(ch.LocalBalance), localPct)))
	lines = append(lines, "  "+theme.Label.Render("Remote:       ")+
		theme.Value.Render(fmt.Sprintf("%s sats (%d%%)",
			formatSats(ch.RemoteBalance), remotePct)))
	lines = append(lines, "")

	statusText := theme.Success.Render("Active")
	if !ch.Active {
		statusText = theme.Warning.Render("Peer offline")
	}
	lines = append(lines, "  "+theme.Label.Render("Status:       ")+statusText)

	privateText := "No"
	if ch.Private {
		privateText = "Yes"
	}
	lines = append(lines, "  "+theme.Label.Render("Private:      ")+
		theme.Value.Render(privateText))

	initiatorText := "Remote peer"
	if ch.Initiator {
		initiatorText = "You"
	}
	lines = append(lines, "  "+theme.Label.Render("Initiator:    ")+
		theme.Value.Render(initiatorText))

	lines = append(lines, "  "+theme.Label.Render("Channel ID:   ")+
		theme.Mono.Render(fmt.Sprintf("%d", ch.ChanID)))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Peer Pubkey:"))
	lines = append(lines, "  "+theme.Mono.Render(ch.RemotePubkey))

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Channel Details ")
	footer := theme.Footer.Render("  backspace back • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func renderBalanceBar(local, remote, capacity int64, width int) string {
	if capacity <= 0 {
		return strings.Repeat("░", width)
	}
	localWidth := int(float64(local) / float64(capacity) * float64(width))
	if localWidth < 0 {
		localWidth = 0
	}
	if localWidth > width {
		localWidth = width
	}
	remoteWidth := width - localWidth
	localBar := theme.Good.Render(strings.Repeat("━", localWidth))
	remoteBar := theme.Dim.Render(strings.Repeat("░", remoteWidth))
	return localBar + remoteBar
}

func formatSats(sats int64) string {
	if sats < 0 {
		return fmt.Sprintf("%d", sats)
	}
	s := fmt.Sprintf("%d", sats)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

func (m Model) channelVisibleCount() int {
	available := theme.BoxHeight - 12
	if available < 3 {
		available = 3
	}
	return available
}
