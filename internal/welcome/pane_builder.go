package welcome

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// paneBuilder constructs consistently-formatted content
// panes. Every pane: blank line → centered title →
// blank line → content → optional buttons.
type paneBuilder struct {
	w     int
	lines []string
}

func newPane(w int) *paneBuilder {
	return &paneBuilder{w: w, lines: []string{""}}
}

func (p *paneBuilder) title(
	style lipgloss.Style, text string,
) *paneBuilder {
	p.lines = append(p.lines,
		centerPad(style.Render(text), p.w))
	p.lines = append(p.lines, "")
	return p
}

func (p *paneBuilder) blank() *paneBuilder {
	p.lines = append(p.lines, "")
	return p
}

func (p *paneBuilder) line(s string) *paneBuilder {
	p.lines = append(p.lines, s)
	return p
}

func (p *paneBuilder) field(
	label, value string,
) *paneBuilder {
	p.lines = append(p.lines,
		" "+theme.Label.Render(label)+
			theme.Value.Render(value))
	return p
}

func (p *paneBuilder) monoField(
	label, value string,
) *paneBuilder {
	p.lines = append(p.lines,
		" "+theme.Label.Render(label)+
			theme.Mono.Render(value))
	return p
}

func (p *paneBuilder) labelLine(
	label string,
) *paneBuilder {
	p.lines = append(p.lines,
		" "+theme.Label.Render(label))
	return p
}

func (p *paneBuilder) mono(text string) *paneBuilder {
	p.lines = append(p.lines,
		" "+theme.Mono.Render(text))
	return p
}

// monoWrap renders a long string across multiple lines,
// splitting at lineW characters per line. Used for txids,
// pubkeys, addresses, invoices, and other long values.
func (p *paneBuilder) monoWrap(text string) *paneBuilder {
	lineW := p.w - 2
	if lineW < 16 {
		lineW = 16
	}
	for len(text) > 0 {
		end := lineW
		if end > len(text) {
			end = len(text)
		}
		p.mono(text[:end])
		text = text[end:]
	}
	return p
}

func (p *paneBuilder) dim(text string) *paneBuilder {
	p.lines = append(p.lines,
		" "+theme.Dim.Render(text))
	return p
}

func (p *paneBuilder) warn(text string) *paneBuilder {
	p.lines = append(p.lines,
		" "+theme.Warning.Render(text))
	return p
}

func (p *paneBuilder) warnWrap(
	text string,
) *paneBuilder {
	lineW := p.w - 2
	if lineW < 16 {
		lineW = 16
	}
	for len(text) > 0 {
		end := lineW
		if end > len(text) {
			end = len(text)
		}
		p.warn(text[:end])
		text = text[end:]
	}
	return p
}

func (p *paneBuilder) success(
	text string,
) *paneBuilder {
	p.lines = append(p.lines,
		" "+theme.Success.Render(text))
	return p
}

func (p *paneBuilder) input(
	label string, ti textinput.Model, focused bool,
) *paneBuilder {
	labelStyle := theme.Label
	marker := " "
	if focused {
		labelStyle = theme.NavActive
		marker = theme.NavActive.Render("▸")
	}
	p.lines = append(p.lines,
		" "+labelStyle.Render(label))
	p.lines = append(p.lines,
		marker+" "+ti.View())
	return p
}

func (p *paneBuilder) buttons(
	labels []string, activeIdx int, focused bool,
) *paneBuilder {
	p.lines = append(p.lines,
		renderButtons(labels, activeIdx, focused, p.w))
	return p
}

func (p *paneBuilder) appendError(
	errMsg string,
) *paneBuilder {
	if errMsg != "" {
		p.lines = append(p.lines, "")
		p.warnWrap(errMsg)
	}
	return p
}

func (p *paneBuilder) render() string {
	return strings.Join(p.lines, "\n")
}

// ── Shared button renderer ───────────────────────────────

func renderButtons(
	labels []string, activeIdx int,
	focused bool, w int,
) string {
	return renderButtonsWithGray(
		labels, activeIdx, focused, w, -1, false)
}

func renderButtonsWithGray(
	labels []string, activeIdx int,
	focused bool, w int,
	grayIdx int, grayCondition bool,
) string {
	btnW := w - 2
	if btnW < 20 {
		btnW = 20
	}
	numBtns := len(labels)
	if numBtns == 0 {
		return ""
	}
	totalGap := (numBtns - 1) * 2
	perBtn := (btnW - totalGap) / numBtns
	if perBtn < 8 {
		perBtn = 8
	}

	var parts []string
	for i, label := range labels {
		if i == grayIdx && grayCondition {
			parts = append(parts,
				lipgloss.NewStyle().
					Foreground(theme.ColorGrayed).
					Width(perBtn).
					AlignHorizontal(lipgloss.Center).
					Render(label))
			continue
		}

		isActive := focused && activeIdx == i
		if isActive {
			parts = append(parts,
				theme.BtnFocused.
					Width(perBtn).
					AlignHorizontal(lipgloss.Center).
					Render(label))
		} else {
			parts = append(parts,
				theme.BtnNormal.
					Width(perBtn).
					AlignHorizontal(lipgloss.Center).
					Render(label))
		}
	}
	return " " + strings.Join(parts, "  ")
}
