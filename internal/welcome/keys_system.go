package welcome

import (
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/system"
)

func (m Model) handleSystemCardKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "backspace":
		m.cardActive = false
		m.svcConfirm = ""
		m.sysConfirm = ""
		return m, nil
	case "q":
		return m, tea.Quit
	case "tab":
		m.activeTab = (m.activeTab + 1) % 5
		m.cardActive = false
		m.svcConfirm = ""
		m.sysConfirm = ""
		return m, nil
	case "shift+tab":
		m.activeTab = (m.activeTab + 4) % 5
		m.cardActive = false
		m.svcConfirm = ""
		m.sysConfirm = ""
		return m, nil
	}

	if m.sysCard == cardServices {
		return m.handleServicesCardKey(key)
	}
	if m.sysCard == cardSysStats {
		return m.handleSysStatsCardKey(key)
	}

	return m, nil
}

func (m Model) handleServicesCardKey(key string) (tea.Model, tea.Cmd) {
	if m.svcConfirm != "" {
		switch key {
		case "y":
			svc := m.svcName(m.svcCursor)
			action := m.svcConfirm
			m.svcConfirm = ""
			return m, func() tea.Msg {
				system.SudoRun("systemctl", action, svc)
				return svcActionDoneMsg{}
			}
		default:
			m.svcConfirm = ""
			return m, nil
		}
	}

	switch key {
	case "up", "k":
		if m.svcCursor > 0 {
			m.svcCursor--
		}
	case "down", "j":
		if m.svcCursor < m.svcCount()-1 {
			m.svcCursor++
		}
	case "r":
		m.svcConfirm = "restart"
	case "s":
		m.svcConfirm = "stop"
	case "a":
		m.svcConfirm = "start"
	case "l":
		svc := m.svcName(m.svcCursor)
		c := exec.Command("bash", "-c",
			"clear && sudo journalctl -u "+svc+" -n 100 --no-pager"+
				" && echo && echo '  Press Enter to return...' && read")
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return svcActionDoneMsg{}
		})
	}
	return m, nil
}

func (m Model) handleSysStatsCardKey(key string) (tea.Model, tea.Cmd) {
	if m.sysConfirm != "" {
		switch key {
		case "y":
			action := m.sysConfirm
			m.sysConfirm = ""
			if action == "update" {
				c := exec.Command("bash", "-c",
					"clear && sudo apt-get update && sudo apt-get upgrade -y"+
						" && echo && echo '  Update complete'"+
						" && echo '  Press Enter to return...' && read")
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return svcActionDoneMsg{}
				})
			}
			if action == "reboot" {
				return m, func() tea.Msg {
					system.SudoRun("reboot")
					return svcActionDoneMsg{}
				}
			}
		default:
			m.sysConfirm = ""
			return m, nil
		}
		return m, nil
	}

	switch key {
	case "u":
		m.sysConfirm = "update"
	case "r":
		if m.status != nil && m.status.rebootRequired {
			m.sysConfirm = "reboot"
		}
	}
	return m, nil
}

// handleSystemTabKey handles keys on the System tab when no card is active
// and no update confirm is pending. Install actions for LND/wallet are
// triggered from the Update card area via enter → handleSystemEnter.
func (m Model) handleSystemTabKey(key string) (tea.Model, tea.Cmd) {
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
	}
	return m, nil
}

// isUpdateAvailable checks if a newer version exists.
func isUpdateAvailable(latestVersion string) bool {
	return latestVersion != "" && latestVersion != installer.GetVersion()
}
