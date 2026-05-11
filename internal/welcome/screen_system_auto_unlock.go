package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── AutoUnlockScreen ──────────────────────────────────
// Standalone screen for configuring LND auto-unlock.
// Two modes, determined by current cfg.AutoUnlock state
// at construction:
//
//   enable mode  — cfg.AutoUnlock == false
//                  Two masked password inputs + Confirm
//                  button. SetupAutoUnlock writes the
//                  wallet password file and rewrites
//                  the LND service.
//
//   disable mode — cfg.AutoUnlock == true
//                  Single confirm screen. DisableAutoUnlock
//                  removes the password file and reverts
//                  the LND service to its initial form.
//
// Entry points:
//   1. 'u' hotkey on the LND service row in System home
//   2. Auto-launched after wallet creation completes
//      (stage 2; the wallet creation flow swaps its tab
//      into this screen via the walletCreatedMsg handler
//      in update.go)

type autoUnlockMode int

const (
	autoUnlockEnable  autoUnlockMode = iota // configure auto-unlock
	autoUnlockDisable                       // turn off existing auto-unlock
)

type autoUnlockState int

const (
	auState_form    autoUnlockState = iota // entering passwords / confirm
	auState_running                        // installer call in flight
	auState_doneOK                         // success — Done button
	auState_doneErr                        // failure — Done button + error
)

const (
	auZoneInput1  = 0 // password 1 input
	auZoneInput2  = 1 // password 2 input
	auZoneButtons = 2 // Cancel/Confirm buttons
)

// Messages emitted by the auto-unlock command runners.
// Both unique to this screen so they don't collide with
// any other async flow.
type autoUnlockSetupDoneMsg struct{ err error }
type autoUnlockDisableDoneMsg struct{ err error }

type AutoUnlockScreen struct {
	ctx  *ScreenContext
	mode autoUnlockMode

	// Form / interaction state
	state     autoUnlockState
	focusZone int
	btnIdx    int // 0 = Cancel/Skip, 1 = Confirm/Disable

	// Enable mode — two masked inputs
	pw1 textinput.Model
	pw2 textinput.Model

	// Inline error string (e.g. "Passwords do not match")
	errMsg string

	// Final result of installer call (after running)
	resultErr error
}

func NewAutoUnlockScreen(
	ctx *ScreenContext,
) *AutoUnlockScreen {
	mode := autoUnlockEnable
	if ctx.Cfg.AutoUnlock {
		mode = autoUnlockDisable
	}

	s := &AutoUnlockScreen{
		ctx:    ctx,
		mode:   mode,
		state:  auState_form,
		btnIdx: 1, // default focus on Confirm
	}

	if mode == autoUnlockEnable {
		s.pw1 = newAutoUnlockPwInput()
		s.pw2 = newAutoUnlockPwInput()
		s.focusZone = auZoneInput1
		s.pw1.Focus()
	} else {
		// Disable mode has no inputs; focus goes
		// straight to the buttons.
		s.focusZone = auZoneButtons
	}

	return s
}

func newAutoUnlockPwInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.CharLimit = 256
	ti.SetWidth(40)
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.Prompt = "  "
	applyInputStyles(&ti)
	return ti
}

// ── Screen interface ────────────────────────────────────

func (s *AutoUnlockScreen) Init() tea.Cmd {
	if s.mode == autoUnlockEnable {
		return s.pw1.Focus()
	}
	return nil
}

func (s *AutoUnlockScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	// Running state: block all keys (background work
	// in flight). Mirrors InstallProgressScreen behavior.
	if s.state == auState_running {
		if keyStr == "ctrl+c" {
			return s, tea.Quit
		}
		return s, nil
	}

	// Done states
	if s.state == auState_doneOK ||
		s.state == auState_doneErr {
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

	// Form state — split by mode
	if s.mode == autoUnlockDisable {
		return s.handleDisableKey(keyStr, msg)
	}
	return s.handleEnableKey(keyStr, msg)
}

// ── Disable mode key handling ───────────────────────────
// No inputs — focus is always on the buttons.

func (s *AutoUnlockScreen) handleDisableKey(
	keyStr string, msg tea.KeyPressMsg,
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
	case "up":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "enter":
		if s.btnIdx == 0 {
			return s, emitCloseTab
		}
		// Disable
		s.state = auState_running
		return s, disableAutoUnlockCmd()
	case "backspace":
		return s, emitFocusParent
	}
	return s, nil
}

// ── Enable mode key handling ────────────────────────────
// Three focus zones: pw1, pw2, buttons.

func (s *AutoUnlockScreen) handleEnableKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit

	case "left":
		// On buttons: move between buttons, or
		// to sidebar from leftmost button.
		if s.focusZone == auZoneButtons {
			if s.btnIdx > 0 {
				s.btnIdx--
				return s, nil
			}
			return s, emitFocusSidebar
		}
		// On inputs: pass through for cursor movement
		return s, s.passthroughInput(msg)

	case "right":
		if s.focusZone == auZoneButtons {
			if s.btnIdx < 1 {
				s.btnIdx++
			}
			return s, nil
		}
		return s, s.passthroughInput(msg)

	case "up":
		if s.focusZone == auZoneInput2 {
			s.focusZone = auZoneInput1
			s.pw2.Blur()
			s.pw1.Focus()
			return s, nil
		}
		if s.focusZone == auZoneButtons {
			s.focusZone = auZoneInput2
			s.pw2.Focus()
			return s, nil
		}
		// On pw1: go to tab bar if available
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "down", "tab":
		if s.focusZone == auZoneInput1 {
			s.focusZone = auZoneInput2
			s.pw1.Blur()
			s.pw2.Focus()
			return s, nil
		}
		if s.focusZone == auZoneInput2 {
			s.focusZone = auZoneButtons
			s.pw2.Blur()
			return s, nil
		}
		return s, nil

	case "shift+tab":
		if s.focusZone == auZoneButtons {
			s.focusZone = auZoneInput2
			s.pw2.Focus()
			return s, nil
		}
		if s.focusZone == auZoneInput2 {
			s.focusZone = auZoneInput1
			s.pw2.Blur()
			s.pw1.Focus()
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "backspace":
		// On inputs: delete characters
		if s.focusZone == auZoneInput1 ||
			s.focusZone == auZoneInput2 {
			return s, s.passthroughInput(msg)
		}
		// On buttons: navigate to parent
		return s, emitFocusParent

	case "enter":
		if s.focusZone == auZoneInput1 {
			// Advance to second password
			s.focusZone = auZoneInput2
			s.pw1.Blur()
			s.pw2.Focus()
			return s, nil
		}
		if s.focusZone == auZoneInput2 {
			// Advance to buttons
			s.focusZone = auZoneButtons
			s.pw2.Blur()
			return s, nil
		}
		// Buttons zone
		if s.btnIdx == 0 {
			return s, emitCloseTab
		}
		return s.tryConfirm()

	default:
		return s, s.passthroughInput(msg)
	}
}

// passthroughInput forwards a key press to whichever
// input currently has focus. Returns the resulting cmd.
// Also clears any existing error message — the user is
// editing, so the error is no longer current.
func (s *AutoUnlockScreen) passthroughInput(
	msg tea.KeyPressMsg,
) tea.Cmd {
	s.errMsg = ""
	var cmd tea.Cmd
	switch s.focusZone {
	case auZoneInput1:
		s.pw1, cmd = s.pw1.Update(msg)
	case auZoneInput2:
		s.pw2, cmd = s.pw2.Update(msg)
	}
	return cmd
}

// tryConfirm validates the two password inputs and, if
// they pass, kicks off the SetupAutoUnlock command. On
// validation failure, sets errMsg and refocuses the
// first input.
func (s *AutoUnlockScreen) tryConfirm() (
	Screen, tea.Cmd,
) {
	pw1 := s.pw1.Value()
	pw2 := s.pw2.Value()

	if pw1 == "" {
		s.errMsg = "Password cannot be empty"
		s.refocusFirstInput()
		return s, nil
	}
	if pw1 != pw2 {
		s.errMsg = "Passwords do not match"
		s.pw1.SetValue("")
		s.pw2.SetValue("")
		s.refocusFirstInput()
		return s, nil
	}

	s.state = auState_running
	return s, setupAutoUnlockCmd(pw1)
}

func (s *AutoUnlockScreen) refocusFirstInput() {
	s.focusZone = auZoneInput1
	s.pw2.Blur()
	s.pw1.Focus()
}

// ── HandleMsg ───────────────────────────────────────────

func (s *AutoUnlockScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case autoUnlockSetupDoneMsg:
		if m.err != nil {
			s.state = auState_doneErr
			s.resultErr = m.err
			return s, nil
		}
		s.ctx.Cfg.AutoUnlock = true
		s.state = auState_doneOK
		return s, func() tea.Msg {
			config.Save(s.ctx.Cfg)
			return refreshStatusMsg{}
		}
	case autoUnlockDisableDoneMsg:
		if m.err != nil {
			s.state = auState_doneErr
			s.resultErr = m.err
			return s, nil
		}
		s.ctx.Cfg.AutoUnlock = false
		s.state = auState_doneOK
		return s, func() tea.Msg {
			config.Save(s.ctx.Cfg)
			return refreshStatusMsg{}
		}
	case tea.PasteMsg:
		if s.mode != autoUnlockEnable ||
			s.state != auState_form {
			return s, nil
		}
		val := strings.TrimSuffix(
			string(m.Content), "\n")
		switch s.focusZone {
		case auZoneInput1:
			s.pw1.SetValue(val)
		case auZoneInput2:
			s.pw2.SetValue(val)
		}
		return s, nil
	}
	return s, nil
}

// ── tea.Cmd factories ───────────────────────────────────

func setupAutoUnlockCmd(password string) tea.Cmd {
	return func() tea.Msg {
		err := installer.SetupAutoUnlock(password)
		return autoUnlockSetupDoneMsg{err: err}
	}
}

func disableAutoUnlockCmd() tea.Cmd {
	return func() tea.Msg {
		err := installer.DisableAutoUnlock()
		return autoUnlockDisableDoneMsg{err: err}
	}
}

// ── View ────────────────────────────────────────────────

func (s *AutoUnlockScreen) View(
	w, h int,
) string {
	switch s.state {
	case auState_running:
		return s.viewRunning(w, h)
	case auState_doneOK:
		return s.viewDone(w, h)
	case auState_doneErr:
		return s.viewError(w, h)
	}
	if s.mode == autoUnlockDisable {
		return s.viewDisable(w, h)
	}
	return s.viewEnable(w, h)
}

func (s *AutoUnlockScreen) viewEnable(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused
	p := newPane(w)

	p.title(theme.Header,
		"Configure Auto-Unlock")

	p.line(" " + theme.Value.Render(
		"LND requires your wallet password to"))
	p.line(" " + theme.Value.Render(
		"unlock the wallet on every startup."))
	p.blank()
	p.line(" " + theme.Value.Render(
		"Auto-unlock stores your password in a"))
	p.line(" " + theme.Value.Render(
		"permission-locked file owned by the"))
	p.line(" " + theme.Value.Render(
		"LND service user, so LND can unlock"))
	p.line(" " + theme.Value.Render(
		"itself automatically after a reboot."))
	p.blank()
	p.line(" " + theme.Warning.Render(
		"If you are not an advanced user,"))
	p.line(" " + theme.Warning.Render(
		"configure auto-unlock now by typing"))
	p.line(" " + theme.Warning.Render(
		"in your password."))
	p.blank()
	p.line(" " + theme.Value.Render(
		"Enter the SAME password you used when"))
	p.line(" " + theme.Value.Render(
		"creating your wallet (NOT YOUR 24 WORD"))
	p.line(" " + theme.Value.Render(
		"SEED, not the optional seed passphrase)."))
	p.blank()

	p.input("Wallet password:",
		s.pw1.View(),
		isFocused && s.focusZone == auZoneInput1)
	if len(s.pw1.Value()) > 0 {
		p.dim(fmt.Sprintf("(%d chars)", len(s.pw1.Value())))
	}
	p.blank()
	p.input("Confirm password:",
		s.pw2.View(),
		isFocused && s.focusZone == auZoneInput2)
	if len(s.pw2.Value()) > 0 {
		p.dim(fmt.Sprintf("(%d chars)", len(s.pw2.Value())))
	}

	p.appendError(s.errMsg)

	return p.renderWithBottomButtons(
		[]string{"Skip", "Confirm"},
		s.btnIdx,
		isFocused && s.focusZone == auZoneButtons, h)
}

func (s *AutoUnlockScreen) viewDisable(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused
	p := newPane(w)

	p.title(theme.Header,
		"Disable Auto-Unlock")

	p.line(" " + theme.Value.Render(
		"Auto-unlock is currently enabled."))
	p.blank()
	p.line(" " + theme.Value.Render(
		"Disabling it will:"))
	p.line(" " + theme.Value.Render(
		"  • Remove the stored wallet password"))
	p.line(" " + theme.Value.Render(
		"  • Restart LND"))
	p.line(" " + theme.Value.Render(
		"  • Require manual unlock after every"))
	p.line(" " + theme.Value.Render(
		"    reboot (run: lncli unlock)"))
	p.blank()
	p.line(" " + theme.Warning.Render(
		"Until you unlock LND manually after a"))
	p.line(" " + theme.Warning.Render(
		"reboot, no Lightning operations will"))
	p.line(" " + theme.Warning.Render(
		"work."))

	return p.renderWithBottomButtons(
		[]string{"Cancel", "Disable"},
		s.btnIdx, isFocused, h)
}

func (s *AutoUnlockScreen) viewRunning(
	w, h int,
) string {
	p := newPane(w)
	if s.mode == autoUnlockDisable {
		p.title(theme.Header,
			"Disabling Auto-Unlock")
		p.blank()
		p.dim("Restarting LND...")
	} else {
		p.title(theme.Header,
			"Configuring Auto-Unlock")
		p.blank()
		p.dim("Writing password file and " +
			"restarting LND...")
	}
	return p.render()
}

func (s *AutoUnlockScreen) viewDone(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused
	p := newPane(w)

	if s.mode == autoUnlockDisable {
		p.title(theme.Header,
			"Auto-Unlock Disabled")
		p.line(" " + theme.Good.Render(
			"Auto-unlock has been turned off."))
		p.blank()
		p.line(" " + theme.Warning.Render(
			"LND is currently locked."))
		p.blank()
		p.line(" " + theme.Value.Render(
			"To bring LND back online, either"))
		p.line(" " + theme.Value.Render(
			"re-enable auto-unlock from this"))
		p.line(" " + theme.Value.Render(
			"screen, or run:"))
		p.mono("lncli unlock")
	} else {
		p.title(theme.Header,
			"Auto-Unlock Configured")
		p.line(" " + theme.Good.Render(
			"Your wallet will now unlock"))
		p.line(" " + theme.Good.Render(
			"automatically on every reboot."))
	}

	return p.renderWithBottomButtons(
		[]string{"Done"}, 0, isFocused, h)
}

func (s *AutoUnlockScreen) viewError(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused
	p := newPane(w)

	if s.mode == autoUnlockDisable {
		p.title(theme.Header,
			"Failed to Disable Auto-Unlock")
	} else {
		p.title(theme.Header,
			"Failed to Configure Auto-Unlock")
	}
	p.blank()
	if s.resultErr != nil {
		p.warnWrap(s.resultErr.Error())
	}

	return p.renderWithBottomButtons(
		[]string{"Done"}, 0, isFocused, h)
}

// ── HelpBindings ────────────────────────────────────────

func (s *AutoUnlockScreen) HelpBindings() []key.Binding {
	if s.state == auState_running {
		return []key.Binding{kQuit}
	}
	if s.state == auState_doneOK ||
		s.state == auState_doneErr {
		return resultBindings(s.ctx.HasTabs)
	}

	if s.mode == autoUnlockDisable {
		return s.disableButtonBindings()
	}
	return s.enableBindings()
}

func (s *AutoUnlockScreen) enableBindings() []key.Binding {
	if s.focusZone == auZoneInput1 ||
		s.focusZone == auZoneInput2 {
		return []key.Binding{
			kEnterNext,
			kTabNextField,
			bind("⇧tab", "prev field", "shift+tab"),
			kQuit,
		}
	}

	// Button zone
	binds := buttonNav(s.btnIdx)
	binds = append(binds,
		kEnter,
		bind("⇧tab", "fields", "shift+tab"))
	if s.ctx.HasTabs {
		binds = append(binds, kUpTabBar)
	}
	binds = append(binds, kBack, kQuit)
	return binds
}

func (s *AutoUnlockScreen) disableButtonBindings() []key.Binding {
	return actionButtonBindings(s.btnIdx, s.ctx.HasTabs)
}
