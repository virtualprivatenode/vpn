// internal/welcome/keys.go

package welcome

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
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
				logger.TUI("Deactivated account %s (balance: %s sats)", acct.Label, msg.balance)
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
			m.cfg.SyncthingDevices = append(m.cfg.SyncthingDevices, config.SyncthingDevice{
				Name:     "Device " + fmt.Sprintf("%d", len(m.cfg.SyncthingDevices)+1),
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
	case tickMsg:
		if m.fetchInFlight {
			return m, tickEvery(30 * time.Second)
		}
		m.fetchInFlight = true
		return m, tea.Batch(
			fetchStatus(m.cfg, m.lndClient),
			tickEvery(30*time.Second),
		)
	}
	return m, nil
}

func isAllowedHubNameChar(key string) bool {
	if len(key) != 1 {
		return false
	}
	c := key[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == ' ' || c == '-'
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// ── Text input subviews ──────────────────────────────

	if m.subview == svLndHubCreateName {
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "backspace":
			if len(m.hubNameInput) > 0 {
				m.hubNameInput = m.hubNameInput[:len(m.hubNameInput)-1]
			} else {
				m.subview = svLndHubManage
			}
			return m, nil
		case "enter":
			if m.hubNameInput != "" {
				return m, createLndHubAccountCmd(m.cfg.LndHubAdminToken)
			}
			return m, nil
		default:
			if isAllowedHubNameChar(key) && len(m.hubNameInput) < 30 {
				m.hubNameInput += key
			}
			return m, nil
		}
	}

	if m.subview == svSyncthingPairInput {
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "backspace":
			if len(m.syncDeviceInput) > 0 {
				m.syncDeviceInput = m.syncDeviceInput[:len(m.syncDeviceInput)-1]
			} else {
				m.syncPairError = ""
				m.syncPairSuccess = false
				m.subview = svSyncthingDetail
			}
			return m, nil
		case "enter":
			if m.syncPairSuccess {
				m.syncDeviceInput = ""
				m.syncPairSuccess = false
				m.subview = svSyncthingDetail
				return m, nil
			}
			if m.syncDeviceInput != "" {
				parts := strings.Split(m.syncDeviceInput, "-")
				if len(parts) != 8 {
					m.syncPairError = "Invalid Device ID format. Expected 8 groups separated by hyphens."
					return m, nil
				}
				for _, p := range parts {
					if len(p) != 7 {
						m.syncPairError = "Invalid Device ID format. Each group should be 7 characters."
						return m, nil
					}
				}
				m.syncPairError = ""
				return m, pairSyncthingDeviceCmd(m.syncDeviceInput)
			}
			return m, nil
		default:
			for _, ch := range key {
				if len(m.syncDeviceInput) >= 63 {
					break
				}
				if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') ||
					(ch >= '0' && ch <= '9') || ch == '-' {
					m.syncDeviceInput += strings.ToUpper(string(ch))
				}
			}
			return m, nil
		}
	}

	// ── Channel open subviews ────────────────────────────

	if m.subview == svChannelOpen {
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

	if m.subview == svChannelCustomPeer {
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

	if m.subview == svChannelAmountSelect {
		// Custom amount is the last preset (value 0)
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
			m.chanOpenPrivate = true // Default to unannounced
			m.chanOpenError = ""
			m.subview = svChannelOpenConfirm
			return m, nil
		default:
			// Allow typing digits when custom is selected
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

	if m.subview == svChannelOpenConfirm {
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

	if m.subview == svChannelOpening {
		if key == "q" || key == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}

	if m.subview == svChannelOpenResult {
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

	if m.subview == svChannelFundWallet {
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

	// ── Generic subview handlers ─────────────────────────

	if m.subview != svNone {
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
			case svSyncthingDetail:
				m.subview = svNone
			case svSyncthingDeviceDetail:
				m.subview = svSyncthingDetail
			case svSyncthingWebUI:
				m.subview = svSyncthingDetail
			case svSyncthingDeviceQR:
				m.subview = svSyncthingDetail
			case svChannelDetail:
				m.subview = svNone
			case svLndHubManage:
				m.subview = svNone
			case svLndHubCreateName:
				m.hubNameInput = ""
				m.subview = svLndHubManage
			case svLndHubCreateAccount:
				m.lastAccount = nil
				m.hubNameInput = ""
				m.subview = svLndHubManage
			case svLndHubAccountDetail:
				m.subview = svLndHubManage
			case svLndHubDeactivateConfirm:
				m.subview = svLndHubManage
			default:
				m.subview = svNone
			}
			return m, nil
		case "s":
			if m.subview == svSyncthingWebUI {
				m.showSecrets = !m.showSecrets
				return m, nil
			}
		case "a":
			if m.subview == svSyncthingDetail {
				m.syncDeviceInput = ""
				m.syncPairError = ""
				m.syncPairSuccess = false
				m.subview = svSyncthingPairInput
				return m, nil
			}
		case "d":
			if m.subview == svSyncthingDetail {
				m.subview = svSyncthingDeviceQR
				return m, nil
			}
		case "m":
			if m.subview == svZeus {
				return m, showMacaroonCmd(m.cfg)
			}
		case "r":
			if m.subview == svZeus {
				m.qrMode = "tor"
				m.qrLabel = ""
				m.subview = svQR
				return m, nil
			}
			if m.subview == svLndHubCreateAccount && m.lastAccount != nil {
				hubOnion := readOnion(paths.TorLndHubHostname)
				if hubOnion != "" {
					m.urlTarget = fmt.Sprintf("lndhub://%s:%s@http://%s:%s",
						m.lastAccount.Login, m.lastAccount.Password,
						hubOnion, paths.LndHubExternalPort)
					m.qrLabel = m.hubNameInput + " — Tor"
					m.subview = svQR
				}
				return m, nil
			}
		case "c":
			if m.subview == svZeus && m.cfg.P2PMode == "hybrid" {
				m.qrMode = "clearnet"
				m.qrLabel = ""
				m.subview = svQR
				return m, nil
			}
			if m.subview == svLndHubManage {
				m.hubNameInput = ""
				m.subview = svLndHubCreateName
				return m, nil
			}
			if m.subview == svLndHubCreateAccount &&
				m.cfg.P2PMode == "hybrid" && m.lastAccount != nil {
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
				return m, nil
			}
		case "p":
			if m.subview == svLightning && m.cfg.P2PMode == "tor" && m.cfg.HasLND() {
				m.shellAction = svP2PUpgrade
				return m, tea.Quit
			}
		case "u":
			if m.subview == svSyncthingDetail {
				m.subview = svSyncthingWebUI
				return m, nil
			}
			if m.subview == svSyncthingWebUI {
				syncOnion := readOnion(paths.TorSyncthingHostname)
				if syncOnion != "" {
					m.urlTarget = "http://" + syncOnion + ":8384"
					m.urlReturnTo = svSyncthingWebUI
					m.subview = svFullURL
				}
				return m, nil
			}
			if m.subview == svLndHubManage {
				hubOnion := readOnion(paths.TorLndHubHostname)
				if hubOnion != "" {
					m.urlTarget = "http://" + hubOnion + ":" + paths.LndHubExternalPort
					m.urlReturnTo = svLndHubManage
					m.subview = svFullURL
				}
				return m, nil
			}
		case "x":
			if m.subview == svLndHubManage && len(m.cfg.LndHubAccounts) > 0 {
				if m.hubCursor < len(m.cfg.LndHubAccounts) &&
					m.cfg.LndHubAccounts[m.hubCursor].Active {
					m.subview = svLndHubDeactivateConfirm
				}
				return m, nil
			}
		case "y":
			if m.subview == svLndHubDeactivateConfirm {
				if m.hubCursor < len(m.cfg.LndHubAccounts) {
					login := m.cfg.LndHubAccounts[m.hubCursor].Login
					return m, deactivateLndHubAccountCmd(login)
				}
				return m, nil
			}
		case "n":
			if m.subview == svLndHubDeactivateConfirm {
				m.subview = svLndHubManage
				return m, nil
			}
		case "enter":
			if m.subview == svSyncthingDetail && len(m.cfg.SyncthingDevices) > 0 {
				m.subview = svSyncthingDeviceDetail
				return m, nil
			}
			if m.subview == svLndHubManage && len(m.cfg.LndHubAccounts) > 0 {
				m.subview = svLndHubAccountDetail
				return m, nil
			}
			if m.subview == svLndHubCreateAccount {
				m.lastAccount = nil
				m.hubNameInput = ""
				m.subview = svLndHubManage
				return m, nil
			}
		case "up", "k":
			if m.subview == svSyncthingDetail && m.syncCursor > 0 {
				m.syncCursor--
			}
			if m.subview == svLndHubManage && m.hubCursor > 0 {
				m.hubCursor--
			}
			return m, nil
		case "down", "j":
			if m.subview == svSyncthingDetail &&
				m.syncCursor < len(m.cfg.SyncthingDevices)-1 {
				m.syncCursor++
			}
			if m.subview == svLndHubManage &&
				m.hubCursor < len(m.cfg.LndHubAccounts)-1 {
				m.hubCursor++
			}
			return m, nil
		}
		return m, nil
	}

	// ── Inside a card ────────────────────────────────────

	if m.cardActive {
		return m.handleCardKey(key)
	}

	// ── Settings tab ─────────────────────────────────────

	if m.activeTab == tabSettings {
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activeTab = (m.activeTab + 1) % 5
			m.updateConfirm = false
			return m, nil
		case "shift+tab":
			m.activeTab = (m.activeTab + 4) % 5
			m.updateConfirm = false
			return m, nil
		case "1":
			m.activeTab = tabDashboard
			m.updateConfirm = false
		case "2":
			m.activeTab = tabChannels
			m.updateConfirm = false
		case "3":
			m.activeTab = tabPairing
			m.updateConfirm = false
		case "4":
			m.activeTab = tabAddons
			m.updateConfirm = false
		case "5":
			// already on settings
		default:
			m = handleSettingsKey(m, key)
			if m.shellAction != svNone {
				return m, tea.Quit
			}
			return m, nil
		}
		return m, nil
	}

	// ── Main navigation ──────────────────────────────────

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
		m.activeTab = tabChannels
	case "3":
		m.activeTab = tabPairing
	case "4":
		m.activeTab = tabAddons
	case "5":
		m.activeTab = tabSettings
	case "o":
		if m.activeTab == tabChannels {
			return m.startChannelOpen()
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

func (m Model) handleCardKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "backspace":
		m.cardActive = false
		m.svcConfirm = ""
		m.sysConfirm = ""
		return m, nil
	case "q":
		return m, tea.Quit
	case "tab":
		m.activeTab = (m.activeTab + 1) % 5
		m.cardActive = false
		m.svcConfirm = ""
		m.sysConfirm = ""
		return m, nil
	case "shift+tab":
		m.activeTab = (m.activeTab + 4) % 5
		m.cardActive = false
		m.svcConfirm = ""
		m.sysConfirm = ""
		return m, nil
	}

	if m.dashCard == cardServices {
		if m.svcConfirm != "" {
			switch key {
			case "y":
				svc := m.svcName(m.svcCursor)
				action := m.svcConfirm
				m.svcConfirm = ""
				return m, func() tea.Msg {
					system.SudoRun("systemctl", action, svc)
					return svcActionDoneMsg{}
				}
			default:
				m.svcConfirm = ""
				return m, nil
			}
		}
		switch key {
		case "up", "k":
			if m.svcCursor > 0 {
				m.svcCursor--
			}
		case "down", "j":
			if m.svcCursor < m.svcCount()-1 {
				m.svcCursor++
			}
		case "r":
			m.svcConfirm = "restart"
		case "s":
			m.svcConfirm = "stop"
		case "a":
			m.svcConfirm = "start"
		case "l":
			svc := m.svcName(m.svcCursor)
			c := exec.Command("bash", "-c",
				"clear && sudo journalctl -u "+svc+" -n 100 --no-pager"+
					" && echo && echo '  Press Enter to return...' && read")
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return svcActionDoneMsg{}
			})
		}
	}

	if m.dashCard == cardSystem {
		if m.sysConfirm != "" {
			switch key {
			case "y":
				action := m.sysConfirm
				m.sysConfirm = ""
				if action == "update" {
					c := exec.Command("bash", "-c",
						"clear && sudo apt-get update && sudo apt-get upgrade -y"+
							" && echo && echo '  ✅ Update complete'"+
							" && echo '  Press Enter to return...' && read")
					return m, tea.ExecProcess(c, func(err error) tea.Msg {
						return svcActionDoneMsg{}
					})
				}
				if action == "reboot" {
					return m, func() tea.Msg {
						system.SudoRun("reboot")
						return svcActionDoneMsg{}
					}
				}
			default:
				m.sysConfirm = ""
				return m, nil
			}
			return m, nil
		}
		switch key {
		case "u":
			m.sysConfirm = "update"
		case "r":
			if m.status != nil && m.status.rebootRequired {
				m.sysConfirm = "reboot"
			}
		}
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
	case tabChannels:
		if m.chanCursor > 0 {
			m.chanCursor--
			if m.chanCursor < m.chanScrollOffset {
				m.chanScrollOffset = m.chanCursor
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
	case tabChannels:
		if m.status != nil && m.chanCursor < len(m.status.channels)-1 {
			m.chanCursor++
			visibleCount := m.channelVisibleCount()
			if m.chanCursor >= m.chanScrollOffset+visibleCount {
				m.chanScrollOffset = m.chanCursor - visibleCount + 1
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
			m.subview = svLightning
		}
	case tabChannels:
		if m.status != nil && len(m.status.channels) == 0 {
			return m.startChannelOpen()
		}
		if m.status != nil && len(m.status.channels) > 0 &&
			m.chanCursor < len(m.status.channels) {
			m.subview = svChannelDetail
			return m, nil
		}
	case tabPairing:
		if m.cfg.HasLND() && m.cfg.WalletExists() {
			m.subview = svZeus
		}
	case tabAddons:
		return m.handleAddonEnter()
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

func (m Model) svcCount() int {
	return len(serviceNames(m.cfg))
}

func (m Model) svcName(i int) string {
	names := serviceNames(m.cfg)
	if i < len(names) {
		return names[i]
	}
	return ""
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
