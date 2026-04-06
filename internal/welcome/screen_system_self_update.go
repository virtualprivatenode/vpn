package welcome

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── SelfUpdateScreen ──────────────────────────────────
// Flow: confirm → install progress → done.
// Opens as a tab from SystemHomeScreen when the user
// presses Enter on the Update Node button.
//
// Simplest install flow — no config save, no rollback.
// Steps are idempotent (download + verify). The binary
// isn't replaced until the final step. Update takes
// effect on next SSH login.

type selfUpdateStep int

const (
	selfUpdateConfirm  selfUpdateStep = iota
	selfUpdateProgress                // delegated to InstallProgressScreen
)

type SelfUpdateScreen struct {
	ctx  *ScreenContext
	step selfUpdateStep

	// Confirm step — 0=Cancel, 1=Proceed
	btnIdx int

	// Progress step — embedded screen
	progress *InstallProgressScreen
}

func NewSelfUpdateScreen(
	ctx *ScreenContext,
) *SelfUpdateScreen {
	return &SelfUpdateScreen{
		ctx:    ctx,
		step:   selfUpdateConfirm,
		btnIdx: 1, // default focus on Proceed
	}
}

// ── Screen interface ────────────────────────────────────

func (s *SelfUpdateScreen) Init() tea.Cmd {
	return nil
}

func (s *SelfUpdateScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	if s.step == selfUpdateProgress &&
		s.progress != nil {
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

	// Confirm step
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
	case "backspace":
		return s, emitCloseTab
	case "enter":
		if s.btnIdx == 0 {
			return s, emitCloseTab
		}
		return s.startInstall()
	}
	return s, nil
}

func (s *SelfUpdateScreen) startInstall() (
	Screen, tea.Cmd,
) {
	steps := installer.SelfUpdateSteps(
		s.ctx.LatestVersion)

	s.progress = NewInstallProgressScreen(
		s.ctx, steps, s.onDone, nil)
	s.step = selfUpdateProgress
	return s, s.progress.Init()
}

func (s *SelfUpdateScreen) onDone() tea.Cmd {
	return emitRefreshStatus
}

func (s *SelfUpdateScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	if s.step == selfUpdateProgress &&
		s.progress != nil {
		newScreen, cmd := s.progress.HandleMsg(msg)
		s.progress =
			newScreen.(*InstallProgressScreen)
		return s, cmd
	}
	return s, nil
}

// ── View ────────────────────────────────────────────────

func (s *SelfUpdateScreen) View(
	w, h int,
) string {
	if s.step == selfUpdateProgress &&
		s.progress != nil {
		return s.progress.View(w, h)
	}
	return s.viewConfirm(w, h)
}

func (s *SelfUpdateScreen) viewConfirm(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused
	p := newPane(w)

	p.title(theme.Header,
		"Update Virtual Private Node")

	p.field("Current: ", "v"+s.ctx.Version)
	p.field("Latest:  ", "v"+s.ctx.LatestVersion)
	p.blank()
	p.line(" " + theme.Value.Render(
		"This will download and verify the"))
	p.line(" " + theme.Value.Render(
		"new binary using GPG signature and"))
	p.line(" " + theme.Value.Render(
		"SHA256 checksum."))
	p.blank()
	p.line(" " + theme.Value.Render(
		"All downloads are routed through Tor."))
	p.blank()
	p.dim("The update takes effect on next SSH login.")

	return p.renderWithBottomButtons(
		[]string{"Cancel", "Proceed"},
		s.btnIdx, isFocused, h)
}

// ── HelpBindings ────────────────────────────────────────

func (s *SelfUpdateScreen) HelpBindings() []key.Binding {
	if s.step == selfUpdateProgress &&
		s.progress != nil {
		return s.progress.HelpBindings()
	}

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
