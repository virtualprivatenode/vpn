// internal/welcome/view.go

package welcome

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	switch m.subview {
	case svLightning:
		return m.viewLightning()
	case svZeus:
		return m.viewZeus()
	case svSyncthingDetail:
		return m.viewSyncthingDetail()
	case svSyncthingPairInput:
		return m.viewSyncthingPairInput()
	case svSyncthingDeviceDetail:
		return m.viewSyncthingDeviceDetail()
	case svSyncthingWebUI:
		return m.viewSyncthingWebUI()
	case svSyncthingDeviceQR:
		return m.viewSyncthingDeviceQR()
	case svLITDetail:
		return m.viewLITDetail()
	case svLndHubManage:
		return m.viewLndHubManage()
	case svLndHubCreateName:
		return m.viewLndHubCreateName()
	case svLndHubCreateAccount:
		return m.viewLndHubNewAccount()
	case svLndHubAccountDetail:
		return m.viewLndHubAccountDetail()
	case svLndHubDeactivateConfirm:
		return m.viewLndHubDeactivateConfirm()
	case svQR:
		return m.viewQR()
	case svFullURL:
		return m.viewFullURL()
	}

	bw := min(m.width-4, theme.ContentWidth)
	var content string
	switch m.activeTab {
	case tabDashboard:
		content = m.viewDashboard(bw)
	case tabPairing:
		content = m.viewPairing(bw)
	case tabAddons:
		content = m.viewAddons(bw)
	case tabSettings:
		content = m.viewSettings(bw)
	}

	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(fmt.Sprintf(" Virtual Private Node v%s ",
			m.version))
	tabs := m.viewTabs(bw)
	footer := m.viewFooter()
	body := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", tabs, "", content)
	full := lipgloss.JoinVertical(lipgloss.Center,
		body, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewTabs(tw int) string {
	tabs := []struct {
		n string
		t wTab
	}{
		{"Dashboard", tabDashboard},
		{"Pairing", tabPairing},
		{"Add-ons", tabAddons},
		{"Settings", tabSettings},
	}
	w := tw / len(tabs)
	var out []string
	for _, t := range tabs {
		if t.t == m.activeTab {
			out = append(out,
				theme.ActiveTab.Width(w).
					Align(lipgloss.Center).Render(t.n))
		} else {
			out = append(out,
				theme.InactiveTab.Width(w).
					Align(lipgloss.Center).Render(t.n))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, out...)
}

func (m Model) viewFooter() string {
	if m.cardActive {
		if m.dashCard == cardServices {
			return theme.Footer.Render(
				"  ↑↓ select • [r]estart [s]top [a]start [l]ogs • backspace back • q quit  ")
		}
		if m.dashCard == cardSystem {
			if m.status != nil && m.status.rebootRequired {
				return theme.Footer.Render(
					"  [u]pdate • [r]eboot • backspace back • q quit  ")
			}
			return theme.Footer.Render(
				"  [u]pdate system • backspace back • q quit  ")
		}
	}
	switch m.activeTab {
	case tabDashboard:
		return theme.Footer.Render(
			"  ↑↓←→ navigate • enter select • tab switch • q quit  ")
	case tabPairing:
		return theme.Footer.Render(
			"  ←→ select • enter open • tab switch • q quit  ")
	case tabAddons:
		return theme.Footer.Render(
			"  ←→ select • enter install/view • tab switch • q quit  ")
	case tabSettings:
		if m.updateConfirm {
			return theme.Footer.Render(
				"  y confirm • any key cancel  ")
		}
		return theme.Footer.Render(
			"  enter update • tab switch • q quit  ")
	}
	return ""
}

func (m Model) viewFullURL() string {
	title := theme.Header.Render(
		"Full URL — Copy and paste into Tor Browser")
	hint := theme.Dim.Render(
		"Select and copy. Press backspace to go back.")
	content := lipgloss.JoinVertical(lipgloss.Left,
		"", title, "", hint, "", m.urlTarget, "")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, content)
}

func padLines(lines []string, target int) string {
	for len(lines) < target {
		lines = append(lines, "")
	}
	if len(lines) > target {
		lines = lines[:target]
	}
	return strings.Join(lines, "\n")
}
