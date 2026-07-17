package welcome

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ── SyncthingWebUIScreen ───────────────────────────────
// Shows Syncthing Web UI connection info: URL, user,
// password with show/hide toggle. Two buttons: Full URL
// and Show/Hide Password.

type SyncthingWebUIScreen struct {
	ctx         *ScreenContext
	btnIdx      int // 0=Full URL, 1=Show/Hide Password
	showSecrets bool
}

func NewSyncthingWebUIScreen(
	ctx *ScreenContext,
) *SyncthingWebUIScreen {
	return &SyncthingWebUIScreen{
		ctx: ctx,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *SyncthingWebUIScreen) Init() tea.Cmd {
	return nil
}

func (s *SyncthingWebUIScreen) HandleKey(
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
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		return s, nil
	case "backspace":
		return s, emitFocusParent
	case "enter":
		return s.handleEnter()
	}
	return s, nil
}

func (s *SyncthingWebUIScreen) handleEnter() (
	Screen, tea.Cmd,
) {
	switch s.btnIdx {
	case 0: // Full URL
		syncOnion := readOnion(
			paths.TorSyncthingHostname)
		if syncOnion != "" {
			url := "http://" + syncOnion + ":8384"
			return s, func() tea.Msg {
				return showFullURLMsg{URL: url}
			}
		}
	case 1: // Show/Hide Password
		s.showSecrets = !s.showSecrets
	}
	return s, nil
}

func (s *SyncthingWebUIScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	return s, nil
}

func (s *SyncthingWebUIScreen) View(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "↻ Syncthing Web UI")

	syncOnion := readOnion(paths.TorSyncthingHostname)
	if syncOnion == "" {
		p.warn("Tor address not available yet.")
		return p.renderWithBottomButtons(
			[]string{"Waiting..."}, 0, false, h)
	}

	url := "http://" + syncOnion + ":8384"
	if len(url) > w-4 {
		url = url[:w-7] + "..."
	}

	p.labelLine("URL:")
	if s.showSecrets {
		p.mono(url)
	}
	p.blank()
	p.monoField("User: ", "admin")

	if s.ctx.Cfg.SyncthingPassword != "" {
		if s.showSecrets {
			p.monoField("Pass: ",
				s.ctx.Cfg.SyncthingPassword)
		} else {
			p.line(" " +
				theme.Label.Render("Pass: ") +
				theme.Dim.Render("••••••••"))
		}
	}

	showLabel := "Show Password"
	if s.showSecrets {
		showLabel = "Hide Password"
	}

	return p.renderWithBottomButtons(
		[]string{"Full URL", showLabel},
		s.btnIdx, s.ctx.ContentFocused, h)
}

func (s *SyncthingWebUIScreen) HelpBindings() []key.Binding {
	return tabButtonBindings(s.ctx.HasTabs)
}
