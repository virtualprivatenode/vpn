package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── AddonsHomeScreen ──────────────────────────────────
// Section home for Add-ons. Single focus zone: a
// two-item vertical list (Syncthing, LndHub). Enter
// opens a detail tab if the addon is installed, or
// triggers an install flow (opens install tab) if
// LND is ready.
//
// No async data — reads ctx.Cfg pointer directly for
// installed/enabled state.

type AddonsHomeScreen struct {
	ctx    *ScreenContext
	cursor int // 0=Syncthing, 1=LndHub
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
	case "left", "backspace":
		return s, emitFocusSidebar
	case "up":
		if s.cursor > 0 {
			s.cursor--
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		if s.cursor < 1 {
			s.cursor++
		}
		return s, nil
	case "shift+tab":
		// Step backward through cards first, then up
		// to the tab bar — matches the two-press
		// pattern used by every other home screen.
		if s.cursor > 0 {
			s.cursor--
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "enter":
		return s.handleEnter()
	}
	return s, nil
}

func (s *AddonsHomeScreen) handleEnter() (
	Screen, tea.Cmd,
) {
	cfg := s.ctx.Cfg

	switch s.cursor {
	case 0: // Syncthing
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
	case 1: // LndHub
		if cfg.LndHubInstalled {
			screen := NewLndHubManageScreen(s.ctx)
			return s, func() tea.Msg {
				return openTabMsg{
					Kind:   tabLndHub,
					Label:  "LndHub",
					Screen: screen,
				}
			}
		}
		if cfg.HasLND() && cfg.WalletExists() {
			// Block install during IBD — LndHub needs
			// a synced LND to bake its macaroon.
			if s.ctx.Status == nil ||
				!s.ctx.Status.btcSynced ||
				!s.ctx.Status.lndSyncedChain {
				return s, nil
			}
			screen := NewLndHubInstallScreen(s.ctx)
			return s, func() tea.Msg {
				return openTabMsg{
					Kind:   tabLndHubInstall,
					Label:  "Installing",
					Screen: screen,
				}
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
		"🔄", "Syncthing",
		"Auto-backup LND channel state",
		syncStat1, syncStat2,
		syncSelected,
	)

	// ── LndHub ─────────────────────────────────
	hubSelected := isFocused && s.cursor == 1

	var hubStat1, hubStat2 string
	if cfg.LndHubInstalled {
		activeCount := 0
		for _, a := range cfg.LndHubAccounts {
			if a.Active {
				activeCount++
			}
		}
		hubStat1 = theme.GreenDot.Render("●") +
			" " + theme.Good.Render("Installed")
		hubStat2 = theme.Dim.Render(
			fmt.Sprintf("%d active", activeCount))
	} else {
		hubStat1 = theme.RedDot.Render("●") +
			" " + theme.Dim.Render("Not installed")
		hubStat2 = ""
		if !cfg.WalletExists() {
			hubStat2 = theme.Warn.Render(
				"Requires LND wallet")
		} else if s.ctx.Status == nil ||
			!s.ctx.Status.btcSynced ||
			!s.ctx.Status.lndSyncedChain {
			hubStat2 = theme.Warn.Render(
				"Requires synced node")
		}
	}

	hubLines := renderSection(
		"⚡", "LndHub",
		"Lightning accounts for family & friends",
		hubStat1, hubStat2,
		hubSelected,
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

	// Bottom half: LndHub
	lines = append(lines,
		centerInHalf(hubLines, botH)...)

	return strings.Join(lines, "\n")
}

// ── HelpBindings ────────────────────────────────────────

func (s *AddonsHomeScreen) HelpBindings() []key.Binding {
	binds := []key.Binding{
		key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "select")),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "open")),
		kSidebar,
	}
	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑", "tab bar")))
	}
	binds = append(binds, kQuit)
	return binds
}
