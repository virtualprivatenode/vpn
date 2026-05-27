package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Receive screen steps ────────────────────────────────

type recvStep int

const (
	recvStepInput   recvStep = iota // amount + memo entry
	recvStepWaiting                 // invoice created, waiting for payment
	recvStepPaid                    // payment received
	recvStepExpired                 // invoice expired
	recvStepError                   // connection lost / error
)

// ── Input focus zones ───────────────────────────────────

const (
	recvZoneAmount  = 0
	recvZoneMemo    = 1
	recvZoneBlind   = 2
	recvZoneButtons = 3
)

// ── ReceiveScreen ───────────────────────────────────────

type ReceiveScreen struct {
	ctx  *ScreenContext
	step recvStep

	// Input state
	amountInput AmountInput
	memoInput   textinput.Model
	blindPaths  bool // blinded paths on invoice (privacy)
	focusZone   int  // 0=amount, 1=memo, 2=blind, 3=buttons
	btnIdx      int  // 0=Clear, 1=Create Invoice
	inputError  string

	// Invoice state (set after creation)
	payReq      string
	paymentHash string
	amountSats  int64

	// Waiting state
	buttonIdx int // 0=Show QR, 1=Copyable Invoice

	// Result state
	settled bool
	expired bool
	error   string
}

func NewReceiveScreen(
	ctx *ScreenContext,
) *ReceiveScreen {
	amt := NewAmountInput()
	amt.Focus() // amount is the initial focus zone
	return &ReceiveScreen{
		ctx:         ctx,
		step:        recvStepInput,
		amountInput: amt,
		memoInput:   newRecvMemoInput(),
		blindPaths:  true,
		focusZone:   recvZoneAmount,
		btnIdx:      1, // default to Create Invoice
	}
}

// ── Screen interface ────────────────────────────────────

func (s *ReceiveScreen) Init() tea.Cmd {
	return nil
}

func (s *ReceiveScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case recvStepInput:
		return s.handleInputKey(keyStr, msg)
	case recvStepWaiting:
		return s.handleWaitingKey(keyStr)
	case recvStepPaid:
		return s.handlePaidKey(keyStr)
	case recvStepExpired:
		return s.handleExpiredKey(keyStr)
	case recvStepError:
		return s.handleErrorKey(keyStr)
	}
	return s, nil
}

func (s *ReceiveScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		return s.handlePaste(msg)
	case invoiceCreatedMsg:
		return s.handleInvoiceCreated(msg)
	case invoiceSettledMsg:
		return s.handleInvoiceSettled(msg)
	}
	return s, nil
}

func (s *ReceiveScreen) View(w, h int) string {
	switch s.step {
	case recvStepInput:
		return s.viewInput(w, h)
	case recvStepWaiting:
		return s.viewWaiting(w, h)
	case recvStepPaid:
		return s.viewPaid(w, h)
	case recvStepExpired:
		return s.viewExpired(w, h)
	case recvStepError:
		return s.viewError(w, h)
	}
	return ""
}

func (s *ReceiveScreen) HelpBindings() []key.Binding {
	switch s.step {
	case recvStepInput:
		return s.inputBindings()
	case recvStepWaiting:
		return actionButtonBindings(
			s.buttonIdx, s.ctx.HasTabs)
	case recvStepPaid, recvStepExpired, recvStepError:
		return resultBindings(s.ctx.HasTabs)
	}
	return nil
}

func (s *ReceiveScreen) inputBindings() []key.Binding {
	var binds []key.Binding
	switch s.focusZone {
	case recvZoneAmount:
		binds = append(binds,
			kUpDownFields, kTabNext, kEnterCreate,
			kSidebar)
		if s.ctx.HasTabs {
			binds = append(binds, kShiftTabBar)
		}
	case recvZoneMemo:
		binds = append(binds,
			kUpDownFields, kTabNext, kEnterCreate,
			bind("⇧tab", "amount", "shift+tab"),
			kSidebar)
	case recvZoneBlind:
		binds = append(binds,
			kUpDownFields,
			bind("space", "toggle", "space"),
			kEnterNext, kTabNext,
			bind("⇧tab", "memo", "shift+tab"),
			kSidebar)
	case recvZoneButtons:
		binds = append(binds,
			kLeftRightButtons, kEnter,
			bind("⇧tab", "toggle", "shift+tab"),
			kBack)
	}
	binds = append(binds, kQuit)
	return binds
}

// ── Focus helpers ──────────────────────────────────────

func (s *ReceiveScreen) focusInputZone() {
	s.amountInput.Blur()
	s.memoInput.Blur()
	switch s.focusZone {
	case recvZoneAmount:
		s.amountInput.Focus()
	case recvZoneMemo:
		s.memoInput.Focus()
	case recvZoneBlind:
		// no text input to focus
	}
}

// ── Input step ──────────────────────────────────────────

func (s *ReceiveScreen) handleInputKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.focusZone {
	case recvZoneAmount:
		return s.handleAmountKey(keyStr, msg)
	case recvZoneMemo:
		return s.handleMemoKey(keyStr, msg)
	case recvZoneBlind:
		return s.handleBlindKey(keyStr)
	case recvZoneButtons:
		return s.handleButtonKey(keyStr)
	}
	return s, nil
}

// advanceToButtons blurs text inputs and moves focus to
// the buttons zone with Create Invoice as the default.
func (s *ReceiveScreen) advanceToButtons() {
	s.amountInput.Blur()
	s.memoInput.Blur()
	s.focusZone = recvZoneButtons
	s.btnIdx = 1 // default to Create Invoice
}

// ── Per-zone key handlers ─────────────────────────────

func (s *ReceiveScreen) handleAmountKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if !s.amountInput.Empty() {
			cmd := s.amountInput.Update(
				tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusSidebar
	case "right":
		cmd := s.amountInput.Update(tea.Msg(msg))
		return s, cmd
	case "backspace":
		cmd := s.amountInput.Update(tea.Msg(msg))
		return s, cmd
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		s.focusZone = recvZoneMemo
		s.focusInputZone()
		return s, nil
	case "enter":
		s.advanceToButtons()
		return s, nil
	default:
		cmd := s.amountInput.Update(tea.Msg(msg))
		return s, cmd
	}
}

func (s *ReceiveScreen) handleMemoKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.memoInput.Value() != "" {
			var cmd tea.Cmd
			s.memoInput, cmd =
				s.memoInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusSidebar
	case "right":
		var cmd tea.Cmd
		s.memoInput, cmd =
			s.memoInput.Update(tea.Msg(msg))
		return s, cmd
	case "backspace":
		var cmd tea.Cmd
		s.memoInput, cmd =
			s.memoInput.Update(tea.Msg(msg))
		return s, cmd
	case "up", "shift+tab":
		s.focusZone = recvZoneAmount
		s.focusInputZone()
		return s, nil
	case "down", "tab":
		s.focusZone = recvZoneBlind
		s.focusInputZone()
		return s, nil
	case "enter":
		s.advanceToButtons()
		return s, nil
	default:
		var cmd tea.Cmd
		s.memoInput, cmd =
			s.memoInput.Update(tea.Msg(msg))
		return s, cmd
	}
}

func (s *ReceiveScreen) handleBlindKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		return s, emitFocusSidebar
	case "backspace":
		return s, emitFocusParent
	case "up", "shift+tab":
		s.focusZone = recvZoneMemo
		s.focusInputZone()
		return s, nil
	case "down", "tab", "enter":
		s.advanceToButtons()
		return s, nil
	case "space":
		s.blindPaths = !s.blindPaths
		return s, nil
	}
	return s, nil
}

func (s *ReceiveScreen) handleButtonKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.btnIdx < 1 {
			s.btnIdx++
		}
		return s, nil
	case "up", "shift+tab":
		s.focusZone = recvZoneBlind
		s.focusInputZone()
		return s, nil
	case "backspace":
		return s, emitFocusParent
	case "enter":
		switch s.btnIdx {
		case 0: // Clear
			s.amountInput.Clear()
			s.memoInput = newRecvMemoInput()
			s.blindPaths = true
			s.inputError = ""
			s.focusZone = recvZoneAmount
			s.focusInputZone()
			return s, nil
		case 1: // Create Invoice
			return s.submitInvoice()
		}
	}
	return s, nil
}

// submitInvoice validates and creates the invoice.
func (s *ReceiveScreen) submitInvoice() (
	Screen, tea.Cmd,
) {
	if s.amountInput.Empty() {
		s.inputError = "Enter an amount"
		return s, nil
	}
	amt := s.amountInput.Sats()
	if amt < 1 {
		s.inputError = "Minimum 1 sat"
		return s, nil
	}
	s.amountSats = amt
	s.inputError = ""
	return s, createInvoiceCmd(
		s.ctx.LndClient, amt,
		s.memoInput.Value(),
		s.blindPaths)
}

// ── Waiting step ────────────────────────────────────────

func (s *ReceiveScreen) handleWaitingKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.buttonIdx > 0 {
			s.buttonIdx--
		} else {
			return s, emitFocusSidebar
		}
		return s, nil
	case "up":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		return s, nil
	case "backspace":
		return s, emitFocusParent
	case "right":
		if s.buttonIdx < 1 {
			s.buttonIdx++
		}
		return s, nil
	case "enter":
		if s.buttonIdx == 0 && s.payReq != "" {
			return s, func() tea.Msg {
				return showQRMsg{
					URL: s.payReq,
					Label: fmt.Sprintf(
						"Invoice — %s sats",
						formatSats(s.amountSats)),
				}
			}
		}
		if s.buttonIdx == 1 && s.payReq != "" {
			return s, showInvoiceCmd(s.payReq)
		}
	}
	return s, nil
}

// ── Paid step ───────────────────────────────────────────

func (s *ReceiveScreen) handlePaidKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "enter":
		return s, tea.Batch(
			emitCloseTab,
			emitRefreshStatus,
			fetchPaymentHistoryCmd(
				s.ctx.LndClient))
	case "left":
		return s, emitFocusSidebar
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
	case "backspace":
		return s, emitFocusParent
	}
	return s, nil
}

// ── Expired step ────────────────────────────────────────

func (s *ReceiveScreen) handleExpiredKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "enter":
		return s, emitCloseTab
	case "left":
		return s, emitFocusSidebar
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
	case "backspace":
		return s, emitFocusParent
	}
	return s, nil
}

// ── Error step ──────────────────────────────────────────

func (s *ReceiveScreen) handleErrorKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "enter":
		return s, emitCloseTab
	case "left":
		return s, emitFocusSidebar
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
	case "backspace":
		return s, emitFocusParent
	}
	return s, nil
}

// ── Paste handling ──────────────────────────────────────

func (s *ReceiveScreen) handlePaste(
	msg tea.PasteMsg,
) (Screen, tea.Cmd) {
	if s.step != recvStepInput {
		return s, nil
	}
	var cmd tea.Cmd
	if s.focusZone == recvZoneAmount {
		cmd = s.amountInput.Update(msg)
	} else if s.focusZone == recvZoneMemo {
		s.memoInput, cmd =
			s.memoInput.Update(msg)
	}
	return s, cmd
}

// ── Async message handlers ──────────────────────────────

func (s *ReceiveScreen) handleInvoiceCreated(
	msg invoiceCreatedMsg,
) (Screen, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		if s.blindPaths && (strings.Contains(errStr,
			"blinded") || strings.Contains(errStr,
			"routes to self")) {
			s.inputError = errStr +
				" — try turning off blinded paths"
		} else {
			s.inputError = errStr
		}
		return s, nil
	}
	s.payReq = msg.payReq
	s.paymentHash = msg.paymentHash
	s.amountSats = msg.amountSats
	s.step = recvStepWaiting
	return s, waitForInvoiceCmd(
		s.ctx.LndClient, msg.paymentHash)
}

func (s *ReceiveScreen) handleInvoiceSettled(
	msg invoiceSettledMsg,
) (Screen, tea.Cmd) {
	if msg.err != nil {
		s.error = msg.err.Error()
		s.step = recvStepError
		return s, nil
	}
	if msg.settled {
		s.settled = true
		s.step = recvStepPaid
	} else if msg.expired {
		s.expired = true
		s.step = recvStepExpired
	}
	return s, nil
}

// ── Views ───────────────────────────────────────────────

func (s *ReceiveScreen) viewInput(w, h int) string {
	p := newPane(w)
	p.title(theme.Header, "⚡ Receive Payment")

	if !s.ctx.Cfg.HasLND() ||
		!s.ctx.Cfg.WalletExists() {
		p.dim("Create LND wallet to receive.")
		return p.render()
	}
	if s.ctx.Status == nil ||
		!s.ctx.Status.lndResponding {
		p.dim("Waiting for LND...")
		return p.render()
	}

	isFocused := s.ctx.ContentFocused
	amtFocused := isFocused &&
		s.focusZone == recvZoneAmount
	memoFocused := isFocused &&
		s.focusZone == recvZoneMemo

	p.input("Amount (sats):",
		s.amountInput.View(), amtFocused)
	p.blank()
	p.input("Memo (optional):",
		s.memoInput.View(), memoFocused)
	p.dim("Visible to the sender.")
	p.blank()

	// ── Blinded paths toggle ──
	blindFocused := isFocused &&
		s.focusZone == recvZoneBlind
	blindLabel := theme.Header
	blindMarker := " "
	if blindFocused {
		blindMarker = theme.NavActive.Render("▸")
	}
	blindValue := theme.Good.Render("● on")
	if !s.blindPaths {
		blindValue = theme.Dim.Render("○ off")
	}
	p.line(" " + blindLabel.Render("Blinded paths:"))
	p.line(blindMarker + " " + blindValue)
	p.dim("Hides your node identity on invoices.")

	p.appendError(s.inputError)

	// ── Buttons pinned to bottom ──
	btnFocused := isFocused &&
		s.focusZone == recvZoneButtons
	return p.renderWithBottomButtons(
		[]string{"Clear", "Create Invoice"},
		s.btnIdx, btnFocused, h)
}

func (s *ReceiveScreen) viewWaiting(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Waiting for Payment")

	p.field("Amount: ",
		formatSats(s.amountSats)+" sats")
	p.blank()

	if s.payReq != "" {
		p.labelLine("Invoice:")
		display := s.payReq
		maxChars := (w - 2) * 4 // ~4 wrapped lines
		if len(display) > maxChars {
			display = display[:maxChars] + "..."
		}
		p.monoWrap(display)
		p.blank()

		p.dim("Waiting for payment...")
	}

	// ── Buttons pinned to bottom ──
	btnFocused := s.ctx.ContentFocused
	return p.renderWithBottomButtons(
		[]string{"Show QR", "Copyable Invoice"},
		s.buttonIdx, btnFocused, h)
}

func (s *ReceiveScreen) viewPaid(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Success, "Payment Received")
	p.field("Amount: ",
		formatSats(s.amountSats)+" sats")
	return p.renderWithBottomButtons(
		[]string{"Done"}, 0,
		s.ctx.ContentFocused, h)
}

func (s *ReceiveScreen) viewExpired(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Warning, "Invoice Expired")
	p.dim("Create a new invoice to try again.")
	return p.renderWithBottomButtons(
		[]string{"Done"}, 0,
		s.ctx.ContentFocused, h)
}

func (s *ReceiveScreen) viewError(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Warning, "Receive Failed")
	p.warnWrap(s.error)
	p.blank()
	p.dim("The connection to LND was lost while")
	p.dim("waiting for payment. Your invoice may")
	p.dim("still be valid — check your payment")
	p.dim("history after reconnecting.")
	return p.renderWithBottomButtons(
		[]string{"Done"}, 0,
		s.ctx.ContentFocused, h)
}
