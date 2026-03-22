package welcome

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func isWalletSubview(sv wSubview) bool {
	switch sv {
	case svReceive, svReceiveWaiting, svReceivePaid,
		svReceiveExpired, svSend, svSendConfirm,
		svSendInFlight, svSendResult:
		return true
	}
	return false
}

func (m Model) handleWalletKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch m.subview {
	case svReceive:
		return m.handleReceiveKey(key, msg)
	case svReceiveWaiting:
		return m.handleReceiveWaitingKey(key)
	case svReceivePaid:
		return m.handleReceivePaidKey(key)
	case svReceiveExpired:
		return m.handleReceiveExpiredKey(key)
	case svSend:
		return m.handleSendKey(key, msg)
	case svSendConfirm:
		return m.handleSendConfirmKey(key)
	case svSendInFlight:
		return m.handleSendInFlightKey(key)
	case svSendResult:
		return m.handleSendResultKey(key)
	}
	return m, nil
}

func (m *Model) returnToSidebar() {
	m.subview = svNone
	m.btnIdx = 0
	m.contentFocused = false
	m.nav.Focus()
}

func (m Model) handleReceiveKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.resetReceiveState()
		m.subview = svNone
		m.btnIdx = 0
		return m, nil
	case "backspace":
		m.resetReceiveState()
		m.subview = svNone
		m.btnIdx = 0
		return m, nil
	case "tab", "down":
		if m.recvAmountInput.Focused() {
			m.recvAmountInput.Blur()
			m.recvMemoInput.Focus()
		} else {
			m.recvMemoInput.Blur()
			m.recvAmountInput.Focus()
		}
		return m, nil
	case "up":
		if m.recvMemoInput.Focused() {
			m.recvMemoInput.Blur()
			m.recvAmountInput.Focus()
		} else {
			// At top field, go to tab bar
			if m.hasDetailTabs() {
				m.tabFocused = true
				m.contentFocused = false
				m.tabCursorX = 0
				m.activeTab = m.findFlowTab()
				return m, nil
			}
		}
		return m, nil
	case "enter":
		val := m.recvAmountInput.Value()
		if val == "" {
			m.recvError = "Enter an amount"
			return m, nil
		}
		amt, err := parseRecvAmount(val)
		if err != nil {
			m.recvError = err.Error()
			return m, nil
		}
		m.recvAmountSats = amt
		m.recvError = ""
		return m, createInvoiceCmd(
			m.lndClient, amt, m.recvMemoInput.Value())
	default:
		var cmd tea.Cmd
		if m.recvAmountInput.Focused() {
			m.recvAmountInput, cmd =
				m.recvAmountInput.Update(tea.Msg(msg))
		} else {
			m.recvMemoInput, cmd =
				m.recvMemoInput.Update(tea.Msg(msg))
		}
		return m, cmd
	}
}

func (m Model) handleReceiveWaitingKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
		m.resetReceiveState()
		m.subview = svNone
		m.btnIdx = 0
		return m, nil
	case "left":
		if m.recvButtonIdx > 0 {
			m.recvButtonIdx--
		}
		return m, nil
	case "right":
		if m.recvButtonIdx < 1 {
			m.recvButtonIdx++
		}
		return m, nil
	case "enter":
		if m.recvButtonIdx == 0 && m.recvPayReq != "" {
			m.urlTarget = m.recvPayReq
			m.qrLabel = fmt.Sprintf(
				"Invoice — %s sats",
				formatSats(m.recvAmountSats))
			m.urlReturnTo = svReceiveWaiting
			m.subview = svQR
			return m, nil
		}
		if m.recvButtonIdx == 1 && m.recvPayReq != "" {
			m.urlTarget = m.recvPayReq
			m.urlReturnTo = svReceiveWaiting
			m.subview = svFullURL
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleReceivePaidKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "backspace":
		m.resetReceiveState()
		m.subview = svNone
		m.btnIdx = 0
		m.nav.SetActive(secWallet)
		m.contentFocused = true
		m.nav.Blur()
		return m, tea.Sequence(
			fetchStatus(m.cfg, m.lndClient),
			fetchPaymentHistoryCmd(m.lndClient))
	}
	return m, nil
}

func (m Model) handleReceiveExpiredKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "backspace":
		m.resetReceiveState()
		m.subview = svNone
		m.btnIdx = 0
		return m, nil
	}
	return m, nil
}

func (m Model) handleSendKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
		m.resetSendState()
		m.subview = svNone
		m.btnIdx = 0
		return m, nil
	case "enter":
		payReq := strings.TrimSpace(m.sendInput.Value())
		if payReq == "" {
			m.sendError = "Paste a payment request"
			return m, nil
		}
		payReq = cleanPayReq(payReq)
		m.sendInput.SetValue(payReq)
		if !strings.HasPrefix(payReq, "lnbc") &&
			!strings.HasPrefix(payReq, "lntb") &&
			!strings.HasPrefix(payReq, "lnbcrt") {
			m.sendError = "Not a valid Lightning invoice"
			return m, nil
		}
		m.sendError = ""
		return m, decodePayReqCmd(m.lndClient, payReq)
	default:
		var cmd tea.Cmd
		m.sendInput, cmd =
			m.sendInput.Update(tea.Msg(msg))
		return m, cmd
	}
}

func (m Model) handleSendConfirmKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
		m.sendError = ""
		m.subview = svSend
		return m, nil
	case "y":
		if m.sendInFlight {
			return m, nil
		}
		m.sendInFlight = true
		m.sendError = ""
		m.subview = svSendInFlight
		return m, sendPaymentCmd(
			m.lndClient,
			strings.TrimSpace(m.sendInput.Value()))
	}
	return m, nil
}

func (m Model) handleSendInFlightKey(
	key string,
) (tea.Model, tea.Cmd) {
	if key == "q" || key == "ctrl+c" {
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleSendResultKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "backspace":
		m.resetSendState()
		m.subview = svNone
		m.btnIdx = 0
		m.nav.SetActive(secWallet)
		m.contentFocused = true
		m.nav.Blur()
		return m, tea.Sequence(
			fetchStatus(m.cfg, m.lndClient),
			fetchPaymentHistoryCmd(m.lndClient))
	}
	return m, nil
}

func (m Model) handleChannelsKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
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

func (m Model) handleChannelOpenKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
		m.subview = svNone
		m.chanOpenError = ""
		m.btnIdx = 0
		return m, nil
	case "up", "k":
		if m.chanOpenPeerIdx > 0 {
			m.chanOpenPeerIdx--
		}
	case "down", "j":
		if m.chanOpenPeerIdx < len(m.chanPeerList) {
			m.chanOpenPeerIdx++
		}
	case "enter":
		customIdx := len(m.chanPeerList)
		if m.chanOpenPeerIdx == customIdx {
			m.chanPubkeyInput = newChanPubkeyInput()
			m.chanHostInput = newChanHostInput()
			cw := min(m.width, 96) - m.nav.Width - 5
			if cw > 66 {
				cw = 66
			}
			if cw < 20 {
				cw = 20
			}
			m.chanPubkeyInput.SetWidth(cw)
			m.chanHostInput.SetWidth(cw)
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
		}
	}
	return m, nil
}

func (m Model) handleChannelCustomPeerKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.subview = svChannelOpen
		m.chanOpenError = ""
		return m, nil
	case "tab", "down":
		if m.chanPubkeyInput.Focused() {
			m.chanPubkeyInput.Blur()
			m.chanHostInput.Focus()
		} else {
			m.chanHostInput.Blur()
			m.chanPubkeyInput.Focus()
		}
		return m, nil
	case "up":
		if m.chanHostInput.Focused() {
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
			m.chanOpenError = "Pubkey must be 66 hex chars"
			return m, nil
		}
		if host == "" {
			m.chanOpenError = "Host required"
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
			m.chanPubkeyInput, cmd =
				m.chanPubkeyInput.Update(tea.Msg(msg))
		} else {
			m.chanHostInput, cmd =
				m.chanHostInput.Update(tea.Msg(msg))
		}
		return m, cmd
	}
}

func (m Model) handleChannelAmountKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	isCustom := m.chanAmountPreset == len(amountPresets)-1
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.subview = svChannelOpen
		m.chanOpenError = ""
		return m, nil
	case "up", "k":
		if !isCustom && m.chanAmountPreset > 0 {
			m.chanAmountPreset--
			m.chanOpenError = ""
		}
	case "down", "j":
		if !isCustom &&
			m.chanAmountPreset < len(amountPresets)-1 {
			m.chanAmountPreset++
			m.chanOpenError = ""
		}
	case "backspace":
		if isCustom && m.chanAmountInput.Value() != "" {
			// fall through to textinput
		} else {
			m.subview = svChannelOpen
			m.chanOpenError = ""
			return m, nil
		}
	case "enter":
		if isCustom {
			amt, err := parseCustomAmount(
				m.chanAmountInput.Value())
			if err != nil {
				m.chanOpenError = err.Error()
				return m, nil
			}
			m.chanOpenAmount = amt
		} else {
			m.chanOpenAmount =
				amountPresets[m.chanAmountPreset]
		}
		m.chanOpenPrivate = true
		m.chanOpenError = ""
		m.subview = svChannelOpenConfirm
		return m, nil
	}
	if isCustom {
		var cmd tea.Cmd
		m.chanAmountInput, cmd =
			m.chanAmountInput.Update(tea.Msg(msg))
		return m, cmd
	}
	return m, nil
}

func (m Model) handleChannelConfirmKey(
	key string,
) (tea.Model, tea.Cmd) {
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
			m.lndClient, m.chanOpenPubkey,
			m.chanOpenHost, m.chanOpenAmount,
			m.chanOpenPrivate)
	}
	return m, nil
}

func (m Model) handleChannelOpeningKey(
	key string,
) (tea.Model, tea.Cmd) {
	if key == "q" || key == "ctrl+c" {
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleChannelResultKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "backspace":
		m.subview = svNone
		m.chanOpenError = ""
		m.chanOpenTxid = ""
		m.chanOpenInFlight = false
		m.btnIdx = 0
		m.nav.SetActive(secChannels)
		m.nav.Focus()
		m.contentFocused = false
		return m, fetchStatus(m.cfg, m.lndClient)
	}
	return m, nil
}

func (m Model) handleChannelFundKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
		m.subview = svNone
		m.chanFundAddress = ""
		m.btnIdx = 0
		return m, nil
	}
	return m, nil
}

func (m *Model) resetReceiveState() {
	m.recvAmountInput = newRecvAmountInput()
	m.recvMemoInput = newRecvMemoInput()
	m.recvPayReq = ""
	m.recvPaymentHash = ""
	m.recvAmountSats = 0
	m.recvSettled = false
	m.recvExpired = false
	m.recvError = ""
	m.recvButtonIdx = 0
}

func (m *Model) resetSendState() {
	m.sendInput = newSendPayReqInput()
	m.sendDecodedValid = false
	m.sendDecodedDesc = ""
	m.sendDecodedAmt = 0
	m.sendDecodedDest = ""
	m.sendDecodedExp = ""
	m.sendInFlight = false
	m.sendError = ""
	m.sendPreimage = ""
	m.sendRouteHops = nil
	m.sendFeeSats = 0
}

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

func isBolt11Char(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= '0' && ch <= '9') ||
		(ch >= 'A' && ch <= 'Z')
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
