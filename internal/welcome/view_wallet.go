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
			if entry.IsIncoming &&
				entry.Status == "SETTLED" {
				runBal -= entry.AmountSats
			} else if !entry.IsIncoming {
				runBal += entry.AmountSats +
					entry.FeeSats
			}
		}

		negStyle := lipgloss.NewStyle().
			Foreground(theme.ColorDanger)
		posStyle := lipgloss.NewStyle().
			Foreground(theme.ColorPrimary)
		dimStyle := theme.Dim
		selBg := lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true)

		for i, entry := range m.payHistory {
			isSelected := i == m.payHistoryCursor &&
				isFocused &&
				m.contentFocus() == 1

			date := formatTimestampTable(
				entry.CreationDate)
			dateStr := fmt.Sprintf("%-*s",
				dateW, date)
			memo := entry.Memo
			if memo == "" {
				if entry.IsIncoming &&
					entry.Status == "OPEN" {
					memo = "(pending)"
				} else if entry.IsIncoming &&
					entry.Status == "EXPIRED" {
					memo = "(expired)"
				} else if entry.IsIncoming &&
					entry.Status == "CANCELED" {
					memo = "(canceled)"
				} else if entry.IsIncoming &&
					entry.Status == "ACCEPTED" {
					memo = "(accepted)"
				} else {
					memo = "—"
				}
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

			// OPEN/EXPIRED incoming: no balance impact
			var balStr string
			if entry.IsIncoming &&
				entry.Status != "SETTLED" {
				balStr = fmt.Sprintf("%*s", balW, "—")
			} else {
				bal := balances[i]
				balStr = fmt.Sprintf("%*s",
					balW, formatSats(bal))
			}

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
				if entry.IsIncoming &&
					entry.Status == "SETTLED" {
					valRendered =
						posStyle.Render(valStr)
				} else if entry.IsIncoming {
					// OPEN or EXPIRED — dim
					valRendered =
						dimStyle.Render(valStr)
				} else {
					valRendered =
						negStyle.Render(valStr)
				}

				var dateRendered, memoRendered,
					balRendered string
				if entry.IsIncoming &&
					entry.Status != "SETTLED" {
					dateRendered =
						dimStyle.Render(dateStr)
					memoRendered =
						dimStyle.Render(memoStr)
					balRendered =
						dimStyle.Render(balStr)
				} else {
					dateRendered =
						theme.Value.Render(dateStr)
					memoRendered =
						theme.Dim.Render(memoStr)
					balRendered =
						theme.Value.Render(balStr)
				}
				midLines = append(midLines,
					marker+
						dateRendered+
						memoRendered+
						valRendered+
						balRendered)
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
		len(m.payHistory) > 0 && m.contentFocus() == 1)

	// ── Assemble output ──────────────────────────
	return header + "\n" + vpRendered
}

func (m Model) walletButtons(w int) string {
	isFocused := m.contentFocused &&
		!m.tabFocused &&
		m.contentFocus() == 0
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
		switch entry.Status {
		case "SETTLED":
			p.title(theme.Success,
				"Received Payment")
		case "OPEN":
			p.title(theme.Header,
				"Pending Invoice")
		case "EXPIRED":
			p.title(theme.Warning,
				"Expired Invoice")
		case "CANCELED":
			p.title(theme.Warning,
				"Canceled Invoice")
		case "ACCEPTED":
			p.title(theme.Header,
				"Accepting Payment")
		default:
			p.title(theme.Header,
				"Incoming Invoice")
		}
	} else {
		p.title(theme.Warning, "Sent Payment")
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
