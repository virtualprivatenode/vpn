package welcome

import (
	"os/user"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── ChangePasswordScreen ───────────────────────────────
// Three-step flow opened as its own tab from
// SSHKeysScreen. Lets the operator change the login
// password for the user running the TUI.
//
// Targets the current real user (os/user.Current()), not
// a hardcoded username — this matches the "change MY
// password" expectation and avoids assuming the codebase
// always runs as ripsline. Refuses to operate if running
// as root (uid 0) so we never accidentally rewrite root's
// password from a misconfigured launch.

const minPasswordLen = 16

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

type changePwDoneMsg struct{ err error }

func setUserPasswordCmd(
	username, newPassword string,
) tea.Cmd {
	return func() tea.Msg {
		return changePwDoneMsg{
			err: installer.SetUserPassword(
				username, newPassword)}
	}
}

type ChangePasswordScreen struct {
	ctx       *ScreenContext
	step      changePwStep
	username  string
	loadErr   string
	newInput  textinput.Model
	confInput textinput.Model
	focusZone int
	btnIdx    int
	inputErr  string
	resultErr string
}

func NewChangePasswordScreen(
	ctx *ScreenContext,
) *ChangePasswordScreen {
	username, loadErr := currentUsername()

	newIn := newUserPasswordInput()
	confIn := newUserPasswordInput()
	newIn.Focus()

	return &ChangePasswordScreen{
		ctx:       ctx,
		step:      changePwStepInput,
		username:  username,
		loadErr:   loadErr,
		newInput:  newIn,
		confInput: confIn,
		focusZone: changePwZoneInputNew,
	}
}

// currentUsername returns the login name of the user
// running this process, or an error string if it can't
// be determined or if running as root.
func currentUsername() (string, string) {
	u, err := user.Current()
	if err != nil {
		return "", "cannot determine current user: " +
			err.Error()
	}
	if u.Uid == "0" {
		return "", "refusing to change root's password " +
			"— run the TUI as a normal user"
	}
	return u.Username, ""
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
		if keyStr == "ctrl+c" {
			return s, tea.Quit
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
		s.step = changePwStepResult
		if msg.err != nil {
			s.resultErr = msg.err.Error()
		} else {
			s.resultErr = ""
		}
		return s, nil

	case tea.PasteMsg:
		// Bracketed paste arrives as its own message
		// type (not a sequence of KeyPressMsg). Route
		// to whichever input is currently focused.
		// Strip a single trailing newline since password
		// managers often add one; embedded newlines are
		// rejected by SetUserPassword anyway.
		if s.step != changePwStepInput {
			return s, nil
		}
		val := strings.TrimSuffix(
			string(msg.Content), "\n")
		switch s.focusZone {
		case changePwZoneInputNew:
			s.newInput.SetValue(val)
		case changePwZoneInputConfirm:
			s.confInput.SetValue(val)
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
		} else {
			binds = append(binds,
				key.NewBinding(
					key.WithKeys("tab"),
					key.WithHelp("tab", "next field")),
				key.NewBinding(
					key.WithKeys("enter"),
					key.WithHelp("enter", "next")),
				kSidebar)
		}
		binds = append(binds, kQuit)
		return binds
	case changePwStepWorking:
		return []key.Binding{kQuit}
	case changePwStepResult:
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "done")),
			kQuit,
		}
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
		if s.isOnInput() && s.activeInputValue() != "" {
			return s.routeKeyToInput(msg)
		}
		// Empty input + backspace = cancel = close tab
		return s, emitCloseTab

	case "enter":
		if s.focusZone == changePwZoneButtons {
			switch s.btnIdx {
			case 0: // Cancel
				return s, emitCloseTab
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
	if s.loadErr != "" {
		s.inputErr = s.loadErr
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
	if len(newPw) < minPasswordLen {
		s.inputErr = "Password must be at least " +
			strconv.Itoa(minPasswordLen) + " characters"
		return s, nil
	}

	s.inputErr = ""
	s.step = changePwStepWorking
	return s, setUserPasswordCmd(s.username, newPw)
}

func (s *ChangePasswordScreen) viewInput(w, h int) string {
	p := newPane(w)
	p.title(theme.Header, "Change Login Password")
	p.blank()

	if s.loadErr != "" {
		p.warn(s.loadErr)
		return p.renderWithBottomButtons(
			[]string{"Cancel"}, 0,
			s.ctx.ContentFocused, h)
	}

	p.field("User:        ", s.username)
	p.blank()

	p.dim("Use a password manager to generate and")
	p.dim("store a strong password. Save it there")
	p.dim("before submitting — this screen will not")
	p.dim("show it back to you.")
	p.blank()
	p.dim("Minimum length: " + strconv.Itoa(minPasswordLen) +
		" characters.")
	p.blank()

	isFocused := s.ctx.ContentFocused
	newFocused := isFocused &&
		s.focusZone == changePwZoneInputNew
	confFocused := isFocused &&
		s.focusZone == changePwZoneInputConfirm

	p.input("New Password:", s.newInput, newFocused)
	p.input("Confirm:     ", s.confInput, confFocused)

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
	case "enter", "backspace":
		return s, emitCloseTab
	}
	return s, nil
}

func (s *ChangePasswordScreen) viewResult(
	w, h int,
) string {
	p := newPane(w)

	if s.resultErr != "" {
		p.title(theme.Warning, "Error")
		p.warnWrap(s.resultErr)
	} else {
		p.title(theme.Success,
			"Password changed successfully")
		p.blank()
		p.dim("Make sure your password manager has")
		p.dim("the new value saved.")
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

func (s *ChangePasswordScreen) activeInputValue() string {
	switch s.focusZone {
	case changePwZoneInputNew:
		return s.newInput.Value()
	case changePwZoneInputConfirm:
		return s.confInput.Value()
	}
	return ""
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
