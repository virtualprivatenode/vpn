package welcome

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func isWalletSubview(sv wSubview) bool {
	switch sv {
	case svReceive, svReceiveWaiting, svReceivePaid, svReceiveExpired,
		svSend, svSendConfirm, svSendInFlight, svSendResult,
		svPaymentHistory, svPaymentDetail:
		return true
	}
	return false
}

func (m Model) handleWalletKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	case svPaymentHistory:
		return m.handlePaymentHistoryKey(key)
	case svPaymentDetail:
		return m.handlePaymentDetailKey(key)
	}
	return m, nil
}

// ── Receive ──────────────────────────────────────────────

func (m Model) handleReceiveKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.resetReceiveState()
		m.subview = svNone
		return m, nil
	case "tab":
		// Toggle focus between amount and memo
		if m.recvAmountInput.Focused() {
			m.recvAmountInput.Blur()
			m.recvMemoInput.Focus()
		} else {
			m.recvMemoInput.Blur()
			m.recvAmountInput.Focus()
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
		// Forward to focused textinput (cast to tea.Msg for full event handling)
		var cmd tea.Cmd
		if m.recvAmountInput.Focused() {
			m.recvAmountInput, cmd = m.recvAmountInput.Update(tea.Msg(msg))
		} else {
			m.recvMemoInput, cmd = m.recvMemoInput.Update(tea.Msg(msg))
		}
		return m, cmd
	}
}

func (m Model) handleReceiveWaitingKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
		m.resetReceiveState()
		m.subview = svNone
		return m, nil
	case "f":
		if m.recvPayReq != "" {
			m.urlTarget = m.recvPayReq
			m.urlReturnTo = svReceiveWaiting
			m.subview = svFullURL
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleReceivePaidKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "backspace":
		m.resetReceiveState()
		m.subview = svNone
		return m, fetchStatus(m.cfg, m.lndClient)
	}
	return m, nil
}

func (m Model) handleReceiveExpiredKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "backspace":
		m.resetReceiveState()
		m.subview = svNone
		return m, nil
	}
	return m, nil
}

// ── Send ─────────────────────────────────────────────────

func (m Model) handleSendKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.resetSendState()
		m.subview = svNone
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
		m.sendInput, cmd = m.sendInput.Update(tea.Msg(msg))
		return m, cmd
	}
}

func (m Model) handleSendConfirmKey(key string) (tea.Model, tea.Cmd) {
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
			m.lndClient, strings.TrimSpace(m.sendInput.Value()))
	}
	return m, nil
}

func (m Model) handleSendInFlightKey(key string) (tea.Model, tea.Cmd) {
	if key == "q" || key == "ctrl+c" {
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleSendResultKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "backspace":
		m.resetSendState()
		m.subview = svNone
		return m, fetchStatus(m.cfg, m.lndClient)
	}
	return m, nil
}

// ── Payment History ──────────────────────────────────────

func (m Model) handlePaymentHistoryKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
		m.subview = svNone
		return m, nil
	case "up", "k":
		if m.payHistoryCursor > 0 {
			m.payHistoryCursor--
		}
		return m, nil
	case "down", "j":
		if m.payHistoryCursor < len(m.payHistory)-1 {
			m.payHistoryCursor++
		}
		return m, nil
	case "enter":
		if len(m.payHistory) > 0 &&
			m.payHistoryCursor < len(m.payHistory) {
			m.subview = svPaymentDetail
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handlePaymentDetailKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
		m.subview = svPaymentHistory
		return m, nil
	}
	return m, nil
}

// ── State reset helpers ──────────────────────────────────

func (m *Model) resetReceiveState() {
	m.recvAmountInput = newRecvAmountInput()
	m.recvMemoInput = newRecvMemoInput()
	m.recvPayReq = ""
	m.recvPaymentHash = ""
	m.recvAmountSats = 0
	m.recvSettled = false
	m.recvExpired = false
	m.recvError = ""
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

// ── Input helpers ────────────────────────────────────────

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
		return 0, fmt.Errorf("minimum amount is 1 sat")
	}
	return n, nil
}

// isBolt11Char returns true if the rune is valid in a bolt11 invoice.
func isBolt11Char(ch rune) bool {
	if ch >= 'a' && ch <= 'z' {
		return true
	}
	if ch >= '0' && ch <= '9' {
		return true
	}
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	return false
}

// cleanPayReq strips common paste artifacts from a payment request.
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
