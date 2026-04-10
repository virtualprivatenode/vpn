package welcome

import (
	"fmt"

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
	recvZoneButtons = 2
)

// ── ReceiveScreen ───────────────────────────────────────

type ReceiveScreen struct {
	ctx  *ScreenContext
	step recvStep

	// Input state
	amountInput textinput.Model
	memoInput   textinput.Model
	focusZone   int // 0=amount, 1=memo, 2=buttons
	btnIdx      int // 0=Clear, 1=Create Invoice
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
	return &ReceiveScreen{
		ctx:         ctx,
		step:        recvStepInput,
		amountInput: newRecvAmountInput(),
		memoInput:   newRecvMemoInput(),
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
		return s.waitingBindings()
	case recvStepPaid, recvStepExpired, recvStepError:
		return newResultBindings().ShortHelp()
	}
	return nil
}

// inputBindings returns dynamic help bindings for the
// input step based on current focus zone.
func (s *ReceiveScreen) inputBindings() []key.Binding {
	var binds []key.Binding

	switch s.focusZone {
	case recvZoneAmount, recvZoneMemo:
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("up", "down"),
				key.WithHelp("↑↓", "fields")),
			key.NewBinding(
				key.WithKeys("tab"),
				key.WithHelp("tab", "next")),
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "create")),
			kSidebar)
		if s.ctx.HasTabs {
			binds = append(binds,
				key.NewBinding(
					key.WithKeys("shift+tab"),
					key.WithHelp("⇧tab", "tab bar")))
		}
	case recvZoneButtons:
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left", "right"),
				key.WithHelp("←→", "buttons")),
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "select")),
			key.NewBinding(
				key.WithKeys("shift+tab"),
				key.WithHelp("⇧tab", "back")))
	}

	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑", "tab bar")))
	}

	binds = append(binds, kQuit)
	return binds
}

// waitingBindings returns dynamic help bindings for the
// waiting step. When cursor is on the leftmost button,
// left arrow goes to sidebar. Otherwise left/right
// navigate between buttons.
func (s *ReceiveScreen) waitingBindings() []key.Binding {
	var binds []key.Binding

	if s.buttonIdx == 0 {
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

// ── Focus helpers ──────────────────────────────────────

func (s *ReceiveScreen) focusInputZone() {
	s.amountInput.Blur()
	s.memoInput.Blur()
	switch s.focusZone {
	case recvZoneAmount:
		s.amountInput.Focus()
	case recvZoneMemo:
		s.memoInput.Focus()
	}
}

// ── Input step ──────────────────────────────────────────

func (s *ReceiveScreen) handleInputKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit

	case "left":
		// Buttons: move left
		if s.focusZone == recvZoneButtons &&
			s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		// Text inputs: pass through for cursor
		if s.focusZone == recvZoneAmount {
			if s.amountInput.Value() != "" {
				var cmd tea.Cmd
				s.amountInput, cmd =
					s.amountInput.Update(
						tea.Msg(msg))
				return s, cmd
			}
		}
		if s.focusZone == recvZoneMemo {
			if s.memoInput.Value() != "" {
				var cmd tea.Cmd
				s.memoInput, cmd =
					s.memoInput.Update(
						tea.Msg(msg))
				return s, cmd
			}
		}
		return s, emitFocusSidebar

	case "right":
		// Buttons: move right
		if s.focusZone == recvZoneButtons &&
			s.btnIdx < 1 {
			s.btnIdx++
			return s, nil
		}
		// Text inputs: pass through for cursor
		if s.focusZone == recvZoneAmount {
			var cmd tea.Cmd
			s.amountInput, cmd =
				s.amountInput.Update(tea.Msg(msg))
			return s, cmd
		}
		if s.focusZone == recvZoneMemo {
			var cmd tea.Cmd
			s.memoInput, cmd =
				s.memoInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil

	case "backspace":
		// Clean backspace: only deletes characters,
		// never navigates.
		if s.focusZone == recvZoneAmount &&
			s.amountInput.Value() != "" {
			var cmd tea.Cmd
			s.amountInput, cmd =
				s.amountInput.Update(tea.Msg(msg))
			return s, cmd
		}
		if s.focusZone == recvZoneMemo &&
			s.memoInput.Value() != "" {
			var cmd tea.Cmd
			s.memoInput, cmd =
				s.memoInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil

	case "tab":
		// Express forward jump between zones
		if s.focusZone < recvZoneButtons {
			s.focusZone++
			if s.focusZone == recvZoneButtons {
				s.amountInput.Blur()
				s.memoInput.Blur()
				s.btnIdx = 1 // default to Create Invoice
			} else {
				s.focusInputZone()
			}
		}
		return s, nil

	case "shift+tab":
		// Express backward jump between zones
		if s.focusZone > recvZoneAmount {
			s.focusZone--
			s.focusInputZone()
		} else if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "down":
		// Move to next zone at boundary
		if s.focusZone < recvZoneButtons {
			s.focusZone++
			if s.focusZone == recvZoneButtons {
				s.amountInput.Blur()
				s.memoInput.Blur()
				s.btnIdx = 1
			} else {
				s.focusInputZone()
			}
		}
		return s, nil

	case "up":
		// Move to previous zone at boundary
		if s.focusZone > recvZoneAmount {
			s.focusZone--
			s.focusInputZone()
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "enter":
		// Buttons
		if s.focusZone == recvZoneButtons {
			switch s.btnIdx {
			case 0: // Clear
				s.amountInput = newRecvAmountInput()
				s.memoInput = newRecvMemoInput()
				s.inputError = ""
				s.focusZone = recvZoneAmount
				s.focusInputZone()
				return s, nil
			case 1: // Create Invoice
				return s.submitInvoice()
			}
			return s, nil
		}
		// Enter in input field → advance to buttons
		s.focusZone = recvZoneButtons
		s.amountInput.Blur()
		s.memoInput.Blur()
		s.btnIdx = 1 // default to Create Invoice
		return s, nil

	default:
		var cmd tea.Cmd
		if s.focusZone == recvZoneAmount {
			s.amountInput, cmd =
				s.amountInput.Update(tea.Msg(msg))
		} else if s.focusZone == recvZoneMemo {
			s.memoInput, cmd =
				s.memoInput.Update(tea.Msg(msg))
		}
		return s, cmd
	}
}

// submitInvoice validates and creates the invoice.
func (s *ReceiveScreen) submitInvoice() (
	Screen, tea.Cmd,
) {
	val := s.amountInput.Value()
	if val == "" {
		s.inputError = "Enter an amount"
		return s, nil
	}
	amt, err := parseRecvAmount(val)
	if err != nil {
		s.inputError = err.Error()
		return s, nil
	}
	s.amountSats = amt
	s.inputError = ""
	return s, createInvoiceCmd(
		s.ctx.LndClient, amt,
		s.memoInput.Value())
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
	case "enter", "backspace":
		return s, tea.Batch(
			emitCloseTab,
			emitRefreshStatus,
			fetchPaymentHistoryCmd(
				s.ctx.LndClient))
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
	case "enter", "backspace":
		return s, emitCloseTab
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
	case "enter", "backspace":
		return s, emitCloseTab
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
		s.amountInput, cmd =
			s.amountInput.Update(msg)
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
		s.inputError = msg.err.Error()
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
		s.amountInput, amtFocused)
	p.blank()
	p.input("Memo (optional):",
		s.memoInput, memoFocused)

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
		p.monoWrap(s.payReq)
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
