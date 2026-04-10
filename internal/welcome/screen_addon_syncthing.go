package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── SyncthingDetailScreen ──────────────────────────────
// Main Syncthing manage tab: header with Pair Device /
// Web UI buttons, scrollable paired device table below.
// Reads device list live through ctx.Cfg pointer.

const (
	syncDetailZoneButtons = 0
	syncDetailZoneList    = 1
)

type SyncthingDetailScreen struct {
	ctx       *ScreenContext
	btnIdx    int // 0=Pair Device, 1=Web UI
	focusZone int // 0=buttons, 1=device list
	cursor    int // position in device list
}

func NewSyncthingDetailScreen(
	ctx *ScreenContext,
) *SyncthingDetailScreen {
	return &SyncthingDetailScreen{
		ctx: ctx,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *SyncthingDetailScreen) Init() tea.Cmd {
	return nil
}

func (s *SyncthingDetailScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	devices := s.ctx.Cfg.SyncthingDevices
	s.clampCursor()

	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.focusZone == syncDetailZoneButtons &&
			s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.focusZone == syncDetailZoneButtons &&
			s.btnIdx < 1 {
			s.btnIdx++
		}
		return s, nil
	case "up":
		if s.focusZone == syncDetailZoneList {
			if s.cursor > 0 {
				s.cursor--
			} else {
				s.focusZone = syncDetailZoneButtons
				s.btnIdx = 0
			}
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		if s.focusZone == syncDetailZoneButtons {
			if len(devices) > 0 {
				s.focusZone = syncDetailZoneList
				s.cursor = 0
			}
			return s, nil
		}
		if s.focusZone == syncDetailZoneList {
			if s.cursor < len(devices)-1 {
				s.cursor++
			}
		}
		return s, nil
	case "shift+tab":
		if s.focusZone == syncDetailZoneList {
			s.focusZone = syncDetailZoneButtons
			s.btnIdx = 0
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "backspace":
		// Clean backspace: does nothing
		return s, nil
	case "enter":
		return s.handleEnter()
	}
	return s, nil
}

func (s *SyncthingDetailScreen) handleEnter() (
	Screen, tea.Cmd,
) {
	if s.focusZone == syncDetailZoneButtons {
		switch s.btnIdx {
		case 0: // Pair Device
			screen := NewSyncthingPairScreen(s.ctx)
			return s, func() tea.Msg {
				return openTabMsg{
					Kind:   tabSyncthingPair,
					Label:  "Pair Device",
					Screen: screen,
				}
			}
		case 1: // Web UI
			screen := NewSyncthingWebUIScreen(s.ctx)
			return s, func() tea.Msg {
				return openTabMsg{
					Kind:   tabSyncthingWebUI,
					Label:  "Web UI",
					Screen: screen,
				}
			}
		}
		return s, nil
	}

	// Device list — open device detail
	devices := s.ctx.Cfg.SyncthingDevices
	if s.cursor < len(devices) {
		dev := devices[s.cursor]
		label := dev.Name
		if len(label) > 17 {
			label = label[:17] + "..."
		}
		screen := NewSyncthingDeviceScreen(
			s.ctx, dev, s.cursor)
		idx := s.cursor
		return s, func() tea.Msg {
			return openTabMsg{
				Kind:        tabSyncthingDevice,
				Label:       label,
				Index:       idx,
				Screen:      screen,
				FocusTabBar: true,
			}
		}
	}
	return s, nil
}

func (s *SyncthingDetailScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	return s, nil
}

func (s *SyncthingDetailScreen) View(
	w, h int,
) string {
	s.clampCursor()
	devices := s.ctx.Cfg.SyncthingDevices
	isFocused := s.ctx.ContentFocused
	onButtons := isFocused &&
		s.focusZone == syncDetailZoneButtons

	// ── Fixed header: title + buttons ────────────
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render(
				"Syncthing — Details"), w))
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		renderButtons(
			[]string{"Pair Device", "Web UI"},
			s.btnIdx, onButtons, w))
	headerLines = append(headerLines, "")

	headerH := len(headerLines)
	header := strings.Join(headerLines, "\n")

	// ── Scrollable body ─────────────────────────
	var midLines []string

	pairedCount := len(devices)
	midLines = append(midLines,
		" "+theme.Label.Render(fmt.Sprintf(
			"Paired Devices (%d)", pairedCount)))
	midLines = append(midLines, "")

	cursorLine := 0

	if pairedCount == 0 {
		midLines = append(midLines,
			" "+theme.Dim.Render(
				"No devices paired yet"))
	} else {
		hdrStyle := theme.TableHeader
		sepStyle := theme.TableDim

		nameW := 20
		idW := 24
		dateW := w - nameW - idW - 6
		if dateW < 12 {
			dateW = 12
		}

		hdr := " " +
			hdrStyle.Render(pad("Name", nameW)) +
			hdrStyle.Render(pad("Device ID", idW)) +
			hdrStyle.Render(
				fmt.Sprintf("%-*s", dateW, "Paired"))
		midLines = append(midLines, hdr)
		midLines = append(midLines,
			" "+sepStyle.Render(
				strings.Repeat("─", w-2)))

		onList := isFocused &&
			s.focusZone == syncDetailZoneList

		selStyle := theme.NavActive

		tableStart := len(midLines)

		for i := 0; i < pairedCount; i++ {
			d := devices[i]

			name := d.Name
			if len(name) > nameW-1 {
				name = name[:nameW-2] + ".."
			}
			nameStr := pad(name, nameW)

			devID := d.DeviceID
			if len(devID) > idW-1 {
				devID = devID[:idW-4] + "..."
			}
			idStr := pad(devID, idW)

			dateStr := fmt.Sprintf("%-*s",
				dateW, d.PairedAt)

			isSelected := onList && s.cursor == i

			marker := " "
			if isSelected {
				marker = "▸"
				cursorLine = tableStart + i
				midLines = append(midLines,
					marker+
						selStyle.Render(nameStr)+
						selStyle.Render(idStr)+
						selStyle.Render(dateStr))
			} else {
				midLines = append(midLines,
					marker+
						theme.Value.Render(nameStr)+
						theme.Dim.Render(idStr)+
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
			s.focusZone == syncDetailZoneList)

	return header + "\n" + vpRendered
}

func (s *SyncthingDetailScreen) HelpBindings() []key.Binding {
	var binds []key.Binding

	if s.focusZone == syncDetailZoneButtons {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "select")))
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
				key.WithKeys("down"),
				key.WithHelp("↓", "devices")))
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
// the device list may have shrunk (e.g. after a remove).
func (s *SyncthingDetailScreen) clampCursor() {
	devices := s.ctx.Cfg.SyncthingDevices
	if s.cursor >= len(devices) {
		s.cursor = len(devices) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
	if len(devices) == 0 &&
		s.focusZone == syncDetailZoneList {
		s.focusZone = syncDetailZoneButtons
		s.btnIdx = 0
	}
}
