package welcome

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/system"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── P2PUpgradeScreen ──────────────────────────────────
// Flow: noIP error | confirm → install progress → done.
// Opens as a tab from SystemHomeScreen when the user
// presses 'p' on the LND service row in Tor-only mode.
//
// Three states:
//   p2pNoIP    — PublicIPv4 returned ""; show error, Done
//   p2pConfirm — privacy warning + typed PUBLISH MY IP
//   p2pProgress — delegated to InstallProgressScreen

type p2pStep int

const (
	p2pNoIP p2pStep = iota
	p2pConfirm
	p2pProgress
)

const (
	p2pZoneInput   = 0
	p2pZoneButtons = 1
)

type P2PUpgradeScreen struct {
	ctx  *ScreenContext
	step p2pStep

	// Confirm step
	input     textinput.Model
	focusZone int
	btnIdx    int // 0=Cancel, 1=Proceed

	// Progress step — embedded screen
	progress *InstallProgressScreen

	// Detected at construction
	publicIP string
}

func NewP2PUpgradeScreen(
	ctx *ScreenContext,
) *P2PUpgradeScreen {
	ip := system.PublicIPv4()

	s := &P2PUpgradeScreen{
		ctx:      ctx,
		publicIP: ip,
		btnIdx:   1, // default focus on Proceed
	}

	if ip == "" {
		s.step = p2pNoIP
		return s
	}

	s.step = p2pConfirm
	s.focusZone = p2pZoneInput
	s.input = newP2PConfirmInput()
	return s
}

func newP2PConfirmInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "PUBLISH MY IP"
	ti.CharLimit = 13
	ti.SetWidth(20)
	ti.Validate = validatePrintableASCII
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Focus()
	return ti
}

// ── Screen interface ────────────────────────────────────

func (s *P2PUpgradeScreen) Init() tea.Cmd {
	return s.input.Focus()
}

func (s *P2PUpgradeScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	// No-IP state: Done button only
	if s.step == p2pNoIP {
		switch keyStr {
		case "ctrl+c":
			return s, tea.Quit
		case "enter", "backspace":
			return s, emitCloseTab
		case "left":
			return s, emitFocusSidebar
		case "up":
			if s.ctx.HasTabs {
				return s, emitFocusTabBar
			}
		}
		return s, nil
	}

	// Progress state
	if s.step == p2pProgress && s.progress != nil {
		if !s.progress.done {
			// Block all keys during active install
			return s, nil
		}
		switch keyStr {
		case "left":
			return s, emitFocusSidebar
		case "up":
			if s.ctx.HasTabs {
				return s, emitFocusTabBar
			}
		}
		newScreen, cmd := s.progress.HandleKey(
			keyStr, msg)
		s.progress =
			newScreen.(*InstallProgressScreen)
		return s, cmd
	}

	// Confirm state
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "backspace":
		if s.focusZone == p2pZoneInput {
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return s, cmd
		}
		return s, emitCloseTab

	case "left":
		if s.focusZone == p2pZoneButtons {
			if s.btnIdx > 0 {
				s.btnIdx--
				return s, nil
			}
			return s, emitFocusSidebar
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, cmd

	case "right":
		if s.focusZone == p2pZoneButtons {
			if s.btnIdx < 1 {
				s.btnIdx++
			}
			return s, nil
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, cmd

	case "up":
		if s.focusZone == p2pZoneButtons {
			s.focusZone = p2pZoneInput
			s.input.Focus()
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "down", "tab":
		if s.focusZone == p2pZoneInput {
			s.focusZone = p2pZoneButtons
			s.input.Blur()
			return s, nil
		}
		return s, nil

	case "shift+tab":
		if s.focusZone == p2pZoneButtons {
			s.focusZone = p2pZoneInput
			s.input.Focus()
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "enter":
		if s.focusZone == p2pZoneInput {
			// Enter on text field → move to buttons
			s.focusZone = p2pZoneButtons
			s.input.Blur()
			return s, nil
		}
		// Button zone
		if s.btnIdx == 0 {
			return s, emitCloseTab
		}
		// Proceed — only if input matches exactly
		if s.input.Value() != "PUBLISH MY IP" {
			return s, nil
		}
		return s.startInstall()

	default:
		if s.focusZone == p2pZoneInput {
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return s, cmd
		}
	}
	return s, nil
}

func (s *P2PUpgradeScreen) startInstall() (
	Screen, tea.Cmd,
) {
	cfg := s.ctx.Cfg

	// Set hybrid before building steps so LND config
	// and firewall include clearnet listeners.
	cfg.P2PMode = "hybrid"

	steps := installer.P2PUpgradeSteps(
		cfg, s.publicIP)

	s.progress = NewInstallProgressScreen(
		s.ctx, steps,
		s.onInstallDone, s.onInstallFail)
	s.step = p2pProgress
	return s, s.progress.Init()
}

func (s *P2PUpgradeScreen) onInstallDone() tea.Cmd {
	return func() tea.Msg {
		config.Save(s.ctx.Cfg)
		// The P2P upgrade deletes and regenerates LND's
		// TLS cert. Our existing gRPC connection is now
		// stale — explicitly reconnect so the next
		// status poll succeeds immediately rather than
		// failing once and reconnecting on the cycle
		// after that.
		if s.ctx.LndClient != nil {
			s.ctx.LndClient.Reconnect()
		}
		return refreshStatusMsg{}
	}
}

func (s *P2PUpgradeScreen) onInstallFail() tea.Cmd {
	return func() tea.Msg {
		s.ctx.Cfg.P2PMode = "tor"
		config.Save(s.ctx.Cfg)
		return refreshStatusMsg{}
	}
}

func (s *P2PUpgradeScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	if s.step == p2pProgress && s.progress != nil {
		newScreen, cmd := s.progress.HandleMsg(msg)
		s.progress =
			newScreen.(*InstallProgressScreen)
		return s, cmd
	}
	return s, nil
}

// ── View ────────────────────────────────────────────────

func (s *P2PUpgradeScreen) View(
	w, h int,
) string {
	if s.step == p2pNoIP {
		return s.viewNoIP(w, h)
	}
	if s.step == p2pProgress && s.progress != nil {
		return s.progress.View(w, h)
	}
	return s.viewConfirm(w, h)
}

func (s *P2PUpgradeScreen) viewNoIP(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused
	p := newPane(w)

	p.title(theme.Header,
		"Cannot Detect Public IP")
	p.line(" " + theme.Value.Render(
		"Could not determine your server's"))
	p.line(" " + theme.Value.Render(
		"public IP address."))
	p.blank()
	p.line(" " + theme.Value.Render(
		"Hybrid mode requires a public IPv4"))
	p.line(" " + theme.Value.Render(
		"address."))

	return p.renderWithBottomButtons(
		[]string{"Done"}, 0, isFocused, h)
}

func (s *P2PUpgradeScreen) viewConfirm(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused
	cfg := s.ctx.Cfg
	p := newPane(w)

	p.title(theme.Header,
		"Upgrade to Clearnet + Tor (Hybrid P2P)")

	p.field("Server IP: ", s.publicIP)
	p.blank()
	p.line(" " + theme.Warn.Render(
		"This will permanently change your"+
			" node's privacy:"))
	p.blank()
	p.line(" " + theme.Value.Render(
		"  • Your server IP will be published to the"))
	p.line(" " + theme.Value.Render(
		"    Lightning Network gossip protocol"))
	p.line(" " + theme.Value.Render(
		"  • Every node on the network will"+
			" learn your IP"))
	p.line(" " + theme.Value.Render(
		"  • This links your IP to your Lightning"+
			" node identity"))
	p.line(" " + theme.Value.Render(
		"  • This CANNOT be undone — once published,"))
	p.line(" " + theme.Value.Render(
		"    your IP cannot be retracted from"+
			" network gossip"))
	p.blank()
	p.line(" " + theme.Value.Render(
		"It will also:"))
	p.blank()
	p.line(" " + theme.Value.Render(
		"  • Open ports 9735 and 8080"+
			" in the firewall"))
	p.line(" " + theme.Value.Render(
		"  • Allow Zeus to connect over clearnet"))
	p.line(" " + theme.Value.Render(
		"  • Restart LND with the new config"))
	if cfg.LndHubInstalled {
		p.line(" " + theme.Value.Render(
			"  • Install TLS proxy for"+
				" LndHub clearnet"))
		p.line(" " + theme.Value.Render(
			"  • Open port 3000 for encrypted"+
				" LndHub access"))
	}

	p.blank()
	p.input("Type PUBLISH MY IP to proceed:",
		s.input,
		isFocused &&
			s.focusZone == p2pZoneInput)

	return p.renderWithBottomButtons(
		[]string{"Cancel", "Proceed"},
		s.btnIdx,
		isFocused &&
			s.focusZone == p2pZoneButtons, h)
}

// ── HelpBindings ────────────────────────────────────────

func (s *P2PUpgradeScreen) HelpBindings() []key.Binding {
	if s.step == p2pNoIP {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "close")),
			kSidebar,
			kQuit,
		}
	}

	if s.step == p2pProgress && s.progress != nil {
		return s.progress.HelpBindings()
	}

	// Confirm step
	if s.focusZone == p2pZoneInput {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "buttons")),
			key.NewBinding(
				key.WithKeys("tab"),
				key.WithHelp("tab", "buttons")),
			kBack,
			kQuit,
		}
	}

	// Button zone
	var binds []key.Binding
	if s.btnIdx == 0 {
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
			key.WithHelp("enter", "select")),
		key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("⇧tab", "input")),
		kBack)
	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑", "tab bar")))
	}
	binds = append(binds, kQuit)
	return binds
}
