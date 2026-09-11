package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/loginpassword"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ChangePasswordScreen changes the fixed vpn operator login password.
// Application code owns the helper request; this screen owns its result.

type changePwStep int

const (
	changePwStepInput changePwStep = iota
	changePwStepWorking
	changePwStepResult
)

const (
	changePwZoneInputNew     = 1
	changePwZoneInputConfirm = 2
	changePwZoneButtons      = 0
)

type changePwDoneMsg struct {
	owner   *ChangePasswordScreen
	attempt uint64
	result  app.LoginPasswordResult
}
type closeLoginPasswordMsg struct {
	owner   *ChangePasswordScreen
	attempt uint64
}

func (s *ChangePasswordScreen) closeCommand() tea.Cmd {
	attempt := s.attempt
	return func() tea.Msg { return closeLoginPasswordMsg{owner: s, attempt: attempt} }
}

type ChangePasswordScreen struct {
	ctx       *ScreenContext
	step      changePwStep
	attempt   uint64
	newInput  textinput.Model
	confInput textinput.Model
	focusZone int
	btnIdx    int
	inputErr  string
	result    app.LoginPasswordResult
}

func NewChangePasswordScreen(
	ctx *ScreenContext,
) *ChangePasswordScreen {
	newIn := newUserPasswordInput()
	confIn := newUserPasswordInput()
	newIn.Focus()

	return &ChangePasswordScreen{
		ctx:       ctx,
		step:      changePwStepInput,
		newInput:  newIn,
		confInput: confIn,
		focusZone: changePwZoneInputNew,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *ChangePasswordScreen) Init() tea.Cmd { return nil }

func (s *ChangePasswordScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case changePwStepInput:
		return s.handleInputKey(keyStr, msg)
	case changePwStepWorking:
		switch keyStr {
		case "ctrl+c":
			return s, tea.Quit
		case "left":
			return s, emitFocusSidebar
		case "up", "shift+tab":
			if s.ctx.HasTabs {
				return s, emitFocusTabBar
			}
		}
	case changePwStepResult:
		return s.handleResultKey(keyStr)
	}
	return s, nil
}

func (s *ChangePasswordScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case changePwDoneMsg:
		if msg.owner != s || msg.attempt != s.attempt || s.step != changePwStepWorking {
			return s, nil
		}
		s.step = changePwStepResult
		s.result = msg.result
		return s, nil

	case tea.PasteMsg:
		if s.step != changePwStepInput {
			return s, nil
		}
		// Accept the password manager's single trailing newline, but never
		// silently normalize or truncate the password inside the masked input.
		value := strings.TrimSuffix(msg.Content, "\n")
		var input *textinput.Model
		switch s.focusZone {
		case changePwZoneInputNew:
			input = &s.newInput
		case changePwZoneInputConfirm:
			input = &s.confInput
		default:
			return s, nil
		}
		input.SetValue(value)
		if input.Value() != value {
			input.Reset()
			s.inputErr = "Paste rejected and field cleared: unsupported characters or more than 256 characters"
		} else {
			s.inputErr = ""
		}
		return s, nil
	}
	return s, nil
}

func (s *ChangePasswordScreen) View(w, h int) string {
	switch s.step {
	case changePwStepInput:
		return s.viewInput(w, h)
	case changePwStepWorking:
		return s.viewWorking(w, h)
	case changePwStepResult:
		return s.viewResult(w, h)
	}
	return ""
}

func (s *ChangePasswordScreen) HelpBindings() []key.Binding {
	switch s.step {
	case changePwStepInput:
		var binds []key.Binding
		if s.focusZone == changePwZoneButtons {
			binds = append(binds, buttonNav(s.btnIdx)...)
			binds = append(binds,
				kEnter,
				bind("⇧tab", "fields", "shift+tab"),
				kBack)
		} else if s.focusZone == changePwZoneInputNew {
			binds = append(binds,
				kTabNextField,
				kEnterNext,
				kSidebar)
			if s.ctx.HasTabs {
				binds = append(binds, kShiftTabBar)
			}
		} else {
			binds = append(binds,
				kTabButtons,
				kEnterNext,
				bind("⇧tab", "prev field", "shift+tab"),
				kSidebar)
		}
		binds = append(binds, kQuit)
		return binds
	case changePwStepWorking:
		bindings := []key.Binding{kSidebar}
		if s.ctx.HasTabs {
			bindings = append(bindings, kShiftTabBar)
		}
		return append(bindings, kQuit)
	case changePwStepResult:
		return resultBindings(s.ctx.HasTabs)
	}
	return nil
}

// ── Input step ──────────────────────────────────────────

func (s *ChangePasswordScreen) handleInputKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit

	case "up":
		switch s.focusZone {
		case changePwZoneInputNew:
			if s.ctx.HasTabs {
				return s, emitFocusTabBar
			}
			return s, nil
		case changePwZoneInputConfirm:
			s.focusConfirm(false)
			s.focusNew(true)
			return s, nil
		case changePwZoneButtons:
			s.focusButtons(false)
			s.focusConfirm(true)
			return s, nil
		}
		return s, nil

	case "down":
		switch s.focusZone {
		case changePwZoneInputNew:
			s.focusNew(false)
			s.focusConfirm(true)
			return s, nil
		case changePwZoneInputConfirm:
			s.focusConfirm(false)
			s.focusButtons(true)
			s.btnIdx = 1
			return s, nil
		case changePwZoneButtons:
			return s, nil
		}
		return s, nil

	case "tab":
		switch s.focusZone {
		case changePwZoneInputNew:
			s.focusNew(false)
			s.focusConfirm(true)
		case changePwZoneInputConfirm:
			s.focusConfirm(false)
			s.focusButtons(true)
			s.btnIdx = 1
		}
		return s, nil

	case "shift+tab":
		switch s.focusZone {
		case changePwZoneButtons:
			s.focusButtons(false)
			s.focusConfirm(true)
		case changePwZoneInputConfirm:
			s.focusConfirm(false)
			s.focusNew(true)
		case changePwZoneInputNew:
			if s.ctx.HasTabs {
				return s, emitFocusTabBar
			}
		}
		return s, nil

	case "left":
		if s.focusZone == changePwZoneButtons &&
			s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		if s.isOnInput() {
			return s.routeKeyToInput(msg)
		}
		return s, emitFocusSidebar

	case "right":
		if s.focusZone == changePwZoneButtons &&
			s.btnIdx < 1 {
			s.btnIdx++
			return s, nil
		}
		if s.isOnInput() {
			return s.routeKeyToInput(msg)
		}
		return s, nil

	case "backspace":
		if s.isOnInput() {
			return s.routeKeyToInput(msg)
		}
		return s, emitFocusParent

	case "enter":
		if s.focusZone == changePwZoneButtons {
			switch s.btnIdx {
			case 0: // Cancel
				return s, s.closeCommand()
			case 1: // Change
				return s.submit()
			}
			return s, nil
		}
		// Enter from new field → confirm field
		if s.focusZone == changePwZoneInputNew {
			s.focusNew(false)
			s.focusConfirm(true)
			return s, nil
		}
		// Enter from confirm field → buttons
		s.focusConfirm(false)
		s.focusButtons(true)
		s.btnIdx = 1
		return s, nil

	default:
		if s.isOnInput() {
			return s.routeKeyToInput(msg)
		}
	}
	return s, nil
}

func (s *ChangePasswordScreen) submit() (Screen, tea.Cmd) {
	if s.step != changePwStepInput {
		return s, nil
	}
	newPw := s.newInput.Value()
	confPw := s.confInput.Value()

	if newPw == "" || confPw == "" {
		s.inputErr = "Both fields are required"
		return s, nil
	}
	if newPw != confPw {
		s.inputErr = "Passwords do not match"
		return s, nil
	}
	// Share password validation with installation and the helper boundary.
	pw, err := loginpassword.New(newPw)
	if err != nil {
		s.inputErr = err.Error()
		return s, nil
	}

	s.inputErr = ""
	s.step = changePwStepWorking
	s.attempt++
	attempt := s.attempt
	results := s.ctx.loginPasswords().Change(pw)
	s.newInput.Reset()
	s.confInput.Reset()
	return s, func() tea.Msg {
		return changePwDoneMsg{owner: s, attempt: attempt, result: <-results}
	}
}

func (s *ChangePasswordScreen) viewInput(w, h int) string {
	p := newPane(w)
	p.title(theme.Header, "Change Login Password")
	p.blank()

	p.field("User:        ", paths.AdminUser)
	p.blank()

	p.dim("Use a password manager to generate and")
	p.dim("store a strong password. Save it there")
	p.dim("before submitting. This screen will not")
	p.dim("show it back to you.")
	p.blank()
	p.dim("Minimum length: " +
		strconv.Itoa(loginpassword.MinLength) +
		" bytes.")
	p.blank()

	isFocused := s.ctx.ContentFocused
	newFocused := isFocused &&
		s.focusZone == changePwZoneInputNew
	confFocused := isFocused &&
		s.focusZone == changePwZoneInputConfirm

	// Match the shared validation length without revealing the masked value.
	p.input("New Password:", s.newInput.View(), newFocused)
	if len(s.newInput.Value()) > 0 {
		p.dim(fmt.Sprintf("(%d bytes)",
			len(s.newInput.Value())))
	}
	p.input("Confirm:     ", s.confInput.View(), confFocused)
	if len(s.confInput.Value()) > 0 {
		p.dim(fmt.Sprintf("(%d bytes)",
			len(s.confInput.Value())))
	}

	p.appendError(s.inputErr)

	btnFocused := isFocused &&
		s.focusZone == changePwZoneButtons
	return p.renderWithBottomButtons(
		[]string{"Cancel", "Change"}, s.btnIdx,
		btnFocused, h)
}

// ── Working step ────────────────────────────────────────

func (s *ChangePasswordScreen) viewWorking(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Changing password...")
	p.blank()
	p.line(" " + theme.Value.Render("Working..."))
	p.dim("Closing the TUI does not cancel a change")
	p.dim("already accepted by the helper.")
	return p.renderWithBottomButtons(
		[]string{"Working..."}, 0, false, h)
}

// ── Result step ─────────────────────────────────────────

func (s *ChangePasswordScreen) handleResultKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "enter":
		return s, s.closeCommand()
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

func (s *ChangePasswordScreen) viewResult(
	w, h int,
) string {
	p := newPane(w)

	switch s.result.Outcome {
	case app.LoginPasswordChanged:
		p.title(theme.Success, "Password changed successfully")
		p.blank()
		p.dim("Make sure your password manager has")
		p.dim("the new value saved.")
	case app.LoginPasswordNotChanged:
		p.title(theme.Warning, "Password not changed")
		if s.result.Err != nil {
			p.warnWrap(s.result.Err.Error())
		}
	default:
		p.title(theme.Warning, "Password change not confirmed")
		p.warnWrap("The password may have changed, and the request may still complete. Keep this session open. Verify the helper has finished, then check access from another login or console before changing it again.")
		if s.result.Err != nil {
			p.appendError(s.result.Err.Error())
		}
	}

	return p.renderWithBottomButtons(
		[]string{"Done"}, 0,
		s.ctx.ContentFocused, h)
}

// ── Focus / input helpers ───────────────────────────────

func (s *ChangePasswordScreen) focusNew(on bool) {
	if on {
		s.focusZone = changePwZoneInputNew
		s.newInput.Focus()
	} else {
		s.newInput.Blur()
	}
}

func (s *ChangePasswordScreen) focusConfirm(on bool) {
	if on {
		s.focusZone = changePwZoneInputConfirm
		s.confInput.Focus()
	} else {
		s.confInput.Blur()
	}
}

func (s *ChangePasswordScreen) focusButtons(on bool) {
	if on {
		s.focusZone = changePwZoneButtons
	}
}

func (s *ChangePasswordScreen) isOnInput() bool {
	return s.focusZone == changePwZoneInputNew ||
		s.focusZone == changePwZoneInputConfirm
}

func (s *ChangePasswordScreen) routeKeyToInput(
	msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	var cmd tea.Cmd
	switch s.focusZone {
	case changePwZoneInputNew:
		s.newInput, cmd = s.newInput.Update(tea.Msg(msg))
	case changePwZoneInputConfirm:
		s.confInput, cmd =
			s.confInput.Update(tea.Msg(msg))
	}
	return s, cmd
}
