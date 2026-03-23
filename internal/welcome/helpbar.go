package welcome

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) renderHelpBar(maxW int) string {
	bindings := m.currentBindings()
	return renderBindings(bindings, maxW)
}

func renderBindings(
	bindings []key.Binding, maxW int,
) string {
	if maxW <= 0 {
		return ""
	}

	sep := theme.HelpSep.Render(" │ ")
	sepW := lipgloss.Width(sep)

	// Build all parts first
	type helpPart struct {
		rendered string
		width    int
	}
	var parts []helpPart
	for _, b := range bindings {
		if !b.Enabled() {
			continue
		}
		h := b.Help()
		if h.Key == "" {
			continue
		}
		part := theme.HelpKey.Render(h.Key) +
			" " +
			theme.HelpDesc.Render(h.Desc)
		parts = append(parts, helpPart{
			rendered: part,
			width:    lipgloss.Width(part),
		})
	}

	if len(parts) == 0 {
		return ""
	}

	// Calculate how many parts fit within maxW
	// Start with all parts, drop from end if too wide
	fitCount := len(parts)
	for fitCount > 0 {
		totalW := 0
		for i := 0; i < fitCount; i++ {
			totalW += parts[i].width
			if i < fitCount-1 {
				totalW += sepW
			}
		}
		if totalW <= maxW {
			break
		}
		fitCount--
	}

	if fitCount == 0 {
		// Even one part doesn't fit, show first
		// truncated
		if len(parts) > 0 {
			return parts[0].rendered
		}
		return ""
	}

	var strs []string
	for i := 0; i < fitCount; i++ {
		strs = append(strs, parts[i].rendered)
	}

	return strings.Join(strs, sep)
}

func (m Model) currentBindings() []key.Binding {
	hasTabs := m.hasDetailTabs()

	// ── Fullscreen views ─────────────────────────
	if m.subview == svQR || m.subview == svFullURL ||
		m.subview == svSyncthingDeviceQR {
		return newFullscreenBindings().ShortHelp()
	}

	// ── Confirm dialogs ──────────────────────────
	if m.svcConfirm != "" || m.sysConfirm != "" ||
		m.updateConfirm {
		return newConfirmBindings().ShortHelp()
	}

	// ── Sidebar focused ──────────────────────────
	if m.nav.Focused {
		return newSidebarBindings().ShortHelp()
	}

	// ── Tab bar focused ──────────────────────────
	if m.tabFocused {
		return newTabBarBindings().ShortHelp()
	}

	// ── Detail tab content (view-only) ───────────
	tabs := m.effectiveTabs()
	if m.activeTab > 0 && m.activeTab < len(tabs) {
		tab := tabs[m.activeTab]
		switch tab.Kind {
		case tabChannel, tabPayment, tabOnChainTx:
			return newDetailTabBindings(hasTabs).
				ShortHelp()
		}
	}

	// ── Content focused — dispatch by subview ────
	switch m.subview {
	case svChannelOpen:
		return newPeerSelectBindings(hasTabs).
			ShortHelp()
	case svChannelCustomPeer:
		return newTwoFieldBindings(hasTabs).
			ShortHelp()
	case svChannelAmountSelect:
		return newAmountSelectBindings(hasTabs).
			ShortHelp()
	case svChannelOpenConfirm:
		return newChanConfirmBindings(hasTabs).
			ShortHelp()
	case svChannelOpening:
		return newWaitingBindings().ShortHelp()
	case svChannelOpenResult:
		return newResultBindings().ShortHelp()
	case svChannelFundWallet:
		return newTextInputBindings(hasTabs).
			ShortHelp()
	case svSend:
		return newSendInputBindings(hasTabs).
			ShortHelp()
	case svSendConfirm:
		return newPayConfirmBindings(hasTabs).
			ShortHelp()
	case svSendInFlight:
		return newWaitingBindings().ShortHelp()
	case svSendResult:
		return newResultBindings().ShortHelp()
	case svReceive:
		return newRecvInputBindings(hasTabs).
			ShortHelp()
	case svReceiveWaiting:
		return newRecvWaitingBindings(hasTabs).
			ShortHelp()
	case svReceivePaid:
		return newResultBindings().ShortHelp()
	case svReceiveExpired:
		return newResultBindings().ShortHelp()
	case svOnChainSendAddr:
		return newTextInputBindings(hasTabs).
			ShortHelp()
	case svOnChainSendAmount:
		return newOCSendAmountBindings(hasTabs).
			ShortHelp()
	case svOnChainSendConfirm:
		return newPayConfirmBindings(hasTabs).
			ShortHelp()
	case svOnChainSendBroadcast:
		return newWaitingBindings().ShortHelp()
	case svOnChainResult:
		return newResultBindings().ShortHelp()
	case svSyncthingDetail:
		return newAddonDetailBindings(hasTabs).
			ShortHelp()
	case svSyncthingPairInput:
		return newTextInputBindings(hasTabs).
			ShortHelp()
	case svSyncthingWebUI:
		return newAddonDetailBindings(hasTabs).
			ShortHelp()
	case svSyncthingDeviceDetail:
		return newTextInputBindings(hasTabs).
			ShortHelp()
	case svLndHubManage:
		return newAddonDetailBindings(hasTabs).
			ShortHelp()
	case svLndHubCreateName:
		return newTextInputBindings(hasTabs).
			ShortHelp()
	case svLndHubCreateAccount:
		return newResultBindings().ShortHelp()
	case svLndHubAccountDetail:
		return newTextInputBindings(hasTabs).
			ShortHelp()
	case svLndHubDeactivateConfirm:
		return newConfirmBindings().ShortHelp()
	case svWalletPairing, svZeusPairing:
		return newAddonDetailBindings(hasTabs).
			ShortHelp()
	}

	// ── Section home views ───────────────────────
	sec := m.nav.ActiveSection()
	switch sec {
	case secChannels:
		return newChannelsHomeBindings(
			hasTabs,
			m.contentFocus == 1).ShortHelp()
	case secWallet:
		return newWalletHomeBindings(
			hasTabs,
			m.contentFocus == 1).ShortHelp()
	case secOnChain:
		return newOnChainHomeBindings(hasTabs,
			m.onChainTxFocus).ShortHelp()
	case secAddons:
		return newAddonsHomeBindings(hasTabs).
			ShortHelp()
	case secSystem:
		return newSystemHomeBindings(hasTabs,
			m.contentFocus == 0).ShortHelp()
	}

	return []key.Binding{kQuit}
}
