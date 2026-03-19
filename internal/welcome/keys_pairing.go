package welcome

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/paths"
)

func (m Model) handlePairingKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		switch m.subview {
		case svQR:
			if m.qrLabel != "" {
				m.subview = svLndHubCreateAccount
			} else {
				m.subview = svZeus
			}
			m.qrLabel = ""
		case svFullURL:
			if m.urlReturnTo != svNone {
				m.subview = m.urlReturnTo
				m.urlReturnTo = svNone
			} else {
				m.subview = svNone
			}
		default:
			m.subview = svNone
		}
		return m, nil
	}

	switch m.subview {
	case svZeus:
		return m.handleZeusKey(key)
	}
	return m, nil
}

func (m Model) handleZeusKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "m":
		return m, showMacaroonCmd(m.cfg)
	case "r":
		m.qrMode = "tor"
		m.qrLabel = ""
		m.subview = svQR
		return m, nil
	case "c":
		if m.cfg.P2PMode == "hybrid" {
			m.qrMode = "clearnet"
			m.qrLabel = ""
			m.subview = svQR
			return m, nil
		}
	}
	return m, nil
}

// handleLndHubQRKeys handles QR-related keys from the LndHub
// new account screen. Called from keys_addons.go.
func (m Model) handleLndHubQRKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "r":
		if m.lastAccount != nil {
			hubOnion := readOnion(paths.TorLndHubHostname)
			if hubOnion != "" {
				m.urlTarget = fmt.Sprintf("lndhub://%s:%s@http://%s:%s",
					m.lastAccount.Login, m.lastAccount.Password,
					hubOnion, paths.LndHubExternalPort)
				m.qrLabel = m.hubNameInput + " — Tor"
				m.subview = svQR
			}
		}
		return m, nil
	case "c":
		if m.cfg.P2PMode == "hybrid" && m.lastAccount != nil {
			ip := ""
			if m.status != nil {
				ip = m.status.publicIP
			}
			if ip != "" {
				m.urlTarget = fmt.Sprintf("lndhub://%s:%s@https://%s:%s",
					m.lastAccount.Login, m.lastAccount.Password,
					ip, paths.LndHubExternalPort)
				m.qrLabel = m.hubNameInput + " — Clearnet"
				m.subview = svQR
			}
		}
		return m, nil
	}
	return m, nil
}
