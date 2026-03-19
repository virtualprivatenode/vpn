package welcome

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) View() tea.View {
	var content string

	if m.width == 0 {
		content = "Loading..."
	} else {
		switch m.subview {
		case svWalletInfo:
			content = m.viewWalletInfo()
		case svZeus:
			content = m.viewZeus()
		case svSyncthingDetail:
			content = m.viewSyncthingDetail()
		case svSyncthingPairInput:
			content = m.viewSyncthingPairInput()
		case svSyncthingDeviceDetail:
			content = m.viewSyncthingDeviceDetail()
		case svSyncthingWebUI:
			content = m.viewSyncthingWebUI()
		case svSyncthingDeviceQR:
			content = m.viewSyncthingDeviceQR()
		case svChannelDetail:
			content = m.viewChannelDetail()
		case svChannelOpen:
			content = m.viewChannelOpen()
		case svChannelAmountSelect:
			content = m.viewChannelAmountSelect()
		case svChannelCustomPeer:
			content = m.viewChannelCustomPeer()
		case svChannelOpenConfirm:
			content = m.viewChannelOpenConfirm()
		case svChannelOpening:
			content = m.viewChannelOpening()
		case svChannelOpenResult:
			content = m.viewChannelOpenResult()
		case svChannelFundWallet:
			content = m.viewChannelFundWallet()
		case svLndHubManage:
			content = m.viewLndHubManage()
		case svLndHubCreateName:
			content = m.viewLndHubCreateName()
		case svLndHubCreateAccount:
			content = m.viewLndHubNewAccount()
		case svLndHubAccountDetail:
			content = m.viewLndHubAccountDetail()
		case svLndHubDeactivateConfirm:
			content = m.viewLndHubDeactivateConfirm()
		case svQR:
			content = m.viewQR()
		case svFullURL:
			content = m.viewFullURL()
		case svReceive:
			content = m.viewReceive()
		case svReceiveWaiting:
			content = m.viewReceiveWaiting()
		case svReceivePaid:
			content = m.viewReceivePaid()
		case svReceiveExpired:
			content = m.viewReceiveExpired()
		case svSend:
			content = m.viewSend()
		case svSendConfirm:
			content = m.viewSendConfirm()
		case svSendInFlight:
			content = m.viewSendInFlight()
		case svSendResult:
			content = m.viewSendResult()
		case svPaymentHistory:
			content = m.viewPaymentHistory()
		case svPaymentDetail:
			content = m.viewPaymentDetail()
		default:
			bw := min(m.width-4, theme.ContentWidth)
			var tabContent string
			switch m.activeTab {
			case tabDashboard:
				tabContent = m.viewDashboard(bw)
			case tabWallet:
				tabContent = m.viewWalletTab(bw)
			case tabPairing:
				tabContent = m.viewPairing(bw)
			case tabAddons:
				tabContent = m.viewAddons(bw)
			case tabSystem:
				tabContent = m.viewSystem(bw)
			}

			title := theme.Title.Width(bw).Align(lipgloss.Center).
				Render(fmt.Sprintf(" Virtual Private Node v%s ", m.version))
			tabs := m.viewTabs(bw)
			footer := m.viewFooter()
			body := lipgloss.JoinVertical(lipgloss.Center,
				"", title, "", tabs, "", tabContent)
			content = lipgloss.JoinVertical(lipgloss.Center, body, "", footer)
			content = lipgloss.Place(m.width, m.height,
				lipgloss.Center, lipgloss.Center, content)
		}
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) viewTabs(tw int) string {
	w := tw / len(m.tabBar.Labels)

	// Create a copy so we don't mutate the model during render
	bg := m.tabBar
	bg.ActiveIndex = int(m.activeTab)
	bg.SetWidth(w)

	return bg.View()
}

func (m Model) viewFooter() string {
	// System tab card-active footers
	if m.cardActive && m.activeTab == tabSystem {
		if m.sysCard == cardServices {
			return theme.Footer.Render(
				"  ↑↓ select • [r]estart [s]top [a]start [l]ogs • backspace back • q quit  ")
		}
		if m.sysCard == cardSysStats {
			if m.status != nil && m.status.rebootRequired {
				return theme.Footer.Render(
					"  [u]pdate • [r]eboot • backspace back • q quit  ")
			}
			return theme.Footer.Render(
				"  [u]pdate system • backspace back • q quit  ")
		}
	}
	switch m.activeTab {
	case tabDashboard:
		if m.status != nil && len(m.status.channels) > 0 {
			return theme.Footer.Render(
				"  ↑↓ select  enter details  o open channel  tab switch  q quit  ")
		}
		return theme.Footer.Render(
			"  tab switch  q quit  ")
	case tabWallet:
		if m.walletPaneFocused {
			switch m.walletSidebar.ActiveIndex {
			case walletSectionTransactions:
				if len(m.payHistory) > 0 {
					return theme.Footer.Render(
						"  ↑↓ select  enter details  ←/backspace sidebar  tab switch  q quit  ")
				}
				return theme.Footer.Render(
					"  ←/backspace sidebar  tab switch  q quit  ")
			default:
				return theme.Footer.Render(
					"  ←/backspace sidebar  tab switch  q quit  ")
			}
		}
		return theme.Footer.Render(
			"  ↑↓ navigate  enter/→ select  tab switch  q quit  ")
	case tabPairing:
		return theme.Footer.Render(
			"  enter open  tab switch  q quit  ")
	case tabAddons:
		return theme.Footer.Render(
			"  ←→ select  enter install/view  tab switch  q quit  ")
	case tabSystem:
		if m.updateConfirm {
			return theme.Footer.Render(
				"  y confirm  any key cancel  ")
		}
		return theme.Footer.Render(
			"  ↑↓←→ navigate  enter select  tab switch  q quit  ")
	}
	return ""
}

func (m Model) viewFullURL() string {
	title := theme.Header.Render("Full URL — Copy and paste into Tor Browser")
	hint := theme.Dim.Render("Select and copy. Press backspace to go back.")
	content := lipgloss.JoinVertical(lipgloss.Left,
		"", title, "", hint, "", m.urlTarget, "")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, content)
}

func (m Model) viewWalletTab(bw int) string {
	sidebarW := 18
	gap := 2
	contentW := bw - sidebarW - gap

	// Render sidebar
	sidebarContent := m.walletSidebar.View()

	// Render content pane based on active sidebar button
	var content string
	switch m.walletSidebar.ActiveIndex {
	case walletSectionTransactions:
		content = m.walletTransactionsPane(contentW)
	case walletSectionSend:
		content = m.walletSendPane(contentW)
	case walletSectionReceive:
		content = m.walletReceivePane(contentW)
	case walletSectionOnChain:
		content = m.walletOnChainPane(contentW)
	default:
		content = m.walletTransactionsPane(contentW)
	}

	// Match sidebar height to content pane height
	contentHeight := lipgloss.Height(content)
	sidebar := lipgloss.NewStyle().
		Height(contentHeight).
		Render(sidebarContent)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		sidebar, strings.Repeat(" ", gap), content)
}

func (m Model) viewWalletInfo() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡️ Wallet & Node Info"))
	lines = append(lines, "")

	if m.cfg.WalletExists() {
		lines = append(lines, "  "+theme.Label.Render("Wallet: ")+
			theme.Success.Render("created"))
		if m.cfg.AutoUnlock {
			lines = append(lines, "  "+theme.Label.Render("Auto-unlock: ")+
				theme.Success.Render("enabled"))
		}
		lines = append(lines, "  "+theme.Label.Render("P2P Mode: ")+
			theme.Value.Render(p2pModeLabel(m.cfg.P2PMode)))

		if m.status != nil && m.status.lndResponding {
			if m.status.lndBalance != "" {
				lines = append(lines, "  "+theme.Label.Render("Balance: ")+
					theme.Value.Render(m.status.lndBalance+" sats"))
			}
			if m.status.lndChannels > 0 {
				lines = append(lines, "  "+theme.Label.Render("Channels: ")+
					theme.Value.Render(fmt.Sprintf("%d", m.status.lndChannels)))
			}
			if m.status.lndPubkey != "" {
				lines = append(lines, "")
				lines = append(lines, "  "+theme.Label.Render("Pubkey:"))
				lines = append(lines, "  "+
					theme.Mono.Render(m.status.lndPubkey))
			}
		} else {
			lines = append(lines, "")
			lines = append(lines, "  "+
				theme.Dim.Render("Waiting for LND..."))
		}

		if m.cfg.P2PMode == "tor" {
			lines = append(lines, "")
			lines = append(lines, "  "+theme.Action.Render(
				"[p] upgrade to clearnet+tor"))
		}
	} else {
		lines = append(lines, "  "+
			theme.Warning.Render("Wallet not created"))
	}

	content := strings.Join(lines, "\n")
	box := theme.Box.Width(bw).Padding(1, 2).Render(content)
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡️ Wallet & Node Info ")
	var footer string
	if m.cfg.P2PMode == "tor" {
		footer = theme.Footer.Render(
			"  p upgrade P2P • backspace back • q quit  ")
	} else {
		footer = theme.Footer.Render(
			"  backspace back • q quit  ")
	}
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func padLines(lines []string, target int) string {
	for len(lines) < target {
		lines = append(lines, "")
	}
	if len(lines) > target {
		lines = lines[:target]
	}
	return strings.Join(lines, "\n")
}

func parseBalance(s string) int64 {
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	return n
}
