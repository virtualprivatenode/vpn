package welcome

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"

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

	sendLabel := "Send"
	if len(m.utxoSelected) > 0 {
		sendLabel = fmt.Sprintf("Send Selected (%s)",
			formatSats(m.utxoSelectedTotal))
	}
	headerLines = append(headerLines,
		renderButtons(
			[]string{"Receive", sendLabel},
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
	confW = max(confW, 6)

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
	addrW = max(addrW, 10)

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
			isChecked := m.utxoSelected[i]

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
			if isChecked {
				marker = "✓"
			}
			if isSelected && !isChecked {
				marker = "▸"
			}

			switch {
			case isSelected:
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
			case isChecked:
				checkStyle := lipgloss.NewStyle().
					Foreground(
						lipgloss.Color("82"))
				utxoMidLines = append(utxoMidLines,
					marker+
						checkStyle.Render(txidStr)+
						checkStyle.Render(uAmtStr)+
						checkStyle.Render(uConfStr)+
						checkStyle.Render(uAddrStr))
			default:
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
	remainH := max(h-fixedH, 2)

	txVPH := remainH / 2
	utxoVPH := remainH - txVPH
	txVPH = max(txVPH, 1)
	utxoVPH = max(utxoVPH, 1)

	utxoVPRendered := renderViewport(
		utxoMidContent, w, utxoVPH,
		m.utxoCursor,
		len(utxoMidLines),
		len(m.utxos) > 0 &&
			m.onChainTxFocus == 1)

	txVPRendered := renderViewport(
		txMidContent, w, txVPH,
		m.onChainTxCursor,
		len(txMidLines),
		len(m.onChainTxs) > 0 &&
			m.onChainTxFocus == 2)

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

// ── On-Chain single-screen send pane ─────────────────────
//
// Steps:
//
//	0 = Address input
//	1 = Amount input
//	2 = Max / Send All button
//	3 = Fee tier selector (1/2/3 sat/vB + Custom)
//	4 = Custom fee input (only when Custom selected)
//	5 = Buttons (Clear / Create Transaction)
func (m Model) onChainSendPane(w, h int) string {
	isFocused := m.contentFocused && !m.tabFocused

	var lines []string
	lines = append(lines, "")
	lines = append(lines, centerPad(
		theme.Header.Render("⛓ Send On-Chain"), w))
	lines = append(lines, "")

	// Balance
	onchain := "0"
	if m.status != nil && m.status.lndBalance != "" {
		onchain = m.status.lndBalance
	}
	lines = append(lines,
		" "+theme.Label.Render("Balance:  ")+
			theme.Value.Render(
				formatSats(parseBalance(onchain))+
					" sats"))
	lines = append(lines, "")

	// ── Address input (step 0) ───────────────────
	addrActive := isFocused && m.ocSendStep == 0
	addrLabel := theme.Label
	addrMarker := " "
	if addrActive {
		addrLabel = navActiveStyle
		addrMarker = navActiveStyle.Render("▸")
	}
	lines = append(lines,
		" "+addrLabel.Render("To:"))
	lines = append(lines,
		addrMarker+" "+m.ocSendAddrInput.View())
	lines = append(lines, "")

	// ── Amount input (step 1) ────────────────────
	amtActive := isFocused && m.ocSendStep == 1
	amtLabel := theme.Label
	amtMarker := " "
	if amtActive {
		amtLabel = navActiveStyle
		amtMarker = navActiveStyle.Render("▸")
	}

	lines = append(lines,
		" "+amtLabel.Render("Amount (sats):"))
	if m.ocSendAll {
		// Show the computed amount as read-only
		amtVal := m.ocSendAmtInput.Value()
		if amtVal == "" {
			amtVal = "calculating..."
		} else {
			parsed := parseSendAmount(amtVal)
			if parsed > 0 {
				amtVal = formatSats(parsed)
			}
		}
		lines = append(lines,
			"  "+theme.Value.Render(amtVal+" sats")+
				"  "+theme.Dim.Render("(max)"))
	} else {
		lines = append(lines,
			amtMarker+" "+m.ocSendAmtInput.View())
	}
	lines = append(lines, "")

	// ── Max / Send All button (step 2) ───────────
	maxActive := isFocused && m.ocSendStep == 2
	maxLabel := "Max"
	if len(m.utxoSelected) > 0 && !m.ocSendAll {
		maxLabel = fmt.Sprintf(
			"Max (%d UTXOs: %s sats)",
			len(m.utxoSelected),
			formatSats(m.utxoSelectedTotal))
	}
	if m.ocSendAll {
		maxLabel = "Clear Max"
	}

	var maxBtn string
	switch {
	case maxActive:
		maxBtn = theme.BtnFocused.Render(maxLabel)
	case m.ocSendAll:
		maxBtn = theme.BtnFocused.Render(maxLabel)
	default:
		maxBtn = theme.BtnNormal.Render(maxLabel)
	}
	maxMarker := " "
	if maxActive {
		maxMarker = navActiveStyle.Render("▸")
	}
	lines = append(lines, maxMarker+" "+maxBtn)
	lines = append(lines, "")

	// ── Fee tier selector (step 3) ───────────────
	feeActive := isFocused && m.ocSendStep == 3
	feeLabelStyle := theme.Label
	if feeActive {
		feeLabelStyle = navActiveStyle
	}
	lines = append(lines,
		" "+feeLabelStyle.Render("Fee Rate:"))

	tierLine := " "
	fixedRates := []string{
		"1 sat/vB", "2 sat/vB", "3 sat/vB"}
	for i, label := range fixedRates {
		isSel := isFocused &&
			m.ocSendStep == 3 &&
			m.ocSelectedTier == i
		if isSel {
			tierLine += "▸ " +
				theme.BtnFocused.Render(label) +
				"  "
		} else {
			tierLine += "  " +
				theme.BtnNormal.Render(label) +
				"  "
		}
	}
	isCustomSel := isFocused &&
		m.ocSendStep == 3 &&
		m.ocSelectedTier == 3
	if isCustomSel {
		tierLine += "▸ " +
			theme.BtnFocused.Render("Custom")
	} else {
		tierLine += "  " +
			theme.BtnNormal.Render("Custom")
	}
	lines = append(lines, tierLine)

	// Custom fee input (step 4)
	if m.ocSelectedTier == 3 {
		lines = append(lines, "")
		custActive := isFocused && m.ocSendStep == 4
		custLabel := theme.Label
		custMarker := " "
		if custActive {
			custLabel = navActiveStyle
			custMarker = navActiveStyle.Render("▸")
		}
		lines = append(lines,
			" "+custLabel.Render("sat/vB:"))
		lines = append(lines,
			custMarker+" "+
				m.ocCustomFeeInput.View())
	}
	lines = append(lines, "")

	// ── Transaction preview diagram ──────────────
	sendAmt := parseSendAmount(
		m.ocSendAmtInput.Value())
	feeRate := getSendFeeRate(m)
	showPreview := sendAmt > 0

	var diagLines []string
	if showPreview {
		// Determine which outpoints to show as inputs
		diagOutpoints := m.utxoOutpoints
		if len(diagOutpoints) == 0 && m.ocSendAll &&
			len(m.utxos) > 0 {
			// No coin control + Max: show all UTXOs
			for _, u := range m.utxos {
				diagOutpoints = append(diagOutpoints,
					fmt.Sprintf("%s:%d",
						u.Txid, u.Vout))
			}
		}

		numInputs := max(len(diagOutpoints), 1)
		numOutputs := 2
		if m.ocSendAll {
			numOutputs = 1
		}
		estFee := estimateSimpleFee(
			numInputs, numOutputs, feeRate)

		dispAmt := formatSats(sendAmt)

		var changeStr string
		if !m.ocSendAll {
			if len(m.utxoSelected) > 0 {
				ch := m.utxoSelectedTotal -
					sendAmt - estFee
				if ch > 0 {
					changeStr = "~" +
						formatSats(ch)
				} else {
					changeStr = "~?"
				}
			} else {
				changeStr = "~?"
			}
		}

		feeStr := "~" + formatSats(estFee)
		destAddr := strings.TrimSpace(
			m.ocSendAddrInput.Value())
		diagInputs := m.buildDiagramInputs(
			diagOutpoints)
		diagLines = renderTxDiagram(
			diagInputs, destAddr, dispAmt,
			changeStr, feeStr, m.ocSendAll, w)
	}

	// Error
	var errLines []string
	if m.onChainSendError != "" {
		errLines = append(errLines, "")
		errLines = append(errLines,
			" "+theme.Warning.Render(
				m.onChainSendError))
	}

	// ── Bottom buttons (step 5) ──────────────────
	btnFocused := isFocused && m.ocSendStep == 5
	btnLine := renderButtons(
		[]string{"Clear", "Create Transaction"},
		m.ocSendBtnIdx, btnFocused, w)

	// ── Layout: form top, diagram centered in
	// remaining space, buttons pinned at bottom ───
	formH := len(lines)
	diagH := len(diagLines) + len(errLines)
	totalPad := h - formH - diagH - 1 // -1 for btn
	totalPad = max(totalPad, 2)

	// Split padding: half above diagram, half below
	padAbove := totalPad / 2
	padBelow := totalPad - padAbove
	padAbove = max(padAbove, 1)
	padBelow = max(padBelow, 0)

	for i := 0; i < padAbove; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, diagLines...)
	lines = append(lines, errLines...)
	for i := 0; i < padBelow; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, btnLine)

	return strings.Join(lines, "\n")
}

// ── Transaction diagram ──────────────────────────────────
//
// Renders a Sparrow-style transaction diagram with inputs
// on the left, "Transaction" centered, and outputs on
// the right. Uses lipgloss/tree for outputs. Inputs are
// shown with their label (from tx history) or truncated
// address + amount.
//
//   test (recv)  ─────╮              ╭── bc1q..f39m   1,479,949
//   003f8bce4d.. ─────┤              │
//   8c7c97e111.. ─────┤ Transaction  │
//   452a4c2b09.. ─────┤              ╰── fee               ~449
//   2a191e9700.. ─────╯

// txDiagramInput holds display info for a transaction
// input in the diagram.
type txDiagramInput struct {
	label string // display label (tx label, address, or txid)
	amt   string // formatted amount
}

func renderTxDiagram(
	inputs []txDiagramInput,
	destAddr string,
	destAmt string,
	changeAmt string,
	feeAmt string,
	sendAll bool,
	availW int,
) []string {
	// ── Fallback if no inputs ────────────────────
	if len(inputs) == 0 {
		inputs = []txDiagramInput{
			{label: "? inputs", amt: ""},
		}
	}

	// ── Build outputs tree using lipgloss/tree ───
	destLabel := destAddr
	if len(destLabel) > 12 {
		destLabel = destLabel[:6] + ".." +
			destLabel[len(destLabel)-4:]
	}
	if destLabel == "" {
		destLabel = "dest"
	}

	// Find widest output value for alignment
	outValueW := max(
		utf8.RuneCountInString(destAmt),
		utf8.RuneCountInString(feeAmt))
	if changeAmt != "" {
		outValueW = max(outValueW,
			utf8.RuneCountInString(changeAmt))
	}

	outTree := tree.New().
		Enumerator(tree.RoundedEnumerator).
		EnumeratorStyle(theme.Dim).
		ItemStyleFunc(func(
			children tree.Children, i int,
		) lipgloss.Style {
			if i == 0 {
				return theme.Value
			}
			return theme.Dim
		})

	outTree.Child(fmt.Sprintf("%-12s %*s",
		destLabel, outValueW, destAmt))
	if !sendAll && changeAmt != "" {
		outTree.Child(fmt.Sprintf("%-12s %*s",
			"change", outValueW, changeAmt))
	}
	outTree.Child(fmt.Sprintf("%-12s %*s",
		"fee", outValueW, feeAmt))

	outputsRendered := outTree.String()

	// ── Build inputs column ──────────────────────
	// Each input: "label ────╮" (no amounts — those
	// are visible in the UTXO table above)
	inLabelW := 8
	for _, inp := range inputs {
		inLabelW = max(inLabelW,
			utf8.RuneCountInString(inp.label))
	}
	// Cap label width to keep diagram within pane
	inLabelW = min(inLabelW, 12)

	inH := len(inputs)

	var inputLines []string
	for i, inp := range inputs {
		label := inp.label
		if utf8.RuneCountInString(label) > inLabelW {
			label = label[:inLabelW-2] + ".."
		}
		padded := fmt.Sprintf("%*s", inLabelW, label)

		// Connector
		var conn string
		switch {
		case inH == 1:
			conn = " ────"
		case i == 0:
			conn = " ───╮"
		case i == inH-1:
			conn = " ───╯"
		default:
			conn = " ───┤"
		}

		inputLines = append(inputLines,
			theme.Mono.Render(padded)+
				theme.Dim.Render(conn))
	}
	inputsRendered := strings.Join(
		inputLines, "\n")

	// ── Transaction label with connecting lines ──
	txStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15"))
	txLabel := theme.Dim.Render("── ") +
		txStyle.Render("Transaction") +
		theme.Dim.Render(" ──")

	// ── Join horizontally, centered vertically ───
	diagram := lipgloss.JoinHorizontal(
		lipgloss.Center,
		inputsRendered,
		txLabel,
		outputsRendered,
	)

	// ── Center within available width ────────────
	diagLines := strings.Split(diagram, "\n")
	maxDiagW := 0
	for _, line := range diagLines {
		maxDiagW = max(maxDiagW,
			lipgloss.Width(line))
	}
	leftPad := (availW - maxDiagW) / 2
	leftPad = max(leftPad, 1)
	padStr := strings.Repeat(" ", leftPad)

	var result []string
	for _, line := range diagLines {
		result = append(result, padStr+line)
	}

	return result
}

// buildDiagramInputs creates txDiagramInput entries from
// outpoints, cross-referencing UTXOs and tx history for
// labels and amounts.
func (m Model) buildDiagramInputs(
	outpoints []string,
) []txDiagramInput {
	var inputs []txDiagramInput
	for _, op := range outpoints {
		inp := txDiagramInput{label: op}

		// Parse txid from outpoint
		txid := op
		if idx := strings.Index(txid, ":"); idx > 0 {
			txid = txid[:idx]
		}

		// Look up UTXO for amount and address
		for _, u := range m.utxos {
			uOP := fmt.Sprintf("%s:%d", u.Txid, u.Vout)
			if uOP == op {
				inp.amt = formatSats(u.AmountSats)
				// Use truncated address as label
				if len(u.Address) > 14 {
					inp.label = u.Address[:8] + ".." +
						u.Address[len(u.Address)-4:]
				} else {
					inp.label = u.Address
				}
				break
			}
		}

		// Check tx history for a user-set label
		for _, tx := range m.onChainTxs {
			if tx.Txid == txid && tx.Label != "" {
				inp.label = tx.Label
				break
			}
		}

		// Fallback: truncated txid
		if inp.label == op {
			if len(txid) > 12 {
				inp.label = txid[:10] + ".."
			} else {
				inp.label = txid
			}
		}

		inputs = append(inputs, inp)
	}
	return inputs
}

// ── Fee/amount parse helpers ─────────────────────────────

func parseSendAmount(val string) int64 {
	val = strings.ReplaceAll(val, ",", "")
	val = strings.TrimSpace(val)
	if val == "" {
		return 0
	}
	var n int64
	for _, c := range val {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func getSendFeeRate(m Model) int64 {
	if m.ocSelectedTier < 3 {
		return int64(m.ocSelectedTier + 1)
	}
	val := strings.TrimSpace(
		m.ocCustomFeeInput.Value())
	if val == "" {
		return 1
	}
	var n int64
	for _, c := range val {
		if c < '0' || c > '9' {
			return 1
		}
		n = n*10 + int64(c-'0')
	}
	return max(n, 1)
}

func estimateSimpleFee(
	numInputs, numOutputs int, satPerVB int64,
) int64 {
	vbytes := int64(10 + numInputs*68 + numOutputs*31)
	return vbytes * satPerVB
}

// ── On-Chain send confirm pane ──────────────────────────

func (m Model) onChainSendConfirmPane(
	w, h int,
) string {
	isFocused := m.contentFocused && !m.tabFocused

	var lines []string
	lines = append(lines, "")
	lines = append(lines, centerPad(
		theme.Warning.Render("Confirm On-Chain Send"),
		w))
	lines = append(lines, "")

	addr := m.ocSendAddrVal
	if len(addr) > w-14 {
		addr = addr[:w-17] + "..."
	}
	lines = append(lines,
		" "+theme.Label.Render("To:       ")+
			theme.Mono.Render(addr))
	switch {
	case m.ocSendAll && m.ocSendAmtVal > 0:
		// Max was pressed — show the computed amount
		lines = append(lines,
			" "+theme.Label.Render("Amount:   ")+
				theme.Value.Render(
					formatSats(m.ocSendAmtVal)+
						" sats (max)"))
	case m.ocSendAll:
		lines = append(lines,
			" "+theme.Label.Render("Amount:   ")+
				theme.Value.Render("Send All"))
	default:
		lines = append(lines,
			" "+theme.Label.Render("Amount:   ")+
				theme.Value.Render(
					formatSats(m.ocSendAmtVal)+
						" sats"))
	}
	if len(m.utxoOutpoints) > 0 {
		lines = append(lines,
			" "+theme.Label.Render("Inputs:   ")+
				theme.Value.Render(
					fmt.Sprintf("%d selected UTXOs",
						len(m.utxoSelected))))
	}
	lines = append(lines,
		" "+theme.Label.Render("Fee Rate: ")+
			theme.Value.Render(
				fmt.Sprintf("%d sat/vB",
					m.ocSendFeeRate)))
	if m.ocConfirmFee > 0 {
		lines = append(lines,
			" "+theme.Label.Render("Est. Fee: ")+
				theme.Value.Render(
					formatSats(m.ocConfirmFee)+
						" sats"))
		if !m.ocSendAll && m.ocSendAmtVal > 0 {
			total := m.ocSendAmtVal + m.ocConfirmFee
			lines = append(lines,
				" "+theme.Label.Render("Total:    ")+
					theme.Value.Render(
						formatSats(total)+" sats"))
		}
	}
	lines = append(lines, "")

	// ── Diagram with real fee numbers ────────────
	// Always use the computed amount from the input,
	// even when ocSendAll is true (Max fills actual sats)
	var destAmt string
	if m.ocSendAmtVal > 0 {
		destAmt = formatSats(m.ocSendAmtVal)
	} else if m.ocSendAll {
		// Fallback: show balance if amtVal wasn't set
		if len(m.utxoSelected) > 0 {
			destAmt = formatSats(m.utxoSelectedTotal)
		} else {
			destAmt = formatSats(
				parseBalance(m.status.lndBalance))
		}
	} else {
		destAmt = "0"
	}

	var changeStr string
	if !m.ocSendAll && m.ocConfirmFee > 0 &&
		len(m.utxoSelected) > 0 {
		ch := m.utxoSelectedTotal -
			m.ocSendAmtVal - m.ocConfirmFee
		if ch > 0 {
			changeStr = formatSats(ch)
		}
	}

	var feeStr string
	if m.ocConfirmFee > 0 {
		feeStr = formatSats(m.ocConfirmFee)
	} else {
		feeRate := getSendFeeRate(m)
		numInputs := max(len(m.utxoOutpoints), 1)
		numOutputs := 2
		if m.ocSendAll {
			numOutputs = 1
		}
		feeStr = "~" + formatSats(
			estimateSimpleFee(
				numInputs, numOutputs, feeRate))
	}

	// Show all UTXOs as inputs when sendAll
	diagOutpoints := m.utxoOutpoints
	if len(diagOutpoints) == 0 && m.ocSendAll &&
		len(m.utxos) > 0 {
		for _, u := range m.utxos {
			diagOutpoints = append(diagOutpoints,
				fmt.Sprintf("%s:%d",
					u.Txid, u.Vout))
		}
	}

	diagInputs := m.buildDiagramInputs(diagOutpoints)
	diagLines := renderTxDiagram(
		diagInputs, m.ocSendAddrVal, destAmt,
		changeStr, feeStr, m.ocSendAll, w)
	lines = append(lines, diagLines...)

	// Warning
	lines = append(lines, "")
	if m.ocSendAll {
		lines = append(lines,
			" "+theme.Warning.Render(
				"Send entire balance?"))
	} else {
		lines = append(lines,
			" "+theme.Warning.Render(
				"Send "+formatSats(m.ocSendAmtVal)+
					" sats?"))
	}

	// Error
	if m.onChainSendError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(
				m.onChainSendError))
	}

	// ── Bottom buttons, pinned ───────────────────
	btnFocused := isFocused
	btnLine := renderButtons(
		[]string{"Go Back", "Confirm & Broadcast"},
		m.ocConfirmBtnIdx, btnFocused, w)

	contentH := len(lines)
	padH := max(h-contentH-1, 1)
	for i := 0; i < padH; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, btnLine)

	return strings.Join(lines, "\n")
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
			maxOP := w - 8 - len(ownership)
			maxOP = max(maxOP, 16)
			outpoint := inp.Outpoint
			if len(outpoint) > maxOP {
				outpoint = outpoint[:maxOP-3] +
					"..."
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
			fixedW := 13 + len(amtStr) +
				len(labelStr)
			addrMax := w - fixedW
			addrMax = max(addrMax, 12)
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
				theme.Value.Render(
					amtStr+" sats"),
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
		p.blank()
		p.line(renderButtons(
			[]string{"Edit Label"},
			0, isFocused && m.contentFocus == 1, w))
	}

	return p.render()
}

func (m Model) utxoTxLabel(txid string) string {
	for _, tx := range m.onChainTxs {
		if tx.Txid == txid {
			return tx.Label
		}
	}
	return ""
}
