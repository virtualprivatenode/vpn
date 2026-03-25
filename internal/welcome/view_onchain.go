package welcome

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── On-Chain overview ────────────────────────────────────

func (m Model) onChainOverview(w, h int) string {
	isFocused := m.contentFocused && !m.tabFocused

	// ── Fixed header ─────────────────────────────
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render("On-Chain Wallet"),
			w))
	headerLines = append(headerLines, "")

	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		headerLines = append(headerLines,
			theme.Dim.Render(
				" Install LND and create wallet."))
		return strings.Join(headerLines, "\n")
	}
	if m.status == nil || !m.status.lndResponding {
		headerLines = append(headerLines,
			theme.Dim.Render(" Waiting for LND..."))
		return strings.Join(headerLines, "\n")
	}

	onchain := "0"
	if m.status.lndBalance != "" {
		onchain = m.status.lndBalance
	}
	headerLines = append(headerLines,
		" "+theme.Label.Render("Balance:  ")+
			theme.Value.Render(
				formatSats(parseBalance(onchain))+
					" sats"))
	headerLines = append(headerLines, "")

	if m.onChainAddress != "" {
		headerLines = append(headerLines,
			" "+theme.Label.Render("Address:"))
		addr := m.onChainAddress
		if len(addr) > w-4 {
			addr = addr[:w-7] + "..."
		}
		headerLines = append(headerLines,
			" "+theme.Mono.Render(addr))
	}
	headerLines = append(headerLines, "")

	headerLines = append(headerLines,
		renderButtons(
			[]string{"Receive", "Send", "Refresh"},
			m.onChainBtnIdx,
			isFocused && m.onChainTxFocus == 0,
			w))
	headerLines = append(headerLines, "")

	header := strings.Join(headerLines, "\n")
	headerH := len(headerLines)

	// ── Transaction table header (fixed) ─────────
	hdrStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Bold(true)
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	dateW := 12
	typeW := 16
	amtW := 14
	confW := w - dateW - typeW - amtW - 5
	if confW < 6 {
		confW = 6
	}

	var txHeaderLines []string
	txHeaderLines = append(txHeaderLines,
		" "+theme.Header.Render("Transactions"))
	txHeaderLines = append(txHeaderLines, "")

	txHdr := " " +
		hdrStyle.Render(pad("Date", dateW)) +
		hdrStyle.Render(pad("Label", typeW)) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", amtW, "Amount")) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", confW, "Confs"))
	txHeaderLines = append(txHeaderLines, txHdr)
	txHeaderLines = append(txHeaderLines,
		" "+sepStyle.Render(
			strings.Repeat("─", w-2)))

	txHeader := strings.Join(txHeaderLines, "\n")
	txHeaderH := len(txHeaderLines)

	// ── Scrollable tx rows ───────────────────────
	var txMidLines []string

	if len(m.onChainTxs) == 0 {
		txMidLines = append(txMidLines,
			" "+theme.Dim.Render(
				"No on-chain transactions."))
	} else {
		for i, tx := range m.onChainTxs {
			isSelected := isFocused &&
				m.onChainTxFocus == 2 &&
				m.onChainTxCursor == i

			date := formatTimestamp(tx.Timestamp)
			dateStr := pad(date, dateW)
			txType := tx.Label
			if len(txType) > typeW-1 {
				txType = txType[:typeW-2] + ".."
			}
			typeStr := pad(txType, typeW)

			var amtStr string
			if tx.Amount >= 0 {
				amtStr = fmt.Sprintf("%*s", amtW,
					"+"+formatSats(tx.Amount))
			} else {
				amtStr = fmt.Sprintf("%*s", amtW,
					formatSats(tx.Amount))
			}
			confStr := fmt.Sprintf("%*d",
				confW, tx.Confirmations)
			if tx.Confirmations == 0 {
				confStr = fmt.Sprintf(
					"%*s", confW, "pending")
			}

			marker := " "
			if isSelected {
				marker = "▸"
				selStyle := lipgloss.NewStyle().
					Foreground(
						lipgloss.Color("220")).
					Bold(true)
				txMidLines = append(txMidLines,
					marker+
						selStyle.Render(dateStr)+
						selStyle.Render(typeStr)+
						selStyle.Render(amtStr)+
						selStyle.Render(confStr))
			} else {
				amtStyle := lipgloss.NewStyle().
					Foreground(
						lipgloss.Color("15"))
				if tx.Amount < 0 {
					amtStyle = lipgloss.NewStyle().
						Foreground(
							lipgloss.Color("196"))
				}
				txMidLines = append(txMidLines,
					marker+
						theme.Dim.Render(dateStr)+
						theme.Value.Render(typeStr)+
						amtStyle.Render(amtStr)+
						theme.Dim.Render(confStr))
			}
		}
	}

	txMidContent := strings.Join(txMidLines, "\n")

	// ── UTXO table header (fixed) ────────────────
	txidW := 20
	utxoAmtW := 14
	utxoConfW := 8
	addrW := w - txidW - utxoAmtW - utxoConfW - 6
	if addrW < 10 {
		addrW = 10
	}

	var utxoHeaderLines []string
	utxoHeaderLines = append(utxoHeaderLines, "")
	utxoHeaderLines = append(utxoHeaderLines,
		" "+theme.Header.Render("UTXOs"))
	utxoHeaderLines = append(utxoHeaderLines, "")

	utxoHdr := " " +
		hdrStyle.Render(pad("Txid", txidW)) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", utxoAmtW, "Amount")) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", utxoConfW, "Confs")) +
		hdrStyle.Render(
			fmt.Sprintf("  %-*s", addrW, "Address"))
	utxoHeaderLines = append(utxoHeaderLines, utxoHdr)
	utxoHeaderLines = append(utxoHeaderLines,
		" "+sepStyle.Render(
			strings.Repeat("─", w-2)))

	utxoHeader := strings.Join(utxoHeaderLines, "\n")
	utxoHeaderH := len(utxoHeaderLines)

	// ── Scrollable UTXO rows ─────────────────────
	var utxoMidLines []string

	if len(m.utxos) == 0 {
		utxoMidLines = append(utxoMidLines,
			" "+theme.Dim.Render("No UTXOs found."))
	} else {
		for i, u := range m.utxos {
			isSelected := isFocused &&
				m.onChainTxFocus == 1 &&
				m.utxoCursor == i

			txid := u.Txid
			if len(txid) > txidW-3 {
				txid = txid[:txidW-3] + "..."
			}
			txidStr := pad(txid, txidW)
			uAmtStr := fmt.Sprintf("%*s", utxoAmtW,
				formatSats(u.AmountSats))
			uConfStr := fmt.Sprintf("%*d",
				utxoConfW, u.Confirmations)
			uAddr := u.Address
			if len(uAddr) > addrW {
				uAddr = uAddr[:addrW-3] + "..."
			}
			uAddrStr := fmt.Sprintf("  %-*s",
				addrW, uAddr)

			marker := " "
			if isSelected {
				marker = "▸"
				selStyle := lipgloss.NewStyle().
					Foreground(
						lipgloss.Color("220")).
					Bold(true)
				utxoMidLines = append(utxoMidLines,
					marker+
						selStyle.Render(txidStr)+
						selStyle.Render(uAmtStr)+
						selStyle.Render(uConfStr)+
						selStyle.Render(uAddrStr))
			} else {
				utxoMidLines = append(utxoMidLines,
					marker+
						theme.Mono.Render(txidStr)+
						theme.Value.Render(uAmtStr)+
						theme.Dim.Render(uConfStr)+
						theme.Dim.Render(uAddrStr))
			}
		}
	}

	utxoMidContent := strings.Join(utxoMidLines, "\n")

	// ── Size viewports ───────────────────────────
	fixedH := headerH + txHeaderH + utxoHeaderH
	remainH := h - fixedH
	if remainH < 2 {
		remainH = 2
	}

	txVPH := remainH / 2
	utxoVPH := remainH - txVPH
	if txVPH < 1 {
		txVPH = 1
	}
	if utxoVPH < 1 {
		utxoVPH = 1
	}

	// ── UTXO viewport ────────────────────────────
	utxoVPRendered := renderViewport(
		utxoMidContent, w, utxoVPH,
		m.utxoCursor,
		len(utxoMidLines),
		len(m.utxos) > 0 &&
			m.onChainTxFocus == 1)

	// ── TX viewport ──────────────────────────────
	txVPRendered := renderViewport(
		txMidContent, w, txVPH,
		m.onChainTxCursor,
		len(txMidLines),
		len(m.onChainTxs) > 0 &&
			m.onChainTxFocus == 2)

	/// ── Assemble output ──────────────────────────
	return header + "\n" +
		utxoHeader + "\n" +
		utxoVPRendered + "\n" +
		txHeader + "\n" +
		txVPRendered
}

// ── On-Chain Receive pane ────────────────────────────────

func (m Model) onChainReceivePane(w int) string {
	p := newPane(w)
	p.title(theme.Header, "⛓ Receive On-Chain")

	if m.ocRecvAddress == "" {
		p.dim("Generating address...")
		return p.render()
	}

	p.labelLine("Address:")
	addr := m.ocRecvAddress
	if len(addr) > w-4 {
		addr = addr[:w-7] + "..."
	}
	p.mono(addr)
	p.blank()
	p.dim("Send Bitcoin to this address.")
	p.dim("Funds appear after 1 confirmation.")
	p.blank()

	btnFocused := m.contentFocused && !m.tabFocused
	p.buttons(
		[]string{"Show QR", "New Address"},
		m.ocRecvBtnIdx, btnFocused)

	p.appendError(m.ocRecvError)

	return p.render()
}

// ── On-Chain send panes ──────────────────────────────────

func (m Model) onChainSendAddrPane(w int) string {
	p := newPane(w)
	p.title(theme.Header, "⛓ Send On-Chain")

	onchain := "0"
	if m.status != nil && m.status.lndBalance != "" {
		onchain = m.status.lndBalance
	}
	p.field("Balance:  ",
		formatSats(parseBalance(onchain))+" sats")
	p.blank()

	p.input("Destination Address:",
		m.ocSendAddrInput,
		m.contentFocused && !m.tabFocused)

	p.appendError(m.onChainSendError)
	return p.render()
}

func (m Model) onChainSendAmountPane(w int) string {
	p := newPane(w)
	p.title(theme.Header, "⛓ Send On-Chain")

	isFocused := m.contentFocused && !m.tabFocused
	addr := m.ocSendAddrVal
	if len(addr) > w-14 {
		addr = addr[:w-17] + "..."
	}
	p.monoField("To: ", addr)
	p.blank()

	amtActive := isFocused && m.ocSendStep == 0
	if m.ocSendAll {
		p.input("Amount:", m.ocSendAmtInput, false)
		p.lines[len(p.lines)-1] = " " +
			theme.Action.Render("[Send All]")
	} else {
		p.input("Amount (sats):",
			m.ocSendAmtInput, amtActive)
	}
	p.dim("Tab to toggle Send All")
	p.blank()

	feeActive := isFocused && m.ocSendStep == 1
	feeLabelStyle := theme.Label
	if feeActive {
		feeLabelStyle = navActiveStyle
	}
	p.line(" " + feeLabelStyle.Render("Fee Rate:"))

	anyTier := false
	for _, t := range m.ocFeeTiers {
		if t.SatPerVB > 0 {
			anyTier = true
			break
		}
	}
	if !anyTier {
		p.dim("Loading fee estimates...")
	} else {
		tierLine := " "
		for i, t := range m.ocFeeTiers {
			isSelected := isFocused &&
				m.ocSendStep == 1 &&
				m.ocSelectedTier == i
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
		customLabel := "Custom"
		isCustom := isFocused &&
			m.ocSendStep == 1 &&
			m.ocSelectedTier == 4
		if isCustom {
			tierLine += "▸ " +
				theme.BtnFocused.Render(
					customLabel)
		} else {
			tierLine += "  " +
				theme.BtnNormal.Render(
					customLabel)
		}
		p.line(tierLine)

		if m.ocSelectedTier == 4 {
			p.blank()
			custActive := isFocused &&
				m.ocSendStep == 2
			p.input("sat/vB:",
				m.ocCustomFeeInput, custActive)
		}
	}

	p.appendError(m.onChainSendError)

	return p.render()
}

func (m Model) onChainSendConfirmPane(w int) string {
	p := newPane(w)
	p.title(theme.Warning, "Confirm On-Chain Send")

	addr := m.ocSendAddrVal
	if len(addr) > w-14 {
		addr = addr[:w-17] + "..."
	}
	p.monoField("To:       ", addr)
	if m.ocSendAll {
		p.field("Amount:   ", "Send All")
	} else {
		p.field("Amount:   ",
			formatSats(m.ocSendAmtVal)+" sats")
	}
	p.field("Fee Rate: ",
		fmt.Sprintf("%d sat/vB", m.ocSendFeeRate))
	if m.ocSelectedTier < 4 {
		tier := m.ocFeeTiers[m.ocSelectedTier]
		p.field("Target:   ", tier.Label)
	}
	if m.ocConfirmFee > 0 {
		p.field("Est. Fee: ",
			formatSats(m.ocConfirmFee)+" sats")
		if !m.ocSendAll && m.ocSendAmtVal > 0 {
			total := m.ocSendAmtVal + m.ocConfirmFee
			p.field("Total:    ",
				formatSats(total)+" sats")
		}
	}
	p.blank()
	if m.ocSendAll {
		p.warn("Send entire balance?")
	} else {
		p.warn("Send " +
			formatSats(m.ocSendAmtVal) + " sats?")
	}

	p.appendError(m.onChainSendError)

	return p.render()
}

func (m Model) onChainSendBroadcastPane(
	w int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Broadcasting...")
	p.line(" " + theme.Value.Render(
		"Sending transaction to the network."))
	p.blank()
	p.dim("Do not close the terminal.")
	return p.render()
}

func (m Model) onChainResultContent(w int) string {
	p := newPane(w)

	if m.onChainSendError != "" {
		p.title(theme.Warning,
			"On-Chain Send Failed")
		p.warn(m.onChainSendError)
	} else {
		p.title(theme.Success,
			"Transaction Broadcast")
		if m.onChainSendTxid != "" {
			p.labelLine("TX ID:")
			txid := m.onChainSendTxid
			if len(txid) > w-4 {
				txid = txid[:w-7] + "..."
			}
			p.mono(txid)
		}
	}

	return p.render()
}

// ── On-Chain tx detail ───────────────────────────────────

func (m Model) onChainTxDetailPane(
	tx lndrpc.OnChainTx, w int,
) string {
	p := newPane(w)

	switch tx.TxType {
	case "channel_open":
		p.title(theme.Header, "⚡ Channel Open")
	case "channel_close":
		p.title(theme.Warning, "⚡ Channel Close")
	case "send":
		p.title(theme.Warning, "↑ On-Chain Send")
	default:
		p.title(theme.Success, "↓ On-Chain Receive")
	}

	if tx.ChannelPeer != "" {
		p.field("Peer:    ", tx.ChannelPeer)
	}
	absAmt := tx.Amount
	if absAmt < 0 {
		absAmt = -absAmt
	}
	p.field("Amount:  ",
		formatSats(absAmt)+" sats")
	if tx.Fee > 0 {
		p.field("Fee:     ",
			formatSats(tx.Fee)+" sats")
	}
	confStr := fmt.Sprintf("%d", tx.Confirmations)
	if tx.Confirmations == 0 {
		confStr = "pending"
	}
	p.field("Confs:   ", confStr)
	if tx.BlockHeight > 0 {
		p.field("Block:   ",
			fmt.Sprintf("%d", tx.BlockHeight))
	}
	p.field("Date:    ",
		formatTimestampFull(tx.Timestamp))

	p.blank()
	p.labelLine("TX ID:")
	txid := tx.Txid
	if len(txid) > w-4 {
		txid = txid[:w-7] + "..."
	}
	p.mono(txid)

	if len(tx.Inputs) > 0 {
		p.blank()
		p.labelLine("Inputs")
		for i, inp := range tx.Inputs {
			isLast := i == len(tx.Inputs)-1
			connector := "├──"
			if isLast {
				connector = "└──"
			}
			ownership := ""
			if inp.IsOurs {
				ownership = " (ours)"
			}
			// 8 = "  ├── " prefix + trailing space
			maxOP := w - 8 - len(ownership)
			if maxOP < 16 {
				maxOP = 16
			}
			outpoint := inp.Outpoint
			if len(outpoint) > maxOP {
				outpoint = outpoint[:maxOP-3] + "..."
			}
			line := fmt.Sprintf("  %s %s%s",
				connector,
				theme.Mono.Render(outpoint),
				theme.Dim.Render(ownership))
			p.line(line)
			if !isLast {
				p.line("  │")
			}
		}
	}

	if len(tx.Outputs) > 0 {
		p.blank()
		p.labelLine("Outputs")
		for i, out := range tx.Outputs {
			amtStr := formatSats(out.Amount)
			if out.Amount == 0 {
				amtStr = "—"
			}
			labelStr := ""
			if out.Label != "" {
				labelStr = " (" + out.Label + ")"
			}
			// 13 = "  ├── " + "  " + " sats"
			fixedW := 13 + len(amtStr) +
				len(labelStr)
			addrMax := w - fixedW
			if addrMax < 12 {
				addrMax = 12
			}
			addr := out.Address
			if len(addr) > addrMax {
				addr = addr[:addrMax-3] + "..."
			}
			isLast := i == len(tx.Outputs)-1
			connector := "├──"
			if isLast {
				connector = "└──"
			}
			addrStyle := theme.Mono
			if out.Label == "destination" ||
				out.Label == "channel" {
				addrStyle = theme.Value
			}
			line := fmt.Sprintf("  %s %s  %s%s",
				connector,
				addrStyle.Render(addr),
				theme.Value.Render(amtStr+" sats"),
				theme.Dim.Render(labelStr))
			p.line(line)
			if !isLast {
				p.line("  │")
			}
		}
	}

	if tx.Fee > 0 {
		p.blank()
		p.field("Fee:     ",
			formatSats(tx.Fee)+" sats")
	}
	return p.render()
}

// ── UTXO detail pane ────────────────────────────────────

func (m Model) utxoDetailPane(w int) string {
	if m.utxoCursor >= len(m.utxos) {
		return theme.Dim.Render(" UTXO not found")
	}
	u := m.utxos[m.utxoCursor]

	p := newPane(w)
	p.title(theme.Header, "UTXO Detail")

	p.field("Amount:  ",
		formatSats(u.AmountSats)+" sats")
	confStr := fmt.Sprintf("%d", u.Confirmations)
	if u.Confirmations == 0 {
		confStr = "pending"
	}
	p.field("Confs:   ", confStr)
	p.blank()

	p.labelLine("Address:")
	addr := u.Address
	if len(addr) > w-4 {
		addr = addr[:w-7] + "..."
	}
	p.mono(addr)
	p.blank()

	p.labelLine("Outpoint:")
	outpoint := fmt.Sprintf("%s:%d", u.Txid, u.Vout)
	if len(outpoint) > w-4 {
		outpoint = outpoint[:w-7] + "..."
	}
	p.mono(outpoint)
	p.blank()

	// Label (from matching transaction)
	txLabel := m.utxoTxLabel(u.Txid)
	if m.utxoLabelEditing {
		p.labelLine("Label:")
		p.line("  " + m.utxoLabelInput.View())
	} else {
		if txLabel != "" {
			p.field("Label:   ", txLabel)
		} else {
			p.field("Label:   ",
				theme.Dim.Render("none"))
		}
		isFocused := m.contentFocused &&
			!m.tabFocused
		if isFocused && m.contentFocus == 1 {
			p.blank()
			p.line(renderButtons(
				[]string{"Edit Label"},
				0, true, w))
		} else {
			p.blank()
			p.line(renderButtons(
				[]string{"Edit Label"},
				0, false, w))
		}
	}

	return p.render()
}

// utxoTxLabel looks up the transaction label for
// a UTXO's parent transaction.
func (m Model) utxoTxLabel(txid string) string {
	for _, tx := range m.onChainTxs {
		if tx.Txid == txid {
			return tx.Label
		}
	}
	return ""
}
