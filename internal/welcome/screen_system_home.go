package welcome

import (
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/bitcoin"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── SystemHomeScreen ──────────────────────────────────
// Section home for System. Two focus zones: buttons
// and scrollable service list with hotkey actions.
//
// Buttons are dynamic — only actionable buttons appear:
//   Update Packages (always), SSH Keys (always),
//   Update Node (when available), Reboot (when required).
//
// Confirms are screen-owned: svcConfirm and sysConfirm
// live here. When active, they intercept all keys and
// show y/n prompt in the view and helpbar.

const (
	sysHomeZoneButtons  = 0
	sysHomeZoneServices = 1
)

// sysBtn identifies a logical button action independent
// of its position in the dynamic button bar.
type sysBtn int

const (
	sysBtnUpdatePkg sysBtn = iota
	sysBtnSSHKeys
	sysBtnUpdateNode
	sysBtnReboot
)

type SystemHomeScreen struct {
	ctx       *ScreenContext
	btnIdx    int
	focusZone int
	svcCursor int

	// Confirm dialogs — screen-owned
	svcConfirm string // "Restart", "Stop", "Start"
	sysConfirm string // "Update packages", "Reboot"

	// Background service action in progress
	svcPending  string // "restarting...", "stopping...", "starting..."
	pkgUpdating bool   // true while apt-get runs in background
}

func NewSystemHomeScreen(
	ctx *ScreenContext,
) *SystemHomeScreen {
	return &SystemHomeScreen{ctx: ctx}
}

// ── Screen interface ────────────────────────────────────

func (s *SystemHomeScreen) Init() tea.Cmd {
	return nil
}

func (s *SystemHomeScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	// Confirm dialogs intercept all keys
	if s.svcConfirm != "" {
		return s.handleSvcConfirm(keyStr)
	}
	if s.sysConfirm != "" {
		return s.handleSysConfirm(keyStr)
	}

	// Block service actions while one is pending
	if s.svcPending != "" {
		switch keyStr {
		case "r", "s", "a", "p", "u":
			return s, nil
		}
	}

	actions := s.buttonActions()
	maxBtn := len(actions) - 1
	if s.btnIdx > maxBtn {
		s.btnIdx = maxBtn
	}

	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.focusZone == sysHomeZoneButtons &&
			s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.focusZone == sysHomeZoneButtons &&
			s.btnIdx < maxBtn {
			s.btnIdx++
		}
		return s, nil
	case "up":
		if s.focusZone == sysHomeZoneServices {
			if s.svcCursor > 0 {
				s.svcCursor--
			} else {
				s.focusZone = sysHomeZoneButtons
				s.btnIdx = 0
			}
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down", "tab":
		if s.focusZone == sysHomeZoneButtons {
			if s.svcCount() > 0 {
				s.focusZone = sysHomeZoneServices
				s.svcCursor = 0
			}
			return s, nil
		}
		if s.focusZone == sysHomeZoneServices {
			if s.svcCursor < s.svcCount()-1 {
				s.svcCursor++
			}
		}
		return s, nil
	case "shift+tab":
		if s.focusZone == sysHomeZoneServices {
			s.focusZone = sysHomeZoneButtons
			s.btnIdx = 0
			return s, nil
		}
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "r":
		if s.focusZone == sysHomeZoneServices {
			s.svcConfirm = "Restart"
		}
		return s, nil
	case "s":
		if s.focusZone == sysHomeZoneServices {
			s.svcConfirm = "Stop"
		}
		return s, nil
	case "a":
		if s.focusZone == sysHomeZoneServices {
			s.svcConfirm = "Start"
		}
		return s, nil
	case "l":
		if s.focusZone == sysHomeZoneServices {
			svc := s.svcName(s.svcCursor)
			c := exec.Command("bash", "-c",
				"clear && sudo journalctl -u "+svc+
					" -n 100 --no-pager"+
					" && echo && echo "+
					"'  Press Enter to return...'"+
					" && read && clear")
			return s, tea.ExecProcess(c,
				func(err error) tea.Msg {
					return svcActionDoneMsg{}
				})
		}
		return s, nil
	case "p":
		if s.focusZone == sysHomeZoneServices &&
			s.svcName(s.svcCursor) == "lnd" &&
			s.ctx.Cfg.P2PMode == "tor" {
			screen := NewP2PUpgradeScreen(s.ctx)
			return s, func() tea.Msg {
				return openTabMsg{
					Kind:   tabP2PUpgrade,
					Label:  "P2P Upgrade",
					Screen: screen,
				}
			}
		}
		return s, nil
	case "u":
		if s.focusZone == sysHomeZoneServices &&
			s.svcName(s.svcCursor) == "lnd" &&
			s.ctx.Cfg.WalletExists() {
			screen := NewAutoUnlockScreen(s.ctx)
			label := "Auto-Unlock"
			if s.ctx.Cfg.AutoUnlock {
				label = "Disable Auto-Unlock"
			}
			return s, func() tea.Msg {
				return openTabMsg{
					Kind:   tabAutoUnlock,
					Label:  label,
					Screen: screen,
				}
			}
		}
		return s, nil
	case "backspace":
		return s, emitFocusSidebar
	case "enter":
		if s.focusZone == sysHomeZoneButtons &&
			s.btnIdx < len(actions) {
			switch actions[s.btnIdx] {
			case sysBtnUpdatePkg:
				if !s.pkgUpdating {
					s.sysConfirm = "Update packages"
				}
			case sysBtnSSHKeys:
				screen := NewSSHKeysScreen(s.ctx)
				return s, func() tea.Msg {
					return openTabMsg{
						Kind:   tabSSHKeys,
						Label:  "SSH Keys",
						Screen: screen,
					}
				}
			case sysBtnUpdateNode:
				screen := NewSelfUpdateScreen(
					s.ctx)
				return s, func() tea.Msg {
					return openTabMsg{
						Kind:   tabSelfUpdate,
						Label:  "Updating",
						Screen: screen,
					}
				}
			case sysBtnReboot:
				s.sysConfirm = "Reboot"
			}
		}
		return s, nil
	}
	return s, nil
}

// ── Confirm handlers ────────────────────────────────────

func (s *SystemHomeScreen) handleSvcConfirm(
	keyStr string,
) (Screen, tea.Cmd) {
	action := s.svcConfirm
	s.svcConfirm = ""
	if keyStr == "y" {
		svc := s.svcName(s.svcCursor)
		if svc != "" {
			switch action {
			case "Restart":
				s.svcPending = "restarting..."
			case "Stop":
				s.svcPending = "stopping..."
			case "Start":
				s.svcPending = "starting..."
			}
			return s, runSvcActionCmd(action, svc)
		}
	}
	return s, nil
}

func (s *SystemHomeScreen) handleSysConfirm(
	keyStr string,
) (Screen, tea.Cmd) {
	action := s.sysConfirm
	s.sysConfirm = ""
	if keyStr == "y" {
		if action == "Reboot" {
			return s, runRebootCmd()
		}
		s.pkgUpdating = true
		return s, runUpdatePackagesCmd()
	}
	return s, nil
}

func (s *SystemHomeScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg.(type) {
	case svcActionDoneMsg:
		s.svcPending = ""
	case pkgUpdateDoneMsg:
		s.pkgUpdating = false
	}
	return s, nil
}

// ── View ────────────────────────────────────────────────

func (s *SystemHomeScreen) View(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused
	cfg := s.ctx.Cfg
	status := s.ctx.Status

	boxW := w - 4
	if boxW < 30 {
		boxW = 30
	}

	border := theme.AddonBorderNormal
	titleStyle := lipgloss.NewStyle().
		Foreground(theme.ColorPrimary).
		Bold(true)

	// ── Fixed header (version + buttons) ─────────
	var headerLines []string
	headerLines = append(headerLines, "")

	verText := "Virtual Private Node v" +
		installer.GetVersion()
	headerLines = append(headerLines,
		centerPad(theme.Action.Render(verText), w))

	hasUpdate := s.hasUpdate()
	if hasUpdate {
		updateText := "Update available: v" +
			s.ctx.LatestVersion
		headerLines = append(headerLines,
			centerPad(
				lipgloss.NewStyle().
					Foreground(theme.ColorUpdate).
					Render(updateText), w))
	}

	headerLines = append(headerLines, "")

	btnLabels := s.buttonLabels()

	headerLines = append(headerLines,
		renderButtons(
			btnLabels, s.btnIdx,
			isFocused &&
				s.focusZone == sysHomeZoneButtons, w))
	headerLines = append(headerLines, "")

	if s.sysConfirm != "" {
		headerLines = append(headerLines,
			" "+theme.Warning.Render(
				fmt.Sprintf("%s? [y/n]",
					s.sysConfirm)))
		headerLines = append(headerLines, "")
	}

	header := strings.Join(headerLines, "\n")
	headerH := len(headerLines)

	// ── Scrollable middle (all cards) ────────────
	var midLines []string

	// Services card
	midLines = append(midLines,
		"  "+border.Render(
			"┌"+strings.Repeat("─", boxW-2)+"┐"))

	svcTitle := " Services"
	svcTitlePad := boxW - 2 - len(svcTitle)
	if svcTitlePad < 0 {
		svcTitlePad = 0
	}
	midLines = append(midLines,
		"  "+border.Render("│")+
			titleStyle.Render(svcTitle)+
			strings.Repeat(" ", svcTitlePad)+
			border.Render("│"))

	names := serviceNames(cfg)
	for i, name := range names {
		dot := theme.RedDot.Render("●")
		if status != nil {
			if active, ok :=
				status.services[name]; ok &&
				active {
				dot = theme.GreenDot.Render("●")
			}
		}

		isSelected := isFocused &&
			s.focusZone == sysHomeZoneServices &&
			s.svcCursor == i

		prefix := " "
		style := theme.Value
		if isSelected {
			prefix = "▸"
			style = theme.NavActive
		}

		svcLine := " " + prefix + " " + dot + " " +
			style.Render(name)

		if isSelected && s.svcPending != "" {
			svcLine += "  " +
				theme.Dim.Render(s.svcPending)
		} else if isSelected {
			// Standard service hints — dim
			hint := theme.Dim.Render(
				"  r restart  s stop  a start  l logs")
			// LND-specific destructive hotkeys —
			// the hotkey letter itself is rendered
			// in red so users see they're sensitive,
			// while the description stays dim.
			if name == "lnd" &&
				s.ctx.Cfg.P2PMode == "tor" {
				hint += "  " +
					theme.Warning.Render("p") +
					theme.Dim.Render(" p2p")
			}
			if name == "lnd" &&
				s.ctx.Cfg.WalletExists() {
				uLabel := " unlock"
				if s.ctx.Cfg.AutoUnlock {
					uLabel = " lock"
				}
				hint += "  " +
					theme.Warning.Render("u") +
					theme.Dim.Render(uLabel)
			}
			svcLine += hint
		}

		svcVis := lipgloss.Width(svcLine)
		svcPad := boxW - 2 - svcVis
		if svcPad < 0 {
			svcPad = 0
		}
		midLines = append(midLines,
			"  "+border.Render("│")+
				svcLine+
				strings.Repeat(" ", svcPad)+
				border.Render("│"))
	}

	if s.svcConfirm != "" {
		svc := s.svcName(s.svcCursor)
		confirmLine := " " + theme.Warning.Render(
			fmt.Sprintf(" %s %s? [y/n]",
				s.svcConfirm, svc))
		confirmVis := lipgloss.Width(confirmLine)
		confirmPad := boxW - 2 - confirmVis
		if confirmPad < 0 {
			confirmPad = 0
		}
		midLines = append(midLines,
			"  "+border.Render("│")+
				confirmLine+
				strings.Repeat(" ", confirmPad)+
				border.Render("│"))
	}

	midLines = append(midLines,
		"  "+border.Render(
			"└"+strings.Repeat("─", boxW-2)+"┘"))

	midLines = append(midLines, "")

	// Resources card
	midLines = append(midLines,
		"  "+border.Render(
			"┌"+strings.Repeat("─", boxW-2)+"┐"))

	resTitle := " Resources"
	resTitlePad := boxW - 2 - len(resTitle)
	if resTitlePad < 0 {
		resTitlePad = 0
	}
	midLines = append(midLines,
		"  "+border.Render("│")+
			titleStyle.Render(resTitle)+
			strings.Repeat(" ", resTitlePad)+
			border.Render("│"))

	if status != nil {
		resRows := []string{
			" " + theme.Label.Render("Disk: ") +
				theme.Value.Render(
					fmt.Sprintf("%s / %s (%s)",
						status.diskUsed,
						status.diskTotal,
						status.diskPct)),
			" " + theme.Label.Render("RAM:  ") +
				theme.Value.Render(
					fmt.Sprintf("%s / %s (%s)",
						status.ramUsed,
						status.ramTotal,
						status.ramPct)),
			" " + theme.Label.Render("BTC:  ") +
				theme.Value.Render(
					status.btcSize),
		}
		if cfg.HasLND() {
			resRows = append(resRows,
				" "+theme.Label.Render("LND:  ")+
					theme.Value.Render(
						status.lndSize))
		}
		if status.rebootRequired {
			resRows = append(resRows,
				" "+theme.Warning.Render(
					"⚠ Reboot required"))
		}

		for _, rl := range resRows {
			rlVis := lipgloss.Width(rl)
			rlPad := boxW - 2 - rlVis
			if rlPad < 0 {
				rlPad = 0
			}
			midLines = append(midLines,
				"  "+border.Render("│")+
					rl+
					strings.Repeat(" ", rlPad)+
					border.Render("│"))
		}
	} else {
		loadLine := " " +
			theme.Dim.Render(" Loading...")
		loadVis := lipgloss.Width(loadLine)
		loadPad := boxW - 2 - loadVis
		if loadPad < 0 {
			loadPad = 0
		}
		midLines = append(midLines,
			"  "+border.Render("│")+
				loadLine+
				strings.Repeat(" ", loadPad)+
				border.Render("│"))
	}

	midLines = append(midLines,
		"  "+border.Render(
			"└"+strings.Repeat("─", boxW-2)+"┘"))

	midLines = append(midLines, "")

	// Bitcoin card
	midLines = append(midLines,
		"  "+border.Render(
			"┌"+strings.Repeat("─", boxW-2)+"┐"))

	btcTitle := " ₿ Bitcoin"
	btcTitlePad := boxW - 2 -
		lipgloss.Width(btcTitle)
	if btcTitlePad < 0 {
		btcTitlePad = 0
	}
	midLines = append(midLines,
		"  "+border.Render("│")+
			theme.Bitcoin.Render(btcTitle)+
			strings.Repeat(" ", btcTitlePad)+
			border.Render("│"))

	if status == nil {
		btcLoad := " " +
			theme.Dim.Render(" Loading...")
		btcLoadVis := lipgloss.Width(btcLoad)
		btcLoadPad := boxW - 2 - btcLoadVis
		if btcLoadPad < 0 {
			btcLoadPad = 0
		}
		midLines = append(midLines,
			"  "+border.Render("│")+
				btcLoad+
				strings.Repeat(" ", btcLoadPad)+
				border.Render("│"))
	} else if !status.btcResponding {
		btcErr := " " +
			theme.Warn.Render(" Not responding")
		btcErrVis := lipgloss.Width(btcErr)
		btcErrPad := boxW - 2 - btcErrVis
		if btcErrPad < 0 {
			btcErrPad = 0
		}
		midLines = append(midLines,
			"  "+border.Render("│")+
				btcErr+
				strings.Repeat(" ", btcErrPad)+
				border.Render("│"))
	} else {
		var btcRows []string
		syncVal := theme.Good.Render("synced")
		if !status.btcSynced {
			syncVal = theme.Warn.Render("syncing")
		}
		btcRows = append(btcRows,
			" "+theme.Label.Render("Sync:     ")+
				syncVal)
		btcRows = append(btcRows,
			" "+theme.Label.Render("Height:   ")+
				theme.Value.Render(
					fmt.Sprintf("%d / %d",
						status.btcBlocks,
						status.btcHeaders)))
		if status.btcProgress > 0 {
			btcRows = append(btcRows,
				" "+theme.Label.Render("Progress: ")+
					theme.Value.Render(
						bitcoin.FormatProgress(
							status.btcProgress)))
		}
		btcRows = append(btcRows,
			" "+theme.Label.Render("Network:  ")+
				theme.Value.Render(cfg.Network))

		for _, bl := range btcRows {
			blVis := lipgloss.Width(bl)
			blPad := boxW - 2 - blVis
			if blPad < 0 {
				blPad = 0
			}
			midLines = append(midLines,
				"  "+border.Render("│")+
					bl+
					strings.Repeat(" ", blPad)+
					border.Render("│"))
		}
	}

	midLines = append(midLines,
		"  "+border.Render(
			"└"+strings.Repeat("─", boxW-2)+"┘"))

	midContent := strings.Join(midLines, "\n")

	// ── Viewport ─────────────────────────────────
	vpH := h - headerH
	if vpH < 1 {
		vpH = 1
	}

	// Services start at line 2 (top border + title)
	cursorLine := 2 + s.svcCursor

	vpRendered := renderViewport(
		midContent, w, vpH, cursorLine,
		len(midLines),
		s.focusZone == sysHomeZoneServices &&
			len(names) > 0)

	// ── Assemble output ──────────────────────────
	return header + "\n" + vpRendered
}

// ── HelpBindings ────────────────────────────────────────

func (s *SystemHomeScreen) HelpBindings() []key.Binding {
	if s.svcConfirm != "" || s.sysConfirm != "" {
		return confirmDialogBindings()
	}
	if s.focusZone == sysHomeZoneServices {
		return s.serviceBindings()
	}
	return homeButtonBindings(
		"services", s.btnIdx, s.ctx.HasTabs)
}

func (s *SystemHomeScreen) serviceBindings() []key.Binding {
	binds := []key.Binding{
		bind("↑↓", "services", "up", "down"),
		bind("r", "restart", "r"),
		bind("s", "stop", "s"),
		bind("a", "start", "a"),
		bind("l", "logs", "l"),
	}
	if s.svcName(s.svcCursor) == "lnd" &&
		s.ctx.Cfg.P2PMode == "tor" {
		binds = append(binds,
			bind("p", "p2p", "p"))
	}
	if s.svcName(s.svcCursor) == "lnd" &&
		s.ctx.Cfg.WalletExists() {
		uHelp := "unlock"
		if s.ctx.Cfg.AutoUnlock {
			uHelp = "lock"
		}
		binds = append(binds,
			bind("u", uHelp, "u"))
	}
	binds = append(binds, kShiftTabButtons, kSidebar, kBack, kQuit)
	return binds
}

// ── Helpers ─────────────────────────────────────────────

func (s *SystemHomeScreen) hasUpdate() bool {
	return s.ctx.LatestVersion != "" &&
		s.ctx.LatestVersion != s.ctx.Version
}

func (s *SystemHomeScreen) buttonActions() []sysBtn {
	actions := []sysBtn{sysBtnUpdatePkg, sysBtnSSHKeys}
	if s.hasUpdate() {
		actions = append(actions, sysBtnUpdateNode)
	}
	if s.ctx.Status != nil &&
		s.ctx.Status.rebootRequired {
		actions = append(actions, sysBtnReboot)
	}
	return actions
}

var sysBtnLabel = map[sysBtn]string{
	sysBtnUpdatePkg:  "Update Packages",
	sysBtnSSHKeys:    "SSH Keys",
	sysBtnUpdateNode: "Update Node",
	sysBtnReboot:     "Reboot",
}

func (s *SystemHomeScreen) buttonLabels() []string {
	actions := s.buttonActions()
	labels := make([]string, len(actions))
	for i, a := range actions {
		labels[i] = sysBtnLabel[a]
		if a == sysBtnUpdatePkg && s.pkgUpdating {
			labels[i] = "Updating..."
		}
	}
	return labels
}

func (s *SystemHomeScreen) svcCount() int {
	return len(serviceNames(s.ctx.Cfg))
}

func (s *SystemHomeScreen) svcName(i int) string {
	names := serviceNames(s.ctx.Cfg)
	if i < len(names) {
		return names[i]
	}
	return ""
}
