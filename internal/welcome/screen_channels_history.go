package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/lndrpc"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ── ChannelHistoryScreen ───────────────────────────────
// Scrollable table of closed channels.

type ChannelHistoryScreen struct {
	ctx     *ScreenContext
	entries []channelHistoryEntry
	cursor  int
}

func NewChannelHistoryScreen(
	ctx *ScreenContext,
	entries []channelHistoryEntry,
) *ChannelHistoryScreen {
	return &ChannelHistoryScreen{
		ctx:     ctx,
		entries: entries,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *ChannelHistoryScreen) Init() tea.Cmd {
	return nil
}

func (s *ChannelHistoryScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		return s, emitFocusSidebar
	case "up":
		if s.cursor > 0 {
			s.cursor--
		} else if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
	case "down", "tab":
		if s.cursor < len(s.entries)-1 {
			s.cursor++
		}
	case "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
	case "backspace":
		return s, emitFocusParent
	}
	return s, nil
}

func (s *ChannelHistoryScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tabActivatedMsg:
		// Re-fetch closed channels so the table
		// reflects any changes since this tab was
		// last viewed (e.g. a channel closed from
		// the detail tab). The closedChannelsMsg
		// handler below rebuilds entries from
		// current status + fresh closed data.
		return s, fetchClosedChannelsCmd(
			s.ctx.LndClient)
	case closedChannelsMsg:
		if msg.err == nil {
			// Rebuild entries with current channel
			// data from status + closed channels
			var channels []channelInfo
			var waiting []lndrpc.WaitingCloseChannel
			var pending []lndrpc.PendingForceCloseChannel
			if s.ctx.Status != nil {
				channels = s.ctx.Status.channels
				waiting =
					s.ctx.Status.waitingCloseChannels
				pending =
					s.ctx.Status.pendingForceCloseChannels
			}
			s.entries = buildChannelHistoryEntries(
				channels, waiting, pending,
				msg.channels)
		}
	}
	return s, nil
}

func (s *ChannelHistoryScreen) View(
	w, h int,
) string {
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render("Channel History"),
			w))
	headerLines = append(headerLines, "")

	if len(s.entries) == 0 {
		headerLines = append(headerLines,
			" "+theme.Dim.Render(
				"No channel history."))
		return strings.Join(headerLines, "\n")
	}

	isFocused := s.ctx.ContentFocused

	hdrStyle := theme.TableHeader
	sepStyle := theme.TableDim

	peerW := 16
	capW := 10
	statusW := 14
	closeW := w - peerW - capW - statusW - 5
	if closeW < 8 {
		closeW = 8
	}

	hdr := " " +
		hdrStyle.Render(pad("Peer", peerW)) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", capW, "Capacity")) +
		hdrStyle.Render(
			pad("  Status", statusW)) +
		hdrStyle.Render(
			fmt.Sprintf("%*s", closeW, "Close"))
	headerLines = append(headerLines, hdr)
	headerLines = append(headerLines,
		" "+sepStyle.Render(
			strings.Repeat("─", w-2)))

	header := strings.Join(headerLines, "\n")
	headerH := len(headerLines)

	// ── Scrollable rows ──────────────────────────
	var midLines []string

	selStyle := theme.NavActive

	for i, ch := range s.entries {
		isSelected := isFocused &&
			s.cursor == i

		peer := ch.PeerAlias
		if peer == "" {
			if len(ch.RemotePubkey) > 12 {
				peer = ch.RemotePubkey[:12] + ".."
			} else {
				peer = ch.RemotePubkey
			}
		}
		if len(peer) > peerW-1 {
			peer = peer[:peerW-2] + ".."
		}
		peerStr := pad(peer, peerW)

		capStr := fmt.Sprintf("%*s", capW,
			formatSatsCompact(ch.Capacity))

		statusStr := pad("  "+ch.Status, statusW)
		closeLabel := ch.CloseType
		if ch.Status == "waiting close" {
			closeLabel = "unconfirmed"
		} else if ch.BlocksRemaining > 0 {
			closeLabel = fmt.Sprintf("~%d blks",
				ch.BlocksRemaining)
		}
		closeStr := fmt.Sprintf("%*s",
			closeW, closeLabel)

		marker := " "
		if isSelected {
			marker = theme.NavActive.Render("▸")
			midLines = append(midLines,
				marker+
					selStyle.Render(peerStr)+
					selStyle.Render(capStr)+
					selStyle.Render(statusStr)+
					selStyle.Render(closeStr))
		} else {
			var statusRendered string
			switch ch.Status {
			case "active":
				statusRendered =
					theme.Success.Render(statusStr)
			case "pending close", "waiting close":
				statusRendered =
					theme.Warning.Render(statusStr)
			case "force close":
				statusRendered =
					theme.Warning.Render(statusStr)
			case "closed":
				statusRendered =
					theme.Dim.Render(statusStr)
			default:
				statusRendered =
					theme.Value.Render(statusStr)
			}

			var closeRendered string
			switch ch.CloseType {
			case "force":
				closeRendered =
					theme.Warning.Render(closeStr)
			default:
				closeRendered =
					theme.Dim.Render(closeStr)
			}

			midLines = append(midLines,
				marker+
					theme.Value.Render(peerStr)+
					theme.Value.Render(capStr)+
					statusRendered+
					closeRendered)
		}
	}

	midContent := strings.Join(midLines, "\n")

	vpH := h - headerH
	if vpH < 1 {
		vpH = 1
	}

	vpRendered := renderViewport(
		midContent, w, vpH, s.cursor,
		len(midLines),
		len(s.entries) > 0 && isFocused)

	return header + "\n" + vpRendered
}

func (s *ChannelHistoryScreen) HelpBindings() []key.Binding {
	binds := []key.Binding{kUpDownChannels, kSidebar}
	if s.ctx.HasTabs {
		binds = append(binds, kShiftTabBar)
	}
	binds = append(binds, kQuit)
	return binds
}
