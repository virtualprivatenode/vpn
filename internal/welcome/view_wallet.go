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
	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		p := newPane(w)
		p.dim("Install LND and create wallet.")
		p.blank()
		p.line(m.walletButtons(w))
		return p.render()
	}
	if m.status == nil || !m.status.lndResponding {
		p := newPane(w)
		p.dim("Waiting for LND...")
		p.blank()
		p.line(m.walletButtons(w))
		return p.render()
	}

	isFocused := m.contentFocused && !m.tabFocused

	// ── Fixed header ─────────────────────────────
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render("Off-Chain Wallet"),
			w))
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		m.renderBalanceSummary(w)...)
	headerLines = append(headerLines, "")
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		m.walletButtons(w))
	headerLines = append(headerLines, "")
	headerLines = append(headerLines, "")

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

	hdrStyle := theme.TableHeader
	sepStyle := theme.TableDim

	hdr := " " +
		hdrStyle.Render(
			fmt.Sprintf("%-*s", dateW, "Date")) +
		hdrStyle.Render(
			fmt.Sprintf("%-*s", memoW, "Memo")) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", valW, "Value")) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", balW, "Balance"))
	headerLines = append(headerLines, hdr)
	headerLines = append(headerLines,
		" "+sepStyle.Render(
			strings.Repeat("─", tableW)))

	header := strings.Join(headerLines, "\n")
	headerH := len(headerLines)

	// ── Scrollable middle (payment rows) ─────────
	var midLines []string

	if len(m.payHistory) == 0 {
		midLines = append(midLines,
			" "+theme.Dim.Render("No payments yet."))
	} else {
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

		negStyle := lipgloss.NewStyle().
			Foreground(theme.ColorDanger)
		posStyle := lipgloss.NewStyle().
			Foreground(theme.ColorPrimary)
		selBg := lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true)

		for i, entry := range m.payHistory {
			isSelected := i == m.payHistoryCursor &&
				isFocused &&
				m.contentFocus == 1

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
					"-"+formatSats(
						entry.AmountSats))
			}

			bal := balances[i]
			balStr := fmt.Sprintf("%*s",
				balW, formatSats(bal))

			marker := " "
			if isSelected {
				marker = "▸"
				midLines = append(midLines,
					marker+
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
				midLines = append(midLines,
					marker+
						theme.Value.Render(dateStr)+
						theme.Dim.Render(memoStr)+
						valRendered+
						theme.Value.Render(balStr))
			}
		}
	}

	midContent := strings.Join(midLines, "\n")

	// ── Viewport ─────────────────────────────────
	vpH := h - headerH
	if vpH < 1 {
		vpH = 1
	}

	vpRendered := renderViewport(
		midContent, w, vpH, m.payHistoryCursor,
		len(midLines),
		len(m.payHistory) > 0 && m.contentFocus == 1)

	// ── Assemble output ──────────────────────────
	return header + "\n" + vpRendered
}

func (m Model) walletButtons(w int) string {
	isFocused := m.contentFocused &&
		!m.tabFocused &&
		m.contentFocus == 0
	return renderButtons(
		[]string{"Send", "Receive", "Pairing"},
		m.btnIdx, isFocused, w)
}

// ── Payment detail ───────────────────────────────────────

func (m Model) paymentDetailContent(w int) string {
	if m.payHistoryCursor >= len(m.payHistory) {
		p := newPane(w)
		p.warn("Payment not found")
		return p.render()
	}

	entry := m.payHistory[m.payHistoryCursor]
	p := newPane(w)

	if entry.IsIncoming {
		p.title(theme.Success, "↓ Received Payment")
	} else {
		p.title(theme.Warning, "↑ Sent Payment")
	}

	p.field("Amount:  ",
		formatSats(entry.AmountSats)+" sats")
	if entry.FeeSats > 0 {
		p.field("Fee:     ",
			formatSats(entry.FeeSats)+" sats")
	}
	p.field("Status:  ", entry.Status)
	if entry.Memo != "" {
		p.field("Memo:    ", entry.Memo)
	}
	p.field("Date:    ",
		formatTimestampFull(entry.CreationDate))

	if entry.Preimage != "" {
		p.blank()
		p.labelLine("Preimage:")
		p.monoWrap(entry.Preimage)
	}
	if entry.PaymentHash != "" {
		p.blank()
		p.labelLine("Payment Hash:")
		p.monoWrap(entry.PaymentHash)
	}
	if len(entry.Hops) > 0 {
		p.blank()
		p.labelLine("Route:")
		p.line(renderRouteDiagram(entry.Hops, w))
	}
	return p.render()
}

// ── Send panes ───────────────────────────────────────────

func (m Model) walletSendPane(w int) string {
	p := newPane(w)
	p.title(theme.Header, "⚡ Send Payment")

	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		p.dim("Install LND and create wallet to send.")
		return p.render()
	}
	if m.status == nil || !m.status.lndResponding {
		p.dim("Waiting for LND...")
		return p.render()
	}

	var totalLocal int64
	if m.status != nil {
		for _, ch := range m.status.channels {
			totalLocal += ch.LocalBalance
		}
	}
	p.field("Spendable: ",
		formatSats(totalLocal)+" sats")
	p.blank()

	p.input("Payment Request:",
		m.sendInput, m.contentFocused)
	p.blank()
	p.dim("Paste a bolt11 invoice")

	p.appendError(m.sendError)
	return p.render()
}

func (m Model) walletSendConfirmPane(w int) string {
	p := newPane(w)
	p.title(theme.Warning, "Confirm Payment")

	p.field("Amount:      ",
		formatSats(m.sendDecodedAmt)+" sats")
	if m.sendDecodedDesc != "" {
		p.field("Description: ", m.sendDecodedDesc)
	}
	p.labelLine("Destination:")
	p.monoWrap(m.sendDecodedDest)
	p.blank()
	p.warn("Send " +
		formatSats(m.sendDecodedAmt) + " sats?")

	p.appendError(m.sendError)
	return p.render()
}

func (m Model) walletSendInFlightPane(w int) string {
	p := newPane(w)
	p.title(theme.Header, "Sending Payment...")
	p.line(" " + theme.Value.Render(
		"Routing "+formatSats(m.sendDecodedAmt)+
			" sats"))
	p.blank()
	p.dim("May take up to 60 seconds over Tor.")
	return p.render()
}

func (m Model) walletSendResultPane(w int) string {
	p := newPane(w)

	if m.sendError != "" {
		p.title(theme.Warning, "Payment Failed")
		p.warn(m.sendError)
	} else {
		p.title(theme.Success, "Payment Sent")
		p.field("Amount: ",
			formatSats(m.sendDecodedAmt)+" sats")
		p.field("Fee:    ",
			formatSats(m.sendFeeSats)+" sats")
		if m.sendPreimage != "" {
			p.blank()
			p.labelLine("Preimage:")
			p.monoWrap(m.sendPreimage)
		}
		if len(m.sendRouteHops) > 0 {
			p.blank()
			p.labelLine("Route:")
			p.line(renderRouteDiagram(
				m.sendRouteHops, w))
		}
	}

	return p.render()
}

// ── Receive panes ────────────────────────────────────────

func (m Model) walletReceivePane(w int) string {
	p := newPane(w)
	p.title(theme.Header, "⚡ Receive Payment")

	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		p.dim("Install LND and create wallet.")
		return p.render()
	}
	if m.status == nil || !m.status.lndResponding {
		p.dim("Waiting for LND...")
		return p.render()
	}

	amtFocused := m.recvAmountInput.Focused() &&
		m.contentFocused
	memoFocused := m.recvMemoInput.Focused() &&
		m.contentFocused

	p.input("Amount (sats):",
		m.recvAmountInput, amtFocused)
	p.blank()
	p.input("Memo (optional):",
		m.recvMemoInput, memoFocused)

	p.appendError(m.recvError)
	return p.render()
}

func (m Model) walletReceiveWaitingPane(
	w int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Waiting for Payment")

	p.field("Amount: ",
		formatSats(m.recvAmountSats)+" sats")
	p.blank()

	if m.recvPayReq != "" {
		p.labelLine("Invoice:")
		p.monoWrap(m.recvPayReq)
		p.blank()

		btnFocused := m.contentFocused &&
			!m.tabFocused
		p.buttons(
			[]string{"Show QR", "Copyable Invoice"},
			m.recvButtonIdx, btnFocused)
	}

	p.blank()
	p.dim("Waiting for payment...")

	return p.render()
}

func (m Model) walletReceivePaidPane(w int) string {
	p := newPane(w)
	p.title(theme.Success, "Payment Received")
	p.field("Amount: ",
		formatSats(m.recvAmountSats)+" sats")
	return p.render()
}

func (m Model) walletReceiveExpiredPane(w int) string {
	p := newPane(w)
	p.title(theme.Warning, "Invoice Expired")
	p.dim("Create a new invoice to try again.")
	return p.render()
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
