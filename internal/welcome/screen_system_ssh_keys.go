package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── SSHKeysScreen ──────────────────────────────────────
// Pure list/management screen. Add and per-key detail
// are opened as their own tabs (tabSSHKeyAdd,
// tabSSHKeyDetail). Mirrors the Syncthing manage pattern.
//
// authorized_keys is small and lives on local disk, so we
// read it synchronously in the constructor — no async
// load, no first-paint bounce. Refreshes after Add/Remove
// arrive via sshKeysListMsg from the child screens.

// ── Focus zones ────────────────────────────────────────

const (
	sshZoneButtons = 0
	sshZoneKeys    = 1
)

// ── Messages ───────────────────────────────────────────

// sshKeysListMsg is a refresh signal emitted by child
// screens (Add, Detail) after a successful mutation.
// The initial list is read synchronously in the
// constructor — this msg is only for re-syncing after
// the file has changed.
type sshKeysListMsg struct {
	keys []installer.SSHKeyInfo
	err  error
}

type sshKeyAddMsg struct{ err error }

type sshKeyRemoveMsg struct{ err error }

// ── Commands ───────────────────────────────────────────

func listSSHKeysCmd() tea.Cmd {
	return func() tea.Msg {
		keys, err := installer.ListAuthorizedKeys()
		return sshKeysListMsg{keys: keys, err: err}
	}
}

func addSSHKeyCmd(line string) tea.Cmd {
	return func() tea.Msg {
		return sshKeyAddMsg{
			err: installer.AppendAuthorizedKey(line)}
	}
}

func removeSSHKeyCmd(
	fingerprint string, passwordAuthEnabled bool,
) tea.Cmd {
	return func() tea.Msg {
		return sshKeyRemoveMsg{
			err: installer.RemoveAuthorizedKey(
				fingerprint, passwordAuthEnabled)}
	}
}

// ── Screen ─────────────────────────────────────────────

type SSHKeysScreen struct {
	ctx *ScreenContext

	keys      []installer.SSHKeyInfo
	keyCursor int
	loadErr   error
	focusZone int
	btnIdx    int
}

func NewSSHKeysScreen(
	ctx *ScreenContext,
) *SSHKeysScreen {
	keys, err := installer.ListAuthorizedKeys()
	return &SSHKeysScreen{
		ctx:     ctx,
		keys:    keys,
		loadErr: err,
	}
}

func (s *SSHKeysScreen) Init() tea.Cmd { return nil }

func (s *SSHKeysScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	s.clampCursor()

	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit

	case "left":
		if s.focusZone == sshZoneButtons &&
			s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		return s, emitFocusSidebar

	case "right":
		if s.focusZone == sshZoneButtons &&
			s.btnIdx < 2 {
			s.btnIdx++
		}
		return s, nil

	case "up":
		if s.focusZone == sshZoneKeys {
			if s.keyCursor > 0 {
				s.keyCursor--
			} else {
				s.focusZone = sshZoneButtons
				s.btnIdx = 0
			}
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "down", "tab":
		if s.focusZone == sshZoneButtons {
			if len(s.keys) > 0 {
				s.focusZone = sshZoneKeys
				s.keyCursor = 0
			}
			return s, nil
		}
		if s.focusZone == sshZoneKeys {
			if s.keyCursor < len(s.keys)-1 {
				s.keyCursor++
			}
		}
		return s, nil

	case "shift+tab":
		if s.focusZone == sshZoneKeys {
			s.focusZone = sshZoneButtons
			s.btnIdx = 0
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil

	case "backspace":
		return s, emitFocusParent

	case "enter":
		if s.focusZone == sshZoneButtons {
			switch s.btnIdx {
			case 0:
				return s.openAddTab()
			case 1:
				return s.openPasswordAuthTab()
			case 2:
				return s.openChangePasswordTab()
			}
			return s, nil
		}
		return s.openDetailTab()
	}
	return s, nil
}

func (s *SSHKeysScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case sshKeysListMsg:
		// Refresh from child-screen success. Don't
		// touch focusZone — the user's cursor stays
		// where they left it.
		s.keys = msg.keys
		s.loadErr = msg.err
		s.clampCursor()
		return s, nil
	}
	return s, nil
}

func (s *SSHKeysScreen) View(w, h int) string {
	return s.viewList(w, h)
}

func (s *SSHKeysScreen) HelpBindings() []key.Binding {
	if s.focusZone == sshZoneButtons {
		return manageButtonBindings(
			"keys", s.btnIdx, s.ctx.HasTabs)
	}
	return homeListBindings(
		"navigate", "open", "buttons")
}

// ── Tab openers ────────────────────────────────────────

func (s *SSHKeysScreen) openAddTab() (Screen, tea.Cmd) {
	screen := NewSSHKeyAddScreen(s.ctx)
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabSSHKeyAdd,
			Label:  "Add Key",
			Screen: screen,
			Parent: tabSSHKeys,
		}
	}
}

func (s *SSHKeysScreen) openPasswordAuthTab() (
	Screen, tea.Cmd,
) {
	screen := NewSSHPasswordAuthScreen(s.ctx)
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabSSHPasswordAuth,
			Label:  "Password Auth",
			Screen: screen,
			Parent: tabSSHKeys,
		}
	}
}

func (s *SSHKeysScreen) openChangePasswordTab() (
	Screen, tea.Cmd,
) {
	screen := NewChangePasswordScreen(s.ctx)
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabSSHChangePassword,
			Label:  "Change Password",
			Screen: screen,
			Parent: tabSSHKeys,
		}
	}
}

func (s *SSHKeysScreen) openDetailTab() (Screen, tea.Cmd) {
	if s.keyCursor >= len(s.keys) {
		return s, nil
	}
	k := s.keys[s.keyCursor]
	label := k.Comment
	if label == "" {
		label = k.Type
	}
	if len(label) > 17 {
		label = label[:17] + "..."
	}
	screen := NewSSHKeyDetailScreen(
		s.ctx, k, len(s.keys))
	idx := s.keyCursor + 1
	return s, func() tea.Msg {
		return openTabMsg{
			Kind:   tabSSHKeyDetail,
			Label:  label,
			Index:  idx,
			Screen: screen,
			Parent: tabSSHKeys,
		}
	}
}

// ── View ──────────────────────────────────────────────

func (s *SSHKeysScreen) viewList(w, h int) string {
	s.clampCursor()
	isFocused := s.ctx.ContentFocused
	onButtons := isFocused &&
		s.focusZone == sshZoneButtons

	// ── Fixed header: title + button ────────────
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render("SSH Keys"), w))
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		renderButtons(
			[]string{
				"Add Key",
				"Password Auth",
				"Change Password",
			},
			s.btnIdx, onButtons, w))
	headerLines = append(headerLines, "")

	// Status line: shows the actual SSH password auth
	// state. Mirrors what's surfaced in the Password
	// Auth screen.
	pwAuthLabel := theme.Success.Render("enabled")
	if s.ctx.Cfg.SSHPasswordAuthDisabled {
		pwAuthLabel = theme.Warning.Render("disabled")
	}
	headerLines = append(headerLines,
		" "+theme.Label.Render("Password Auth: ")+
			pwAuthLabel)
	headerLines = append(headerLines, "")

	headerH := len(headerLines)
	header := strings.Join(headerLines, "\n")

	// ── Scrollable body ────────────────────────
	cursorLine := 0
	var midLines []string

	if s.loadErr != nil {
		midLines = append(midLines,
			" "+theme.Warning.Render(
				s.loadErr.Error()))
	} else if len(s.keys) == 0 {
		midLines = append(midLines,
			" "+theme.Dim.Render(
				"No authorized keys found"))
		midLines = append(midLines, "")
		midLines = append(midLines,
			" "+theme.Dim.Render(
				"Press enter to add a key"))
	} else {
		midLines = append(midLines,
			" "+theme.Label.Render(fmt.Sprintf(
				"Authorized Keys (%d)",
				len(s.keys))))
		midLines = append(midLines, "")

		hdrStyle := theme.TableHeader
		sepStyle := theme.TableDim

		typeW := 16
		fpW := 28
		commentW := w - typeW - fpW - 4
		if commentW < 10 {
			commentW = 10
		}

		hdr := " " +
			hdrStyle.Render(pad("Type", typeW)) +
			hdrStyle.Render(
				pad("Fingerprint", fpW)) +
			hdrStyle.Render(
				fmt.Sprintf("%-*s", commentW,
					"Comment"))
		midLines = append(midLines, hdr)
		midLines = append(midLines,
			" "+sepStyle.Render(
				strings.Repeat("─", w-2)))

		onList := isFocused &&
			s.focusZone == sshZoneKeys
		selStyle := theme.NavActive

		for i, k := range s.keys {
			keyType := k.Type
			if len(keyType) > typeW-1 {
				keyType = keyType[:typeW-2] + ".."
			}
			typeStr := pad(keyType, typeW)

			fp := k.Fingerprint
			if len(fp) > fpW-1 {
				fp = fp[:fpW-4] + "..."
			}
			fpStr := pad(fp, fpW)

			comment := k.Comment
			if comment == "" {
				comment = "(no comment)"
			}
			if len(comment) > commentW-1 {
				comment = comment[:commentW-4] +
					"..."
			}
			commentStr := fmt.Sprintf("%-*s",
				commentW, comment)

			isSelected := onList && s.keyCursor == i

			marker := " "
			if isSelected {
				marker = theme.NavActive.Render("▸")
				cursorLine = len(midLines)
				midLines = append(midLines,
					marker+
						selStyle.Render(typeStr)+
						selStyle.Render(fpStr)+
						selStyle.Render(
							commentStr))
			} else {
				midLines = append(midLines,
					marker+
						theme.Value.Render(
							typeStr)+
						theme.Dim.Render(fpStr)+
						theme.Dim.Render(
							commentStr))
			}
		}
	}

	midContent := strings.Join(midLines, "\n")

	// ── Viewport ───────────────────────────────
	vpH := h - headerH
	if vpH < 1 {
		vpH = 1
	}

	vpRendered := renderViewport(
		midContent, w, vpH, cursorLine,
		len(midLines),
		isFocused &&
			s.focusZone == sshZoneKeys)

	return header + "\n" + vpRendered
}

// ── Helpers ───────────────────────────────────────────

func (s *SSHKeysScreen) clampCursor() {
	if s.keyCursor >= len(s.keys) {
		s.keyCursor = len(s.keys) - 1
	}
	if s.keyCursor < 0 {
		s.keyCursor = 0
	}
	if len(s.keys) == 0 &&
		s.focusZone == sshZoneKeys {
		s.focusZone = sshZoneButtons
		s.btnIdx = 0
	}
}
