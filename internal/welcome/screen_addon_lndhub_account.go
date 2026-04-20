package welcome

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── LndHubAccountScreen ────────────────────────────────
// Account detail with optional Deactivate button.
// Snapshot data at construction time.

type hubAccountStep int

const (
	hubAcctStepDetail  hubAccountStep = iota
	hubAcctStepConfirm                // Go Back / Deactivate
)

type LndHubAccountScreen struct {
	ctx          *ScreenContext
	step         hubAccountStep
	account      config.LndHubAccount // snapshot
	accountIndex int                  // index in config
	viewBtnIdx   int                  // 0=Cancel, 1=Deactivate
	confirmIdx   int                  // 0=Go Back, 1=Deactivate
	deactError   string
}

func NewLndHubAccountScreen(
	ctx *ScreenContext,
	account config.LndHubAccount,
	index int,
) *LndHubAccountScreen {
	return &LndHubAccountScreen{
		ctx:          ctx,
		account:      account,
		accountIndex: index,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *LndHubAccountScreen) Init() tea.Cmd {
	return nil
}

func (s *LndHubAccountScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case hubAcctStepDetail:
		return s.handleDetailKey(keyStr)
	case hubAcctStepConfirm:
		return s.handleConfirmKey(keyStr)
	}
	return s, nil
}

func (s *LndHubAccountScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case lndhubDeactivatedMsg:
		if msg.err != nil {
			s.deactError = msg.err.Error()
			s.step = hubAcctStepDetail
			return s, nil
		}
		// Success — Model already mutated config.
		// Refresh the snapshot so view shows new state.
		if s.accountIndex <
			len(s.ctx.Cfg.LndHubAccounts) {
			s.account =
				s.ctx.Cfg.LndHubAccounts[s.accountIndex]
		}
		s.step = hubAcctStepDetail
		return s, nil
	}
	return s, nil
}

func (s *LndHubAccountScreen) View(
	w, h int,
) string {
	switch s.step {
	case hubAcctStepDetail:
		return s.viewDetail(w, h)
	case hubAcctStepConfirm:
		return s.viewConfirm(w, h)
	}
	return ""
}

func (s *LndHubAccountScreen) HelpBindings() []key.Binding {
	switch s.step {
	case hubAcctStepDetail:
		if s.account.Active {
			return detailActionBindings(
				"deactivate", s.viewBtnIdx,
				s.ctx.HasTabs)
		}
		return viewDetailBindings(s.ctx.HasTabs)
	case hubAcctStepConfirm:
		return tabButtonBindings(s.ctx.HasTabs)
	}
	return nil
}

// ── Detail step ────────────────────────────────────────

func (s *LndHubAccountScreen) handleDetailKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.account.Active && s.viewBtnIdx > 0 {
			s.viewBtnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.account.Active && s.viewBtnIdx < 1 {
			s.viewBtnIdx++
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
		if s.account.Active {
			if s.viewBtnIdx == 0 {
				return s, emitCloseTab
			}
			s.step = hubAcctStepConfirm
			s.confirmIdx = 0
			s.deactError = ""
			return s, nil
		}
		// Deactivated account: no action
		return s, nil
	}
	return s, nil
}

func (s *LndHubAccountScreen) viewDetail(
	w, h int,
) string {
	acct := s.account
	p := newPane(w)
	p.title(theme.Header, acct.Label)

	p.monoField("Login:   ", acct.Login)
	p.field("Created: ", acct.CreatedAt)

	if acct.Active {
		p.line(" " + theme.Label.Render("Status:  ") +
			theme.Success.Render("active"))
	} else {
		p.line(" " + theme.Label.Render("Status:  ") +
			theme.Warning.Render("deactivated"))
		if acct.DeactivatedAt != "" {
			p.field("Deactivated: ",
				acct.DeactivatedAt)
		}
		if acct.BalanceOnDeactivate != "" {
			p.field("Balance:     ",
				acct.BalanceOnDeactivate+" sats")
		}
	}

	p.appendError(s.deactError)

	if acct.Active {
		return p.renderWithBottomButtons(
			[]string{"Cancel", "Deactivate"},
			s.viewBtnIdx,
			s.ctx.ContentFocused, h)
	}

	return p.render()
}

// ── Confirm step ───────────────────────────────────────

func (s *LndHubAccountScreen) handleConfirmKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.confirmIdx > 0 {
			s.confirmIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.confirmIdx < 1 {
			s.confirmIdx++
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
		s.step = hubAcctStepDetail
		return s, nil
	case "enter":
		switch s.confirmIdx {
		case 0: // Go Back
			s.step = hubAcctStepDetail
			return s, nil
		case 1: // Deactivate
			return s, lndhubDeactivateWithLoginCmd(
				s.account.Login)
		}
	}
	return s, nil
}

func (s *LndHubAccountScreen) viewConfirm(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Warning,
		"Deactivate "+s.account.Label+"?")
	p.line(" " + theme.Value.Render(
		"• Block wallet access"))
	p.line(" " + theme.Value.Render(
		"• Record balance"))
	p.line(" " + theme.Value.Render(
		"• Login stops working"))

	return p.renderWithBottomButtons(
		[]string{"Go Back", "Deactivate"},
		s.confirmIdx,
		s.ctx.ContentFocused, h)
}
