package welcome

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/system"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case svcActionDoneMsg:
		return m, fetchStatus(m.cfg, m.lndClient)
	case statusMsg:
		m.fetchInFlight = false
		m.status = &msg
		if msg.walletDetected && !m.cfg.WalletCreated {
			m.cfg.WalletCreated = true
			m.saveCfg()
		}
		return m, nil
	case latestVersionMsg:
		m.latestVersion = string(msg)
		return m, nil
	case lndhubAccountCreatedMsg:
		if msg.err != nil {
			logger.TUI("Warning: failed to create LndHub account: %v", msg.err)
			m.subview = svLndHubManage
			return m, nil
		}
		if msg.account != nil {
			m.lastAccount = msg.account
			m.cfg.LndHubAccounts = append(m.cfg.LndHubAccounts, config.LndHubAccount{
				Label:     m.hubNameInput,
				Login:     msg.account.Login,
				CreatedAt: time.Now().Format("2006-01-02"),
				Active:    true,
			})
			m.saveCfg()
			m.subview = svLndHubCreateAccount
		}
		return m, nil
	case lndhubDeactivatedMsg:
		if m.hubCursor < len(m.cfg.LndHubAccounts) {
			acct := &m.cfg.LndHubAccounts[m.hubCursor]
			if msg.err != nil {
				logger.TUI("Warning: deactivate failed: %v", msg.err)
			} else {
				acct.Active = false
				acct.DeactivatedAt = time.Now().Format("2006-01-02")
				acct.BalanceOnDeactivate = msg.balance
				m.saveCfg()
				logger.TUI("Deactivated account %s (balance: %s sats)",
					acct.Label, msg.balance)
			}
		}
		m.subview = svLndHubManage
		return m, nil
	case syncthingPairedMsg:
		if msg.err != nil {
			m.syncPairError = msg.err.Error()
			m.syncPairSuccess = false
			logger.TUI("Syncthing pairing failed: %v", msg.err)
		} else {
			m.syncPairError = ""
			m.syncPairSuccess = true
			m.cfg.SyncthingDevices = append(m.cfg.SyncthingDevices,
				config.SyncthingDevice{
					Name: "Device " + fmt.Sprintf("%d",
						len(m.cfg.SyncthingDevices)+1),
					DeviceID: m.syncDeviceInput,
					PairedAt: time.Now().Format("2006-01-02"),
				})
			m.saveCfg()
			logger.TUI("Syncthing device paired successfully")
		}
		return m, nil
	case channelOpenResultMsg:
		m.chanOpenInFlight = false
		if msg.err != nil {
			m.chanOpenError = msg.err.Error()
			m.subview = svChannelOpenResult
			logger.TUI("Channel open failed: %v", msg.err)
		} else {
			m.chanOpenTxid = msg.txid
			m.chanOpenError = ""
			m.subview = svChannelOpenResult
			logger.TUI("Channel opened: tx=%s", msg.txid)
		}
		return m, nil
	case newAddressMsg:
		if msg.err == nil {
			m.chanFundAddress = msg.address
		}
		return m, nil
	case invoiceCreatedMsg:
		if msg.err != nil {
			m.recvError = msg.err.Error()
			return m, nil
		}
		m.recvPayReq = msg.payReq
		m.recvPaymentHash = msg.paymentHash
		m.recvAmountSats = msg.amountSats
		m.subview = svReceiveWaiting
		return m, waitForInvoiceCmd(m.lndClient, msg.paymentHash)
	case invoiceSettledMsg:
		if msg.err != nil {
			logger.TUI("Invoice settlement error: %v", msg.err)
			return m, nil
		}
		if msg.settled {
			m.recvSettled = true
			m.subview = svReceivePaid
		} else if msg.expired {
			m.recvExpired = true
			m.subview = svReceiveExpired
		}
		return m, nil
	case payReqDecodedMsg:
		if msg.err != nil {
			m.sendError = msg.err.Error()
			return m, nil
		}
		if msg.decoded.IsExpired {
			m.sendError = "This invoice has expired"
			return m, nil
		}
		m.sendDecodedValid = true
		m.sendDecodedAmt = msg.decoded.AmountSats
		m.sendDecodedDesc = msg.decoded.Description
		m.sendDecodedDest = msg.decoded.Destination
		m.subview = svSendConfirm
		return m, nil
	case sendPaymentResultMsg:
		m.sendInFlight = false
		if msg.err != nil {
			m.sendError = msg.err.Error()
			m.subview = svSendResult
			return m, nil
		}
		if msg.result.Status == "SUCCEEDED" {
			m.sendPreimage = msg.result.Preimage
			m.sendFeeSats = msg.result.FeeSats
			m.sendRouteHops = msg.result.Hops
			m.sendError = ""
		} else {
			m.sendError = msg.result.Error
		}
		m.subview = svSendResult
		return m, nil
	case paymentHistoryMsg:
		if msg.err == nil {
			m.payHistory = msg.entries
		}
		return m, nil
	case tickMsg:
		if m.fetchInFlight {
			return m, tickEvery(m.pollInterval())
		}
		m.fetchInFlight = true
		return m, tea.Batch(
			fetchStatus(m.cfg, m.lndClient),
			tickEvery(m.pollInterval()),
		)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// ── Text input subviews (must be handled first) ──────
	if m.subview == svLndHubCreateName {
		return m.handleLndHubCreateNameKey(key)
	}
	if m.subview == svSyncthingPairInput {
		return m.handleSyncthingPairInputKey(key)
	}

	// ── Channel open subviews ────────────────────────────
	if isChannelSubview(m.subview) {
		return m.handleChannelsKey(key)
	}

	// ── Wallet subviews ──────────────────────────────────
	if isWalletSubview(m.subview) {
		return m.handleWalletKey(key, msg)
	}

	// ── Pairing subviews ─────────────────────────────────
	if isPairingSubview(m.subview) {
		return m.handlePairingKey(key)
	}

	// ── Addon subviews ───────────────────────────────────
	if isAddonSubview(m.subview) {
		return m.handleAddonsKey(key)
	}

	// ── Generic subview handlers ─────────────────────────
	if m.subview != svNone {
		return m.handleGenericSubviewKey(key)
	}

	// ── Inside a card ────────────────────────────────────
	if m.cardActive {
		return m.handleDashboardCardKey(key)
	}

	// ── Settings tab ─────────────────────────────────────
	if m.activeTab == tabSettings {
		return m.handleSettingsTabKey(key)
	}

	// ── Main navigation ──────────────────────────────────
	return m.handleMainNavKey(key)
}

func (m Model) handleGenericSubviewKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "backspace":
		switch m.subview {
		case svWalletInfo:
			m.subview = svNone
		default:
			m.subview = svNone
		}
		return m, nil
	case "p":
		if m.subview == svWalletInfo && m.cfg.P2PMode == "tor" && m.cfg.HasLND() {
			m.shellAction = svP2PUpgrade
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) handleMainNavKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.activeTab = (m.activeTab + 1) % 5
		m.cardActive = false
		m.svcConfirm = ""
		return m, nil
	case "shift+tab":
		m.activeTab = (m.activeTab + 4) % 5
		m.cardActive = false
		m.svcConfirm = ""
		return m, nil
	case "1":
		m.activeTab = tabDashboard
	case "2":
		m.activeTab = tabLightning
	case "3":
		m.activeTab = tabPairing
	case "4":
		m.activeTab = tabAddons
	case "5":
		m.activeTab = tabSettings
	case "o":
		if m.activeTab == tabLightning && m.lightningFocus == 0 {
			return m.startChannelOpen()
		}
	case "s":
		if m.activeTab == tabLightning && m.lightningFocus == 1 &&
			m.cfg.HasLND() && m.cfg.WalletExists() {
			m.resetSendState()
			m.subview = svSend
			return m, nil
		}
	case "r":
		if m.activeTab == tabLightning && m.lightningFocus == 1 &&
			m.cfg.HasLND() && m.cfg.WalletExists() {
			m.resetReceiveState()
			m.subview = svReceive
			return m, nil
		}
	case "v":
		if m.activeTab == tabLightning && m.lightningFocus == 1 &&
			m.cfg.HasLND() && m.cfg.WalletExists() {
			m.payHistoryCursor = 0
			m.subview = svPaymentHistory
			return m, fetchPaymentHistoryCmd(m.lndClient)
		}
	case "up", "k":
		m = m.navUp()
	case "down", "j":
		m = m.navDown()
	case "left", "h":
		m = m.navLeft()
	case "right", "l":
		m = m.navRight()
	case "enter":
		return m.handleEnter()
	}
	return m, nil
}

func (m Model) navUp() Model {
	switch m.activeTab {
	case tabDashboard:
		switch m.dashCard {
		case cardBitcoin:
			m.dashCard = cardServices
		case cardLightning:
			m.dashCard = cardSystem
		}
	case tabLightning:
		if m.lightningFocus == 0 {
			if m.chanCursor > 0 {
				m.chanCursor--
				if m.chanCursor < m.chanScrollOffset {
					m.chanScrollOffset = m.chanCursor
				}
			}
		}
	}
	return m
}

func (m Model) navDown() Model {
	switch m.activeTab {
	case tabDashboard:
		switch m.dashCard {
		case cardServices:
			m.dashCard = cardBitcoin
		case cardSystem:
			m.dashCard = cardLightning
		}
	case tabLightning:
		if m.lightningFocus == 0 {
			if m.status != nil && m.chanCursor < len(m.status.channels)-1 {
				m.chanCursor++
				visibleCount := m.channelVisibleCount()
				if m.chanCursor >= m.chanScrollOffset+visibleCount {
					m.chanScrollOffset = m.chanCursor - visibleCount + 1
				}
			}
		}
	}
	return m
}

func (m Model) navLeft() Model {
	switch m.activeTab {
	case tabDashboard:
		switch m.dashCard {
		case cardSystem:
			m.dashCard = cardServices
		case cardLightning:
			m.dashCard = cardBitcoin
		}
	case tabLightning:
		if m.lightningFocus > 0 {
			m.lightningFocus--
		}
	case tabAddons:
		if m.addonFocus > 0 {
			m.addonFocus--
		}
	}
	return m
}

func (m Model) navRight() Model {
	switch m.activeTab {
	case tabDashboard:
		switch m.dashCard {
		case cardServices:
			m.dashCard = cardSystem
		case cardBitcoin:
			m.dashCard = cardLightning
		}
	case tabLightning:
		if m.lightningFocus < 1 {
			m.lightningFocus++
		}
	case tabAddons:
		if m.addonFocus < 1 {
			m.addonFocus++
		}
	}
	return m
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.activeTab {
	case tabDashboard:
		switch m.dashCard {
		case cardServices:
			m.cardActive = true
			m.svcCursor = 0
			return m, nil
		case cardSystem:
			m.cardActive = true
			return m, nil
		case cardLightning:
			if !m.cfg.HasLND() {
				m.shellAction = svLNDInstall
				return m, tea.Quit
			}
			if !m.cfg.WalletExists() {
				m.shellAction = svWalletCreate
				return m, tea.Quit
			}
			m.activeTab = tabLightning
		}
	case tabLightning:
		return m.handleLightningEnter()
	case tabPairing:
		if m.cfg.HasLND() && m.cfg.WalletExists() {
			m.subview = svZeus
		}
	case tabAddons:
		return m.handleAddonEnter()
	}
	return m, nil
}

func (m Model) handleLightningEnter() (tea.Model, tea.Cmd) {
	switch m.lightningFocus {
	case 0:
		if !m.cfg.HasLND() || !m.cfg.WalletExists() {
			return m, nil
		}
		if m.status != nil && len(m.status.channels) == 0 {
			return m.startChannelOpen()
		}
		if m.status != nil && len(m.status.channels) > 0 &&
			m.chanCursor < len(m.status.channels) {
			m.subview = svChannelDetail
			return m, nil
		}
	case 1:
		if !m.cfg.HasLND() || !m.cfg.WalletExists() {
			return m, nil
		}
		m.subview = svWalletInfo
	}
	return m, nil
}

func (m Model) handleAddonEnter() (tea.Model, tea.Cmd) {
	switch m.addonFocus {
	case 0:
		if m.cfg.SyncthingInstalled {
			m.subview = svSyncthingDetail
			return m, nil
		}
		if !m.cfg.HasLND() || !m.cfg.WalletExists() {
			return m, nil
		}
		m.shellAction = svSyncthingInstall
		return m, tea.Quit
	case 1:
		if m.cfg.LndHubInstalled {
			m.hubCursor = 0
			m.subview = svLndHubManage
			return m, nil
		}
		if !m.cfg.HasLND() || !m.cfg.WalletExists() {
			return m, nil
		}
		if !system.IsServiceActive("lnd") {
			return m, nil
		}
		if m.status != nil && !m.status.btcSynced {
			return m, nil
		}
		m.shellAction = svLndHubInstall
		return m, tea.Quit
	}
	return m, nil
}

func isChannelSubview(sv wSubview) bool {
	switch sv {
	case svChannelOpen, svChannelCustomPeer, svChannelAmountSelect,
		svChannelOpenConfirm, svChannelOpening, svChannelOpenResult,
		svChannelFundWallet:
		return true
	}
	return false
}

func isPairingSubview(sv wSubview) bool {
	switch sv {
	case svZeus, svQR, svFullURL:
		return true
	}
	return false
}

func isAddonSubview(sv wSubview) bool {
	switch sv {
	case svSyncthingDetail, svSyncthingDeviceDetail, svSyncthingWebUI,
		svSyncthingDeviceQR, svLndHubManage, svLndHubCreateAccount,
		svLndHubAccountDetail, svLndHubDeactivateConfirm:
		return true
	}
	return false
}

func isAllowedHubNameChar(key string) bool {
	if len(key) != 1 {
		return false
	}
	c := key[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == ' ' || c == '-'
}

func showMacaroonCmd(cfg *config.AppConfig) tea.Cmd {
	mac := readMacaroonHex(cfg)
	if mac == "" {
		return nil
	}
	tmpFile, err := os.CreateTemp("", "rlvpn-macaroon-")
	if err != nil {
		return nil
	}
	tmpPath := tmpFile.Name()
	tmpFile.WriteString(mac)
	tmpFile.Close()

	c := exec.Command("bash", "-c",
		"clear && echo && echo '  ═══════════════════════════════════════════'"+
			" && echo '    Admin Macaroon (hex)'"+
			" && echo '  ═══════════════════════════════════════════'"+
			" && echo && cat "+tmpPath+
			" && echo && echo && echo '  Press Enter to return...' && read"+
			" && rm -f "+tmpPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		os.Remove(tmpPath)
		return svcActionDoneMsg{}
	})
}

func (m Model) channelVisibleCount() int {
	available := theme.BoxHeight - 12
	if available < 3 {
		available = 3
	}
	return available
}
