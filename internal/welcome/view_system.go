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
	isFocused := m.contentFocused && !m.tabFocused

	boxW := w - 4
	if boxW < 30 {
		boxW = 30
	}

	border := theme.AddonBorderNormal
	titleStyle := lipgloss.NewStyle().
		Foreground(theme.ColorPrimary).
		Bold(true)

	// ── Fixed header (version + buttons) ─────────
	var headerLines []string
	headerLines = append(headerLines, "")

	verText := "Virtual Private Node v" +
		installer.GetVersion()
	headerLines = append(headerLines,
		centerPad(
			lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorAccent).
				Render(verText), w))

	if m.latestVersion != "" &&
		m.latestVersion != installer.GetVersion() {
		updateText := "Update available: v" +
			m.latestVersion
		headerLines = append(headerLines,
			centerPad(
				lipgloss.NewStyle().
					Foreground(theme.ColorUpdate).
					Render(updateText), w))
	}

	headerLines = append(headerLines, "")

	hasUpdate := m.latestVersion != "" &&
		m.latestVersion != m.version

	btnLabels := []string{"Update Packages"}
	if hasUpdate {
		btnLabels = append(btnLabels, "Update Node")
	} else {
		btnLabels = append(btnLabels, "Up to Date")
	}
	if m.status != nil && m.status.rebootRequired {
		btnLabels = append(btnLabels, "Reboot")
	}

	headerLines = append(headerLines,
		renderButtonsWithGray(
			btnLabels, m.btnIdx,
			isFocused && m.contentFocus == 0, w,
			1, !hasUpdate))
	headerLines = append(headerLines, "")

	if m.updateConfirm {
		headerLines = append(headerLines,
			" "+theme.Warning.Render(
				"Update to v"+m.latestVersion+
					"? [y/n]"))
		headerLines = append(headerLines, "")
	}

	if m.sysConfirm != "" {
		headerLines = append(headerLines,
			" "+theme.Warning.Render(
				fmt.Sprintf("%s? [y/n]",
					m.sysConfirm)))
		headerLines = append(headerLines, "")
	}

	header := strings.Join(headerLines, "\n")
	headerH := len(headerLines)

	// ── Scrollable middle (all cards) ────────────
	var midLines []string

	// Services card
	midLines = append(midLines,
		"  "+border.Render(
			"┌"+strings.Repeat("─", boxW-2)+"┐"))

	svcTitle := " Services"
	svcTitlePad := boxW - 2 - len(svcTitle)
	if svcTitlePad < 0 {
		svcTitlePad = 0
	}
	midLines = append(midLines,
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
			m.contentFocus == 1 &&
			m.svcCursor == i

		prefix := " "
		style := theme.Value
		if isSelected {
			prefix = "▸"
			style = theme.NavActive
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
		midLines = append(midLines,
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
		midLines = append(midLines,
			"  "+border.Render("│")+
				confirmLine+
				strings.Repeat(" ", confirmPad)+
				border.Render("│"))
	}

	midLines = append(midLines,
		"  "+border.Render(
			"└"+strings.Repeat("─", boxW-2)+"┘"))

	midLines = append(midLines, "")

	// Resources card
	midLines = append(midLines,
		"  "+border.Render(
			"┌"+strings.Repeat("─", boxW-2)+"┐"))

	resTitle := " Resources"
	resTitlePad := boxW - 2 - len(resTitle)
	if resTitlePad < 0 {
		resTitlePad = 0
	}
	midLines = append(midLines,
		"  "+border.Render("│")+
			titleStyle.Render(resTitle)+
			strings.Repeat(" ", resTitlePad)+
			border.Render("│"))

	if m.status != nil {
		resRows := []string{
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
			resRows = append(resRows,
				" "+theme.Label.Render("LND:  ")+
					theme.Value.Render(
						m.status.lndSize))
		}
		if m.status.rebootRequired {
			resRows = append(resRows,
				" "+theme.Warning.Render(
					"⚠ Reboot required"))
		}

		for _, rl := range resRows {
			rlVis := lipgloss.Width(rl)
			rlPad := boxW - 2 - rlVis
			if rlPad < 0 {
				rlPad = 0
			}
			midLines = append(midLines,
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
		midLines = append(midLines,
			"  "+border.Render("│")+
				loadLine+
				strings.Repeat(" ", loadPad)+
				border.Render("│"))
	}

	midLines = append(midLines,
		"  "+border.Render(
			"└"+strings.Repeat("─", boxW-2)+"┘"))

	midLines = append(midLines, "")

	// Bitcoin card
	midLines = append(midLines,
		"  "+border.Render(
			"┌"+strings.Repeat("─", boxW-2)+"┐"))

	btcTitle := " ₿ Bitcoin"
	btcTitlePad := boxW - 2 -
		lipgloss.Width(btcTitle)
	if btcTitlePad < 0 {
		btcTitlePad = 0
	}
	midLines = append(midLines,
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
		midLines = append(midLines,
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
		midLines = append(midLines,
			"  "+border.Render("│")+
				btcErr+
				strings.Repeat(" ", btcErrPad)+
				border.Render("│"))
	} else {
		var btcRows []string
		syncVal := theme.Good.Render("synced")
		if !m.status.btcSynced {
			syncVal = theme.Warn.Render("syncing")
		}
		btcRows = append(btcRows,
			" "+theme.Label.Render("Sync:     ")+
				syncVal)
		btcRows = append(btcRows,
			" "+theme.Label.Render("Height:   ")+
				theme.Value.Render(
					fmt.Sprintf("%d / %d",
						m.status.btcBlocks,
						m.status.btcHeaders)))
		if m.status.btcProgress > 0 {
			btcRows = append(btcRows,
				" "+theme.Label.Render("Progress: ")+
					theme.Value.Render(
						bitcoin.FormatProgress(
							m.status.btcProgress)))
		}
		btcRows = append(btcRows,
			" "+theme.Label.Render("Network:  ")+
				theme.Value.Render(m.cfg.Network))

		for _, bl := range btcRows {
			blVis := lipgloss.Width(bl)
			blPad := boxW - 2 - blVis
			if blPad < 0 {
				blPad = 0
			}
			midLines = append(midLines,
				"  "+border.Render("│")+
					bl+
					strings.Repeat(" ", blPad)+
					border.Render("│"))
		}
	}

	midLines = append(midLines,
		"  "+border.Render(
			"└"+strings.Repeat("─", boxW-2)+"┘"))

	midContent := strings.Join(midLines, "\n")

	// ── Viewport ─────────────────────────────────
	vpH := h - headerH
	if vpH < 1 {
		vpH = 1
	}

	// Services start at line 2 (top border + title)
	cursorLine := 2 + m.svcCursor

	vpRendered := renderViewport(
		midContent, w, vpH, cursorLine,
		len(midLines),
		m.contentFocus == 1 && len(names) > 0)

	// ── Assemble output ──────────────────────────
	return header + "\n" + vpRendered
}
