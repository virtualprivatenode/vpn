// internal/welcome/channel_open.go

package welcome

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

var amountPresets = []int64{
	100000,
	250000,
	500000,
	1000000,
	2000000,
	0, // custom
}

func presetLabel(sats int64) string {
	if sats == 0 {
		return "Custom amount"
	}
	return formatSats(sats) + " sats"
}

func (m Model) startChannelOpen() (tea.Model, tea.Cmd) {
	if m.lndClient == nil || !m.cfg.HasLND() || !m.cfg.WalletExists() {
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

func (m Model) viewChannelOpen() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Lightning.Render("⚡ Open Channel"))
	lines = append(lines, "")

	balText := "unknown"
	if m.status != nil && m.status.lndBalance != "" {
		balText = m.status.lndBalance + " sats"
	}
	lines = append(lines, "  "+theme.Label.Render("On-chain balance: ")+
		theme.Value.Render(balText))
	lines = append(lines, "")

	if m.cfg.P2PMode == "tor" {
		lines = append(lines, "  "+theme.Dim.Render(
			"🧅 Tor-only mode — peers marked (Tor) accept Tor connections"))
		lines = append(lines, "")
	}

	lines = append(lines, "  "+theme.Header.Render("Select a peer:"))
	lines = append(lines, "")

	for i, peer := range m.chanPeerList {
		prefix := "  "
		style := theme.Value
		if m.chanOpenPeerIdx == i {
			prefix = "▸ "
			style = theme.Action
		}

		name := peer.Alias
		if len(name) > 22 {
			name = name[:22]
		}
		name = fmt.Sprintf("%-22s", name)

		tags := ""
		if peer.TorOnly {
			tags += " (Tor)"
		}
		if peer.MinChanSize > 0 {
			tags += fmt.Sprintf(" min:%s", formatSats(peer.MinChanSize))
		}
		if peer.Note != "" {
			tags += " — " + peer.Note
		}
		if peer.Curated {
			tags += " ★"
		}

		lines = append(lines, fmt.Sprintf("  %s%s%s",
			prefix, style.Render(name), theme.Dim.Render(tags)))
	}

	customIdx := len(m.chanPeerList)
	prefix := "  "
	style := theme.Value
	if m.chanOpenPeerIdx == customIdx {
		prefix = "▸ "
		style = theme.Action
	}
	lines = append(lines, fmt.Sprintf("  %s%s",
		prefix, style.Render("[Custom peer — enter pubkey]")))

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Open Channel ")
	footer := theme.Footer.Render(
		"  ↑↓ select • enter choose • backspace cancel • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewChannelCustomPeer() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Header.Render("Custom Peer"))
	lines = append(lines, "")

	pubkeyStyle := theme.Value
	hostStyle := theme.Value
	pubkeyCursor := ""
	hostCursor := ""
	if m.chanCustomInputField == 0 {
		pubkeyStyle = theme.Action
		pubkeyCursor = "_"
	} else {
		hostStyle = theme.Action
		hostCursor = "_"
	}

	lines = append(lines, "  "+theme.Label.Render("Node Pubkey:"))
	lines = append(lines, "  "+pubkeyStyle.Render(m.chanCustomPubkey+pubkeyCursor))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Host (host:port):"))
	lines = append(lines, "  "+hostStyle.Render(m.chanCustomHost+hostCursor))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render("Tab to switch fields • Enter to continue"))

	if m.chanOpenError != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(m.chanOpenError))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Custom Peer ")
	footer := theme.Footer.Render(
		"  tab switch field • enter continue • backspace cancel  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewChannelAmountSelect() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	// Title shows peer name
	peerName := m.chanOpenAlias
	if peerName == "" {
		if len(m.chanOpenPubkey) > 16 {
			peerName = m.chanOpenPubkey[:16] + "..."
		} else {
			peerName = m.chanOpenPubkey
		}
	}
	lines = append(lines, theme.Lightning.Render("⚡ "+peerName))
	lines = append(lines, "")

	balText := "unknown"
	if m.status != nil && m.status.lndBalance != "" {
		balText = m.status.lndBalance + " sats"
	}
	lines = append(lines, "  "+theme.Label.Render("On-chain balance: ")+
		theme.Value.Render(balText))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Header.Render("Channel size:"))
	lines = append(lines, "")

	for i, amt := range amountPresets {
		prefix := "  "
		style := theme.Value
		if m.chanAmountPreset == i {
			prefix = "▸ "
			style = theme.Action
		}
		label := presetLabel(amt)
		if amt == 0 && m.chanAmountPreset == i && m.chanCustomAmountStr != "" {
			label = "Custom: " + m.chanCustomAmountStr + "_ sats"
		}
		lines = append(lines, fmt.Sprintf("  %s%s", prefix, style.Render(label)))
	}

	if m.chanOpenError != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(m.chanOpenError))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Channel Size ")
	footer := theme.Footer.Render(
		"  ↑↓ select • enter choose • backspace back  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewChannelOpenConfirm() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Warning.Render("⚠ Confirm Channel Open"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Peer:      ")+
		theme.Value.Render(m.chanOpenAlias))
	lines = append(lines, "  "+theme.Label.Render("Amount:    ")+
		theme.Value.Render(formatSats(m.chanOpenAmount)+" sats"))

	privateText := "No (public — announced to network)"
	if m.chanOpenPrivate {
		privateText = "Yes (private — unannounced)"
	}
	lines = append(lines, "  "+theme.Label.Render("Private:   ")+
		theme.Value.Render(privateText))

	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Pubkey:"))
	lines = append(lines, "  "+theme.Mono.Render(m.chanOpenPubkey))

	lines = append(lines, "")
	lines = append(lines, "  "+theme.Warning.Render(
		"This will spend "+formatSats(m.chanOpenAmount)+
			" sats from your on-chain wallet."))
	lines = append(lines, "  "+theme.Warning.Render(
		"The channel requires on-chain confirmations to activate."))

	if m.chanOpenError != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(m.chanOpenError))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Confirm Channel ")
	footer := theme.Footer.Render(
		"  y confirm • p toggle private/public • backspace cancel  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewChannelOpening() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Lightning.Render("⚡ Opening Channel..."))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Value.Render(
		"Connecting to peer and broadcasting funding transaction."))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"This may take up to 2 minutes over Tor."))
	lines = append(lines, "  "+theme.Dim.Render(
		"Do not close the terminal."))

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Opening Channel ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewChannelOpenResult() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	if m.chanOpenError != "" {
		lines = append(lines, theme.Warning.Render("❌ Channel Open Failed"))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(m.chanOpenError))
	} else {
		lines = append(lines, theme.Success.Render("✅ Channel Opening"))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Value.Render(
			"Funding transaction broadcast successfully."))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Label.Render("Peer:   ")+
			theme.Value.Render(m.chanOpenAlias))
		lines = append(lines, "  "+theme.Label.Render("Amount: ")+
			theme.Value.Render(formatSats(m.chanOpenAmount)+" sats"))
		if m.chanOpenTxid != "" {
			lines = append(lines, "  "+theme.Label.Render("TX ID:"))
			lines = append(lines, "  "+theme.Mono.Render(m.chanOpenTxid))
		}
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Dim.Render(
			"The channel will appear as 'pending' in your"))
		lines = append(lines, "  "+theme.Dim.Render(
			"channel list until it has enough confirmations."))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Channel Result ")
	footer := theme.Footer.Render(
		"  enter return to channels • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewChannelFundWallet() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Warning.Render("⚠ Fund Your Wallet"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Value.Render(
		"Your on-chain balance is empty."))
	lines = append(lines, "  "+theme.Value.Render(
		"Send Bitcoin to this address to fund your wallet:"))
	lines = append(lines, "")

	if m.chanFundAddress != "" {
		lines = append(lines, "  "+theme.Mono.Render(m.chanFundAddress))
		lines = append(lines, "")
		qr := renderQRCode(m.chanFundAddress)
		if qr != "" {
			lines = append(lines, qr)
		}
	} else {
		lines = append(lines, "  "+theme.Dim.Render("Generating address..."))
	}

	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"After sending, wait for 1 confirmation"))
	lines = append(lines, "  "+theme.Dim.Render(
		"then return to open a channel."))

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Fund Wallet ")
	footer := theme.Footer.Render("  backspace back • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func isHexChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func parseCustomAmount(s string) (int64, error) {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty amount")
	}
	amt, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number")
	}
	if amt < 20000 {
		return 0, fmt.Errorf("minimum channel size is 20,000 sats")
	}
	if amt > 16777215 {
		return 0, fmt.Errorf("maximum channel size is 16,777,215 sats")
	}
	return amt, nil
}
