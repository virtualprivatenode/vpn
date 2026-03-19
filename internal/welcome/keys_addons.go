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
			m.hubNameInput = newHubNameInput()
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
		return m, nil
	case svSyncthingDeviceQR:
		return m, nil
	case svLndHubManage:
		return m.handleLndHubManageKey(key)
	case svLndHubCreateAccount:
		return m.handleLndHubQRKeys(key)
	case svLndHubAccountDetail:
		return m, nil
	case svLndHubDeactivateConfirm:
		return m.handleLndHubDeactivateKey(key)
	}
	return m, nil
}

func (m Model) handleSyncthingDetailKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "a":
		m.syncDeviceInput = newSyncthingIDInput()
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

func (m Model) handleSyncthingPairInputKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.syncPairError = ""
		m.syncPairSuccess = false
		m.subview = svSyncthingDetail
		return m, nil
	case "enter":
		if m.syncPairSuccess {
			m.syncDeviceInput = newSyncthingIDInput()
			m.syncPairSuccess = false
			m.subview = svSyncthingDetail
			return m, nil
		}
		deviceID := syncthingIDValue(m.syncDeviceInput)
		if deviceID != "" {
			parts := strings.Split(deviceID, "-")
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
			return m, pairSyncthingDeviceCmd(deviceID)
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.syncDeviceInput, cmd = m.syncDeviceInput.Update(tea.Msg(msg))
		return m, cmd
	}
}

func (m Model) handleLndHubCreateNameKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.subview = svLndHubManage
		return m, nil
	case "enter":
		name := m.hubNameInput.Value()
		if name != "" {
			return m, createLndHubAccountCmd(m.cfg.LndHubAdminToken)
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.hubNameInput, cmd = m.hubNameInput.Update(tea.Msg(msg))
		return m, cmd
	}
}

func (m Model) handleLndHubManageKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "c":
		m.hubNameInput = newHubNameInput()
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
