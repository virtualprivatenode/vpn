package welcome

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/installer"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ── SSHKeyDetailScreen ─────────────────────────────────
// Key detail with Cancel / Remove buttons + confirm step.
// Opened as its own tab from SSHKeysScreen list. Mirrors
// SyncthingDeviceScreen (detail + confirm in-screen, close
// the tab on remove success).

type sshKeyDetailStep int

const (
	sshKeyDetailStepView sshKeyDetailStep = iota
	sshKeyDetailStepConfirm
	sshKeyDetailStepWorking
)

type SSHKeyDetailScreen struct {
	ctx        *ScreenContext
	step       sshKeyDetailStep
	keyInfo    installer.SSHKeyInfo
	keyCount   int // snapshot at open time, for warning threshold
	viewBtnIdx int // 0=Cancel, 1=Remove
	confirmIdx int
	removeErr  string
}

func NewSSHKeyDetailScreen(
	ctx *ScreenContext,
	k installer.SSHKeyInfo,
	keyCount int,
) *SSHKeyDetailScreen {
	return &SSHKeyDetailScreen{
		ctx:      ctx,
		keyInfo:  k,
		keyCount: keyCount,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *SSHKeyDetailScreen) Init() tea.Cmd { return nil }

func (s *SSHKeyDetailScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case sshKeyDetailStepView:
		return s.handleViewKey(keyStr)
	case sshKeyDetailStepConfirm:
		return s.handleConfirmKey(keyStr)
	case sshKeyDetailStepWorking:
		if keyStr == "ctrl+c" {
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s *SSHKeyDetailScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case sshKeyRemoveMsg:
		if msg.err != nil {
			s.removeErr = msg.err.Error()
			s.step = sshKeyDetailStepView
			return s, nil
		}
		// Success — close this tab and refresh the
		// parent SSH Keys list.
		return s, tea.Batch(emitCloseTab, listSSHKeysCmd())
	}
	return s, nil
}

func (s *SSHKeyDetailScreen) View(w, h int) string {
	switch s.step {
	case sshKeyDetailStepView:
		return s.viewDetail(w, h)
	case sshKeyDetailStepConfirm:
		return s.viewConfirm(w, h)
	case sshKeyDetailStepWorking:
		return s.viewWorking(w, h)
	}
	return ""
}

func (s *SSHKeyDetailScreen) HelpBindings() []key.Binding {
	switch s.step {
	case sshKeyDetailStepView:
		return detailActionBindings(
			"remove", s.viewBtnIdx, s.ctx.HasTabs)
	case sshKeyDetailStepConfirm:
		return tabButtonBindings(s.ctx.HasTabs)
	case sshKeyDetailStepWorking:
		return waitingBindings()
	}
	return nil
}

// ── View step ───────────────────────────────────────────

func (s *SSHKeyDetailScreen) handleViewKey(
	keyStr string,
) (Screen, tea.Cmd) {
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
		return s, nil
	case "backspace":
		return s, emitFocusParent
	case "enter":
		if s.viewBtnIdx == 0 {
			return s, emitCloseTab
		}
		s.step = sshKeyDetailStepConfirm
		s.confirmIdx = 0
		s.removeErr = ""
		return s, nil
	}
	return s, nil
}

func (s *SSHKeyDetailScreen) viewDetail(
	w, h int,
) string {
	k := s.keyInfo
	p := newPane(w)

	comment := k.Comment
	if comment == "" {
		comment = "(no comment)"
	}
	p.title(theme.Header, comment)

	p.field("Type:        ", k.Type)
	p.labelLine("Fingerprint:")
	p.monoWrap(k.Fingerprint)
	if k.Comment != "" {
		p.field("Comment:     ", k.Comment)
	}

	p.appendError(s.removeErr)

	return p.renderWithBottomButtons(
		[]string{"Cancel", "Remove"}, s.viewBtnIdx,
		s.ctx.ContentFocused, h)
}

// ── Confirm step ────────────────────────────────────────

func (s *SSHKeyDetailScreen) handleConfirmKey(
	keyStr string,
) (Screen, tea.Cmd) {
	// Hard block: only Go Back is reachable. Clamp
	// confirmIdx and refuse right-arrow movement.
	passwordAuthEnabled :=
		!s.ctx.Cfg.SSHPasswordAuthDisabled
	hardBlock := s.keyCount <= 1 && !passwordAuthEnabled
	maxIdx := 1
	if hardBlock {
		maxIdx = 0
		s.confirmIdx = 0
	}

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
		if s.confirmIdx < maxIdx {
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
		s.step = sshKeyDetailStepView
		return s, nil
	case "enter":
		switch s.confirmIdx {
		case 0: // Go Back
			s.step = sshKeyDetailStepView
			return s, nil
		case 1: // Remove
			s.step = sshKeyDetailStepWorking
			return s, removeSSHKeyCmd(
				s.keyInfo.Fingerprint)
		}
	}
	return s, nil
}

func (s *SSHKeyDetailScreen) viewConfirm(
	w, h int,
) string {
	p := newPane(w)

	comment := s.keyInfo.Comment
	if comment == "" {
		comment = "this key"
	}
	p.title(theme.Warning, "Remove "+comment+"?")
	p.blank()

	p.field("Type:        ", s.keyInfo.Type)
	p.labelLine("Fingerprint:")
	p.monoWrap(s.keyInfo.Fingerprint)
	if s.keyInfo.Comment != "" {
		p.field("Comment:     ", s.keyInfo.Comment)
	}

	p.blank()

	// Lockout copy is driven by this app's recorded
	// setting, which can diverge from sshd's effective
	// config — so the copy below never asserts the
	// live state as fact. The authoritative gate is in
	// the installer: removing the LAST key triggers a
	// live query of sshd's effective config, and the
	// removal is refused unless password login is
	// actually available. Invariant: never let the
	// system end up with zero auth methods.
	passwordAuthEnabled :=
		!s.ctx.Cfg.SSHPasswordAuthDisabled
	isLastKey := s.keyCount <= 1
	hardBlock := isLastKey && !passwordAuthEnabled

	switch {
	case hardBlock:
		p.warn("This is your only key, and this app's")
		p.warn("settings say password auth is disabled.")
		p.warn("Removing it could lock you out.")
		p.warn("Re-enable password auth first.")
		p.blank()
	case isLastKey:
		p.warn("This is your only SSH key. Removal is")
		p.warn("only permitted if password login is")
		p.warn("actually available — the system checks")
		p.warn("the live sshd configuration and")
		p.warn("refuses otherwise.")
		p.blank()
	}
	// Under a hard block there is no Remove button, so
	// don't ask a question the operator cannot answer.
	if hardBlock {
		p.warn("Removal is blocked.")
	} else {
		p.warn("Remove this key?")
	}

	// When hard-blocked, show only Go Back (no Remove).
	buttons := []string{"Go Back", "Remove"}
	confirmIdx := s.confirmIdx
	if hardBlock {
		buttons = []string{"Go Back"}
		confirmIdx = 0
	}
	return p.renderWithBottomButtons(
		buttons, confirmIdx,
		s.ctx.ContentFocused, h)
}

// ── Working step ────────────────────────────────────────

func (s *SSHKeyDetailScreen) viewWorking(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Removing key")
	p.blank()
	p.line(" " + theme.Value.Render("Working..."))
	return p.renderWithBottomButtons(
		[]string{"Working..."}, 0, false, h)
}
