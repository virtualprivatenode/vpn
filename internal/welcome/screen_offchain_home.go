package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── WalletHomeScreen ──────────────────────────────────
// Section home for Wallet. Two focus zones: buttons
// (Send, Receive, Pairing) and scrollable payment
// history table. Reads live status through ctx.Status
// pointer. Payment history entries arrive via
// HandleMsg(paymentHistoryMsg) — same pattern as
// ChannelHistoryScreen.

const (
	walletHomeZoneButtons = 0
	walletHomeZoneList    = 1
)

type WalletHomeScreen struct {
	ctx       *ScreenContext
	btnIdx    int // 0=Send, 1=Receive, 2=Pairing
	focusZone int // 0=buttons, 1=payment list
	cursor    int // position in payment list
	entries   []lndrpc.PaymentEntry
}

func NewWalletHomeScreen(
	ctx *ScreenContext,
) *WalletHomeScreen {
	return &WalletHomeScreen{
		ctx: ctx,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *WalletHomeScreen) Init() tea.Cmd {
	return nil
}

func (s *WalletHomeScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	s.clampCursor()

	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.focusZone == walletHomeZoneButtons &&
			s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.focusZone == walletHomeZoneButtons &&
			s.btnIdx < 2 {
			s.btnIdx++
		}
		return s, nil
	case "up":
		if s.focusZone == walletHomeZoneList {
			if s.cursor > 0 {
				s.cursor--
			} else {
				s.focusZone = walletHomeZoneButtons
				s.btnIdx = 0
			}
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		if s.focusZone == walletHomeZoneButtons {
			if len(s.entries) > 0 {
				s.focusZone = walletHomeZoneList
				s.cursor = 0
			}
			return s, nil
		}
		if s.focusZone == walletHomeZoneList {
			if s.cursor < len(s.entries)-1 {
				s.cursor++
			}
		}
		return s, nil
	case "shift+tab":
		if s.focusZone == walletHomeZoneList {
			s.focusZone = walletHomeZoneButtons
			s.btnIdx = 0
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "backspace":
		return s, emitFocusSidebar
	case "enter":
		// No wallet → trigger wallet creation flow
		if !s.ctx.Cfg.WalletExists() {
			screen := NewWalletCreateScreen(s.ctx)
			return s, func() tea.Msg {
				return openTabMsg{
					Kind:   tabWalletCreate,
					Label:  "Create Wallet",
					Screen: screen,
				}
			}
		}
		return s.handleEnter()
	}
	return s, nil
}

func (s *WalletHomeScreen) handleEnter() (
	Screen, tea.Cmd,
) {
	if s.focusZone == walletHomeZoneButtons {
		switch s.btnIdx {
		case 0: // Send
			return s.openSend()
		case 1: // Receive
			return s.openReceive()
		case 2: // Pairing
			return s.openPairing()
		}
		return s, nil
	}

	// Payment list — open payment detail
	if s.cursor < len(s.entries) {
		entry := s.entries[s.cursor]
		label := entry.Memo
		if label == "" {
			if entry.IsIncoming {
				label = "↓ " + formatSats(
					entry.AmountSats)
			} else {
				label = "↑ " + formatSats(
					entry.AmountSats)
			}
		}
		if len(label) > 14 {
			label = label[:12] + ".."
		}
		screen := NewPaymentDetailScreen(
			s.ctx, entry)
		idx := s.cursor
		return s, func() tea.Msg {
			return openTabMsg{
				Kind:        tabPayment,
				Label:       label,
				Index:       idx,
				Screen:      screen,
				FocusTabBar: true,
			}
		}
	}
	return s, nil
}

func (s *WalletHomeScreen) openSend() (
	Screen, tea.Cmd,
) {
	cfg := s.ctx.Cfg
	if s.ctx.LndClient == nil || !cfg.HasLND() ||
		!cfg.WalletExists() {
		return s, nil
	}
	screen := NewSendScreen(s.ctx)
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabSend,
			Label:  "⚡ Send",
			Screen: screen,
		}
	}
}

func (s *WalletHomeScreen) openReceive() (
	Screen, tea.Cmd,
) {
	cfg := s.ctx.Cfg
	if s.ctx.LndClient == nil || !cfg.HasLND() ||
		!cfg.WalletExists() {
		return s, nil
	}
	screen := NewReceiveScreen(s.ctx)
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabReceive,
			Label:  "⚡ Receive",
			Screen: screen,
		}
	}
}

func (s *WalletHomeScreen) openPairing() (
	Screen, tea.Cmd,
) {
	screen := NewPairingScreen(s.ctx)
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabPairing,
			Label:  "⚡ Zeus — LND REST",
			Screen: screen,
		}
	}
}

func (s *WalletHomeScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case paymentHistoryMsg:
		if msg.err == nil {
			s.entries = msg.entries
		}
	}
	return s, nil
}

// ── View ────────────────────────────────────────────────

func (s *WalletHomeScreen) View(
	w, h int,
) string {
	s.clampCursor()
	cfg := s.ctx.Cfg
	status := s.ctx.Status

	if !cfg.HasLND() || !cfg.WalletExists() {
		return renderWalletPrompt(
			w, h, s.ctx.ContentFocused)
	}

	if status == nil || !status.lndResponding {
		return renderWaitingForLND(w, h)
	}

	isFocused := s.ctx.ContentFocused

	// ── Fixed header ─────────────────────────────
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render("Off-Chain Wallet"),
			w))
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		balanceSummaryLines(status, w)...)
	headerLines = append(headerLines, "")
	headerLines = append(headerLines, "")

	// ── Buttons ──────────────────────────────────
	isOnButton := isFocused &&
		s.focusZone == walletHomeZoneButtons
	headerLines = append(headerLines,
		renderButtons(
			[]string{"Send", "Receive", "Pairing"},
			s.btnIdx, isOnButton, w))
	headerLines = append(headerLines, "")
	headerLines = append(headerLines, "")

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

	if len(s.entries) == 0 {
		midLines = append(midLines,
			" "+theme.Dim.Render("No payments yet."))
	} else {
		balances := s.computeBalances()

		negStyle := lipgloss.NewStyle().
			Foreground(theme.ColorDanger)
		posStyle := lipgloss.NewStyle().
			Foreground(theme.ColorPrimary)
		dimStyle := theme.Dim
		selBg := lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true)

		for i, entry := range s.entries {
			isSelected := i == s.cursor &&
				isFocused &&
				s.focusZone == walletHomeZoneList

			date := formatTimestampTable(
				entry.CreationDate)
			dateStr := fmt.Sprintf("%-*s",
				dateW, date)

			// Failed/in-flight outgoing payments
			// didn't move funds — flag for special
			// rendering below.
			isFailed := !entry.IsIncoming &&
				entry.Status != "SUCCEEDED"

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
				} else if isFailed {
					memo = "(failed)"
				} else {
					memo = "—"
				}
			} else if isFailed {
				// Has a memo but still failed —
				// prepend status.
				if len(memo) > memoW-11 {
					memo = memo[:memoW-12] + ".."
				}
				memo = "(failed) " + memo
			}
			if len(memo) > memoW-1 {
				memo = memo[:memoW-2] + ".."
			}
			memoStr := fmt.Sprintf("%-*s",
				memoW, memo)

			var valStr string
			if isFailed {
				// No funds moved — show dash
				valStr = fmt.Sprintf("%*s",
					valW, "—")
			} else if entry.IsIncoming {
				valStr = fmt.Sprintf("%*s", valW,
					formatSats(entry.AmountSats))
			} else {
				valStr = fmt.Sprintf("%*s", valW,
					"-"+formatSats(
						entry.AmountSats))
			}

			// OPEN/EXPIRED incoming and failed
			// outgoing: no balance impact
			var balStr string
			if (entry.IsIncoming &&
				entry.Status != "SETTLED") ||
				isFailed {
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
			} else if isFailed {
				// Entire row dimmed for failed
				midLines = append(midLines,
					marker+
						dimStyle.Render(dateStr)+
						dimStyle.Render(memoStr)+
						dimStyle.Render(valStr)+
						dimStyle.Render(balStr))
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
		midContent, w, vpH, s.cursor,
		len(midLines),
		len(s.entries) > 0 &&
			s.focusZone == walletHomeZoneList)

	// ── Assemble output ──────────────────────────
	return header + "\n" + vpRendered
}

// ── HelpBindings ────────────────────────────────────────

func (s *WalletHomeScreen) HelpBindings() []key.Binding {
	if !s.ctx.Cfg.WalletExists() {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "create wallet")),
			kSidebar,
			kQuit,
		}
	}
	if s.focusZone == walletHomeZoneList {
		return s.listBindings()
	}
	return s.buttonBindings()
}

func (s *WalletHomeScreen) buttonBindings() []key.Binding {
	var binds []key.Binding
	if s.btnIdx == 0 {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left"),
				key.WithHelp("←", "sidebar")),
			key.NewBinding(
				key.WithKeys("right"),
				key.WithHelp("→", "button")))
	} else {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left", "right"),
				key.WithHelp("←→", "buttons")))
	}
	binds = append(binds,
		key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "payments")),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")))
	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑", "tab bar")))
	}
	binds = append(binds, kQuit)
	return binds
}

func (s *WalletHomeScreen) listBindings() []key.Binding {
	binds := []key.Binding{
		key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "payments")),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "details")),
		key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("⇧tab", "buttons")),
		kSidebar,
	}
	binds = append(binds, kQuit)
	return binds
}

// ── Helpers ─────────────────────────────────────────────

func (s *WalletHomeScreen) computeBalances() []int64 {
	balances := make([]int64, len(s.entries))
	var runBal int64
	if s.ctx.Status != nil {
		for _, ch := range s.ctx.Status.channels {
			runBal += ch.LocalBalance
		}
	}
	for i := 0; i < len(s.entries); i++ {
		balances[i] = runBal
		entry := s.entries[i]
		if entry.IsIncoming &&
			entry.Status == "SETTLED" {
			runBal -= entry.AmountSats
		} else if !entry.IsIncoming &&
			entry.Status == "SUCCEEDED" {
			// Only successful outgoing payments
			// affect the running balance.
			runBal += entry.AmountSats +
				entry.FeeSats
		}
	}
	return balances
}

func (s *WalletHomeScreen) clampCursor() {
	if len(s.entries) == 0 {
		s.cursor = 0
		return
	}
	if s.cursor >= len(s.entries) {
		s.cursor = len(s.entries) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
}
