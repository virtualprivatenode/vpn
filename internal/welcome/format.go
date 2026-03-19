// internal/welcome/format.go

package welcome

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

const channelBarWidth = 30

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

	bar := renderBalanceBar(ch.LocalBalance, ch.RemoteBalance,
		ch.Capacity, 40)
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
	lines = append(lines, "  "+theme.Label.Render("Status:       ")+
		statusText)

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
	footer := theme.Footer.Render(
		"  backspace back • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}
