package welcome

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/lndrpc"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ── Send screen steps ────────────────────────────────────

type sendStep int

const (
	sendStepInput    sendStep = iota // paste/type pay req
	sendStepConfirm                  // decoded details, Go Back + Confirm
	sendStepInFlight                 // payment routing
	sendStepResult                   // success or error
)

// ── Input focus zones ───────────────────────────────────

const (
	sendZoneInput   = 0
	sendZoneButtons = 1
)

// ── SendScreen ───────────────────────────────────────────

type SendScreen struct {
	ctx  *ScreenContext
	step sendStep

	// Input state
	sendInput   textinput.Model
	inputBtnIdx int // 0=Clear, 1=Send
	focusZone   int // 0=input field, 1=buttons
	inputError  string

	// Decoded (set after payReqDecodedMsg)
	decodedAmt  int64
	decodedDesc string
	decodedDest string

	// Confirm state
	confirmBtnIdx int // 0=Go Back, 1=Confirm

	// Result state
	error     string
	preimage  string
	routeHops []lndrpc.RouteHop
	feeSats   int64
}

func NewSendScreen(
	ctx *ScreenContext,
) *SendScreen {
	return &SendScreen{
		ctx:         ctx,
		step:        sendStepInput,
		sendInput:   newSendPayReqInput(),
		inputBtnIdx: 1, // default to Send
	}
}

// ── Screen interface ────────────────────────────────────

func (s *SendScreen) Init() tea.Cmd {
	return nil
}

func (s *SendScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case sendStepInput:
		return s.handleInputKey(keyStr, msg)
	case sendStepConfirm:
		return s.handleConfirmKey(keyStr)
	case sendStepInFlight:
		return s.handleInFlightKey(keyStr)
	case sendStepResult:
		return s.handleResultKey(keyStr)
	}
	return s, nil
}

func (s *SendScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		return s.handlePaste(msg)
	case payReqDecodedMsg:
		return s.handlePayReqDecoded(msg)
	case sendPaymentResultMsg:
		return s.handleSendResult(msg)
	}
	return s, nil
}

func (s *SendScreen) View(w, h int) string {
	switch s.step {
	case sendStepInput:
		return s.viewInput(w, h)
	case sendStepConfirm:
		return s.viewConfirm(w, h)
	case sendStepInFlight:
		return s.viewInFlight(w, h)
	case sendStepResult:
		return s.viewResult(w, h)
	}
	return ""
}

func (s *SendScreen) HelpBindings() []key.Binding {
	switch s.step {
	case sendStepInput:
		return s.inputBindings()
	case sendStepConfirm:
		return actionButtonBindings(
			s.confirmBtnIdx, s.ctx.HasTabs)
	case sendStepInFlight:
		return inFlightBindings()
	case sendStepResult:
		return resultBindings(s.ctx.HasTabs)
	}
	return nil
}

// ── Input step ──────────────────────────────────────────

func (s *SendScreen) handleInputKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit

	case "left":
		// Buttons: move left
		if s.focusZone == sendZoneButtons &&
			s.inputBtnIdx > 0 {
			s.inputBtnIdx--
			return s, nil
		}
		// Text input: pass through for cursor
		if s.focusZone == sendZoneInput {
			if s.sendInput.Value() != "" {
				var cmd tea.Cmd
				s.sendInput, cmd =
					s.sendInput.Update(
						tea.Msg(msg))
				return s, cmd
			}
		}
		return s, emitFocusSidebar

	case "right":
		// Buttons: move right
		if s.focusZone == sendZoneButtons &&
			s.inputBtnIdx < 1 {
			s.inputBtnIdx++
			return s, nil
		}
		// Text input: pass through for cursor
		if s.focusZone == sendZoneInput {
			var cmd tea.Cmd
			s.sendInput, cmd =
				s.sendInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil

	case "backspace":
		if s.focusZone == sendZoneInput {
			var cmd tea.Cmd
			s.sendInput, cmd =
				s.sendInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusParent

	case "tab":
		// Express forward jump between zones
		if s.focusZone < sendZoneButtons {
			s.focusZone = sendZoneButtons
			s.sendInput.Blur()
			s.inputBtnIdx = 1 // default to Send
		}
		return s, nil

	case "shift+tab":
		// Express backward jump between zones
		if s.focusZone > sendZoneInput {
			s.focusZone = sendZoneInput
			s.sendInput.Focus()
		} else if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "down":
		// Move to buttons at boundary
		if s.focusZone < sendZoneButtons {
			s.focusZone = sendZoneButtons
			s.sendInput.Blur()
			s.inputBtnIdx = 1
		}
		return s, nil

	case "up":
		// Move to input at boundary
		if s.focusZone > sendZoneInput {
			s.focusZone = sendZoneInput
			s.sendInput.Focus()
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "enter":
		// Buttons
		if s.focusZone == sendZoneButtons {
			switch s.inputBtnIdx {
			case 0: // Clear
				s.sendInput = newSendPayReqInput()
				s.inputError = ""
				s.focusZone = sendZoneInput
				return s, nil
			case 1: // Send
				return s.submitSendPayment()
			}
			return s, nil
		}
		// Enter in input field → advance to buttons
		s.focusZone = sendZoneButtons
		s.sendInput.Blur()
		s.inputBtnIdx = 1 // default to Send
		return s, nil

	default:
		if s.focusZone == sendZoneInput {
			var cmd tea.Cmd
			s.sendInput, cmd =
				s.sendInput.Update(tea.Msg(msg))
			return s, cmd
		}
	}
	return s, nil
}

// submitSendPayment validates the pay req and fires
// decodePayReqCmd.
func (s *SendScreen) submitSendPayment() (
	Screen, tea.Cmd,
) {
	payReq := strings.TrimSpace(
		s.sendInput.Value())
	if payReq == "" {
		s.inputError = "Paste a payment request"
		return s, nil
	}
	payReq = cleanPayReq(payReq)
	s.sendInput.SetValue(payReq)
	if !strings.HasPrefix(payReq, "lnbc") &&
		!strings.HasPrefix(payReq, "lntb") &&
		!strings.HasPrefix(payReq, "lnbcrt") {
		s.inputError =
			"Not a valid Lightning invoice"
		return s, nil
	}
	s.inputError = ""
	return s, decodePayReqCmd(
		s.ctx.LndClient, payReq)
}

// ── Confirm step ────────────────────────────────────────

func (s *SendScreen) handleConfirmKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.confirmBtnIdx > 0 {
			s.confirmBtnIdx--
		} else {
			return s, emitFocusSidebar
		}
		return s, nil
	case "right":
		if s.confirmBtnIdx < 1 {
			s.confirmBtnIdx++
		}
		return s, nil
	case "up":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab", "shift+tab":
		return s, nil
	case "backspace":
		s.backToInput()
		return s, nil
	case "enter":
		switch s.confirmBtnIdx {
		case 0: // Go Back
			s.backToInput()
			return s, nil
		case 1: // Confirm
			s.step = sendStepInFlight
			return s, sendPaymentCmd(
				s.ctx.LndClient,
				strings.TrimSpace(
					s.sendInput.Value()))
		}
	}
	return s, nil
}

// backToInput returns to the input step, clearing
// decoded state and error.
func (s *SendScreen) backToInput() {
	s.step = sendStepInput
	s.inputError = ""
	s.decodedAmt = 0
	s.decodedDesc = ""
	s.decodedDest = ""
	s.confirmBtnIdx = 0
	s.focusZone = sendZoneInput
	s.sendInput.Focus()
}

// ── InFlight step ───────────────────────────────────────

func (s *SendScreen) handleInFlightKey(
	keyStr string,
) (Screen, tea.Cmd) {
	return s, nil
}

// ── Result step ─────────────────────────────────────────

func (s *SendScreen) handleResultKey(
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

// ── Paste handling ──────────────────────────────────────

func (s *SendScreen) handlePaste(
	msg tea.PasteMsg,
) (Screen, tea.Cmd) {
	if s.step != sendStepInput {
		return s, nil
	}
	if s.focusZone != sendZoneInput {
		return s, nil
	}
	var cmd tea.Cmd
	s.sendInput, cmd =
		s.sendInput.Update(msg)
	return s, cmd
}

// ── Async message handlers ──────────────────────────────

func (s *SendScreen) handlePayReqDecoded(
	msg payReqDecodedMsg,
) (Screen, tea.Cmd) {
	if msg.err != nil {
		s.inputError = msg.err.Error()
		return s, nil
	}
	if msg.decoded.IsExpired {
		s.inputError = "This invoice has expired"
		return s, nil
	}
	s.decodedAmt = msg.decoded.AmountSats
	s.decodedDesc = msg.decoded.Description
	s.decodedDest = msg.decoded.Destination
	s.confirmBtnIdx = 1 // default to Confirm
	s.step = sendStepConfirm
	return s, nil
}

func (s *SendScreen) handleSendResult(
	msg sendPaymentResultMsg,
) (Screen, tea.Cmd) {
	if msg.err != nil {
		s.error = msg.err.Error()
	} else if msg.result.Status == "SUCCEEDED" {
		s.preimage = msg.result.Preimage
		s.feeSats = msg.result.FeeSats
		s.routeHops = msg.result.Hops
		s.error = ""
	} else {
		s.error = msg.result.Error
	}
	s.step = sendStepResult
	return s, nil
}

// ── Views ───────────────────────────────────────────────

func (s *SendScreen) viewInput(w, h int) string {
	p := newPane(w)
	p.title(theme.Header, "⚡ Send Payment")

	if !s.ctx.Cfg.HasLND() ||
		!s.ctx.Cfg.WalletExists() {
		p.dim("Create LND wallet to send.")
		return p.render()
	}
	if s.ctx.Status == nil ||
		!s.ctx.Status.lndResponding {
		p.dim("Waiting for LND...")
		return p.render()
	}

	var totalLocal int64
	for _, ch := range s.ctx.Status.channels {
		totalLocal += ch.LocalBalance
	}
	p.field("Spendable: ",
		formatSats(totalLocal)+" sats")
	p.blank()

	isFocused := s.ctx.ContentFocused
	inputFocused := isFocused &&
		s.focusZone == sendZoneInput
	p.input("Payment Request:",
		s.sendInput.View(), inputFocused)
	p.blank()
	p.dim("Paste a bolt11 invoice")

	p.appendError(s.inputError)

	// ── Buttons pinned to bottom ──
	btnFocused := isFocused &&
		s.focusZone == sendZoneButtons
	return p.renderWithBottomButtons(
		[]string{"Clear", "Send"},
		s.inputBtnIdx, btnFocused, h)
}

func (s *SendScreen) viewConfirm(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Warning, "Confirm Payment")

	p.field("Amount:      ",
		formatSats(s.decodedAmt)+" sats")
	if s.decodedDesc != "" {
		p.field("Description: ", s.decodedDesc)
	}
	p.labelLine("Destination:")
	p.mono(s.decodedDest)
	p.blank()
	p.warn("Send " +
		formatSats(s.decodedAmt) + " sats?")

	// ── Buttons pinned to bottom ──
	btnFocused := s.ctx.ContentFocused
	return p.renderWithBottomButtons(
		[]string{"Go Back", "Confirm"},
		s.confirmBtnIdx, btnFocused, h)
}

func (s *SendScreen) viewInFlight(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Sending Payment...")
	p.line(" " + theme.Value.Render(
		"Routing "+formatSats(s.decodedAmt)+
			" sats"))
	p.blank()
	p.dim("May take up to 60 seconds over Tor.")
	return p.renderWithBottomButtons(
		[]string{"Sending..."}, 0, false, h)
}

func (s *SendScreen) viewResult(
	w, h int,
) string {
	p := newPane(w)

	if s.error != "" {
		p.title(theme.Warning, "Payment Failed")
		p.warnWrap(s.error)
	} else {
		p.title(theme.Success, "Payment Sent")
		p.field("Amount: ",
			formatSats(s.decodedAmt)+" sats")
		p.field("Fee:    ",
			formatSats(s.feeSats)+" sats")
		if s.preimage != "" {
			p.blank()
			p.labelLine("Preimage:")
			p.mono(s.preimage)
		}
		if len(s.routeHops) > 0 {
			p.blank()
			p.labelLine("Route:")
			p.line(renderRouteDiagram(
				s.routeHops, w))
		}
	}

	return p.renderWithBottomButtons(
		[]string{"Done"}, 0,
		s.ctx.ContentFocused, h)
}

// ── Helpbar bindings ────────────────────────────────────

func (s *SendScreen) inputBindings() []key.Binding {
	var binds []key.Binding
	switch s.focusZone {
	case sendZoneInput:
		binds = append(binds,
			kLeftRightCursor,
			kTabNext,
			bind("enter", "send", "enter"),
			kSidebar)
		if s.ctx.HasTabs {
			binds = append(binds, kShiftTabBar)
		}
	case sendZoneButtons:
		binds = append(binds,
			kLeftRightButtons, kEnter, kShiftTabInput,
			kBack)
	}
	binds = append(binds, kQuit)
	return binds
}
