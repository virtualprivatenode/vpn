package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ChannelDetailScreen retains the selected funding outpoint and owns its close
// flow. The embedded screen owns approval and the result for that channel.
type ChannelDetailScreen struct {
	ctx         *ScreenContext
	channel     channelInfo
	unavailable bool

	// Button index for detail view (0=Cancel, 1=Close)
	viewBtnIdx int

	// A nil close screen means the detail view is active;
	// non-nil means the close flow is active and all
	// input/rendering delegates to it.
	closeScreen *ChannelCloseScreen
}

func NewChannelDetailScreen(
	ctx *ScreenContext,
	ch channelInfo,
) *ChannelDetailScreen {
	return &ChannelDetailScreen{
		ctx:     ctx,
		channel: ch,
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

	// Pending or unavailable channels have no close action.
	if s.channel.Pending || s.unavailable {
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
	switch msg.(type) {
	case tabActivatedMsg:
		// Re-find the channel in live status data
		// so the detail view reflects any changes
		// since this tab was last viewed (e.g.
		// balance change after payment settlement).
		if s.ctx.Status != nil && s.ctx.Status.Channels.Known() {
			s.unavailable = true
			for _, ch := range s.ctx.Status.Channels.Value.Channels {
				if ch.ChannelPoint ==
					s.channel.ChannelPoint {
					s.unavailable = false
					s.channel = ch
					break
				}
			}
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
		name = ch.RemotePubkey
		if len(name) > 16 {
			name = name[:16] + "..."
		}
	}
	p.title(theme.Header, name)

	status := theme.Success.Render("active")
	if !ch.Active {
		status = theme.Warning.Render("inactive")
	}
	if ch.Pending {
		status = theme.Dim.Render("pending")
	}
	if s.unavailable {
		status = theme.Warning.Render("unavailable; check pending channels and history")
	}

	if s.ctx.Status != nil && !s.ctx.Status.Channels.Fresh() {
		p.warn("Channel data stale; refresh unavailable.")
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

	// Only available open channels offer a close action.
	if !ch.Pending && !s.unavailable {
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
	if s.channel.Pending || s.unavailable {
		return viewDetailBindings(s.ctx.HasTabs)
	}
	return detailActionBindings(
		"close channel", s.viewBtnIdx, s.ctx.HasTabs)
}

// ── Close channel launch ───────────────────────────────

func (s *ChannelDetailScreen) launchClose() (
	Screen, tea.Cmd,
) {
	if s.unavailable || s.channel.Pending {
		return s, nil
	}
	s.closeScreen = NewChannelCloseScreen(
		s.ctx,
		s.channel.ChannelPoint,
		s.channel.PeerAlias,
		s.channel.Capacity,
		s.channel.LocalBalance)
	return s, closeFeeTiersCmd(s.closeScreen)
}
