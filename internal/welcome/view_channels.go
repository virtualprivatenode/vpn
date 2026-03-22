package welcome

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Button styles ────────────────────────────────────────

var (
	btnNormal = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("15")).
			Padding(0, 1)

	btnFocused = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Background(lipgloss.Color("15")).
			Bold(true).
			Padding(0, 1)
)

// ── Channels overview ────────────────────────────────────

func (m Model) channelsOverview(w, h int) string {
	if !m.cfg.HasLND() {
		var lines []string
		lines = append(lines, "")
		lines = append(lines, theme.Dim.Render(
			" LND is not installed."))
		lines = append(lines, theme.Dim.Render(
			" Go to System to install."))
		return strings.Join(lines, "\n")
	}

	if !m.cfg.WalletExists() {
		var lines []string
		lines = append(lines, "")
		lines = append(lines, theme.Dim.Render(
			" LND wallet not created."))
		return strings.Join(lines, "\n")
	}

	if m.status == nil || !m.status.lndResponding {
		return theme.Dim.Render(" Waiting for LND...")
	}

	var lines []string

	// Calculate totals
	var totalCap, totalLocal, totalRemote int64
	activeCount, inactiveCount := 0, 0
	for _, ch := range m.status.channels {
		if ch.Pending {
			continue
		}
		totalCap += ch.Capacity
		totalLocal += ch.LocalBalance
		totalRemote += ch.RemoteBalance
		if ch.Active {
			activeCount++
		} else {
			inactiveCount++
		}
	}
	onchain := "0"
	if m.status.lndBalance != "" {
		onchain = m.status.lndBalance
	}

	// ── Layout math ──────────────────────────────
	chanCount := len(m.status.channels)
	headerRows := 8
	chanRows := chanCount * 2
	if chanCount > 0 {
		chanRows--
	}
	buttonRows := 2
	usedRows := headerRows + 1 + chanRows + buttonRows
	freeRows := h - usedRows
	if freeRows < 0 {
		freeRows = 0
	}
	topPad := 1
	midGap := freeRows / 3
	if midGap < 1 {
		midGap = 1
	}
	botGap := freeRows - topPad - midGap
	if botGap < 0 {
		botGap = 0
	}

	for i := 0; i < topPad; i++ {
		lines = append(lines, "")
	}

	// ── Pubkey and P2P (full width, no box) ──────
	if m.status.lndPubkey != "" {
		pk := truncatePubkey(m.status.lndPubkey, w-14)
		lines = append(lines,
			" "+theme.Label.Render("Pubkey:   ")+
				theme.Mono.Render(pk))
	}
	lines = append(lines,
		" "+theme.Label.Render("P2P:      ")+
			theme.Value.Render(
				p2pModeLabel(m.cfg.P2PMode)))
	lines = append(lines, "")

	// ── Liquidity box: right-aligned, wide ───────
	// Box fills from column ~14 to right edge
	localPct := 0
	if totalCap > 0 {
		localPct = int(
			float64(totalLocal) * 100 /
				float64(totalCap))
	}
	remotePct := 100 - localPct

	labelW := 12 // "On-chain: " etc
	leftColW := labelW + 18
	boxW := w - leftColW - 3
	if boxW < 22 {
		boxW = 22
	}

	barInnerW := boxW - 4
	if barInnerW < 8 {
		barInnerW = 8
	}
	localBarW := barInnerW * localPct / 100
	if localBarW < 0 {
		localBarW = 0
	}
	if localBarW > barInnerW {
		localBarW = barInnerW
	}
	remoteBarW := barInnerW - localBarW

	barLocal := lipgloss.NewStyle().
		Foreground(lipgloss.Color("34")).
		Render(strings.Repeat("█", localBarW))
	barRemote := lipgloss.NewStyle().
		Foreground(lipgloss.Color("60")).
		Render(strings.Repeat("█", remoteBarW))
	barLine := " " + barLocal + barRemote + " "

	barVis := lipgloss.Width(barLine)
	barPad := boxW - 2 - barVis
	if barPad < 0 {
		barPad = 0
	}

	chLabel := fmt.Sprintf("%d channels",
		activeCount+inactiveCount)
	if inactiveCount > 0 {
		chLabel = fmt.Sprintf("%d on, %d off",
			activeCount, inactiveCount)
	}

	// Build box lines
	boxTop := "┌" + strings.Repeat("─", boxW-2) + "┐"
	boxLabel := "│" + centerPad(chLabel, boxW-2) + "│"
	boxBar := "│" + barLine +
		strings.Repeat(" ", barPad) + "│"

	pctL := fmt.Sprintf(" Out %d%%", localPct)
	pctR := fmt.Sprintf("In %d%% ", remotePct)
	pctGap := boxW - 2 - len(pctL) - len(pctR)
	if pctGap < 0 {
		pctGap = 0
	}
	boxPct := "│" + theme.Dim.Render(
		pctL+strings.Repeat(" ", pctGap)+pctR) + "│"

	capLine := centerPad(
		formatSats(totalCap)+" sats", boxW-2)
	boxCap := "│" + theme.Dim.Render(capLine) + "│"
	boxBot := "└" + strings.Repeat("─", boxW-2) + "┘"

	boxLines := []string{
		boxTop, boxLabel, boxBar,
		boxPct, boxCap, boxBot,
	}

	// Left column: Outbound, Inbound, On-chain
	var leftLines []string
	leftLines = append(leftLines,
		" "+theme.Label.Render("Outbound: ")+
			theme.Value.Render(
				formatSats(totalLocal)+" sats"))
	leftLines = append(leftLines,
		" "+theme.Label.Render("Inbound:  ")+
			theme.Value.Render(
				formatSats(totalRemote)+" sats"))
	leftLines = append(leftLines,
		" "+theme.Label.Render("On-chain: ")+
			theme.Value.Render(
				formatSats(parseBalance(onchain))+
					" sats"))

	// Merge: left lines align with box rows
	maxH := len(boxLines)
	if len(leftLines) > maxH {
		maxH = len(leftLines)
	}
	for len(leftLines) < maxH {
		leftLines = append(leftLines, "")
	}
	for len(boxLines) < maxH {
		boxLines = append(boxLines, "")
	}

	for i := 0; i < maxH; i++ {
		lft := leftLines[i]
		lftW := lipgloss.Width(lft)
		if lftW < leftColW {
			lft += strings.Repeat(" ", leftColW-lftW)
		}
		lines = append(lines, lft+"  "+boxLines[i])
	}

	// Mid gap
	for i := 0; i < midGap; i++ {
		lines = append(lines, "")
	}

	// ── Channel bars (single line) ───────────────
	rightEdge := w - 1
	nameW := 20
	valsW := 14
	barW := rightEdge - nameW - 4 - valsW
	if barW < 8 {
		barW = 8
	}

	if chanCount == 0 {
		lines = append(lines, theme.Dim.Render(
			" No channels yet."))
	} else {
		for i, ch := range m.status.channels {
			isSelected := m.chanCursor == i &&
				m.contentFocused &&
				m.contentFocus == 0 &&
				!m.tabFocused

			name := ch.PeerAlias
			if name == "" {
				name = ch.RemotePubkey
			}
			if len(name) > nameW {
				name = name[:nameW-3] + "..."
			}

			dot := theme.RedDot.Render("○")
			if ch.Active {
				dot = theme.GreenDot.Render("●")
			}
			if ch.Pending {
				dot = theme.Dim.Render("◌")
			}

			localFill := 0
			if ch.Capacity > 0 {
				localFill = int(
					float64(ch.LocalBalance) /
						float64(ch.Capacity) *
						float64(barW))
			}
			if localFill > barW {
				localFill = barW
			}
			remoteFill := barW - localFill

			var localColor, remoteColor string
			if isSelected {
				localColor = "40"
				remoteColor = "69"
			} else if ch.Active {
				localColor = "34"
				remoteColor = "60"
			} else {
				localColor = "22"
				remoteColor = "237"
			}

			lBar := lipgloss.NewStyle().
				Foreground(
					lipgloss.Color(localColor)).
				Render(strings.Repeat("█", localFill))
			rBar := lipgloss.NewStyle().
				Foreground(
					lipgloss.Color(remoteColor)).
				Render(
					strings.Repeat("█", remoteFill))
			barStr := lBar + rBar

			vals := fmt.Sprintf("%s / %s",
				formatSatsCompact(ch.LocalBalance),
				formatSatsCompact(ch.RemoteBalance))
			valsPad := fmt.Sprintf("%*s", valsW, vals)

			marker := " "
			nameStyle := theme.Value
			if isSelected {
				marker = "▸"
				nameStyle = navActiveStyle
			}
			namePad := fmt.Sprintf("%-*s", nameW, name)

			line := fmt.Sprintf("%s%s %s %s %s",
				marker, dot,
				nameStyle.Render(namePad),
				barStr,
				theme.Dim.Render(valsPad))

			lines = append(lines, line)

			if i < chanCount-1 {
				lines = append(lines, "")
			}
		}

		if m.status.pendingOpen > 0 {
			lines = append(lines, "")
			lines = append(lines,
				" "+theme.Dim.Render(
					fmt.Sprintf("%d pending",
						m.status.pendingOpen)))
		}
	}

	// Bottom gap
	for i := 0; i < botGap; i++ {
		lines = append(lines, "")
	}

	// ── Open Channel button ──────────────────────
	label := "Open Channel"
	isOnButton := m.contentFocused &&
		m.contentFocus == 1 &&
		!m.tabFocused

	var btnStr string
	if isOnButton {
		btnStr = "▸ " + btnFocused.Render(label)
	} else {
		btnStr = "  " + btnNormal.Render(label)
	}

	btnVis := lipgloss.Width(btnStr)
	btnPadding := rightEdge - btnVis
	if btnPadding < 0 {
		btnPadding = 0
	}
	lines = append(lines,
		strings.Repeat(" ", btnPadding)+btnStr)

	return strings.Join(lines, "\n")
}

// ── Channel detail ───────────────────────────────────────

func (m Model) channelDetailPane(w int) string {
	if m.chanCursor >= len(m.status.channels) {
		return theme.Dim.Render(" Channel not found")
	}
	ch := m.status.channels[m.chanCursor]
	var lines []string

	name := ch.PeerAlias
	if name == "" {
		name = ch.RemotePubkey[:16] + "..."
	}
	lines = append(lines, "")
	lines = append(lines, " "+theme.Header.Render(name))
	lines = append(lines, "")

	status := theme.Success.Render("active")
	if !ch.Active {
		status = theme.Warning.Render("inactive")
	}
	if ch.Pending {
		status = theme.Dim.Render("pending")
	}

	lines = append(lines,
		" "+theme.Label.Render("Status:    ")+status)
	lines = append(lines,
		" "+theme.Label.Render("Capacity:  ")+
			theme.Value.Render(
				formatSats(ch.Capacity)+" sats"))
	lines = append(lines,
		" "+theme.Label.Render("Local:     ")+
			theme.Value.Render(
				formatSats(ch.LocalBalance)+" sats"))
	lines = append(lines,
		" "+theme.Label.Render("Remote:    ")+
			theme.Value.Render(
				formatSats(ch.RemoteBalance)+" sats"))

	barW := w - 4
	if barW > 40 {
		barW = 40
	}
	if barW >= 10 {
		lines = append(lines, "")
		lines = append(lines,
			" "+renderLiquidityBar(
				ch.LocalBalance, ch.RemoteBalance,
				ch.Capacity, barW))
	}
	lines = append(lines, "")

	if ch.Private {
		lines = append(lines,
			" "+theme.Label.Render("Type:      ")+
				theme.Value.Render("private"))
	} else {
		lines = append(lines,
			" "+theme.Label.Render("Type:      ")+
				theme.Value.Render("public"))
	}
	if ch.Initiator {
		lines = append(lines,
			" "+theme.Label.Render("Initiator: ")+
				theme.Value.Render("you"))
	}

	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Pubkey:"))
	pk := ch.RemotePubkey
	maxPk := w - 4
	if len(pk) > maxPk {
		pk = pk[:maxPk-3] + "..."
	}
	lines = append(lines, " "+theme.Mono.Render(pk))

	if ch.ChanID > 0 {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Label.Render("Channel ID: ")+
				theme.Mono.Render(
					fmt.Sprintf("%d", ch.ChanID)))
	}

	return strings.Join(lines, "\n")
}

// ── Channel open flow panes ──────────────────────────────

func (m Model) channelOpenContent(w int) string {
	switch m.subview {
	case svChannelOpen:
		return m.channelOpenPane(w)
	case svChannelCustomPeer:
		return m.channelCustomPeerPane(w)
	case svChannelAmountSelect:
		return m.channelAmountPane(w)
	case svChannelOpenConfirm:
		return m.channelConfirmPane(w)
	case svChannelOpening:
		return m.channelOpeningPane(w)
	case svChannelOpenResult:
		return m.channelResultPane(w)
	case svChannelFundWallet:
		return m.channelFundPane(w)
	}
	return ""
}

var amountPresets = []int64{
	100000, 250000, 500000,
	1000000, 2000000,
	0, // custom
}

func presetLabel(sats int64) string {
	if sats == 0 {
		return "Custom amount"
	}
	return formatSats(sats) + " sats"
}

func (m Model) channelOpenPane(w int) string {
	var lines []string

	lines = append(lines,
		theme.Header.Render(" Open Channel"))
	lines = append(lines, "")

	balText := "unknown"
	if m.status != nil && m.status.lndBalance != "" {
		balText = m.status.lndBalance + " sats"
	}
	lines = append(lines,
		" "+theme.Label.Render("On-chain: ")+
			theme.Value.Render(balText))
	lines = append(lines, "")

	if m.cfg.P2PMode == "tor" {
		lines = append(lines, " "+theme.Dim.Render(
			"Tor-only — (Tor) peers accept Tor"))
		lines = append(lines, "")
	}

	lines = append(lines,
		" "+theme.Header.Render("Select a peer:"))
	lines = append(lines, "")

	for i, peer := range m.chanPeerList {
		prefix := " "
		style := theme.Value
		if m.chanOpenPeerIdx == i {
			prefix = "▸"
			style = theme.Action
		}
		name := peer.Alias
		if len(name) > 20 {
			name = name[:20]
		}
		tags := ""
		if peer.TorOnly {
			tags += " (Tor)"
		}
		if peer.Curated {
			tags += " ★"
		}
		lines = append(lines, fmt.Sprintf(" %s %s%s",
			prefix, style.Render(name),
			theme.Dim.Render(tags)))
	}

	prefix := " "
	style := theme.Value
	if m.chanOpenPeerIdx == len(m.chanPeerList) {
		prefix = "▸"
		style = theme.Action
	}
	lines = append(lines, fmt.Sprintf(" %s %s",
		prefix, style.Render("[Custom peer]")))

	return strings.Join(lines, "\n")
}

func (m Model) channelCustomPeerPane(w int) string {
	var lines []string

	lines = append(lines,
		theme.Header.Render(" Custom Peer"))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Node Pubkey:"))
	lines = append(lines,
		" "+m.chanPubkeyInput.View())
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Host (host:port):"))
	lines = append(lines,
		" "+m.chanHostInput.View())
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Tab switch fields  Enter continue"))

	if m.chanOpenError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.chanOpenError))
	}

	return strings.Join(lines, "\n")
}

func (m Model) channelAmountPane(w int) string {
	var lines []string

	peerName := m.chanOpenAlias
	if peerName == "" && len(m.chanOpenPubkey) > 16 {
		peerName = m.chanOpenPubkey[:16] + "..."
	}
	lines = append(lines,
		" "+theme.Header.Render(peerName))
	lines = append(lines, "")

	balText := "unknown"
	if m.status != nil && m.status.lndBalance != "" {
		balText = m.status.lndBalance + " sats"
	}
	lines = append(lines,
		" "+theme.Label.Render("On-chain: ")+
			theme.Value.Render(balText))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Header.Render("Channel size:"))
	lines = append(lines, "")

	for i, amt := range amountPresets {
		prefix := " "
		style := theme.Value
		if m.chanAmountPreset == i {
			prefix = "▸"
			style = theme.Action
		}
		if amt == 0 && m.chanAmountPreset == i {
			lines = append(lines,
				fmt.Sprintf(" %s %s",
					prefix,
					style.Render("Custom:")))
			inputW := w - 6
			if inputW > 20 {
				inputW = 20
			}
			ai := m.chanAmountInput
			ai.SetWidth(inputW)
			lines = append(lines,
				"   "+ai.View())
			continue
		}
		lines = append(lines, fmt.Sprintf(" %s %s",
			prefix, style.Render(presetLabel(amt))))
	}

	if m.chanOpenError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.chanOpenError))
	}

	return strings.Join(lines, "\n")
}

func (m Model) channelConfirmPane(w int) string {
	var lines []string

	lines = append(lines,
		theme.Warning.Render(" Confirm Channel Open"))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Peer:    ")+
			theme.Value.Render(m.chanOpenAlias))
	lines = append(lines,
		" "+theme.Label.Render("Amount:  ")+
			theme.Value.Render(
				formatSats(m.chanOpenAmount)+" sats"))

	priv := "public"
	if m.chanOpenPrivate {
		priv = "private"
	}
	lines = append(lines,
		" "+theme.Label.Render("Type:    ")+
			theme.Value.Render(priv))
	lines = append(lines, "")

	pk := m.chanOpenPubkey
	maxPk := w - 4
	if len(pk) > maxPk {
		pk = pk[:maxPk-3] + "..."
	}
	lines = append(lines,
		" "+theme.Label.Render("Pubkey:"))
	lines = append(lines, " "+theme.Mono.Render(pk))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Warning.Render(
		"Spend "+formatSats(m.chanOpenAmount)+
			" sats?"))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"y confirm  p toggle private  esc cancel"))

	if m.chanOpenError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.chanOpenError))
	}

	return strings.Join(lines, "\n")
}

func (m Model) channelOpeningPane(w int) string {
	var lines []string
	lines = append(lines,
		theme.Header.Render(" Opening Channel..."))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Value.Render(
		"Connecting to peer and broadcasting tx."))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"May take up to 2 minutes over Tor."))
	lines = append(lines, " "+theme.Dim.Render(
		"Do not close the terminal."))
	return strings.Join(lines, "\n")
}

func (m Model) channelResultPane(w int) string {
	var lines []string

	if m.chanOpenError != "" {
		lines = append(lines,
			theme.Warning.Render(
				" Channel Open Failed"))
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.chanOpenError))
	} else {
		lines = append(lines,
			theme.Success.Render(" Channel Opening"))
		lines = append(lines, "")
		lines = append(lines, " "+theme.Value.Render(
			"Funding tx broadcast successfully."))
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Label.Render("Peer:   ")+
				theme.Value.Render(m.chanOpenAlias))
		lines = append(lines,
			" "+theme.Label.Render("Amount: ")+
				theme.Value.Render(
					formatSats(m.chanOpenAmount)+
						" sats"))
		if m.chanOpenTxid != "" {
			lines = append(lines, "")
			lines = append(lines,
				" "+theme.Label.Render("TX ID:"))
			txid := m.chanOpenTxid
			maxTx := w - 4
			if len(txid) > maxTx {
				txid = txid[:maxTx-3] + "..."
			}
			lines = append(lines,
				" "+theme.Mono.Render(txid))
		}
		lines = append(lines, "")
		lines = append(lines, " "+theme.Dim.Render(
			"Channel will appear as pending."))
	}

	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to return"))

	return strings.Join(lines, "\n")
}

func (m Model) channelFundPane(w int) string {
	var lines []string

	lines = append(lines,
		theme.Warning.Render(" Fund Your Wallet"))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Value.Render(
		"On-chain balance is empty."))
	lines = append(lines, " "+theme.Value.Render(
		"Send Bitcoin to this address:"))
	lines = append(lines, "")

	if m.chanFundAddress != "" {
		addr := m.chanFundAddress
		if len(addr) > w-3 {
			addr = addr[:w-6] + "..."
		}
		lines = append(lines,
			" "+theme.Mono.Render(addr))
	} else {
		lines = append(lines,
			" "+theme.Dim.Render(
				"Generating address..."))
	}

	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Wait for 1 confirmation,"))
	lines = append(lines, " "+theme.Dim.Render(
		"then return to open a channel."))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"esc to go back"))

	return strings.Join(lines, "\n")
}

// ── Formatting helpers ───────────────────────────────────

func renderLiquidityBar(
	local, remote, capacity int64, width int,
) string {
	if capacity <= 0 {
		return theme.Dim.Render(
			strings.Repeat("░", width))
	}
	lw := int(float64(local) / float64(capacity) *
		float64(width))
	if lw < 0 {
		lw = 0
	}
	if lw > width {
		lw = width
	}
	rw := width - lw
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("34")).
		Render(strings.Repeat("█", lw)) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("60")).
			Render(strings.Repeat("█", rw))
}

func p2pModeLabel(mode string) string {
	if mode == "hybrid" {
		return "Tor + clearnet"
	}
	return "Tor only"
}

func truncatePubkey(pubkey string, maxLen int) string {
	if len(pubkey) <= maxLen {
		return pubkey
	}
	side := (maxLen - 3) / 2
	return pubkey[:side] + "..." +
		pubkey[len(pubkey)-side:]
}

func formatSatsCompact(sats int64) string {
	if sats >= 100000000 {
		btc := float64(sats) / 100000000
		if btc == float64(int64(btc)) {
			return fmt.Sprintf("%.0fBTC", btc)
		}
		return fmt.Sprintf("%.1fBTC", btc)
	}
	if sats >= 1000000 {
		m := float64(sats) / 1000000
		if m == float64(int64(m)) {
			return fmt.Sprintf("%.0fM", m)
		}
		return fmt.Sprintf("%.1fM", m)
	}
	if sats >= 1000 {
		k := float64(sats) / 1000
		if k == float64(int64(k)) {
			return fmt.Sprintf("%.0fk", k)
		}
		return fmt.Sprintf("%.1fk", k)
	}
	return fmt.Sprintf("%d", sats)
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

func isHexChar(c byte) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'f') ||
		(c >= 'A' && c <= 'F')
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
		return 0, fmt.Errorf("min 20,000 sats")
	}
	if amt > 16777215 {
		return 0, fmt.Errorf("max 16,777,215 sats")
	}
	return amt, nil
}
