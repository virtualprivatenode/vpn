package welcome

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── OCReceiveScreen ────────────────────────────────────
// Displays a fresh on-chain address. Two buttons:
// "New Address" (generates another) and "Show QR"
// (opens fullscreen QR overlay).

type ocRecvStep int

const (
	ocRecvWaiting ocRecvStep = iota // fetching address
	ocRecvReady                     // address visible
)

type OCReceiveScreen struct {
	ctx     *ScreenContext
	step    ocRecvStep
	address string
	errMsg  string
	btnIdx  int // 0=New Address, 1=Show QR
}

func NewOCReceiveScreen(
	ctx *ScreenContext,
) *OCReceiveScreen {
	return &OCReceiveScreen{
		ctx:  ctx,
		step: ocRecvWaiting,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *OCReceiveScreen) Init() tea.Cmd {
	return getNewAddressCmd(s.ctx.LndClient)
}

func (s *OCReceiveScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.step == ocRecvReady && s.btnIdx < 1 {
			s.btnIdx++
		}
		return s, nil
	case "up", "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		return s, nil
	case "backspace":
		return s, emitFocusParent
	case "enter":
		if s.step != ocRecvReady {
			return s, nil
		}
		switch s.btnIdx {
		case 0: // Show QR
			if s.address == "" {
				return s, nil
			}
			addr := s.address
			return s, func() tea.Msg {
				return showQRMsg{
					URL:   addr,
					Label: "On-Chain Address",
				}
			}
		case 1: // New Address
			s.address = ""
			s.errMsg = ""
			s.step = ocRecvWaiting
			return s, getNewAddressCmd(s.ctx.LndClient)
		}
		return s, nil
	}
	return s, nil
}

func (s *OCReceiveScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case newAddressMsg:
		if msg.err != nil {
			s.errMsg = msg.err.Error()
			return s, nil
		}
		s.address = msg.address
		s.step = ocRecvReady
	}
	return s, nil
}

func (s *OCReceiveScreen) View(
	w, h int,
) string {
	if s.step == ocRecvWaiting {
		return s.viewWaiting(w, h)
	}
	return s.viewReady(w, h)
}

func (s *OCReceiveScreen) viewWaiting(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "⛓ Receive On-Chain")
	p.dim("Generating address...")

	if s.errMsg != "" {
		p.blank()
		p.appendError(s.errMsg)
	}

	return p.renderWithBottomButtons(
		[]string{"Generating..."}, 0, false, h)
}

func (s *OCReceiveScreen) viewReady(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "⛓ Receive On-Chain")

	p.labelLine("Address:")
	p.monoWrap(s.address)
	p.blank()
	p.dim("Send Bitcoin to this address.")
	p.dim("Funds appear after 1 confirmation.")

	if s.ctx.Status != nil && !s.ctx.Status.btcSynced {
		p.blank()
		p.line(" " + theme.Warn.Render(
			"Funds will not appear until IBD is complete."))
	}

	if s.errMsg != "" {
		p.blank()
		p.appendError(s.errMsg)
	}

	return p.renderWithBottomButtons(
		[]string{"Show QR", "New Address"},
		s.btnIdx,
		s.ctx.ContentFocused, h)
}

func (s *OCReceiveScreen) HelpBindings() []key.Binding {
	if s.step == ocRecvReady {
		return tabButtonBindings(s.ctx.HasTabs)
	}
	binds := []key.Binding{kSidebar}
	if s.ctx.HasTabs {
		binds = append(binds, kShiftTabBar)
	}
	binds = append(binds, kQuit)
	return binds
}
