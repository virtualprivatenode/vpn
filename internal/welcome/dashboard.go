package welcome

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) viewDashboard(bw int) string {
	if !m.cfg.HasLND() {
		return m.dashboardNoLND(bw)
	}
	if !m.cfg.WalletExists() {
		return m.dashboardNoWallet(bw)
	}
	return m.dashboardOverview(bw)
}

func (m Model) dashboardNoLND(bw int) string {
	var lines []string
	lines = append(lines, theme.Header.Render("  Node Overview"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"LND is not installed."))
	lines = append(lines, "  "+theme.Dim.Render(
		"Go to the System tab to install LND and create a wallet."))
	lines = append(lines, "")

	if m.status != nil {
		if m.status.btcSynced {
			lines = append(lines, "  "+theme.GreenDot.Render("●")+
				" Bitcoin Core synced")
		} else if m.status.btcResponding {
			pct := ""
			if m.status.btcProgress > 0 {
				pct = fmt.Sprintf(" (%.1f%%)",
					m.status.btcProgress*100)
			}
			lines = append(lines, "  "+theme.Dim.Render("◌")+
				" Bitcoin Core syncing"+pct)
		} else {
			lines = append(lines, "  "+theme.RedDot.Render("●")+
				" Bitcoin Core not responding")
		}
	}

	content := strings.Join(lines, "\n")
	return theme.Box.Width(bw).Padding(1, 2).Render(content)
}

func (m Model) dashboardNoWallet(bw int) string {
	var lines []string
	lines = append(lines, theme.Header.Render("  Node Overview"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"LND wallet has not been created."))
	lines = append(lines, "  "+theme.Dim.Render(
		"Go to the System tab to create your wallet."))
	lines = append(lines, "")

	if m.status != nil {
		if m.status.btcSynced {
			lines = append(lines, "  "+theme.GreenDot.Render("●")+
				" Bitcoin Core synced")
		} else if m.status.btcResponding {
			pct := ""
			if m.status.btcProgress > 0 {
				pct = fmt.Sprintf(" (%.1f%%)",
					m.status.btcProgress*100)
			}
			lines = append(lines, "  "+theme.Dim.Render("◌")+
				" Bitcoin Core syncing"+pct)
		}
	}

	content := strings.Join(lines, "\n")
	return theme.Box.Width(bw).Padding(1, 2).Render(content)
}

func (m Model) dashboardOverview(bw int) string {
	var sections []string

	// ── Node Identity ────────────────────────────────────
	identity := m.dashboardIdentity(bw)
	sections = append(sections, identity)

	// ── Balance Summary ──────────────────────────────────
	balances := m.dashboardBalances(bw)
	sections = append(sections, balances)

	// ── Channel List (Interactive) ───────────────────────
	channels := m.dashboardChannels(bw)
	sections = append(sections, channels)

	// ── Status Indicators ────────────────────────────────
	status := m.dashboardStatus(bw)
	sections = append(sections, status)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) dashboardIdentity(bw int) string {
	var lines []string
	lines = append(lines, theme.Header.Render("  Node"))
	lines = append(lines, "")

	if m.status == nil || !m.status.lndResponding {
		lines = append(lines, "  "+theme.Dim.Render(
			"Waiting for LND..."))
		return theme.NormalBorder.Width(bw).Padding(0, 1).
			Render(strings.Join(lines, "\n"))
	}

	if m.status.lndPubkey != "" {
		lines = append(lines, "  "+theme.Label.Render("Pubkey:  ")+
			theme.Mono.Render(m.status.lndPubkey))
	}
	lines = append(lines, "  "+theme.Label.Render("P2P:     ")+
		theme.Value.Render(p2pModeLabel(m.cfg.P2PMode)))
	lines = append(lines, "  "+theme.Label.Render("Network: ")+
		theme.Value.Render(m.cfg.Network))
	if m.cfg.AutoUnlock {
		lines = append(lines, "  "+theme.Label.Render("Unlock:  ")+
			theme.Value.Render("automatic"))
	}

	return theme.NormalBorder.Width(bw).Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (m Model) dashboardBalances(bw int) string {
	var lines []string
	lines = append(lines, theme.Header.Render("  Balances"))
	lines = append(lines, "")

	if m.status == nil || !m.status.lndResponding {
		lines = append(lines, "  "+theme.Dim.Render(
			"Waiting for LND..."))
		return theme.NormalBorder.Width(bw).Padding(0, 1).
			Render(strings.Join(lines, "\n"))
	}

	// On-chain balance
	onchain := "0"
	if m.status.lndBalance != "" {
		onchain = m.status.lndBalance
	}
	lines = append(lines, "  "+theme.Label.Render("On-chain:  ")+
		theme.Value.Render(formatSats(parseBalance(onchain))+" sats"))

	// Channel balances
	var totalCap, totalLocal, totalRemote int64
	for _, ch := range m.status.channels {
		totalCap += ch.Capacity
		totalLocal += ch.LocalBalance
		totalRemote += ch.RemoteBalance
	}

	lines = append(lines, "  "+theme.Label.Render("Sendable:  ")+
		theme.Value.Render(formatSats(totalLocal)+" sats"))
	lines = append(lines, "  "+theme.Label.Render("Receivable:")+
		theme.Value.Render(" "+formatSats(totalRemote)+" sats"))

	if totalCap > 0 {
		lines = append(lines, "  "+theme.Label.Render("Capacity:  ")+
			theme.Dim.Render(formatSats(totalCap)+" sats"))
	}

	// Total
	totalBalance := parseBalance(onchain) + totalLocal
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Total:     ")+
		theme.Good.Render(formatSats(totalBalance)+" sats"))

	return theme.NormalBorder.Width(bw).Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (m Model) dashboardChannels(bw int) string {
	var lines []string

	activeCount := 0
	inactiveCount := 0
	if m.status != nil {
		for _, ch := range m.status.channels {
			if ch.Active {
				activeCount++
			} else {
				inactiveCount++
			}
		}
	}

	headerText := "  Channels"
	if m.status != nil && len(m.status.channels) > 0 {
		headerText = fmt.Sprintf("  Channels (%d active", activeCount)
		if inactiveCount > 0 {
			headerText += fmt.Sprintf(", %d offline", inactiveCount)
		}
		if m.status.pendingOpen > 0 {
			headerText += fmt.Sprintf(", %d pending", m.status.pendingOpen)
		}
		headerText += ")"
	}
	lines = append(lines, theme.Header.Render(headerText))
	lines = append(lines, "")

	if m.status == nil || !m.status.lndResponding {
		lines = append(lines, "  "+theme.Dim.Render("Waiting for LND..."))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Action.Render("o  Open Channel"))
		return theme.NormalBorder.Width(bw).Padding(0, 1).
			Render(strings.Join(lines, "\n"))
	}

	if len(m.status.channels) == 0 {
		lines = append(lines, "  "+theme.Dim.Render(
			"No channels yet. Open a channel to start."))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Action.Render("o  Open Channel"))
		return theme.NormalBorder.Width(bw).Padding(0, 1).
			Render(strings.Join(lines, "\n"))
	}

	// Channel list with selection cursor
	visibleCount := theme.BoxHeight - 10
	if visibleCount < 3 {
		visibleCount = 3
	}

	if m.chanScrollOffset > 0 {
		lines = append(lines, "  "+theme.Dim.Render("  ↑ more"))
	}

	viewEnd := m.chanScrollOffset + visibleCount
	if viewEnd > len(m.status.channels) {
		viewEnd = len(m.status.channels)
	}

	barWidth := bw - 28
	if barWidth < 10 {
		barWidth = 10
	}

	for i := m.chanScrollOffset; i < viewEnd; i++ {
		ch := m.status.channels[i]
		prefix := "  "
		nameStyle := theme.Value
		if m.chanCursor == i {
			prefix = "▸ "
			nameStyle = theme.Action
		}
		dot := theme.RedDot.Render("○")
		if ch.Active {
			dot = theme.GreenDot.Render("●")
		}
		name := ch.PeerAlias
		if name == "" {
			if len(ch.RemotePubkey) > 12 {
				name = ch.RemotePubkey[:12] + "..."
			} else {
				name = ch.RemotePubkey
			}
		}
		if len(name) > 14 {
			name = name[:14]
		}
		name = fmt.Sprintf("%-14s", name)

		bar := renderBalanceBar(ch.LocalBalance, ch.RemoteBalance,
			ch.Capacity, barWidth)
		lines = append(lines, fmt.Sprintf("%s%s %s %s",
			prefix, dot, nameStyle.Render(name), bar))
	}

	if viewEnd < len(m.status.channels) {
		lines = append(lines, "  "+theme.Dim.Render("  ↓ more"))
	}

	// Totals
	lines = append(lines, "")
	var totalLocal, totalRemote int64
	for _, ch := range m.status.channels {
		totalLocal += ch.LocalBalance
		totalRemote += ch.RemoteBalance
	}
	lines = append(lines, "  "+theme.Label.Render("Send: ")+
		theme.Value.Render(formatSats(totalLocal)))
	lines = append(lines, "  "+theme.Label.Render("Recv: ")+
		theme.Value.Render(formatSats(totalRemote)))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Action.Render("o  Open Channel"))

	return theme.NormalBorder.Width(bw).Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (m Model) dashboardStatus(bw int) string {
	var lines []string
	lines = append(lines, theme.Header.Render("  Status"))
	lines = append(lines, "")

	if m.status == nil {
		lines = append(lines, "  "+theme.Dim.Render("Loading..."))
		return theme.NormalBorder.Width(bw).Padding(0, 1).
			Render(strings.Join(lines, "\n"))
	}

	// Bitcoin sync
	if m.status.btcSynced {
		lines = append(lines, "  "+theme.GreenDot.Render("●")+
			" Bitcoin synced")
	} else if m.status.btcResponding {
		pct := ""
		if m.status.btcProgress > 0 {
			pct = fmt.Sprintf(" %.1f%%", m.status.btcProgress*100)
		}
		lines = append(lines, "  "+theme.Dim.Render("◌")+
			" Bitcoin syncing"+pct)
	} else {
		lines = append(lines, "  "+theme.RedDot.Render("●")+
			" Bitcoin not responding")
	}

	// LND status
	if m.status.lndResponding {
		if m.status.lndSyncedChain && m.status.lndSyncedGraph {
			lines = append(lines, "  "+theme.GreenDot.Render("●")+
				" LND fully synced")
		} else if m.status.lndSyncedChain {
			lines = append(lines, "  "+theme.Dim.Render("◌")+
				" LND chain synced, graph syncing")
		} else {
			lines = append(lines, "  "+theme.Dim.Render("◌")+
				" LND syncing")
		}
	} else {
		lines = append(lines, "  "+theme.RedDot.Render("●")+
			" LND not responding")
	}

	// Tor
	if active, ok := m.status.services["tor"]; ok && active {
		lines = append(lines, "  "+theme.GreenDot.Render("●")+
			" Tor connected")
	} else {
		lines = append(lines, "  "+theme.RedDot.Render("●")+
			" Tor not running")
	}

	return theme.NormalBorder.Width(bw).Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func p2pModeLabel(mode string) string {
	if mode == "hybrid" {
		return "Tor + clearnet"
	}
	return "Tor only"
}
