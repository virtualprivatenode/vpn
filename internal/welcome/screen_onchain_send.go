package welcome

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"

	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── On-chain send screen steps ──────────────────────────

type ocSendStep int

const (
	ocStepAddr      ocSendStep = iota // address input
	ocStepAmount                      // amount + Max button
	ocStepLabel                       // label input
	ocStepFee                         // fee rate input
	ocStepButtons                     // Clear / Create Transaction
	ocStepConfirm                     // Go Back / Confirm & Broadcast
	ocStepBroadcast                   // in-flight
	ocStepResult                      // success or error
)

// ── OnChainSendScreen ───────────────────────────────────

type OnChainSendScreen struct {
	ctx   *ScreenContext
	ocCtx *OnChainContext
	step  ocSendStep

	// Inputs (steps 0–3)
	addrInput  textinput.Model
	amtInput   AmountInput
	labelInput textinput.Model
	feeInput   AmountInput
	sendAll    bool
	maxFocused bool // Max button highlighted on amount step

	// Buttons (step 4)
	sendBtnIdx int // 0=Clear, 1=Create Transaction

	// Validated values (set on confirm transition)
	addrVal  string
	amtVal   int64
	feeRate  int64
	labelVal string

	// Confirm (step 5)
	confirmBtnIdx int   // 0=Go Back, 1=Confirm & Broadcast
	confirmFee    int64 // precise fee from estimateTxFeeCmd

	// Result (step 7)
	txid  string
	error string
}

func NewOnChainSendScreen(
	ctx *ScreenContext,
	ocCtx *OnChainContext,
) *OnChainSendScreen {
	s := &OnChainSendScreen{
		ctx:        ctx,
		ocCtx:      ocCtx,
		step:       ocStepAddr,
		addrInput:  newOnChainAddrInput(),
		amtInput:   NewAmountInput(),
		labelInput: newOCSendLabelInput(),
		feeInput:   NewFeeInput(),
		sendBtnIdx: 1, // default to Create Transaction
	}
	return s
}

// ── Screen interface ────────────────────────────────────

func (s *OnChainSendScreen) Init() tea.Cmd {
	return fetchFeeTiersCmd(s.ctx.Cfg)
}

func (s *OnChainSendScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case ocStepAddr, ocStepAmount, ocStepLabel,
		ocStepFee, ocStepButtons:
		return s.handleInputKey(keyStr, msg)
	case ocStepConfirm:
		return s.handleConfirmKey(keyStr)
	case ocStepBroadcast:
		return s.handleBroadcastKey(keyStr)
	case ocStepResult:
		return s.handleResultKey(keyStr)
	}
	return s, nil
}

func (s *OnChainSendScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		return s.handlePaste(msg)
	case sendCoinsResultMsg:
		return s.handleSendCoinsResult(msg)
	case feeTiersMsg:
		return s.handleFeeTiers(msg)
	case feeEstimateMsg:
		return s.handleFeeEstimate(msg)
	}
	return s, nil
}

func (s *OnChainSendScreen) View(
	w, h int,
) string {
	switch s.step {
	case ocStepAddr, ocStepAmount, ocStepLabel,
		ocStepFee, ocStepButtons:
		return s.viewInput(w, h)
	case ocStepConfirm:
		return s.viewConfirm(w, h)
	case ocStepBroadcast:
		return s.viewBroadcast(w, h)
	case ocStepResult:
		return s.viewResult(w, h)
	}
	return ""
}

func (s *OnChainSendScreen) HelpBindings() []key.Binding {
	switch s.step {
	case ocStepAddr, ocStepAmount, ocStepLabel,
		ocStepFee:
		return s.inputFieldBindings()
	case ocStepButtons:
		return s.inputButtonBindings()
	case ocStepConfirm:
		return actionButtonBindings(
			s.confirmBtnIdx, s.ctx.HasTabs)
	case ocStepBroadcast:
		return inFlightBindings()
	case ocStepResult:
		return resultBindings(s.ctx.HasTabs)
	}
	return nil
}

// ── Input steps (0–4) key handling ──────────────────────

func (s *OnChainSendScreen) handleInputKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit

	case "left":
		return s.handleInputLeft(msg)

	case "right":
		return s.handleInputRight(msg)

	case "backspace":
		return s.handleInputBackspace(msg)

	case "up":
		return s.handleInputUp()

	case "down":
		return s.handleInputDown()

	case "tab":
		return s.handleInputTab()

	case "shift+tab":
		return s.handleInputShiftTab()

	case "enter":
		return s.handleInputEnter()

	default:
		return s.handleInputDefault(msg)
	}
}

func (s *OnChainSendScreen) handleInputLeft(
	msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	// Buttons: move left
	if s.step == ocStepButtons &&
		s.sendBtnIdx > 0 {
		s.sendBtnIdx--
		return s, nil
	}
	// Addr input: pass through for cursor
	if s.step == ocStepAddr {
		if s.addrInput.Value() != "" {
			var cmd tea.Cmd
			s.addrInput, cmd =
				s.addrInput.Update(tea.Msg(msg))
			return s, cmd
		}
	}
	// Amount step: Max focused → back to input
	if s.step == ocStepAmount && s.maxFocused {
		s.maxFocused = false
		s.amtInput.Focus()
		return s, nil
	}
	// Amount input: pass through for cursor
	if s.step == ocStepAmount {
		if !s.amtInput.Empty() {
			cmd := s.amtInput.Update(tea.Msg(msg))
			return s, cmd
		}
	}
	// Label input: pass through for cursor
	if s.step == ocStepLabel {
		if s.labelInput.Value() != "" {
			var cmd tea.Cmd
			s.labelInput, cmd =
				s.labelInput.Update(tea.Msg(msg))
			return s, cmd
		}
	}
	// Fee input: pass through for cursor
	if s.step == ocStepFee {
		if !s.feeInput.Empty() {
			cmd := s.feeInput.Update(tea.Msg(msg))
			return s, cmd
		}
	}
	return s, emitFocusSidebar
}

func (s *OnChainSendScreen) handleInputRight(
	msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	// Buttons: move right
	if s.step == ocStepButtons &&
		s.sendBtnIdx < 1 {
		s.sendBtnIdx++
		return s, nil
	}
	// Addr input: pass through for cursor
	if s.step == ocStepAddr {
		var cmd tea.Cmd
		s.addrInput, cmd =
			s.addrInput.Update(tea.Msg(msg))
		return s, cmd
	}
	// Amount step: cursor inside value passes
	// through. At end of value (or empty), jump
	// to Max button (two-step preserved).
	if s.step == ocStepAmount && !s.maxFocused {
		if s.amtInput.Empty() ||
			s.amtInput.CursorAtEnd() {
			s.maxFocused = true
			s.amtInput.Blur()
			return s, nil
		}
		cmd := s.amtInput.Update(tea.Msg(msg))
		return s, cmd
	}
	// Label input: pass through for cursor
	if s.step == ocStepLabel {
		var cmd tea.Cmd
		s.labelInput, cmd =
			s.labelInput.Update(tea.Msg(msg))
		return s, cmd
	}
	// Fee input: pass through for cursor
	if s.step == ocStepFee {
		cmd := s.feeInput.Update(tea.Msg(msg))
		return s, cmd
	}
	return s, nil
}

func (s *OnChainSendScreen) handleInputBackspace(
	msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case ocStepAddr:
		var cmd tea.Cmd
		s.addrInput, cmd =
			s.addrInput.Update(tea.Msg(msg))
		return s, cmd
	case ocStepAmount:
		if !s.maxFocused {
			s.disengageMax()
			cmd := s.amtInput.Update(tea.Msg(msg))
			return s, cmd
		}
		// Max button focused — non-input zone,
		// navigate to parent.
		return s, emitFocusParent
	case ocStepLabel:
		var cmd tea.Cmd
		s.labelInput, cmd =
			s.labelInput.Update(tea.Msg(msg))
		return s, cmd
	case ocStepFee:
		cmd := s.feeInput.Update(tea.Msg(msg))
		s.syncMaxIfEngaged()
		return s, cmd
	case ocStepButtons:
		// Non-input zone — navigate to parent.
		return s, emitFocusParent
	}
	return s, nil
}

func (s *OnChainSendScreen) handleInputUp() (
	Screen, tea.Cmd,
) {
	if s.step > ocStepAddr {
		s.step--
		s.maxFocused = false
		s.focusStep()
	} else if s.ctx.HasTabs {
		return s, emitFocusTabBar
	}
	return s, nil
}

func (s *OnChainSendScreen) handleInputDown() (
	Screen, tea.Cmd,
) {
	next := s.step + 1
	if next > ocStepButtons {
		return s, nil
	}
	s.step = next
	s.maxFocused = false
	s.focusStep()
	return s, nil
}

func (s *OnChainSendScreen) handleInputTab() (
	Screen, tea.Cmd,
) {
	// No lists to skip — same as down
	return s.handleInputDown()
}

func (s *OnChainSendScreen) handleInputShiftTab() (
	Screen, tea.Cmd,
) {
	// Express backward jump one step
	if s.step > ocStepAddr {
		s.step--
		s.maxFocused = false
		s.focusStep()
	} else if s.ctx.HasTabs {
		return s, emitFocusTabBar
	}
	return s, nil
}

func (s *OnChainSendScreen) handleInputEnter() (
	Screen, tea.Cmd,
) {
	// Amount step: Max focused → engage Max
	if s.step == ocStepAmount && s.maxFocused {
		s.applyMax()
		return s, nil
	}
	// Bottom buttons
	if s.step == ocStepButtons {
		switch s.sendBtnIdx {
		case 0: // Clear
			s.resetInputs()
			return s, nil
		case 1: // Create Transaction
			return s.validateAndConfirm()
		}
	}
	// Enter on any other step: advance to next
	next := s.step + 1
	if next > ocStepButtons {
		next = ocStepButtons
	}
	s.step = next
	s.maxFocused = false
	s.focusStep()
	return s, nil
}

func (s *OnChainSendScreen) handleInputDefault(
	msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case ocStepAddr:
		var cmd tea.Cmd
		s.addrInput, cmd =
			s.addrInput.Update(tea.Msg(msg))
		return s, cmd
	case ocStepAmount:
		if !s.maxFocused {
			s.disengageMax()
			cmd := s.amtInput.Update(tea.Msg(msg))
			return s, cmd
		}
	case ocStepLabel:
		var cmd tea.Cmd
		s.labelInput, cmd =
			s.labelInput.Update(tea.Msg(msg))
		return s, cmd
	case ocStepFee:
		cmd := s.feeInput.Update(tea.Msg(msg))
		s.syncMaxIfEngaged()
		return s, cmd
	}
	return s, nil
}

// ── Confirm step (step 5) ──────────────────────────────

func (s *OnChainSendScreen) handleConfirmKey(
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
		case 1: // Confirm & Broadcast
			s.error = ""
			s.step = ocStepBroadcast
			return s, sendCoinsCmd(
				s.ctx.LndClient,
				s.addrVal,
				s.amtVal,
				s.feeRate,
				s.sendAll,
				s.ocCtx.UtxoOutpoints)
		}
	}
	return s, nil
}

// ── Broadcast step (step 6) ────────────────────────────

func (s *OnChainSendScreen) handleBroadcastKey(
	keyStr string,
) (Screen, tea.Cmd) {
	return s, nil
}

// ── Result step (step 7) ───────────────────────────────

func (s *OnChainSendScreen) handleResultKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "enter":
		return s, tea.Batch(
			emitCloseTab,
			listUnspentCmd(s.ctx.LndClient),
			fetchOnChainTxCmd(s.ctx.LndClient),
			fetchStatus(s.ctx.Cfg, s.ctx.LndClient))
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

// ── Paste handling ─────────────────────────────────────

func (s *OnChainSendScreen) handlePaste(
	msg tea.PasteMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case ocStepAddr:
		var cmd tea.Cmd
		s.addrInput, cmd =
			s.addrInput.Update(msg)
		return s, cmd
	case ocStepAmount:
		if !s.maxFocused {
			s.disengageMax()
			cmd := s.amtInput.Update(msg)
			return s, cmd
		}
	case ocStepLabel:
		var cmd tea.Cmd
		s.labelInput, cmd =
			s.labelInput.Update(msg)
		return s, cmd
	case ocStepFee:
		cmd := s.feeInput.Update(msg)
		s.syncMaxIfEngaged()
		return s, cmd
	}
	return s, nil
}

// ── Async message handlers ─────────────────────────────

func (s *OnChainSendScreen) handleSendCoinsResult(
	msg sendCoinsResultMsg,
) (Screen, tea.Cmd) {
	if msg.err != nil {
		s.error = msg.err.Error()
	} else {
		s.txid = msg.txid
		s.error = ""
	}
	s.step = ocStepResult
	// On success: clear UTXO selection + apply label
	if msg.err == nil {
		cmds := []tea.Cmd{emitClearUtxoSelection}
		if s.labelVal != "" {
			cmds = append(cmds,
				labelTxCmd(s.ctx.LndClient,
					msg.txid, s.labelVal))
		}
		return s, tea.Batch(cmds...)
	}
	return s, nil
}

func (s *OnChainSendScreen) handleFeeTiers(
	msg feeTiersMsg,
) (Screen, tea.Cmd) {
	if msg.err != nil {
		return s, nil
	}
	// Pre-fill fee input if still empty
	if s.feeInput.Empty() &&
		msg.tiers[0].SatPerVB > 0 {
		s.feeInput.SetSats(
			int64(msg.tiers[0].SatPerVB))
	}
	s.syncMaxIfEngaged()
	return s, nil
}

func (s *OnChainSendScreen) handleFeeEstimate(
	msg feeEstimateMsg,
) (Screen, tea.Cmd) {
	if msg.err == nil {
		s.confirmFee = msg.feeSats
	} else {
		s.error = msg.err.Error()
	}
	return s, nil
}

// ── Internal helpers ───────────────────────────────────

// focusStep manages text input focus for the current
// step. Blurs all inputs, then focuses the active one.
func (s *OnChainSendScreen) focusStep() {
	s.addrInput.Blur()
	s.amtInput.Blur()
	s.labelInput.Blur()
	s.feeInput.Blur()
	s.maxFocused = false
	switch s.step {
	case ocStepAddr:
		s.addrInput.Focus()
	case ocStepAmount:
		s.amtInput.Focus()
	case ocStepLabel:
		s.labelInput.Focus()
	case ocStepFee:
		s.feeInput.Focus()
	}
}

// ── Max-family helpers (Sparrow model) ────────────────
//
// Max is a one-way engage: pressing the Max button sets
// sendAll and fills the amount. Typing or backspace
// disengages silently. Fee edits auto-sync the amount
// while engaged. The Max button is a no-op when already
// engaged.

// computeMaxAmount returns the max sendable amount in
// sats given the current fee rate and UTXO selection
// (or full wallet balance if no selection).
func (s *OnChainSendScreen) computeMaxAmount() int64 {
	feeRate := s.getFeeRate()
	if len(s.ocCtx.UtxoSelected) > 0 {
		numInputs := max(
			len(s.ocCtx.UtxoOutpoints), 1)
		estFee := estimateSimpleFee(
			numInputs, 1, feeRate)
		maxAmt := s.ocCtx.UtxoSelectedTotal - estFee
		if maxAmt < 0 {
			return 0
		}
		return maxAmt
	}
	if s.ctx.Status != nil &&
		s.ctx.Status.lndBalance != "" {
		bal := parseBalance(
			s.ctx.Status.lndBalance)
		numInputs := max(
			len(s.ocCtx.Utxos), 1)
		estFee := estimateSimpleFee(
			numInputs, 1, feeRate)
		maxAmt := bal - estFee
		if maxAmt < 0 {
			return 0
		}
		return maxAmt
	}
	return 0
}

// applyMax engages send-all mode. No-op when already
// engaged. Called from enter on the Max button.
func (s *OnChainSendScreen) applyMax() {
	if s.sendAll {
		return
	}
	s.sendAll = true
	s.amtInput.SetSats(s.computeMaxAmount())
}

// disengageMax exits send-all mode. No-op when not
// engaged. Called from typed-digit and backspace
// handlers — the keystroke itself goes through to
// AmountInput after this returns.
func (s *OnChainSendScreen) disengageMax() {
	if !s.sendAll {
		return
	}
	s.sendAll = false
}

// syncMaxIfEngaged recomputes the max amount when
// sendAll is active. No-op otherwise. Called after any
// fee-rate mutation so the amount field stays in sync.
func (s *OnChainSendScreen) syncMaxIfEngaged() {
	if !s.sendAll {
		return
	}
	s.amtInput.SetSats(s.computeMaxAmount())
}

// EngageMaxForSelection is the entry point called from
// OnChainHomeScreen.openSend when UTXOs are pre-selected.
// Sets sendAll and fills the amount from the selection
// total minus estimated fee.
func (s *OnChainSendScreen) EngageMaxForSelection() {
	s.sendAll = true
	s.amtInput.SetSats(s.computeMaxAmount())
}

// getFeeRate returns the current fee rate from the fee
// input, defaulting to 1 sat/vB.
func (s *OnChainSendScreen) getFeeRate() int64 {
	n := s.feeInput.Sats()
	if n < 1 {
		return 1
	}
	return n
}

// validateAndConfirm validates all fields and transitions
// to the confirm step.
func (s *OnChainSendScreen) validateAndConfirm() (
	Screen, tea.Cmd,
) {
	// Validate address
	addr := strings.TrimSpace(
		s.addrInput.Value())
	if addr == "" {
		s.error = "Enter an address"
		s.step = ocStepAddr
		s.focusStep()
		return s, nil
	}
	if !isValidOnChainAddr(addr, s.ctx.Cfg.Network) {
		s.error = "Invalid address"
		s.step = ocStepAddr
		s.focusStep()
		return s, nil
	}

	// Validate amount
	var amountSats int64
	if s.sendAll {
		amountSats = 0
		displayVal := s.amtInput.Sats()
		if displayVal > 0 {
			s.amtVal = displayVal
		}
	} else {
		if s.amtInput.Empty() {
			s.error = "Enter an amount"
			s.step = ocStepAmount
			s.focusStep()
			return s, nil
		}
		n := s.amtInput.Sats()
		if n < 546 {
			s.error =
				"Minimum 546 sats (dust limit)"
			s.step = ocStepAmount
			s.focusStep()
			return s, nil
		}
		amountSats = n
	}

	// Validate fee rate
	feeRateVal := s.feeInput.Sats()
	if s.feeInput.Empty() {
		s.error = "Enter a fee rate"
		s.step = ocStepFee
		s.focusStep()
		return s, nil
	}
	if feeRateVal < 1 {
		s.error = "Minimum 1 sat/vB"
		s.step = ocStepFee
		s.focusStep()
		return s, nil
	}

	s.addrVal = addr
	s.amtVal = amountSats
	s.feeRate = feeRateVal
	s.labelVal = strings.TrimSpace(
		s.labelInput.Value())
	s.error = ""
	s.confirmFee = 0
	s.confirmBtnIdx = 0
	s.step = ocStepConfirm

	if !s.sendAll && addr != "" {
		target := int32(1)
		return s, estimateTxFeeCmd(
			s.ctx.LndClient, addr,
			amountSats, target)
	}
	return s, nil
}

// backToInput returns to the input form, preserving
// entered text. Only clears validated state and error.
func (s *OnChainSendScreen) backToInput() {
	s.step = ocStepButtons
	s.error = ""
	s.confirmFee = 0
	s.confirmBtnIdx = 0
}

// resetInputs creates fresh inputs and resets all
// input-phase state.
func (s *OnChainSendScreen) resetInputs() {
	s.addrInput = newOnChainAddrInput()
	s.amtInput = NewAmountInput()
	s.labelInput = newOCSendLabelInput()
	s.feeInput = NewFeeInput()
	s.sendAll = false
	s.maxFocused = false
	s.step = ocStepAddr
	s.sendBtnIdx = 1
	s.confirmFee = 0
	s.confirmBtnIdx = 0
	s.addrVal = ""
	s.amtVal = 0
	s.feeRate = 0
	s.labelVal = ""
	s.error = ""
	// Re-fill fee from cached tiers
	if s.ocCtx.SendFeeTiers[0].SatPerVB > 0 {
		s.feeInput.SetSats(
			int64(s.ocCtx.SendFeeTiers[0].SatPerVB))
	}
}

// ── Views ──────────────────────────────────────────────

func (s *OnChainSendScreen) viewInput(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused

	var lines []string
	lines = append(lines, "")
	lines = append(lines, centerPad(
		theme.Header.Render("⛓ Send On-Chain"), w))
	lines = append(lines, "")

	// Balance
	onchain := "0"
	if s.ctx.Status != nil &&
		s.ctx.Status.lndBalance != "" {
		onchain = s.ctx.Status.lndBalance
	}
	lines = append(lines,
		" "+theme.Label.Render("Balance:  ")+
			theme.Value.Render(
				formatSats(parseBalance(onchain))+
					" sats"))
	lines = append(lines, "")

	// ── Address input (step 0) ──────────────────
	addrActive := isFocused &&
		s.step == ocStepAddr
	addrLabel := theme.Label
	addrMarker := " "
	if addrActive {
		addrLabel = theme.NavActive
		addrMarker = theme.NavActive.Render("▸")
	}
	lines = append(lines,
		" "+addrLabel.Render("To:"))
	lines = append(lines,
		addrMarker+" "+s.addrInput.View())
	lines = append(lines, "")

	// ── Amount input (step 1) ───────────────────
	amtActive := isFocused &&
		s.step == ocStepAmount
	amtLabel := theme.Label
	amtMarker := " "
	if amtActive {
		amtLabel = theme.NavActive
		amtMarker = theme.NavActive.Render("▸")
	}

	lines = append(lines,
		" "+amtLabel.Render("Amount (sats):"))
	maxStyle := theme.BtnNormal
	if amtActive && s.maxFocused {
		maxStyle = theme.BtnFocused
	}
	maxLabel := "Max"
	if n := len(s.ocCtx.UtxoSelected); n == 1 {
		maxLabel = "Max (1 UTXO selected)"
	} else if n > 1 {
		maxLabel = fmt.Sprintf(
			"Max (%d UTXOs selected)", n)
	}
	renderedMax := maxStyle.Render(maxLabel)
	leftPart := amtMarker + " " + s.amtInput.View()
	gap := w - lipgloss.Width(leftPart) -
		lipgloss.Width(renderedMax) - 2
	if gap < 2 {
		gap = 2
	}
	lines = append(lines,
		leftPart+strings.Repeat(" ", gap)+renderedMax)
	lines = append(lines, "")

	// ── Label input (step 2) ────────────────────
	lblActive := isFocused &&
		s.step == ocStepLabel
	lblLabel := theme.Label
	lblMarker := " "
	if lblActive {
		lblLabel = theme.NavActive
		lblMarker = theme.NavActive.Render("▸")
	}
	lines = append(lines,
		" "+lblLabel.Render("Label:"))
	lines = append(lines,
		lblMarker+" "+s.labelInput.View())
	lines = append(lines, "")

	// ── Fee rate input (step 3) ─────────────────
	feeActive := isFocused &&
		s.step == ocStepFee
	feeLabelStyle := theme.Label
	feeMarker := " "
	if feeActive {
		feeLabelStyle = theme.NavActive
		feeMarker = theme.NavActive.Render("▸")
	}
	lines = append(lines,
		" "+feeLabelStyle.Render("Fee Rate (sat/vB):"))
	lines = append(lines,
		feeMarker+" "+s.feeInput.View())

	// Friendly fee reference hints
	hints := formatFeeHints(s.ocCtx.SendFeeTiers)
	if hints != "" {
		lines = append(lines,
			"  "+theme.Dim.Render(hints))
	}
	lines = append(lines, "")

	// ── Transaction preview diagram ─────────────
	sendAmt := s.amtInput.Sats()
	feeRate := s.getFeeRate()
	showPreview := sendAmt > 0

	var diagLines []string
	if showPreview {
		diagOutpoints := s.ocCtx.UtxoOutpoints
		if len(diagOutpoints) == 0 && s.sendAll &&
			len(s.ocCtx.Utxos) > 0 {
			for _, u := range s.ocCtx.Utxos {
				diagOutpoints = append(diagOutpoints,
					fmt.Sprintf("%s:%d",
						u.Txid, u.Vout))
			}
		}

		numInputs := max(len(diagOutpoints), 1)
		numOutputs := 2
		if s.sendAll {
			numOutputs = 1
		}
		estFee := estimateSimpleFee(
			numInputs, numOutputs, feeRate)

		dispAmt := formatSats(sendAmt)

		var changeStr string
		if !s.sendAll {
			if len(s.ocCtx.UtxoSelected) > 0 {
				ch := s.ocCtx.UtxoSelectedTotal -
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
			s.addrInput.Value())
		diagInputs := buildDiagramInputs(
			diagOutpoints,
			s.ocCtx.Utxos,
			s.ocCtx.OnChainTxs)
		diagLines = renderTxDiagram(
			diagInputs, destAddr, dispAmt,
			changeStr, feeStr, s.sendAll, w)
	}

	// Error
	var errLines []string
	if s.error != "" {
		errLines = append(errLines, "")
		lineW := w - 4
		if lineW < 16 {
			lineW = 16
		}
		errText := s.error
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

	// ── Bottom buttons (step 4) ─────────────────
	btnFocused := isFocused &&
		s.step == ocStepButtons
	btnLine := renderButtons(
		[]string{"Clear", "Create Transaction"},
		s.sendBtnIdx, btnFocused, w)

	// ── Layout: form top, diagram centered in
	// remaining space, buttons pinned at bottom ──
	formH := len(lines)
	diagH := len(diagLines) + len(errLines)
	totalPad := h - formH - diagH - 1 // -1 for btn
	totalPad = max(totalPad, 2)

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

func (s *OnChainSendScreen) viewConfirm(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused

	var lines []string
	lines = append(lines, "")
	lines = append(lines, centerPad(
		theme.Warning.Render("Confirm On-Chain Send"),
		w))
	lines = append(lines, "")

	addr := s.addrVal
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
	case s.sendAll && s.amtVal > 0:
		lines = append(lines,
			" "+theme.Label.Render("Amount:   ")+
				theme.Value.Render(
					formatSats(s.amtVal)+
						" sats (max)"))
	case s.sendAll:
		lines = append(lines,
			" "+theme.Label.Render("Amount:   ")+
				theme.Value.Render("Send All"))
	default:
		lines = append(lines,
			" "+theme.Label.Render("Amount:   ")+
				theme.Value.Render(
					formatSats(s.amtVal)+
						" sats"))
	}
	if len(s.ocCtx.UtxoOutpoints) > 0 {
		lines = append(lines,
			" "+theme.Label.Render("Inputs:   ")+
				theme.Value.Render(
					fmt.Sprintf("%d selected UTXOs",
						len(s.ocCtx.UtxoSelected))))
	}
	if s.labelVal != "" {
		lines = append(lines,
			" "+theme.Label.Render("Label:    ")+
				theme.Value.Render(s.labelVal))
	}
	lines = append(lines,
		" "+theme.Label.Render("Fee Rate: ")+
			theme.Value.Render(
				fmt.Sprintf("%d sat/vB",
					s.feeRate)))
	if s.confirmFee > 0 {
		lines = append(lines,
			" "+theme.Label.Render("Est. Fee: ")+
				theme.Value.Render(
					formatSats(s.confirmFee)+
						" sats"))
		if !s.sendAll && s.amtVal > 0 {
			total := s.amtVal + s.confirmFee
			lines = append(lines,
				" "+theme.Label.Render("Total:    ")+
					theme.Value.Render(
						formatSats(total)+" sats"))
		}
	}
	lines = append(lines, "")

	// ── Diagram with real fee numbers ───────────
	var destAmt string
	if s.amtVal > 0 {
		destAmt = formatSats(s.amtVal)
	} else if s.sendAll {
		if len(s.ocCtx.UtxoSelected) > 0 {
			destAmt = formatSats(
				s.ocCtx.UtxoSelectedTotal)
		} else {
			destAmt = formatSats(
				parseBalance(
					s.ctx.Status.lndBalance))
		}
	} else {
		destAmt = "0"
	}

	var changeStr string
	if !s.sendAll && s.confirmFee > 0 &&
		len(s.ocCtx.UtxoSelected) > 0 {
		ch := s.ocCtx.UtxoSelectedTotal -
			s.amtVal - s.confirmFee
		if ch > 0 {
			changeStr = formatSats(ch)
		}
	}

	var feeStr string
	if s.confirmFee > 0 {
		feeStr = formatSats(s.confirmFee)
	} else {
		feeRate := s.getFeeRate()
		numInputs := max(
			len(s.ocCtx.UtxoOutpoints), 1)
		numOutputs := 2
		if s.sendAll {
			numOutputs = 1
		}
		feeStr = "~" + formatSats(
			estimateSimpleFee(
				numInputs, numOutputs, feeRate))
	}

	diagOutpoints := s.ocCtx.UtxoOutpoints
	if len(diagOutpoints) == 0 && s.sendAll &&
		len(s.ocCtx.Utxos) > 0 {
		for _, u := range s.ocCtx.Utxos {
			diagOutpoints = append(diagOutpoints,
				fmt.Sprintf("%s:%d",
					u.Txid, u.Vout))
		}
	}

	diagInputs := buildDiagramInputs(
		diagOutpoints,
		s.ocCtx.Utxos,
		s.ocCtx.OnChainTxs)
	diagLines := renderTxDiagram(
		diagInputs, s.addrVal, destAmt,
		changeStr, feeStr, s.sendAll, w)
	lines = append(lines, diagLines...)

	// Warning
	lines = append(lines, "")
	switch {
	case s.sendAll && len(s.ocCtx.UtxoOutpoints) > 0:
		lines = append(lines,
			" "+theme.Warning.Render(
				"Send all selected UTXOs?"))
	case s.sendAll:
		lines = append(lines,
			" "+theme.Warning.Render(
				"Send entire balance?"))
	default:
		lines = append(lines,
			" "+theme.Warning.Render(
				"Send "+formatSats(s.amtVal)+
					" sats?"))
	}

	// Error
	if s.error != "" {
		lines = append(lines, "")
		lineW := w - 4
		if lineW < 16 {
			lineW = 16
		}
		errText := s.error
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

	// ── Bottom buttons, pinned ──────────────────
	btnFocused := isFocused
	btnLine := renderButtons(
		[]string{"Go Back", "Confirm & Broadcast"},
		s.confirmBtnIdx, btnFocused, w)

	contentH := len(lines)
	padH := max(h-contentH-1, 1)
	for i := 0; i < padH; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, btnLine)

	return strings.Join(lines, "\n")
}

func (s *OnChainSendScreen) viewBroadcast(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Broadcasting...")
	p.line(" " + theme.Value.Render(
		"Sending transaction to the network."))
	p.blank()
	p.dim("Do not close the terminal.")
	return p.renderWithBottomButtons(
		[]string{"Broadcasting..."}, 0, false, h)
}

func (s *OnChainSendScreen) viewResult(
	w, h int,
) string {
	p := newPane(w)

	if s.error != "" {
		p.title(theme.Warning,
			"On-Chain Send Failed")
		p.warnWrap(s.error)
	} else {
		p.title(theme.Success,
			"Transaction Broadcast")
		if s.txid != "" {
			p.labelLine("TX ID:")
			p.monoWrap(s.txid)
		}
	}

	return p.renderWithBottomButtons(
		[]string{"Done"}, 0,
		s.ctx.ContentFocused, h)
}

// ── Helpbar bindings ───────────────────────────────────

func (s *OnChainSendScreen) inputFieldBindings() []key.Binding {
	binds := []key.Binding{
		kUpDownFields, kTabNext, kLeftRightCursor,
		bind("enter", "continue", "enter"),
		kSidebar,
	}
	if s.ctx.HasTabs {
		binds = append(binds, kShiftTabBar)
	}
	binds = append(binds, kQuit)
	return binds
}

func (s *OnChainSendScreen) inputButtonBindings() []key.Binding {
	binds := buttonNav(s.sendBtnIdx)
	binds = append(binds,
		kEnter, kShiftTabBack,
		bind("↑", "fields", "up"),
		kBack, kQuit)
	return binds
}

// ── Diagram helpers ────────────────────────────────────

// ── Transaction diagram ────────────────────────────────
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
func buildDiagramInputs(
	outpoints []string,
	utxos []lndrpc.UTXO,
	txs []lndrpc.OnChainTx,
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
		for _, u := range utxos {
			uOP := fmt.Sprintf("%s:%d", u.Txid, u.Vout)
			if uOP == op {
				inp.amt = formatSats(u.AmountSats)
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
		for _, tx := range txs {
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
