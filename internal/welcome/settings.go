// internal/welcome/settings.go

package welcome

import (
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) viewSettings(bw int) string {
	cardW := bw - 2
	cardH := theme.BoxHeight

	updateCard := m.settingsUpdateCard(cardW, cardH)
	return updateCard
}

func (m Model) settingsUpdateCard(w, h int) string {
	var lines []string
	lines = append(lines, theme.Header.Render("Update"))
	lines = append(lines, "")
	lines = append(lines, theme.Label.Render("Current: ")+
		theme.Value.Render("v"+installer.GetVersion()))
	lines = append(lines, "")

	if m.latestVersion == "" {
		lines = append(lines,
			theme.Dim.Render("Checking for updates..."))
	} else if m.latestVersion == installer.GetVersion() {
		lines = append(lines, theme.GreenDot.Render("●")+" "+
			theme.Good.Render("Up to date"))
	} else {
		lines = append(lines, theme.Label.Render("Latest:  ")+
			theme.Action.Render("v"+m.latestVersion))
		lines = append(lines, "")
		if m.updateConfirm {
			lines = append(lines, theme.Warning.Render(
				"Update to v"+m.latestVersion+"? [y/n]"))
		} else {
			lines = append(lines,
				theme.Action.Render("Select to update ▸"))
		}
	}

	border := theme.SelectedBorder
	return border.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}
