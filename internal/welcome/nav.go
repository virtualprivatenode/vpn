package welcome

import (
	"strings"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// Section IDs — 5 parents + theme toggle
const (
	secChannels    = 0
	secWallet      = 1
	secOnChain     = 2
	secAddons      = 3
	secSystem      = 4
	secThemeToggle = 5
)

const numSections = 5

type NavItem struct {
	Label   string
	Section int
}

type NavSidebar struct {
	Items      []NavItem
	Cursor     int
	ActiveItem int
	Width      int
	Height     int
	Focused    bool
}

// Nav styles are accessed directly from theme.NavItem,
// theme.NavActive, theme.NavCursor — no aliases needed.
// This ensures styles stay current after theme.Toggle().

func NewNavSidebar() NavSidebar {
	items := []NavItem{
		{"Channels", secChannels},
		{"Lightning", secWallet},
		{"Bitcoin", secOnChain},
		{"Add-ons", secAddons},
		{"System", secSystem},
		{theme.ThemeIcon(), secThemeToggle},
	}
	return NavSidebar{
		Items:      items,
		Cursor:     0,
		ActiveItem: 0,
		Width:      12,
		Height:     24,
		Focused:    true,
	}
}

func (n *NavSidebar) Focus() { n.Focused = true }
func (n *NavSidebar) Blur()  { n.Focused = false }

func (n *NavSidebar) clamp() {
	if n.Cursor < 0 {
		n.Cursor = 0
	}
	if n.Cursor >= len(n.Items) {
		n.Cursor = len(n.Items) - 1
	}
	if n.ActiveItem < 0 {
		n.ActiveItem = 0
	}
	// ActiveItem must stay on a real section, never
	// on the theme toggle.
	if n.ActiveItem >= numSections {
		n.ActiveItem = numSections - 1
	}
}

func (n *NavSidebar) MoveUp() {
	if n.Cursor > 0 {
		n.Cursor--
	}
}

func (n *NavSidebar) MoveDown() {
	if n.Cursor < len(n.Items)-1 {
		n.Cursor++
	}
}

func (n *NavSidebar) Activate() int {
	n.clamp()
	// Theme toggle is not a real section — don't
	// activate it via this path.
	if n.Items[n.Cursor].Section == secThemeToggle {
		return n.Items[n.ActiveItem].Section
	}
	n.ActiveItem = n.Cursor
	return n.Items[n.Cursor].Section
}

func (n *NavSidebar) ActiveSection() int {
	n.clamp()
	return n.Items[n.ActiveItem].Section
}

// IsOnThemeToggle reports whether the cursor is on the
// theme toggle item.
func (n *NavSidebar) IsOnThemeToggle() bool {
	n.clamp()
	return n.Items[n.Cursor].Section == secThemeToggle
}

// UpdateThemeLabel refreshes the theme icon label after
// a toggle so the sidebar shows the new icon.
func (n *NavSidebar) UpdateThemeLabel() {
	for i := range n.Items {
		if n.Items[i].Section == secThemeToggle {
			n.Items[i].Label = theme.ThemeIcon()
			return
		}
	}
}

func (n *NavSidebar) SetActive(section int) {
	for i, it := range n.Items {
		if it.Section == section {
			n.Cursor = i
			n.ActiveItem = i
			return
		}
	}
}

// BlockRows returns styled rows for each section block.
// Active section label is yellow (always visible).
// Cursor position shows ▸ when sidebar is focused.
// Cursor-only (not active) shows bright white.
// The theme toggle icon is rendered inside the System
// block, one row below the System label.
func (n NavSidebar) BlockRows(
	w int, blockHeights [numSections]int,
) [numSections][]string {
	var blocks [numSections][]string

	// Find the theme toggle item (last in Items).
	themeIdx := -1
	for i, it := range n.Items {
		if it.Section == secThemeToggle {
			themeIdx = i
			break
		}
	}

	for si := 0; si < numSections; si++ {
		bh := blockHeights[si]
		if bh < 1 {
			bh = 1
		}

		item := n.Items[si]
		isActive := n.ActiveItem == si
		isCursor := n.Cursor == si && n.Focused

		style := theme.NavItem
		if isActive {
			style = theme.NavActive
		} else if isCursor {
			style = theme.NavCursor
		}

		label := item.Label

		labelLen := len([]rune(label))
		totalPad := w - labelLen
		if totalPad < 0 {
			totalPad = 0
		}
		leftPad := totalPad / 2
		rightPad := totalPad - leftPad

		titleRow := bh / 2

		// For the System block, place the label one
		// row higher to make room for the theme icon.
		isSystemBlock := si == secSystem
		if isSystemBlock && bh >= 3 {
			titleRow = bh/2 - 1
		}

		themeRow := -1
		if isSystemBlock {
			themeRow = titleRow + 2
			if themeRow >= bh {
				themeRow = titleRow + 1
			}
			if themeRow >= bh {
				themeRow = -1 // no room
			}
		}

		var rows []string
		for r := 0; r < bh; r++ {
			if r == titleRow {
				if isCursor && leftPad >= 1 {
					markerStyle := theme.NavActive
					if !isActive {
						markerStyle = theme.NavCursor
					}
					row := markerStyle.Render(
						"▸") +
						strings.Repeat(" ",
							leftPad-1) +
						style.Render(label) +
						strings.Repeat(" ",
							rightPad)
					rows = append(rows, row)
				} else {
					rows = append(rows,
						strings.Repeat(" ",
							leftPad)+
							style.Render(label)+
							strings.Repeat(" ",
								rightPad))
				}
			} else if r == themeRow && themeIdx >= 0 {
				// Render theme toggle icon
				icon := n.Items[themeIdx].Label
				isThemeCursor := n.Cursor == themeIdx &&
					n.Focused

				iconStyle := theme.Dim
				if isThemeCursor {
					iconStyle = theme.NavActive
				}

				iconLen := len([]rune(icon))
				iconTotal := w - iconLen
				if iconTotal < 0 {
					iconTotal = 0
				}
				iconLeft := iconTotal / 2
				iconRight := iconTotal - iconLeft

				if isThemeCursor && iconLeft >= 1 {
					row := theme.NavActive.Render(
						"▸") +
						strings.Repeat(" ",
							iconLeft-1) +
						iconStyle.Render(icon) +
						strings.Repeat(" ",
							iconRight)
					rows = append(rows, row)
				} else {
					rows = append(rows,
						strings.Repeat(" ",
							iconLeft)+
							iconStyle.Render(icon)+
							strings.Repeat(" ",
								iconRight))
				}
			} else {
				rows = append(rows,
					strings.Repeat(" ", w))
			}
		}

		blocks[si] = rows
	}

	return blocks
}

// ── String helpers ───────────────────────────────────────

func pad(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

func centerPad(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return string(r[:w])
	}
	left := (w - len(r)) / 2
	right := w - len(r) - left
	return strings.Repeat(" ", left) + s +
		strings.Repeat(" ", right)
}
