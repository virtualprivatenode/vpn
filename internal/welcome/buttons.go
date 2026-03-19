package welcome

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Button ───────────────────────────────────────────────

// ButtonState represents the visual state of a button.
type ButtonState int

const (
	ButtonNormal   ButtonState = iota // Gray border, dim text
	ButtonFocused                     // Gold border, normal text (navigation cursor is here)
	ButtonActive                      // Gold border, gold text (currently selected section)
	ButtonDisabled                    // Dark gray border, grayed text
)

// Button is a single selectable element rendered as a bordered box.
type Button struct {
	Label string
	State ButtonState
	Width int
}

// View renders a single button as a bordered, padded label.
func (b Button) View() string {
	label := b.Label
	if b.Width > 0 {
		// Pad label to fill width, accounting for border + padding
		innerWidth := b.Width - 4
		if innerWidth < len(label) {
			innerWidth = len(label)
		}
		label = padRight(label, innerWidth)
	}

	switch b.State {
	case ButtonActive:
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("220")).
			Foreground(lipgloss.Color("220")).
			Bold(true).
			Padding(0, 1).
			Width(b.Width).
			Render(label)
	case ButtonFocused:
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("220")).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1).
			Width(b.Width).
			Render(label)
	case ButtonDisabled:
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Foreground(lipgloss.Color("240")).
			Padding(0, 1).
			Width(b.Width).
			Render(label)
	default: // ButtonNormal
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("245")).
			Foreground(lipgloss.Color("250")).
			Padding(0, 1).
			Width(b.Width).
			Render(label)
	}
}

// ── ButtonGroup ──────────────────────────────────────────

// Orientation defines how buttons are laid out.
type Orientation int

const (
	Horizontal Orientation = iota
	Vertical
)

// ButtonGroup manages a collection of buttons with focus and selection.
type ButtonGroup struct {
	Labels      []string
	FocusIndex  int // which button has navigation focus
	ActiveIndex int // which button is the active/selected section (-1 if none)
	Orientation Orientation
	Width       int    // per-button width (0 = auto-size to label)
	Disabled    []bool // per-button disabled state
	Gap         int    // spacing between buttons (default 0 for horizontal, 1 for vertical)
	Focused     bool   // whether this group currently has navigation focus
}

// NewButtonGroup creates a button group with the given labels.
func NewButtonGroup(labels []string, orientation Orientation) ButtonGroup {
	gap := 0
	if orientation == Vertical {
		gap = 1
	}
	return ButtonGroup{
		Labels:      labels,
		FocusIndex:  0,
		ActiveIndex: -1,
		Orientation: orientation,
		Disabled:    make([]bool, len(labels)),
		Gap:         gap,
		Focused:     false,
	}
}

// SetWidth sets the width for all buttons in the group.
func (bg *ButtonGroup) SetWidth(w int) {
	bg.Width = w
}

// SetDisabled marks a button at index i as disabled or enabled.
func (bg *ButtonGroup) SetDisabled(i int, disabled bool) {
	if i >= 0 && i < len(bg.Disabled) {
		bg.Disabled[i] = disabled
	}
}

// Focus gives this group navigation focus.
func (bg *ButtonGroup) Focus() {
	bg.Focused = true
	if bg.FocusIndex >= 0 && bg.FocusIndex < len(bg.Labels) {
		if bg.Disabled[bg.FocusIndex] {
			bg.focusNextEnabled(1)
		}
	}
}

// Blur removes navigation focus from this group.
func (bg *ButtonGroup) Blur() {
	bg.Focused = false
}

// FocusNext moves focus to the next non-disabled button.
// Returns true if focus moved.
func (bg *ButtonGroup) FocusNext() bool {
	return bg.focusNextEnabled(1)
}

// FocusPrev moves focus to the previous non-disabled button.
// Returns true if focus moved.
func (bg *ButtonGroup) FocusPrev() bool {
	return bg.focusNextEnabled(-1)
}

// focusNextEnabled moves focus in the given direction, skipping disabled.
func (bg *ButtonGroup) focusNextEnabled(dir int) bool {
	if len(bg.Labels) == 0 {
		return false
	}
	start := bg.FocusIndex
	for i := 0; i < len(bg.Labels)-1; i++ {
		next := (start + dir*(i+1) + len(bg.Labels)*2) % len(bg.Labels)
		if !bg.Disabled[next] {
			bg.FocusIndex = next
			return true
		}
	}
	return false
}

// SetFocus sets focus to a specific button index. No-op if disabled.
func (bg *ButtonGroup) SetFocus(i int) {
	if i >= 0 && i < len(bg.Labels) && !bg.Disabled[i] {
		bg.FocusIndex = i
	}
}

// Activate sets the active index to the currently focused button.
// Returns the activated button index, or -1 if disabled.
func (bg *ButtonGroup) Activate() int {
	if bg.FocusIndex >= 0 && bg.FocusIndex < len(bg.Labels) &&
		!bg.Disabled[bg.FocusIndex] {
		bg.ActiveIndex = bg.FocusIndex
		return bg.ActiveIndex
	}
	return -1
}

// ActivateIndex sets a specific button as active. No-op if disabled.
func (bg *ButtonGroup) ActivateIndex(i int) {
	if i >= 0 && i < len(bg.Labels) && !bg.Disabled[i] {
		bg.ActiveIndex = i
	}
}

// View renders the button group.
func (bg ButtonGroup) View() string {
	buttons := make([]string, len(bg.Labels))
	for i, label := range bg.Labels {
		state := ButtonNormal
		if bg.Disabled[i] {
			state = ButtonDisabled
		} else if bg.Focused && bg.FocusIndex == i {
			if bg.ActiveIndex == i {
				state = ButtonActive
			} else {
				state = ButtonFocused
			}
		} else if bg.ActiveIndex == i {
			state = ButtonActive
		}

		btn := Button{
			Label: label,
			State: state,
			Width: bg.Width,
		}
		buttons[i] = btn.View()
	}

	if bg.Orientation == Horizontal {
		return lipgloss.JoinHorizontal(lipgloss.Top, buttons...)
	}

	if bg.Gap <= 1 {
		return lipgloss.JoinVertical(lipgloss.Left, buttons...)
	}

	var parts []string
	spacer := strings.Repeat("\n", bg.Gap-1)
	for i, btn := range buttons {
		parts = append(parts, btn)
		if i < len(buttons)-1 {
			parts = append(parts, spacer)
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// HandleKeyPress processes arrow key navigation for this group.
// Returns (handled bool, activated int).
// activated >= 0 means Enter was pressed on that button index.
// activated == -1 means no activation.
func (bg *ButtonGroup) HandleKeyPress(key string) (handled bool, activated int) {
	if !bg.Focused {
		return false, -1
	}

	switch bg.Orientation {
	case Horizontal:
		switch key {
		case "left", "h":
			return bg.FocusPrev(), -1
		case "right", "l":
			return bg.FocusNext(), -1
		case "enter":
			return true, bg.Activate()
		}
	case Vertical:
		switch key {
		case "up", "k":
			return bg.FocusPrev(), -1
		case "down", "j":
			return bg.FocusNext(), -1
		case "enter":
			return true, bg.Activate()
		}
	}
	return false, -1
}

// ── Helpers ──────────────────────────────────────────────

// padRight pads a string with spaces to reach the target width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// Ensure theme import is used.
var _ = theme.SelectedBorder
