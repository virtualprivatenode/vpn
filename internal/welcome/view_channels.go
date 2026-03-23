package welcome

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Balance summary (shared by channels + wallet) ────────

func (m Model) renderBalanceSummary(w int) []string {
	if m.status == nil || !m.status.lndResponding {
		return nil
	}

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

	localPct := 0
	if totalCap > 0 {
		localPct = int(
			float64(totalLocal) * 100 /
				float64(totalCap))
	}
	remotePct := 100 - localPct

	labelW := 11
	leftColW := labelW + 18
	boxW := w - leftColW - 2
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
	barPadR := boxW - 2 - barVis
	if barPadR < 0 {
		barPadR = 0
	}

	chLabel := fmt.Sprintf("%d channels",
		activeCount+inactiveCount)
	if inactiveCount > 0 {
		chLabel = fmt.Sprintf("%d on, %d off",
			activeCount, inactiveCount)
	}

	pctInner := fmt.Sprintf("Out %d%%    In %d%%",
		localPct, remotePct)
	if len(pctInner) > boxW-2 {
		pctInner = fmt.Sprintf("Out %d%% In %d%%",
			localPct, remotePct)
	}

	capStr := formatSats(totalCap) + " sats"

	boxTop := "┌" + strings.Repeat("─", boxW-2) + "┐"
	boxLabel := "│" +
		centerPad(chLabel, boxW-2) + "│"
	boxBar := "│" + barLine +
		strings.Repeat(" ", barPadR) + "│"
	boxPct := "│" + theme.Dim.Render(
		centerPad(pctInner, boxW-2)) + "│"
	boxCap := "│" + theme.Dim.Render(
		centerPad(capStr, boxW-2)) + "│"
	boxBot := "└" + strings.Repeat("─", boxW-2) + "┘"

	boxLines := []string{
		boxTop, boxLabel, boxBar,
		boxPct, boxCap, boxBot,
	}

	leftLines := []string{
		" " + theme.Label.Render("Outbound: ") +
			theme.Value.Render(
				formatSats(totalLocal)+" sats"),
		"",
		" " + theme.Label.Render("Inbound:  ") +
			theme.Value.Render(
				formatSats(totalRemote)+" sats"),
		"",
		" " + theme.Label.Render("On-chain: ") +
			theme.Value.Render(
				formatSats(parseBalance(onchain))+
					" sats"),
		"",
	}

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

	var result []string
	for i := 0; i < maxH; i++ {
		lft := leftLines[i]
		lftW := lipgloss.Width(lft)
		if lftW < leftColW {
			lft += strings.Repeat(" ",
				leftColW-lftW)
		}
		result = append(result, lft+" "+boxLines[i])
	}
	return result
}

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

	isFocused := m.contentFocused && !m.tabFocused

	// ── Fixed regions ────────────────────────────
	var headerLines []string
	headerLines = append(headerLines, "")

	// Pubkey and P2P
	if m.status.lndPubkey != "" {
		pk := truncatePubkey(m.status.lndPubkey, w-14)
		headerLines = append(headerLines,
			" "+theme.Label.Render("Pubkey:   ")+
				theme.Mono.Render(pk))
	}
	headerLines = append(headerLines,
		" "+theme.Label.Render("P2P:      ")+
			theme.Value.Render(
				p2pModeLabel(m.cfg.P2PMode)))

	// 2-line gap after P2P
	headerLines = append(headerLines, "")
	headerLines = append(headerLines, "")

	// Balance summary with liquidity box
	headerLines = append(headerLines,
		m.renderBalanceSummary(w)...)

	// 3-line gap before channel bars
	headerLines = append(headerLines, "")
	headerLines = append(headerLines, "")

	headerH := len(headerLines)
	buttonH := 3 // top pad + button + bottom pad
	gapBelowChans := 0

	// Available rows for channel bars
	chanWindowH := h - headerH - buttonH - gapBelowChans
	if chanWindowH < 1 {
		chanWindowH = 1
	}

	// ── Channel bars ─────────────────────────────
	chanCount := len(m.status.channels)
	nameW := 17
	barW := w - nameW - 22
	if barW < 8 {
		barW = 8
	}

	var chanLines []string
	var chanLineToIdx []int

	if chanCount == 0 {
		chanLines = append(chanLines,
			theme.Dim.Render(" No channels yet."))
		chanLineToIdx = append(chanLineToIdx, -1)
	} else {
		for i, ch := range m.status.channels {
			if i > 0 {
				chanLines = append(chanLines, "")
				chanLineToIdx = append(
					chanLineToIdx, -1)
			}

			isSelected := isFocused &&
				m.chanCursor == i &&
				m.contentFocus == 0

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
				Render(
					strings.Repeat("█", localFill))
			rBar := lipgloss.NewStyle().
				Foreground(
					lipgloss.Color(remoteColor)).
				Render(
					strings.Repeat("█", remoteFill))
			barStr := lBar + rBar

			vals := fmt.Sprintf("%s / %s",
				formatSatsCompact(ch.LocalBalance),
				formatSatsCompact(ch.RemoteBalance))
			valsPad := pad(vals, 14)

			marker := " "
			nameStyle := theme.Value
			if isSelected {
				marker = "▸"
				nameStyle = navActiveStyle
			}
			namePad := pad(name, nameW)

			line := marker + " " + dot + " " +
				nameStyle.Render(namePad) + " " +
				barStr + " " +
				theme.Dim.Render(valsPad)

			chanLines = append(chanLines, line)
			chanLineToIdx = append(chanLineToIdx, i)
		}

		if m.status.pendingOpen > 0 {
			chanLines = append(chanLines, "")
			chanLineToIdx = append(chanLineToIdx, -1)
			chanLines = append(chanLines,
				" "+theme.Dim.Render(
					fmt.Sprintf("%d pending",
						m.status.pendingOpen)))
			chanLineToIdx = append(chanLineToIdx, -1)
		}
	}

	// Scroll window
	totalChanLines := len(chanLines)
	needsScroll := totalChanLines > chanWindowH

	cursorLineIdx := 0
	for li, ci := range chanLineToIdx {
		if ci == m.chanCursor {
			cursorLineIdx = li
			break
		}
	}

	scrollOffset := m.chanScrollOffset
	if needsScroll {
		if cursorLineIdx < scrollOffset {
			scrollOffset = cursorLineIdx
		}
		if cursorLineIdx >= scrollOffset+chanWindowH {
			scrollOffset = cursorLineIdx -
				chanWindowH + 1
		}
		maxOffset := totalChanLines - chanWindowH
		if maxOffset < 0 {
			maxOffset = 0
		}
		if scrollOffset > maxOffset {
			scrollOffset = maxOffset
		}
		if scrollOffset < 0 {
			scrollOffset = 0
		}
	} else {
		scrollOffset = 0
	}

	visEnd := scrollOffset + chanWindowH
	if visEnd > totalChanLines {
		visEnd = totalChanLines
	}
	visibleChanLines := chanLines[scrollOffset:visEnd]

	// ── Assemble output ──────────────────────────
	var lines []string

	lines = append(lines, headerLines...)

	if needsScroll && scrollOffset > 0 {
		indicator := strings.Repeat(" ", w-4) +
			theme.Dim.Render(" ▲")
		lines = append(lines, indicator)
		if len(visibleChanLines) > 1 {
			visibleChanLines =
				visibleChanLines[1:]
		}
	}

	lines = append(lines, visibleChanLines...)

	rendered := len(visibleChanLines)
	if needsScroll && scrollOffset > 0 {
		rendered++
	}
	for rendered < chanWindowH {
		lines = append(lines, "")
		rendered++
	}

	if needsScroll && visEnd < totalChanLines {
		indicator := strings.Repeat(" ", w-4) +
			theme.Dim.Render(" ▼")
		if len(lines) > 0 {
			lines[len(lines)-1] = indicator
		}
	}

	// ── Open Channel button (full width, centered)
	label := "Open Channel"
	isOnButton := isFocused && m.contentFocus == 1

	btnW := w - 2
	if btnW < 16 {
		btnW = 16
	}

	var btnStr string
	if isOnButton {
		btnStr = theme.BtnFocused.
			Width(btnW).
			AlignHorizontal(lipgloss.Center).
			Render(label)
	} else {
		btnStr = theme.BtnNormal.
			Width(btnW).
			AlignHorizontal(lipgloss.Center).
			Render(label)
	}

	lines = append(lines, "")
	lines = append(lines, " "+btnStr)
	lines = append(lines, "")

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

	isFocused := m.contentFocused && !m.tabFocused

	for i, peer := range m.chanPeerList {
		prefix := " "
		style := theme.Value
		if isFocused && m.chanOpenPeerIdx == i {
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
	if isFocused &&
		m.chanOpenPeerIdx == len(m.chanPeerList) {
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
		"↑↓ switch fields  Enter continue"))

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

	isFocused := m.contentFocused && !m.tabFocused

	for i, amt := range amountPresets {
		prefix := " "
		style := theme.Value
		if isFocused && m.chanAmountPreset == i {
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
		"y confirm  p toggle private  "+
			"backspace cancel"))

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
		"backspace to go back"))

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
