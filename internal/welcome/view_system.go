package welcome

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/bitcoin"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) systemOverview(w, h int) string {
	var lines []string
	lines = append(lines, "")

	boxW := w - 4
	if boxW < 30 {
		boxW = 30
	}

	border := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Bold(true)

	isFocused := m.contentFocused && !m.tabFocused

	// ── Version title (centered, no border) ──────
	verText := "Virtual Private Node v" +
		installer.GetVersion()
	lines = append(lines,
		centerPad(
			lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("220")).
				Render(verText), w))

	if m.latestVersion != "" &&
		m.latestVersion != installer.GetVersion() {
		updateText := "Update available: v" +
			m.latestVersion
		lines = append(lines,
			centerPad(
				lipgloss.NewStyle().
					Foreground(lipgloss.Color("34")).
					Render(updateText), w))
	}

	lines = append(lines, "")

	// ── Services card ────────────────────────────
	lines = append(lines,
		"  "+border.Render(
			"┌"+strings.Repeat("─", boxW-2)+"┐"))

	svcTitle := " Services"
	svcTitlePad := boxW - 2 - len(svcTitle)
	if svcTitlePad < 0 {
		svcTitlePad = 0
	}
	lines = append(lines,
		"  "+border.Render("│")+
			titleStyle.Render(svcTitle)+
			strings.Repeat(" ", svcTitlePad)+
			border.Render("│"))

	names := serviceNames(m.cfg)
	for i, name := range names {
		dot := theme.RedDot.Render("●")
		if m.status != nil {
			if active, ok :=
				m.status.services[name]; ok &&
				active {
				dot = theme.GreenDot.Render("●")
			}
		}

		isSelected := isFocused &&
			m.contentFocus == 0 &&
			m.svcCursor == i

		prefix := " "
		style := theme.Value
		if isSelected {
			prefix = "▸"
			style = navActiveStyle
		}

		svcLine := " " + prefix + " " + dot + " " +
			style.Render(name)

		if isSelected {
			hint := theme.Dim.Render(
				"  r restart  s stop  a start")
			svcLine += hint
		}

		svcVis := lipgloss.Width(svcLine)
		svcPad := boxW - 2 - svcVis
		if svcPad < 0 {
			svcPad = 0
		}
		lines = append(lines,
			"  "+border.Render("│")+
				svcLine+
				strings.Repeat(" ", svcPad)+
				border.Render("│"))
	}

	if m.svcConfirm != "" {
		svc := m.svcName(m.svcCursor)
		confirmLine := " " + theme.Warning.Render(
			fmt.Sprintf(" %s %s? [y/n]",
				m.svcConfirm, svc))
		confirmVis := lipgloss.Width(confirmLine)
		confirmPad := boxW - 2 - confirmVis
		if confirmPad < 0 {
			confirmPad = 0
		}
		lines = append(lines,
			"  "+border.Render("│")+
				confirmLine+
				strings.Repeat(" ", confirmPad)+
				border.Render("│"))
	}

	lines = append(lines,
		"  "+border.Render(
			"└"+strings.Repeat("─", boxW-2)+"┘"))

	lines = append(lines, "")

	// ── Resources card ───────────────────────────
	lines = append(lines,
		"  "+border.Render(
			"┌"+strings.Repeat("─", boxW-2)+"┐"))

	resTitle := " Resources"
	resTitlePad := boxW - 2 - len(resTitle)
	if resTitlePad < 0 {
		resTitlePad = 0
	}
	lines = append(lines,
		"  "+border.Render("│")+
			titleStyle.Render(resTitle)+
			strings.Repeat(" ", resTitlePad)+
			border.Render("│"))

	if m.status != nil {
		resLines := []string{
			" " + theme.Label.Render("Disk: ") +
				theme.Value.Render(
					fmt.Sprintf("%s / %s (%s)",
						m.status.diskUsed,
						m.status.diskTotal,
						m.status.diskPct)),
			" " + theme.Label.Render("RAM:  ") +
				theme.Value.Render(
					fmt.Sprintf("%s / %s (%s)",
						m.status.ramUsed,
						m.status.ramTotal,
						m.status.ramPct)),
			" " + theme.Label.Render("BTC:  ") +
				theme.Value.Render(
					m.status.btcSize),
		}
		if m.cfg.HasLND() {
			resLines = append(resLines,
				" "+theme.Label.Render("LND:  ")+
					theme.Value.Render(
						m.status.lndSize))
		}
		if m.status.rebootRequired {
			resLines = append(resLines,
				" "+theme.Warning.Render(
					"⚠ Reboot required"))
		}

		for _, rl := range resLines {
			rlVis := lipgloss.Width(rl)
			rlPad := boxW - 2 - rlVis
			if rlPad < 0 {
				rlPad = 0
			}
			lines = append(lines,
				"  "+border.Render("│")+
					rl+
					strings.Repeat(" ", rlPad)+
					border.Render("│"))
		}
	} else {
		loadLine := " " +
			theme.Dim.Render(" Loading...")
		loadVis := lipgloss.Width(loadLine)
		loadPad := boxW - 2 - loadVis
		if loadPad < 0 {
			loadPad = 0
		}
		lines = append(lines,
			"  "+border.Render("│")+
				loadLine+
				strings.Repeat(" ", loadPad)+
				border.Render("│"))
	}

	lines = append(lines,
		"  "+border.Render(
			"└"+strings.Repeat("─", boxW-2)+"┘"))

	lines = append(lines, "")

	// ── Bitcoin card ─────────────────────────────
	lines = append(lines,
		"  "+border.Render(
			"┌"+strings.Repeat("─", boxW-2)+"┐"))

	btcTitle := " ₿ Bitcoin"
	btcTitlePad := boxW - 2 -
		lipgloss.Width(btcTitle)
	if btcTitlePad < 0 {
		btcTitlePad = 0
	}
	lines = append(lines,
		"  "+border.Render("│")+
			theme.Bitcoin.Render(btcTitle)+
			strings.Repeat(" ", btcTitlePad)+
			border.Render("│"))

	if m.status == nil {
		btcLoad := " " +
			theme.Dim.Render(" Loading...")
		btcLoadVis := lipgloss.Width(btcLoad)
		btcLoadPad := boxW - 2 - btcLoadVis
		if btcLoadPad < 0 {
			btcLoadPad = 0
		}
		lines = append(lines,
			"  "+border.Render("│")+
				btcLoad+
				strings.Repeat(" ", btcLoadPad)+
				border.Render("│"))
	} else if !m.status.btcResponding {
		btcErr := " " +
			theme.Warn.Render(" Not responding")
		btcErrVis := lipgloss.Width(btcErr)
		btcErrPad := boxW - 2 - btcErrVis
		if btcErrPad < 0 {
			btcErrPad = 0
		}
		lines = append(lines,
			"  "+border.Render("│")+
				btcErr+
				strings.Repeat(" ", btcErrPad)+
				border.Render("│"))
	} else {
		var btcLines []string
		syncVal := theme.Good.Render("synced")
		if !m.status.btcSynced {
			syncVal = theme.Warn.Render("syncing")
		}
		btcLines = append(btcLines,
			" "+theme.Label.Render("Sync:     ")+
				syncVal)
		btcLines = append(btcLines,
			" "+theme.Label.Render("Height:   ")+
				theme.Value.Render(
					fmt.Sprintf("%d / %d",
						m.status.btcBlocks,
						m.status.btcHeaders)))
		if m.status.btcProgress > 0 {
			btcLines = append(btcLines,
				" "+theme.Label.Render("Progress: ")+
					theme.Value.Render(
						bitcoin.FormatProgress(
							m.status.btcProgress)))
		}
		btcLines = append(btcLines,
			" "+theme.Label.Render("Network:  ")+
				theme.Value.Render(m.cfg.Network))

		for _, bl := range btcLines {
			blVis := lipgloss.Width(bl)
			blPad := boxW - 2 - blVis
			if blPad < 0 {
				blPad = 0
			}
			lines = append(lines,
				"  "+border.Render("│")+
					bl+
					strings.Repeat(" ", blPad)+
					border.Render("│"))
		}
	}

	lines = append(lines,
		"  "+border.Render(
			"└"+strings.Repeat("─", boxW-2)+"┘"))

	if m.updateConfirm {
		lines = append(lines, "")
		lines = append(lines, " "+theme.Warning.Render(
			"Update to v"+m.latestVersion+"? [y/n]"))
	}

	if m.sysConfirm != "" {
		lines = append(lines, "")
		lines = append(lines, " "+theme.Warning.Render(
			fmt.Sprintf("%s? [y/n]", m.sysConfirm)))
	}

	lines = append(lines, "")
	lines = append(lines, m.systemButtons(w))

	return strings.Join(lines, "\n")
}

func (m Model) systemButtons(w int) string {
	isFocused := m.contentFocused && !m.tabFocused &&
		m.contentFocus == 1

	btnW := w - 2
	if btnW < 20 {
		btnW = 20
	}

	hasUpdate := m.latestVersion != "" &&
		m.latestVersion != m.version

	labels := []string{"Update Packages"}
	if hasUpdate {
		labels = append(labels, "Update Node")
	} else {
		labels = append(labels, "Up to Date")
	}
	if m.status != nil && m.status.rebootRequired {
		labels = append(labels, "Reboot")
	}

	numBtns := len(labels)
	gaps := numBtns - 1
	totalGap := gaps * 2
	perBtn := (btnW - totalGap) / numBtns
	if perBtn < 10 {
		perBtn = 10
	}

	var parts []string
	for i, label := range labels {
		isActive := isFocused && m.btnIdx == i

		// Gray out "Up to Date" button
		if i == 1 && !hasUpdate {
			parts = append(parts,
				lipgloss.NewStyle().
					Foreground(lipgloss.Color("240")).
					Width(perBtn).
					AlignHorizontal(lipgloss.Center).
					Render(label))
			continue
		}

		if isActive {
			parts = append(parts,
				theme.BtnFocused.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		} else {
			parts = append(parts,
				theme.BtnNormal.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		}
	}

	return " " + strings.Join(parts, "  ")
}
