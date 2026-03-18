// internal/welcome/keys_settings.go

package welcome

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ripsline/virtual-private-node/internal/installer"
)

func (m Model) handleSettingsTabKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.activeTab = (m.activeTab + 1) % 5
		m.updateConfirm = false
		return m, nil
	case "shift+tab":
		m.activeTab = (m.activeTab + 4) % 5
		m.updateConfirm = false
		return m, nil
	case "1":
		m.activeTab = tabDashboard
		m.updateConfirm = false
	case "2":
		m.activeTab = tabLightning
		m.updateConfirm = false
	case "3":
		m.activeTab = tabPairing
		m.updateConfirm = false
	case "4":
		m.activeTab = tabAddons
		m.updateConfirm = false
	case "5":
		// already on settings
	default:
		if m.updateConfirm {
			switch key {
			case "y":
				m.updateConfirm = false
				m.shellAction = svSelfUpdate
				return m, tea.Quit
			default:
				m.updateConfirm = false
				return m, nil
			}
		}
		switch key {
		case "enter":
			if m.latestVersion != "" &&
				m.latestVersion != installer.GetVersion() {
				m.updateConfirm = true
			}
		}
		return m, nil
	}
	return m, nil
}
