package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── ChannelDetailScreen ────────────────────────────────
// Channel detail view. Displays channel info and a Close
// Channel button. When the user presses Close, the screen
// delegates to an embedded ChannelCloseScreen rather than
// opening a separate tab — the close flow, result, and
// tab close all happen within this detail tab. No stale
// detail tab after channel closure.
//
// The close screen is a separate type composed via a
// pointer field (option 2 in the design discussion). The
// detail screen acts as a thin router: when closeScreen
// is non-nil, all interface methods delegate to it. The
// close screen stays independently testable.

type ChannelDetailScreen struct {
	ctx     *ScreenContext
	channel channelInfo

	// Button index for detail view (0=Cancel, 1=Close)
	viewBtnIdx int

	// Fee tiers snapshot for passing to close screen
	feeTiers [4]feeTier

	// Close flow delegation — nil means detail view,
	// non-nil means the close flow is active and all
	// input/rendering delegates to it.
	closeScreen *ChannelCloseScreen
}

func NewChannelDetailScreen(
	ctx *ScreenContext,
	ch channelInfo,
	feeTiers [4]feeTier,
) *ChannelDetailScreen {
	return &ChannelDetailScreen{
		ctx:      ctx,
		channel:  ch,
		feeTiers: feeTiers,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *ChannelDetailScreen) Init() tea.Cmd {
	return nil
}

func (s *ChannelDetailScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	if s.closeScreen != nil {
		newClose, cmd :=
			s.closeScreen.HandleKey(keyStr, msg)
		s.closeScreen = newClose.(*ChannelCloseScreen)
		if s.closeScreen.Cancelled {
			s.closeScreen = nil
			return s, nil
		}
		return s, cmd
	}

	// Pending channels: view-only, no button
	if s.channel.Pending {
		switch keyStr {
		case "ctrl+c":
			return s, tea.Quit
		}
		return s, emitFocusTabBar
	}

	// Non-pending: Cancel / Close Channel buttons
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.viewBtnIdx > 0 {
			s.viewBtnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.viewBtnIdx < 1 {
			s.viewBtnIdx++
		}
		return s, nil
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		// Already on buttons, nowhere to go
		return s, nil
	case "backspace":
		return s, emitFocusParent
	case "enter":
		if s.viewBtnIdx == 0 {
			return s, emitCloseTab
		}
		return s.launchClose()
	}
	return s, nil
}

func (s *ChannelDetailScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	if s.closeScreen != nil {
		newClose, cmd := s.closeScreen.HandleMsg(msg)
		s.closeScreen = newClose.(*ChannelCloseScreen)
		return s, cmd
	}
	switch msg := msg.(type) {
	case tabActivatedMsg:
		// Re-find the channel in live status data
		// so the detail view reflects any changes
		// since this tab was last viewed (e.g.
		// balance change after payment settlement).
		if s.ctx.Status != nil {
			for _, ch := range s.ctx.Status.channels {
				if ch.ChannelPoint ==
					s.channel.ChannelPoint {
					s.channel = ch
					break
				}
			}
		}
		return s, nil
	case feeTiersMsg:
		if msg.err == nil {
			s.feeTiers = msg.tiers
		}
		return s, nil
	}
	return s, nil
}

func (s *ChannelDetailScreen) View(
	w, h int,
) string {
	if s.closeScreen != nil {
		return s.closeScreen.View(w, h)
	}

	ch := s.channel
	p := newPane(w)

	name := ch.PeerAlias
	if name == "" {
		name = ch.RemotePubkey[:16] + "..."
	}
	p.title(theme.Header, name)

	status := theme.Success.Render("active")
	if !ch.Active {
		status = theme.Warning.Render("inactive")
	}
	if ch.Pending {
		status = theme.Dim.Render("pending")
	}

	p.line(" " + theme.Label.Render("Status:    ") +
		status)
	p.field("Capacity:  ",
		formatSats(ch.Capacity)+" sats")
	p.field("Local:     ",
		formatSats(ch.LocalBalance)+" sats")
	p.field("Remote:    ",
		formatSats(ch.RemoteBalance)+" sats")

	barW := w - 4
	if barW > 40 {
		barW = 40
	}
	if barW >= 10 {
		p.blank()
		p.line(" " + renderLiquidityBar(
			ch.LocalBalance, ch.RemoteBalance,
			ch.Capacity, barW))
	}
	p.blank()

	if ch.Private {
		p.field("Type:      ", "private")
	} else {
		p.field("Type:      ", "public")
	}
	if strings.Contains(ch.CommitmentType, "TAPROOT") {
		p.field("Channel:   ", "taproot")
	}
	if ch.Initiator {
		p.field("Initiator: ", "you")
	}

	p.blank()
	p.labelLine("Pubkey:")
	p.mono(ch.RemotePubkey)

	if ch.ChanID > 0 {
		p.blank()
		p.monoField("Channel ID: ",
			fmt.Sprintf("%d", ch.ChanID))
	}

	// Cancel / Close Channel pinned to bottom (not for pending)
	if !ch.Pending {
		btnFocused := s.ctx.ContentFocused
		return p.renderWithBottomButtons(
			[]string{"Cancel", "Close Channel"},
			s.viewBtnIdx, btnFocused, h)
	}

	return p.render()
}

func (s *ChannelDetailScreen) HelpBindings() []key.Binding {
	if s.closeScreen != nil {
		return s.closeScreen.HelpBindings()
	}
	if s.channel.Pending {
		return viewDetailBindings(s.ctx.HasTabs)
	}
	return detailActionBindings(
		"close channel", s.viewBtnIdx, s.ctx.HasTabs)
}

// ── Close channel launch ───────────────────────────────

func (s *ChannelDetailScreen) launchClose() (
	Screen, tea.Cmd,
) {
	s.closeScreen = NewChannelCloseScreen(
		s.ctx,
		s.channel.ChannelPoint,
		s.channel.PeerAlias,
		s.channel.Capacity,
		s.channel.LocalBalance,
		s.channel.RemoteBalance,
		s.feeTiers)
	return s, fetchFeeTiersCmd(s.ctx.Cfg)
}
