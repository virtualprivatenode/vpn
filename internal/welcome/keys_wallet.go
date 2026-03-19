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
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if m.recvInputField == 0 {
			if len(m.recvAmountStr) > 0 {
				m.recvAmountStr = m.recvAmountStr[:len(m.recvAmountStr)-1]
			} else {
				m.recvError = ""
				m.subview = svNone
			}
		} else {
			if len(m.recvMemo) > 0 {
				m.recvMemo = m.recvMemo[:len(m.recvMemo)-1]
			}
		}
		return m, nil
	case "tab":
		m.recvInputField = (m.recvInputField + 1) % 2
		return m, nil
	case "enter":
		if m.recvAmountStr == "" {
			m.recvError = "Enter an amount"
			return m, nil
		}
		amt, err := parseRecvAmount(m.recvAmountStr)
		if err != nil {
			m.recvError = err.Error()
			return m, nil
		}
		m.recvAmountSats = amt
		m.recvError = ""
		return m, createInvoiceCmd(
			m.lndClient, amt, m.recvMemo)
	case "up", "down", "left", "right":
		// Ignore arrow keys in text input
		return m, nil
	default:
		//
		text := msg.Text
		if len(text) == 0 {
			return m, nil
		}
		if m.recvInputField == 0 {
			for _, ch := range text {
				if ch >= '0' && ch <= '9' &&
					len(m.recvAmountStr) < 10 {
					m.recvAmountStr += string(ch)
				}
			}
		} else {
			for _, ch := range text {
				if len(m.recvMemo) < 100 && ch >= 32 && ch < 127 {
					m.recvMemo += string(ch)
				}
			}
		}
		return m, nil
	}
}

func (m Model) handleReceiveWaitingKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
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
	case "enter", "backspace":
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
	case "enter", "backspace":
		m.resetReceiveState()
		m.subview = svNone
		return m, nil
	}
	return m, nil
}

// ── Send ─────────────────────────────────────────────────

func (m Model) handleSendKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if len(m.sendPayReqInput) > 0 {
			m.sendPayReqInput = m.sendPayReqInput[:len(m.sendPayReqInput)-1]
			m.sendError = ""
		} else {
			m.resetSendState()
			m.subview = svNone
		}
		return m, nil
	case "ctrl+u":
		// Clear entire input field
		m.sendPayReqInput = ""
		m.sendError = ""
		return m, nil
	case "enter":
		payReq := strings.TrimSpace(m.sendPayReqInput)
		if payReq == "" {
			m.sendError = "Paste a payment request"
			return m, nil
		}
		// Strip any bracket paste artifacts
		payReq = cleanPayReq(payReq)
		m.sendPayReqInput = payReq
		if !strings.HasPrefix(payReq, "lnbc") &&
			!strings.HasPrefix(payReq, "lntb") &&
			!strings.HasPrefix(payReq, "lnbcrt") {
			m.sendError = "Not a valid Lightning invoice"
			return m, nil
		}
		m.sendError = ""
		return m, decodePayReqCmd(m.lndClient, payReq)
	case "up", "down", "left", "right":
		// Ignore arrow keys in text input
		return m, nil
	default:
		//
		text := msg.Text
		if len(text) == 0 {
			return m, nil
		}
		for _, ch := range text {
			// Accept only valid bolt11 characters
			if isBolt11Char(ch) && len(m.sendPayReqInput) < 1500 {
				m.sendPayReqInput += string(ch)
			}
		}
		return m, nil
	}
}

func (m Model) handleSendConfirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
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
			m.lndClient, strings.TrimSpace(m.sendPayReqInput))
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
	case "enter", "backspace":
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
	case "backspace":
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
	case "backspace":
		m.subview = svPaymentHistory
		return m, nil
	}
	return m, nil
}

// ── State reset helpers ──────────────────────────────────

func (m *Model) resetReceiveState() {
	m.recvAmountStr = ""
	m.recvMemo = ""
	m.recvPayReq = ""
	m.recvPaymentHash = ""
	m.recvAmountSats = 0
	m.recvSettled = false
	m.recvExpired = false
	m.recvInputField = 0
	m.recvError = ""
}

func (m *Model) resetSendState() {
	m.sendPayReqInput = ""
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
// Bolt11 invoices are bech32-encoded: lowercase alphanumeric only,
// plus the "lnbc"/"lntb" prefix.
func isBolt11Char(ch rune) bool {
	if ch >= 'a' && ch <= 'z' {
		return true
	}
	if ch >= '0' && ch <= '9' {
		return true
	}
	// Some wallets output uppercase — accept and we'll lowercase
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	return false
}

// cleanPayReq strips common paste artifacts from a payment request.
func cleanPayReq(s string) string {
	// Remove bracket paste artifacts
	s = strings.ReplaceAll(s, "[", "")
	s = strings.ReplaceAll(s, "]", "")
	// Remove quotes some terminals add
	s = strings.ReplaceAll(s, "\"", "")
	s = strings.ReplaceAll(s, "'", "")
	// Remove whitespace
	s = strings.TrimSpace(s)
	// Remove "lightning:" URI prefix
	s = strings.TrimPrefix(s, "lightning:")
	s = strings.TrimPrefix(s, "LIGHTNING:")
	return s
}
