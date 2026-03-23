package welcome

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Wallet overview ──────────────────────────────────────

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

	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Header.Render("Off-Chain Wallet"), w))
	lines = append(lines, "")
	balSummary := m.renderBalanceSummary(w)
	lines = append(lines, balSummary...)
	lines = append(lines, "")
	lines = append(lines, "")
	lines = append(lines, m.walletButtons(w))
	lines = append(lines, "")
	lines = append(lines, "")

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
	lines = append(lines,
		" "+sepStyle.Render(
			strings.Repeat("─", tableW)))

	if len(m.payHistory) == 0 {
		lines = append(lines,
			" "+theme.Dim.Render("No payments yet."))
	} else {
		usedLines := len(lines)
		visH := h - usedLines - 1
		if visH < 3 {
			visH = 3
		}

		balances := make([]int64, len(m.payHistory))
		var runBal int64
		var totalLocal int64
		for _, ch := range m.status.channels {
			totalLocal += ch.LocalBalance
		}
		runBal = totalLocal
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

			date := formatTimestampTable(
				entry.CreationDate)
			dateStr := fmt.Sprintf("%-*s",
				dateW, date)
			memo := entry.Memo
			if memo == "" {
				memo = "—"
			}
			if len(memo) > memoW-1 {
				memo = memo[:memoW-2] + ".."
			}
			memoStr := fmt.Sprintf("%-*s",
				memoW, memo)

			var valStr string
			if entry.IsIncoming {
				valStr = fmt.Sprintf("%*s", valW,
					formatSats(entry.AmountSats))
			} else {
				valStr = fmt.Sprintf("%*s", valW,
					"-"+formatSats(entry.AmountSats))
			}

			bal := balances[i]
			balStr := fmt.Sprintf("%*s",
				balW, formatSats(bal))

			marker := " "
			if isSelected {
				marker = "▸"
				lines = append(lines, marker+
					selBg.Render(dateStr)+
					selBg.Render(memoStr)+
					selBg.Render(valStr)+
					selBg.Render(balStr))
			} else {
				var valRendered string
				if entry.IsIncoming {
					valRendered =
						posStyle.Render(valStr)
				} else {
					valRendered =
						negStyle.Render(valStr)
				}
				lines = append(lines, marker+
					theme.Value.Render(dateStr)+
					theme.Dim.Render(memoStr)+
					valRendered+
					theme.Value.Render(balStr))
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
	labels := []string{"Send", "Receive", "Pairing"}

	isFocused := m.contentFocused &&
		!m.tabFocused &&
		m.contentFocus == 1

	btnW := w - 2
	if btnW < 20 {
		btnW = 20
	}
	numBtns := len(labels)
	totalGap := (numBtns - 1) * 2
	perBtn := (btnW - totalGap) / numBtns
	if perBtn < 8 {
		perBtn = 8
	}

	var parts []string
	for i, label := range labels {
		isActive := isFocused && m.btnIdx == i
		if isActive {
			parts = append(parts,
				theme.BtnFocused.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		} else {
			parts = append(parts,
				theme.BtnNormal.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
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
		lines = append(lines, "")
		lines = append(lines,
			centerPad(theme.Success.Render(
				"↓ Received Payment"), w))
	} else {
		lines = append(lines, "")
		lines = append(lines,
			centerPad(theme.Warning.Render(
				"↑ Sent Payment"), w))
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

// ── Send panes ───────────────────────────────────────────

func (m Model) walletSendPane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Header.Render("⚡ Send Payment"), w))
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

	var totalLocal int64
	if m.status != nil {
		for _, ch := range m.status.channels {
			totalLocal += ch.LocalBalance
		}
	}
	lines = append(lines,
		" "+theme.Label.Render("Spendable: ")+
			theme.Value.Render(
				formatSats(totalLocal)+" sats"))
	lines = append(lines, "")

	labelStyle := theme.Label
	markerStyle := theme.Dim
	if m.contentFocused {
		labelStyle = navActiveStyle
		markerStyle = navActiveStyle
	}
	lines = append(lines,
		" "+labelStyle.Render("Payment Request:"))
	lines = append(lines,
		markerStyle.Render("▸")+" "+
			m.sendInput.View())
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Paste a bolt11 invoice"))
	lines = append(lines, " "+theme.Dim.Render(
		"←→ cursor  Enter decode  ⌫ cancel"))

	if m.sendError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.sendError))
	}
	return strings.Join(lines, "\n")
}

func (m Model) walletSendConfirmPane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Warning.Render(
				"Confirm Payment"), w))
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
		"y confirm  backspace cancel"))

	if m.sendError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.sendError))
	}
	return strings.Join(lines, "\n")
}

func (m Model) walletSendInFlightPane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Header.Render(
				"Sending Payment..."), w))
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
		lines = append(lines, "")
		lines = append(lines,
			centerPad(
				theme.Warning.Render(
					"Payment Failed"), w))
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.sendError))
	} else {
		lines = append(lines, "")
		lines = append(lines,
			centerPad(
				theme.Success.Render(
					"Payment Sent"), w))
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

// ── Receive panes ────────────────────────────────────────

func (m Model) walletReceivePane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Header.Render(
				"⚡ Receive Payment"), w))
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

	amtFocused := m.recvAmountInput.Focused() &&
		m.contentFocused
	memoFocused := m.recvMemoInput.Focused() &&
		m.contentFocused

	amtMarker := " "
	amtLabelStyle := theme.Label
	memoMarker := " "
	memoLabelStyle := theme.Label
	if amtFocused {
		amtMarker = navActiveStyle.Render("▸")
		amtLabelStyle = navActiveStyle
	}
	if memoFocused {
		memoMarker = navActiveStyle.Render("▸")
		memoLabelStyle = navActiveStyle
	}

	lines = append(lines,
		" "+amtLabelStyle.Render("Amount (sats):"))
	lines = append(lines,
		amtMarker+" "+m.recvAmountInput.View())
	lines = append(lines, "")
	lines = append(lines,
		" "+memoLabelStyle.Render("Memo (optional):"))
	lines = append(lines,
		memoMarker+" "+m.recvMemoInput.View())
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"↑↓ switch fields"))
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to create invoice  "+
			"Backspace to cancel"))

	if m.recvError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.recvError))
	}
	return strings.Join(lines, "\n")
}

func (m Model) walletReceiveWaitingPane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Header.Render(
				"Waiting for Payment"), w))
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

		btnFocused := m.contentFocused &&
			!m.tabFocused
		btnLabels := []string{
			"Show QR", "Full Invoice",
		}
		btnW := w - 2
		if btnW < 20 {
			btnW = 20
		}
		numBtns := len(btnLabels)
		totalGap := (numBtns - 1) * 2
		perBtn := (btnW - totalGap) / numBtns
		if perBtn < 10 {
			perBtn = 10
		}

		var btnParts []string
		for i, label := range btnLabels {
			isActive := btnFocused &&
				m.recvButtonIdx == i
			if isActive {
				btnParts = append(btnParts,
					theme.BtnFocused.
						Width(perBtn).
						AlignHorizontal(
							lipgloss.Center).
						Render(label))
			} else {
				btnParts = append(btnParts,
					theme.BtnNormal.
						Width(perBtn).
						AlignHorizontal(
							lipgloss.Center).
						Render(label))
			}
		}
		lines = append(lines,
			" "+strings.Join(btnParts, "  "))
	}

	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Waiting for payment..."))
	lines = append(lines, " "+theme.Dim.Render(
		"backspace cancel"))
	return strings.Join(lines, "\n")
}

func (m Model) walletReceivePaidPane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Success.Render(
				"Payment Received"), w))
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
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Warning.Render(
				"Invoice Expired"), w))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Create a new invoice to try again."))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to return"))
	return strings.Join(lines, "\n")
}

// ── On-Chain overview ────────────────────────────────────

func (m Model) onChainOverview(w, h int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Header.Render("On-Chain Wallet"), w))
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

	btnFocused := m.contentFocused && !m.tabFocused
	btnLabels := []string{
		"Receive", "Send", "Refresh",
	}
	btnW := w - 2
	if btnW < 20 {
		btnW = 20
	}
	numBtns := len(btnLabels)
	totalGap := (numBtns - 1) * 2
	perBtn := (btnW - totalGap) / numBtns
	if perBtn < 8 {
		perBtn = 8
	}

	var btnParts []string
	for i, label := range btnLabels {
		isActive := btnFocused &&
			m.onChainTxFocus == 0 &&
			m.onChainBtnIdx == i
		if isActive {
			btnParts = append(btnParts,
				theme.BtnFocused.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		} else {
			btnParts = append(btnParts,
				theme.BtnNormal.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		}
	}
	lines = append(lines,
		" "+strings.Join(btnParts, "  "))
	lines = append(lines, "")

	lines = append(lines,
		" "+theme.Header.Render("Transactions"))
	lines = append(lines, "")

	if len(m.onChainTxs) == 0 {
		lines = append(lines,
			" "+theme.Dim.Render(
				"No on-chain transactions."))
	} else {
		hdrStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Bold(true)
		sepStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

		dateW := 12
		typeW := 16
		amtW := 14
		confW := w - dateW - typeW - amtW - 5
		if confW < 6 {
			confW = 6
		}

		hdr := " " +
			hdrStyle.Render(pad("Date", dateW)) +
			hdrStyle.Render(pad("Type", typeW)) +
			hdrStyle.Render(
				fmt.Sprintf("%*s", amtW, "Amount")) +
			hdrStyle.Render(
				fmt.Sprintf("%*s", confW, "Confs"))
		lines = append(lines, hdr)
		lines = append(lines,
			" "+sepStyle.Render(
				strings.Repeat("─", w-2)))

		maxRows := 8
		if len(m.onChainTxs) < maxRows {
			maxRows = len(m.onChainTxs)
		}
		startIdx := 0
		if m.onChainTxFocus == 1 &&
			m.onChainTxCursor >= maxRows {
			startIdx = m.onChainTxCursor -
				maxRows + 1
		}
		endIdx := startIdx + maxRows
		if endIdx > len(m.onChainTxs) {
			endIdx = len(m.onChainTxs)
		}

		for i := startIdx; i < endIdx; i++ {
			tx := m.onChainTxs[i]
			isSelected := btnFocused &&
				m.onChainTxFocus == 1 &&
				m.onChainTxCursor == i

			date := formatTimestamp(tx.Timestamp)
			dateStr := pad(date, dateW)
			txType := tx.Label
			if len(txType) > typeW-1 {
				txType = txType[:typeW-2] + ".."
			}
			typeStr := pad(txType, typeW)

			var amtStr string
			if tx.Amount >= 0 {
				amtStr = fmt.Sprintf("%*s", amtW,
					"+"+formatSats(tx.Amount))
			} else {
				amtStr = fmt.Sprintf("%*s", amtW,
					formatSats(tx.Amount))
			}
			confStr := fmt.Sprintf("%*d",
				confW, tx.Confirmations)
			if tx.Confirmations == 0 {
				confStr = fmt.Sprintf(
					"%*s", confW, "pending")
			}

			marker := " "
			if isSelected {
				marker = "▸"
				selStyle := lipgloss.NewStyle().
					Foreground(
						lipgloss.Color("220")).
					Bold(true)
				lines = append(lines, marker+
					selStyle.Render(dateStr)+
					selStyle.Render(typeStr)+
					selStyle.Render(amtStr)+
					selStyle.Render(confStr))
			} else {
				amtStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("15"))
				if tx.Amount < 0 {
					amtStyle = lipgloss.NewStyle().
						Foreground(
							lipgloss.Color("196"))
				}
				lines = append(lines, marker+
					theme.Dim.Render(dateStr)+
					theme.Value.Render(typeStr)+
					amtStyle.Render(amtStr)+
					theme.Dim.Render(confStr))
			}
		}

		if startIdx > 0 {
			lines = append(lines,
				" "+theme.Dim.Render("  ↑ more"))
		}
		if endIdx < len(m.onChainTxs) {
			lines = append(lines,
				" "+theme.Dim.Render("  ↓ more"))
		}
	}

	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Header.Render("UTXOs"))
	lines = append(lines, "")

	if len(m.utxos) == 0 {
		lines = append(lines,
			" "+theme.Dim.Render("No UTXOs found."))
	} else {
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
			hdrStyle.Render(pad("Txid", txidW)) +
			hdrStyle.Render(
				fmt.Sprintf("%*s", amtW, "Amount")) +
			hdrStyle.Render(
				fmt.Sprintf("%*s", confW, "Confs")) +
			hdrStyle.Render(
				fmt.Sprintf("  %-*s", addrW,
					"Address"))
		lines = append(lines, hdr)
		lines = append(lines,
			" "+sepStyle.Render(
				strings.Repeat("─", w-2)))

		for i, u := range m.utxos {
			isSelected := btnFocused &&
				m.onChainTxFocus == 2 &&
				m.utxoCursor == i

			txid := u.Txid
			if len(txid) > txidW-3 {
				txid = txid[:txidW-3] + "..."
			}
			txidStr := pad(txid, txidW)
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
					Foreground(
						lipgloss.Color("220")).
					Bold(true)
				lines = append(lines, marker+
					selStyle.Render(txidStr)+
					selStyle.Render(amtStr)+
					selStyle.Render(confStr)+
					selStyle.Render(addrStr))
			} else {
				lines = append(lines, marker+
					theme.Mono.Render(txidStr)+
					theme.Value.Render(amtStr)+
					theme.Dim.Render(confStr)+
					theme.Dim.Render(addrStr))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// ── On-Chain Receive pane ────────────────────────────────

func (m Model) onChainReceivePane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Header.Render(
				"⛓ Receive On-Chain"), w))
	lines = append(lines, "")

	if m.ocRecvAddress == "" {
		lines = append(lines,
			" "+theme.Dim.Render(
				"Generating address..."))
		return strings.Join(lines, "\n")
	}

	lines = append(lines,
		" "+theme.Label.Render("Address:"))
	addr := m.ocRecvAddress
	if len(addr) > w-4 {
		addr = addr[:w-7] + "..."
	}
	lines = append(lines,
		" "+theme.Mono.Render(addr))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Send Bitcoin to this address."))
	lines = append(lines, " "+theme.Dim.Render(
		"Funds appear after 1 confirmation."))
	lines = append(lines, "")

	btnFocused := m.contentFocused && !m.tabFocused
	btnLabels := []string{"Show QR", "New Address"}
	btnW := w - 2
	if btnW < 20 {
		btnW = 20
	}
	numBtns := len(btnLabels)
	totalGap := (numBtns - 1) * 2
	perBtn := (btnW - totalGap) / numBtns
	if perBtn < 10 {
		perBtn = 10
	}

	var btnParts []string
	for i, label := range btnLabels {
		isActive := btnFocused &&
			m.ocRecvBtnIdx == i
		if isActive {
			btnParts = append(btnParts,
				theme.BtnFocused.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		} else {
			btnParts = append(btnParts,
				theme.BtnNormal.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		}
	}
	lines = append(lines,
		" "+strings.Join(btnParts, "  "))

	if m.ocRecvError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(m.ocRecvError))
	}

	return strings.Join(lines, "\n")
}

// ── On-Chain send panes ──────────────────────────────────

func (m Model) onChainSendAddrPane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Header.Render(
				"⛓ Send On-Chain"), w))
	lines = append(lines, "")

	onchain := "0"
	if m.status != nil && m.status.lndBalance != "" {
		onchain = m.status.lndBalance
	}
	lines = append(lines,
		" "+theme.Label.Render("Balance:  ")+
			theme.Value.Render(
				formatSats(parseBalance(onchain))+
					" sats"))
	lines = append(lines, "")

	labelStyle := theme.Label
	markerStyle := theme.Dim
	if m.contentFocused && !m.tabFocused {
		labelStyle = navActiveStyle
		markerStyle = navActiveStyle
	}
	lines = append(lines,
		" "+labelStyle.Render("Destination Address:"))
	lines = append(lines,
		markerStyle.Render("▸")+" "+
			m.ocSendAddrInput.View())
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to continue  Backspace to cancel"))

	if m.onChainSendError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(
				m.onChainSendError))
	}
	return strings.Join(lines, "\n")
}

func (m Model) onChainSendAmountPane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Header.Render(
				"⛓ Send On-Chain"), w))
	lines = append(lines, "")

	isFocused := m.contentFocused && !m.tabFocused
	addr := m.ocSendAddrVal
	if len(addr) > w-14 {
		addr = addr[:w-17] + "..."
	}
	lines = append(lines,
		" "+theme.Label.Render("To: ")+
			theme.Mono.Render(addr))
	lines = append(lines, "")

	amtActive := isFocused && m.ocSendStep == 0
	amtMarker := " "
	amtLabelStyle := theme.Label
	if amtActive {
		amtMarker = navActiveStyle.Render("▸")
		amtLabelStyle = navActiveStyle
	}
	if m.ocSendAll {
		lines = append(lines,
			" "+amtLabelStyle.Render("Amount:"))
		lines = append(lines,
			amtMarker+" "+theme.Action.Render(
				"[Send All]"))
	} else {
		lines = append(lines,
			" "+amtLabelStyle.Render(
				"Amount (sats):"))
		lines = append(lines,
			amtMarker+" "+m.ocSendAmtInput.View())
	}
	lines = append(lines, " "+theme.Dim.Render(
		"Tab to toggle Send All"))
	lines = append(lines, "")

	feeActive := isFocused && m.ocSendStep == 1
	feeLabelStyle := theme.Label
	if feeActive {
		feeLabelStyle = navActiveStyle
	}
	lines = append(lines,
		" "+feeLabelStyle.Render("Fee Rate:"))

	anyTier := false
	for _, t := range m.ocFeeTiers {
		if t.SatPerVB > 0 {
			anyTier = true
			break
		}
	}
	if !anyTier {
		lines = append(lines,
			" "+theme.Dim.Render(
				"Loading fee estimates..."))
	} else {
		tierLine := " "
		for i, t := range m.ocFeeTiers {
			isSelected := isFocused &&
				m.ocSendStep == 1 &&
				m.ocSelectedTier == i
			var label string
			if t.SatPerVB > 0 {
				label = fmt.Sprintf("%s %.0f",
					t.Label, t.SatPerVB)
			} else {
				label = t.Label + " n/a"
			}
			if isSelected {
				tierLine += "▸ " +
					theme.BtnFocused.Render(label) +
					"  "
			} else {
				tierLine += "  " +
					theme.BtnNormal.Render(label) +
					"  "
			}
		}
		customLabel := "Custom"
		isCustom := isFocused &&
			m.ocSendStep == 1 &&
			m.ocSelectedTier == 4
		if isCustom {
			tierLine += "▸ " +
				theme.BtnFocused.Render(customLabel)
		} else {
			tierLine += "  " +
				theme.BtnNormal.Render(customLabel)
		}
		lines = append(lines, tierLine)

		if m.ocSelectedTier == 4 {
			lines = append(lines, "")
			custActive := isFocused &&
				m.ocSendStep == 2
			custMarker := " "
			custLabelStyle := theme.Label
			if custActive {
				custMarker =
					navActiveStyle.Render("▸")
				custLabelStyle = navActiveStyle
			}
			lines = append(lines,
				" "+custLabelStyle.Render("sat/vB:"))
			lines = append(lines,
				custMarker+" "+
					m.ocCustomFeeInput.View())
		}
	}

	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"↑↓ fields  ←→ fee tier  "+
			"Enter continue  Backspace back"))

	if m.onChainSendError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(
				m.onChainSendError))
	}
	return strings.Join(lines, "\n")
}

func (m Model) onChainSendConfirmPane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Warning.Render(
				"Confirm On-Chain Send"), w))
	lines = append(lines, "")

	addr := m.ocSendAddrVal
	if len(addr) > w-14 {
		addr = addr[:w-17] + "..."
	}
	lines = append(lines,
		" "+theme.Label.Render("To:       ")+
			theme.Mono.Render(addr))
	if m.ocSendAll {
		lines = append(lines,
			" "+theme.Label.Render("Amount:   ")+
				theme.Value.Render("Send All"))
	} else {
		lines = append(lines,
			" "+theme.Label.Render("Amount:   ")+
				theme.Value.Render(
					formatSats(m.ocSendAmtVal)+
						" sats"))
	}
	lines = append(lines,
		" "+theme.Label.Render("Fee Rate: ")+
			theme.Value.Render(
				fmt.Sprintf("%d sat/vB",
					m.ocSendFeeRate)))
	if m.ocSelectedTier < 4 {
		tier := m.ocFeeTiers[m.ocSelectedTier]
		lines = append(lines,
			" "+theme.Label.Render("Target:   ")+
				theme.Value.Render(tier.Label))
	}
	if m.ocConfirmFee > 0 {
		lines = append(lines,
			" "+theme.Label.Render("Est. Fee: ")+
				theme.Value.Render(
					formatSats(m.ocConfirmFee)+
						" sats"))
		if !m.ocSendAll && m.ocSendAmtVal > 0 {
			total := m.ocSendAmtVal + m.ocConfirmFee
			lines = append(lines,
				" "+theme.Label.Render("Total:    ")+
					theme.Value.Render(
						formatSats(total)+
							" sats"))
		}
	}
	lines = append(lines, "")
	if m.ocSendAll {
		lines = append(lines, " "+theme.Warning.Render(
			"Send entire balance?"))
	} else {
		lines = append(lines, " "+theme.Warning.Render(
			"Send "+formatSats(m.ocSendAmtVal)+
				" sats?"))
	}
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"y confirm  backspace cancel"))

	if m.onChainSendError != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(
				m.onChainSendError))
	}
	return strings.Join(lines, "\n")
}

func (m Model) onChainSendBroadcastPane(w int) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines,
		centerPad(
			theme.Header.Render(
				"Broadcasting..."), w))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Value.Render(
		"Sending transaction to the network."))
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Do not close the terminal."))
	return strings.Join(lines, "\n")
}

func (m Model) onChainResultContent(w int) string {
	var lines []string

	if m.onChainSendError != "" {
		lines = append(lines, "")
		lines = append(lines,
			centerPad(
				theme.Warning.Render(
					"On-Chain Send Failed"), w))
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render(
				m.onChainSendError))
	} else {
		lines = append(lines, "")
		lines = append(lines,
			centerPad(
				theme.Success.Render(
					"Transaction Broadcast"), w))
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

func (m Model) onChainTxDetailPane(
	tx lndrpc.OnChainTx, w int,
) string {
	var lines []string

	switch tx.TxType {
	case "channel_open":
		lines = append(lines, "")
		lines = append(lines,
			centerPad(theme.Header.Render(
				"⚡ Channel Open"), w))
	case "channel_close":
		lines = append(lines, "")
		lines = append(lines,
			centerPad(theme.Warning.Render(
				"⚡ Channel Close"), w))
	case "send":
		lines = append(lines, "")
		lines = append(lines,
			centerPad(theme.Warning.Render(
				"↑ On-Chain Send"), w))
	default:
		lines = append(lines, "")
		lines = append(lines,
			centerPad(theme.Success.Render(
				"↓ On-Chain Receive"), w))
	}
	lines = append(lines, "")

	if tx.ChannelPeer != "" {
		lines = append(lines,
			" "+theme.Label.Render("Peer:    ")+
				theme.Value.Render(tx.ChannelPeer))
	}
	absAmt := tx.Amount
	if absAmt < 0 {
		absAmt = -absAmt
	}
	lines = append(lines,
		" "+theme.Label.Render("Amount:  ")+
			theme.Value.Render(
				formatSats(absAmt)+" sats"))
	if tx.Fee > 0 {
		lines = append(lines,
			" "+theme.Label.Render("Fee:     ")+
				theme.Value.Render(
					formatSats(tx.Fee)+" sats"))
	}
	confStr := fmt.Sprintf("%d", tx.Confirmations)
	if tx.Confirmations == 0 {
		confStr = "pending"
	}
	lines = append(lines,
		" "+theme.Label.Render("Confs:   ")+
			theme.Value.Render(confStr))
	if tx.BlockHeight > 0 {
		lines = append(lines,
			" "+theme.Label.Render("Block:   ")+
				theme.Value.Render(
					fmt.Sprintf("%d",
						tx.BlockHeight)))
	}
	lines = append(lines,
		" "+theme.Label.Render("Date:    ")+
			theme.Value.Render(
				formatTimestampFull(tx.Timestamp)))

	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("TX ID:"))
	txid := tx.Txid
	if len(txid) > w-4 {
		txid = txid[:w-7] + "..."
	}
	lines = append(lines,
		" "+theme.Mono.Render(txid))

	if len(tx.Outputs) > 0 {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Label.Render("Outputs"))
		for i, out := range tx.Outputs {
			addr := out.Address
			if len(addr) > w-26 {
				addr = addr[:w-29] + "..."
			}
			amtStr := formatSats(out.Amount)
			if out.Amount == 0 {
				amtStr = "—"
			}
			labelStr := ""
			if out.Label != "" {
				labelStr = " (" + out.Label + ")"
			}
			isLast := i == len(tx.Outputs)-1
			connector := "├──"
			if isLast {
				connector = "└──"
			}
			addrStyle := theme.Mono
			if out.Label == "destination" ||
				out.Label == "channel" {
				addrStyle = theme.Value
			}
			line := fmt.Sprintf("  %s %s  %s%s",
				connector,
				addrStyle.Render(addr),
				theme.Value.Render(amtStr+" sats"),
				theme.Dim.Render(labelStr))
			lines = append(lines, line)
			if !isLast {
				lines = append(lines, "  │")
			}
		}
		if tx.Fee > 0 {
			lines = append(lines, "")
			lines = append(lines,
				"  "+theme.Dim.Render("Fee: ")+
					theme.Value.Render(
						formatSats(tx.Fee)+
							" sats"))
		}
	}

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

func formatTimestampTable(unix int64) string {
	if unix == 0 {
		return "—"
	}
	return time.Unix(unix, 0).
		Format("2006-01-02 15:04")
}
