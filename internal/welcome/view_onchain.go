package welcome

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

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

// ── UTXO detail pane ────────────────────────────────────

func (m Model) utxoTxLabel(txid string) string {
	for _, tx := range m.onChainTxs {
		if tx.Txid == txid {
			return tx.Label
		}
	}
	return ""
}
