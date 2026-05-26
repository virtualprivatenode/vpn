package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── UTXO + transaction fetch for channel open ─────────
// Separate message types so the Model-level utxoListMsg
// and onChainTxMsg handlers (which populate OnChainContext)
// are unaffected. Both routed via dispatchToTab(tabOpenChannel)
// in update.go. Fetched in parallel via tea.Batch,
// matching the previewSection(secOnChain) pattern.

type coUtxoListMsg struct {
	utxos []lndrpc.UTXO
	err   error
}

type coTxListMsg struct {
	txs []lndrpc.OnChainTx
	err error
}

func fetchChannelUtxosCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return coUtxoListMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		utxos, err := client.ListUnspent(0, 999999)
		return coUtxoListMsg{utxos: utxos, err: err}
	}
}

func fetchChannelTxsCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return coTxListMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		txs, err := client.GetTransactions()
		return coTxListMsg{txs: txs, err: err}
	}
}

func (s *ChannelOpenScreen) handleUtxoList(
	msg coUtxoListMsg,
) (Screen, tea.Cmd) {
	if msg.err != nil {
		s.error = msg.err.Error()
		return s, nil
	}
	s.utxos = msg.utxos
	s.utxoFetched = true
	// Prune selections beyond new UTXO range
	for idx := range s.utxoSelected {
		if idx >= len(s.utxos) {
			delete(s.utxoSelected, idx)
		}
	}
	s.recalcUtxoTotal()
	return s, nil
}

func (s *ChannelOpenScreen) handleTxList(
	msg coTxListMsg,
) (Screen, tea.Cmd) {
	if msg.err != nil {
		// Non-fatal: table still works without
		// date and label columns.
		return s, nil
	}
	s.txs = msg.txs
	return s, nil
}

// ── Transaction lookup helpers ────────────────────────
// Same pattern as on-chain home's utxoDate / utxoTxLabel
// but reading from the screen's own local tx list.

func (s *ChannelOpenScreen) utxoDate(
	txid string,
) string {
	for _, tx := range s.txs {
		if tx.Txid == txid {
			return formatDateShort(tx.Timestamp)
		}
	}
	return "—"
}

func (s *ChannelOpenScreen) utxoTxLabel(
	txid string,
) string {
	for _, tx := range s.txs {
		if tx.Txid == txid {
			return tx.Label
		}
	}
	return ""
}

// ── Coin control sub-step ─────────────────────────────
// Opened via enter on the [Coin control] button in the
// amounts zone. Same sub-step pattern as coStepCustomPeer.

const coUtxoVisibleRows = 10

func (s *ChannelOpenScreen) handleCoinControlKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch s.ccZone {
	case coCCZoneList:
		return s.handleCCListKey(keyStr)
	case coCCZoneButtons:
		return s.handleCCButtonKey(keyStr)
	}
	return s, nil
}

func (s *ChannelOpenScreen) handleCCListKey(
	keyStr string,
) (Screen, tea.Cmd) {
	if !s.utxoFetched || len(s.utxos) == 0 {
		switch keyStr {
		case "ctrl+c":
			return s, tea.Quit
		case "left":
			return s, emitFocusSidebar
		case "up", "shift+tab":
			if s.ctx.HasTabs {
				return s, emitFocusTabBar
			}
			return s, nil
		case "down", "tab":
			s.ccZone = coCCZoneButtons
			return s, nil
		case "backspace":
			return s.returnFromCoinControl(false)
		}
		return s, nil
	}

	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		return s, emitFocusSidebar
	case "up":
		if s.utxoCursor > 0 {
			s.utxoCursor--
		} else if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down":
		if s.utxoCursor < len(s.utxos)-1 {
			s.utxoCursor++
		} else {
			s.ccZone = coCCZoneButtons
		}
		return s, nil
	case "tab":
		s.ccZone = coCCZoneButtons
		return s, nil
	case "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "space", "enter":
		s.toggleUtxoSelection(s.utxoCursor)
		return s, nil
	case "backspace":
		return s.returnFromCoinControl(false)
	}
	return s, nil
}

func (s *ChannelOpenScreen) handleCCButtonKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.ccBtnIdx > 0 {
			s.ccBtnIdx--
		} else {
			return s, emitFocusSidebar
		}
		return s, nil
	case "right":
		if s.ccBtnIdx < 1 {
			s.ccBtnIdx++
		}
		return s, nil
	case "up", "shift+tab":
		s.ccZone = coCCZoneList
		return s, nil
	case "enter":
		switch s.ccBtnIdx {
		case 0: // Go Back
			return s.returnFromCoinControl(false)
		case 1: // Confirm
			return s.returnFromCoinControl(true)
		}
		return s, nil
	case "backspace":
		return s.returnFromCoinControl(false)
	}
	return s, nil
}

// returnFromCoinControl exits the coin control sub-step.
// If confirm=true and UTXOs are selected, pre-fills amount
// with the UTXO total, sets FundMax, auto-confirms, and
// advances to the fee zone. Otherwise returns to amounts.
func (s *ChannelOpenScreen) returnFromCoinControl(
	confirm bool,
) (Screen, tea.Cmd) {
	s.step = coStepInput
	s.error = ""

	if confirm && len(s.utxoSelected) > 0 {
		// Pre-fill amount, auto-confirm, advance
		s.amount = s.utxoSelectedTotal
		s.amountInput.SetSats(s.amount)
		s.fundMax = true
		s.amountConfirmed = true
		s.amountIdx = 1
		s.amountInput.Blur()
		s.focusZone = coZoneFee
		s.feeInput.Focus()
		return s, nil
	}

	// No selection or cancelled
	if len(s.utxoSelected) == 0 {
		s.amount = 0
		s.fundMax = false
		s.amountConfirmed = false
		s.amountInput.Clear()
	}
	s.focusZone = coZoneAmounts
	s.amountIdx = 0
	return s, nil
}

// ── Selection helpers ──────────────────────────────────

func (s *ChannelOpenScreen) toggleUtxoSelection(
	idx int,
) {
	if idx < 0 || idx >= len(s.utxos) {
		return
	}
	if s.utxoSelected[idx] {
		delete(s.utxoSelected, idx)
	} else {
		s.utxoSelected[idx] = true
	}
	s.recalcUtxoTotal()
}

func (s *ChannelOpenScreen) recalcUtxoTotal() {
	s.utxoSelectedTotal = 0
	s.utxoOutpoints = nil
	for idx := range s.utxoSelected {
		if idx < len(s.utxos) {
			s.utxoSelectedTotal +=
				s.utxos[idx].AmountSats
			s.utxoOutpoints = append(
				s.utxoOutpoints,
				fmt.Sprintf("%s:%d",
					s.utxos[idx].Txid,
					s.utxos[idx].Vout))
		}
	}
}

// ── Coin control sub-step view ──────────────────────────

func (s *ChannelOpenScreen) viewCoinControl(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Coin Control")

	// Selection summary
	if len(s.utxoSelected) > 0 {
		p.field("Selected: ",
			fmt.Sprintf("%d UTXO (%s sats)",
				len(s.utxoSelected),
				formatSats(s.utxoSelectedTotal)))
	} else {
		p.dim(" Select UTXOs for the channel open.")
	}
	p.blank()

	focused := s.ctx.ContentFocused &&
		s.ccZone == coCCZoneList
	s.viewUtxoTable(p, w, focused)

	p.appendError(s.error)

	btnFocused := s.ctx.ContentFocused &&
		s.ccZone == coCCZoneButtons
	return p.renderWithBottomButtons(
		[]string{"Go Back", "Confirm"},
		s.ccBtnIdx, btnFocused, h)
}

// ── UTXO table rendering ───────────────────────────────
// Column layout matches on-chain home: Date, Label,
// Address, Value. Shows up to coUtxoVisibleRows with
// scroll indicators.

func (s *ChannelOpenScreen) viewUtxoTable(
	p *paneBuilder, w int, focused bool,
) {
	if !s.utxoFetched {
		p.dim(" Loading...")
		return
	}
	if len(s.utxos) == 0 {
		p.dim(" No UTXOs available.")
		return
	}

	// Column widths matching on-chain home
	dateW := 12
	labelW := 15
	addrW := 18
	valW := w - dateW - labelW - addrW - 5
	if valW < 12 {
		valW = 12
	}

	sepStyle := theme.TableDim
	hdrStyle := theme.TableHeader

	// Column headers
	p.line(" " +
		hdrStyle.Render(pad("Date", dateW)) +
		hdrStyle.Render(pad("Label", labelW)) +
		hdrStyle.Render(pad("Address", addrW)) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", valW, "Value")))

	// Separator
	p.line(" " + sepStyle.Render(
		strings.Repeat("─", w-2)))

	// Visible window
	visRows := coUtxoVisibleRows
	if len(s.utxos) < visRows {
		visRows = len(s.utxos)
	}
	startIdx := s.utxoCursor - visRows/2
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx+visRows > len(s.utxos) {
		startIdx = len(s.utxos) - visRows
		if startIdx < 0 {
			startIdx = 0
		}
	}
	endIdx := startIdx + visRows

	hasAbove := startIdx > 0
	hasBelow := endIdx < len(s.utxos)

	checkStyle := lipgloss.NewStyle().
		Foreground(theme.ColorCheck)

	for i := startIdx; i < endIdx; i++ {
		u := s.utxos[i]
		isCursor := focused && s.utxoCursor == i
		isChecked := s.utxoSelected[i]

		// Date from tx lookup
		dateStr := s.utxoDate(u.Txid)
		if u.Confirmations == 0 {
			dateStr = "unconfirmed"
		}

		// Label from tx lookup
		uLabel := s.utxoTxLabel(u.Txid)
		maxLbl := labelW - 1
		if len(uLabel) > maxLbl {
			uLabel = uLabel[:maxLbl-2] + ".."
		}

		// Address: first7..last7
		addr := u.Address
		if len(addr) > 16 {
			addr = addr[:7] + ".." +
				addr[len(addr)-7:]
		}

		// Formatted cells
		dateCell := pad(dateStr, dateW)
		labelCell := pad(uLabel, labelW)
		addrCell := pad(addr, addrW)
		valCell := fmt.Sprintf("%*s",
			valW, formatSats(u.AmountSats))

		// Marker (1 char, matching on-chain home)
		marker := " "
		if isChecked {
			marker = "✓"
		}
		if isCursor && !isChecked {
			marker = theme.NavActive.Render("▸")
		}

		// Scroll indicator on edge rows
		scrollInd := ""
		if i == startIdx && hasAbove {
			scrollInd = " ▲"
		}
		if i == endIdx-1 && hasBelow {
			scrollInd = " ▼"
		}

		switch {
		case isCursor:
			p.line(marker +
				theme.NavActive.Render(dateCell) +
				theme.NavActive.Render(labelCell) +
				theme.NavActive.Render(addrCell) +
				theme.NavActive.Render(valCell) +
				theme.Dim.Render(scrollInd))
		case isChecked:
			p.line(marker +
				checkStyle.Render(dateCell) +
				checkStyle.Render(labelCell) +
				checkStyle.Render(addrCell) +
				checkStyle.Render(valCell) +
				theme.Dim.Render(scrollInd))
		default:
			p.line(marker +
				theme.Dim.Render(dateCell) +
				theme.Value.Render(labelCell) +
				theme.Dim.Render(addrCell) +
				theme.Value.Render(valCell) +
				theme.Dim.Render(scrollInd))
		}
	}
}

// ── Helpbar bindings ───────────────────────────────────

func (s *ChannelOpenScreen) coinControlBindings() []key.Binding {
	if s.ccZone == coCCZoneButtons {
		binds := buttonNav(s.ccBtnIdx)
		binds = append(binds,
			kEnter, kShiftTabBack, kBack, kQuit)
		return binds
	}
	if !s.utxoFetched || len(s.utxos) == 0 {
		return []key.Binding{
			kTabNext, kBack, kQuit,
		}
	}
	return []key.Binding{
		bind("↑↓", "navigate", "up", "down"),
		bind("enter/space", "select", "enter", "space"),
		kTabNext, kBack, kQuit,
	}
}
