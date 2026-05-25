package welcome

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Confirm step ───────────────────────────────────────

func (s *ChannelOpenScreen) handleConfirmKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.confirmBtnIdx > 0 {
			s.confirmBtnIdx--
		} else {
			return s, emitFocusSidebar
		}
		return s, nil
	case "right":
		if s.confirmBtnIdx < 1 {
			s.confirmBtnIdx++
		}
		return s, nil
	case "up":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "backspace":
		s.backToInput()
		return s, nil
	case "enter":
		switch s.confirmBtnIdx {
		case 0: // Go Back
			s.backToInput()
			return s, nil
		case 1: // Confirm
			if s.inFlight {
				return s, nil
			}
			s.inFlight = true
			s.error = ""
			s.step = coStepOpening
			feeRate := s.feeInput.Sats()
			return s, openChannelCmd(
				s.ctx.LndClient,
				s.selectedPubkey(),
				s.selectedHost(),
				s.amount,
				s.private,
				s.taproot,
				s.utxoOutpoints,
				s.fundMax,
				uint64(feeRate),
			)
		}
	}
	return s, nil
}

func (s *ChannelOpenScreen) backToInput() {
	s.step = coStepInput
	s.error = ""
	s.confirmBtnIdx = 0
	s.focusZone = coZoneButtons
	s.btnIdx = 1
}

// ── Opening step ───────────────────────────────────────

func (s *ChannelOpenScreen) handleOpeningKey(
	keyStr string,
) (Screen, tea.Cmd) {
	// Fund-moving operation in progress — block all keys.
	return s, nil
}

// ── Result step ────────────────────────────────────────

func (s *ChannelOpenScreen) handleResultKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "enter":
		return s, tea.Batch(
			emitCloseTab,
			emitRefreshStatus)
	case "left":
		return s, emitFocusSidebar
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
	case "backspace":
		return s, emitFocusParent
	}
	return s, nil
}

// ── Confirm view ───────────────────────────────────────

func (s *ChannelOpenScreen) viewConfirm(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Warning, "Confirm Channel Open")

	p.field("Peer:    ", s.selectedAlias())

	// Estimate fee for display
	feeRate := s.feeInput.Sats()
	numInputs := len(s.utxoSelected)
	if numInputs < 1 {
		numInputs = 1
	}
	numOutputs := 1
	if !s.fundMax {
		numOutputs = 2 // channel + change
	}
	estFee := estimateSimpleFee(
		numInputs, numOutputs, feeRate)

	if s.fundMax {
		if len(s.utxoSelected) > 0 {
			chanAmt := s.utxoSelectedTotal - estFee
			if chanAmt < 0 {
				chanAmt = 0
			}
			p.field("Amount:  ",
				fmt.Sprintf("~%s sats (full UTXO minus fee)",
					formatSats(chanAmt)))
		} else {
			p.field("Amount:  ",
				"Max (full balance minus fee)")
		}
	} else {
		p.field("Amount:  ",
			formatSats(s.amount)+" sats")
	}

	chanType := "public"
	if s.private {
		chanType = "private"
	}
	if s.taproot {
		chanType += ", taproot"
	}
	p.field("Type:    ", chanType)

	if feeRate > 0 {
		p.field("Fee:     ",
			fmt.Sprintf("%d sat/vB (~%s sats)",
				feeRate, formatSats(estFee)))
	} else {
		p.field("Fee:     ", "auto (LND default)")
	}

	if len(s.utxoSelected) > 0 {
		p.field("UTXOs:   ",
			fmt.Sprintf("%d selected (%s sats)",
				len(s.utxoSelected),
				formatSats(s.utxoSelectedTotal)))
	}

	// Change warning for custom amount with coin control
	if !s.fundMax && len(s.utxoSelected) > 0 &&
		s.amount < s.utxoSelectedTotal {
		change := s.utxoSelectedTotal -
			s.amount - estFee
		if change > 0 {
			p.field("Change:  ",
				theme.Warning.Render(
					fmt.Sprintf("~%s sats",
						formatSats(change))))
		}
	}

	p.blank()

	p.labelLine("Pubkey:")
	p.mono(s.selectedPubkey())
	p.blank()

	if s.fundMax {
		p.warn("Spend full UTXO amount minus fee?")
	} else {
		p.warn("Spend " +
			formatSats(s.amount) + " sats?")
	}

	p.appendError(s.error)

	return p.renderWithBottomButtons(
		[]string{"Go Back", "Confirm"},
		s.confirmBtnIdx, s.ctx.ContentFocused, h)
}

// ── Opening view ───────────────────────────────────────

func (s *ChannelOpenScreen) viewOpening(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Opening Channel...")
	p.line(" " + theme.Value.Render(
		"Connecting to peer and broadcasting tx."))
	p.blank()
	p.dim("May take up to 2 minutes over Tor.")
	p.dim("Do not close the terminal.")
	return p.renderWithBottomButtons(
		[]string{"Opening..."}, 0, false, h)
}

// ── Result view ────────────────────────────────────────

func (s *ChannelOpenScreen) viewResult(
	w int,
) string {
	p := newPane(w)

	if s.error != "" {
		p.title(theme.Warning, "Channel Open Failed")
		p.warnWrap(s.error)
	} else {
		p.title(theme.Success, "Channel Opening")
		p.line(" " + theme.Value.Render(
			"Funding tx broadcast successfully."))
		p.blank()
		p.field("Peer:   ", s.selectedAlias())
		if s.fundMax {
			if len(s.utxoSelected) > 0 {
				p.field("Amount: ",
					fmt.Sprintf(
						"Max (%s sats minus fee)",
						formatSats(
							s.utxoSelectedTotal)))
			} else {
				p.field("Amount: ",
					"Max (full balance minus fee)")
			}
		} else {
			p.field("Amount: ",
				formatSats(s.amount)+" sats")
		}
		if s.txid != "" {
			p.blank()
			p.labelLine("TX ID:")
			p.monoWrap(s.txid)
		}
		p.blank()
		p.dim("Channel will appear as pending.")
	}

	return p.render()
}
