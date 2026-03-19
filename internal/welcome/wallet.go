package welcome

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Receive ──────────────────────────────────────────────

func (m Model) viewReceive() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Lightning.Render("⚡ Receive Payment"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Amount (sats):"))
	lines = append(lines, "  "+theme.Value.Render(m.recvAmountStr+"_"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Memo (optional):"))
	if m.recvInputField == 0 {
		lines = append(lines, "  "+theme.Dim.Render(m.recvMemo))
	} else {
		lines = append(lines, "  "+theme.Value.Render(m.recvMemo+"_"))
	}
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render("Tab to switch fields"))
	lines = append(lines, "  "+theme.Dim.Render(
		"Enter to create invoice"))

	if m.recvError != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(m.recvError))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Receive ")
	footer := theme.Footer.Render(
		"  tab switch field • enter create invoice • backspace back  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewReceiveWaiting() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Lightning.Render("⚡ Waiting for Payment"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Amount: ")+
		theme.Value.Render(formatSats(m.recvAmountSats)+" sats"))
	if m.recvMemo != "" {
		lines = append(lines, "  "+theme.Label.Render("Memo:   ")+
			theme.Value.Render(m.recvMemo))
	}
	lines = append(lines, "")

	if m.recvPayReq != "" {
		qr := renderQRCode(m.recvPayReq)
		if qr != "" {
			lines = append(lines, qr)
		}
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Label.Render("Invoice:"))
		// Show truncated invoice for copy
		display := m.recvPayReq
		if len(display) > 60 {
			display = display[:60] + "..."
		}
		lines = append(lines, "  "+theme.Mono.Render(display))
		lines = append(lines, "")
		lines = append(lines, "  "+
			theme.Action.Render("[f] full invoice for copy"))
	}

	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"Scan QR or share invoice with sender."))
	lines = append(lines, "  "+theme.Dim.Render(
		"This screen updates when payment arrives."))

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Invoice Created ")
	footer := theme.Footer.Render(
		"  f full invoice • backspace cancel • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewReceivePaid() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Success.Render("✅ Payment Received!"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Amount: ")+
		theme.Value.Render(formatSats(m.recvAmountSats)+" sats"))
	if m.recvMemo != "" {
		lines = append(lines, "  "+theme.Label.Render("Memo:   ")+
			theme.Value.Render(m.recvMemo))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Payment Received ")
	footer := theme.Footer.Render("  enter done  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewReceiveExpired() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Warning.Render("⏰ Invoice Expired"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Value.Render(
		"The invoice was not paid before it expired."))
	lines = append(lines, "  "+theme.Value.Render(
		"Create a new invoice to try again."))

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Invoice Expired ")
	footer := theme.Footer.Render("  enter return • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

// ── Send ─────────────────────────────────────────────────

func (m Model) viewSend() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Lightning.Render("⚡ Send Payment"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Payment Request:"))
	lines = append(lines, "")

	display := m.sendPayReqInput
	if len(display) > 55 {
		// Show last 55 chars with ellipsis at start
		display = "..." + display[len(display)-55:]
	}
	lines = append(lines, "  "+theme.Value.Render(display+"_"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"Paste a bolt11 Lightning invoice"))
	lines = append(lines, "  "+theme.Dim.Render(
		"(starts with lnbc or lntb)"))
	lines = append(lines, "  "+theme.Dim.Render(
		"ctrl+u to clear field"))

	if m.sendError != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(m.sendError))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Send ")
	footer := theme.Footer.Render(
		"  enter decode • backspace back  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewSendConfirm() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Warning.Render("⚠ Confirm Payment"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Amount:      ")+
		theme.Value.Render(formatSats(m.sendDecodedAmt)+" sats"))
	if m.sendDecodedDesc != "" {
		lines = append(lines, "  "+theme.Label.Render("Description: ")+
			theme.Value.Render(m.sendDecodedDesc))
	}
	lines = append(lines, "  "+theme.Label.Render("Destination:"))
	dest := m.sendDecodedDest
	if len(dest) > 60 {
		dest = dest[:30] + "..." + dest[len(dest)-10:]
	}
	lines = append(lines, "  "+theme.Mono.Render(dest))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Fee limit:   ")+
		theme.Value.Render("1,000 sats max"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Warning.Render(
		"This will send "+formatSats(m.sendDecodedAmt)+
			" sats from your wallet."))
	lines = append(lines, "  "+theme.Warning.Render(
		"Payments cannot be reversed."))

	if m.sendError != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(m.sendError))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Confirm Payment ")
	footer := theme.Footer.Render(
		"  y confirm • backspace cancel  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewSendInFlight() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Lightning.Render("⚡ Sending Payment..."))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Value.Render(
		"Routing "+formatSats(m.sendDecodedAmt)+" sats"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"Finding a route through the Lightning Network."))
	lines = append(lines, "  "+theme.Dim.Render(
		"This may take up to 60 seconds over Tor."))
	lines = append(lines, "  "+theme.Dim.Render(
		"Do not close the terminal."))

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Sending ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewSendResult() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	if m.sendError != "" {
		lines = append(lines, theme.Warning.Render("❌ Payment Failed"))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(m.sendError))
	} else {
		lines = append(lines, theme.Success.Render("✅ Payment Sent!"))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Label.Render("Amount: ")+
			theme.Value.Render(formatSats(m.sendDecodedAmt)+" sats"))
		lines = append(lines, "  "+theme.Label.Render("Fee:    ")+
			theme.Value.Render(formatSats(m.sendFeeSats)+" sats"))
		if m.sendDecodedDesc != "" {
			lines = append(lines, "  "+theme.Label.Render("Memo:   ")+
				theme.Value.Render(m.sendDecodedDesc))
		}

		// Route visualization
		if len(m.sendRouteHops) > 0 {
			lines = append(lines, "")
			lines = append(lines, "  "+
				theme.Header.Render("Payment Route"))
			lines = append(lines, "")
			lines = append(lines, renderRouteVisualization(
				m.sendRouteHops))
			lines = append(lines, "")
			for i, hop := range m.sendRouteHops {
				name := hop.Alias
				if name == "" {
					if len(hop.PubKey) > 16 {
						name = hop.PubKey[:16] + "..."
					} else {
						name = hop.PubKey
					}
				}
				hopLabel := fmt.Sprintf("  Hop %d:", i+1)
				feeStr := ""
				if hop.FeeSats > 0 {
					feeStr = fmt.Sprintf(" (fee: %s sats)",
						formatSats(hop.FeeSats))
				}
				lines = append(lines, "  "+
					theme.Label.Render(hopLabel)+" "+
					theme.Value.Render(name)+
					theme.Dim.Render(feeStr))
			}
		}

		if m.sendPreimage != "" {
			lines = append(lines, "")
			lines = append(lines, "  "+
				theme.Label.Render("Preimage:"))
			lines = append(lines, "  "+
				theme.Mono.Render(m.sendPreimage))
		}
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Payment Result ")
	footer := theme.Footer.Render(
		"  enter return to wallet • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

// renderRouteVisualization creates a text diagram of the payment route.
func renderRouteVisualization(hops []lndrpc.RouteHop) string {
	if len(hops) == 0 {
		return ""
	}
	var parts []string
	parts = append(parts, "  You")
	for _, hop := range hops {
		name := hop.Alias
		if name == "" {
			if len(hop.PubKey) > 8 {
				name = hop.PubKey[:8]
			} else {
				name = hop.PubKey
			}
		}
		if len(name) > 12 {
			name = name[:12]
		}
		parts = append(parts, name)
	}
	return "  " + theme.Good.Render(strings.Join(parts, " ━━▸ "))
}

// ── Payment History ──────────────────────────────────────

func (m Model) viewPaymentHistory() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	lines = append(lines, theme.Lightning.Render("⚡ Payment History"))
	lines = append(lines, "")

	if len(m.payHistory) == 0 {
		lines = append(lines, "  "+theme.Dim.Render(
			"No payments yet."))
	} else {
		visibleCount := theme.BoxHeight - 8
		if visibleCount < 5 {
			visibleCount = 5
		}

		viewStart := 0
		if m.payHistoryCursor >= viewStart+visibleCount {
			viewStart = m.payHistoryCursor - visibleCount + 1
		}
		viewEnd := viewStart + visibleCount
		if viewEnd > len(m.payHistory) {
			viewEnd = len(m.payHistory)
		}

		if viewStart > 0 {
			lines = append(lines, "  "+
				theme.Dim.Render("  ↑ more"))
		}

		for i := viewStart; i < viewEnd; i++ {
			entry := m.payHistory[i]
			prefix := "  "
			style := theme.Value
			if m.payHistoryCursor == i {
				prefix = "▸ "
				style = theme.Action
			}

			direction := theme.Warning.Render("↑ sent")
			if entry.IsIncoming {
				direction = theme.Success.Render("↓ recv")
			}

			amount := formatSats(entry.AmountSats) + " sats"
			statusDot := theme.GreenDot.Render("●")
			if entry.Status == "FAILED" {
				statusDot = theme.RedDot.Render("●")
			} else if entry.Status == "EXPIRED" {
				statusDot = theme.Dim.Render("○")
			} else if entry.Status == "IN_FLIGHT" ||
				entry.Status == "OPEN" {
				statusDot = theme.Dim.Render("◌")
			}

			memo := entry.Memo
			if len(memo) > 20 {
				memo = memo[:20] + "..."
			}
			if memo == "" {
				memo = "—"
			}

			ts := formatTimestamp(entry.CreationDate)

			lines = append(lines, fmt.Sprintf(
				"%s%s %s  %s  %s  %s  %s",
				prefix, statusDot, direction,
				style.Render(fmt.Sprintf("%-12s", amount)),
				theme.Dim.Render(fmt.Sprintf("%-22s", memo)),
				theme.Dim.Render(ts),
				""))
		}

		if viewEnd < len(m.payHistory) {
			lines = append(lines, "  "+
				theme.Dim.Render("  ↓ more"))
		}
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Payment History ")
	footer := theme.Footer.Render(
		"  ↑↓ select • enter details • backspace back • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewPaymentDetail() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	if m.payHistoryCursor >= len(m.payHistory) {
		lines = append(lines,
			theme.Warn.Render("Payment not found"))
	} else {
		entry := m.payHistory[m.payHistoryCursor]

		if entry.IsIncoming {
			lines = append(lines,
				theme.Success.Render("↓ Received Payment"))
		} else {
			lines = append(lines,
				theme.Warning.Render("↑ Sent Payment"))
		}
		lines = append(lines, "")

		lines = append(lines, "  "+theme.Label.Render("Amount:  ")+
			theme.Value.Render(formatSats(entry.AmountSats)+" sats"))
		if entry.FeeSats > 0 {
			lines = append(lines, "  "+theme.Label.Render("Fee:     ")+
				theme.Value.Render(formatSats(entry.FeeSats)+" sats"))
		}
		lines = append(lines, "  "+theme.Label.Render("Status:  ")+
			theme.Value.Render(entry.Status))
		if entry.Memo != "" {
			lines = append(lines, "  "+theme.Label.Render("Memo:    ")+
				theme.Value.Render(entry.Memo))
		}
		lines = append(lines, "  "+theme.Label.Render("Date:    ")+
			theme.Value.Render(
				formatTimestampFull(entry.CreationDate)))

		if entry.Preimage != "" {
			lines = append(lines, "")
			lines = append(lines, "  "+
				theme.Label.Render("Preimage:"))
			lines = append(lines, "  "+
				theme.Mono.Render(entry.Preimage))
		}

		if entry.PaymentHash != "" {
			lines = append(lines, "")
			lines = append(lines, "  "+
				theme.Label.Render("Payment Hash:"))
			hash := entry.PaymentHash
			if len(hash) > 60 {
				hash = hash[:60] + "..."
			}
			lines = append(lines, "  "+theme.Mono.Render(hash))
		}

		// Route hops for outgoing payments
		if !entry.IsIncoming && len(entry.Hops) > 0 {
			lines = append(lines, "")
			lines = append(lines, "  "+
				theme.Header.Render("Route"))
			lines = append(lines, renderRouteVisualization(
				entry.Hops))
			for i, hop := range entry.Hops {
				name := hop.Alias
				if name == "" {
					if len(hop.PubKey) > 16 {
						name = hop.PubKey[:16] + "..."
					} else {
						name = hop.PubKey
					}
				}
				feeStr := ""
				if hop.FeeSats > 0 {
					feeStr = fmt.Sprintf(" fee: %s",
						formatSats(hop.FeeSats))
				}
				lines = append(lines, fmt.Sprintf(
					"  %s %s%s",
					theme.Label.Render(
						fmt.Sprintf("Hop %d:", i+1)),
					theme.Value.Render(name),
					theme.Dim.Render(feeStr)))
			}
		}
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡ Payment Details ")
	footer := theme.Footer.Render(
		"  backspace back • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

// ── Time formatting ──────────────────────────────────────

func formatTimestamp(unix int64) string {
	if unix == 0 {
		return "—"
	}
	t := time.Unix(unix, 0)
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	}
	if diff < 7*24*time.Hour {
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	}
	return t.Format("Jan 2")
}

func formatTimestampFull(unix int64) string {
	if unix == 0 {
		return "—"
	}
	return time.Unix(unix, 0).Format("2006-01-02 15:04:05")
}
