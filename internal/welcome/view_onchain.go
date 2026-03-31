package welcome

import (
	"fmt"
	"strings"
	"time"
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
	utxoCount := len(m.utxos)
	headerLines = append(headerLines,
		" "+theme.Label.Render("Balance:  ")+
			theme.Value.Render(
				formatSats(parseBalance(onchain))+
					" sats")+
			theme.Dim.Render(
				fmt.Sprintf("  (%d UTXOs)",
					utxoCount)))
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
			isFocused && m.contentFocus() == 0,
			w))
	headerLines = append(headerLines, "")

	header := strings.Join(headerLines, "\n")
	headerH := len(headerLines)

	// ── Shared styles ────────────────────────────
	hdrStyle := theme.TableHeader
	sepStyle := theme.TableDim

	// ── UTXO table header ───────────────────────
	uDateW := 12
	uLabelW := 15 // +1 ensures gap before Address
	uAddrW := 18  // first7..last7 = 16 chars + padding
	uValW := w - uDateW - uLabelW - uAddrW - 5
	uValW = max(uValW, 12)

	var utxoHeaderLines []string
	utxoHeaderLines = append(utxoHeaderLines,
		centerPad(
			theme.Header.Render("UTXOs"), w))

	utxoHdr := " " +
		hdrStyle.Render(pad("Date", uDateW)) +
		hdrStyle.Render(pad("Label", uLabelW)) +
		hdrStyle.Render(pad("Address", uAddrW)) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", uValW, "Value"))
	utxoHeaderLines = append(utxoHeaderLines, utxoHdr)
	utxoHeaderLines = append(utxoHeaderLines,
		" "+sepStyle.Render(
			strings.Repeat("─", w-2)))

	utxoHeader := strings.Join(utxoHeaderLines, "\n")
	utxoHeaderH := len(utxoHeaderLines)

	// ── Scrollable UTXO rows ────────────────────
	var utxoMidLines []string

	if len(m.utxos) == 0 {
		utxoMidLines = append(utxoMidLines,
			" "+theme.Dim.Render("No UTXOs found."))
	} else {
		for i, u := range m.utxos {
			isSelected := isFocused &&
				m.contentFocus() == 1 &&
				m.utxoCursor == i
			isChecked := m.utxoSelected[i]

			// Date from tx lookup
			dateStr := m.utxoDate(u.Txid)
			if u.Confirmations == 0 {
				dateStr = "unconfirmed"
			}
			dateStr = pad(dateStr, uDateW)

			// Label from tx lookup
			uLabel := m.utxoTxLabel(u.Txid)
			maxLbl := uLabelW - 2
			if isSelected && !m.utxoLabelEditing {
				// Reserve 3 chars for " ✎ "
				maxLbl = uLabelW - 4
			}
			if len(uLabel) > maxLbl {
				uLabel = uLabel[:maxLbl-2] + ".."
			}

			// Build label cell with pencil for
			// selected row
			var labelCell string
			if isSelected && !m.utxoLabelEditing {
				pencilStyle := theme.Dim
				labelStyle := lipgloss.NewStyle()
				if m.utxoPencilFocused {
					pencilStyle = theme.NavActive
					labelStyle = theme.NavActive
				}
				labelCell = labelStyle.Render(
					pad(uLabel, maxLbl)) +
					" " +
					pencilStyle.Render("✎")
				// Pad to full column width
				cellW := maxLbl + 2
				if cellW < uLabelW {
					labelCell += strings.Repeat(
						" ", uLabelW-cellW)
				}
			} else {
				labelCell = pad(uLabel, uLabelW)
			}

			// Address: first7..last7
			uAddr := u.Address
			if len(uAddr) > 16 {
				uAddr = uAddr[:7] + ".." +
					uAddr[len(uAddr)-7:]
			}
			uAddrStr := pad(uAddr, uAddrW)

			// Value
			uValStr := fmt.Sprintf("%*s",
				uValW, formatSats(u.AmountSats))

			marker := " "
			if isChecked {
				marker = "✓"
			}
			if isSelected && !isChecked {
				marker = "▸"
			}

			selStyle := lipgloss.NewStyle().
				Foreground(
					theme.ColorAccent).
				Bold(true)

			switch {
			case isSelected && m.utxoPencilFocused &&
				!m.utxoLabelEditing:
				// Pencil focused: only label+pencil
				// are yellow, rest stays dim
				utxoMidLines = append(utxoMidLines,
					marker+
						theme.Dim.Render(dateStr)+
						labelCell+
						theme.Dim.Render(uAddrStr)+
						theme.Value.Render(uValStr))
			case isSelected:
				utxoMidLines = append(utxoMidLines,
					marker+
						selStyle.Render(dateStr)+
						labelCell+
						selStyle.Render(uAddrStr)+
						selStyle.Render(uValStr))
			case isChecked:
				checkStyle := lipgloss.NewStyle().
					Foreground(
						theme.ColorCheck)
				utxoMidLines = append(utxoMidLines,
					marker+
						checkStyle.Render(dateStr)+
						checkStyle.Render(labelCell)+
						checkStyle.Render(uAddrStr)+
						checkStyle.Render(uValStr))
			default:
				utxoMidLines = append(utxoMidLines,
					marker+
						theme.Dim.Render(dateStr)+
						theme.Value.Render(labelCell)+
						theme.Dim.Render(uAddrStr)+
						theme.Value.Render(uValStr))
			}

			// Label edit popup
			if m.utxoLabelEditing &&
				m.utxoCursor == i {
				popLines := m.renderLabelPopup(w)
				utxoMidLines = append(utxoMidLines,
					popLines...)
			}
		}
	}

	utxoMidContent := strings.Join(utxoMidLines, "\n")

	// ── Transaction table header ────────────────
	tDateW := 12
	tLabelW := 14
	tConfW := 3 // conf indicator column (no header)
	tValW := 14
	tBalW := w - tDateW - tLabelW - tConfW - tValW - 5
	tBalW = max(tBalW, 10)

	confStyle := lipgloss.NewStyle().
		Foreground(theme.ColorLabel)

	var txHeaderLines []string
	txHeaderLines = append(txHeaderLines,
		centerPad(
			theme.Header.Render("Transactions"), w))

	txHdr := " " +
		hdrStyle.Render(pad("Date", tDateW)) +
		hdrStyle.Render(pad("Label", tLabelW)) +
		hdrStyle.Render(pad("", tConfW)) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", tValW, "Value")) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", tBalW, "Balance"))
	txHeaderLines = append(txHeaderLines, txHdr)
	txHeaderLines = append(txHeaderLines,
		" "+sepStyle.Render(
			strings.Repeat("─", w-2)))

	txHeader := strings.Join(txHeaderLines, "\n")
	txHeaderH := len(txHeaderLines)

	// ── Scrollable tx rows ──────────────────────
	var txMidLines []string

	// Compute running balances
	txBalances := m.computeTxBalances()

	if len(m.onChainTxs) == 0 {
		txMidLines = append(txMidLines,
			" "+theme.Dim.Render(
				"No on-chain transactions."))
	} else {
		for i, tx := range m.onChainTxs {
			isSelected := isFocused &&
				m.contentFocus() == 2 &&
				m.onChainTxCursor == i

			// Date
			date := "unconfirmed"
			if tx.IsAnchorSweep {
				date = formatDateShort(tx.Timestamp)
			} else if tx.Confirmations > 0 {
				date = formatDateShort(tx.Timestamp)
			}
			dateStr := pad(date, tDateW)

			// Label
			txLabel := tx.Label
			if len(txLabel) > tLabelW-1 {
				txLabel = txLabel[:tLabelW-2] + ".."
			}
			labelStr := pad(txLabel, tLabelW)

			// Conf indicator (always gray)
			confIcon := confIndicator(tx.Confirmations)
			if tx.IsAnchorSweep {
				confIcon = "–"
			}
			confCell := confStyle.Render(
				" " + pad(confIcon, tConfW-1))

			// Value (pure number)
			valNum := formatSats(tx.Amount)
			if tx.Amount >= 0 {
				valNum = "+" + valNum
			}
			valStr := fmt.Sprintf("%*s",
				tValW, valNum)

			// Balance
			bal := int64(0)
			if i < len(txBalances) {
				bal = txBalances[i]
			}
			balStr := fmt.Sprintf("%*s",
				tBalW, formatSats(bal))

			marker := " "
			if isSelected {
				marker = "▸"
				selStyle := lipgloss.NewStyle().
					Foreground(
						theme.ColorAccent).
					Bold(true)
				txMidLines = append(txMidLines,
					marker+
						selStyle.Render(dateStr)+
						selStyle.Render(labelStr)+
						confCell+
						selStyle.Render(valStr)+
						selStyle.Render(balStr))
			} else {
				amtStyle := lipgloss.NewStyle().
					Foreground(
						theme.ColorPrimary)
				if tx.Amount < 0 {
					amtStyle = lipgloss.NewStyle().
						Foreground(
							theme.ColorDanger)
				}
				txMidLines = append(txMidLines,
					marker+
						theme.Dim.Render(dateStr)+
						theme.Value.Render(labelStr)+
						confCell+
						amtStyle.Render(valStr)+
						theme.Dim.Render(balStr))
			}
		}
	}

	txMidContent := strings.Join(txMidLines, "\n")

	// ── Size viewports ──────────────────────────
	fixedH := headerH + txHeaderH + utxoHeaderH
	remainH := max(h-fixedH, 2)

	txVPH := remainH / 2
	utxoVPH := remainH - txVPH
	txVPH = max(txVPH, 1)
	utxoVPH = max(utxoVPH, 1)

	// Compute cursor line accounting for popup
	utxoCursorLine := m.utxoCursor
	if m.utxoLabelEditing {
		// Popup adds ~4 lines below the selected row
		// Scroll to show both the row and the popup
		utxoCursorLine = m.utxoCursor + 4
	}

	utxoVPRendered := renderViewport(
		utxoMidContent, w, utxoVPH,
		utxoCursorLine,
		len(utxoMidLines),
		len(m.utxos) > 0 &&
			m.contentFocus() == 1)

	txVPRendered := renderViewport(
		txMidContent, w, txVPH,
		m.onChainTxCursor,
		len(txMidLines),
		len(m.onChainTxs) > 0 &&
			m.contentFocus() == 2)

	return header + "\n" +
		utxoHeader + "\n" +
		utxoVPRendered + "\n" +
		txHeader + "\n" +
		txVPRendered
}

// renderLabelPopup renders the label edit popup below
// the selected UTXO row.
func (m Model) renderLabelPopup(w int) []string {
	isFocused := m.contentFocused && !m.tabFocused
	boxW := w - 4
	if boxW < 30 {
		boxW = 30
	}

	border := lipgloss.NewStyle().
		Foreground(theme.ColorGrayed)

	var lines []string
	lines = append(lines,
		"  "+border.Render(
			"┌"+strings.Repeat("─", boxW)+"┐"))

	// Label input line
	lblActive := isFocused && !m.utxoLabelOnBtn
	lblStyle := theme.Label
	marker := " "
	if lblActive {
		lblStyle = theme.NavActive
		marker = theme.NavActive.Render("▸")
	}
	inputView := m.utxoLabelInput.View()
	inputW := lipgloss.Width(inputView)
	lblPrefix := marker + lblStyle.Render("Label: ")
	prefixW := lipgloss.Width(lblPrefix)
	padR := boxW - prefixW - inputW
	if padR < 0 {
		padR = 0
	}
	lines = append(lines,
		"  "+border.Render("│")+
			lblPrefix+inputView+
			strings.Repeat(" ", padR)+
			border.Render("│"))

	// Save + Cancel buttons
	btnStr := renderButtons(
		[]string{"Save", "Cancel"},
		m.utxoLabelBtnIdx,
		isFocused && m.utxoLabelOnBtn, boxW)
	btnW := lipgloss.Width(btnStr)
	btnPad := boxW - btnW
	if btnPad < 0 {
		btnPad = 0
	}
	lines = append(lines,
		"  "+border.Render("│")+
			btnStr+
			strings.Repeat(" ", btnPad)+
			border.Render("│"))

	lines = append(lines,
		"  "+border.Render(
			"└"+strings.Repeat("─", boxW)+"┘"))

	return lines
}

// openLabelPopup opens the label edit popup for the
// current UTXO.
func (m *Model) openLabelPopup() {
	if m.utxoCursor >= len(m.utxos) {
		return
	}
	txLabel := m.utxoTxLabel(
		m.utxos[m.utxoCursor].Txid)
	contentW := tuiWidth - 2 - m.nav.Width - 1
	fieldW := contentW - 16
	if fieldW < 20 {
		fieldW = 20
	}
	m.utxoLabelInput = newDetailField(txLabel, fieldW)
	m.utxoLabelInput.Placeholder = "enter label"
	m.utxoLabelInput.CharLimit = 64
	m.utxoLabelInput.Focus()
	m.utxoLabelEditing = true
	m.utxoLabelOnBtn = false
	m.utxoLabelBtnIdx = 0
	m.utxoPencilFocused = false
}

// closeLabelPopup closes the label edit popup.
func (m *Model) closeLabelPopup() {
	m.utxoLabelEditing = false
	m.utxoLabelOnBtn = false
	m.utxoLabelBtnIdx = 0
	m.utxoLabelInput.Blur()
}

// ── View-only UTXO detail pane ──────────────────────────

func (m Model) utxoDetailPane(w int) string {
	if m.utxoCursor >= len(m.utxos) {
		return theme.Dim.Render(" UTXO not found")
	}
	u := m.utxos[m.utxoCursor]

	p := newPane(w)
	p.title(theme.Header, "UTXO Detail")

	p.field("Amount:    ",
		formatSats(u.AmountSats)+" sats")

	confStr := fmt.Sprintf("%d", u.Confirmations)
	if u.Confirmations == 0 {
		confStr = "unconfirmed"
	}
	p.field("Confs:     ", confStr)

	dateStr := m.utxoDate(u.Txid)
	if u.Confirmations == 0 {
		dateStr = "unconfirmed"
	}
	p.field("Date:      ", dateStr)
	p.blank()

	// Outpoint on multiple lines for full visibility
	p.labelLine("Outpoint:")
	outpoint := fmt.Sprintf("%s:%d", u.Txid, u.Vout)
	p.monoWrap(outpoint)
	p.blank()

	// Address on multiple lines if needed
	p.labelLine("Address:")
	p.monoWrap(u.Address)
	p.blank()

	txLabel := m.utxoTxLabel(u.Txid)
	if txLabel != "" {
		p.field("Label:     ", txLabel)
	} else {
		p.field("Label:     ",
			theme.Dim.Render("none"))
	}

	return p.render()
}

// ── Helpers ─────────────────────────────────────────────

// utxoDate returns the YYYY-MM-DD date for a UTXO by
// looking up its transaction timestamp.
func (m Model) utxoDate(txid string) string {
	for _, tx := range m.onChainTxs {
		if tx.Txid == txid {
			return formatDateShort(tx.Timestamp)
		}
	}
	return "—"
}

// formatDateShort returns YYYY-MM-DD for a unix timestamp.
func formatDateShort(unix int64) string {
	if unix == 0 {
		return "—"
	}
	return time.Unix(unix, 0).Format("2006-01-02")
}

// confIndicator returns a single-char confirmation
// progress indicator for the transaction table.
func confIndicator(confs int32) string {
	switch {
	case confs >= 100:
		return " "
	case confs == 0:
		return "○"
	case confs <= 2:
		return "◔"
	case confs <= 4:
		return "◑"
	case confs <= 6:
		return "◕"
	default:
		return "●"
	}
}

// computeTxBalances returns a running balance for each
// transaction. Transactions are sorted newest-first, so
// the first entry has the current balance.
func (m Model) computeTxBalances() []int64 {
	if len(m.onChainTxs) == 0 {
		return nil
	}
	bal := parseBalance("0")
	if m.status != nil && m.status.lndBalance != "" {
		bal = parseBalance(m.status.lndBalance)
	}

	balances := make([]int64, len(m.onChainTxs))
	for i := range m.onChainTxs {
		balances[i] = bal
		bal -= m.onChainTxs[i].Amount
	}
	return balances
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
	p.monoWrap(m.ocRecvAddress)
	p.blank()
	p.dim("Send Bitcoin to this address.")
	p.dim("Funds appear after 1 confirmation.")
	p.blank()

	btnFocused := m.contentFocused && !m.tabFocused

	qr := renderQRCode(m.ocRecvAddress)
	if qr != "" {
		for _, line := range strings.Split(
			qr, "\n") {
			lineW := lipgloss.Width(line)
			pad := (w - lineW) / 2
			if pad < 0 {
				pad = 0
			}
			p.line(strings.Repeat(" ", pad) + line)
		}
	}
	p.blank()

	p.buttons(
		[]string{"New Address"},
		0, btnFocused)

	p.appendError(m.ocRecvError)

	return p.render()
}

// ── On-Chain single-screen send pane ─────────────────────
//
// Steps:
//
//	0 = Address input
//	1 = Amount input (right arrow → Max button)
//	2 = Label input
//	3 = Fee rate input (sat/vB)
//	4 = Buttons (Clear / Create Transaction)
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
		addrLabel = theme.NavActive
		addrMarker = theme.NavActive.Render("▸")
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
		amtLabel = theme.NavActive
		amtMarker = theme.NavActive.Render("▸")
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
		clearStyle := theme.BtnNormal
		if amtActive && m.ocMaxFocused {
			clearStyle = theme.BtnFocused
		}
		lines = append(lines,
			"  "+theme.Value.Render(amtVal+" sats")+
				"  "+clearStyle.Render("Clear Max"))
	} else {
		maxStyle := theme.BtnNormal
		if amtActive && m.ocMaxFocused {
			maxStyle = theme.BtnFocused
		}
		maxLabel := "Max"
		if len(m.utxoSelected) > 0 {
			maxLabel = fmt.Sprintf("Max (%s)",
				formatSats(m.utxoSelectedTotal))
		}
		lines = append(lines,
			amtMarker+" "+m.ocSendAmtInput.View()+
				"  "+maxStyle.Render(maxLabel))
	}
	lines = append(lines, "")

	// ── Label input (step 2) ────────────────────
	lblActive := isFocused && m.ocSendStep == 2
	lblLabel := theme.Label
	lblMarker := " "
	if lblActive {
		lblLabel = theme.NavActive
		lblMarker = theme.NavActive.Render("▸")
	}
	lines = append(lines,
		" "+lblLabel.Render("Label:"))
	lines = append(lines,
		lblMarker+" "+m.ocSendLabelInput.View())
	lines = append(lines, "")

	// ── Fee rate input (step 4) ──────────────────
	feeActive := isFocused && m.ocSendStep == 3
	feeLabelStyle := theme.Label
	feeMarker := " "
	if feeActive {
		feeLabelStyle = theme.NavActive
		feeMarker = theme.NavActive.Render("▸")
	}
	lines = append(lines,
		" "+feeLabelStyle.Render("Fee Rate (sat/vB):"))
	lines = append(lines,
		feeMarker+" "+m.ocCustomFeeInput.View())

	// Friendly fee reference hints
	hints := formatFeeHints(m.sendFeeTiers)
	if hints != "" {
		lines = append(lines,
			"  "+theme.Dim.Render(hints))
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
		lineW := w - 4
		if lineW < 16 {
			lineW = 16
		}
		errText := m.onChainSendError
		for len(errText) > 0 {
			end := lineW
			if end > len(errText) {
				end = len(errText)
			}
			errLines = append(errLines,
				" "+theme.Warning.Render(
					errText[:end]))
			errText = errText[end:]
		}
	}

	// ── Bottom buttons (step 5) ──────────────────
	btnFocused := isFocused && m.ocSendStep == 4
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
		Foreground(theme.ColorPrimary)
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
	lineW := w - 14
	if len(addr) <= lineW {
		lines = append(lines,
			" "+theme.Label.Render("To:       ")+
				theme.Mono.Render(addr))
	} else {
		lines = append(lines,
			" "+theme.Label.Render("To:       ")+
				theme.Mono.Render(addr[:lineW]))
		lines = append(lines,
			"           "+
				theme.Mono.Render(addr[lineW:]))
	}
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
	if m.ocSendLabelVal != "" {
		lines = append(lines,
			" "+theme.Label.Render("Label:    ")+
				theme.Value.Render(m.ocSendLabelVal))
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
		lineW := w - 4
		if lineW < 16 {
			lineW = 16
		}
		errText := m.onChainSendError
		for len(errText) > 0 {
			end := lineW
			if end > len(errText) {
				end = len(errText)
			}
			lines = append(lines,
				" "+theme.Warning.Render(
					errText[:end]))
			errText = errText[end:]
		}
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
		p.warnWrap(m.onChainSendError)
	} else {
		p.title(theme.Success,
			"Transaction Broadcast")
		if m.onChainSendTxid != "" {
			p.labelLine("TX ID:")
			p.monoWrap(m.onChainSendTxid)
		}
	}

	return p.render()
}

// ── On-Chain tx detail ───────────────────────────────────

func (m Model) onChainTxDetailPane(
	tx lndrpc.OnChainTx, w int,
) string {
	p := newPane(w)

	switch {
	case tx.IsAnchorSweep:
		// No title — explanation text is the header
	case tx.TxType == "channel_open":
		p.title(theme.Header, "Channel Open")
	case tx.TxType == "channel_close":
		p.title(theme.Warning, "Channel Close")
	case tx.TxType == "send":
		p.title(theme.Warning, "On-Chain Send")
	default:
		p.title(theme.Success, "On-Chain Receive")
	}

	if tx.IsAnchorSweep {
		p.dim("330-sat anchor from force close.")
		p.dim("Sweep fee exceeded value.")
		p.dim("No funds lost -- this is normal.")
		p.blank()
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
		if tx.IsAnchorSweep {
			confStr = "abandoned"
		} else {
			confStr = "unconfirmed"
		}
	}
	p.field("Confs:   ", confStr)

	// Pending force close: show blocks remaining
	if tx.TxType == "channel_close" &&
		tx.Confirmations > 0 &&
		m.status != nil {
		for _, fc := range m.status.
			pendingForceCloseChannels {
			if fc.ClosingTxid == tx.Txid &&
				fc.BlocksRemaining > 0 {
				p.field("Locked:  ",
					fmt.Sprintf(
						"~%d blocks remaining",
						fc.BlocksRemaining))
				break
			}
		}
	}

	if tx.BlockHeight > 0 {
		p.field("Block:   ",
			fmt.Sprintf("%d", tx.BlockHeight))
	}
	p.field("Date:    ",
		formatTimestampFull(tx.Timestamp))

	p.blank()
	p.labelLine("TX ID:")
	p.monoWrap(tx.Txid)

	if len(tx.Inputs) > 0 {
		p.blank()
		p.labelLine("Inputs")
		// prefix " └── " = 5 chars
		maxW := max(w-5, 16)
		for i, inp := range tx.Inputs {
			isLast := i == len(tx.Inputs)-1
			connector := "├──"
			if isLast {
				connector = "└──"
			}
			cont := "│  "
			if isLast {
				cont = "   "
			}
			ownership := ""
			if inp.IsOurs {
				ownership = " (ours)"
			}
			outpoint := inp.Outpoint
			if len(outpoint)+len(ownership) <= maxW {
				p.line(fmt.Sprintf(" %s %s%s",
					connector,
					theme.Mono.Render(outpoint),
					theme.Dim.Render(ownership)))
			} else {
				// Wrap outpoint across lines
				rem := outpoint
				first := true
				for len(rem) > 0 {
					pfx := " " + cont + " "
					if first {
						pfx = " " + connector + " "
						first = false
					}
					end := maxW
					if end > len(rem) {
						end = len(rem)
					}
					p.line(pfx +
						theme.Mono.Render(rem[:end]))
					rem = rem[end:]
				}
				if ownership != "" {
					p.line(" " + cont + " " +
						theme.Dim.Render(ownership))
				}
			}
			if !isLast {
				p.line(" │")
			}
		}
	}

	if len(tx.Outputs) > 0 {
		p.blank()
		p.labelLine("Outputs")
		// prefix " └── " = 5 chars
		maxW := max(w-5, 16)
		for i, out := range tx.Outputs {
			amtStr := formatSats(out.Amount)
			if out.Amount == 0 {
				amtStr = "—"
			}
			labelStr := ""
			if out.Label != "" {
				labelStr = " (" + out.Label + ")"
			}
			isLast := i == len(tx.Outputs)-1
			connector := "├──"
			if isLast {
				connector = "└──"
			}
			cont := "│  "
			if isLast {
				cont = "   "
			}
			addrStyle := theme.Mono
			if out.Label == "destination" ||
				out.Label == "channel" {
				addrStyle = theme.Value
			}
			addr := out.Address
			valLine := amtStr + " sats" + labelStr
			// Try single line: addr + value
			if len(addr)+2+len(valLine) <= maxW {
				p.line(fmt.Sprintf(" %s %s%s",
					connector,
					addrStyle.Render(addr),
					theme.Value.Render("  "+
						amtStr+" sats")+
						theme.Dim.Render(labelStr)))
			} else {
				// Wrap address across lines, then
				// value on continuation
				rem := addr
				first := true
				for len(rem) > 0 {
					pfx := " " + cont + " "
					if first {
						pfx = " " + connector + " "
						first = false
					}
					end := maxW
					if end > len(rem) {
						end = len(rem)
					}
					p.line(pfx +
						addrStyle.Render(rem[:end]))
					rem = rem[end:]
				}
				p.line(fmt.Sprintf(" %s %s%s",
					cont,
					theme.Value.Render(
						amtStr+" sats"),
					theme.Dim.Render(labelStr)))
			}
			if !isLast {
				p.line(" │")
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

func (m Model) utxoTxLabel(txid string) string {
	for _, tx := range m.onChainTxs {
		if tx.Txid == txid {
			return tx.Label
		}
	}
	return ""
}
