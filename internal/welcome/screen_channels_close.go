package welcome

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Close screen steps ──────────────────────────────────

type closeStep int

const (
	closeStepType    closeStep = iota // cooperative / force
	closeStepConfirm                  // fee input + buttons
	closeStepClosing                  // broadcasting
	closeStepResult                   // success or error
)

// ── Type step focus zones ────────────────────────────────

const (
	closeTypeZoneOptions = 0
	closeTypeZoneButtons = 1
)

// ── Confirm focus zones ────────────────────────────────

const (
	closeZoneFee     = 0
	closeZoneButtons = 1
)

// ── ChannelCloseScreen ─────────────────────────────────

type ChannelCloseScreen struct {
	ctx  *ScreenContext
	step closeStep

	// Channel info (snapshot at creation)
	chanPoint string
	peerAlias string
	capacity  int64
	localBal  int64
	remoteBal int64

	// Type step
	typeIdx    int // 0=cooperative, 1=force
	typeBtnIdx int // 0=Go Back, 1=Confirm

	// Confirm step
	force         bool
	feeInput      AmountInput
	feeTiers      [4]feeTier
	focusZone     int // type step: 0=options, 1=buttons; confirm step: 0=fee, 1=buttons
	confirmBtnIdx int // 0=Go Back, 1=Close/Force Close
	inFlight      bool

	// Result step
	txid  string
	error string

	// Cancelled is set when the user presses Cancel
	// on the type step. The embedding screen checks
	// this after delegation to dismiss the close flow.
	Cancelled bool
}

func NewChannelCloseScreen(
	ctx *ScreenContext,
	chanPoint string,
	peerAlias string,
	capacity int64,
	localBal int64,
	remoteBal int64,
	feeTiers [4]feeTier,
) *ChannelCloseScreen {
	return &ChannelCloseScreen{
		ctx:       ctx,
		step:      closeStepType,
		chanPoint: chanPoint,
		peerAlias: peerAlias,
		capacity:  capacity,
		localBal:  localBal,
		remoteBal: remoteBal,
		feeTiers:  feeTiers,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *ChannelCloseScreen) Init() tea.Cmd {
	return nil
}

func (s *ChannelCloseScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case closeStepType:
		return s.handleTypeKey(keyStr)
	case closeStepConfirm:
		return s.handleConfirmKey(keyStr, msg)
	case closeStepClosing:
		return s.handleClosingKey(keyStr)
	case closeStepResult:
		return s.handleResultKey(keyStr)
	}
	return s, nil
}

func (s *ChannelCloseScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case closeChannelMsg:
		return s.handleCloseResult(msg)
	case feeTiersMsg:
		if msg.err == nil {
			s.feeTiers = msg.tiers
		}
		return s, nil
	case tea.PasteMsg:
		if s.step == closeStepConfirm &&
			!s.force &&
			s.focusZone == closeZoneFee {
			cmd := s.feeInput.Update(msg)
			return s, cmd
		}
		return s, nil
	}
	return s, nil
}

func (s *ChannelCloseScreen) View(
	w, h int,
) string {
	switch s.step {
	case closeStepType:
		return s.viewType(w, h)
	case closeStepConfirm:
		return s.viewConfirm(w, h)
	case closeStepClosing:
		return s.viewClosing(w, h)
	case closeStepResult:
		return s.viewResult(w, h)
	}
	return ""
}

func (s *ChannelCloseScreen) HelpBindings() []key.Binding {
	switch s.step {
	case closeStepType:
		return s.typeBindings()
	case closeStepConfirm:
		return s.confirmBindings()
	case closeStepClosing:
		return inFlightBindings()
	case closeStepResult:
		return resultBindings(s.ctx.HasTabs)
	}
	return nil
}

// ── Type step ──────────────────────────────────────────

func (s *ChannelCloseScreen) handleTypeKey(
	keyStr string,
) (Screen, tea.Cmd) {
	// Buttons zone
	if s.focusZone == closeTypeZoneButtons {
		return s.handleTypeBtnKey(keyStr)
	}

	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit

	case "left":
		return s, emitFocusSidebar

	case "up":
		if s.typeIdx > 0 {
			s.typeIdx--
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "down", "tab":
		if s.typeIdx < 1 {
			s.typeIdx++
			return s, nil
		}
		// At bottom of options — move to buttons
		s.focusZone = closeTypeZoneButtons
		s.typeBtnIdx = 1 // default to Confirm
		return s, nil

	case "shift+tab":
		if s.typeIdx > 0 {
			s.typeIdx--
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "backspace":
		s.Cancelled = true
		return s, nil

	case "enter":
		// Select type and move to buttons with
		// Confirm focused
		s.focusZone = closeTypeZoneButtons
		s.typeBtnIdx = 1
		return s, nil
	}
	return s, nil
}

func (s *ChannelCloseScreen) handleTypeBtnKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.typeBtnIdx > 0 {
			s.typeBtnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.typeBtnIdx < 1 {
			s.typeBtnIdx++
		}
		return s, nil
	case "up", "shift+tab":
		s.focusZone = closeTypeZoneOptions
		return s, nil
	case "down", "tab":
		return s, nil
	case "backspace":
		s.Cancelled = true
		return s, nil
	case "enter":
		if s.typeBtnIdx == 0 { // Go Back
			s.Cancelled = true
			return s, nil
		}
		// Confirm — advance to confirm step
		s.force = s.typeIdx == 1
		s.confirmBtnIdx = 0
		s.error = ""

		if !s.force {
			s.feeInput = NewFeeInput()
			if s.feeTiers[0].SatPerVB > 0 {
				s.feeInput.SetSats(
					int64(s.feeTiers[0].SatPerVB))
			}
			s.focusZone = closeZoneFee
		} else {
			s.focusZone = closeZoneButtons
		}

		s.step = closeStepConfirm
		return s, nil
	}
	return s, nil
}

// ── Confirm step ───────────────────────────────────────

func (s *ChannelCloseScreen) handleConfirmKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	// Force close: no fee input, buttons only
	if s.force {
		return s.handleConfirmBtnKey(keyStr)
	}

	// Cooperative close: fee input + buttons
	if s.focusZone == closeZoneFee {
		return s.handleConfirmFeeKey(keyStr, msg)
	}

	return s.handleConfirmBtnKey(keyStr)
}

func (s *ChannelCloseScreen) handleConfirmFeeKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit

	case "left":
		if !s.feeInput.Empty() {
			cmd := s.feeInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusSidebar

	case "right":
		cmd := s.feeInput.Update(tea.Msg(msg))
		return s, cmd

	case "backspace":
		cmd := s.feeInput.Update(tea.Msg(msg))
		return s, cmd

	case "down", "tab", "enter":
		// Advance to buttons (pattern 19)
		s.feeInput.Blur()
		s.focusZone = closeZoneButtons
		s.confirmBtnIdx = 1 // default to action
		return s, nil

	case "up", "shift+tab":
		if s.ctx.HasTabs {
			s.feeInput.Blur()
			return s, emitFocusTabBar
		}
		return s, nil

	default:
		cmd := s.feeInput.Update(tea.Msg(msg))
		return s, cmd
	}
}

func (s *ChannelCloseScreen) handleConfirmBtnKey(
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

	case "up", "shift+tab":
		if !s.force {
			// Go back to fee input
			s.focusZone = closeZoneFee
			s.feeInput.Focus()
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "down", "tab":
		return s, nil

	case "backspace":
		s.step = closeStepType
		s.focusZone = closeTypeZoneOptions
		s.error = ""
		return s, nil

	case "enter":
		switch s.confirmBtnIdx {
		case 0: // Go Back
			s.step = closeStepType
			s.focusZone = closeTypeZoneOptions
			s.error = ""
			return s, nil
		case 1: // Close / Force Close
			if s.inFlight {
				return s, nil
			}
			s.inFlight = true
			s.error = ""
			s.step = closeStepClosing

			var feeRate uint64
			if !s.force {
				r := s.feeInput.Sats()
				if r > 0 {
					feeRate = uint64(r)
				}
			}

			return s, closeChannelCmd(
				s.ctx.LndClient,
				s.chanPoint,
				s.force,
				feeRate)
		}
	}
	return s, nil
}

// ── Closing step ───────────────────────────────────────

func (s *ChannelCloseScreen) handleClosingKey(
	keyStr string,
) (Screen, tea.Cmd) {
	return s, nil
}

// ── Result step ────────────────────────────────────────

func (s *ChannelCloseScreen) handleResultKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "enter":
		return s, tea.Batch(
			emitCloseTab,
			emitRefreshStatus)
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

// ── Async message handler ──────────────────────────────

func (s *ChannelCloseScreen) handleCloseResult(
	msg closeChannelMsg,
) (Screen, tea.Cmd) {
	s.inFlight = false
	if msg.err != nil {
		s.error = msg.err.Error()
	} else {
		s.txid = msg.txid
		s.error = ""
	}
	s.step = closeStepResult
	return s, nil
}

// ── Views ──────────────────────────────────────────────

func (s *ChannelCloseScreen) viewType(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Close Channel")

	p.field("Peer:     ", s.peerAlias)
	p.field("Capacity: ",
		formatSats(s.capacity)+" sats")
	p.field("Local:    ",
		formatSats(s.localBal)+" sats")
	p.blank()

	isFocused := s.ctx.ContentFocused
	onOptions := isFocused &&
		s.focusZone == closeTypeZoneOptions
	onButtons := isFocused &&
		s.focusZone == closeTypeZoneButtons

	p.line(" " +
		theme.Header.Render("Close type:"))
	p.blank()

	coopPrefix := " "
	coopStyle := theme.Value
	if onOptions && s.typeIdx == 0 {
		coopPrefix = "▸"
		coopStyle = theme.Action
	} else if onButtons && s.typeIdx == 0 {
		coopPrefix = "●"
		coopStyle = theme.Action
	}
	p.line(fmt.Sprintf(" %s %s",
		coopPrefix,
		coopStyle.Render("Cooperative close")))
	p.line("   " + theme.Dim.Render(
		"Requires peer online. Funds available"+
			" immediately."))
	p.blank()

	forcePrefix := " "
	forceStyle := theme.Value
	if onOptions && s.typeIdx == 1 {
		forcePrefix = "▸"
		forceStyle = theme.Warning
	} else if onButtons && s.typeIdx == 1 {
		forcePrefix = "●"
		forceStyle = theme.Warning
	}
	p.line(fmt.Sprintf(" %s %s",
		forcePrefix,
		forceStyle.Render("Force close")))
	p.line("   " + theme.Dim.Render(
		"Unilateral. Funds locked ~2 weeks."))

	return p.renderWithBottomButtons(
		[]string{"Go Back", "Confirm"},
		s.typeBtnIdx, onButtons, h)
}

func (s *ChannelCloseScreen) viewConfirm(
	w, h int,
) string {
	p := newPane(w)

	if s.force {
		p.title(theme.Warning,
			"⚠ Force Close Channel")
	} else {
		p.title(theme.Warning, "Close Channel")
	}

	p.field("Peer:     ", s.peerAlias)
	p.field("Capacity: ",
		formatSats(s.capacity)+" sats")
	p.field("Your bal: ",
		formatSats(s.localBal)+" sats")
	p.blank()

	if s.force {
		p.warn("Force close will lock your funds")
		p.warn("for up to 2,016 blocks (~2 weeks).")
		p.warn("Use cooperative close when possible.")
		p.blank()
	} else {
		// Fee rate input
		isFeeZone := s.ctx.ContentFocused &&
			s.focusZone == closeZoneFee
		p.input("Fee rate (sat/vB):",
			s.feeInput.View(), isFeeZone)

		// Estimated total fee
		feeRate := s.feeInput.Sats()
		if feeRate > 0 {
			estFee := int64(feeRate * 170)
			p.line(" " + theme.Dim.Render(
				fmt.Sprintf("Est. fee: ~%s sats",
					formatSats(estFee))))
		}

		// Fee reference hints
		hints := formatFeeHints(s.feeTiers)
		if hints != "" {
			p.blank()
			p.dim(hints)
		}
		p.blank()
	}

	if s.force {
		p.warn("Force close this channel?")
	} else {
		p.warn("Close this channel cooperatively?")
	}

	p.appendError(s.error)

	// ── Buttons pinned to bottom ──
	isBtnFocused := s.ctx.ContentFocused &&
		s.focusZone == closeZoneButtons
	if s.force {
		isBtnFocused = s.ctx.ContentFocused
	}

	var labels []string
	if s.force {
		labels = []string{"Go Back", "Force Close"}
	} else {
		labels = []string{"Go Back", "Close Channel"}
	}

	return p.renderWithBottomButtons(
		labels, s.confirmBtnIdx, isBtnFocused, h)
}

func (s *ChannelCloseScreen) viewClosing(
	w, h int,
) string {
	p := newPane(w)
	if s.force {
		p.title(theme.Warning,
			"Force Closing Channel...")
	} else {
		p.title(theme.Header,
			"Closing Channel...")
	}
	p.line(" " + theme.Value.Render(
		"Broadcasting close transaction."))
	p.blank()
	p.dim("May take up to 2 minutes over Tor.")
	p.dim("Do not close the terminal.")

	return p.renderWithBottomButtons(
		[]string{"Closing..."}, 0, false, h)
}

func (s *ChannelCloseScreen) viewResult(
	w, h int,
) string {
	p := newPane(w)

	if s.error != "" {
		p.title(theme.Warning,
			"Channel Close Failed")
		p.warnWrap(s.error)
	} else {
		if s.force {
			p.title(theme.Warning,
				"Force Close Broadcast")
			p.line(" " + theme.Value.Render(
				"Force close transaction broadcast."))
			p.blank()
			p.warn("Funds locked for ~2,016 blocks" +
				" (~2 weeks).")
		} else {
			p.title(theme.Success,
				"Channel Closing")
			p.line(" " + theme.Value.Render(
				"Cooperative close broadcast."))
		}
		p.blank()
		p.field("Peer:   ", s.peerAlias)
		if s.txid != "" {
			p.blank()
			p.labelLine("Closing TX:")
			p.monoWrap(s.txid)
		}
	}

	return p.renderWithBottomButtons(
		[]string{"Done"}, 0,
		s.ctx.ContentFocused, h)
}

// ── Helpbar bindings ───────────────────────────────────

func (s *ChannelCloseScreen) typeBindings() []key.Binding {
	var binds []key.Binding
	if s.focusZone == closeTypeZoneButtons {
		binds = append(binds,
			kLeftRightButtons, kEnter,
			bind("↑", "back", "up"),
			kSidebar)
	} else {
		binds = append(binds,
			bind("↑↓", "close type", "up", "down"),
			kEnter, kSidebar)
		if s.ctx.HasTabs {
			binds = append(binds, kShiftTabBar)
		}
	}
	binds = append(binds, kBack, kQuit)
	return binds
}

func (s *ChannelCloseScreen) confirmBindings() []key.Binding {
	var binds []key.Binding
	if !s.force && s.focusZone == closeZoneFee {
		binds = append(binds,
			kTabNext, kEnterNext, kSidebar)
		if s.ctx.HasTabs {
			binds = append(binds, kShiftTabBar)
		}
	} else {
		binds = append(binds,
			kLeftRightButtons, kEnter)
		if !s.force {
			binds = append(binds, kShiftTabBack)
		} else {
			binds = append(binds, kSidebar)
			if s.ctx.HasTabs {
				binds = append(binds, kUpTabBar)
			}
		}
		binds = append(binds, kBack)
	}
	binds = append(binds, kQuit)
	return binds
}
