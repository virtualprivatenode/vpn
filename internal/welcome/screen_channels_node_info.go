package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ── NodeInfoScreen ──────────────────────────────────────
// Displays the node's public identity — alias, pubkey,
// P2P mode, inbound liquidity — with Show QR buttons
// (reusing showQRMsg like the off-chain receive flow)
// and a Copy URIs button that hands the terminal to a
// shell displaying all advertised URIs for native
// terminal copy-paste (reusing the showInvoiceCmd
// pattern from update.go).
//
// Reads live data through ctx.Status so the displayed
// values stay in sync with LND's current state.
//
// Node URIs are NOT rendered in the TUI body. They're
// too long to fit cleanly at the 67-char pane width
// (a Tor URI wraps awkwardly mid-hostname) and the
// shell Copy URIs view is strictly better for the
// actual use case of "hand this to a peer". The TUI
// body shows identity, the shell view shows URIs.
//
// Button row is dynamic based on which URI types LND
// advertises:
//   - 0 URIs:     (no buttons) + warning text
//   - clearnet:   [ Show QR (Clearnet) ] [ Copy URIs ]
//   - tor:        [ Show QR (Tor) ] [ Copy URIs ]
//   - both:       [ Show QR (Clearnet) ] [ Show QR (Tor) ] [ Copy URIs ]
//
// There is deliberately no Done button. Node Info is a
// view-only informational screen like channel-history
// or channel-detail, not a flow — the user exits via
// the tab bar (up arrow → close), the sidebar (left
// arrow), backspace (focus parent tab), or by
// switching tabs.
//
// QR buttons are always type-labeled even when there's
// only one URI, because sovereignty-focused users want
// explicit confirmation of which network advertisement
// they're handing to a peer.
//
// The clearnet/Tor classification is by substring check
// on ".onion:" — any URI containing that marker is Tor,
// anything else is treated as clearnet. Matches the
// inline pattern used in screen_channels_open.go.

type NodeInfoScreen struct {
	ctx       *ScreenContext
	buttonIdx int
}

func NewNodeInfoScreen(
	ctx *ScreenContext,
) *NodeInfoScreen {
	return &NodeInfoScreen{ctx: ctx}
}

// ── Screen interface ────────────────────────────────────

func (s *NodeInfoScreen) Init() tea.Cmd {
	return nil
}

// isTorURI classifies a node URI as Tor or clearnet by
// looking for the ".onion:" substring. Matches the
// inline pattern used elsewhere in the welcome package.
func isTorURI(uri string) bool {
	return strings.Contains(uri, ".onion:")
}

// classifyURIs splits the URI list into clearnet-first
// and tor-second order, matching LND's typical
// advertisement order. Returns the split lists so the
// screen can label buttons and QRs independently.
func classifyURIs(
	uris []string,
) (clearnet, tor []string) {
	for _, u := range uris {
		if isTorURI(u) {
			tor = append(tor, u)
		} else {
			clearnet = append(clearnet, u)
		}
	}
	return
}

// buttons returns the current dynamic button labels
// in the order they render. Re-computed each time
// instead of cached, because URIs can change across
// status ticks (e.g. if P2P mode changes under us).
//
// When there are no URIs, returns an empty slice —
// the button row will not render, and the warn text
// in the view body explains the degraded state. The
// user still navigates away via left/up arrows.
func (s *NodeInfoScreen) buttons() []string {
	if s.ctx.Status == nil {
		return nil
	}
	clearnet, tor := classifyURIs(s.ctx.Status.lndURIs)
	var out []string
	if len(clearnet) > 0 {
		out = append(out, "Show QR (Clearnet)")
	}
	if len(tor) > 0 {
		out = append(out, "Show QR (Tor)")
	}
	if len(s.ctx.Status.lndURIs) > 0 {
		out = append(out, "Copy URIs")
	}
	return out
}

// buttonAction returns the tea.Cmd for the button at
// the given index in the current buttons() slice, or
// nil if the index is out of range / the action has no
// effect.
func (s *NodeInfoScreen) buttonAction(
	idx int,
) tea.Cmd {
	if s.ctx.Status == nil {
		return nil
	}
	clearnet, tor := classifyURIs(s.ctx.Status.lndURIs)
	buttons := s.buttons()
	if idx < 0 || idx >= len(buttons) {
		return nil
	}
	label := buttons[idx]
	switch label {
	case "Show QR (Clearnet)":
		if len(clearnet) == 0 {
			return nil
		}
		uri := clearnet[0]
		return func() tea.Msg {
			return showQRMsg{
				URL:   uri,
				Label: "Node URI (Clearnet)",
			}
		}
	case "Show QR (Tor)":
		if len(tor) == 0 {
			return nil
		}
		uri := tor[0]
		return func() tea.Msg {
			return showQRMsg{
				URL:   uri,
				Label: "Node URI (Tor)",
			}
		}
	case "Copy URIs":
		return showNodeURIsCmd(s.ctx.Status.lndURIs)
	}
	return nil
}

func (s *NodeInfoScreen) maxBtn() int {
	return len(s.buttons()) - 1
}

func (s *NodeInfoScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.buttonIdx > 0 {
			s.buttonIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.buttonIdx < s.maxBtn() {
			s.buttonIdx++
		}
		return s, nil
	case "up":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		return s, nil
	case "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "backspace":
		return s, emitFocusParent
	case "enter":
		return s, s.buttonAction(s.buttonIdx)
	}
	return s, nil
}

func (s *NodeInfoScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	// No async messages routed to this screen —
	// live data comes through ctx.Status pointer
	// updates done by Model on statusMsg.
	//
	// No defensive clamp on buttonIdx either. If the
	// button count changes under us (user toggles
	// P2P mode, URI list shifts), the three consumers
	// of buttonIdx — HandleKey's left/right bounds
	// checks and buttonAction's range check — are all
	// independently range-safe. Worst case is a stale
	// buttonIdx that points past the new end; enter
	// does nothing, the user presses left, the
	// highlight snaps back into range. Same pattern as
	// PairingScreen.
	return s, nil
}

func (s *NodeInfoScreen) View(w, h int) string {
	status := s.ctx.Status
	cfg := s.ctx.Cfg

	p := newPane(w)
	p.title(theme.Header, "Node Info")

	if status == nil || !status.lndResponding {
		p.dim("Waiting for LND...")
		// No buttons — user navigates away via arrow
		// keys like any other non-flow screen.
		return p.renderWithBottomButtons(
			nil, 0, s.ctx.ContentFocused, h)
	}

	// ── Pubkey ────────────────────────────────────
	// The pubkey's first char aligns with the
	// "Pubkey:" label's P via p.mono's standard
	// 1-char leading space. A 66-char pubkey plus
	// that space fills the 67-char pane exactly, so
	// the right edge touches the pane border with no
	// margin. That is a deliberate trade for label
	// alignment over symmetric breathing room.
	p.labelLine("Pubkey:")
	if status.lndPubkey != "" {
		p.mono(status.lndPubkey)
	} else {
		p.dim("(unavailable)")
	}
	p.blank()

	// labelW is the width of the longest label in
	// the aligned field groups below ("Outbound
	// Liquidity:" at 19 chars) plus 1 char of gap
	// between the colon and the value.
	const labelW = 20

	// ── Identity group ────────────────────────────
	alias := status.lndAlias
	if alias == "" {
		alias = "(none)"
	}
	p.fieldAligned("Alias:", alias, labelW)
	p.fieldAligned("P2P Mode:",
		p2pModeLabel(cfg.P2PMode), labelW)
	if status.lndVersion != "" {
		p.fieldAligned("LND Version:",
			status.lndVersion, labelW)
	}
	p.blank()

	// ── Network presence group ────────────────────
	// Node Capacity is the sum of channel capacities
	// (static total), distinct from
	// Outbound+Inbound (dynamic split minus reserves
	// and commitment fees).
	var totalCap, totalLocal, totalRemote int64
	activeCount := 0
	for _, ch := range status.channels {
		if ch.Pending {
			continue
		}
		totalCap += ch.Capacity
		totalLocal += ch.LocalBalance
		totalRemote += ch.RemoteBalance
		if ch.Active {
			activeCount++
		}
	}
	p.fieldAligned("Peers:",
		fmt.Sprintf("%d", status.lndPeers), labelW)
	p.fieldAligned("Active Channels:",
		fmt.Sprintf("%d", activeCount), labelW)
	p.fieldAligned("Node Capacity:",
		fmt.Sprintf("%s sats", formatSats(totalCap)),
		labelW)
	p.blank()

	// ── Balances group ────────────────────────────
	// Total Spendable is Outbound + On-Chain — what
	// the user can actually send right now.
	// Inbound is deliberately excluded because it's
	// not the user's money; it's the remote side's
	// liquidity that can be received into.
	onchainStr := "0"
	if status.lndBalance != "" {
		onchainStr = status.lndBalance
	}
	onChain := parseBalance(onchainStr)
	totalSpendable := totalLocal + onChain
	p.fieldAligned("Outbound Liquidity:",
		fmt.Sprintf("%s sats", formatSats(totalLocal)),
		labelW)
	p.fieldAligned("Inbound Liquidity:",
		fmt.Sprintf("%s sats", formatSats(totalRemote)),
		labelW)
	p.fieldAligned("On-Chain Balance:",
		fmt.Sprintf("%s sats", formatSats(onChain)),
		labelW)
	p.fieldAligned("Total Spendable:",
		fmt.Sprintf("%s sats",
			formatSats(totalSpendable)),
		labelW)
	p.blank()

	// ── URI status ────────────────────────────────
	// Node URIs themselves aren't rendered in the
	// TUI body — they live in the Copy URIs shell
	// view where the terminal can display them
	// cleanly and the user can select with native
	// mouse selection. Here we either explain how to
	// access them or warn that none are advertised.
	if len(status.lndURIs) == 0 {
		p.warn(
			"No URIs advertised by LND — configure")
		p.warn(
			"listen addresses to make your node")
		p.warn("reachable.")
	} else {
		p.dim(
			"Press Copy URIs to view your node URIs")
		p.dim(
			"in a shell for easy copy-paste, or Show")
		p.dim(
			"QR to scan one into another device.")
	}

	// ── Buttons pinned to bottom ──────────────────
	return p.renderWithBottomButtons(
		s.buttons(), s.buttonIdx,
		s.ctx.ContentFocused, h)
}

func (s *NodeInfoScreen) HelpBindings() []key.Binding {
	binds := buttonNav(s.buttonIdx)
	binds = append(binds, kEnter)
	if s.ctx.HasTabs {
		binds = append(binds, kShiftTabBar)
	}
	binds = append(binds, kQuit)
	return binds
}
