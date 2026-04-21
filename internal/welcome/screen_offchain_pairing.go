package welcome

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── PairingScreen ──────────────────────────────────────
// Zeus wallet pairing: shows LND REST connection info
// with QR (Tor), Macaroon, and optional QR (Clearnet)
// buttons.

type PairingScreen struct {
	ctx    *ScreenContext
	btnIdx int
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
	return nil
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
		restOnion := readOnion(
			paths.TorLNDRESTHostname)
		mac := readMacaroonHex(s.ctx.Cfg)
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
			s.ctx.Status.publicIP != "" {
			mac := readMacaroonHex(s.ctx.Cfg)
			if mac != "" {
				url := fmt.Sprintf(
					"lndconnect://%s:8080"+
						"?macaroon=%s",
					s.ctx.Status.publicIP,
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
		return s, showMacaroonCmd(s.ctx.Cfg)
	}
	return s, nil
}

func (s *PairingScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	return s, nil
}

func (s *PairingScreen) View(
	w, h int,
) string {
	cfg := s.ctx.Cfg
	status := s.ctx.Status

	if !cfg.HasLND() || !cfg.WalletExists() {
		p := newPane(w)
		p.title(theme.Lightning, "⚡ Zeus Wallet")
		p.dim("Create LND wallet first")
		return p.renderWithBottomButtons(
			[]string{"Done"}, 0, false, h)
	}

	if status == nil || !status.lndResponding {
		p := newPane(w)
		p.title(theme.Lightning, "⚡ Zeus Wallet")
		p.dim("Waiting for LND...")
		return p.renderWithBottomButtons(
			[]string{"Waiting..."}, 0, false, h)
	}

	p := newPane(w)
	p.title(theme.Lightning, "⚡ Zeus — LND REST")

	restOnion := readOnion(paths.TorLNDRESTHostname)

	if cfg.P2PMode == "hybrid" {
		p.line(" " + theme.Header.Render(
			"Clearnet"))
		if status.publicIP != "" {
			p.labelLine("Server:")
			p.monoWrap(status.publicIP)
			p.blank()
			p.labelLine("Port:")
			p.monoWrap("8080")
		}
		p.blank()
		p.line(" " + theme.Header.Render("Tor"))
	}

	if restOnion == "" {
		p.warn("Tor not available")
	} else {
		p.labelLine("Server:")
		p.monoWrap(restOnion)
		p.blank()
		p.labelLine("Port:")
		p.monoWrap("8080")
	}

	mac := readMacaroonHex(cfg)
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
