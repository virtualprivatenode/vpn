package welcome

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Transactions Pane ────────────────────────────────────

func (m Model) walletTransactionsPane(w int) string {
	var lines []string

	lines = append(lines, theme.Header.Render("Transactions"))
	lines = append(lines, "")

	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		lines = append(lines, theme.Dim.Render(
			"Install LND and create wallet to view transactions."))
		return theme.NormalBorder.Width(w).Padding(1, 2).
			Render(padLines(lines, theme.BoxHeight))
	}

	if m.status == nil || !m.status.lndResponding {
		lines = append(lines, theme.Dim.Render("Waiting for LND..."))
		return theme.NormalBorder.Width(w).Padding(1, 2).
			Render(padLines(lines, theme.BoxHeight))
	}

	if len(m.payHistory) == 0 {
		lines = append(lines, theme.Dim.Render("No payments yet."))
		return theme.NormalBorder.Width(w).Padding(1, 2).
			Render(padLines(lines, theme.BoxHeight))
	}

	// Header row
	lines = append(lines, fmt.Sprintf("  %s  %s  %s  %s",
		theme.Label.Render(fmt.Sprintf("%-6s", "Type")),
		theme.Label.Render(fmt.Sprintf("%-12s", "Amount")),
		theme.Label.Render(fmt.Sprintf("%-16s", "Memo")),
		theme.Label.Render("Date")))
	lines = append(lines, "  "+theme.Dim.Render(
		strings.Repeat("─", w-8)))

	// Account for border (2) + padding (2) + header (2) + separator (1)
	// + summary (3) + scroll indicators (2)
	visibleCount := theme.BoxHeight - 12
	if visibleCount < 3 {
		visibleCount = 3
	}

	viewStart := 0
	if m.payHistoryCursor >= visibleCount {
		viewStart = m.payHistoryCursor - visibleCount + 1
	}
	viewEnd := viewStart + visibleCount
	if viewEnd > len(m.payHistory) {
		viewEnd = len(m.payHistory)
	}

	if viewStart > 0 {
		lines = append(lines, "  "+theme.Dim.Render("  ↑ more"))
	}

	for i := viewStart; i < viewEnd; i++ {
		entry := m.payHistory[i]
		prefix := "  "
		style := theme.Value
		if m.payHistoryCursor == i && m.walletPaneFocused {
			prefix = "▸ "
			style = theme.Action
		}

		statusDot := theme.GreenDot.Render("●")
		if entry.Status == "FAILED" {
			statusDot = theme.RedDot.Render("●")
		} else if entry.Status == "EXPIRED" {
			statusDot = theme.Dim.Render("○")
		} else if entry.Status == "IN_FLIGHT" || entry.Status == "OPEN" {
			statusDot = theme.Dim.Render("◌")
		}

		direction := theme.Warning.Render("↑ sent")
		if entry.IsIncoming {
			direction = theme.Success.Render("↓ recv")
		}

		amount := formatSats(entry.AmountSats) + " sats"

		memo := entry.Memo
		if len(memo) > 16 {
			memo = memo[:16] + "..."
		}
		if memo == "" {
			memo = "—"
		}

		ts := formatTimestamp(entry.CreationDate)

		lines = append(lines, fmt.Sprintf("%s%s %s  %s  %s  %s",
			prefix, statusDot, direction,
			style.Render(fmt.Sprintf("%-12s", amount)),
			theme.Dim.Render(fmt.Sprintf("%-16s", memo)),
			theme.Dim.Render(ts)))
	}

	if viewEnd < len(m.payHistory) {
		lines = append(lines, "  "+theme.Dim.Render("  ↓ more"))
	}

	// Summary at bottom
	var totalRecv, totalSent int64
	for _, entry := range m.payHistory {
		if entry.IsIncoming && entry.Status == "SETTLED" {
			totalRecv += entry.AmountSats
		} else if !entry.IsIncoming && entry.Status == "SUCCEEDED" {
			totalSent += entry.AmountSats
		}
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %s %s    %s %s",
		theme.Label.Render("Received:"),
		theme.Success.Render(formatSats(totalRecv)+" sats"),
		theme.Label.Render("Sent:"),
		theme.Warning.Render(formatSats(totalSent)+" sats")))

	return theme.NormalBorder.Width(w).Padding(1, 2).
		Render(padLines(lines, theme.BoxHeight))
}

// ── Send Pane ────────────────────────────────────────────

func (m Model) walletSendPane(w int) string {
	var lines []string

	lines = append(lines, theme.Header.Render("Send Payment"))
	lines = append(lines, "")

	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		lines = append(lines, theme.Dim.Render(
			"Install LND and create wallet to send payments."))
		return theme.NormalBorder.Width(w).Padding(1, 2).
			Render(padLines(lines, theme.BoxHeight))
	}

	if m.status == nil || !m.status.lndResponding {
		lines = append(lines, theme.Dim.Render("Waiting for LND..."))
		return theme.NormalBorder.Width(w).Padding(1, 2).
			Render(padLines(lines, theme.BoxHeight))
	}

	// Balance context
	balance := "0"
	if m.status.lndBalance != "" {
		balance = m.status.lndBalance
	}
	var totalLocal int64
	if m.status != nil {
		for _, ch := range m.status.channels {
			totalLocal += ch.LocalBalance
		}
	}
	lines = append(lines, "  "+theme.Label.Render("On-chain: ")+
		theme.Value.Render(formatSats(parseBalance(balance))+" sats"))
	lines = append(lines, "  "+theme.Label.Render("Sendable: ")+
		theme.Value.Render(formatSats(totalLocal)+" sats"))
	lines = append(lines, "")

	lines = append(lines, "  "+theme.Label.Render("Payment Request:"))
	lines = append(lines, m.sendInput.View())
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"Paste a bolt11 Lightning invoice"))
	lines = append(lines, "  "+theme.Dim.Render(
		"(starts with lnbc or lntb)"))

	if m.sendError != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(m.sendError))
	}

	return theme.NormalBorder.Width(w).Padding(1, 2).
		Render(padLines(lines, theme.BoxHeight))
}

// ── Receive Pane ─────────────────────────────────────────

func (m Model) walletReceivePane(w int) string {
	var lines []string

	lines = append(lines, theme.Header.Render("Receive Payment"))
	lines = append(lines, "")

	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		lines = append(lines, theme.Dim.Render(
			"Install LND and create wallet to receive payments."))
		return theme.NormalBorder.Width(w).Padding(1, 2).
			Render(padLines(lines, theme.BoxHeight))
	}

	if m.status == nil || !m.status.lndResponding {
		lines = append(lines, theme.Dim.Render("Waiting for LND..."))
		return theme.NormalBorder.Width(w).Padding(1, 2).
			Render(padLines(lines, theme.BoxHeight))
	}

	lines = append(lines, "  "+theme.Label.Render("Amount (sats):"))
	lines = append(lines, m.recvAmountInput.View())
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Memo (optional):"))
	lines = append(lines, m.recvMemoInput.View())
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render("Tab to switch fields"))
	lines = append(lines, "  "+theme.Dim.Render(
		"Enter to create invoice"))

	if m.recvError != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(m.recvError))
	}

	return theme.NormalBorder.Width(w).Padding(1, 2).
		Render(padLines(lines, theme.BoxHeight))
}

// ── On-Chain Pane (Stub) ─────────────────────────────────

func (m Model) walletOnChainPane(w int) string {
	var lines []string

	lines = append(lines, theme.Header.Render("On-Chain"))
	lines = append(lines, "")
	lines = append(lines, theme.Dim.Render(
		"On-chain wallet management coming soon."))
	lines = append(lines, "")
	lines = append(lines, theme.Dim.Render(
		"Planned features:"))
	lines = append(lines, theme.Dim.Render(
		"  • UTXO list with labels"))
	lines = append(lines, theme.Dim.Render(
		"  • On-chain transaction history"))
	lines = append(lines, theme.Dim.Render(
		"  • Transaction diagrams (inputs/outputs)"))
	lines = append(lines, theme.Dim.Render(
		"  • On-chain sends"))

	return theme.NormalBorder.Width(w).Padding(1, 2).
		Render(padLines(lines, theme.BoxHeight))
}

// Ensure lipgloss import is used.
var _ = lipgloss.Center
