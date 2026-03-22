package welcome

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Section IDs — 4 parents
const (
	secChannels = 0
	secWallet   = 1
	secAddons   = 2
	secSystem   = 3
)

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

	navSepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("239"))
)

func NewNavSidebar() NavSidebar {
	items := []NavItem{
		{"Channels", secChannels},
		{"Wallet", secWallet},
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

// BlockRows returns styled rows for each of the 4 section
// blocks. Each block is vertically centered text, sized to
// fill the sidebar evenly. No borders here — the layout
// frame handles all border drawing.
//
// When focused, the active section shows ▸ to the left of
// the label. The label stays centered — ▸ occupies the
// margin space that would otherwise be blank.
func (n NavSidebar) BlockRows(
	w int, blockHeights [4]int,
) [4][]string {
	var blocks [4][]string

	for si := 0; si < 4; si++ {
		bh := blockHeights[si]
		if bh < 1 {
			bh = 1
		}

		item := n.Items[si]
		isActive := n.ActiveItem == si

		style := navItemStyle
		if isActive && n.Focused {
			style = navActiveStyle
		}

		label := item.Label

		// Center the label within w
		labelLen := len([]rune(label))
		totalPad := w - labelLen
		if totalPad < 0 {
			totalPad = 0
		}
		leftPad := totalPad / 2
		rightPad := totalPad - leftPad

		// Build the title row
		titleRow := bh / 2
		var rows []string
		for r := 0; r < bh; r++ {
			if r == titleRow {
				// Place ▸ at position 0 when
				// active+focused. The label is
				// centered as normal — ▸ replaces
				// the first padding space.
				if isActive && n.Focused &&
					leftPad >= 1 {
					row := "▸" +
						strings.Repeat(" ",
							leftPad-1) +
						style.Render(label) +
						strings.Repeat(" ",
							rightPad)
					// Render ▸ in the active style
					row = navActiveStyle.Render(
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

func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return string(r[:1])
	}
	return string(r[:w-1]) + "…"
}

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
