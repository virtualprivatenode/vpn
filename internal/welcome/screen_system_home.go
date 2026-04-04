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
// (Update Packages, Update Node, Reboot) and scrollable
// service list with hotkey actions (r/s/a/l).
//
// Confirms are screen-owned: svcConfirm, sysConfirm,
// and updateConfirm all live here. When active, they
// intercept all keys and show y/n prompt in the view
// and helpbar.

const (
	sysHomeZoneButtons  = 0
	sysHomeZoneServices = 1
)

type SystemHomeScreen struct {
	ctx       *ScreenContext
	btnIdx    int // 0=Update Packages, 1=Update Node, 2=Reboot
	focusZone int
	svcCursor int

	// Confirm dialogs — screen-owned
	svcConfirm    string // "Restart", "Stop", "Start"
	sysConfirm    string // "Update packages", "Reboot"
	updateConfirm bool   // Update Node confirm
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
	if s.updateConfirm {
		return s.handleUpdateConfirm(keyStr)
	}

	hasUpdate := s.hasUpdate()
	maxBtn := 1
	if s.ctx.Status != nil &&
		s.ctx.Status.rebootRequired {
		maxBtn = 2
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
	case "backspace":
		return s, emitFocusSidebar
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
					" && read")
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
			return s, func() tea.Msg {
				return shellActionMsg{
					action: svP2PUpgrade,
				}
			}
		}
		return s, nil
	case "enter":
		if s.focusZone == sysHomeZoneButtons {
			switch s.btnIdx {
			case 0:
				s.sysConfirm = "Update packages"
			case 1:
				if hasUpdate {
					s.updateConfirm = true
				}
			case 2:
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
		return s, runUpdatePackagesCmd()
	}
	return s, nil
}

func (s *SystemHomeScreen) handleUpdateConfirm(
	keyStr string,
) (Screen, tea.Cmd) {
	s.updateConfirm = false
	if keyStr == "y" {
		return s, func() tea.Msg {
			return shellActionMsg{
				action: svSelfUpdate,
			}
		}
	}
	return s, nil
}

func (s *SystemHomeScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
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
		centerPad(
			lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorAccent).
				Render(verText), w))

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

	btnLabels := []string{"Update Packages"}
	if hasUpdate {
		btnLabels = append(btnLabels, "Update Node")
	} else {
		btnLabels = append(btnLabels, "Up to Date")
	}
	if status != nil && status.rebootRequired {
		btnLabels = append(btnLabels, "Reboot")
	}

	headerLines = append(headerLines,
		renderButtonsWithGray(
			btnLabels, s.btnIdx,
			isFocused &&
				s.focusZone == sysHomeZoneButtons, w,
			1, !hasUpdate))
	headerLines = append(headerLines, "")

	if s.updateConfirm {
		headerLines = append(headerLines,
			" "+theme.Warning.Render(
				"Update to v"+
					s.ctx.LatestVersion+
					"? [y/n]"))
		headerLines = append(headerLines, "")
	}

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

		if isSelected {
			hint := theme.Dim.Render(
				"  r restart  s stop  a start  l logs")
			if name == "lnd" &&
				s.ctx.Cfg.P2PMode == "tor" {
				hint += theme.Dim.Render(
					"  p upgrade p2p")
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
	// Confirm dialogs override everything
	if s.svcConfirm != "" || s.sysConfirm != "" ||
		s.updateConfirm {
		return newConfirmBindings().ShortHelp()
	}

	if s.focusZone == sysHomeZoneServices {
		return s.serviceBindings()
	}
	return s.buttonBindings()
}

func (s *SystemHomeScreen) buttonBindings() []key.Binding {
	binds := []key.Binding{
		key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "services")),
		key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "buttons")),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")),
		kSidebar,
	}
	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑", "tab bar")))
	}
	binds = append(binds, kQuit)
	return binds
}

func (s *SystemHomeScreen) serviceBindings() []key.Binding {
	binds := []key.Binding{
		key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "services")),
		key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "restart")),
		key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "stop")),
		key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "start")),
		key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "logs")),
	}
	if s.svcName(s.svcCursor) == "lnd" &&
		s.ctx.Cfg.P2PMode == "tor" {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("p"),
				key.WithHelp("p", "upgrade p2p")))
	}
	binds = append(binds,
		key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("⇧tab", "buttons")),
		kSidebar,
	)
	binds = append(binds, kQuit)
	return binds
}

// ── Helpers ─────────────────────────────────────────────

func (s *SystemHomeScreen) hasUpdate() bool {
	return s.ctx.LatestVersion != "" &&
		s.ctx.LatestVersion != s.ctx.Version
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
