package welcome

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Section IDs — 5 parents
const (
	secChannels = 0
	secWallet   = 1
	secOnChain  = 2
	secAddons   = 3
	secSystem   = 4
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

var (
	navItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))

	navActiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

	navCursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true)
)

func NewNavSidebar() NavSidebar {
	items := []NavItem{
		{"Channels", secChannels},
		{"Wallet", secWallet},
		{"On-Chain", secOnChain},
		{"Add-ons", secAddons},
		{"System", secSystem},
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
	if n.ActiveItem >= len(n.Items) {
		n.ActiveItem = len(n.Items) - 1
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
	n.ActiveItem = n.Cursor
	return n.Items[n.Cursor].Section
}

func (n *NavSidebar) ActiveSection() int {
	n.clamp()
	return n.Items[n.ActiveItem].Section
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
func (n NavSidebar) BlockRows(
	w int, blockHeights [numSections]int,
) [numSections][]string {
	var blocks [numSections][]string

	for si := 0; si < numSections; si++ {
		bh := blockHeights[si]
		if bh < 1 {
			bh = 1
		}

		item := n.Items[si]
		isActive := n.ActiveItem == si
		isCursor := n.Cursor == si && n.Focused

		style := navItemStyle
		if isActive {
			style = navActiveStyle
		} else if isCursor {
			style = navCursorStyle
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
		var rows []string
		for r := 0; r < bh; r++ {
			if r == titleRow {
				if isCursor && leftPad >= 1 {
					markerStyle := navActiveStyle
					if !isActive {
						markerStyle = navCursorStyle
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
