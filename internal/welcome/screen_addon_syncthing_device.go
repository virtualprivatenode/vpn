package welcome

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

// ── SyncthingDeviceScreen ──────────────────────────────
// Device detail with Cancel / Remove buttons, plus
// confirm step. Snapshot data at construction time.

type syncDeviceStep int

const (
	syncDeviceStepDetail syncDeviceStep = iota
	syncDeviceStepConfirm
)

type SyncthingDeviceScreen struct {
	ctx         *ScreenContext
	step        syncDeviceStep
	device      config.SyncthingDevice // snapshot
	deviceIndex int                    // index in config
	viewBtnIdx  int                    // 0=Cancel, 1=Remove
	confirmIdx  int                    // 0=Go Back, 1=Remove
	removeError string
}

func NewSyncthingDeviceScreen(
	ctx *ScreenContext,
	device config.SyncthingDevice,
	index int,
) *SyncthingDeviceScreen {
	return &SyncthingDeviceScreen{
		ctx:         ctx,
		device:      device,
		deviceIndex: index,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *SyncthingDeviceScreen) Init() tea.Cmd {
	return nil
}

func (s *SyncthingDeviceScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case syncDeviceStepDetail:
		return s.handleDetailKey(keyStr)
	case syncDeviceStepConfirm:
		return s.handleConfirmKey(keyStr)
	}
	return s, nil
}

func (s *SyncthingDeviceScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case syncthingRemovedMsg:
		if msg.err != nil {
			s.removeError = msg.err.Error()
			s.step = syncDeviceStepDetail
			return s, nil
		}
		// Success — close tab (Model already
		// mutated config and adjusted cursor)
		return s, emitCloseTab
	}
	return s, nil
}

func (s *SyncthingDeviceScreen) View(
	w, h int,
) string {
	switch s.step {
	case syncDeviceStepDetail:
		return s.viewDetail(w, h)
	case syncDeviceStepConfirm:
		return s.viewConfirm(w, h)
	}
	return ""
}

func (s *SyncthingDeviceScreen) HelpBindings() []key.Binding {
	switch s.step {
	case syncDeviceStepDetail:
		return detailActionBindings(
			"remove", s.viewBtnIdx, s.ctx.HasTabs)
	case syncDeviceStepConfirm:
		return tabButtonBindings(s.ctx.HasTabs)
	}
	return nil
}

// ── Detail step ─────────────────────────────────────────
// Read-only info with Cancel / Remove buttons. Cancel
// closes the tab; Remove advances to confirm step.

func (s *SyncthingDeviceScreen) handleDetailKey(
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
		s.step = syncDeviceStepConfirm
		s.confirmIdx = 0
		s.removeError = ""
		return s, nil
	}
	return s, nil
}

func (s *SyncthingDeviceScreen) viewDetail(
	w, h int,
) string {
	dev := s.device
	p := newPane(w)
	p.title(theme.Header, dev.Name)

	p.labelLine("Device ID:")
	id := dev.DeviceID
	if len(id) > w-4 {
		id = id[:w-7] + "..."
	}
	p.mono(id)
	p.blank()
	p.field("Paired: ", dev.PairedAt)

	p.appendError(s.removeError)

	return p.renderWithBottomButtons(
		[]string{"Cancel", "Remove"}, s.viewBtnIdx,
		s.ctx.ContentFocused, h)
}

// ── Confirm step ────────────────────────────────────────

func (s *SyncthingDeviceScreen) handleConfirmKey(
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
		s.step = syncDeviceStepDetail
		return s, nil
	case "enter":
		switch s.confirmIdx {
		case 0: // Go Back
			s.step = syncDeviceStepDetail
			return s, nil
		case 1: // Remove
			return s, removeSyncthingDeviceCmd(
				s.device.DeviceID)
		}
	}
	return s, nil
}

func (s *SyncthingDeviceScreen) viewConfirm(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Warning,
		"Remove "+s.device.Name+"?")
	p.line(" " + theme.Value.Render(
		"• Stop syncing channel backups"+
			" to this device"))
	p.line(" " + theme.Value.Render(
		"• Remove device from Syncthing"))
	p.line(" " + theme.Value.Render(
		"• Does not delete data on the"+
			" remote device"))

	return p.renderWithBottomButtons(
		[]string{"Go Back", "Remove"},
		s.confirmIdx,
		s.ctx.ContentFocused, h)
}
