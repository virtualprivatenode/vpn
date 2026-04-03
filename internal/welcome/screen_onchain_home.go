package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── OnChainHomeScreen ─────────────────────────────────
// Section home for On-Chain. Three focus zones: buttons
// (Receive, Send/Send Selected), UTXO table with pencil
// edit and coin control, and transaction table. Reads
// live status through ctx.Status pointer. UTXOs and
// transactions arrive via HandleMsg. Coin control
// selection is shared with OnChainSendScreen through
// the *OnChainContext pointer.

const (
	ocHomeZoneButtons = 0
	ocHomeZoneUtxos   = 1
	ocHomeZoneTxs     = 2
)

type OnChainHomeScreen struct {
	ctx   *ScreenContext
	ocCtx *OnChainContext

	btnIdx    int // 0=Receive, 1=Send
	focusZone int // 0=buttons, 1=utxo table, 2=tx table

	utxoCursor    int
	txCursor      int
	pencilFocused bool

	// Label popup state
	labelEditing bool
	labelInput   textinput.Model
	labelOnBtn   bool // true when on Save/Cancel buttons
	labelBtnIdx  int  // 0=Save, 1=Cancel
}

func NewOnChainHomeScreen(
	ctx *ScreenContext,
	ocCtx *OnChainContext,
) *OnChainHomeScreen {
	return &OnChainHomeScreen{
		ctx:   ctx,
		ocCtx: ocCtx,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *OnChainHomeScreen) Init() tea.Cmd {
	return nil
}

func (s *OnChainHomeScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	// Label popup intercepts ALL keys
	if s.labelEditing {
		return s.handleLabelPopupKey(keyStr, msg)
	}

	s.clampCursors()

	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		return s.handleLeft()
	case "right":
		return s.handleRight()
	case "up":
		return s.handleUp()
	case "down":
		return s.handleDown()
	case "tab":
		return s.handleTab()
	case "shift+tab":
		return s.handleShiftTab()
	case "backspace":
		return s, emitFocusSidebar
	case "space":
		if s.focusZone == ocHomeZoneUtxos &&
			s.utxoCursor < len(s.ocCtx.Utxos) {
			s.toggleSelection(s.utxoCursor)
		}
		return s, nil
	case "enter":
		return s.handleEnter()
	}
	return s, nil
}

func (s *OnChainHomeScreen) handleLeft() (
	Screen, tea.Cmd,
) {
	switch s.focusZone {
	case ocHomeZoneUtxos:
		if s.pencilFocused {
			s.pencilFocused = false
			return s, nil
		}
	case ocHomeZoneButtons:
		if s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
	}
	return s, emitFocusSidebar
}

func (s *OnChainHomeScreen) handleRight() (
	Screen, tea.Cmd,
) {
	switch s.focusZone {
	case ocHomeZoneButtons:
		if s.btnIdx < 1 {
			s.btnIdx++
		}
	case ocHomeZoneUtxos:
		if s.utxoCursor < len(s.ocCtx.Utxos) &&
			!s.pencilFocused {
			s.pencilFocused = true
		}
	}
	return s, nil
}

func (s *OnChainHomeScreen) handleUp() (
	Screen, tea.Cmd,
) {
	switch s.focusZone {
	case ocHomeZoneTxs:
		if s.txCursor > 0 {
			s.txCursor--
		} else {
			s.focusZone = ocHomeZoneUtxos
			if len(s.ocCtx.Utxos) > 0 {
				s.utxoCursor =
					len(s.ocCtx.Utxos) - 1
			}
		}
	case ocHomeZoneUtxos:
		if s.utxoCursor > 0 {
			s.utxoCursor--
			s.pencilFocused = false
		} else {
			s.focusZone = ocHomeZoneButtons
			s.btnIdx = 0
			s.pencilFocused = false
		}
	case ocHomeZoneButtons:
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
	}
	return s, nil
}

func (s *OnChainHomeScreen) handleDown() (
	Screen, tea.Cmd,
) {
	switch s.focusZone {
	case ocHomeZoneButtons:
		if len(s.ocCtx.Utxos) > 0 {
			s.focusZone = ocHomeZoneUtxos
			s.utxoCursor = 0
			s.pencilFocused = false
		} else if len(s.ocCtx.OnChainTxs) > 0 {
			s.focusZone = ocHomeZoneTxs
			s.txCursor = 0
		}
	case ocHomeZoneUtxos:
		if s.utxoCursor < len(s.ocCtx.Utxos)-1 {
			s.utxoCursor++
			s.pencilFocused = false
		} else if len(s.ocCtx.OnChainTxs) > 0 {
			s.focusZone = ocHomeZoneTxs
			s.txCursor = 0
			s.pencilFocused = false
		}
	case ocHomeZoneTxs:
		if s.txCursor < len(s.ocCtx.OnChainTxs)-1 {
			s.txCursor++
		}
	}
	return s, nil
}

func (s *OnChainHomeScreen) handleTab() (
	Screen, tea.Cmd,
) {
	switch s.focusZone {
	case ocHomeZoneButtons:
		if len(s.ocCtx.Utxos) > 0 {
			s.focusZone = ocHomeZoneUtxos
			s.utxoCursor = 0
			s.pencilFocused = false
		} else if len(s.ocCtx.OnChainTxs) > 0 {
			s.focusZone = ocHomeZoneTxs
			s.txCursor = 0
		}
	case ocHomeZoneUtxos:
		if len(s.ocCtx.OnChainTxs) > 0 {
			s.focusZone = ocHomeZoneTxs
			s.txCursor = 0
			s.pencilFocused = false
		}
	case ocHomeZoneTxs:
		// At bottom zone — nowhere to go
	}
	return s, nil
}

func (s *OnChainHomeScreen) handleShiftTab() (
	Screen, tea.Cmd,
) {
	switch s.focusZone {
	case ocHomeZoneTxs:
		s.focusZone = ocHomeZoneUtxos
		if len(s.ocCtx.Utxos) > 0 {
			s.utxoCursor = 0
		} else {
			s.focusZone = ocHomeZoneButtons
			s.btnIdx = 0
		}
		return s, nil
	case ocHomeZoneUtxos:
		s.focusZone = ocHomeZoneButtons
		s.btnIdx = 0
		s.pencilFocused = false
		return s, nil
	case ocHomeZoneButtons:
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
	}
	return s, nil
}

func (s *OnChainHomeScreen) handleEnter() (
	Screen, tea.Cmd,
) {
	switch s.focusZone {
	case ocHomeZoneButtons:
		switch s.btnIdx {
		case 0: // Receive
			return s.openReceive()
		case 1: // Send
			return s.openSend()
		}
	case ocHomeZoneUtxos:
		if s.utxoCursor < len(s.ocCtx.Utxos) {
			if s.pencilFocused {
				s.openLabelPopup()
				return s, nil
			}
			return s.openUtxoDetail()
		}
	case ocHomeZoneTxs:
		if s.txCursor < len(s.ocCtx.OnChainTxs) {
			return s.openTxDetail()
		}
	}
	return s, nil
}

func (s *OnChainHomeScreen) openReceive() (
	Screen, tea.Cmd,
) {
	screen := NewOCReceiveScreen(s.ctx)
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabOCReceive,
			Label:  "⛓ Receive",
			Screen: screen,
		}
	}
}

func (s *OnChainHomeScreen) openSend() (
	Screen, tea.Cmd,
) {
	screen := NewOnChainSendScreen(
		s.ctx, s.ocCtx)
	// Pre-fill amount from UTXO selection
	if len(s.ocCtx.UtxoSelected) > 0 {
		screen.amtInput.SetValue(
			fmt.Sprintf("%d",
				s.ocCtx.UtxoSelectedTotal))
	}
	// Pre-fill fee from cached tiers
	if s.ocCtx.SendFeeTiers[0].SatPerVB > 0 {
		screen.feeInput.SetValue(
			fmt.Sprintf("%.0f",
				s.ocCtx.SendFeeTiers[0].SatPerVB))
	}
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabOnChain,
			Label:  "⛓ Send",
			Screen: screen,
		}
	}
}

func (s *OnChainHomeScreen) openUtxoDetail() (
	Screen, tea.Cmd,
) {
	u := s.ocCtx.Utxos[s.utxoCursor]
	label := u.Address
	if len(label) > 14 {
		label = label[:12] + ".."
	}
	txDate := s.utxoDate(u.Txid)
	txLabel := s.utxoTxLabel(u.Txid)
	screen := NewUtxoDetailScreen(
		s.ctx, u, txDate, txLabel)
	idx := s.utxoCursor
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:        tabUtxoDetail,
			Label:       label,
			Index:       idx,
			Screen:      screen,
			FocusTabBar: true,
		}
	}
}

func (s *OnChainHomeScreen) openTxDetail() (
	Screen, tea.Cmd,
) {
	tx := s.ocCtx.OnChainTxs[s.txCursor]
	label := tx.Label
	if len(label) > 14 {
		label = label[:12] + ".."
	}
	var pfc []lndrpc.PendingForceCloseChannel
	if s.ctx.Status != nil {
		pfc = s.ctx.Status.pendingForceCloseChannels
	}
	screen := NewOnChainTxScreen(s.ctx, tx, pfc)
	idx := s.txCursor
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:        tabOnChainTx,
			Label:       label,
			Index:       idx,
			Screen:      screen,
			FocusTabBar: true,
		}
	}
}

// ── Label popup keys ────────────────────────────────────

func (s *OnChainHomeScreen) handleLabelPopupKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "up":
		if s.labelOnBtn {
			s.labelOnBtn = false
			s.labelInput.Focus()
		}
		return s, nil
	case "down", "tab":
		if !s.labelOnBtn {
			s.labelOnBtn = true
			s.labelBtnIdx = 0
			s.labelInput.Blur()
		}
		return s, nil
	case "left":
		if s.labelOnBtn {
			if s.labelBtnIdx > 0 {
				s.labelBtnIdx--
			}
			return s, nil
		}
		var cmd tea.Cmd
		s.labelInput, cmd =
			s.labelInput.Update(tea.Msg(msg))
		return s, cmd
	case "right":
		if s.labelOnBtn {
			if s.labelBtnIdx < 1 {
				s.labelBtnIdx++
			}
			return s, nil
		}
		var cmd tea.Cmd
		s.labelInput, cmd =
			s.labelInput.Update(tea.Msg(msg))
		return s, cmd
	case "enter":
		if s.labelOnBtn {
			switch s.labelBtnIdx {
			case 0: // Save
				if s.utxoCursor <
					len(s.ocCtx.Utxos) {
					txid := s.ocCtx.Utxos[s.utxoCursor].Txid
					label := s.labelInput.Value()
					return s, labelTxCmd(
						s.ctx.LndClient,
						txid, label)
				}
				s.closeLabelPopup()
				return s, nil
			case 1: // Cancel
				s.closeLabelPopup()
				return s, nil
			}
		}
		// On label field — move to buttons
		s.labelOnBtn = true
		s.labelBtnIdx = 0
		s.labelInput.Blur()
		return s, nil
	case "backspace":
		if !s.labelOnBtn {
			var cmd tea.Cmd
			s.labelInput, cmd =
				s.labelInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil
	default:
		if !s.labelOnBtn {
			var cmd tea.Cmd
			s.labelInput, cmd =
				s.labelInput.Update(tea.Msg(msg))
			return s, cmd
		}
	}
	return s, nil
}

// ── HandleMsg ───────────────────────────────────────────

func (s *OnChainHomeScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case labelTxMsg:
		if msg.err == nil {
			s.closeLabelPopup()
			return s, tea.Batch(
				fetchOnChainTxCmd(s.ctx.LndClient),
				listUnspentCmd(s.ctx.LndClient))
		}
		return s, nil
	case tea.PasteMsg:
		if s.labelEditing && !s.labelOnBtn {
			var cmd tea.Cmd
			s.labelInput, cmd =
				s.labelInput.Update(msg)
			return s, cmd
		}
	}
	return s, nil
}

// ── View ────────────────────────────────────────────────

func (s *OnChainHomeScreen) View(
	w, h int,
) string {
	s.clampCursors()
	cfg := s.ctx.Cfg
	status := s.ctx.Status

	// ── Fixed header ─────────────────────────────
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render("On-Chain Wallet"),
			w))
	headerLines = append(headerLines, "")

	if !cfg.HasLND() || !cfg.WalletExists() {
		headerLines = append(headerLines,
			theme.Dim.Render(
				" Install LND and create wallet."))
		return strings.Join(headerLines, "\n")
	}
	if status == nil || !status.lndResponding {
		headerLines = append(headerLines,
			theme.Dim.Render(" Waiting for LND..."))
		return strings.Join(headerLines, "\n")
	}

	isFocused := s.ctx.ContentFocused
	utxos := s.ocCtx.Utxos
	txs := s.ocCtx.OnChainTxs

	onchain := "0"
	if status.lndBalance != "" {
		onchain = status.lndBalance
	}
	headerLines = append(headerLines,
		" "+theme.Label.Render("Balance:  ")+
			theme.Value.Render(
				formatSats(parseBalance(onchain))+
					" sats")+
			theme.Dim.Render(
				fmt.Sprintf("  (%d UTXOs)",
					len(utxos))))
	headerLines = append(headerLines, "")

	sendLabel := "Send"
	if len(s.ocCtx.UtxoSelected) > 0 {
		sendLabel = fmt.Sprintf("Send Selected (%s)",
			formatSats(s.ocCtx.UtxoSelectedTotal))
	}
	headerLines = append(headerLines,
		renderButtons(
			[]string{"Receive", sendLabel},
			s.btnIdx,
			isFocused &&
				s.focusZone == ocHomeZoneButtons,
			w))
	headerLines = append(headerLines, "")

	header := strings.Join(headerLines, "\n")
	headerH := len(headerLines)

	// ── Shared styles ────────────────────────────
	hdrStyle := theme.TableHeader
	sepStyle := theme.TableDim

	// ── UTXO table header ───────────────────────
	uDateW := 12
	uLabelW := 15
	uAddrW := 18
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

	if len(utxos) == 0 {
		utxoMidLines = append(utxoMidLines,
			" "+theme.Dim.Render("No UTXOs found."))
	} else {
		for i, u := range utxos {
			isSelected := isFocused &&
				s.focusZone == ocHomeZoneUtxos &&
				s.utxoCursor == i
			isChecked := s.ocCtx.UtxoSelected[i]

			// Date from tx lookup
			dateStr := s.utxoDate(u.Txid)
			if u.Confirmations == 0 {
				dateStr = "unconfirmed"
			}
			dateStr = pad(dateStr, uDateW)

			// Label from tx lookup
			uLabel := s.utxoTxLabel(u.Txid)
			maxLbl := uLabelW - 2
			if isSelected && !s.labelEditing {
				maxLbl = uLabelW - 4
			}
			if len(uLabel) > maxLbl {
				uLabel = uLabel[:maxLbl-2] + ".."
			}

			// Build label cell with pencil
			var labelCell string
			if isSelected && !s.labelEditing {
				pencilStyle := theme.Dim
				labelStyle := lipgloss.NewStyle()
				if s.pencilFocused {
					pencilStyle = theme.NavActive
					labelStyle = theme.NavActive
				}
				labelCell = labelStyle.Render(
					pad(uLabel, maxLbl)) +
					" " +
					pencilStyle.Render("✎")
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
			case isSelected && s.pencilFocused &&
				!s.labelEditing:
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
			if s.labelEditing &&
				s.utxoCursor == i {
				popLines := s.renderLabelPopup(w)
				utxoMidLines = append(utxoMidLines,
					popLines...)
			}
		}
	}

	utxoMidContent := strings.Join(utxoMidLines, "\n")

	// ── Transaction table header ────────────────
	tDateW := 12
	tLabelW := 14
	tConfW := 3
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

	txBalances := s.computeTxBalances()

	if len(txs) == 0 {
		txMidLines = append(txMidLines,
			" "+theme.Dim.Render(
				"No on-chain transactions."))
	} else {
		for i, tx := range txs {
			isSelected := isFocused &&
				s.focusZone == ocHomeZoneTxs &&
				s.txCursor == i

			date := "unconfirmed"
			if tx.IsAnchorSweep {
				date = formatDateShort(tx.Timestamp)
			} else if tx.Confirmations > 0 {
				date = formatDateShort(tx.Timestamp)
			}
			dateStr := pad(date, tDateW)

			txLabel := tx.Label
			if len(txLabel) > tLabelW-1 {
				txLabel = txLabel[:tLabelW-2] + ".."
			}
			labelStr := pad(txLabel, tLabelW)

			confIcon := confIndicator(
				tx.Confirmations)
			if tx.IsAnchorSweep {
				confIcon = "–"
			}
			confCell := confStyle.Render(
				" " + pad(confIcon, tConfW-1))

			valNum := formatSats(tx.Amount)
			if tx.Amount >= 0 {
				valNum = "+" + valNum
			}
			valStr := fmt.Sprintf("%*s",
				tValW, valNum)

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

	// Cursor line accounting for popup
	utxoCursorLine := s.utxoCursor
	if s.labelEditing {
		utxoCursorLine = s.utxoCursor + 4
	}

	utxoVPRendered := renderViewport(
		utxoMidContent, w, utxoVPH,
		utxoCursorLine,
		len(utxoMidLines),
		len(utxos) > 0 &&
			s.focusZone == ocHomeZoneUtxos)

	txVPRendered := renderViewport(
		txMidContent, w, txVPH,
		s.txCursor,
		len(txMidLines),
		len(txs) > 0 &&
			s.focusZone == ocHomeZoneTxs)

	return header + "\n" +
		utxoHeader + "\n" +
		utxoVPRendered + "\n" +
		txHeader + "\n" +
		txVPRendered
}

// ── Label popup ─────────────────────────────────────────

func (s *OnChainHomeScreen) openLabelPopup() {
	if s.utxoCursor >= len(s.ocCtx.Utxos) {
		return
	}
	txLabel := s.utxoTxLabel(
		s.ocCtx.Utxos[s.utxoCursor].Txid)
	contentW := tuiWidth - 2 - 12 - 1 // nav width=12
	fieldW := contentW - 16
	if fieldW < 20 {
		fieldW = 20
	}
	s.labelInput = newDetailField(txLabel, fieldW)
	s.labelInput.Placeholder = "enter label"
	s.labelInput.CharLimit = 64
	s.labelInput.Focus()
	s.labelEditing = true
	s.labelOnBtn = false
	s.labelBtnIdx = 0
	s.pencilFocused = false
}

func (s *OnChainHomeScreen) closeLabelPopup() {
	s.labelEditing = false
	s.labelOnBtn = false
	s.labelBtnIdx = 0
	s.labelInput.Blur()
}

func (s *OnChainHomeScreen) renderLabelPopup(
	w int,
) []string {
	isFocused := s.ctx.ContentFocused
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

	lblActive := isFocused && !s.labelOnBtn
	lblStyle := theme.Label
	marker := " "
	if lblActive {
		lblStyle = theme.NavActive
		marker = theme.NavActive.Render("▸")
	}
	inputView := s.labelInput.View()
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

	btnStr := renderButtons(
		[]string{"Save", "Cancel"},
		s.labelBtnIdx,
		isFocused && s.labelOnBtn, boxW)
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

// ── HelpBindings ────────────────────────────────────────

func (s *OnChainHomeScreen) HelpBindings() []key.Binding {
	if s.labelEditing {
		return s.labelPopupBindings()
	}
	switch s.focusZone {
	case ocHomeZoneUtxos:
		return s.utxoBindings()
	case ocHomeZoneTxs:
		return s.txBindings()
	}
	return s.buttonBindings()
}

func (s *OnChainHomeScreen) buttonBindings() []key.Binding {
	var binds []key.Binding
	if s.btnIdx == 0 {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left"),
				key.WithHelp("←", "sidebar")),
			key.NewBinding(
				key.WithKeys("right"),
				key.WithHelp("→", "button")))
	} else {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left", "right"),
				key.WithHelp("←→", "buttons")))
	}
	binds = append(binds,
		key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "UTXOs")),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")))
	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑", "tab bar")))
	}
	binds = append(binds, kQuit)
	return binds
}

func (s *OnChainHomeScreen) utxoBindings() []key.Binding {
	var binds []key.Binding
	if s.pencilFocused {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left"),
				key.WithHelp("←", "UTXO")),
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "edit label")))
	} else {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("up", "down"),
				key.WithHelp("↑↓", "UTXOs")),
			key.NewBinding(
				key.WithKeys("space"),
				key.WithHelp("space", "select")),
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "details")))
	}
	binds = append(binds,
		key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "transactions")),
		key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("⇧tab", "buttons")),
		kSidebar)
	binds = append(binds, kQuit)
	return binds
}

func (s *OnChainHomeScreen) txBindings() []key.Binding {
	binds := []key.Binding{
		key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "transactions")),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "tx details")),
		key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("⇧tab", "UTXOs")),
		kSidebar,
	}
	binds = append(binds, kQuit)
	return binds
}

func (s *OnChainHomeScreen) labelPopupBindings() []key.Binding {
	if s.labelOnBtn {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("left", "right"),
				key.WithHelp("←→", "buttons")),
			key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑", "label")),
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "select")),
			kQuit,
		}
	}
	return []key.Binding{
		key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "buttons")),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "buttons")),
		kQuit,
	}
}

// ── Coin control ────────────────────────────────────────

func (s *OnChainHomeScreen) toggleSelection(idx int) {
	if idx < 0 || idx >= len(s.ocCtx.Utxos) {
		return
	}
	if s.ocCtx.UtxoSelected[idx] {
		delete(s.ocCtx.UtxoSelected, idx)
	} else {
		s.ocCtx.UtxoSelected[idx] = true
	}
	s.recalcSelectedTotal()
}

func (s *OnChainHomeScreen) recalcSelectedTotal() {
	s.ocCtx.UtxoSelectedTotal = 0
	s.ocCtx.UtxoOutpoints = nil
	for idx := range s.ocCtx.UtxoSelected {
		if idx < len(s.ocCtx.Utxos) {
			s.ocCtx.UtxoSelectedTotal +=
				s.ocCtx.Utxos[idx].AmountSats
			s.ocCtx.UtxoOutpoints = append(
				s.ocCtx.UtxoOutpoints,
				fmt.Sprintf("%s:%d",
					s.ocCtx.Utxos[idx].Txid,
					s.ocCtx.Utxos[idx].Vout))
		}
	}
}

// ── Helpers ─────────────────────────────────────────────

func (s *OnChainHomeScreen) utxoDate(
	txid string,
) string {
	for _, tx := range s.ocCtx.OnChainTxs {
		if tx.Txid == txid {
			return formatDateShort(tx.Timestamp)
		}
	}
	return "—"
}

func (s *OnChainHomeScreen) utxoTxLabel(
	txid string,
) string {
	for _, tx := range s.ocCtx.OnChainTxs {
		if tx.Txid == txid {
			return tx.Label
		}
	}
	return ""
}

func (s *OnChainHomeScreen) computeTxBalances() []int64 {
	txs := s.ocCtx.OnChainTxs
	if len(txs) == 0 {
		return nil
	}
	bal := parseBalance("0")
	if s.ctx.Status != nil &&
		s.ctx.Status.lndBalance != "" {
		bal = parseBalance(s.ctx.Status.lndBalance)
	}
	balances := make([]int64, len(txs))
	for i := range txs {
		balances[i] = bal
		bal -= txs[i].Amount
	}
	return balances
}

func (s *OnChainHomeScreen) clampCursors() {
	utxos := s.ocCtx.Utxos
	if len(utxos) == 0 {
		s.utxoCursor = 0
	} else if s.utxoCursor >= len(utxos) {
		s.utxoCursor = len(utxos) - 1
	}
	if s.utxoCursor < 0 {
		s.utxoCursor = 0
	}

	txs := s.ocCtx.OnChainTxs
	if len(txs) == 0 {
		s.txCursor = 0
	} else if s.txCursor >= len(txs) {
		s.txCursor = len(txs) - 1
	}
	if s.txCursor < 0 {
		s.txCursor = 0
	}
}
