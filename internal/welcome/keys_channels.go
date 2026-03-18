// internal/welcome/keys_channels.go

package welcome

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleChannelsKey(key string) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svChannelOpen:
		return m.handleChannelOpenKey(key)
	case svChannelCustomPeer:
		return m.handleChannelCustomPeerKey(key)
	case svChannelAmountSelect:
		return m.handleChannelAmountKey(key)
	case svChannelOpenConfirm:
		return m.handleChannelConfirmKey(key)
	case svChannelOpening:
		return m.handleChannelOpeningKey(key)
	case svChannelOpenResult:
		return m.handleChannelResultKey(key)
	case svChannelFundWallet:
		return m.handleChannelFundKey(key)
	}
	return m, nil
}

func (m Model) handleChannelOpenKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		m.subview = svNone
		m.chanOpenError = ""
		return m, nil
	case "up", "k":
		if m.chanOpenPeerIdx > 0 {
			m.chanOpenPeerIdx--
		}
		return m, nil
	case "down", "j":
		if m.chanOpenPeerIdx < len(m.chanPeerList) {
			m.chanOpenPeerIdx++
		}
		return m, nil
	case "enter":
		customIdx := len(m.chanPeerList)
		if m.chanOpenPeerIdx == customIdx {
			m.chanCustomPubkey = ""
			m.chanCustomHost = ""
			m.chanCustomInputField = 0
			m.chanOpenError = ""
			m.subview = svChannelCustomPeer
			return m, nil
		}
		if m.chanOpenPeerIdx < len(m.chanPeerList) {
			peer := m.chanPeerList[m.chanOpenPeerIdx]
			m.chanOpenPubkey = peer.Pubkey
			m.chanOpenHost = peer.Host
			m.chanOpenAlias = peer.Alias
			m.chanAmountPreset = 0
			m.chanCustomAmountStr = ""
			m.chanOpenError = ""
			m.subview = svChannelAmountSelect
			return m, nil
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleChannelCustomPeerKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if m.chanCustomInputField == 0 {
			if len(m.chanCustomPubkey) > 0 {
				m.chanCustomPubkey = m.chanCustomPubkey[:len(m.chanCustomPubkey)-1]
			} else {
				m.subview = svChannelOpen
				m.chanOpenError = ""
			}
		} else {
			if len(m.chanCustomHost) > 0 {
				m.chanCustomHost = m.chanCustomHost[:len(m.chanCustomHost)-1]
			}
		}
		return m, nil
	case "tab":
		m.chanCustomInputField = (m.chanCustomInputField + 1) % 2
		return m, nil
	case "enter":
		if m.chanCustomPubkey == "" {
			m.chanOpenError = "Pubkey is required"
			return m, nil
		}
		if len(m.chanCustomPubkey) != 66 {
			m.chanOpenError = "Pubkey must be 66 hex characters"
			return m, nil
		}
		if m.chanCustomHost == "" {
			m.chanOpenError = "Host is required (e.g., 1.2.3.4:9735)"
			return m, nil
		}
		m.chanOpenPubkey = m.chanCustomPubkey
		m.chanOpenHost = m.chanCustomHost
		m.chanOpenAlias = m.chanCustomPubkey[:16] + "..."
		m.chanOpenError = ""
		m.chanAmountPreset = 0
		m.chanCustomAmountStr = ""
		m.subview = svChannelAmountSelect
		return m, nil
	default:
		if m.chanCustomInputField == 0 {
			for _, ch := range key {
				if len(m.chanCustomPubkey) < 66 && isHexChar(byte(ch)) {
					m.chanCustomPubkey += string(ch)
				}
			}
		} else {
			for _, ch := range key {
				if len(m.chanCustomHost) < 80 {
					m.chanCustomHost += string(ch)
				}
			}
		}
		return m, nil
	}
}

func (m Model) handleChannelAmountKey(key string) (tea.Model, tea.Cmd) {
	isCustomSelected := m.chanAmountPreset == len(amountPresets)-1

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if isCustomSelected && len(m.chanCustomAmountStr) > 0 {
			m.chanCustomAmountStr = m.chanCustomAmountStr[:len(m.chanCustomAmountStr)-1]
			m.chanOpenError = ""
			return m, nil
		}
		m.subview = svChannelOpen
		m.chanCustomAmountStr = ""
		m.chanOpenError = ""
		return m, nil
	case "up", "k":
		if m.chanAmountPreset > 0 {
			m.chanAmountPreset--
			m.chanOpenError = ""
		}
		return m, nil
	case "down", "j":
		if m.chanAmountPreset < len(amountPresets)-1 {
			m.chanAmountPreset++
			m.chanOpenError = ""
		}
		return m, nil
	case "enter":
		if isCustomSelected {
			amt, err := parseCustomAmount(m.chanCustomAmountStr)
			if err != nil {
				m.chanOpenError = err.Error()
				return m, nil
			}
			m.chanOpenAmount = amt
		} else {
			m.chanOpenAmount = amountPresets[m.chanAmountPreset]
		}
		m.chanOpenPrivate = true
		m.chanOpenError = ""
		m.subview = svChannelOpenConfirm
		return m, nil
	default:
		if isCustomSelected {
			for _, ch := range key {
				if ch >= '0' && ch <= '9' && len(m.chanCustomAmountStr) < 10 {
					m.chanCustomAmountStr += string(ch)
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleChannelConfirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		m.chanOpenError = ""
		m.subview = svChannelAmountSelect
		return m, nil
	case "p":
		m.chanOpenPrivate = !m.chanOpenPrivate
		return m, nil
	case "y":
		if m.chanOpenInFlight {
			return m, nil
		}
		m.chanOpenInFlight = true
		m.chanOpenError = ""
		m.subview = svChannelOpening
		return m, openChannelCmd(
			m.lndClient, m.chanOpenPubkey, m.chanOpenHost,
			m.chanOpenAmount, m.chanOpenPrivate)
	}
	return m, nil
}

func (m Model) handleChannelOpeningKey(key string) (tea.Model, tea.Cmd) {
	if key == "q" || key == "ctrl+c" {
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleChannelResultKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter", "backspace":
		m.subview = svNone
		m.chanOpenError = ""
		m.chanOpenTxid = ""
		m.chanOpenInFlight = false
		return m, fetchStatus(m.cfg, m.lndClient)
	}
	return m, nil
}

func (m Model) handleChannelFundKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		m.subview = svNone
		m.chanFundAddress = ""
		return m, nil
	}
	return m, nil
}
