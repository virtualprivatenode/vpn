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
			// Cap width to content pane:
			// 82 total - 2 frame - sidebarW - 1 div
			// - 6 padding
			cw := 82 - 2 - m.nav.Width - 1 - 6
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

// ── On-chain send flow keys ──────────────────────────────

func (m Model) handleOCSendAddrKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
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
		if m.ocSendAddrInput.Value() != "" {
			var cmd tea.Cmd
			m.ocSendAddrInput, cmd =
				m.ocSendAddrInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		return m.closeTab(m.activeTab)
	case "enter":
		addr := strings.TrimSpace(
			m.ocSendAddrInput.Value())
		if addr == "" {
			m.onChainSendError = "Enter an address"
			return m, nil
		}
		if !isValidOnChainAddr(
			addr, m.cfg.Network) {
			m.onChainSendError = "Invalid address"
			return m, nil
		}
		m.ocSendAddrVal = addr
		m.onChainSendError = ""
		m.subview = svOnChainSendAmount
		m.ocSendStep = 0
		m.ocSendAll = false
		return m, fetchFeeTiersCmd(m.cfg)
	default:
		var cmd tea.Cmd
		m.ocSendAddrInput, cmd =
			m.ocSendAddrInput.Update(tea.Msg(msg))
		return m, cmd
	}
}

func (m Model) handleOCSendAmountKey(
	key string, msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "left", "h":
		if m.ocSendStep == 1 &&
			m.ocSelectedTier > 0 {
			m.ocSelectedTier--
			return m, nil
		}
		m.focusSidebar()
		return m, nil
	case "backspace":
		if m.ocSendStep == 0 && !m.ocSendAll {
			if m.ocSendAmtInput.Value() != "" {
				var cmd tea.Cmd
				m.ocSendAmtInput, cmd =
					m.ocSendAmtInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		if m.ocSendStep == 2 {
			if m.ocCustomFeeInput.Value() != "" {
				var cmd tea.Cmd
				m.ocCustomFeeInput, cmd =
					m.ocCustomFeeInput.Update(
						tea.Msg(msg))
				return m, cmd
			}
		}
		m.subview = svOnChainSendAddr
		m.onChainSendError = ""
		return m, nil
	case "tab":
		m.ocSendAll = !m.ocSendAll
		if m.ocSendAll {
			m.ocSendAmtInput.Blur()
		} else {
			m.ocSendAmtInput.Focus()
		}
		return m, nil
	case "up", "k":
		if m.ocSendStep == 1 {
			m.ocSendStep = 0
			if !m.ocSendAll {
				m.ocSendAmtInput.Focus()
			}
		} else if m.ocSendStep == 2 {
			m.ocSendStep = 1
			m.ocCustomFeeInput.Blur()
		} else if m.ocSendStep == 0 {
			if m.hasDetailTabs() {
				m.focusTabBar()
				m.tabCursorX = 0
				m.activeTab = m.findFlowTab()
				return m, nil
			}
		}
		return m, nil
	case "down", "j":
		if m.ocSendStep == 0 {
			m.ocSendStep = 1
			m.ocSendAmtInput.Blur()
		} else if m.ocSendStep == 1 &&
			m.ocSelectedTier == 4 {
			m.ocSendStep = 2
			m.ocCustomFeeInput.Focus()
		}
		return m, nil
	case "right", "l":
		if m.ocSendStep == 1 &&
			m.ocSelectedTier < 4 {
			m.ocSelectedTier++
		}
		return m, nil
	case "enter":
		var amountSats int64
		if m.ocSendAll {
			amountSats = 0
		} else {
			val := strings.TrimSpace(
				m.ocSendAmtInput.Value())
			val = strings.ReplaceAll(val, ",", "")
			if val == "" {
				m.onChainSendError =
					"Enter an amount"
				return m, nil
			}
			for _, c := range val {
				if c < '0' || c > '9' {
					m.onChainSendError =
						"Invalid number"
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
				return m, nil
			}
			amountSats = n
		}

		var feeRate int64
		if m.ocSelectedTier < 4 {
			tier := m.ocFeeTiers[m.ocSelectedTier]
			if tier.SatPerVB <= 0 {
				m.onChainSendError =
					"Fee estimate not available"
				return m, nil
			}
			feeRate = int64(tier.SatPerVB)
			if feeRate < 1 {
				feeRate = 1
			}
		} else {
			feeVal := strings.TrimSpace(
				m.ocCustomFeeInput.Value())
			if feeVal == "" {
				m.onChainSendError =
					"Enter a custom fee rate"
				return m, nil
			}
			var n int64
			for _, c := range feeVal {
				if c < '0' || c > '9' {
					m.onChainSendError =
						"Invalid fee rate"
					return m, nil
				}
				n = n*10 + int64(c-'0')
			}
			if n < 1 {
				m.onChainSendError =
					"Minimum 1 sat/vB"
				return m, nil
			}
			feeRate = n
		}

		m.ocSendAmtVal = amountSats
		m.ocSendFeeRate = feeRate
		m.onChainSendError = ""
		m.ocConfirmFee = 0
		m.subview = svOnChainSendConfirm

		if !m.ocSendAll &&
			m.ocSendAddrVal != "" {
			target := int32(1)
			if m.ocSelectedTier < 4 {
				target = int32(
					m.ocFeeTiers[m.ocSelectedTier].
						Target)
			}
			return m, estimateTxFeeCmd(
				m.lndClient, m.ocSendAddrVal,
				amountSats, target)
		}
		return m, nil
	default:
		if m.ocSendStep == 0 && !m.ocSendAll {
			var cmd tea.Cmd
			m.ocSendAmtInput, cmd =
				m.ocSendAmtInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
		if m.ocSendStep == 2 {
			var cmd tea.Cmd
			m.ocCustomFeeInput, cmd =
				m.ocCustomFeeInput.Update(
					tea.Msg(msg))
			return m, cmd
		}
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
		m.subview = svOnChainSendAmount
		m.onChainSendError = ""
		return m, nil
	case "y":
		m.onChainSendError = ""
		m.subview = svOnChainSendBroadcast
		return m, sendCoinsCmd(
			m.lndClient,
			m.ocSendAddrVal,
			m.ocSendAmtVal,
			m.ocSendFeeRate,
			m.ocSendAll)
	}
	return m, nil
}

func (m *Model) resetOnChainSendState() {
	m.ocSendAddrInput = newOnChainAddrInput()
	m.ocSendAmtInput = newOnChainAmtInput()
	m.ocCustomFeeInput = newCustomFeeInput()
	m.ocSendAll = false
	m.ocSendStep = 0
	m.ocFeeTiers = [4]feeTier{}
	m.ocSelectedTier = 0
	m.ocConfirmFee = 0
	m.ocSendAddrVal = ""
	m.ocSendAmtVal = 0
	m.ocSendFeeRate = 0
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
