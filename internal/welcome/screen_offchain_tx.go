package welcome

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── PaymentDetailScreen ────────────────────────────────
// View-only tab showing a single payment's details.
// Done button pinned to bottom.

type PaymentDetailScreen struct {
	ctx   *ScreenContext
	entry lndrpc.PaymentEntry
}

func NewPaymentDetailScreen(
	ctx *ScreenContext,
	entry lndrpc.PaymentEntry,
) *PaymentDetailScreen {
	return &PaymentDetailScreen{
		ctx:   ctx,
		entry: entry,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *PaymentDetailScreen) Init() tea.Cmd {
	return nil
}

func (s *PaymentDetailScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		return s, emitFocusSidebar
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		return s, nil
	case "backspace":
		// Clean backspace: does nothing
		return s, nil
	case "enter":
		return s, emitCloseTab
	}
	return s, nil
}

func (s *PaymentDetailScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	return s, nil
}

func (s *PaymentDetailScreen) View(
	w, h int,
) string {
	entry := s.entry
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

	return p.renderWithBottomButtons(
		[]string{"Done"}, 0,
		s.ctx.ContentFocused, h)
}

func (s *PaymentDetailScreen) HelpBindings() []key.Binding {
	var binds []key.Binding

	binds = append(binds,
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "done")),
		kSidebar)

	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("shift+tab"),
				key.WithHelp("⇧tab", "tab bar")))
	}

	binds = append(binds, kQuit)
	return binds
}
