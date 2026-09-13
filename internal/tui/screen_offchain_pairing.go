package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ── PairingScreen ──────────────────────────────────────
// Zeus wallet pairing: shows LND REST connection info
// with QR (Tor), Macaroon, and optional QR (Clearnet)
// buttons.
//
// The REST onion address is live-read at screen entry (no
// stored copy exists anywhere) and held only for this
// screen's lifetime.

type PairingScreen struct {
	ctx       *ScreenContext
	btnIdx    int
	restOnion string
	fetched   bool // the live read answered
	fetchErr  bool // ...with an error (already logged)
}

func NewPairingScreen(
	ctx *ScreenContext,
) *PairingScreen {
	return &PairingScreen{
		ctx: ctx,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *PairingScreen) Init() tea.Cmd {
	return fetchNodeAddressesCmd(tabPairing)
}

func (s *PairingScreen) maxBtn() int {
	return len(s.buttons()) - 1
}

func (s *PairingScreen) HandleKey(
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
		if s.btnIdx < s.maxBtn() {
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
		return s.handleEnter()
	}
	return s, nil
}

func (s *PairingScreen) buttons() []string {
	btns := []string{"Show QR (Tor)"}
	if s.ctx.Cfg.P2PMode == "hybrid" {
		btns = append(btns, "Show QR (Clearnet)")
	}
	btns = append(btns, "Copyable Macaroon")
	return btns
}

func (s *PairingScreen) handleEnter() (Screen, tea.Cmd) {
	btns := s.buttons()
	if s.btnIdx < 0 || s.btnIdx >= len(btns) {
		return s, nil
	}
	switch btns[s.btnIdx] {
	case "Show QR (Tor)":
		restOnion := s.restOnion
		mac := readMacaroonHex()
		if restOnion != "" && mac != "" {
			url := fmt.Sprintf(
				"lndconnect://%s:8080?macaroon=%s",
				restOnion, hexToBase64URL(mac))
			return s, func() tea.Msg {
				return showQRMsg{
					URL:   url,
					Label: "LND Connect — Tor",
				}
			}
		}
	case "Show QR (Clearnet)":
		if s.ctx.Cfg.P2PMode == "hybrid" &&
			s.ctx.Status != nil &&
			s.ctx.Status.PublicIP.Fresh() && s.ctx.Status.PublicIP.Value != "" {
			mac := readMacaroonHex()
			if mac != "" {
				url := fmt.Sprintf(
					"lndconnect://%s:8080"+
						"?macaroon=%s",
					s.ctx.Status.PublicIP.Value,
					hexToBase64URL(mac))
				return s, func() tea.Msg {
					return showQRMsg{
						URL:   url,
						Label: "LND Connect — Clearnet",
					}
				}
			}
		}
	case "Copyable Macaroon":
		return s, showMacaroonCmd()
	}
	return s, nil
}

func (s *PairingScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tabActivatedMsg:
		// Re-entering the tab re-asks: screen entry is the
		// cadence at which live-read facts are read.
		return s, fetchNodeAddressesCmd(tabPairing)
	case nodeAddressesMsg:
		s.restOnion = msg.addrs.LNDRESTOnion
		s.fetched = true
		s.fetchErr = msg.err != nil
	}
	return s, nil
}

func (s *PairingScreen) View(
	w, h int,
) string {
	cfg := s.ctx.Cfg
	status := s.ctx.Status

	if !cfg.HasLND() || !s.ctx.walletExists() {
		p := newPane(w)
		p.title(theme.Lightning, "⚡ Zeus Wallet")
		p.dim("Create LND wallet first")
		return p.renderWithBottomButtons(
			[]string{"Done"}, 0, false, h)
	}

	if status == nil || !status.Node.Fresh() {
		p := newPane(w)
		p.title(theme.Lightning, "⚡ Zeus Wallet")
		p.dim("Waiting for LND...")
		return p.renderWithBottomButtons(
			[]string{"Waiting..."}, 0, false, h)
	}

	p := newPane(w)
	p.title(theme.Lightning, "⚡ Zeus — LND REST")

	restOnion := s.restOnion

	if cfg.P2PMode == "hybrid" {
		p.line(" " + theme.Header.Render(
			"Clearnet"))
		if status.PublicIP.Value != "" {
			p.labelLine("Server:")
			p.monoWrap(observationText(status.PublicIP, status.PublicIP.Value))
			p.blank()
			p.labelLine("Port:")
			p.monoWrap("8080")
		} else {
			p.dim("Server address unavailable.")
		}
		p.blank()
		p.line(" " + theme.Header.Render("Tor"))
	}

	if restOnion == "" {
		switch {
		case !s.fetched:
			p.dim("Reading the node's Tor address...")
		case s.fetchErr:
			p.warn("Cannot read the node's Tor address — " +
				"check: journalctl -u vpn-helperd")
		default:
			p.warn("Tor not available")
		}
	} else {
		p.labelLine("Server:")
		p.monoWrap(restOnion)
		p.blank()
		p.labelLine("Port:")
		p.monoWrap("8080")
	}

	mac := readMacaroonHex()
	if mac != "" {
		p.blank()
		preview := mac[:min(24, len(mac))] + "..."
		p.labelLine("Macaroon:")
		p.monoWrap(preview)
	}

	return p.renderWithBottomButtons(
		s.buttons(), s.btnIdx,
		s.ctx.ContentFocused, h)
}

func (s *PairingScreen) HelpBindings() []key.Binding {
	return tabButtonBindings(s.ctx.HasTabs)
}
