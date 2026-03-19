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

	// ── Channel Liquidity ────────────────────────────────
	if m.status != nil && len(m.status.channels) > 0 {
		liquidity := m.dashboardLiquidity(bw)
		sections = append(sections, liquidity)
	}

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
		pubkey := m.status.lndPubkey
		if len(pubkey) > 20 {
			pubkey = pubkey[:20] + "..."
		}
		lines = append(lines, "  "+theme.Label.Render("Pubkey:  ")+
			theme.Mono.Render(pubkey))
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

func (m Model) dashboardLiquidity(bw int) string {
	var lines []string
	lines = append(lines, theme.Header.Render("  Channel Liquidity"))
	lines = append(lines, "")

	if m.status == nil || len(m.status.channels) == 0 {
		lines = append(lines, "  "+theme.Dim.Render(
			"No channels"))
		return theme.NormalBorder.Width(bw).Padding(0, 1).
			Render(strings.Join(lines, "\n"))
	}

	// Bar width for channel visualization
	barWidth := bw - 28
	if barWidth < 20 {
		barWidth = 20
	}

	// Show up to 10 channels, sorted by capacity (status already has them)
	maxShow := 10
	if maxShow > len(m.status.channels) {
		maxShow = len(m.status.channels)
	}

	for i := 0; i < maxShow; i++ {
		ch := m.status.channels[i]
		name := ch.PeerAlias
		if name == "" {
			if len(ch.RemotePubkey) > 10 {
				name = ch.RemotePubkey[:10] + ".."
			} else {
				name = ch.RemotePubkey
			}
		}
		if len(name) > 12 {
			name = name[:12]
		}
		name = fmt.Sprintf("%-12s", name)

		dot := theme.RedDot.Render("○")
		if ch.Active {
			dot = theme.GreenDot.Render("●")
		}

		bar := renderBalanceBar(ch.LocalBalance, ch.RemoteBalance,
			ch.Capacity, barWidth)
		lines = append(lines, fmt.Sprintf("  %s %s %s",
			dot, theme.Dim.Render(name), bar))
	}

	if len(m.status.channels) > maxShow {
		lines = append(lines, fmt.Sprintf("  "+
			theme.Dim.Render("  ... and %d more"),
			len(m.status.channels)-maxShow))
	}

	// Aggregate bar
	var totalLocal, totalRemote, totalCap int64
	for _, ch := range m.status.channels {
		totalLocal += ch.LocalBalance
		totalRemote += ch.RemoteBalance
		totalCap += ch.Capacity
	}
	lines = append(lines, "")
	aggBar := renderBalanceBar(totalLocal, totalRemote, totalCap, barWidth)
	lines = append(lines, "  "+theme.Label.Render("Total:       ")+aggBar)
	localPct := 0
	if totalCap > 0 {
		localPct = int(totalLocal * 100 / totalCap)
	}
	lines = append(lines, "  "+theme.Dim.Render(
		fmt.Sprintf("             %d%% outbound / %d%% inbound",
			localPct, 100-localPct)))

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

	// Channels summary
	if len(m.status.channels) > 0 {
		activeCount := 0
		for _, ch := range m.status.channels {
			if ch.Active {
				activeCount++
			}
		}
		inactive := len(m.status.channels) - activeCount
		chanText := fmt.Sprintf("%d channels", len(m.status.channels))
		if inactive > 0 {
			chanText += fmt.Sprintf(" (%d active, %d offline)",
				activeCount, inactive)
		}
		if m.status.pendingOpen > 0 {
			chanText += fmt.Sprintf(", %d pending", m.status.pendingOpen)
		}
		lines = append(lines, "  "+theme.GreenDot.Render("●")+
			" "+chanText)
	} else {
		lines = append(lines, "  "+theme.Dim.Render("○")+
			" No channels")
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
