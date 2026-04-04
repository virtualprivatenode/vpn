package welcome

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── OCReceiveScreen ────────────────────────────────────
// Displays a fresh on-chain address with inline QR code.
// Single button: "New Address" (ready) / "Generating..."
// (waiting).

type ocRecvStep int

const (
	ocRecvWaiting ocRecvStep = iota // fetching address
	ocRecvReady                     // address + QR visible
)

type OCReceiveScreen struct {
	ctx     *ScreenContext
	step    ocRecvStep
	address string
	errMsg  string
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
		if s.step == ocRecvReady {
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
			"Bitcoin Core is syncing. Funds will not"))
		p.line(" " + theme.Warn.Render(
			"appear until sync is complete."))
	}

	p.blank()

	qr := renderQRCode(s.address)
	if qr != "" {
		for _, line := range strings.Split(
			qr, "\n") {
			lineW := lipgloss.Width(line)
			pad := (w - lineW) / 2
			if pad < 0 {
				pad = 0
			}
			p.line(strings.Repeat(" ", pad) + line)
		}
	}

	if s.errMsg != "" {
		p.blank()
		p.appendError(s.errMsg)
	}

	return p.renderWithBottomButtons(
		[]string{"New Address"}, 0,
		s.ctx.ContentFocused, h)
}

func (s *OCReceiveScreen) HelpBindings() []key.Binding {
	var binds []key.Binding

	if s.step == ocRecvReady {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "new addr")))
	}

	binds = append(binds, kSidebar)

	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("shift+tab"),
				key.WithHelp("⇧tab", "tab bar")))
	}

	binds = append(binds, kQuit)
	return binds
}
