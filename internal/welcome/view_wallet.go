package welcome

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Wallet overview (Sparrow-style) ──────────────────────

func (m Model) walletOverview(w, h int) string {
	var lines []string

	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		lines = append(lines, theme.Dim.Render(
			" Install LND and create wallet."))
		lines = append(lines, "")
		lines = append(lines, m.walletButtons(w))
		return strings.Join(lines, "\n")
	}
	if m.status == nil || !m.status.lndResponding {
		lines = append(lines,
			theme.Dim.Render(" Waiting for LND..."))
		lines = append(lines, "")
		lines = append(lines, m.walletButtons(w))
		return strings.Join(lines, "\n")
	}

	// ── Action buttons row ───────────────────────
	lines = append(lines, "")
	lines = append(lines, m.walletButtons(w))
	lines = append(lines, "")

	// ── Table header ─────────────────────────────
	tableW := w - 2
	if tableW < 40 {
		tableW = 40
	}

	dateW := 18
	memoW := tableW - dateW - 14 - 14 - 3
	if memoW < 6 {
		memoW = 6
	}
	valW := 14
	balW := 14

	hdrStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Bold(true)
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	hdr := " " +
		hdrStyle.Render(
			fmt.Sprintf("%-*s", dateW, "Date")) +
		hdrStyle.Render(
			fmt.Sprintf("%-*s", memoW, "Memo")) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", valW, "Value")) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", balW, "Balance"))
	lines = append(lines, hdr)

	sep := " " + sepStyle.Render(
		strings.Repeat("─", tableW))
	lines = append(lines, sep)

	// ── Table rows ───────────────────────────────
	if len(m.payHistory) == 0 {
		lines = append(lines,
			" "+theme.Dim.Render("No payments yet."))
	} else {
		visH := h - 7
		if visH < 3 {
			visH = 3
		}

		// Calculate running balance
		// payHistory is newest-first, so we need to
		// compute balance from oldest up
		balances := make([]int64, len(m.payHistory))
		if len(m.payHistory) > 0 {
			// Start from an estimated current balance
			var runBal int64
			onchain := parseBalance(
				m.status.lndBalance)
			var totalLocal int64
			for _, ch := range m.status.channels {
				totalLocal += ch.LocalBalance
			}
			runBal = onchain + totalLocal

			// Walk newest→oldest to assign balances
			for i := 0; i < len(m.payHistory); i++ {
				balances[i] = runBal
				entry := m.payHistory[i]
				if entry.IsIncoming {
					runBal -= entry.AmountSats
				} else {
					runBal += entry.AmountSats +
						entry.FeeSats
				}
			}
		}

		// Scroll offset
		startIdx := 0
		sel := m.payHistoryCursor
		if sel >= startIdx+visH {
			startIdx = sel - visH + 1
		}
		endIdx := startIdx + visH
		if endIdx > len(m.payHistory) {
			endIdx = len(m.payHistory)
		}

		negStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
		posStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))
		selBg := lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

		for i := startIdx; i < endIdx; i++ {
			entry := m.payHistory[i]
			isSelected := i == m.payHistoryCursor &&
				m.contentFocused &&
				m.contentFocus == 0

			// Date
			date := formatTimestampTable(
				entry.CreationDate)
			dateStr := fmt.Sprintf("%-*s",
				dateW, date)

			// Memo
			memo := entry.Memo
			if memo == "" {
				memo = "—"
			}
			if len(memo) > memoW-1 {
				memo = memo[:memoW-2] + ".."
			}
			memoStr := fmt.Sprintf("%-*s",
				memoW, memo)

			// Value (signed)
			var valStr string
			if entry.IsIncoming {
				valStr = fmt.Sprintf("%*s",
					valW,
					formatSats(entry.AmountSats))
			} else {
				valStr = fmt.Sprintf("%*s",
					valW,
					"-"+formatSats(entry.AmountSats))
			}

			// Balance
			bal := balances[i]
			balStr := fmt.Sprintf("%*s",
				balW, formatSats(bal))

			marker := " "
			if isSelected {
				marker = "▸"
				row := marker +
					selBg.Render(dateStr) +
					selBg.Render(memoStr) +
					selBg.Render(valStr) +
					selBg.Render(balStr)
				lines = append(lines, row)
			} else {
				var valRendered string
				if entry.IsIncoming {
					valRendered =
						posStyle.Render(valStr)
				} else {
					valRendered =
						negStyle.Render(valStr)
				}
				row := marker +
					theme.Value.Render(dateStr) +
					theme.Dim.Render(memoStr) +
					valRendered +
					theme.Value.Render(balStr)
				lines = append(lines, row)
			}
		}

		if startIdx > 0 {
			lines = append(lines,
				" "+theme.Dim.Render("  ↑ more"))
		}
		if endIdx < len(m.payHistory) {
			lines = append(lines,
				" "+theme.Dim.Render("  ↓ more"))
		}
	}

	return strings.Join(lines, "\n")
}

func (m Model) walletButtons(w int) string {
	labels := []string{
		"Send", "Receive", "On-Chain", "Pairing",
	}

	var parts []string
	for i, label := range labels {
		isActive := m.contentFocused &&
			m.contentFocus == 1 &&
			m.btnIdx == i

		if isActive {
			parts = append(parts,
				"▸ "+btnFocused.Render(label))
		} else {
			parts = append(parts,
				"  "+btnNormal.Render(label))
		}
	}

	return " " + strings.Join(parts, "  ")
}

// ── Payment detail ───────────────────────────────────────

func (m Model) paymentDetailContent(w int) string {
	var lines []string

	if m.payHistoryCursor >= len(m.payHistory) {
		lines = append(lines,
			" "+theme.Warn.Render("Payment not found"))
		return strings.Join(lines, "\n")
	}

	entry := m.payHistory[m.payHistoryCursor]
	if entry.IsIncoming {
		lines = append(lines,
			theme.Success.Render(
				" ↓ Received Payment"))
	} else {
		lines = append(lines,
			theme.Warning.Render(" ↑ Sent Payment"))
	}
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Amount:  ")+
			theme.Value.Render(
				formatSats(entry.AmountSats)+" sats"))
	if entry.FeeSats > 0 {
		lines = append(lines,
			" "+theme.Label.Render("Fee:     ")+
				theme.Value.Render(
					formatSats(entry.FeeSats)+
						" sats"))
	}
	lines = append(lines,
		" "+theme.Label.Render("Status:  ")+
			theme.Value.Render(entry.Status))
	if entry.Memo != "" {
		lines = append(lines,
			" "+theme.Label.Render("Memo:    ")+
				theme.Value.Render(entry.Memo))
	}
	lines = append(lines,
		" "+theme.Label.Render("Date:    ")+
			theme.Value.Render(
				formatTimestampFull(
					entry.CreationDate)))
	if entry.Preimage != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Label.Render("Preimage:"))
		pre := entry.Preimage
		if len(pre) > w-4 {
			pre = pre[:w-7] + "..."
		}
		lines = append(lines,
			" "+theme.Mono.Render(pre))
	}
	if entry.PaymentHash != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Label.Render("Payment Hash:"))
		hash := entry.PaymentHash
		if len(hash) > w-4 {
			hash = hash[:w-7] + "..."
		}
		lines = append(lines,
			" "+theme.Mono.Render(hash))
	}

	if len(entry.Hops) > 0 {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Label.Render("Route:"))
		lines = append(lines,
			renderRouteDiagram(entry.Hops, w))
	}

	return strings.Join(lines, "\n")
}

// ── Send/Receive panes ───────────────────────────────────

func (m Model) walletSendPane(w int) string {
	var lines []string
	lines = append(lines,
		theme.Header.Render(" Send Payment"))
	lines = append(lines, "")

	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		lines = append(lines, theme.Dim.Render(
			" Install LND and create wallet to send."))
		return strings.Join(lines, "\n")
	}
	if m.status == nil || !m.status.lndResponding {
		lines = append(lines,
			theme.Dim.Render(" Waiting for LND..."))
		return strings.Join(lines, "\n")
	}

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
	lines = append(lines,
		" "+theme.Label.Render("On-chain: ")+
			theme.Value.Render(
				formatSats(parseBalance(balance))+
					" sats"))
	lines = append(lines,
		" "+theme.Label.Render("Sendable: ")+
			theme.Value.Render(
				formatSats(totalLocal)+" sats"))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Payment Request:"))
	lines = append(lines,
		"▸ "+m.sendInput.View())
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Paste a bolt11 invoice"))
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to decode  Esc to cancel"))

	if m.sendError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.sendError))
	}

	return strings.Join(lines, "\n")
}

func (m Model) walletSendConfirmPane(w int) string {
	var lines []string
	lines = append(lines,
		theme.Warning.Render(" Confirm Payment"))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Amount:      ")+
			theme.Value.Render(
				formatSats(m.sendDecodedAmt)+" sats"))
	if m.sendDecodedDesc != "" {
		lines = append(lines,
			" "+theme.Label.Render("Description: ")+
				theme.Value.Render(m.sendDecodedDesc))
	}
	lines = append(lines,
		" "+theme.Label.Render("Destination:"))
	dest := m.sendDecodedDest
	maxDest := w - 4
	if len(dest) > maxDest {
		dest = dest[:maxDest-3] + "..."
	}
	lines = append(lines, " "+theme.Mono.Render(dest))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Warning.Render(
		"Send "+formatSats(m.sendDecodedAmt)+" sats?"))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"y confirm  esc cancel"))

	if m.sendError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.sendError))
	}
	return strings.Join(lines, "\n")
}

func (m Model) walletSendInFlightPane(w int) string {
	var lines []string
	lines = append(lines,
		theme.Header.Render(" Sending Payment..."))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Value.Render(
		"Routing "+formatSats(m.sendDecodedAmt)+
			" sats"))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"May take up to 60 seconds over Tor."))
	return strings.Join(lines, "\n")
}

func (m Model) walletSendResultPane(w int) string {
	var lines []string

	if m.sendError != "" {
		lines = append(lines,
			theme.Warning.Render(" Payment Failed"))
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.sendError))
	} else {
		lines = append(lines,
			theme.Success.Render(" Payment Sent"))
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Label.Render("Amount: ")+
				theme.Value.Render(
					formatSats(m.sendDecodedAmt)+
						" sats"))
		lines = append(lines,
			" "+theme.Label.Render("Fee:    ")+
				theme.Value.Render(
					formatSats(m.sendFeeSats)+
						" sats"))
		if m.sendPreimage != "" {
			lines = append(lines, "")
			lines = append(lines,
				" "+theme.Label.Render("Preimage:"))
			pre := m.sendPreimage
			if len(pre) > w-4 {
				pre = pre[:w-7] + "..."
			}
			lines = append(lines,
				" "+theme.Mono.Render(pre))
		}
		if len(m.sendRouteHops) > 0 {
			lines = append(lines, "")
			lines = append(lines,
				" "+theme.Label.Render("Route:"))
			lines = append(lines,
				renderRouteDiagram(
					m.sendRouteHops, w))
		}
	}

	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to return"))
	return strings.Join(lines, "\n")
}

func (m Model) walletReceivePane(w int) string {
	var lines []string
	lines = append(lines,
		theme.Header.Render(" Receive Payment"))
	lines = append(lines, "")

	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		lines = append(lines, theme.Dim.Render(
			" Install LND and create wallet."))
		return strings.Join(lines, "\n")
	}
	if m.status == nil || !m.status.lndResponding {
		lines = append(lines,
			theme.Dim.Render(" Waiting for LND..."))
		return strings.Join(lines, "\n")
	}

	amtMarker := " "
	memoMarker := " "
	if m.recvAmountInput.Focused() {
		amtMarker = "▸"
	}
	if m.recvMemoInput.Focused() {
		memoMarker = "▸"
	}

	lines = append(lines,
		" "+theme.Label.Render("Amount (sats):"))
	lines = append(lines,
		amtMarker+" "+m.recvAmountInput.View())
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Memo (optional):"))
	lines = append(lines,
		memoMarker+" "+m.recvMemoInput.View())
	lines = append(lines,
		" "+m.recvMemoInput.View())
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Tab to switch fields"))
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to create invoice  Esc to cancel"))

	if m.recvError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.recvError))
	}
	return strings.Join(lines, "\n")
}

func (m Model) walletReceiveWaitingPane(w int) string {
	var lines []string
	lines = append(lines,
		theme.Header.Render(" Waiting for Payment"))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Amount: ")+
			theme.Value.Render(
				formatSats(m.recvAmountSats)+" sats"))
	lines = append(lines, "")

	if m.recvPayReq != "" {
		lines = append(lines,
			" "+theme.Label.Render("Invoice:"))
		display := m.recvPayReq
		if len(display) > w-4 {
			display = display[:w-7] + "..."
		}
		lines = append(lines,
			" "+theme.Mono.Render(display))
		lines = append(lines, "")

		qrBtn := " Show QR "
		copyBtn := " Full Invoice "
		if m.recvButtonIdx == 0 {
			qrBtn = theme.ActiveTab.Render(qrBtn)
			copyBtn = theme.InactiveTab.Render(copyBtn)
		} else {
			qrBtn = theme.InactiveTab.Render(qrBtn)
			copyBtn = theme.ActiveTab.Render(copyBtn)
		}
		lines = append(lines, " "+qrBtn+"  "+copyBtn)
	}

	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Waiting for payment..."))
	lines = append(lines, " "+theme.Dim.Render(
		"esc cancel"))
	return strings.Join(lines, "\n")
}

func (m Model) walletReceivePaidPane(w int) string {
	var lines []string
	lines = append(lines,
		theme.Success.Render(" Payment Received"))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Amount: ")+
			theme.Value.Render(
				formatSats(m.recvAmountSats)+" sats"))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to return"))
	return strings.Join(lines, "\n")
}

func (m Model) walletReceiveExpiredPane(w int) string {
	var lines []string
	lines = append(lines,
		theme.Warning.Render(" Invoice Expired"))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Create a new invoice to try again."))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to return"))
	return strings.Join(lines, "\n")
}

func (m Model) walletOnChainContent(w int) string {
	var lines []string
	lines = append(lines,
		theme.Header.Render(" On-Chain Wallet"))
	lines = append(lines, "")

	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		lines = append(lines, theme.Dim.Render(
			" Install LND and create wallet."))
		return strings.Join(lines, "\n")
	}
	if m.status == nil || !m.status.lndResponding {
		lines = append(lines,
			theme.Dim.Render(" Waiting for LND..."))
		return strings.Join(lines, "\n")
	}

	// On-chain result screen
	if m.subview == svOnChainResult {
		return m.onChainResultContent(w)
	}

	// Balance
	onchain := "0"
	if m.status.lndBalance != "" {
		onchain = m.status.lndBalance
	}
	lines = append(lines,
		" "+theme.Label.Render("Balance:  ")+
			theme.Value.Render(
				formatSats(parseBalance(onchain))+
					" sats"))
	lines = append(lines, "")

	// Address
	if m.onChainAddress != "" {
		lines = append(lines,
			" "+theme.Label.Render("Address:"))
		addr := m.onChainAddress
		if len(addr) > w-4 {
			addr = addr[:w-7] + "..."
		}
		lines = append(lines,
			" "+theme.Mono.Render(addr))
	}
	lines = append(lines, "")

	// Buttons
	btnLabels := []string{"New Address", "Refresh UTXOs"}
	var btnParts []string
	for i, label := range btnLabels {
		isActive := m.onChainFocus == 0 &&
			m.onChainBtnIdx == i
		if isActive {
			btnParts = append(btnParts,
				"▸ "+btnFocused.Render(label))
		} else {
			btnParts = append(btnParts,
				"  "+btnNormal.Render(label))
		}
	}
	lines = append(lines,
		" "+strings.Join(btnParts, "  "))
	lines = append(lines, "")

	// UTXO table
	lines = append(lines,
		" "+theme.Header.Render("UTXOs"))
	lines = append(lines, "")

	if len(m.utxos) == 0 {
		lines = append(lines,
			" "+theme.Dim.Render(
				"No UTXOs. Press Refresh to load."))
	} else {
		// Table header
		hdrStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Bold(true)
		sepStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

		txidW := 20
		amtW := 14
		confW := 8
		addrW := w - txidW - amtW - confW - 6
		if addrW < 10 {
			addrW = 10
		}

		hdr := " " +
			hdrStyle.Render(
				fmt.Sprintf("%-*s", txidW, "Txid")) +
			hdrStyle.Render(
				fmt.Sprintf("%*s", amtW, "Amount")) +
			hdrStyle.Render(
				fmt.Sprintf("%*s", confW, "Confs")) +
			hdrStyle.Render(
				fmt.Sprintf("  %-*s", addrW, "Address"))
		lines = append(lines, hdr)
		lines = append(lines,
			" "+sepStyle.Render(
				strings.Repeat("─", w-2)))

		for i, u := range m.utxos {
			isSelected := m.onChainFocus == 1 &&
				m.utxoCursor == i

			txid := u.Txid
			if len(txid) > txidW-3 {
				txid = txid[:txidW-3] + "..."
			}
			txidStr := fmt.Sprintf("%-*s", txidW, txid)

			amtStr := fmt.Sprintf("%*s", amtW,
				formatSats(u.AmountSats))

			confStr := fmt.Sprintf("%*d", confW,
				u.Confirmations)

			addr := u.Address
			if len(addr) > addrW {
				addr = addr[:addrW-3] + "..."
			}
			addrStr := fmt.Sprintf("  %-*s",
				addrW, addr)

			marker := " "
			if isSelected {
				marker = "▸"
				selStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("220")).
					Bold(true)
				lines = append(lines,
					marker+
						selStyle.Render(txidStr)+
						selStyle.Render(amtStr)+
						selStyle.Render(confStr)+
						selStyle.Render(addrStr))
			} else {
				lines = append(lines,
					marker+
						theme.Mono.Render(txidStr)+
						theme.Value.Render(amtStr)+
						theme.Dim.Render(confStr)+
						theme.Dim.Render(addrStr))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (m Model) onChainResultContent(w int) string {
	var lines []string

	if m.onChainSendError != "" {
		lines = append(lines,
			theme.Warning.Render(
				" On-Chain Send Failed"))
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(
				m.onChainSendError))
	} else {
		lines = append(lines,
			theme.Success.Render(
				" Transaction Broadcast"))
		lines = append(lines, "")
		if m.onChainSendTxid != "" {
			lines = append(lines,
				" "+theme.Label.Render("TX ID:"))
			txid := m.onChainSendTxid
			if len(txid) > w-4 {
				txid = txid[:w-7] + "..."
			}
			lines = append(lines,
				" "+theme.Mono.Render(txid))
		}
	}

	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to return"))

	return strings.Join(lines, "\n")
}

// ── Route diagram ────────────────────────────────────────

func renderRouteDiagram(
	hops []lndrpc.RouteHop, w int,
) string {
	if len(hops) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, " You")
	lines = append(lines, "  │")
	for i, hop := range hops {
		name := hop.Alias
		if name == "" {
			if len(hop.PubKey) > 12 {
				name = hop.PubKey[:12] + "..."
			} else {
				name = hop.PubKey
			}
		}
		if len(name) > 16 {
			name = name[:16]
		}
		feeStr := ""
		if hop.FeeSats > 0 {
			feeStr = fmt.Sprintf(" (fee: %s)",
				formatSats(hop.FeeSats))
		}
		if i < len(hops)-1 {
			lines = append(lines,
				fmt.Sprintf("  ├── %s%s",
					theme.Value.Render(name),
					theme.Dim.Render(feeStr)))
			lines = append(lines, "  │")
		} else {
			lines = append(lines,
				fmt.Sprintf("  └── %s%s",
					theme.Success.Render(name),
					theme.Dim.Render(feeStr)))
		}
	}
	return strings.Join(lines, "\n")
}

// ── Timestamp helpers ────────────────────────────────────

func formatTimestamp(unix int64) string {
	if unix == 0 {
		return "—"
	}
	t := time.Unix(unix, 0)
	diff := time.Since(t)
	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm ago",
			int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh ago",
			int(diff.Hours()))
	}
	if diff < 7*24*time.Hour {
		return fmt.Sprintf("%dd ago",
			int(diff.Hours()/24))
	}
	return t.Format("Jan 2")
}

func formatTimestampFull(unix int64) string {
	if unix == 0 {
		return "—"
	}
	return time.Unix(unix, 0).
		Format("2006-01-02 15:04:05")
}

// formatTimestampTable returns a Sparrow-style date
func formatTimestampTable(unix int64) string {
	if unix == 0 {
		return "—"
	}
	return time.Unix(unix, 0).
		Format("2006-01-02 15:04")
}
