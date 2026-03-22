package welcome

import (
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// renderHelpBar returns a styled help string for the
// current state. Uses our own renderer instead of
// bubbles/help to match the existing visual style.

func (m Model) renderHelpBar(maxW int) string {
	bindings := m.currentBindings()
	return renderBindings(bindings, maxW)
}

func renderBindings(
	bindings []key.Binding, maxW int,
) string {
	var parts []string
	for _, b := range bindings {
		if !b.Enabled() {
			continue
		}
		h := b.Help()
		if h.Key == "" && h.Desc == "" {
			continue
		}
		if h.Key == "" {
			continue
		}
		part := theme.HelpKey.Render(h.Key) +
			" " +
			theme.HelpDesc.Render(h.Desc)
		parts = append(parts, part)
	}

	sep := theme.HelpSep.Render(" │ ")
	result := strings.Join(parts, sep)

	// Truncate if too wide
	if maxW > 0 && len(result) > maxW*2 {
		// rough truncation — visual width is less
		// than byte length due to ANSI codes
		result = result[:maxW*2]
	}

	return result
}

// currentBindings returns the key bindings appropriate
// for the current focus state and subview.
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
		return newTextInputBindings(hasTabs).
			ShortHelp()
	case svSendConfirm:
		return newPayConfirmBindings(hasTabs).
			ShortHelp()
	case svSendInFlight:
		return newWaitingBindings().ShortHelp()
	case svSendResult:
		return newResultBindings().ShortHelp()
	case svReceive:
		return newTwoFieldBindings(hasTabs).
			ShortHelp()
	case svReceiveWaiting:
		return newRecvWaitingBindings(hasTabs).
			ShortHelp()
	case svReceivePaid:
		return newResultBindings().ShortHelp()
	case svReceiveExpired:
		return newResultBindings().ShortHelp()
	case svOnChain:
		return newOnChainHomeBindings(hasTabs,
			m.onChainTxFocus).ShortHelp()
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
	case secAddons:
		return newAddonsHomeBindings(hasTabs).
			ShortHelp()
	case secSystem:
		return newSystemHomeBindings(hasTabs).
			ShortHelp()
	}

	return []key.Binding{kQuit}
}
