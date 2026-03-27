package welcome

import (
	"fmt"
	"image/color"
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
		Foreground(theme.ColorChanLocal).
		Render(strings.Repeat("█", localBarW))
	barRemote := lipgloss.NewStyle().
		Foreground(theme.ColorChanRemote).
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
		p := newPane(w)
		p.dim("LND is not installed.")
		p.dim("Go to System to install.")
		return p.render()
	}

	if !m.cfg.WalletExists() {
		p := newPane(w)
		p.dim("LND wallet not created.")
		return p.render()
	}

	if m.status == nil || !m.status.lndResponding {
		return theme.Dim.Render(" Waiting for LND...")
	}

	isFocused := m.contentFocused && !m.tabFocused

	// ── Fixed header ─────────────────────────────
	var headerLines []string
	headerLines = append(headerLines, "")

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

	headerLines = append(headerLines, "")
	headerLines = append(headerLines, "")

	headerLines = append(headerLines,
		m.renderBalanceSummary(w)...)

	headerLines = append(headerLines, "")
	headerLines = append(headerLines, "")

	header := strings.Join(headerLines, "\n")
	headerH := len(headerLines)

	// ── Fixed footer (buttons) ───────────────────
	isOnButton := isFocused && m.contentFocus == 1
	var footerLines []string
	footerLines = append(footerLines, "")
	footerLines = append(footerLines,
		renderButtons(
			[]string{"Open Channel", "History"},
			m.btnIdx, isOnButton, w))
	footerLines = append(footerLines, "")

	footer := strings.Join(footerLines, "\n")
	footerH := len(footerLines)

	// ── Scrollable middle (all channel bars) ─────
	chanCount := len(m.status.channels)
	nameW := 17
	barW := w - nameW - 22
	if barW < 8 {
		barW = 8
	}

	var midLines []string

	if chanCount == 0 {
		midLines = append(midLines,
			theme.Dim.Render(" No channels yet."))
	} else {
		for i, ch := range m.status.channels {
			if i > 0 {
				midLines = append(midLines, "")
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

			var lColor, rColor color.Color
			if isSelected {
				lColor = theme.ColorChanLocalActive
				rColor = theme.ColorChanRemoteActive
			} else if ch.Active {
				lColor = theme.ColorChanLocal
				rColor = theme.ColorChanRemote
			} else {
				lColor = theme.ColorChanLocalDim
				rColor = theme.ColorChanRemoteDim
			}

			lBar := lipgloss.NewStyle().
				Foreground(lColor).
				Render(
					strings.Repeat("█", localFill))
			rBar := lipgloss.NewStyle().
				Foreground(rColor).
				Render(strings.Repeat("█",
					remoteFill))
			barStr := lBar + rBar

			vals := fmt.Sprintf("%s / %s",
				formatSatsCompact(ch.LocalBalance),
				formatSatsCompact(ch.RemoteBalance))
			valsPad := pad(vals, 14)

			marker := " "
			nameStyle := theme.Value
			if isSelected {
				marker = "▸"
				nameStyle = theme.NavActive
			}
			namePad := pad(name, nameW)

			line := marker + " " + dot + " " +
				nameStyle.Render(namePad) + " " +
				barStr + " " +
				theme.Dim.Render(valsPad)

			midLines = append(midLines, line)
		}

		if m.status.pendingOpen > 0 {
			midLines = append(midLines, "")
			midLines = append(midLines,
				" "+theme.Dim.Render(
					fmt.Sprintf("%d pending",
						m.status.pendingOpen)))
		}
	}

	midContent := strings.Join(midLines, "\n")

	// ── Viewport ─────────────────────────────────
	vpH := h - headerH - footerH
	if vpH < 1 {
		vpH = 1
	}

	// Each channel is 2 lines (bar + blank gap)
	// except last which is 1 line
	cursorLine := m.chanCursor * 2

	vpRendered := renderViewport(
		midContent, w, vpH, cursorLine,
		len(midLines),
		chanCount > 0 && m.contentFocus == 0)

	// ── Assemble output ──────────────────────────
	return header + "\n" + vpRendered + "\n" + footer
}

// ── Channel detail ───────────────────────────────────────

func (m Model) channelDetailPane(w int) string {
	if m.chanCursor >= len(m.status.channels) {
		return theme.Dim.Render(" Channel not found")
	}
	ch := m.status.channels[m.chanCursor]

	name := ch.PeerAlias
	if name == "" {
		name = ch.RemotePubkey[:16] + "..."
	}

	p := newPane(w)
	p.title(theme.Header, name)

	status := theme.Success.Render("active")
	if !ch.Active {
		status = theme.Warning.Render("inactive")
	}
	if ch.Pending {
		status = theme.Dim.Render("pending")
	}

	p.line(" " + theme.Label.Render("Status:    ") +
		status)
	p.field("Capacity:  ",
		formatSats(ch.Capacity)+" sats")
	p.field("Local:     ",
		formatSats(ch.LocalBalance)+" sats")
	p.field("Remote:    ",
		formatSats(ch.RemoteBalance)+" sats")

	barW := w - 4
	if barW > 40 {
		barW = 40
	}
	if barW >= 10 {
		p.blank()
		p.line(" " + renderLiquidityBar(
			ch.LocalBalance, ch.RemoteBalance,
			ch.Capacity, barW))
	}
	p.blank()

	if ch.Private {
		p.field("Type:      ", "private")
	} else {
		p.field("Type:      ", "public")
	}
	if ch.Initiator {
		p.field("Initiator: ", "you")
	}

	p.blank()
	p.labelLine("Pubkey:")
	p.monoWrap(ch.RemotePubkey)

	if ch.ChanID > 0 {
		p.blank()
		p.monoField("Channel ID: ",
			fmt.Sprintf("%d", ch.ChanID))
	}

	// Close button (not shown for pending)
	if !ch.Pending {
		p.blank()
		isFocused := m.contentFocused &&
			!m.tabFocused
		isOnButton := isFocused &&
			m.contentFocus == 1
		p.line(renderButtons(
			[]string{"Close Channel"},
			0, isOnButton, w))
	}

	return p.render()
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
	p := newPane(w)
	p.title(theme.Header, "Open Channel")

	balText := "unknown"
	if m.status != nil && m.status.lndBalance != "" {
		balText = m.status.lndBalance + " sats"
	}
	p.field("On-chain: ", balText)
	p.blank()

	if m.cfg.P2PMode == "tor" {
		p.dim("Tor-only — (Tor) peers accept Tor")
		p.blank()
	}

	p.line(" " + theme.Header.Render("Select a peer:"))
	p.blank()

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
		p.line(fmt.Sprintf(" %s %s%s",
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
	p.line(fmt.Sprintf(" %s %s",
		prefix, style.Render("[Custom peer]")))

	return p.render()
}

func (m Model) channelCustomPeerPane(w int) string {
	p := newPane(w)
	p.title(theme.Header, "Custom Peer")

	isFocused := m.contentFocused && !m.tabFocused

	p.input("Node Pubkey:",
		m.chanPubkeyInput,
		isFocused && m.chanPubkeyInput.Focused())
	p.blank()
	p.input("Host (host:port):",
		m.chanHostInput,
		isFocused && m.chanHostInput.Focused())

	p.appendError(m.chanOpenError)

	return p.render()
}

func (m Model) channelAmountPane(w int) string {
	p := newPane(w)

	peerName := m.chanOpenAlias
	if peerName == "" && len(m.chanOpenPubkey) > 16 {
		peerName = m.chanOpenPubkey[:16] + "..."
	}
	p.title(theme.Header, peerName)

	balText := "unknown"
	if m.status != nil && m.status.lndBalance != "" {
		balText = m.status.lndBalance + " sats"
	}
	p.field("On-chain: ", balText)
	p.blank()
	p.line(" " + theme.Header.Render("Channel size:"))
	p.blank()

	isFocused := m.contentFocused && !m.tabFocused

	for i, amt := range amountPresets {
		prefix := " "
		style := theme.Value
		if isFocused && m.chanAmountPreset == i {
			prefix = "▸"
			style = theme.Action
		}
		if amt == 0 && m.chanAmountPreset == i {
			p.line(fmt.Sprintf(" %s %s",
				prefix, style.Render("Custom:")))
			inputW := w - 6
			if inputW > 20 {
				inputW = 20
			}
			ai := m.chanAmountInput
			ai.SetWidth(inputW)
			p.line("   " + ai.View())
			continue
		}
		p.line(fmt.Sprintf(" %s %s",
			prefix, style.Render(presetLabel(amt))))
	}

	p.appendError(m.chanOpenError)

	return p.render()
}

func (m Model) channelConfirmPane(w int) string {
	p := newPane(w)
	p.title(theme.Warning, "Confirm Channel Open")

	p.field("Peer:    ", m.chanOpenAlias)
	p.field("Amount:  ",
		formatSats(m.chanOpenAmount)+" sats")

	priv := "public"
	if m.chanOpenPrivate {
		priv = "private"
	}
	p.field("Type:    ", priv)
	p.blank()

	p.labelLine("Pubkey:")
	p.monoWrap(m.chanOpenPubkey)
	p.blank()
	p.warn("Spend " +
		formatSats(m.chanOpenAmount) + " sats?")

	p.appendError(m.chanOpenError)

	return p.render()
}

func (m Model) channelOpeningPane(w int) string {
	p := newPane(w)
	p.title(theme.Header, "Opening Channel...")
	p.line(" " + theme.Value.Render(
		"Connecting to peer and broadcasting tx."))
	p.blank()
	p.dim("May take up to 2 minutes over Tor.")
	p.dim("Do not close the terminal.")
	return p.render()
}

func (m Model) channelResultPane(w int) string {
	p := newPane(w)

	if m.chanOpenError != "" {
		p.title(theme.Warning, "Channel Open Failed")
		p.warn(m.chanOpenError)
	} else {
		p.title(theme.Success, "Channel Opening")
		p.line(" " + theme.Value.Render(
			"Funding tx broadcast successfully."))
		p.blank()
		p.field("Peer:   ", m.chanOpenAlias)
		p.field("Amount: ",
			formatSats(m.chanOpenAmount)+" sats")
		if m.chanOpenTxid != "" {
			p.blank()
			p.labelLine("TX ID:")
			p.monoWrap(m.chanOpenTxid)
		}
		p.blank()
		p.dim("Channel will appear as pending.")
	}

	return p.render()
}

func (m Model) channelFundPane(w int) string {
	p := newPane(w)
	p.title(theme.Warning, "Fund Your Wallet")

	p.line(" " + theme.Value.Render(
		"On-chain balance is empty."))
	p.line(" " + theme.Value.Render(
		"Send Bitcoin to this address:"))
	p.blank()

	if m.chanFundAddress != "" {
		p.monoWrap(m.chanFundAddress)
	} else {
		p.dim("Generating address...")
	}

	p.blank()
	p.dim("Wait for 1 confirmation,")
	p.dim("then return to open a channel.")

	return p.render()
}

// ── Channel close flow panes ─────────────────────────────

func (m Model) channelCloseContent(w int) string {
	switch m.subview {
	case svCloseType:
		return m.channelCloseTypePane(w)
	case svCloseConfirm:
		return m.channelCloseConfirmPane(w)
	case svClosing:
		return m.channelClosingPane(w)
	case svCloseResult:
		return m.channelCloseResultPane(w)
	}
	return ""
}

func (m Model) channelCloseTypePane(w int) string {
	p := newPane(w)
	p.title(theme.Header, "Close Channel")

	p.field("Peer:     ", m.closePeerAlias)
	p.field("Capacity: ",
		formatSats(m.closeCapacity)+" sats")
	p.field("Local:    ",
		formatSats(m.closeLocalBal)+" sats")
	p.blank()

	isFocused := m.contentFocused && !m.tabFocused

	p.line(" " +
		theme.Header.Render("Close type:"))
	p.blank()

	coopPrefix := " "
	coopStyle := theme.Value
	if isFocused && m.closeBtnIdx == 0 {
		coopPrefix = "▸"
		coopStyle = theme.Action
	}
	p.line(fmt.Sprintf(" %s %s",
		coopPrefix,
		coopStyle.Render("Cooperative close")))
	p.line("   " + theme.Dim.Render(
		"Requires peer online. Funds available"+
			" immediately."))
	p.blank()

	forcePrefix := " "
	forceStyle := theme.Value
	if isFocused && m.closeBtnIdx == 1 {
		forcePrefix = "▸"
		forceStyle = theme.Warning
	}
	p.line(fmt.Sprintf(" %s %s",
		forcePrefix,
		forceStyle.Render("Force close")))
	p.line("   " + theme.Dim.Render(
		"Unilateral. Funds locked ~2 weeks."))

	return p.render()
}

func (m Model) channelCloseConfirmPane(w int) string {
	p := newPane(w)

	if m.closeForce {
		p.title(theme.Warning, "⚠ Force Close Channel")
	} else {
		p.title(theme.Warning, "Close Channel")
	}

	p.field("Peer:     ", m.closePeerAlias)
	p.field("Capacity: ",
		formatSats(m.closeCapacity)+" sats")
	p.field("Your bal: ",
		formatSats(m.closeLocalBal)+" sats")
	p.blank()

	if m.closeForce {
		p.warn("⚠ Force close will lock your funds")
		p.warn("for up to 2,016 blocks (~2 weeks).")
		p.warn("Use cooperative close when possible.")
		p.blank()
	} else {
		// Fee tier display for cooperative close
		anyTier := false
		for _, t := range m.closeFeeTiers {
			if t.SatPerVB > 0 {
				anyTier = true
				break
			}
		}
		if anyTier {
			p.line(" " + theme.Label.Render(
				"Fee Rate:"))
			tierLine := " "
			isFocused := m.contentFocused &&
				!m.tabFocused
			for i, t := range m.closeFeeTiers {
				isSelected := isFocused &&
					m.closeFeeIdx == i
				var label string
				if t.SatPerVB > 0 {
					label = fmt.Sprintf("%s %.0f",
						t.Label, t.SatPerVB)
				} else {
					label = t.Label + " n/a"
				}
				if isSelected {
					tierLine += "▸ " +
						theme.BtnFocused.Render(
							label) + "  "
				} else {
					tierLine += "  " +
						theme.BtnNormal.Render(
							label) + "  "
				}
			}
			p.line(tierLine)
			p.blank()
		}
	}

	if m.closeForce {
		p.warn("Force close this channel?")
	} else {
		p.warn("Close this channel cooperatively?")
	}

	p.blank()
	p.dim("y confirm    ⌫ back")

	p.appendError(m.closeError)

	return p.render()
}

func (m Model) channelClosingPane(w int) string {
	p := newPane(w)
	if m.closeForce {
		p.title(theme.Warning,
			"Force Closing Channel...")
	} else {
		p.title(theme.Header,
			"Closing Channel...")
	}
	p.line(" " + theme.Value.Render(
		"Broadcasting close transaction."))
	p.blank()
	p.dim("May take up to 2 minutes over Tor.")
	p.dim("Do not close the terminal.")
	return p.render()
}

func (m Model) channelCloseResultPane(w int) string {
	p := newPane(w)

	if m.closeError != "" {
		p.title(theme.Warning, "Channel Close Failed")
		p.warn(m.closeError)
	} else {
		if m.closeForce {
			p.title(theme.Warning,
				"Force Close Broadcast")
			p.line(" " + theme.Value.Render(
				"Force close transaction broadcast."))
			p.blank()
			p.warn("Funds locked for ~2,016 blocks" +
				" (~2 weeks).")
		} else {
			p.title(theme.Success,
				"Channel Closing")
			p.line(" " + theme.Value.Render(
				"Cooperative close broadcast."))
		}
		p.blank()
		p.field("Peer:   ", m.closePeerAlias)
		if m.closeTxid != "" {
			p.blank()
			p.labelLine("Closing TX:")
			p.monoWrap(m.closeTxid)
		}
	}

	return p.render()
}

// ── Channel history pane ─────────────────────────────────

func (m Model) channelHistoryPane(w, h int) string {
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render("Channel History"),
			w))
	headerLines = append(headerLines, "")

	if len(m.chanHistory) == 0 {
		headerLines = append(headerLines,
			" "+theme.Dim.Render(
				"No channel history."))
		return strings.Join(headerLines, "\n")
	}

	isFocused := m.contentFocused && !m.tabFocused

	hdrStyle := theme.TableHeader
	sepStyle := theme.TableDim

	peerW := 16
	capW := 10
	statusW := 14
	closeW := w - peerW - capW - statusW - 5
	if closeW < 8 {
		closeW = 8
	}

	hdr := " " +
		hdrStyle.Render(pad("Peer", peerW)) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", capW, "Capacity")) +
		hdrStyle.Render(
			pad("  Status", statusW)) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", closeW, "Close"))
	headerLines = append(headerLines, hdr)
	headerLines = append(headerLines,
		" "+sepStyle.Render(
			strings.Repeat("─", w-2)))

	header := strings.Join(headerLines, "\n")
	headerH := len(headerLines)

	// ── Scrollable rows ──────────────────────────
	var midLines []string

	selStyle := lipgloss.NewStyle().
		Foreground(theme.ColorAccent).
		Bold(true)

	for i, ch := range m.chanHistory {
		isSelected := isFocused &&
			m.chanHistoryCursor == i

		peer := ch.PeerAlias
		if peer == "" {
			if len(ch.RemotePubkey) > 12 {
				peer = ch.RemotePubkey[:12] + ".."
			} else {
				peer = ch.RemotePubkey
			}
		}
		if len(peer) > peerW-1 {
			peer = peer[:peerW-2] + ".."
		}
		peerStr := pad(peer, peerW)

		capStr := fmt.Sprintf("%*s", capW,
			formatSatsCompact(ch.Capacity))

		statusStr := pad("  "+ch.Status, statusW)
		closeLabel := ch.CloseType
		if ch.Status == "waiting close" {
			closeLabel = "unconfirmed"
		} else if ch.BlocksRemaining > 0 {
			closeLabel = fmt.Sprintf("~%d blks",
				ch.BlocksRemaining)
		}
		closeStr := fmt.Sprintf("%*s",
			closeW, closeLabel)

		marker := " "
		if isSelected {
			marker = "▸"
			midLines = append(midLines,
				marker+
					selStyle.Render(peerStr)+
					selStyle.Render(capStr)+
					selStyle.Render(statusStr)+
					selStyle.Render(closeStr))
		} else {
			var statusRendered string
			switch ch.Status {
			case "active":
				statusRendered =
					theme.Good.Render(statusStr)
			case "inactive":
				statusRendered =
					theme.Warn.Render(statusStr)
			case "pending open":
				statusRendered =
					theme.Dim.Render(statusStr)
			case "pending close", "waiting close":
				statusRendered =
					theme.Warn.Render(statusStr)
			case "force close":
				statusRendered =
					theme.Warning.Render(statusStr)
			case "closed":
				statusRendered =
					theme.Dim.Render(statusStr)
			default:
				statusRendered =
					theme.Dim.Render(statusStr)
			}

			var closeRendered string
			switch ch.CloseType {
			case "force", "breach":
				closeRendered =
					theme.Warning.Render(closeStr)
			default:
				closeRendered =
					theme.Dim.Render(closeStr)
			}

			midLines = append(midLines,
				marker+
					theme.Value.Render(peerStr)+
					theme.Value.Render(capStr)+
					statusRendered+
					closeRendered)
		}
	}

	midContent := strings.Join(midLines, "\n")

	// ── Viewport ─────────────────────────────────
	vpH := h - headerH
	if vpH < 1 {
		vpH = 1
	}

	vpRendered := renderViewport(
		midContent, w, vpH,
		m.chanHistoryCursor,
		len(midLines),
		isFocused && len(m.chanHistory) > 0)

	// ── Assemble output ──────────────────────────
	return header + "\n" + vpRendered
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
		Foreground(theme.ColorChanLocal).
		Render(strings.Repeat("█", lw)) +
		lipgloss.NewStyle().
			Foreground(theme.ColorChanRemote).
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
