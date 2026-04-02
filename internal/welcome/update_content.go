package welcome

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ripsline/virtual-private-node/internal/paths"
)

// ── Syncthing flow handlers ──────────────────────────────

func (m Model) handleSyncWebUIKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left":
		if m.addonBtnIdx > 0 {
			m.addonBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "right":
		if m.addonBtnIdx < 1 {
			m.addonBtnIdx++
		}
	case "up":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
	case "enter":
		switch m.addonBtnIdx {
		case 0:
			syncOnion := readOnion(
				paths.TorSyncthingHostname)
			if syncOnion != "" {
				m.urlTarget = "http://" +
					syncOnion + ":8384"
				m.urlReturnTo = svSyncthingWebUI
				m.subview = svFullURL
			}
		case 1:
			m.showSecrets = !m.showSecrets
		}
	case "backspace":
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

func (m Model) handleSyncthingPairInputKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	// Post-pair screen: button navigation only
	if m.syncPairSuccess {
		return m.handleSyncPostPairKey(key)
	}

	// Pre-pair screen: input (contentFocus=0) +
	// buttons (contentFocus=1)
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left":
		if m.contentFocus() == 1 {
			if m.addonBtnIdx > 0 {
				m.addonBtnIdx--
			}
			return m, nil
		}
		if m.syncDeviceInput.Value() != "" {
			var cmd tea.Cmd
			m.syncDeviceInput, cmd =
				m.syncDeviceInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		m.focusSidebar()
		return m, nil
	case "right":
		if m.contentFocus() == 1 {
			if m.addonBtnIdx < 1 {
				m.addonBtnIdx++
			}
			return m, nil
		}
		if m.syncDeviceInput.Value() != "" {
			var cmd tea.Cmd
			m.syncDeviceInput, cmd =
				m.syncDeviceInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		return m, nil
	case "up":
		if m.contentFocus() == 1 {
			m.setContentFocus(0)
			m.syncDeviceInput.Focus()
			return m, nil
		}
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "down", "tab":
		if m.contentFocus() == 0 {
			m.setContentFocus(1)
			m.addonBtnIdx = 1 // default to Pair
			m.syncDeviceInput.Blur()
			return m, nil
		}
		return m, nil
	case "backspace":
		if m.contentFocus() == 1 {
			m.setContentFocus(0)
			m.syncDeviceInput.Focus()
			return m, nil
		}
		if m.syncDeviceInput.Value() != "" {
			var cmd tea.Cmd
			m.syncDeviceInput, cmd =
				m.syncDeviceInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		m.syncPairError = ""
		return m.closeTab(m.activeTab)
	case "enter":
		if m.contentFocus() == 1 {
			switch m.addonBtnIdx {
			case 0: // Clear
				m.syncDeviceInput =
					newSyncthingIDInput()
				m.syncPairError = ""
				m.setContentFocus(0)
				return m, nil
			case 1: // Pair
				return m.submitSyncthingPair()
			}
			return m, nil
		}
		// Enter in input field → submit
		return m.submitSyncthingPair()
	default:
		if m.contentFocus() == 0 {
			var cmd tea.Cmd
			m.syncDeviceInput, cmd =
				m.syncDeviceInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
	}
	return m, nil
}

// submitSyncthingPair validates and submits the pair.
func (m Model) submitSyncthingPair() (
	tea.Model, tea.Cmd,
) {
	deviceID := syncthingIDValue(
		m.syncDeviceInput)
	if deviceID == "" {
		m.syncPairError = "Paste a Device ID"
		return m, nil
	}
	parts := strings.Split(deviceID, "-")
	if len(parts) != 8 {
		m.syncPairError =
			"Invalid format. Expected 8 groups" +
				" separated by hyphens."
		return m, nil
	}
	for _, p := range parts {
		if len(p) != 7 {
			m.syncPairError =
				"Invalid format. Each group" +
					" should be 7 characters."
			return m, nil
		}
	}
	// Check for duplicate
	for _, d := range m.cfg.SyncthingDevices {
		if d.DeviceID == deviceID {
			m.syncPairError =
				"Device already paired."
			return m, nil
		}
	}
	m.syncPairError = ""
	return m, pairSyncthingDeviceCmd(deviceID)
}

// handleSyncPostPairKey handles the post-pair screen
// with Show QR / Done buttons.
func (m Model) handleSyncPostPairKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left":
		if m.addonBtnIdx > 0 {
			m.addonBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "right":
		if m.addonBtnIdx < 1 {
			m.addonBtnIdx++
		}
		return m, nil
	case "up":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "enter":
		switch m.addonBtnIdx {
		case 0: // Show QR
			m.subview = svSyncthingPairQR
		case 1: // Done
			m.syncPairSuccess = false
			return m.closeTab(m.activeTab)
		}
		return m, nil
	case "backspace":
		m.syncPairSuccess = false
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

// handleSyncPairQRKey handles the QR code subview
// with a single Back button.
func (m Model) handleSyncPairQRKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left":
		m.focusSidebar()
		return m, nil
	case "up":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
	case "enter", "backspace":
		m.subview = svSyncthingPairInput
		m.addonBtnIdx = 0
		return m, nil
	}
	return m, nil
}

// ── LndHub create name handler ───────────────────────────

func (m Model) handleLndHubCreateNameKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left":
		// Buttons: move left
		if m.contentFocus() == 1 &&
			m.hubCreateBtnIdx > 0 {
			m.hubCreateBtnIdx--
			return m, nil
		}
		// Input: pass through for cursor
		if m.contentFocus() == 0 {
			if m.hubNameInput.Value() != "" {
				var cmd tea.Cmd
				m.hubNameInput, cmd =
					m.hubNameInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		m.focusSidebar()
		return m, nil
	case "right":
		// Buttons: move right
		if m.contentFocus() == 1 &&
			m.hubCreateBtnIdx < 1 {
			m.hubCreateBtnIdx++
			return m, nil
		}
		// Input: pass through for cursor
		if m.contentFocus() == 0 {
			if m.hubNameInput.Value() != "" {
				var cmd tea.Cmd
				m.hubNameInput, cmd =
					m.hubNameInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		return m, nil
	case "up":
		if m.contentFocus() == 1 {
			m.setContentFocus(0)
			m.hubNameInput.Focus()
			return m, nil
		}
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "down", "tab":
		if m.contentFocus() == 0 {
			m.setContentFocus(1)
			m.hubCreateBtnIdx = 1 // default to Create
			m.hubNameInput.Blur()
			return m, nil
		}
		return m, nil
	case "backspace":
		// Buttons: go back to input
		if m.contentFocus() == 1 {
			m.setContentFocus(0)
			m.hubNameInput.Focus()
			return m, nil
		}
		if m.hubNameInput.Value() != "" {
			var cmd tea.Cmd
			m.hubNameInput, cmd =
				m.hubNameInput.Update(tea.Msg(msg))
			return m, cmd
		}
		m.subview = svLndHubManage
		return m, nil
	case "enter":
		// Buttons
		if m.contentFocus() == 1 {
			switch m.hubCreateBtnIdx {
			case 0: // Clear
				m.hubNameInput = newHubNameInput()
				m.setContentFocus(0)
				return m, nil
			case 1: // Create Account
				name := m.hubNameInput.Value()
				if name != "" {
					return m, createLndHubAccountCmd(
						m.cfg.LndHubAdminToken)
				}
			}
			return m, nil
		}
		// Enter in input field → submit
		name := m.hubNameInput.Value()
		if name != "" {
			return m, createLndHubAccountCmd(
				m.cfg.LndHubAdminToken)
		}
		return m, nil
	default:
		if m.contentFocus() == 0 {
			var cmd tea.Cmd
			m.hubNameInput, cmd =
				m.hubNameInput.Update(tea.Msg(msg))
			return m, cmd
		}
	}
	return m, nil
}

// ── Parse helpers ────────────────────────────────────────

func parseRecvAmount(s string) (int64, error) {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty amount")
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid number")
		}
		n = n*10 + int64(c-'0')
	}
	if n < 1 {
		return 0, fmt.Errorf("minimum 1 sat")
	}
	return n, nil
}

func cleanPayReq(s string) string {
	s = strings.ReplaceAll(s, "[", "")
	s = strings.ReplaceAll(s, "]", "")
	s = strings.ReplaceAll(s, "\"", "")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "lightning:")
	s = strings.TrimPrefix(s, "LIGHTNING:")
	return s
}
