// internal/welcome/keys_dashboard.go

package welcome

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ripsline/virtual-private-node/internal/system"
)

func (m Model) handleDashboardCardKey(key string) (tea.Model, tea.Cmd) {
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

	if m.dashCard == cardServices {
		return m.handleServicesCardKey(key)
	}
	if m.dashCard == cardSystem {
		return m.handleSystemCardKey(key)
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

func (m Model) handleSystemCardKey(key string) (tea.Model, tea.Cmd) {
	if m.sysConfirm != "" {
		switch key {
		case "y":
			action := m.sysConfirm
			m.sysConfirm = ""
			if action == "update" {
				c := exec.Command("bash", "-c",
					"clear && sudo apt-get update && sudo apt-get upgrade -y"+
						" && echo && echo '  ✅ Update complete'"+
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
