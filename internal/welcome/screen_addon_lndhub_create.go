package welcome

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── LndHubCreateScreen steps ───────────────────────────

type hubCreateStep int

const (
	hubCreateStepInput    hubCreateStep = iota // name entry
	hubCreateStepCreating                      // waiting for API
	hubCreateStepCreated                       // result display
)

// ── Focus zones for input step ─────────────────────────

const (
	hubCreateZoneInput   = 0
	hubCreateZoneButtons = 1
)

// ── LndHubCreateScreen ─────────────────────────────────

type LndHubCreateScreen struct {
	ctx         *ScreenContext
	step        hubCreateStep
	nameInput   textinput.Model
	focusZone   int // 0=input, 1=buttons
	btnIdx      int
	lastAccount *installer.LndHubAccount
	accountName string // saved for display after creation
}

func NewLndHubCreateScreen(
	ctx *ScreenContext,
) *LndHubCreateScreen {
	return &LndHubCreateScreen{
		ctx:       ctx,
		step:      hubCreateStepInput,
		nameInput: newHubNameInput(),
	}
}

// ── Screen interface ────────────────────────────────────

func (s *LndHubCreateScreen) Init() tea.Cmd {
	return nil
}

func (s *LndHubCreateScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case hubCreateStepInput:
		return s.handleInputKey(keyStr, msg)
	case hubCreateStepCreating:
		return s.handleCreatingKey(keyStr)
	case hubCreateStepCreated:
		return s.handleCreatedKey(keyStr)
	}
	return s, nil
}

func (s *LndHubCreateScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if s.step == hubCreateStepInput &&
			s.focusZone == hubCreateZoneInput {
			var cmd tea.Cmd
			s.nameInput, cmd =
				s.nameInput.Update(msg)
			return s, cmd
		}
	case lndhubAccountCreatedMsg:
		if msg.err != nil {
			// Error — return to input step
			s.step = hubCreateStepInput
			s.focusZone = hubCreateZoneInput
			s.nameInput.Focus()
			return s, nil
		}
		if msg.account != nil {
			s.lastAccount = msg.account
			s.accountName = msg.label
			s.step = hubCreateStepCreated
			s.btnIdx = 0
		}
		return s, nil
	}
	return s, nil
}

func (s *LndHubCreateScreen) View(
	w, h int,
) string {
	switch s.step {
	case hubCreateStepInput:
		return s.viewInput(w, h)
	case hubCreateStepCreating:
		return s.viewCreating(w, h)
	case hubCreateStepCreated:
		return s.viewCreated(w, h)
	}
	return ""
}

func (s *LndHubCreateScreen) HelpBindings() []key.Binding {
	switch s.step {
	case hubCreateStepInput:
		return s.inputBindings()
	case hubCreateStepCreating:
		return s.creatingBindings()
	case hubCreateStepCreated:
		return s.createdBindings()
	}
	return nil
}

// ── Input step ─────────────────────────────────────────

func (s *LndHubCreateScreen) handleInputKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.focusZone == hubCreateZoneButtons {
			if s.btnIdx > 0 {
				s.btnIdx--
			}
			return s, nil
		}
		if s.nameInput.Value() != "" {
			var cmd tea.Cmd
			s.nameInput, cmd =
				s.nameInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusSidebar
	case "right":
		if s.focusZone == hubCreateZoneButtons {
			if s.btnIdx < 1 {
				s.btnIdx++
			}
			return s, nil
		}
		if s.nameInput.Value() != "" {
			var cmd tea.Cmd
			s.nameInput, cmd =
				s.nameInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil
	case "up":
		if s.focusZone == hubCreateZoneButtons {
			s.focusZone = hubCreateZoneInput
			s.nameInput.Focus()
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "shift+tab":
		if s.focusZone == hubCreateZoneButtons {
			s.focusZone = hubCreateZoneInput
			s.nameInput.Focus()
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		if s.focusZone == hubCreateZoneInput {
			s.focusZone = hubCreateZoneButtons
			s.btnIdx = 1 // default to Create
			s.nameInput.Blur()
		}
		return s, nil
	case "backspace":
		if s.focusZone == hubCreateZoneInput &&
			s.nameInput.Value() != "" {
			var cmd tea.Cmd
			s.nameInput, cmd =
				s.nameInput.Update(tea.Msg(msg))
			return s, cmd
		}
		// Clean backspace: does nothing when empty
		// or on buttons
		return s, nil
	case "enter":
		if s.focusZone == hubCreateZoneButtons {
			switch s.btnIdx {
			case 0: // Clear
				s.nameInput = newHubNameInput()
				s.focusZone = hubCreateZoneInput
				return s, nil
			case 1: // Create Account
				return s.submitCreate()
			}
			return s, nil
		}
		// Enter in input field → move to buttons
		s.focusZone = hubCreateZoneButtons
		s.btnIdx = 1 // focus on Create
		s.nameInput.Blur()
		return s, nil
	default:
		if s.focusZone == hubCreateZoneInput {
			var cmd tea.Cmd
			s.nameInput, cmd =
				s.nameInput.Update(tea.Msg(msg))
			return s, cmd
		}
	}
	return s, nil
}

func (s *LndHubCreateScreen) submitCreate() (
	Screen, tea.Cmd,
) {
	name := s.nameInput.Value()
	if name == "" {
		return s, nil
	}
	s.step = hubCreateStepCreating
	return s, createLndHubAccountWithLabelCmd(
		s.ctx.Cfg.LndHubAdminToken, name)
}

func (s *LndHubCreateScreen) viewInput(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Create New Account")

	p.dim("Create a custodial Lightning wallet account.")
	p.dim("The recipient will receive a login and")
	p.dim("password to connect via BlueWallet or Zeus.")
	p.blank()

	inputFocused := s.ctx.ContentFocused &&
		s.focusZone == hubCreateZoneInput
	p.input("Name:", s.nameInput.View(), inputFocused)
	p.blank()
	p.dim("Letters, numbers, spaces, hyphens")

	btnFocused := s.ctx.ContentFocused &&
		s.focusZone == hubCreateZoneButtons

	return p.renderWithBottomButtons(
		[]string{"Clear", "Create Account"},
		s.btnIdx, btnFocused, h)
}

func (s *LndHubCreateScreen) inputBindings() []key.Binding {
	var binds []key.Binding

	if s.focusZone == hubCreateZoneInput {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "create")),
			key.NewBinding(
				key.WithKeys("tab"),
				key.WithHelp("tab", "buttons")),
			kSidebar)
		if s.ctx.HasTabs {
			binds = append(binds,
				key.NewBinding(
					key.WithKeys("shift+tab"),
					key.WithHelp("⇧tab", "tab bar")))
		}
	} else {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "select")),
			key.NewBinding(
				key.WithKeys("left", "right"),
				key.WithHelp("←→", "buttons")),
			key.NewBinding(
				key.WithKeys("shift+tab"),
				key.WithHelp("⇧tab", "input")))
	}

	binds = append(binds, kQuit)
	return binds
}

// ── Creating step (waiting) ────────────────────────────

func (s *LndHubCreateScreen) handleCreatingKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	}
	return s, nil
}

func (s *LndHubCreateScreen) viewCreating(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Create New Account")

	p.dim("Creating account...")

	return p.renderWithBottomButtons(
		[]string{"Clear", "Create Account"},
		1, false, h)
}

func (s *LndHubCreateScreen) creatingBindings() []key.Binding {
	return []key.Binding{kQuit}
}

// ── Created step ───────────────────────────────────────

// createdButtons returns the button labels for the
// created step based on available connection methods.
func (s *LndHubCreateScreen) createdButtons() []string {
	hubOnion := readOnion(paths.TorLndHubHostname)
	hasClearnet := s.ctx.Cfg.P2PMode == "hybrid" &&
		s.ctx.Status != nil &&
		s.ctx.Status.publicIP != ""

	var btns []string
	if hubOnion != "" {
		btns = append(btns, "Show QR (Tor)")
	}
	if hasClearnet {
		btns = append(btns, "Show QR (Clearnet)")
	}
	btns = append(btns, "Done")
	return btns
}

func (s *LndHubCreateScreen) handleCreatedKey(
	keyStr string,
) (Screen, tea.Cmd) {
	buttons := s.createdButtons()
	maxBtn := len(buttons) - 1

	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.btnIdx > 0 {
			s.btnIdx--
		} else {
			return s, emitFocusSidebar
		}
		return s, nil
	case "right":
		if s.btnIdx < maxBtn {
			s.btnIdx++
		}
		return s, nil
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		return s, nil
	case "backspace":
		// Clean backspace: does nothing
		return s, nil
	case "enter":
		if s.btnIdx >= 0 && s.btnIdx < len(buttons) {
			label := buttons[s.btnIdx]
			switch label {
			case "Show QR (Tor)":
				if s.lastAccount == nil {
					return s, nil
				}
				hubOnion := readOnion(
					paths.TorLndHubHostname)
				if hubOnion == "" {
					return s, nil
				}
				url := fmt.Sprintf(
					"lndhub://%s:%s@http://%s:%s",
					s.lastAccount.Login,
					s.lastAccount.Password,
					hubOnion,
					paths.LndHubExternalPort)
				return s, func() tea.Msg {
					return showQRMsg{
						URL:   url,
						Label: "LndHub — Tor",
					}
				}
			case "Show QR (Clearnet)":
				if s.lastAccount == nil ||
					s.ctx.Status == nil ||
					s.ctx.Status.publicIP == "" {
					return s, nil
				}
				url := fmt.Sprintf(
					"lndhub://%s:%s@https://%s:%s",
					s.lastAccount.Login,
					s.lastAccount.Password,
					s.ctx.Status.publicIP,
					paths.LndHubExternalPort)
				return s, func() tea.Msg {
					return showQRMsg{
						URL:   url,
						Label: "LndHub — Clearnet",
					}
				}
			case "Done":
				return s, emitCloseTab
			}
		}
	}
	return s, nil
}

func (s *LndHubCreateScreen) viewCreated(
	w, h int,
) string {
	cfg := s.ctx.Cfg
	p := newPane(w)
	p.title(theme.Success,
		"Account created: "+s.accountName)

	if s.lastAccount != nil {
		hubOnion := readOnion(paths.TorLndHubHostname)
		if hubOnion != "" {
			p.labelLine("Tor:")
			tor := hubOnion + ":" +
				paths.LndHubExternalPort
			if len(tor) > w-4 {
				tor = tor[:w-7] + "..."
			}
			p.mono(tor)
		}
		if cfg.P2PMode == "hybrid" &&
			s.ctx.Status != nil &&
			s.ctx.Status.publicIP != "" {
			p.labelLine("Clearnet (HTTPS):")
			p.mono(s.ctx.Status.publicIP + ":" +
				paths.LndHubExternalPort)
		}
		p.blank()
		p.monoField("Login:    ",
			s.lastAccount.Login)
		p.monoField("Password: ",
			s.lastAccount.Password)
		p.blank()
		p.warn("Share with " +
			s.accountName +
			". Won't be shown again.")
	}

	buttons := s.createdButtons()

	return p.renderWithBottomButtons(
		buttons,
		s.btnIdx, s.ctx.ContentFocused, h)
}

func (s *LndHubCreateScreen) createdBindings() []key.Binding {
	var binds []key.Binding

	binds = append(binds,
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")))

	buttons := s.createdButtons()
	if len(buttons) > 1 {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left", "right"),
				key.WithHelp("←→", "buttons")))
	}

	binds = append(binds, kSidebar)

	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("shift+tab"),
				key.WithHelp("⇧tab", "tab bar")))
	}

	binds = append(binds, kQuit)
	return binds
}

// ── Command helpers ────────────────────────────────────

func createLndHubAccountWithLabelCmd(
	adminToken string, label string,
) tea.Cmd {
	return func() tea.Msg {
		account, err := installer.CreateLndHubAccount(
			adminToken)
		return lndhubAccountCreatedMsg{
			account: account,
			label:   label,
			err:     err,
		}
	}
}

func lndhubDeactivateWithLoginCmd(
	login string,
) tea.Cmd {
	return func() tea.Msg {
		balance, _ := installer.GetUserBalance(login)
		err := installer.DeactivateUser(login)
		return lndhubDeactivatedMsg{
			login:   login,
			balance: balance,
			err:     err,
		}
	}
}
