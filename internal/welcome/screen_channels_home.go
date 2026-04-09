package welcome

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── ChannelsHomeScreen ─────────────────────────────────
// Section home for Channels. Two focus zones: buttons
// (Open Channel, Node Info, History) and scrollable
// channel list. Reads live data through ctx.Status
// pointer — no snapshot, since the home screen persists
// for the lifetime of the program and must always show
// current data.

const (
	chanHomeZoneButtons = 0
	chanHomeZoneList    = 1
)

type ChannelsHomeScreen struct {
	ctx       *ScreenContext
	btnIdx    int // 0=Open Channel, 1=Node Info, 2=History
	focusZone int // 0=buttons, 1=channel list
	cursor    int // position in channel list

	// Zero balance interstitial
	zeroBalanceMsg bool
	fundBtnIdx     int // 0=Fund Wallet, 1=Go Back
}

func NewChannelsHomeScreen(
	ctx *ScreenContext,
) *ChannelsHomeScreen {
	return &ChannelsHomeScreen{
		ctx: ctx,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *ChannelsHomeScreen) Init() tea.Cmd {
	return nil
}

func (s *ChannelsHomeScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	// Zero-balance interstitial intercepts all keys
	if s.zeroBalanceMsg {
		return s.handleZeroBalanceKey(keyStr)
	}

	channels := s.channels()
	s.clampCursor()

	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.focusZone == chanHomeZoneButtons &&
			s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.focusZone == chanHomeZoneButtons &&
			s.btnIdx < 2 {
			s.btnIdx++
		}
		return s, nil
	case "up":
		if s.focusZone == chanHomeZoneList {
			if s.cursor > 0 {
				s.cursor--
			} else {
				s.focusZone = chanHomeZoneButtons
				s.btnIdx = 0
			}
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		if s.focusZone == chanHomeZoneButtons {
			if len(channels) > 0 {
				s.focusZone = chanHomeZoneList
				s.cursor = 0
			}
			return s, nil
		}
		if s.focusZone == chanHomeZoneList {
			if s.cursor < len(channels)-1 {
				s.cursor++
			}
		}
		return s, nil
	case "shift+tab":
		if s.focusZone == chanHomeZoneList {
			s.focusZone = chanHomeZoneButtons
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

func (s *ChannelsHomeScreen) handleEnter() (
	Screen, tea.Cmd,
) {
	if s.focusZone == chanHomeZoneButtons {
		switch s.btnIdx {
		case 0: // Open Channel
			return s.openChannel()
		case 1: // Node Info
			return s.openNodeInfo()
		case 2: // History
			return s.openHistory()
		}
		return s, nil
	}

	// Channel list — open channel detail
	channels := s.channels()
	if s.cursor < len(channels) &&
		!channels[s.cursor].Pending {
		ch := channels[s.cursor]
		label := ch.PeerAlias
		if label == "" {
			label = ch.RemotePubkey[:12] + ".."
		}
		if len(label) > 17 {
			label = label[:17] + "..."
		}
		screen := NewChannelDetailScreen(
			s.ctx, ch, s.feeTiers())
		idx := s.cursor
		return s, func() tea.Msg {
			return openTabMsg{
				Kind:        tabChannel,
				Label:       label,
				Index:       idx,
				Screen:      screen,
				FocusTabBar: true,
			}
		}
	}
	return s, nil
}

func (s *ChannelsHomeScreen) openChannel() (
	Screen, tea.Cmd,
) {
	cfg := s.ctx.Cfg
	if s.ctx.LndClient == nil || !cfg.HasLND() ||
		!cfg.WalletExists() {
		return s, nil
	}
	// Zero balance: show educational message
	if s.ctx.Status != nil &&
		s.ctx.Status.lndBalance == "0" {
		s.zeroBalanceMsg = true
		s.fundBtnIdx = 0
		return s, nil
	}
	// Normal path: channel open
	screen := NewChannelOpenScreen(s.ctx)
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabOpenChannel,
			Label:  "Open Channel",
			Screen: screen,
		}
	}
}

func (s *ChannelsHomeScreen) openNodeInfo() (
	Screen, tea.Cmd,
) {
	screen := NewNodeInfoScreen(s.ctx)
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabNodeInfo,
			Label:  "Node Info",
			Screen: screen,
		}
	}
}

func (s *ChannelsHomeScreen) openHistory() (
	Screen, tea.Cmd,
) {
	screen := NewChannelHistoryScreen(
		s.ctx, nil) // entries populated by closedChannelsMsg
	openCmd := func() tea.Msg {
		return openTabMsg{
			Kind:   tabChannelHistory,
			Label:  "History",
			Screen: screen,
		}
	}
	return s, tea.Batch(openCmd,
		fetchClosedChannelsCmd(s.ctx.LndClient))
}

func (s *ChannelsHomeScreen) handleZeroBalanceKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.fundBtnIdx > 0 {
			s.fundBtnIdx--
		}
		return s, nil
	case "right":
		if s.fundBtnIdx < 1 {
			s.fundBtnIdx++
		}
		return s, nil
	case "backspace":
		s.zeroBalanceMsg = false
		return s, nil
	case "enter":
		if s.fundBtnIdx == 0 {
			// Fund Wallet → open on-chain receive
			s.zeroBalanceMsg = false
			screen := NewOCReceiveScreen(s.ctx)
			return s, func() tea.Msg {
				return openTabMsg{
					Kind:   tabOCReceive,
					Label:  "⛓ Receive",
					Screen: screen,
				}
			}
		}
		// Go Back
		s.zeroBalanceMsg = false
		return s, nil
	}
	return s, nil
}

func (s *ChannelsHomeScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	return s, nil
}

// ── View ────────────────────────────────────────────────

func (s *ChannelsHomeScreen) View(
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

	// Zero-balance educational message
	if s.zeroBalanceMsg {
		p := newPane(w)
		p.blank()
		p.line(centerPad(
			theme.Header.Render("Fund Your Wallet"), w))
		p.blank()
		p.dim("  Opening a Lightning channel requires")
		p.dim("  on-chain Bitcoin. Send Bitcoin to your")
		p.dim("  on-chain address first, then return")
		p.dim("  here to open a channel.")
		if !status.btcSynced {
			p.blank()
			p.line("  " + theme.Warn.Render(
				"Bitcoin Core is syncing. If you have"))
			p.line("  " + theme.Warn.Render(
				"already sent funds, they will appear"))
			p.line("  " + theme.Warn.Render(
				"once sync is complete."))
		}
		p.blank()
		p.line(renderButtons(
			[]string{"Fund Wallet", "Go Back"},
			s.fundBtnIdx, true, w))
		return p.render()
	}

	isFocused := s.ctx.ContentFocused
	channels := status.channels

	// ── Fixed header ─────────────────────────────
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render(
				"Lightning Channels Dashboard"),
			w))
	headerLines = append(headerLines, "")

	// P2P Mode sits above the balance group. The
	// balanceSummaryLines helper begins its output
	// with a blank row (row 0 of leftLines pairs with
	// the box top border on the right), so no explicit
	// separator is needed here — the caller-side blank
	// plus the helper's blank would produce two rows
	// of gap, which is too much.
	headerLines = append(headerLines,
		" "+theme.Label.Render("P2P Mode: ")+
			theme.Value.Render(
				p2pModeLabel(cfg.P2PMode)))

	headerLines = append(headerLines,
		balanceSummaryLines(status, w)...)

	headerLines = append(headerLines, "")
	headerLines = append(headerLines, "")

	header := strings.Join(headerLines, "\n")
	headerH := len(headerLines)

	// ── Buttons (below balance summary) ─────────
	isOnButton := isFocused &&
		s.focusZone == chanHomeZoneButtons
	var btnLines []string
	btnLines = append(btnLines,
		renderButtons(
			[]string{
				"Open Channel",
				"Node Info",
				"History",
			},
			s.btnIdx, isOnButton, w))
	btnLines = append(btnLines, "")
	btnLines = append(btnLines, "")

	btnContent := strings.Join(btnLines, "\n")
	btnH := len(btnLines)

	// ── Scrollable middle (all channel bars) ─────
	chanCount := len(channels)
	nameW := 17
	barW := w - nameW - 22
	if barW < 8 {
		barW = 8
	}

	var midLines []string

	if chanCount == 0 {
		midLines = append(midLines,
			theme.Dim.Render(" No channels yet."))
	} else {
		for i, ch := range channels {
			if i > 0 {
				midLines = append(midLines, "")
			}

			isSelected := isFocused &&
				s.cursor == i &&
				s.focusZone == chanHomeZoneList

			name := ch.PeerAlias
			if name == "" {
				name = ch.RemotePubkey
			}
			if len(name) > nameW {
				name = name[:nameW-3] + "..."
			}

			dot := theme.RedDot.Render("○")
			if ch.Active {
				dot = theme.GreenDot.Render("●")
			}
			if ch.Pending {
				dot = theme.Dim.Render("◌")
			}

			localFill := 0
			if ch.Capacity > 0 {
				localFill = int(
					float64(ch.LocalBalance) /
						float64(ch.Capacity) *
						float64(barW))
			}
			if localFill > barW {
				localFill = barW
			}
			remoteFill := barW - localFill

			var lColor, rColor color.Color
			if isSelected {
				lColor = theme.ColorChanLocalActive
				rColor = theme.ColorChanRemoteActive
			} else if ch.Active {
				lColor = theme.ColorChanLocal
				rColor = theme.ColorChanRemote
			} else {
				lColor = theme.ColorChanLocalDim
				rColor = theme.ColorChanRemoteDim
			}

			lBar := lipgloss.NewStyle().
				Foreground(lColor).
				Render(
					strings.Repeat("█", localFill))
			rBar := lipgloss.NewStyle().
				Foreground(rColor).
				Render(strings.Repeat("█",
					remoteFill))
			barStr := lBar + rBar

			vals := fmt.Sprintf("%s / %s",
				formatSatsCompact(ch.LocalBalance),
				formatSatsCompact(ch.RemoteBalance))
			valsPad := pad(vals, 14)

			marker := " "
			nameStyle := theme.Value
			if isSelected {
				marker = "▸"
				nameStyle = theme.NavActive
			}
			namePad := pad(name, nameW)

			line := marker + " " + dot + " " +
				nameStyle.Render(namePad) + " " +
				barStr + " " +
				theme.Dim.Render(valsPad)

			midLines = append(midLines, line)
		}

		if status.pendingOpen > 0 {
			midLines = append(midLines, "")
			midLines = append(midLines,
				" "+theme.Dim.Render(
					fmt.Sprintf("%d pending",
						status.pendingOpen)))
		}
	}

	midContent := strings.Join(midLines, "\n")

	// ── Viewport ─────────────────────────────────
	vpH := h - headerH - btnH
	if vpH < 1 {
		vpH = 1
	}

	// Each channel is 2 lines (bar + blank gap)
	// except last which is 1 line
	cursorLine := s.cursor * 2

	vpRendered := renderViewport(
		midContent, w, vpH, cursorLine,
		len(midLines),
		chanCount > 0 &&
			s.focusZone == chanHomeZoneList)

	// ── Assemble output ──────────────────────────
	return header + "\n" + btnContent + "\n" + vpRendered
}

// ── HelpBindings ────────────────────────────────────────

func (s *ChannelsHomeScreen) HelpBindings() []key.Binding {
	if !s.ctx.Cfg.WalletExists() {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "create wallet")),
			kSidebar,
			kQuit,
		}
	}
	if s.zeroBalanceMsg {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("left", "right"),
				key.WithHelp("←→", "buttons")),
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "select")),
			key.NewBinding(
				key.WithKeys("backspace"),
				key.WithHelp("⌫", "back")),
			kQuit,
		}
	}
	if s.focusZone == chanHomeZoneList {
		return s.listBindings()
	}
	return s.buttonBindings()
}

func (s *ChannelsHomeScreen) buttonBindings() []key.Binding {
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
			key.WithHelp("↓", "channels")),
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

func (s *ChannelsHomeScreen) listBindings() []key.Binding {
	binds := []key.Binding{
		key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "channels")),
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

func (s *ChannelsHomeScreen) channels() []channelInfo {
	if s.ctx.Status == nil {
		return nil
	}
	return s.ctx.Status.channels
}

func (s *ChannelsHomeScreen) feeTiers() [4]feeTier {
	// Fee tiers are stored on OnChainContext but the
	// screen doesn't have access to it. Return zero
	// tiers — ChannelDetailScreen will receive them
	// via feeTiersMsg routed by Model.
	return [4]feeTier{}
}

func (s *ChannelsHomeScreen) clampCursor() {
	channels := s.channels()
	if len(channels) == 0 {
		s.cursor = 0
		return
	}
	if s.cursor >= len(channels) {
		s.cursor = len(channels) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
}
