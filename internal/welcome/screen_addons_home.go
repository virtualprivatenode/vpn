package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── AddonsHomeScreen ──────────────────────────────────
// Section home for Add-ons. Single item (Syncthing).
// Enter opens a detail tab if installed, or triggers an
// install flow if LND is ready. Bottom half reserved for
// a future add-on.
//
// No async data — reads ctx.Cfg pointer directly for
// installed/enabled state.

type AddonsHomeScreen struct {
	ctx    *ScreenContext
	cursor int // 0=Syncthing (only item for now)
}

func NewAddonsHomeScreen(
	ctx *ScreenContext,
) *AddonsHomeScreen {
	return &AddonsHomeScreen{ctx: ctx}
}

// ── Screen interface ────────────────────────────────────

func (s *AddonsHomeScreen) Init() tea.Cmd {
	return nil
}

func (s *AddonsHomeScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		return s, emitFocusSidebar
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "backspace":
		return s, emitFocusSidebar
	case "enter", "right":
		return s.handleEnter()
	}
	return s, nil
}

func (s *AddonsHomeScreen) handleEnter() (
	Screen, tea.Cmd,
) {
	cfg := s.ctx.Cfg

	if cfg.SyncthingInstalled {
		screen := NewSyncthingDetailScreen(s.ctx)
		return s, func() tea.Msg {
			return openTabMsg{
				Kind:   tabSyncthing,
				Label:  "Syncthing",
				Screen: screen,
			}
		}
	}
	if cfg.HasLND() && cfg.WalletExists() {
		screen := NewSyncthingInstallScreen(s.ctx)
		return s, func() tea.Msg {
			return openTabMsg{
				Kind:   tabSyncthingInstall,
				Label:  "Installing",
				Screen: screen,
			}
		}
	}
	return s, nil
}

func (s *AddonsHomeScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	return s, nil
}

// ── View ────────────────────────────────────────────────

func (s *AddonsHomeScreen) View(
	w, h int,
) string {
	cfg := s.ctx.Cfg
	isFocused := s.ctx.ContentFocused

	titleNormal := theme.AddonTitleNormal
	titleActive := theme.AddonTitleActive
	sepStyle := theme.TableDim

	renderSection := func(
		icon, name, desc string,
		statusLine1, statusLine2 string,
		selected bool,
	) []string {
		ttl := titleNormal
		if selected {
			ttl = titleActive
		}

		marker := "  "
		if selected {
			marker =
				theme.NavActive.Render("▸") + " "
		}

		var lines []string
		lines = append(lines,
			marker+icon+" "+ttl.Render(name))
		lines = append(lines, "")
		lines = append(lines,
			"   "+theme.Dim.Render(desc))
		lines = append(lines, "")
		lines = append(lines,
			"   "+statusLine1)
		if statusLine2 != "" {
			lines = append(lines, "")
			lines = append(lines,
				"   "+statusLine2)
		}
		return lines
	}

	// ── Syncthing ──────────────────────────────
	syncSelected := isFocused && s.cursor == 0

	var syncStat1, syncStat2 string
	if cfg.SyncthingInstalled {
		syncStat1 = theme.GreenDot.Render("●") +
			" " + theme.Good.Render("Installed")
		syncStat2 = theme.Dim.Render(fmt.Sprintf(
			"%d paired",
			len(cfg.SyncthingDevices)))
	} else {
		syncStat1 = theme.RedDot.Render("●") +
			" " + theme.Dim.Render("Not installed")
		syncStat2 = ""
		if !cfg.WalletExists() {
			syncStat2 = theme.Warn.Render(
				"Requires LND wallet")
		}
	}

	syncLines := renderSection(
		"↻", "Syncthing",
		"Auto-backup LND channel state",
		syncStat1, syncStat2,
		syncSelected,
	)

	// ── Layout: two halves + divider ───────────
	bodyH := h
	if bodyH < 4 {
		bodyH = 4
	}

	topH := (bodyH - 1) / 2
	botH := bodyH - 1 - topH

	centerInHalf := func(
		content []string, halfH int,
	) []string {
		pad := (halfH - len(content)) / 2
		if pad < 0 {
			pad = 0
		}
		blank := ""
		var out []string
		for i := 0; i < pad; i++ {
			out = append(out, blank)
		}
		out = append(out, content...)
		for len(out) < halfH {
			out = append(out, blank)
		}
		return out
	}

	var lines []string

	// Top half: Syncthing
	lines = append(lines,
		centerInHalf(syncLines, topH)...)

	// Divider
	lines = append(lines,
		sepStyle.Render(
			strings.Repeat("─", w)))

	// Bottom half: reserved for future add-on
	for i := 0; i < botH; i++ {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// ── HelpBindings ────────────────────────────────────────

func (s *AddonsHomeScreen) HelpBindings() []key.Binding {
	binds := []key.Binding{
		kEnterOpen,
		bind("←/⌫", "sidebar", "left", "backspace"),
	}
	if s.ctx.HasTabs {
		binds = append(binds, kUpTabBar)
	}
	binds = append(binds, kQuit)
	return binds
}
