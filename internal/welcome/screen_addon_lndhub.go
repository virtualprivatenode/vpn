package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Focus zones ────────────────────────────────────────

const (
	hubManageZoneButtons = 0
	hubManageZoneList    = 1
)

// ── LndHubManageScreen ─────────────────────────────────
// Main LndHub manage tab: header with Create New Account
// button, scrollable account table below. Reads account
// list live through ctx.Cfg pointer.

type LndHubManageScreen struct {
	ctx       *ScreenContext
	focusZone int // 0=buttons, 1=account list
	btnIdx    int // button index (single button)
	cursor    int // position in account list
}

func NewLndHubManageScreen(
	ctx *ScreenContext,
) *LndHubManageScreen {
	return &LndHubManageScreen{
		ctx: ctx,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *LndHubManageScreen) Init() tea.Cmd {
	return nil
}

func (s *LndHubManageScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	accounts := s.ctx.Cfg.LndHubAccounts
	s.clampCursor()

	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.focusZone == hubManageZoneButtons &&
			s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		// Single button — no right navigation
		return s, nil
	case "up":
		if s.focusZone == hubManageZoneList {
			if s.cursor > 0 {
				s.cursor--
			} else {
				s.focusZone = hubManageZoneButtons
				s.btnIdx = 0
			}
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		if s.focusZone == hubManageZoneButtons {
			if len(accounts) > 0 {
				s.focusZone = hubManageZoneList
				s.cursor = 0
			}
			return s, nil
		}
		if s.focusZone == hubManageZoneList {
			if s.cursor < len(accounts)-1 {
				s.cursor++
			}
		}
		return s, nil
	case "shift+tab":
		if s.focusZone == hubManageZoneList {
			s.focusZone = hubManageZoneButtons
			s.btnIdx = 0
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "backspace":
		return s, emitCloseTab
	case "enter":
		return s.handleEnter()
	}
	return s, nil
}

func (s *LndHubManageScreen) handleEnter() (
	Screen, tea.Cmd,
) {
	if s.focusZone == hubManageZoneButtons {
		// Create New Account → open create tab
		screen := NewLndHubCreateScreen(s.ctx)
		return s, func() tea.Msg {
			return openTabMsg{
				Kind:   tabLndHubCreate,
				Label:  "Create Account",
				Screen: screen,
			}
		}
	}

	// Account list — open account detail
	accounts := s.ctx.Cfg.LndHubAccounts
	if s.cursor < len(accounts) {
		acct := accounts[s.cursor]
		label := acct.Label
		if len(label) > 17 {
			label = label[:17] + "..."
		}
		screen := NewLndHubAccountScreen(
			s.ctx, acct, s.cursor)
		idx := s.cursor
		return s, func() tea.Msg {
			return openTabMsg{
				Kind:        tabLndHubAccount,
				Label:       label,
				Index:       idx,
				Screen:      screen,
				FocusTabBar: true,
			}
		}
	}
	return s, nil
}

func (s *LndHubManageScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	return s, nil
}

func (s *LndHubManageScreen) View(
	w, h int,
) string {
	s.clampCursor()
	accounts := s.ctx.Cfg.LndHubAccounts
	isFocused := s.ctx.ContentFocused
	onButtons := isFocused &&
		s.focusZone == hubManageZoneButtons

	// ── Fixed header: title + button ────────────
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render(
				"LndHub Accounts"), w))
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		renderButtons(
			[]string{"Create New Account"},
			s.btnIdx, onButtons, w))
	headerLines = append(headerLines, "")

	headerH := len(headerLines)
	header := strings.Join(headerLines, "\n")

	// ── Scrollable body ─────────────────────────
	var midLines []string
	cursorLine := 0

	if len(accounts) == 0 {
		midLines = append(midLines,
			" "+theme.Dim.Render("No accounts yet"))
	} else {
		hdrStyle := theme.TableHeader
		sepStyle := theme.TableDim

		nameW := 18
		loginW := 16
		statusW := 12
		dateW := w - nameW - loginW - statusW - 6
		if dateW < 12 {
			dateW = 12
		}

		hdr := " " +
			hdrStyle.Render(pad("Name", nameW)) +
			hdrStyle.Render(pad("Login", loginW)) +
			hdrStyle.Render(
				pad("Status", statusW)) +
			hdrStyle.Render(
				fmt.Sprintf("%-*s", dateW, "Created"))
		midLines = append(midLines, hdr)
		midLines = append(midLines,
			" "+sepStyle.Render(
				strings.Repeat("─", w-2)))

		onList := isFocused &&
			s.focusZone == hubManageZoneList

		selStyle := lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true)

		tableStart := len(midLines)

		for i, a := range accounts {
			name := a.Label
			if len(name) > nameW-1 {
				name = name[:nameW-2] + ".."
			}
			nameStr := pad(name, nameW)

			login := a.Login
			if len(login) > loginW-1 {
				login = login[:loginW-2] + ".."
			}
			loginStr := pad(login, loginW)

			var statusStr string
			if a.Active {
				statusStr = pad("● active", statusW)
			} else {
				statusStr = pad("● off", statusW)
			}

			dateStr := fmt.Sprintf("%-*s",
				dateW, a.CreatedAt)

			isSelected := onList && s.cursor == i

			marker := " "
			if isSelected {
				marker = "▸"
				cursorLine = tableStart + i
				midLines = append(midLines,
					marker+
						selStyle.Render(nameStr)+
						selStyle.Render(loginStr)+
						selStyle.Render(statusStr)+
						selStyle.Render(dateStr))
			} else {
				var stRendered string
				if a.Active {
					stRendered = theme.Good.Render(
						statusStr)
				} else {
					stRendered = theme.Warn.Render(
						statusStr)
				}
				midLines = append(midLines,
					marker+
						theme.Value.Render(nameStr)+
						theme.Dim.Render(loginStr)+
						stRendered+
						theme.Dim.Render(dateStr))
			}
		}
	}

	midContent := strings.Join(midLines, "\n")

	// ── Viewport ────────────────────────────────
	vpH := h - headerH
	if vpH < 1 {
		vpH = 1
	}

	vpRendered := renderViewport(
		midContent, w, vpH, cursorLine,
		len(midLines),
		isFocused &&
			s.focusZone == hubManageZoneList)

	return header + "\n" + vpRendered
}

func (s *LndHubManageScreen) HelpBindings() []key.Binding {
	var binds []key.Binding

	if s.focusZone == hubManageZoneButtons {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "select")),
			kSidebar)
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("down"),
				key.WithHelp("↓", "accounts")))
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
				key.WithHelp("enter", "open")),
			key.NewBinding(
				key.WithKeys("up", "down"),
				key.WithHelp("↑↓", "navigate")),
			key.NewBinding(
				key.WithKeys("shift+tab"),
				key.WithHelp("⇧tab", "buttons")),
			kSidebar)
	}

	binds = append(binds, kQuit)
	return binds
}

// clampCursor ensures the cursor is within bounds after
// the account list may have changed.
func (s *LndHubManageScreen) clampCursor() {
	accounts := s.ctx.Cfg.LndHubAccounts
	if s.cursor >= len(accounts) {
		s.cursor = len(accounts) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
	if len(accounts) == 0 &&
		s.focusZone == hubManageZoneList {
		s.focusZone = hubManageZoneButtons
		s.btnIdx = 0
	}
}
