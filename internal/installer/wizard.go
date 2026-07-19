// internal/installer/wizard.go

package installer

// The interactive `vpn install` front-end: the ruling-vii
// identity/access screen, the ruling-viii hardware-fit screen,
// and the step-progress renderer, flowing into the completion
// handoff. Born in the unified chrome (ruling xvi(d)): theme
// styles and the step-renderer visual language of the post-
// install TUI — but chrome only, NO ScreenContext/lifecycle
// adoption (the C′ boundary holds; this front-end stays a thin
// driver of the commit-5 engine, making no skip or record
// decisions of its own).
//
// Step-render semantics (ratified, xvi(d)): DIM rows are
// believed-from-ledger (a prior pass completed them — [skip]);
// BRIGHT rows executed or verified THIS pass. Gates always
// re-run, so gates are always bright.
//
// Navigation is buttons-over-keys (operator ruling at the
// commit-6 live run): every screen that can go back has a Back
// button; no esc bindings (esc is unmapped in the rest of the
// TUI); each screen carries a dim hint line naming its keys.
//
// Completion is PERSISTED the moment the last step verifies —
// before the done screen waits for Enter (live-run finding: the
// old order left install_complete unwritten while the done
// screen sat unattended; a dropped connection then meant a
// complete install with no durable completion record until the
// next re-run).

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

type wizardPhase int

const (
	wzAccess wizardPhase = iota
	wzPaste
	wzPassword
	wzHardware
	wzSteps
	wzDone
)

type wizardStepDoneMsg struct {
	index   int
	skipped bool
	err     error
}

type wizardModel struct {
	cfg     *config.AppConfig
	dec     *InstallDecisions
	steps   []InstallStep
	runner  *stepRunner
	version string

	// onComplete persists the completion record; runs once,
	// as soon as the last step verifies (before the done
	// screen). Its error renders on the done screen and fails
	// the run.
	onComplete  func() error
	completeErr string

	phase         wizardPhase
	width, height int

	needIdentity bool
	needHardware bool

	// Access screen
	sources  []KeySource
	keys     []SSHKeyInfo
	selected []bool
	cursor   int // 0..len(keys)-1 = key rows; len(keys) = button row
	btnIdx   int // 0 = Continue, 1 = Paste a key
	accErr   string

	// Paste screen
	pasteInput textinput.Model
	pasteFocus int // 0 = input, 1 = buttons
	pasteBtn   int // 0 = Back, 1 = Add key
	pasteErr   string

	// Password screen
	pwInput   textinput.Model
	pwConfirm textinput.Model
	pwFocus   int // 0 = new, 1 = confirm, 2 = buttons
	pwBtn     int // 0 = Back, 1 = Continue
	pwErr     string

	// Hardware screen
	hw      Hardware
	dbIdx   int
	hwFocus int // 0 = dbcache selector, 1 = buttons
	hwBtn   int // index into hwButtons()

	// Steps
	current           int
	stepsDone, failed bool

	// openConsole records the operator's CHOICE on the done
	// screen: Enter = open the node console (handoff), ctrl+c
	// = just exit. The live run caught the two paths behaving
	// identically — the hint promised an exit that handed off
	// anyway.
	openConsole bool
}

func newWizardModel(
	cfg *config.AppConfig, steps []InstallStep,
	runner *stepRunner, dec *InstallDecisions, version string,
	onComplete func() error,
) wizardModel {
	m := wizardModel{
		cfg: cfg, dec: dec, steps: steps,
		runner: runner, version: version,
		onComplete: onComplete,
	}

	// Resume-aware screen skipping: a screen exists to collect
	// decisions for a step; if the ledger says that step is
	// complete, re-asking would collect answers nothing will
	// apply. (Residual, accepted: an interrupted run that
	// resumes past the btc group keeps bitcoin.conf from the
	// earlier pass while dbcache is re-persisted only at
	// completion — present-correctness is the doctor check's
	// question, not resume's.)
	m.needIdentity = runner.willRun(steps, "identity.access")
	m.needHardware = runner.willRun(steps, "btc.install")

	m.sources = SortKeySources(EnumerateKeySources())
	m.keys = DedupeKeys(m.sources)
	m.selected = make([]bool, len(m.keys))
	for i := range m.selected {
		m.selected[i] = true
	}

	m.pasteInput = newWizardInput(
		"ssh-ed25519 AAAA... comment", 1000, 56)
	m.pwInput = newWizardPasswordInput()
	m.pwConfirm = newWizardPasswordInput()

	m.hw = DetectHardware()
	rec := RecommendDbCache(m.hw.RAMMB)
	for i, v := range dbCacheChoices {
		if v == rec {
			m.dbIdx = i
		}
	}

	switch {
	case m.needIdentity:
		m.phase = wzAccess
	case m.needHardware:
		m.phase = wzHardware
	default:
		m.phase = wzSteps
	}
	return m
}

func newWizardInput(
	placeholder string, limit, width int,
) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = limit
	ti.SetWidth(width)
	ti.Prompt = "  "
	s := textinput.DefaultStyles(theme.IsDark())
	ti.SetStyles(s)
	return ti
}

func newWizardPasswordInput() textinput.Model {
	ti := newWizardInput("", 128, 40)
	ti.EchoMode = textinput.EchoPassword
	return ti
}

// pasteFirstLine normalizes pasted text for a single-line
// input: first line only, trimmed. Pure — unit-tested.
func pasteFirstLine(content string) string {
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		content = content[:idx]
	}
	return strings.TrimSpace(content)
}

// ── tea.Model ────────────────────────────────────────────

func (m wizardModel) Init() tea.Cmd {
	if m.phase == wzSteps {
		return m.startSteps()
	}
	return nil
}

func (m wizardModel) startSteps() tea.Cmd {
	if len(m.steps) == 0 {
		return tea.Quit
	}
	m.steps[0].Status = StepRunning
	return m.runStep(0)
}

func (m wizardModel) runStep(i int) tea.Cmd {
	return func() tea.Msg {
		if i >= len(m.steps) {
			return wizardStepDoneMsg{index: i}
		}
		skipped, err := m.runner.runIndex(i)
		return wizardStepDoneMsg{
			index: i, skipped: skipped, err: err}
	}
}

func (m wizardModel) Update(
	msg tea.Msg,
) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case wizardStepDoneMsg:
		return m.updateSteps(msg)

	case tea.PasteMsg:
		// Terminal paste — routed to the focused input (the
		// live-run finding: keypress-only routing silently
		// dropped pasted keys and passwords).
		return m.handlePaste(msg)

	case tea.KeyPressMsg:
		// Quitting is allowed at any instant — an interrupt is
		// SAFE (the ledger records a step only after it
		// verified complete) and HONEST (a quit before all
		// steps ran classifies as interrupted, never complete —
		// IA-1-9).
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.phase {
		case wzAccess:
			return m.updateAccess(msg)
		case wzPaste:
			return m.updatePaste(msg)
		case wzPassword:
			return m.updatePassword(msg)
		case wzHardware:
			return m.updateHardware(msg)
		case wzDone:
			if msg.String() == "enter" && !m.failed &&
				m.completeErr == "" {
				m.openConsole = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m wizardModel) handlePaste(
	msg tea.PasteMsg,
) (tea.Model, tea.Cmd) {
	line := pasteFirstLine(msg.Content)
	switch {
	case m.phase == wzPaste && m.pasteFocus == 0:
		m.pasteInput.SetValue(line)
		m.pasteErr = ""
	case m.phase == wzPassword && m.pwFocus == 0:
		m.pwInput.SetValue(line)
		m.pwErr = ""
	case m.phase == wzPassword && m.pwFocus == 1:
		m.pwConfirm.SetValue(line)
		m.pwErr = ""
	}
	return m, nil
}

// ── Access screen ────────────────────────────────────────

func (m wizardModel) updateAccess(
	msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	last := len(m.keys) // index of the button row
	switch msg.String() {
	case "up", "shift+tab":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "tab":
		if m.cursor < last {
			m.cursor++
		}
	case "left":
		if m.cursor == last && m.btnIdx > 0 {
			m.btnIdx--
		}
	case "right":
		if m.cursor == last && m.btnIdx < 1 {
			m.btnIdx++
		}
	case "space":
		if m.cursor < len(m.keys) {
			m.selected[m.cursor] = !m.selected[m.cursor]
			m.accErr = ""
		}
	case "enter":
		if m.cursor < len(m.keys) {
			m.selected[m.cursor] = !m.selected[m.cursor]
			m.accErr = ""
			return m, nil
		}
		if m.btnIdx == 1 {
			m.pasteErr = ""
			m.pasteInput.SetValue("")
			m.pasteFocus = 0
			m.pasteBtn = 1
			m.pasteInput.Focus()
			m.phase = wzPaste
			return m, nil
		}
		// Continue: collect selection; refuse a zero-auth
		// outcome (no keys AND password auth observed off —
		// the drop-in will carry that "off" forward, so the
		// admin user would have no network way in at all).
		var chosen []SSHKeyInfo
		for i, k := range m.keys {
			if m.selected[i] {
				chosen = append(chosen, k)
			}
		}
		if len(chosen) == 0 && !m.dec.Obs.PasswordAuth {
			m.accErr = "Password login is disabled on this " +
				"box. Select or paste at least one key, or " +
				"the " + paths.AdminUser +
				" user would have no way in over SSH."
			return m, nil
		}
		m.dec.Keys = chosen
		m.enterPasswordScreen()
		return m, nil
	}
	return m, nil
}

func (m *wizardModel) enterPasswordScreen() {
	m.pwFocus = 0
	m.pwBtn = 1
	m.pwErr = ""
	m.pwInput.Focus()
	m.pwConfirm.Blur()
	m.phase = wzPassword
}

func (m wizardModel) viewAccess(p *wizPane) {
	p.header("Identity and access")
	p.blank()
	p.text("This node's admin user is '" + paths.AdminUser +
		"'. Every SSH login as " + paths.AdminUser +
		" opens the node console.")
	p.blank()

	if len(m.keys) == 0 {
		p.text("No SSH keys were found on this box.")
	} else {
		p.text("SSH keys found on this box. Confirmed keys " +
			"are copied to " + paths.AdminUser + ":")
	}
	p.blank()

	for i, k := range m.keys {
		mark := "[ ]"
		if m.selected[i] {
			mark = "[x]"
		}
		cur := "  "
		if m.cursor == i {
			cur = "> "
		}
		line := fmt.Sprintf("%s%s %s", cur, mark, k.Fingerprint)
		sty := theme.Value
		if m.cursor == i {
			sty = theme.Action
		}
		p.line(" " + sty.Render(line))
		detail := "      " + k.Type
		if k.Comment != "" {
			detail += " (" + k.Comment + ")"
		}
		detail += "  [" + m.keySourceNames(k.Fingerprint) + "]"
		p.dim(detail)
	}
	for _, s := range m.sources {
		if s.Excluded > 0 {
			p.dim(fmt.Sprintf(
				"   %d provider control line(s) in %s excluded,"+
					" not copied", s.Excluded, s.Path))
		}
	}
	if m.accErr != "" {
		p.blank()
		p.warn(m.accErr)
	}
	p.blank()
	p.buttons([]string{"Continue", "Paste a key"},
		m.btnIdx, m.cursor == len(m.keys))
	p.blank()
	p.hint("up/down: move   space: toggle key   " +
		"left/right: choose button   enter: select")
}

// keySourceNames lists which enumerated files carried this
// fingerprint (e.g. "root, debian").
func (m wizardModel) keySourceNames(fingerprint string) string {
	var names []string
	for _, s := range m.sources {
		for _, k := range s.Keys {
			if k.Fingerprint == fingerprint {
				names = append(names, s.User)
				break
			}
		}
	}
	if len(names) == 0 {
		return "pasted"
	}
	return strings.Join(names, ", ")
}

// ── Paste screen ─────────────────────────────────────────

func (m wizardModel) updatePaste(
	msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "shift+tab":
		if m.pasteFocus == 1 {
			m.pasteFocus = 0
			m.pasteInput.Focus()
		}
		return m, nil
	case "down", "tab":
		if m.pasteFocus == 0 {
			m.pasteFocus = 1
			m.pasteInput.Blur()
		}
		return m, nil
	case "left":
		if m.pasteFocus == 1 && m.pasteBtn > 0 {
			m.pasteBtn--
			return m, nil
		}
	case "right":
		if m.pasteFocus == 1 && m.pasteBtn < 1 {
			m.pasteBtn++
			return m, nil
		}
	case "enter":
		if m.pasteFocus == 1 && m.pasteBtn == 0 {
			// Back — nothing kept.
			m.phase = wzAccess
			return m, nil
		}
		// Add key (from the button, or enter in the input).
		line := strings.TrimSpace(m.pasteInput.Value())
		info, err := ParseSSHKey(line)
		if err != nil {
			m.pasteErr = "Not a valid public key line: " +
				err.Error()
			return m, nil
		}
		for i, k := range m.keys {
			if k.Fingerprint == info.Fingerprint {
				m.selected[i] = true
				m.phase = wzAccess
				return m, nil
			}
		}
		m.keys = append(m.keys, info)
		m.selected = append(m.selected, true)
		m.phase = wzAccess
		m.cursor = len(m.keys) - 1
		return m, nil
	}
	if m.pasteFocus == 0 {
		var cmd tea.Cmd
		m.pasteInput, cmd = m.pasteInput.Update(tea.Msg(msg))
		return m, cmd
	}
	return m, nil
}

func (m wizardModel) viewPaste(p *wizPane) {
	p.header("Paste a public key")
	p.blank()
	p.text("Paste one authorized_keys line (type, key data, " +
		"optional comment). It is validated before use.")
	p.blank()
	p.line(" " + m.pasteInput.View())
	if m.pasteErr != "" {
		p.blank()
		p.warn(m.pasteErr)
	}
	p.blank()
	p.buttons([]string{"Back", "Add key"},
		m.pasteBtn, m.pasteFocus == 1)
	p.blank()
	p.hint("up/down: input or buttons   " +
		"left/right: choose button   enter: select")
}

// ── Password screen ──────────────────────────────────────

func (m wizardModel) updatePassword(
	msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "shift+tab":
		if m.pwFocus > 0 {
			m.pwFocus--
			m.syncPwFocus()
		}
		return m, nil
	case "down", "tab":
		if m.pwFocus < 2 {
			m.pwFocus++
			m.syncPwFocus()
		}
		return m, nil
	case "left":
		if m.pwFocus == 2 && m.pwBtn > 0 {
			m.pwBtn--
			return m, nil
		}
	case "right":
		if m.pwFocus == 2 && m.pwBtn < 1 {
			m.pwBtn++
			return m, nil
		}
	case "enter":
		if m.pwFocus < 2 {
			m.pwFocus++
			m.syncPwFocus()
			return m, nil
		}
		if m.pwBtn == 0 {
			// Back to the access screen (selections kept).
			m.phase = wzAccess
			return m, nil
		}
		// Continue. Non-skippable by construction: the only
		// forward button validates.
		if m.pwInput.Value() != m.pwConfirm.Value() {
			m.pwErr = "Passwords do not match."
			return m, nil
		}
		pw, err := NewLoginPassword(m.pwInput.Value())
		if err != nil {
			m.pwErr = err.Error() + "."
			return m, nil
		}
		m.dec.Password = pw
		m.pwErr = ""
		if m.needHardware {
			m.hwFocus = 1
			m.hwBtn = len(m.hwButtons()) - 1
			m.phase = wzHardware
			return m, nil
		}
		m.phase = wzSteps
		return m, m.startSteps()
	}
	if m.pwFocus < 2 {
		var cmd tea.Cmd
		switch m.pwFocus {
		case 0:
			m.pwInput, cmd = m.pwInput.Update(tea.Msg(msg))
		case 1:
			m.pwConfirm, cmd = m.pwConfirm.Update(tea.Msg(msg))
		}
		return m, cmd
	}
	return m, nil
}

func (m *wizardModel) syncPwFocus() {
	m.pwInput.Blur()
	m.pwConfirm.Blur()
	switch m.pwFocus {
	case 0:
		m.pwInput.Focus()
	case 1:
		m.pwConfirm.Focus()
	}
}

func (m wizardModel) viewPassword(p *wizPane) {
	p.header("Login password")
	p.blank()
	// State-aware copy from the preflight observation (ruling
	// xvi, vii refinement): say what this credential IS on this
	// box; never assert what cannot be verified (ruling-xi
	// discipline). The prompt is NON-SKIPPABLE: post-commit-7
	// the admin user is the box's only interactive identity,
	// and without a password a broken SSH setup leaves only
	// rescue mode.
	if m.dec.Obs.PasswordAuth {
		p.text("Set the login password for '" +
			paths.AdminUser + "'. Password login over SSH is " +
			"currently enabled on this box, so this password " +
			"works over the network; once you have verified " +
			"key login, you can disable password auth from " +
			"System, SSH Keys.")
	} else {
		p.text("Set the recovery password for '" +
			paths.AdminUser + "'. Password login over SSH is " +
			"disabled on this box, so this password works " +
			"ONLY at the machine's own login console: your " +
			"provider's console page, or a keyboard on the " +
			"box itself. It is the fallback when SSH is " +
			"broken.")
	}
	p.blank()
	p.text("Use a password manager: generate it, store it " +
		"there first. Minimum " +
		fmt.Sprintf("%d", MinLoginPasswordLen) + " characters.")
	p.blank()

	p.input("Password:", m.pwInput.View(), m.pwFocus == 0)
	if n := len(m.pwInput.Value()); n > 0 {
		p.dim(fmt.Sprintf("   (%d chars)", n))
	}
	p.input("Confirm: ", m.pwConfirm.View(), m.pwFocus == 1)
	if n := len(m.pwConfirm.Value()); n > 0 {
		p.dim(fmt.Sprintf("   (%d chars)", n))
	}
	if m.pwErr != "" {
		p.blank()
		p.warn(m.pwErr)
	}
	p.blank()
	p.buttons([]string{"Back", "Continue"},
		m.pwBtn, m.pwFocus == 2)
	p.blank()
	p.hint("up/down: move   left/right: choose button   " +
		"enter: select   paste works in both fields")
}

// ── Hardware screen ──────────────────────────────────────

// hwButtons: Back only exists when the identity flow ran ahead
// of this screen (otherwise this is the first screen and there
// is nothing to go back to).
func (m wizardModel) hwButtons() []string {
	if m.needIdentity {
		return []string{"Back", "Start install"}
	}
	return []string{"Start install"}
}

func (m wizardModel) updateHardware(
	msg tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	btns := m.hwButtons()
	switch msg.String() {
	case "up", "shift+tab":
		if m.hwFocus == 1 {
			m.hwFocus = 0
		}
	case "down", "tab":
		if m.hwFocus == 0 {
			m.hwFocus = 1
			m.hwBtn = len(btns) - 1
		}
	case "left":
		if m.hwFocus == 0 && m.dbIdx > 0 {
			m.dbIdx--
		}
		if m.hwFocus == 1 && m.hwBtn > 0 {
			m.hwBtn--
		}
	case "right":
		if m.hwFocus == 0 && m.dbIdx < len(dbCacheChoices)-1 {
			m.dbIdx++
		}
		if m.hwFocus == 1 && m.hwBtn < len(btns)-1 {
			m.hwBtn++
		}
	case "enter":
		if m.hwFocus == 0 {
			m.hwFocus = 1
			m.hwBtn = len(btns) - 1
			return m, nil
		}
		if len(btns) == 2 && m.hwBtn == 0 {
			// Back to the password screen.
			m.enterPasswordScreen()
			return m, nil
		}
		m.dec.DbCacheMB = dbCacheChoices[m.dbIdx]
		m.cfg.DbCache = m.dec.DbCacheMB
		m.phase = wzSteps
		return m, m.startSteps()
	}
	return m, nil
}

func fitMark(ok bool) string {
	if ok {
		return "ok"
	}
	return "below recommended"
}

func (m wizardModel) viewHardware(p *wizPane) {
	p.header("Hardware fit")
	p.blank()
	p.text("What this box has, next to the recommended " +
		"minimums for a node (2 CPU cores, 4+ GB RAM, " +
		"90+ GB disk). Short of a minimum is a warning, " +
		"not a refusal.")
	p.blank()

	ram := "unknown"
	if m.hw.RAMMB > 0 {
		ram = fmt.Sprintf("%.1f GB, %s",
			float64(m.hw.RAMMB)/1024,
			fitMark(m.hw.RAMMB >= requiredRAMMB))
	}
	disk := "unknown"
	if m.hw.DiskTotalGB > 0 {
		disk = fmt.Sprintf("%d GB total, %d GB free, %s",
			m.hw.DiskTotalGB, m.hw.DiskFreeGB,
			fitMark(m.hw.DiskTotalGB >= requiredDiskGB))
	}
	p.kv("Memory (RAM)", ram)
	p.kv("Disk", disk)
	p.kv("CPU", fmt.Sprintf("%d cores, %s", m.hw.Cores,
		fitMark(m.hw.Cores >= requiredCores)))
	p.blank()

	rec := RecommendDbCache(m.hw.RAMMB)
	p.text("Bitcoin Core database cache (dbcache). Larger is " +
		"faster for the initial sync; it must leave room for " +
		"LND and Tor.")
	p.blank()
	choice := fmt.Sprintf("  ◂ %4d MB ▸", dbCacheChoices[m.dbIdx])
	sty := theme.Value
	if m.hwFocus == 0 {
		sty = theme.Action
	}
	p.line(" " + sty.Render(choice) + theme.Dim.Render(
		fmt.Sprintf("   (recommended for this box: %d MB)", rec)))
	p.blank()
	p.buttons(m.hwButtons(), m.hwBtn, m.hwFocus == 1)
	p.blank()
	p.hint("up/down: cache size or buttons   left/right: " +
		"adjust or choose   enter: select")
}

// ── Steps phase ──────────────────────────────────────────

func (m wizardModel) updateSteps(
	msg wizardStepDoneMsg,
) (tea.Model, tea.Cmd) {
	if msg.index >= len(m.steps) {
		return m, nil
	}
	if msg.err != nil {
		m.steps[msg.index].Status = StepFailed
		m.steps[msg.index].Err = msg.err
		m.failed = true
		m.stepsDone = true
		m.phase = wzDone
		return m, nil
	}
	if msg.skipped {
		m.steps[msg.index].Status = StepSkipped
	} else {
		m.steps[msg.index].Status = StepDone
	}
	next := msg.index + 1
	if next < len(m.steps) {
		m.current = next
		m.steps[next].Status = StepRunning
		return m, m.runStep(next)
	}
	// Last step verified — persist completion NOW, before the
	// done screen waits for the operator (who may be away for
	// minutes, or whose connection may drop).
	m.stepsDone = true
	if m.onComplete != nil {
		if err := m.onComplete(); err != nil {
			m.completeErr = err.Error()
		}
	}
	m.phase = wzDone
	return m, nil
}

// renderStepRows renders the step list in the ratified
// dim/bright language. Shared shape with the post-install TUI's
// step renderer: dim = believed-from-ledger, bright = this pass,
// gates always bright (they always run).
func renderStepRows(p *wizPane, steps []InstallStep) {
	total := len(steps)
	for i, s := range steps {
		var sty lipgloss.Style
		var ind string
		switch s.Status {
		case StepDone:
			sty, ind = theme.Value, "[done]"
		case StepRunning:
			sty, ind = theme.Action, "[....]"
		case StepFailed:
			sty, ind = theme.Warning, "[FAIL]"
		case StepSkipped:
			// Believed-from-ledger: completed by an earlier
			// pass, not executed now.
			sty, ind = theme.Dim, "[skip]"
		default:
			sty, ind = theme.Grayed, "[wait]"
		}
		p.line(" " + sty.Render(fmt.Sprintf(
			"%s [%2d/%d] %s", ind, i+1, total, s.Name)))
		if s.Status == StepFailed && s.Err != nil {
			p.warn("    Error: " + s.Err.Error())
		}
	}
}

func (m wizardModel) viewSteps(p *wizPane) {
	p.header("Installing")
	p.blank()
	renderStepRows(p, m.steps)
	p.blank()
	p.hint("installing, do not close the terminal " +
		"(ctrl+c interrupts; a re-run resumes)")
}

// sshTarget renders the everyday login command with the box's
// address when it is known.
func sshTarget() string {
	if ip := system.PublicIPv4(); ip != "" {
		return "ssh " + paths.AdminUser + "@" + ip
	}
	return "ssh " + paths.AdminUser + "@<your-server-ip>"
}

func (m wizardModel) viewDone(p *wizPane) {
	if m.failed {
		p.header("Install failed")
		p.blank()
		renderStepRows(p, m.steps)
		p.blank()
		p.warn("The install failed. Nothing needs undoing: " +
			"fix the reported problem and run " +
			"'sudo vpn install' again; it resumes from the " +
			"first incomplete step.")
		p.blank()
		p.hint("ctrl+c: exit")
		return
	}
	if m.completeErr != "" {
		p.header("Install steps complete, but not recorded")
		p.blank()
		p.warn("Every step finished, but writing the " +
			"completion record failed: " + m.completeErr)
		p.blank()
		p.warn("Run 'sudo vpn install' again; it will skip " +
			"all finished steps and retry the record.")
		p.blank()
		p.hint("ctrl+c: exit")
		return
	}
	p.header("Install complete")
	p.blank()
	renderStepRows(p, m.steps)
	p.blank()
	p.text("Your everyday way into the node, from any " +
		"terminal:")
	p.line(" " + theme.Action.Render("   "+sshTarget()))
	p.blank()
	p.text("Press Enter to open the node console as user '" +
		paths.AdminUser + "' on this terminal, or run the " +
		"command above from a SECOND terminal first to " +
		"verify your access.")
	p.blank()
	p.hint("enter: open the node console   ctrl+c: exit to " +
		"your shell (your install is saved; connect any time " +
		"with the command above)")
}

// ── View plumbing ────────────────────────────────────────

// wizPane is a minimal line collector in the welcome pane
// idiom — chrome, not machinery (no ScreenContext).
type wizPane struct {
	width int
	lines []string
}

func (p *wizPane) line(s string) { p.lines = append(p.lines, s) }
func (p *wizPane) blank()        { p.line("") }
func (p *wizPane) header(s string) {
	p.line(" " + theme.Header.Render(s))
}
func (p *wizPane) dim(s string) {
	for _, l := range wrapText(s, p.width-4) {
		p.line(" " + theme.Dim.Render(l))
	}
}
func (p *wizPane) hint(s string) {
	for _, l := range wrapText(s, p.width-4) {
		p.line(" " + theme.Grayed.Render(l))
	}
}
func (p *wizPane) warn(s string) {
	for _, l := range wrapText(s, p.width-4) {
		p.line(" " + theme.Warning.Render(l))
	}
}
func (p *wizPane) text(s string) {
	for _, l := range wrapText(s, p.width-4) {
		p.line(" " + theme.Label.Render(l))
	}
}
func (p *wizPane) kv(k, v string) {
	p.line(" " + theme.Label.Render(
		fmt.Sprintf("  %-14s", k+":")) +
		theme.Value.Render(" "+v))
}
func (p *wizPane) input(label, view string, focused bool) {
	sty := theme.Label
	if focused {
		sty = theme.Action
	}
	p.line(" " + sty.Render("  "+label) + view)
}
func (p *wizPane) buttons(labels []string, idx int, focused bool) {
	var parts []string
	for i, l := range labels {
		if i == idx && focused {
			parts = append(parts, theme.BtnFocused.Render(l))
		} else {
			parts = append(parts, theme.BtnNormal.Render(l))
		}
	}
	p.line(" " + strings.Join(parts, "  "))
}

func (m wizardModel) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}
	bw := min(m.width-4, theme.ContentWidth)
	p := &wizPane{width: bw}

	switch m.phase {
	case wzAccess:
		m.viewAccess(p)
	case wzPaste:
		m.viewPaste(p)
	case wzPassword:
		m.viewPassword(p)
	case wzHardware:
		m.viewHardware(p)
	case wzSteps:
		m.viewSteps(p)
	case wzDone:
		m.viewDone(p)
	}

	title := theme.Title.Render(
		"Virtual Private Node — Install")
	box := theme.Box.Width(bw).Render(
		strings.Join(p.lines, "\n"))
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, box)
	content := lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "Virtual Private Node"
	return v
}

// willRun reports whether the planner will execute the step with
// the given key this pass (false = ledger-skip; true also when
// the key is absent from the list, the conservative side for
// screen-skipping).
func (r *stepRunner) willRun(
	steps []InstallStep, key string,
) bool {
	for i, s := range steps {
		if s.Key == key {
			return r.plan[i].Run
		}
	}
	return true
}

// runInstallWizard drives the interactive install: wizard
// screens, then the engine steps, reporting HOW the run ended via
// RunResult (only RunComplete may reach the InstallComplete
// write — IA-1-9). onComplete is invoked exactly once, when the
// last step verifies, BEFORE the done screen blocks on input; a
// persist failure is surfaced as the run's error.
func runInstallWizard(
	cfg *config.AppConfig, steps []InstallStep,
	dec *InstallDecisions, version string,
	onComplete func() error,
) (RunResult, bool, error) {
	runner, err := newStepRunner(
		steps, version, paths.InstallStateFile)
	if err != nil {
		return RunResult{}, false, err
	}
	theme.Init(cfg.Theme != "light")
	m := newWizardModel(cfg, steps, runner, dec, version,
		onComplete)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return RunResult{}, false, err
	}
	final := result.(wizardModel)
	if final.completeErr != "" {
		return RunResult{}, false, fmt.Errorf(
			"install steps complete but the completion record "+
				"failed: %s — run sudo vpn install again to retry",
			final.completeErr)
	}
	return classifyRun(final.steps,
		final.stepsDone && !final.failed,
		final.failed, final.current), final.openConsole, nil
}
