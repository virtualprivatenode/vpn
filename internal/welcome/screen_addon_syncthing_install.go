package welcome

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── SyncthingInstallScreen ─────────────────────────────
// Flow: confirm → install progress → done.
// Opens as a tab from AddonsHomeScreen when Syncthing is
// not installed and the user presses Enter.

type syncInstallStep int

const (
	syncInstallConfirm  syncInstallStep = iota
	syncInstallProgress                 // delegated to InstallProgressScreen
)

type SyncthingInstallScreen struct {
	ctx  *ScreenContext
	step syncInstallStep

	// Confirm step — 0=Cancel, 1=Proceed
	btnIdx int

	// Progress step — embedded screen
	progress *InstallProgressScreen

	// Generated during confirm → progress transition
	syncPassword string
}

func NewSyncthingInstallScreen(
	ctx *ScreenContext,
) *SyncthingInstallScreen {
	return &SyncthingInstallScreen{
		ctx:    ctx,
		step:   syncInstallConfirm,
		btnIdx: 1, // default focus on Proceed
	}
}

// ── Screen interface ────────────────────────────────────

func (s *SyncthingInstallScreen) Init() tea.Cmd {
	return nil
}

func (s *SyncthingInstallScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	if s.step == syncInstallProgress && s.progress != nil {
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

func (s *SyncthingInstallScreen) startInstall() (
	Screen, tea.Cmd,
) {
	cfg := s.ctx.Cfg

	// Set installed flag before building steps so Tor
	// and firewall configs include Syncthing services.
	cfg.SyncthingInstalled = true

	steps, password, err := installer.SyncthingInstallSteps(cfg)
	if err != nil {
		// Revert flag on error
		cfg.SyncthingInstalled = false
		return s, nil
	}
	s.syncPassword = password

	s.progress = NewInstallProgressScreen(
		s.ctx, steps, s.onInstallDone, s.onInstallFail)
	s.step = syncInstallProgress
	return s, s.progress.Init()
}

func (s *SyncthingInstallScreen) onInstallDone() tea.Cmd {
	return func() tea.Msg {
		cfg := s.ctx.Cfg
		cfg.SyncthingPassword = s.syncPassword
		config.Save(cfg)
		return refreshStatusMsg{}
	}
}

func (s *SyncthingInstallScreen) onInstallFail() tea.Cmd {
	return func() tea.Msg {
		cfg := s.ctx.Cfg
		cfg.SyncthingInstalled = false
		installer.RebuildTorConfig(cfg)
		installer.RestartTor()
		config.Save(cfg)
		return refreshStatusMsg{}
	}
}

func (s *SyncthingInstallScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	if s.step == syncInstallProgress && s.progress != nil {
		newScreen, cmd := s.progress.HandleMsg(msg)
		s.progress = newScreen.(*InstallProgressScreen)
		return s, cmd
	}
	return s, nil
}

// ── View ────────────────────────────────────────────────

func (s *SyncthingInstallScreen) View(
	w, h int,
) string {
	if s.step == syncInstallProgress && s.progress != nil {
		return s.progress.View(w, h)
	}

	isFocused := s.ctx.ContentFocused
	p := newPane(w)

	p.title(theme.Header, "Install Syncthing")
	p.line(" " + theme.Value.Render("This will:"))
	p.blank()
	p.line(" " + theme.Value.Render(
		"  * Install Syncthing from official repository"))
	p.line(" " + theme.Value.Render(
		"  * Open port 22000 for sync connections"))
	p.line(" " + theme.Value.Render(
		"  * Create Tor hidden service for web UI"))
	p.line(" " + theme.Value.Render(
		"  * Auto-configure LND channel backup sync"))
	p.line(" " + theme.Value.Render(
		"  * Restart Tor"))
	p.blank()
	p.dim("After install, pair your local Syncthing")
	p.dim("from the Syncthing details screen.")

	return p.renderWithBottomButtons(
		[]string{"Cancel", "Proceed"},
		s.btnIdx, isFocused, h)
}

// ── HelpBindings ────────────────────────────────────────

func (s *SyncthingInstallScreen) HelpBindings() []key.Binding {
	if s.step == syncInstallProgress && s.progress != nil {
		return s.progress.HelpBindings()
	}
	return actionButtonBindings(s.btnIdx, s.ctx.HasTabs)
}
