package welcome

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) handleChannelsKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svChannelOpen:
		return m.handleChannelOpenKey(key)
	case svChannelCustomPeer:
		return m.handleChannelCustomPeerKey(key, msg)
	case svChannelAmountSelect:
		return m.handleChannelAmountKey(key, msg)
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
	case "esc", "backspace":
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
			m.chanPubkeyInput = newChanPubkeyInput()
			m.chanHostInput = newChanHostInput()
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
			m.chanAmountInput = newChanAmountInput()
			m.chanOpenError = ""
			m.subview = svChannelAmountSelect
			return m, nil
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleChannelCustomPeerKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.subview = svChannelOpen
		m.chanOpenError = ""
		return m, nil
	case "tab":
		if m.chanPubkeyInput.Focused() {
			m.chanPubkeyInput.Blur()
			m.chanHostInput.Focus()
		} else {
			m.chanHostInput.Blur()
			m.chanPubkeyInput.Focus()
		}
		return m, nil
	case "enter":
		pubkey := m.chanPubkeyInput.Value()
		host := m.chanHostInput.Value()
		if pubkey == "" {
			m.chanOpenError = "Pubkey is required"
			return m, nil
		}
		if len(pubkey) != 66 {
			m.chanOpenError = "Pubkey must be 66 hex characters"
			return m, nil
		}
		if host == "" {
			m.chanOpenError = "Host is required (e.g., 1.2.3.4:9735)"
			return m, nil
		}
		m.chanOpenPubkey = pubkey
		m.chanOpenHost = host
		m.chanOpenAlias = pubkey[:16] + "..."
		m.chanOpenError = ""
		m.chanAmountPreset = 0
		m.chanAmountInput = newChanAmountInput()
		m.subview = svChannelAmountSelect
		return m, nil
	default:
		var cmd tea.Cmd
		if m.chanPubkeyInput.Focused() {
			m.chanPubkeyInput, cmd = m.chanPubkeyInput.Update(tea.Msg(msg))
		} else {
			m.chanHostInput, cmd = m.chanHostInput.Update(tea.Msg(msg))
		}
		return m, cmd
	}
}

func (m Model) handleChannelAmountKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	isCustomSelected := m.chanAmountPreset == len(amountPresets)-1

	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.subview = svChannelOpen
		m.chanOpenError = ""
		return m, nil
	case "up", "k":
		if !isCustomSelected {
			if m.chanAmountPreset > 0 {
				m.chanAmountPreset--
				m.chanOpenError = ""
			}
			return m, nil
		}
	case "down", "j":
		if !isCustomSelected {
			if m.chanAmountPreset < len(amountPresets)-1 {
				m.chanAmountPreset++
				m.chanOpenError = ""
			}
			return m, nil
		}
	case "backspace":
		if isCustomSelected && m.chanAmountInput.Value() != "" {
			// Fall through to textinput update below
		} else if !isCustomSelected {
			m.subview = svChannelOpen
			m.chanOpenError = ""
			return m, nil
		} else {
			// Custom selected but empty — go back
			m.subview = svChannelOpen
			m.chanOpenError = ""
			return m, nil
		}
	case "enter":
		if isCustomSelected {
			amt, err := parseCustomAmount(m.chanAmountInput.Value())
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
	}
	if isCustomSelected {
		var cmd tea.Cmd
		m.chanAmountInput, cmd = m.chanAmountInput.Update(tea.Msg(msg))
		return m, cmd
	}
	return m, nil
}

func (m Model) handleChannelConfirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
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
	case "enter", "esc", "backspace":
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
	case "esc", "backspace":
		m.subview = svNone
		m.chanFundAddress = ""
		return m, nil
	}
	return m, nil
}
