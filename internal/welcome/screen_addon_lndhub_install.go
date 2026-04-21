package welcome

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── LndHubInstallScreen ────────────────────────────────
// Flow: confirm → install progress → done.
// Opens as a tab from AddonsHomeScreen when LndHub is
// not installed and the user presses Enter.

type lndhubInstallStep int

const (
	lndhubInstallConfirm lndhubInstallStep = iota
	lndhubInstallProgress
)

type LndHubInstallScreen struct {
	ctx  *ScreenContext
	step lndhubInstallStep

	// Confirm step — 0=Cancel, 1=Proceed
	btnIdx int

	// Progress step — embedded screen
	progress *InstallProgressScreen

	// Generated during confirm → progress transition
	adminToken string
	dbPassword string
}

func NewLndHubInstallScreen(
	ctx *ScreenContext,
) *LndHubInstallScreen {
	return &LndHubInstallScreen{
		ctx:    ctx,
		step:   lndhubInstallConfirm,
		btnIdx: 1, // default focus on Proceed
	}
}

// ── Screen interface ────────────────────────────────────

func (s *LndHubInstallScreen) Init() tea.Cmd {
	return nil
}

func (s *LndHubInstallScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	if s.step == lndhubInstallProgress && s.progress != nil {
		if !s.progress.done {
			// Block all keys during active install —
			// no navigation, no quit.
			return s, nil
		}
		// After done/failed, allow navigation
		switch keyStr {
		case "left":
			return s, emitFocusSidebar
		case "up":
			if s.ctx.HasTabs {
				return s, emitFocusTabBar
			}
		}
		newScreen, cmd := s.progress.HandleKey(keyStr, msg)
		s.progress = newScreen.(*InstallProgressScreen)
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
	case "enter":
		if s.btnIdx == 0 {
			// Cancel
			return s, emitCloseTab
		}
		// Proceed — build steps and transition
		return s.startInstall()
	case "backspace":
		return s, emitFocusParent
	}
	return s, nil
}

func (s *LndHubInstallScreen) startInstall() (
	Screen, tea.Cmd,
) {
	cfg := s.ctx.Cfg

	// Set installed flag before building steps so Tor
	// and firewall configs include LndHub services.
	cfg.LndHubInstalled = true

	steps, adminToken, dbPassword, err :=
		installer.LndHubInstallSteps(cfg)
	if err != nil {
		cfg.LndHubInstalled = false
		return s, nil
	}
	s.adminToken = adminToken
	s.dbPassword = dbPassword

	s.progress = NewInstallProgressScreen(
		s.ctx, steps, s.onInstallDone, s.onInstallFail)
	s.step = lndhubInstallProgress
	return s, s.progress.Init()
}

func (s *LndHubInstallScreen) onInstallDone() tea.Cmd {
	return func() tea.Msg {
		cfg := s.ctx.Cfg
		cfg.LndHubAdminToken = s.adminToken
		cfg.LndHubDBPassword = s.dbPassword
		config.Save(cfg)
		return refreshStatusMsg{}
	}
}

func (s *LndHubInstallScreen) onInstallFail() tea.Cmd {
	return func() tea.Msg {
		cfg := s.ctx.Cfg
		cfg.LndHubInstalled = false
		installer.RebuildTorConfig(cfg)
		installer.RestartTor()
		config.Save(cfg)
		return refreshStatusMsg{}
	}
}

func (s *LndHubInstallScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	if s.step == lndhubInstallProgress && s.progress != nil {
		newScreen, cmd := s.progress.HandleMsg(msg)
		s.progress = newScreen.(*InstallProgressScreen)
		return s, cmd
	}
	return s, nil
}

// ── View ────────────────────────────────────────────────

func (s *LndHubInstallScreen) View(
	w, h int,
) string {
	if s.step == lndhubInstallProgress && s.progress != nil {
		return s.progress.View(w, h)
	}

	isFocused := s.ctx.ContentFocused
	p := newPane(w)

	p.title(theme.Header, "Install LndHub.go")
	p.line(" " + theme.Value.Render("This will:"))
	p.blank()
	p.line(" " + theme.Value.Render(
		"  * Install Go toolchain (for building from source)"))
	p.line(" " + theme.Value.Render(
		"  * Install PostgreSQL database"))
	p.line(" " + theme.Value.Render(
		"  * Clone and build LndHub.go v"+
			installer.LndHubVersionStr()))
	p.line(" " + theme.Value.Render(
		"  * Bake restricted LND macaroon"))
	p.line(" " + theme.Value.Render(
		"  * Create Tor hidden service"))
	p.line(" " + theme.Value.Render(
		"  * Create accounts for family/friends from TUI"))
	p.blank()
	p.dim("Accounts can be managed from the")
	p.dim("LndHub details screen after install.")

	return p.renderWithBottomButtons(
		[]string{"Cancel", "Proceed"},
		s.btnIdx, isFocused, h)
}

// ── HelpBindings ────────────────────────────────────────

func (s *LndHubInstallScreen) HelpBindings() []key.Binding {
	if s.step == lndhubInstallProgress && s.progress != nil {
		return s.progress.HelpBindings()
	}
	return actionButtonBindings(s.btnIdx, s.ctx.HasTabs)
}
