// internal/welcome/keys_addons.go

package welcome

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/paths"
)

func (m Model) handleAddonsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		switch m.subview {
		case svSyncthingDetail:
			m.subview = svNone
		case svSyncthingDeviceDetail:
			m.subview = svSyncthingDetail
		case svSyncthingWebUI:
			m.subview = svSyncthingDetail
		case svSyncthingDeviceQR:
			m.subview = svSyncthingDetail
		case svLndHubManage:
			m.subview = svNone
		case svLndHubCreateAccount:
			m.lastAccount = nil
			m.hubNameInput = ""
			m.subview = svLndHubManage
		case svLndHubAccountDetail:
			m.subview = svLndHubManage
		case svLndHubDeactivateConfirm:
			m.subview = svLndHubManage
		default:
			m.subview = svNone
		}
		return m, nil
	}

	switch m.subview {
	case svSyncthingDetail:
		return m.handleSyncthingDetailKey(key)
	case svSyncthingWebUI:
		return m.handleSyncthingWebUIKey(key)
	case svSyncthingDeviceDetail:
		// no extra keys
		return m, nil
	case svSyncthingDeviceQR:
		// no extra keys
		return m, nil
	case svLndHubManage:
		return m.handleLndHubManageKey(key)
	case svLndHubCreateAccount:
		return m.handleLndHubQRKeys(key)
	case svLndHubAccountDetail:
		// no extra keys
		return m, nil
	case svLndHubDeactivateConfirm:
		return m.handleLndHubDeactivateKey(key)
	}
	return m, nil
}

func (m Model) handleSyncthingDetailKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "a":
		m.syncDeviceInput = ""
		m.syncPairError = ""
		m.syncPairSuccess = false
		m.subview = svSyncthingPairInput
		return m, nil
	case "d":
		m.subview = svSyncthingDeviceQR
		return m, nil
	case "u":
		m.subview = svSyncthingWebUI
		return m, nil
	case "enter":
		if len(m.cfg.SyncthingDevices) > 0 {
			m.subview = svSyncthingDeviceDetail
		}
		return m, nil
	case "up", "k":
		if m.syncCursor > 0 {
			m.syncCursor--
		}
		return m, nil
	case "down", "j":
		if m.syncCursor < len(m.cfg.SyncthingDevices)-1 {
			m.syncCursor++
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleSyncthingWebUIKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "s":
		m.showSecrets = !m.showSecrets
		return m, nil
	case "u":
		syncOnion := readOnion(paths.TorSyncthingHostname)
		if syncOnion != "" {
			m.urlTarget = "http://" + syncOnion + ":8384"
			m.urlReturnTo = svSyncthingWebUI
			m.subview = svFullURL
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleSyncthingPairInputKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if len(m.syncDeviceInput) > 0 {
			m.syncDeviceInput = m.syncDeviceInput[:len(m.syncDeviceInput)-1]
		} else {
			m.syncPairError = ""
			m.syncPairSuccess = false
			m.subview = svSyncthingDetail
		}
		return m, nil
	case "enter":
		if m.syncPairSuccess {
			m.syncDeviceInput = ""
			m.syncPairSuccess = false
			m.subview = svSyncthingDetail
			return m, nil
		}
		if m.syncDeviceInput != "" {
			parts := strings.Split(m.syncDeviceInput, "-")
			if len(parts) != 8 {
				m.syncPairError = "Invalid Device ID format. Expected 8 groups separated by hyphens."
				return m, nil
			}
			for _, p := range parts {
				if len(p) != 7 {
					m.syncPairError = "Invalid Device ID format. Each group should be 7 characters."
					return m, nil
				}
			}
			m.syncPairError = ""
			return m, pairSyncthingDeviceCmd(m.syncDeviceInput)
		}
		return m, nil
	default:
		for _, ch := range key {
			if len(m.syncDeviceInput) >= 63 {
				break
			}
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') ||
				(ch >= '0' && ch <= '9') || ch == '-' {
				m.syncDeviceInput += strings.ToUpper(string(ch))
			}
		}
		return m, nil
	}
}

func (m Model) handleLndHubCreateNameKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if len(m.hubNameInput) > 0 {
			m.hubNameInput = m.hubNameInput[:len(m.hubNameInput)-1]
		} else {
			m.subview = svLndHubManage
		}
		return m, nil
	case "enter":
		if m.hubNameInput != "" {
			return m, createLndHubAccountCmd(m.cfg.LndHubAdminToken)
		}
		return m, nil
	default:
		if isAllowedHubNameChar(key) && len(m.hubNameInput) < 30 {
			m.hubNameInput += key
		}
		return m, nil
	}
}

func (m Model) handleLndHubManageKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "c":
		m.hubNameInput = ""
		m.subview = svLndHubCreateName
		return m, nil
	case "u":
		hubOnion := readOnion(paths.TorLndHubHostname)
		if hubOnion != "" {
			m.urlTarget = "http://" + hubOnion + ":" + paths.LndHubExternalPort
			m.urlReturnTo = svLndHubManage
			m.subview = svFullURL
		}
		return m, nil
	case "x":
		if len(m.cfg.LndHubAccounts) > 0 &&
			m.hubCursor < len(m.cfg.LndHubAccounts) &&
			m.cfg.LndHubAccounts[m.hubCursor].Active {
			m.subview = svLndHubDeactivateConfirm
		}
		return m, nil
	case "enter":
		if len(m.cfg.LndHubAccounts) > 0 {
			m.subview = svLndHubAccountDetail
		}
		return m, nil
	case "up", "k":
		if m.hubCursor > 0 {
			m.hubCursor--
		}
		return m, nil
	case "down", "j":
		if m.hubCursor < len(m.cfg.LndHubAccounts)-1 {
			m.hubCursor++
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleLndHubDeactivateKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y":
		if m.hubCursor < len(m.cfg.LndHubAccounts) {
			login := m.cfg.LndHubAccounts[m.hubCursor].Login
			return m, deactivateLndHubAccountCmd(login)
		}
		return m, nil
	case "n":
		m.subview = svLndHubManage
		return m, nil
	}
	return m, nil
}
