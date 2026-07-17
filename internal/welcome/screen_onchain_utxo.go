package welcome

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/lndrpc"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ── UtxoDetailScreen ───────────────────────────────────
// View-only tab showing a single UTXO's details.
// No buttons — navigate away via backspace (parent).

type UtxoDetailScreen struct {
	ctx   *ScreenContext
	utxo  lndrpc.UTXO
	date  string // pre-resolved from tx list
	label string // pre-resolved from tx list
}

func NewUtxoDetailScreen(
	ctx *ScreenContext,
	utxo lndrpc.UTXO,
	txDate string,
	txLabel string,
) *UtxoDetailScreen {
	return &UtxoDetailScreen{
		ctx:   ctx,
		utxo:  utxo,
		date:  txDate,
		label: txLabel,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *UtxoDetailScreen) Init() tea.Cmd {
	return nil
}

func (s *UtxoDetailScreen) HandleKey(
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
		return s, emitFocusParent
	}
	return s, nil
}

func (s *UtxoDetailScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	return s, nil
}

func (s *UtxoDetailScreen) View(
	w, h int,
) string {
	u := s.utxo
	p := newPane(w)
	p.title(theme.Header, "UTXO Detail")

	p.field("Amount:    ",
		formatSats(u.AmountSats)+" sats")

	confStr := fmt.Sprintf("%d", u.Confirmations)
	if u.Confirmations == 0 {
		confStr = "unconfirmed"
	}
	p.field("Confs:     ", confStr)

	dateStr := s.date
	if u.Confirmations == 0 {
		dateStr = "unconfirmed"
	}
	p.field("Date:      ", dateStr)
	p.blank()

	p.labelLine("Outpoint:")
	outpoint := fmt.Sprintf("%s:%d", u.Txid, u.Vout)
	p.monoWrap(outpoint)
	p.blank()

	p.labelLine("Address:")
	p.monoWrap(u.Address)
	p.blank()

	if s.label != "" {
		p.field("Label:     ", s.label)
	} else {
		p.field("Label:     ",
			theme.Dim.Render("none"))
	}

	return p.render()
}

func (s *UtxoDetailScreen) HelpBindings() []key.Binding {
	return viewDetailBindings(s.ctx.HasTabs)
}
