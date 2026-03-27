package welcome

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ripsline/virtual-private-node/internal/paths"
)

// ── Receive flow handlers ────────────────────────────────

func (m Model) handleReceiveKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "right", "l":
		var cmd tea.Cmd
		if m.recvAmountInput.Focused() {
			m.recvAmountInput, cmd =
				m.recvAmountInput.Update(
					tea.Msg(msg))
		} else {
			m.recvMemoInput, cmd =
				m.recvMemoInput.Update(
					tea.Msg(msg))
		}
		return m, cmd
	case "backspace":
		if m.recvAmountInput.Focused() &&
			m.recvAmountInput.Value() != "" {
			var cmd tea.Cmd
			m.recvAmountInput, cmd =
				m.recvAmountInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		if m.recvMemoInput.Focused() &&
			m.recvMemoInput.Value() != "" {
			var cmd tea.Cmd
			m.recvMemoInput, cmd =
				m.recvMemoInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		m.resetReceiveState()
		return m.closeTab(m.activeTab)
	case "down", "j":
		if m.recvAmountInput.Focused() {
			m.recvAmountInput.Blur()
			m.recvMemoInput.Focus()
		}
		return m, nil
	case "up", "k":
		if m.recvMemoInput.Focused() {
			m.recvMemoInput.Blur()
			m.recvAmountInput.Focus()
		} else {
			if m.hasDetailTabs() {
				m.focusTabBar()
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
			m.lndClient, amt,
			m.recvMemoInput.Value())
	default:
		var cmd tea.Cmd
		if m.recvAmountInput.Focused() {
			m.recvAmountInput, cmd =
				m.recvAmountInput.Update(
					tea.Msg(msg))
		} else {
			m.recvMemoInput, cmd =
				m.recvMemoInput.Update(
					tea.Msg(msg))
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
	case "left", "h":
		if m.recvButtonIdx > 0 {
			m.recvButtonIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "down", "j":
		return m, nil
	case "backspace":
		m.resetReceiveState()
		return m.closeTab(m.activeTab)
	case "right", "l":
		if m.recvButtonIdx < 1 {
			m.recvButtonIdx++
		}
		return m, nil
	case "enter":
		if m.recvButtonIdx == 0 &&
			m.recvPayReq != "" {
			m.urlTarget = m.recvPayReq
			m.qrLabel = fmt.Sprintf(
				"Invoice — %s sats",
				formatSats(m.recvAmountSats))
			m.urlReturnTo = svReceiveWaiting
			m.subview = svQR
			return m, nil
		}
		if m.recvButtonIdx == 1 &&
			m.recvPayReq != "" {
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
	case "enter", "backspace":
		m.resetReceiveState()
		m.nav.SetActive(secWallet)
		cm, cmd := m.closeTab(m.activeTab)
		return cm, tea.Batch(cmd,
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
	case "enter", "backspace":
		m.resetReceiveState()
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

// ── Send flow handlers ───────────────────────────────────

func (m Model) handleSendKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.sendInput.Value() != "" {
			var cmd tea.Cmd
			m.sendInput, cmd =
				m.sendInput.Update(tea.Msg(msg))
			return m, cmd
		}
		m.focusSidebar()
		return m, nil
	case "right", "l":
		if m.sendInput.Value() != "" {
			var cmd tea.Cmd
			m.sendInput, cmd =
				m.sendInput.Update(tea.Msg(msg))
			return m, cmd
		}
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "backspace":
		if m.sendInput.Value() != "" {
			var cmd tea.Cmd
			m.sendInput, cmd =
				m.sendInput.Update(tea.Msg(msg))
			return m, cmd
		}
		m.resetSendState()
		return m.closeTab(m.activeTab)
	case "enter":
		payReq := strings.TrimSpace(
			m.sendInput.Value())
		if payReq == "" {
			m.sendError = "Paste a payment request"
			return m, nil
		}
		payReq = cleanPayReq(payReq)
		m.sendInput.SetValue(payReq)
		if !strings.HasPrefix(payReq, "lnbc") &&
			!strings.HasPrefix(payReq, "lntb") &&
			!strings.HasPrefix(payReq, "lnbcrt") {
			m.sendError =
				"Not a valid Lightning invoice"
			return m, nil
		}
		m.sendError = ""
		return m, decodePayReqCmd(
			m.lndClient, payReq)
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
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
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
	case "enter", "backspace":
		m.resetSendState()
		m.nav.SetActive(secWallet)
		cm, cmd := m.closeTab(m.activeTab)
		return cm, tea.Batch(cmd,
			fetchStatus(m.cfg, m.lndClient),
			fetchPaymentHistoryCmd(m.lndClient))
	}
	return m, nil
}

// ── Channel open flow handlers ───────────────────────────

func (m Model) handleChannelOpenKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.chanOpenPeerIdx > 0 {
			m.chanOpenPeerIdx--
		} else if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
	case "backspace":
		m.subview = svNone
		m.chanOpenError = ""
		return m.closeTab(m.activeTab)
	case "down", "j":
		if m.chanOpenPeerIdx < len(m.chanPeerList) {
			m.chanOpenPeerIdx++
		}
	case "enter":
		customIdx := len(m.chanPeerList)
		if m.chanOpenPeerIdx == customIdx {
			m.chanPubkeyInput = newChanPubkeyInput()
			m.chanHostInput = newChanHostInput()
			// Cap width to content pane
			cw := tuiWidth - 2 - m.nav.Width - 1 - 6
			if cw > 58 {
				cw = 58
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
			peer :=
				m.chanPeerList[m.chanOpenPeerIdx]
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
	case "left", "h":
		if m.chanPubkeyInput.Focused() &&
			m.chanPubkeyInput.Value() != "" {
			var cmd tea.Cmd
			m.chanPubkeyInput, cmd =
				m.chanPubkeyInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		if m.chanHostInput.Focused() &&
			m.chanHostInput.Value() != "" {
			var cmd tea.Cmd
			m.chanHostInput, cmd =
				m.chanHostInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		m.focusSidebar()
		return m, nil
	case "right", "l":
		if m.chanPubkeyInput.Focused() {
			var cmd tea.Cmd
			m.chanPubkeyInput, cmd =
				m.chanPubkeyInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		if m.chanHostInput.Focused() {
			var cmd tea.Cmd
			m.chanHostInput, cmd =
				m.chanHostInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		return m, nil
	case "backspace":
		if m.chanPubkeyInput.Focused() &&
			m.chanPubkeyInput.Value() != "" {
			var cmd tea.Cmd
			m.chanPubkeyInput, cmd =
				m.chanPubkeyInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		if m.chanHostInput.Focused() &&
			m.chanHostInput.Value() != "" {
			var cmd tea.Cmd
			m.chanHostInput, cmd =
				m.chanHostInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		m.subview = svChannelOpen
		m.chanOpenError = ""
		return m, nil
	case "tab", "down", "j":
		if m.chanPubkeyInput.Focused() {
			m.chanPubkeyInput.Blur()
			m.chanHostInput.Focus()
		} else {
			m.chanHostInput.Blur()
			m.chanPubkeyInput.Focus()
		}
		return m, nil
	case "up", "k":
		if m.chanHostInput.Focused() {
			m.chanHostInput.Blur()
			m.chanPubkeyInput.Focus()
		} else if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
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
			m.chanOpenError =
				"Pubkey must be 66 hex chars"
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
				m.chanPubkeyInput.Update(
					tea.Msg(msg))
		} else {
			m.chanHostInput, cmd =
				m.chanHostInput.Update(
					tea.Msg(msg))
		}
		return m, cmd
	}
}

func (m Model) handleChannelAmountKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	isCustom :=
		m.chanAmountPreset == len(amountPresets)-1
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if isCustom &&
			m.chanAmountInput.Value() != "" {
			var cmd tea.Cmd
			m.chanAmountInput, cmd =
				m.chanAmountInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		m.focusSidebar()
		return m, nil
	case "right", "l":
		if isCustom {
			var cmd tea.Cmd
			m.chanAmountInput, cmd =
				m.chanAmountInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		return m, nil
	case "up", "k":
		if !isCustom && m.chanAmountPreset > 0 {
			m.chanAmountPreset--
			m.chanOpenError = ""
		} else if m.chanAmountPreset == 0 {
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = m.findFlowTab()
				return m, nil
			}
		}
	case "down", "j":
		if !isCustom &&
			m.chanAmountPreset <
				len(amountPresets)-1 {
			m.chanAmountPreset++
			m.chanOpenError = ""
		}
	case "backspace":
		if isCustom &&
			m.chanAmountInput.Value() != "" {
			var cmd tea.Cmd
			m.chanAmountInput, cmd =
				m.chanAmountInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		m.subview = svChannelOpen
		m.chanOpenError = ""
		return m, nil
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
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
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
	case "enter", "backspace":
		m.subview = svNone
		m.chanOpenError = ""
		m.chanOpenTxid = ""
		m.chanOpenInFlight = false
		m.nav.SetActive(secChannels)
		cm, cmd := m.closeTab(m.activeTab)
		return cm, tea.Batch(cmd,
			fetchStatus(m.cfg, m.lndClient))
	}
	return m, nil
}

func (m Model) handleChannelFundKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		m.focusSidebar()
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "backspace":
		m.chanFundAddress = ""
		m.subview = svNone
		return m.closeTab(m.activeTab)
	}
	return m, nil
}

// ── Syncthing flow handlers ──────────────────────────────

func (m Model) handleSyncWebUIKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.addonBtnIdx > 0 {
			m.addonBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "right", "l":
		if m.addonBtnIdx < 1 {
			m.addonBtnIdx++
		}
	case "up", "k":
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
		m.subview = svSyncthingDetail
		m.addonBtnIdx = 0
	}
	return m, nil
}

func (m Model) handleSyncthingPairInputKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.syncDeviceInput.Value() != "" {
			var cmd tea.Cmd
			m.syncDeviceInput, cmd =
				m.syncDeviceInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		m.focusSidebar()
		return m, nil
	case "right", "l":
		if m.syncDeviceInput.Value() != "" {
			var cmd tea.Cmd
			m.syncDeviceInput, cmd =
				m.syncDeviceInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		return m, nil
	case "backspace":
		if m.syncDeviceInput.Value() == "" &&
			!m.syncPairSuccess {
			m.syncPairError = ""
			m.syncPairSuccess = false
			m.subview = svSyncthingDetail
			return m, nil
		}
		if m.syncPairSuccess {
			m.syncDeviceInput =
				newSyncthingIDInput()
			m.syncPairSuccess = false
			m.subview = svSyncthingDetail
			return m, nil
		}
		var cmd tea.Cmd
		m.syncDeviceInput, cmd =
			m.syncDeviceInput.Update(tea.Msg(msg))
		return m, cmd
	case "enter":
		if m.syncPairSuccess {
			m.syncDeviceInput =
				newSyncthingIDInput()
			m.syncPairSuccess = false
			m.subview = svSyncthingDetail
			return m, nil
		}
		deviceID := syncthingIDValue(
			m.syncDeviceInput)
		if deviceID != "" {
			parts := strings.Split(deviceID, "-")
			if len(parts) != 8 {
				m.syncPairError =
					"Invalid Device ID format." +
						" Expected 8 groups" +
						" separated by hyphens."
				return m, nil
			}
			for _, p := range parts {
				if len(p) != 7 {
					m.syncPairError =
						"Invalid Device ID" +
							" format. Each group" +
							" should be 7" +
							" characters."
					return m, nil
				}
			}
			m.syncPairError = ""
			return m,
				pairSyncthingDeviceCmd(deviceID)
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.syncDeviceInput, cmd =
			m.syncDeviceInput.Update(tea.Msg(msg))
		return m, cmd
	}
}

// ── LndHub create name handler ───────────────────────────

func (m Model) handleLndHubCreateNameKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.hubNameInput.Value() != "" {
			var cmd tea.Cmd
			m.hubNameInput, cmd =
				m.hubNameInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		m.focusSidebar()
		return m, nil
	case "right", "l":
		if m.hubNameInput.Value() != "" {
			var cmd tea.Cmd
			m.hubNameInput, cmd =
				m.hubNameInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		return m, nil
	case "backspace":
		if m.hubNameInput.Value() == "" {
			m.subview = svLndHubManage
			return m, nil
		}
		var cmd tea.Cmd
		m.hubNameInput, cmd =
			m.hubNameInput.Update(tea.Msg(msg))
		return m, cmd
	case "enter":
		name := m.hubNameInput.Value()
		if name != "" {
			return m, createLndHubAccountCmd(
				m.cfg.LndHubAdminToken)
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.hubNameInput, cmd =
			m.hubNameInput.Update(tea.Msg(msg))
		return m, cmd
	}
}

// ── On-chain send flow keys (single screen) ─────────────
//
// Steps:
//   0 = Address input
//   1 = Amount input
//   2 = Max / Send All button
//   3 = Label input
//   4 = Fee tier selector
//   5 = Custom fee input (only when Custom selected)
//   6 = Buttons (Clear / Create Transaction)

func (m Model) handleOCSendKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit

	case "left", "h":
		// Buttons: move left
		if m.ocSendStep == 5 &&
			m.ocSendBtnIdx > 0 {
			m.ocSendBtnIdx--
			return m, nil
		}
		// Addr input: pass through for cursor
		if m.ocSendStep == 0 {
			if m.ocSendAddrInput.Value() != "" {
				var cmd tea.Cmd
				m.ocSendAddrInput, cmd =
					m.ocSendAddrInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		// Amount input: pass through for cursor
		if m.ocSendStep == 1 && !m.ocSendAll {
			if m.ocSendAmtInput.Value() != "" {
				var cmd tea.Cmd
				m.ocSendAmtInput, cmd =
					m.ocSendAmtInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		// Label input: pass through for cursor
		if m.ocSendStep == 3 {
			if m.ocSendLabelInput.Value() != "" {
				var cmd tea.Cmd
				m.ocSendLabelInput, cmd =
					m.ocSendLabelInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		// Fee input: pass through for cursor
		if m.ocSendStep == 4 {
			if m.ocCustomFeeInput.Value() != "" {
				var cmd tea.Cmd
				m.ocCustomFeeInput, cmd =
					m.ocCustomFeeInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		m.focusSidebar()
		return m, nil

	case "right", "l":
		// Buttons: move right
		if m.ocSendStep == 5 &&
			m.ocSendBtnIdx < 1 {
			m.ocSendBtnIdx++
			return m, nil
		}
		// Addr input: pass through for cursor
		if m.ocSendStep == 0 {
			var cmd tea.Cmd
			m.ocSendAddrInput, cmd =
				m.ocSendAddrInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		// Amount input: pass through for cursor
		if m.ocSendStep == 1 && !m.ocSendAll {
			var cmd tea.Cmd
			m.ocSendAmtInput, cmd =
				m.ocSendAmtInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		// Label input: pass through for cursor
		if m.ocSendStep == 3 {
			var cmd tea.Cmd
			m.ocSendLabelInput, cmd =
				m.ocSendLabelInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		// Fee input: pass through for cursor
		if m.ocSendStep == 4 {
			var cmd tea.Cmd
			m.ocCustomFeeInput, cmd =
				m.ocCustomFeeInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		return m, nil

	case "backspace":
		// In text inputs: delete char if non-empty
		if m.ocSendStep == 0 {
			if m.ocSendAddrInput.Value() != "" {
				var cmd tea.Cmd
				m.ocSendAddrInput, cmd =
					m.ocSendAddrInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
			// Empty addr input: close tab
			return m.closeTab(m.activeTab)
		}
		if m.ocSendStep == 1 && m.ocSendAll {
			m.ocSendAll = false
			m.ocSendAmtInput.SetValue("")
			m.ocSendAmtInput.Focus()
			return m, nil
		}
		if m.ocSendStep == 1 && !m.ocSendAll {
			if m.ocSendAmtInput.Value() != "" {
				var cmd tea.Cmd
				m.ocSendAmtInput, cmd =
					m.ocSendAmtInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		if m.ocSendStep == 3 {
			if m.ocSendLabelInput.Value() != "" {
				var cmd tea.Cmd
				m.ocSendLabelInput, cmd =
					m.ocSendLabelInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		if m.ocSendStep == 4 {
			if m.ocCustomFeeInput.Value() != "" {
				var cmd tea.Cmd
				m.ocCustomFeeInput, cmd =
					m.ocCustomFeeInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		// Move up one step on backspace with
		// empty input
		if m.ocSendStep > 0 {
			m.ocSendStep--
			m.onChainSendError = ""
			m.focusSendStep()
			return m, nil
		}
		return m.closeTab(m.activeTab)

	case "up", "k":
		if m.ocSendStep > 0 {
			m.ocSendStep--
			m.focusSendStep()
		} else if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil

	case "down", "j":
		next := m.ocSendStep + 1
		if next > 5 {
			return m, nil
		}
		m.ocSendStep = next
		m.focusSendStep()
		return m, nil

	case "enter":
		// Max button: toggle send all
		if m.ocSendStep == 2 {
			if m.ocSendAll {
				// Clear max
				m.ocSendAll = false
				m.ocSendAmtInput.SetValue("")
				m.ocSendAmtInput.Focus()
			} else if len(m.utxoSelected) > 0 {
				// Coin control: fill with selected
				// total minus estimated fee
				feeRate := getSendFeeRate(m)
				numInputs := max(
					len(m.utxoOutpoints), 1)
				estFee := estimateSimpleFee(
					numInputs, 1, feeRate)
				maxAmt := m.utxoSelectedTotal - estFee
				if maxAmt < 0 {
					maxAmt = 0
				}
				m.ocSendAll = true
				m.ocSendAmtInput.SetValue(
					fmt.Sprintf("%d", maxAmt))
				m.ocSendAmtInput.Blur()
			} else {
				// No coin control: compute from
				// on-chain balance minus est fee
				// using actual UTXO count
				m.ocSendAll = true
				if m.status != nil &&
					m.status.lndBalance != "" {
					bal := parseBalance(
						m.status.lndBalance)
					feeRate := getSendFeeRate(m)
					numInputs := max(
						len(m.utxos), 1)
					estFee := estimateSimpleFee(
						numInputs, 1, feeRate)
					maxAmt := bal - estFee
					if maxAmt < 0 {
						maxAmt = 0
					}
					m.ocSendAmtInput.SetValue(
						fmt.Sprintf("%d", maxAmt))
				}
				m.ocSendAmtInput.Blur()
			}
			return m, nil
		}
		// Bottom buttons
		if m.ocSendStep == 5 {
			switch m.ocSendBtnIdx {
			case 0: // Clear
				m.resetOnChainSendState()
				m.clearUtxoSelection()
				return m, nil
			case 1: // Create Transaction
				return m.validateAndConfirmSend()
			}
		}
		// Enter on any other step: advance to next
		next := m.ocSendStep + 1
		next = min(next, 5)
		m.ocSendStep = next
		m.focusSendStep()
		return m, nil

	default:
		// Pass through to active text input
		switch m.ocSendStep {
		case 0:
			var cmd tea.Cmd
			m.ocSendAddrInput, cmd =
				m.ocSendAddrInput.Update(
					tea.Msg(msg))
			return m, cmd
		case 1:
			if !m.ocSendAll {
				var cmd tea.Cmd
				m.ocSendAmtInput, cmd =
					m.ocSendAmtInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		case 3:
			var cmd tea.Cmd
			m.ocSendLabelInput, cmd =
				m.ocSendLabelInput.Update(
					tea.Msg(msg))
			return m, cmd
		case 4:
			var cmd tea.Cmd
			m.ocCustomFeeInput, cmd =
				m.ocCustomFeeInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
	}
	return m, nil
}

// focusSendStep manages text input focus for the
// current ocSendStep.
func (m *Model) focusSendStep() {
	m.ocSendAddrInput.Blur()
	m.ocSendAmtInput.Blur()
	m.ocSendLabelInput.Blur()
	m.ocCustomFeeInput.Blur()
	switch m.ocSendStep {
	case 0:
		m.ocSendAddrInput.Focus()
	case 1:
		if !m.ocSendAll {
			m.ocSendAmtInput.Focus()
		}
	case 3:
		m.ocSendLabelInput.Focus()
	case 4:
		m.ocCustomFeeInput.Focus()
	}
}

// validateAndConfirmSend validates all fields and
// transitions to the confirm screen.
func (m Model) validateAndConfirmSend() (
	tea.Model, tea.Cmd,
) {
	// Validate address
	addr := strings.TrimSpace(
		m.ocSendAddrInput.Value())
	if addr == "" {
		m.onChainSendError = "Enter an address"
		m.ocSendStep = 0
		m.focusSendStep()
		return m, nil
	}
	if !isValidOnChainAddr(addr, m.cfg.Network) {
		m.onChainSendError = "Invalid address"
		m.ocSendStep = 0
		m.focusSendStep()
		return m, nil
	}

	// Validate amount
	var amountSats int64
	if m.ocSendAll {
		// LND handles the sweep — amount=0 in RPC
		amountSats = 0
		// Store the display value from the input
		// for the confirm screen
		displayVal := parseSendAmount(
			m.ocSendAmtInput.Value())
		if displayVal > 0 {
			m.ocSendAmtVal = displayVal
		}
	} else {
		val := strings.TrimSpace(
			m.ocSendAmtInput.Value())
		val = strings.ReplaceAll(val, ",", "")
		if val == "" {
			m.onChainSendError = "Enter an amount"
			m.ocSendStep = 1
			m.focusSendStep()
			return m, nil
		}
		for _, c := range val {
			if c < '0' || c > '9' {
				m.onChainSendError = "Invalid number"
				m.ocSendStep = 1
				m.focusSendStep()
				return m, nil
			}
		}
		var n int64
		for _, c := range val {
			n = n*10 + int64(c-'0')
		}
		if n < 546 {
			m.onChainSendError =
				"Minimum 546 sats (dust limit)"
			m.ocSendStep = 1
			m.focusSendStep()
			return m, nil
		}
		amountSats = n
	}

	// Validate fee rate
	var feeRate int64
	feeVal := strings.TrimSpace(
		m.ocCustomFeeInput.Value())
	if feeVal == "" {
		m.onChainSendError =
			"Enter a fee rate"
		m.ocSendStep = 4
		m.focusSendStep()
		return m, nil
	}
	var fn int64
	for _, c := range feeVal {
		if c < '0' || c > '9' {
			m.onChainSendError =
				"Invalid fee rate"
			m.ocSendStep = 4
			m.focusSendStep()
			return m, nil
		}
		fn = fn*10 + int64(c-'0')
	}
	if fn < 1 {
		m.onChainSendError = "Minimum 1 sat/vB"
		m.ocSendStep = 4
		m.focusSendStep()
		return m, nil
	}
	feeRate = fn

	m.ocSendAddrVal = addr
	m.ocSendAmtVal = amountSats
	m.ocSendFeeRate = feeRate
	m.ocSendLabelVal = strings.TrimSpace(
		m.ocSendLabelInput.Value())
	m.onChainSendError = ""
	m.ocConfirmFee = 0
	m.ocConfirmBtnIdx = 0
	m.subview = svOCSendConfirm

	if !m.ocSendAll && addr != "" {
		target := int32(1)
		return m, estimateTxFeeCmd(
			m.lndClient, addr,
			amountSats, target)
	}
	return m, nil
}

func (m Model) handleOCSendConfirmKey(
	key string,
) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.ocConfirmBtnIdx > 0 {
			m.ocConfirmBtnIdx--
		} else {
			m.focusSidebar()
		}
		return m, nil
	case "right", "l":
		if m.ocConfirmBtnIdx < 1 {
			m.ocConfirmBtnIdx++
		}
		return m, nil
	case "up", "k":
		if m.hasDetailTabs() {
			m.focusTabBar()
			m.tabCursorX = 0
			m.activeTab = m.findFlowTab()
			return m, nil
		}
		return m, nil
	case "down", "j":
		return m, nil
	case "backspace":
		m.subview = svOnChainSend
		m.onChainSendError = ""
		return m, nil
	case "enter":
		switch m.ocConfirmBtnIdx {
		case 0: // Go Back
			m.subview = svOnChainSend
			m.onChainSendError = ""
			return m, nil
		case 1: // Confirm & Broadcast
			m.onChainSendError = ""
			m.subview = svOCSendBroadcast
			return m, sendCoinsCmd(
				m.lndClient,
				m.ocSendAddrVal,
				m.ocSendAmtVal,
				m.ocSendFeeRate,
				m.ocSendAll,
				m.utxoOutpoints)
		}
	}
	return m, nil
}

func (m *Model) resetOnChainSendState() {
	m.ocSendAddrInput = newOnChainAddrInput()
	m.ocSendAmtInput = newOnChainAmtInput()
	m.ocSendLabelInput = newOCSendLabelInput()
	m.ocCustomFeeInput = newCustomFeeInput()
	m.ocSendAll = false
	m.ocSendStep = 0
	m.ocConfirmFee = 0
	m.ocConfirmBtnIdx = 0
	m.ocSendAddrVal = ""
	m.ocSendAmtVal = 0
	m.ocSendFeeRate = 0
	m.ocSendLabelVal = ""
	m.onChainSendError = ""
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
