package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── InstallProgressScreen ──────────────────────────────
// Reusable Screen that runs a list of install steps
// sequentially and renders progress in the content pane.
// Replaces the standalone RunInstallTUI program for all
// post-install flows (Syncthing, LndHub, P2P upgrade,
// self-update).
//
// Usage:
//   steps := []installer.InstallStep{...}
//   screen := NewInstallProgressScreen(ctx, steps, onDone)
//
// The onDone callback fires after all steps succeed. It
// returns a tea.Cmd that typically saves config and emits
// refreshStatusMsg. On failure, onDone is NOT called.

type installStepDoneMsg struct {
	index int
	err   error
}

type InstallProgressScreen struct {
	ctx    *ScreenContext
	steps  []installer.InstallStep
	onDone func() tea.Cmd // called after all steps succeed
	onFail func() tea.Cmd // called on step failure (rollback)

	current int
	done    bool
	failed  bool
}

func NewInstallProgressScreen(
	ctx *ScreenContext,
	steps []installer.InstallStep,
	onDone func() tea.Cmd,
	onFail func() tea.Cmd,
) *InstallProgressScreen {
	return &InstallProgressScreen{
		ctx:    ctx,
		steps:  steps,
		onDone: onDone,
		onFail: onFail,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *InstallProgressScreen) Init() tea.Cmd {
	if len(s.steps) == 0 {
		s.done = true
		return nil
	}
	s.steps[0].Status = installer.StepRunning
	return s.runStep(0)
}

func (s *InstallProgressScreen) runStep(i int) tea.Cmd {
	return func() tea.Msg {
		if i >= len(s.steps) {
			return installStepDoneMsg{index: i}
		}
		return installStepDoneMsg{
			index: i, err: s.steps[i].Fn(),
		}
	}
}

func (s *InstallProgressScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	if s.done {
		switch keyStr {
		case "ctrl+c":
			return s, tea.Quit
		case "enter", "backspace":
			return s, emitCloseTab
		}
	}
	// During active install, all keys are ignored.
	return s, nil
}

func (s *InstallProgressScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case installStepDoneMsg:
		if msg.index >= len(s.steps) {
			return s, nil
		}
		if msg.err != nil {
			s.steps[msg.index].Status = installer.StepFailed
			s.steps[msg.index].Err = msg.err
			s.failed = true
			s.done = true
			if s.onFail != nil {
				return s, s.onFail()
			}
			return s, nil
		}
		s.steps[msg.index].Status = installer.StepDone
		next := msg.index + 1
		if next < len(s.steps) {
			s.current = next
			s.steps[next].Status = installer.StepRunning
			return s, s.runStep(next)
		}
		// All steps complete
		s.done = true
		if s.onDone != nil {
			return s, s.onDone()
		}
	}
	return s, nil
}

// ── View ────────────────────────────────────────────────

func (s *InstallProgressScreen) View(
	w, h int,
) string {
	var lines []string
	lines = append(lines, "")

	for i, step := range s.steps {
		var ind string
		var sty = theme.Dim
		switch step.Status {
		case installer.StepDone:
			ind = "[done]"
			sty = theme.Good
		case installer.StepRunning:
			ind = "[....]"
			sty = theme.Value
		case installer.StepFailed:
			ind = "[FAIL]"
			sty = theme.Warn
		default:
			ind = "[wait]"
		}

		lines = append(lines,
			"  "+sty.Render(fmt.Sprintf(
				"%s [%d/%d] %s",
				ind, i+1, len(s.steps), step.Name)))

		if step.Status == installer.StepFailed &&
			step.Err != nil {
			lines = append(lines,
				"  "+theme.Warn.Render(
					fmt.Sprintf("    Error: %v",
						step.Err)))
		}
	}

	lines = append(lines, "")

	if s.done && !s.failed {
		lines = append(lines,
			"  "+theme.Good.Render(
				"Complete."))
		lines = append(lines, "")
		lines = append(lines,
			"  "+theme.Dim.Render(
				"Press Enter to close."))
	} else if s.failed {
		lines = append(lines,
			"  "+theme.Warn.Render(
				"Installation failed."))
		lines = append(lines, "")
		lines = append(lines,
			"  "+theme.Dim.Render(
				"Press Enter to close."))
	} else {
		lines = append(lines,
			"  "+theme.Dim.Render(
				"Installing... please wait"))
	}

	content := strings.Join(lines, "\n")

	// Center vertically in available space
	contentLines := strings.Split(content, "\n")
	if len(contentLines) < h {
		pad := (h - len(contentLines)) / 3
		prefix := strings.Repeat("\n", pad)
		content = prefix + content
	}

	return content
}

// ── HelpBindings ────────────────────────────────────────

func (s *InstallProgressScreen) HelpBindings() []key.Binding {
	if s.done {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "close")),
			kQuit,
		}
	}
	return []key.Binding{}
}
