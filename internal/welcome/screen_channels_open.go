package welcome

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Channel open screen steps ──────────────────────────

type chanOpenStep int

const (
	coStepInput      chanOpenStep = iota // peer + amount + toggles + buttons
	coStepCustomPeer                     // pubkey + host fields + Cancel/Continue
	coStepConfirm                        // summary + Go Back / Confirm
	coStepOpening                        // in-flight
	coStepResult                         // success or error
)

// ── Focus zones for coStepInput ────────────────────────

const (
	coZonePeers   = 0
	coZoneAmounts = 1
	coZoneToggles = 2
	coZoneButtons = 3
)

// ── Focus zones for coStepCustomPeer ───────────────────

const (
	coCustomZonePubkey  = 0
	coCustomZoneHost    = 1
	coCustomZoneButtons = 2
)

// ── ChannelOpenScreen ──────────────────────────────────

type ChannelOpenScreen struct {
	ctx  *ScreenContext
	step chanOpenStep

	// Peer selection
	peerList     []peerOption
	peerIdx      int
	pubkeyInput  textinput.Model
	hostInput    textinput.Model
	customPubkey string
	customHost   string
	customAlias  string
	customZone   int
	customBtnIdx int

	// Amount selection
	amountPreset int
	amountInput  textinput.Model
	amount       int64

	// Toggles
	private   bool
	taproot   bool
	toggleIdx int

	// Selection state (✓ indicators)
	peerConfirmed   bool
	amountConfirmed bool

	// Navigation
	focusZone int

	// Buttons
	btnIdx int

	// Confirm
	confirmBtnIdx int

	// Result
	inFlight bool
	txid     string
	error    string
}

func NewChannelOpenScreen(
	ctx *ScreenContext,
) *ChannelOpenScreen {
	return &ChannelOpenScreen{
		ctx:          ctx,
		step:         coStepInput,
		peerList:     channelOpenPeers(),
		amountInput:  newChanAmountInput(),
		private:      true,
		taproot:      true,
		btnIdx:       1,
		pubkeyInput:  newChanPubkeyInput(),
		hostInput:    newChanHostInput(),
		customBtnIdx: 1,
	}
}

// ── Screen interface ────────────────────────────────────

func (s *ChannelOpenScreen) Init() tea.Cmd {
	return nil
}

func (s *ChannelOpenScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case coStepInput:
		return s.handleInputKey(keyStr, msg)
	case coStepCustomPeer:
		return s.handleCustomPeerKey(keyStr, msg)
	case coStepConfirm:
		return s.handleConfirmKey(keyStr)
	case coStepOpening:
		return s.handleOpeningKey(keyStr)
	case coStepResult:
		return s.handleResultKey(keyStr)
	}
	return s, nil
}

func (s *ChannelOpenScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		return s.handlePaste(msg)
	case channelOpenResultMsg:
		return s.handleOpenResult(msg)
	}
	return s, nil
}

func (s *ChannelOpenScreen) View(w, h int) string {
	switch s.step {
	case coStepInput:
		return s.viewInput(w, h)
	case coStepCustomPeer:
		return s.viewCustomPeer(w)
	case coStepConfirm:
		return s.viewConfirm(w)
	case coStepOpening:
		return s.viewOpening(w)
	case coStepResult:
		return s.viewResult(w)
	}
	return ""
}

func (s *ChannelOpenScreen) HelpBindings() []key.Binding {
	switch s.step {
	case coStepInput:
		return s.inputBindings()
	case coStepCustomPeer:
		return s.customPeerBindings()
	case coStepConfirm:
		return s.confirmBindings()
	case coStepOpening:
		return s.openingBindings()
	case coStepResult:
		return newResultBindings().ShortHelp()
	}
	return nil
}

// ── Input step ─────────────────────────────────────────

func (s *ChannelOpenScreen) handleInputKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.focusZone {
	case coZonePeers:
		return s.handlePeerListKey(keyStr)
	case coZoneAmounts:
		return s.handleAmountListKey(keyStr, msg)
	case coZoneToggles:
		return s.handleToggleKey(keyStr)
	case coZoneButtons:
		return s.handleButtonKey(keyStr)
	}
	return s, nil
}

func (s *ChannelOpenScreen) handlePeerListKey(
	keyStr string,
) (Screen, tea.Cmd) {
	customIdx := len(s.peerList)
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		return s, emitFocusSidebar
	case "up":
		if s.peerIdx > 0 {
			s.peerIdx--
			s.peerConfirmed = false
		} else if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down":
		if s.peerIdx < customIdx {
			s.peerIdx++
			s.peerConfirmed = false
		} else {
			// Bottom of peer list: cross to amounts
			s.focusZone = coZoneAmounts
		}
		return s, nil
	case "tab":
		s.focusZone = coZoneAmounts
		return s, nil
	case "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "enter":
		if s.peerIdx == customIdx {
			// Open custom peer sub-step
			s.pubkeyInput = newChanPubkeyInput()
			s.hostInput = newChanHostInput()
			cw := tuiWidth - 2 - 14 - 1 - 6
			if cw > 58 {
				cw = 58
			}
			if cw < 20 {
				cw = 20
			}
			s.pubkeyInput.SetWidth(cw)
			s.hostInput.SetWidth(cw)
			s.customZone = coCustomZonePubkey
			s.customBtnIdx = 1
			s.error = ""
			s.step = coStepCustomPeer
			return s, nil
		}
		// Curated peer: confirm + advance
		s.peerConfirmed = true
		s.focusZone = coZoneAmounts
		s.amountPreset = 0
		return s, nil
	}
	return s, nil
}

func (s *ChannelOpenScreen) handleAmountListKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	isCustom := s.amountPreset == len(amountPresets)-1
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if isCustom &&
			s.amountInput.Value() != "" {
			var cmd tea.Cmd
			s.amountInput, cmd =
				s.amountInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusSidebar
	case "right":
		if isCustom {
			var cmd tea.Cmd
			s.amountInput, cmd =
				s.amountInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil
	case "up":
		if isCustom {
			// On custom row: move to previous preset
			s.amountPreset--
			s.amountConfirmed = false
			s.amountInput.Blur()
		} else if s.amountPreset > 0 {
			s.amountPreset--
			s.amountConfirmed = false
		} else {
			// Top of amount list: cross to peers
			s.focusZone = coZonePeers
			s.peerIdx = len(s.peerList)
		}
		return s, nil
	case "down":
		if !isCustom &&
			s.amountPreset < len(amountPresets)-1 {
			s.amountPreset++
			s.amountConfirmed = false
		} else {
			// Bottom of amount list: cross to toggles
			if isCustom {
				s.amountInput.Blur()
			}
			s.focusZone = coZoneToggles
			s.toggleIdx = 0
		}
		return s, nil
	case "tab":
		if isCustom {
			s.amountInput.Blur()
		}
		s.focusZone = coZoneToggles
		s.toggleIdx = 0
		return s, nil
	case "shift+tab":
		if isCustom {
			s.amountInput.Blur()
		}
		s.focusZone = coZonePeers
		return s, nil
	case "backspace":
		if isCustom {
			var cmd tea.Cmd
			s.amountInput, cmd =
				s.amountInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil
	case "enter":
		if isCustom {
			// Validate before confirming
			_, err := parseCustomAmount(
				s.amountInput.Value())
			if err != nil {
				s.error = err.Error()
				return s, nil
			}
			s.error = ""
		}
		s.amountConfirmed = true
		if isCustom {
			s.amountInput.Blur()
		}
		s.focusZone = coZoneToggles
		s.toggleIdx = 0
		return s, nil
	}
	if isCustom {
		var cmd tea.Cmd
		s.amountInput, cmd =
			s.amountInput.Update(tea.Msg(msg))
		return s, cmd
	}
	return s, nil
}

func (s *ChannelOpenScreen) handleToggleKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.toggleIdx > 0 {
			s.toggleIdx--
		} else {
			return s, emitFocusSidebar
		}
		return s, nil
	case "right":
		if s.toggleIdx < 1 {
			s.toggleIdx++
		}
		return s, nil
	case "up":
		if s.toggleIdx > 0 {
			s.toggleIdx--
		} else {
			s.focusZone = coZoneAmounts
		}
		return s, nil
	case "down":
		if s.toggleIdx < 1 {
			s.toggleIdx++
		} else {
			s.focusZone = coZoneButtons
		}
		return s, nil
	case "tab":
		s.focusZone = coZoneButtons
		return s, nil
	case "shift+tab":
		s.focusZone = coZoneAmounts
		return s, nil
	case "enter":
		switch s.toggleIdx {
		case 0:
			s.private = !s.private
		case 1:
			s.taproot = !s.taproot
		}
		return s, nil
	}
	return s, nil
}

func (s *ChannelOpenScreen) handleButtonKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.btnIdx > 0 {
			s.btnIdx--
		} else {
			return s, emitFocusSidebar
		}
		return s, nil
	case "right":
		if s.btnIdx < 1 {
			s.btnIdx++
		}
		return s, nil
	case "up":
		s.focusZone = coZoneToggles
		return s, nil
	case "tab":
		return s, nil
	case "shift+tab":
		s.focusZone = coZoneToggles
		return s, nil
	case "enter":
		switch s.btnIdx {
		case 0: // Clear
			return s.clearForm(), nil
		case 1: // Open Channel
			return s.submitOpenChannel()
		}
		return s, nil
	}
	return s, nil
}

// ── Custom peer step ───────────────────────────────────

func (s *ChannelOpenScreen) handleCustomPeerKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.customZone {
	case coCustomZonePubkey:
		return s.handleCustomPubkeyKey(keyStr, msg)
	case coCustomZoneHost:
		return s.handleCustomHostKey(keyStr, msg)
	case coCustomZoneButtons:
		return s.handleCustomButtonKey(keyStr)
	}
	return s, nil
}

func (s *ChannelOpenScreen) handleCustomPubkeyKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.pubkeyInput.Value() != "" {
			var cmd tea.Cmd
			s.pubkeyInput, cmd =
				s.pubkeyInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusSidebar
	case "right":
		if s.pubkeyInput.Value() != "" {
			var cmd tea.Cmd
			s.pubkeyInput, cmd =
				s.pubkeyInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil
	case "up":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "down":
		s.pubkeyInput.Blur()
		s.hostInput.Focus()
		s.customZone = coCustomZoneHost
		return s, nil
	case "tab":
		s.pubkeyInput.Blur()
		s.hostInput.Focus()
		s.customZone = coCustomZoneHost
		return s, nil
	case "shift+tab":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "backspace":
		if s.pubkeyInput.Value() != "" {
			var cmd tea.Cmd
			s.pubkeyInput, cmd =
				s.pubkeyInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil
	case "enter":
		s.pubkeyInput.Blur()
		s.hostInput.Focus()
		s.customZone = coCustomZoneHost
		return s, nil
	default:
		var cmd tea.Cmd
		s.pubkeyInput, cmd =
			s.pubkeyInput.Update(tea.Msg(msg))
		return s, cmd
	}
}

func (s *ChannelOpenScreen) handleCustomHostKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.hostInput.Value() != "" {
			var cmd tea.Cmd
			s.hostInput, cmd =
				s.hostInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusSidebar
	case "right":
		if s.hostInput.Value() != "" {
			var cmd tea.Cmd
			s.hostInput, cmd =
				s.hostInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil
	case "up":
		s.hostInput.Blur()
		s.pubkeyInput.Focus()
		s.customZone = coCustomZonePubkey
		return s, nil
	case "down":
		s.hostInput.Blur()
		s.customZone = coCustomZoneButtons
		return s, nil
	case "tab":
		s.hostInput.Blur()
		s.customZone = coCustomZoneButtons
		return s, nil
	case "shift+tab":
		s.hostInput.Blur()
		s.pubkeyInput.Focus()
		s.customZone = coCustomZonePubkey
		return s, nil
	case "backspace":
		if s.hostInput.Value() != "" {
			var cmd tea.Cmd
			s.hostInput, cmd =
				s.hostInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil
	case "enter":
		s.hostInput.Blur()
		s.customZone = coCustomZoneButtons
		return s, nil
	default:
		var cmd tea.Cmd
		s.hostInput, cmd =
			s.hostInput.Update(tea.Msg(msg))
		return s, cmd
	}
}

func (s *ChannelOpenScreen) handleCustomButtonKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.customBtnIdx > 0 {
			s.customBtnIdx--
		} else {
			return s, emitFocusSidebar
		}
		return s, nil
	case "right":
		if s.customBtnIdx < 1 {
			s.customBtnIdx++
		}
		return s, nil
	case "up":
		s.customZone = coCustomZoneHost
		s.hostInput.Focus()
		return s, nil
	case "tab":
		return s, nil
	case "shift+tab":
		s.customZone = coCustomZoneHost
		s.hostInput.Focus()
		return s, nil
	case "enter":
		switch s.customBtnIdx {
		case 0: // Cancel
			s.error = ""
			s.step = coStepInput
			return s, nil
		case 1: // Continue
			return s.submitCustomPeer()
		}
		return s, nil
	}
	return s, nil
}

// ── Confirm step ───────────────────────────────────────

func (s *ChannelOpenScreen) handleConfirmKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.confirmBtnIdx > 0 {
			s.confirmBtnIdx--
		} else {
			return s, emitFocusSidebar
		}
		return s, nil
	case "right":
		if s.confirmBtnIdx < 1 {
			s.confirmBtnIdx++
		}
		return s, nil
	case "up":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "backspace":
		s.backToInput()
		return s, nil
	case "enter":
		switch s.confirmBtnIdx {
		case 0: // Go Back
			s.backToInput()
			return s, nil
		case 1: // Confirm
			if s.inFlight {
				return s, nil
			}
			s.inFlight = true
			s.error = ""
			s.step = coStepOpening
			return s, openChannelCmd(
				s.ctx.LndClient,
				s.selectedPubkey(),
				s.selectedHost(),
				s.amount,
				s.private,
				s.taproot,
			)
		}
	}
	return s, nil
}

func (s *ChannelOpenScreen) backToInput() {
	s.step = coStepInput
	s.error = ""
	s.confirmBtnIdx = 0
	s.focusZone = coZoneButtons
	s.btnIdx = 1
}

// ── Opening step ───────────────────────────────────────

func (s *ChannelOpenScreen) handleOpeningKey(
	keyStr string,
) (Screen, tea.Cmd) {
	if keyStr == "ctrl+c" {
		return s, tea.Quit
	}
	return s, nil
}

// ── Result step ────────────────────────────────────────

func (s *ChannelOpenScreen) handleResultKey(
	keyStr string,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "enter", "backspace":
		return s, tea.Batch(
			emitCloseTab,
			emitRefreshStatus)
	}
	return s, nil
}

// ── Paste handling ─────────────────────────────────────

func (s *ChannelOpenScreen) handlePaste(
	msg tea.PasteMsg,
) (Screen, tea.Cmd) {
	if s.step == coStepCustomPeer {
		var cmd tea.Cmd
		if s.customZone == coCustomZonePubkey {
			s.pubkeyInput, cmd =
				s.pubkeyInput.Update(msg)
		} else if s.customZone == coCustomZoneHost {
			s.hostInput, cmd =
				s.hostInput.Update(msg)
		}
		return s, cmd
	}
	if s.step == coStepInput &&
		s.focusZone == coZoneAmounts &&
		s.amountPreset == len(amountPresets)-1 {
		var cmd tea.Cmd
		s.amountInput, cmd =
			s.amountInput.Update(msg)
		return s, cmd
	}
	return s, nil
}

// ── Async message handlers ─────────────────────────────

func (s *ChannelOpenScreen) handleOpenResult(
	msg channelOpenResultMsg,
) (Screen, tea.Cmd) {
	s.inFlight = false
	if msg.err != nil {
		s.error = msg.err.Error()
	} else {
		s.txid = msg.txid
		s.error = ""
	}
	s.step = coStepResult
	return s, nil
}

// ── Form actions ───────────────────────────────────────

func (s *ChannelOpenScreen) clearForm() *ChannelOpenScreen {
	s.peerIdx = 0
	s.peerConfirmed = false
	s.customPubkey = ""
	s.customHost = ""
	s.customAlias = ""
	s.amountPreset = 0
	s.amountConfirmed = false
	s.amountInput = newChanAmountInput()
	s.private = true
	s.taproot = true
	s.toggleIdx = 0
	s.focusZone = coZonePeers
	s.btnIdx = 1
	s.error = ""
	return s
}

func (s *ChannelOpenScreen) submitOpenChannel() (
	Screen, tea.Cmd,
) {
	// Validate peer confirmed
	if !s.peerConfirmed {
		s.error = "Select a peer first"
		return s, nil
	}
	pubkey := s.selectedPubkey()
	if pubkey == "" {
		s.error = "Select a peer first"
		return s, nil
	}

	// Validate amount confirmed
	if !s.amountConfirmed {
		s.error = "Select a channel size first"
		return s, nil
	}

	// Validate amount value
	isCustom :=
		s.amountPreset == len(amountPresets)-1
	if isCustom {
		amt, err := parseCustomAmount(
			s.amountInput.Value())
		if err != nil {
			s.error = err.Error()
			return s, nil
		}
		s.amount = amt
	} else {
		s.amount = amountPresets[s.amountPreset]
	}

	s.error = ""
	s.confirmBtnIdx = 1
	s.step = coStepConfirm
	return s, nil
}

func (s *ChannelOpenScreen) submitCustomPeer() (
	Screen, tea.Cmd,
) {
	pubkey := strings.TrimSpace(
		s.pubkeyInput.Value())
	host := strings.TrimSpace(
		s.hostInput.Value())
	if pubkey == "" {
		s.error = "Pubkey is required"
		return s, nil
	}
	if len(pubkey) != 66 {
		s.error = "Pubkey must be 66 hex chars"
		return s, nil
	}
	if host == "" {
		s.error = "Host required"
		return s, nil
	}
	s.customPubkey = pubkey
	s.customHost = host
	s.customAlias = pubkey[:16] + "..."
	s.error = ""
	// Return to input with custom peer confirmed
	s.peerIdx = len(s.peerList)
	s.peerConfirmed = true
	s.step = coStepInput
	s.focusZone = coZoneAmounts
	s.amountPreset = 0
	return s, nil
}

// ── Helpers ────────────────────────────────────────────

func (s *ChannelOpenScreen) selectedPubkey() string {
	if s.peerIdx < len(s.peerList) {
		return s.peerList[s.peerIdx].Pubkey
	}
	return s.customPubkey
}

func (s *ChannelOpenScreen) selectedHost() string {
	if s.peerIdx < len(s.peerList) {
		return s.peerList[s.peerIdx].Host
	}
	return s.customHost
}

func (s *ChannelOpenScreen) selectedAlias() string {
	if s.peerIdx < len(s.peerList) {
		return s.peerList[s.peerIdx].Alias
	}
	if s.customAlias != "" {
		return s.customAlias
	}
	return "Custom peer"
}

// ── Views ──────────────────────────────────────────────

func (s *ChannelOpenScreen) viewInput(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Open Channel")

	if !s.ctx.Cfg.HasLND() ||
		!s.ctx.Cfg.WalletExists() {
		p.dim("Create wallet first.")
		return p.render()
	}
	if s.ctx.Status == nil ||
		!s.ctx.Status.lndResponding {
		p.dim("Waiting for LND...")
		return p.render()
	}

	balText := "unknown"
	if s.ctx.Status.lndBalance != "" {
		balText = s.ctx.Status.lndBalance + " sats"
	}
	p.field("On-chain: ", balText)
	p.blank()

	isFocused := s.ctx.ContentFocused

	// ── Peer list ──
	p.line(" " + theme.Header.Render("Select a peer:"))
	for i, peer := range s.peerList {
		prefix := " "
		style := theme.Value
		isCursor := isFocused &&
			s.focusZone == coZonePeers &&
			s.peerIdx == i
		isConfirmed := s.peerConfirmed &&
			s.peerIdx == i
		if isCursor {
			prefix = "▸"
			style = theme.Action
		}
		if isConfirmed {
			prefix = "✓"
			style = theme.Action
		}
		name := peer.Alias
		if len(name) > 20 {
			name = name[:20]
		}
		tags := ""
		if peer.TorOnly {
			tags += " (Tor)"
		}
		if peer.Curated {
			tags += " ★"
		}
		p.line(fmt.Sprintf(" %s %s%s",
			prefix, style.Render(name),
			theme.Dim.Render(tags)))
	}
	// [Custom peer] option
	customPrefix := " "
	customStyle := theme.Value
	customCursor := isFocused &&
		s.focusZone == coZonePeers &&
		s.peerIdx == len(s.peerList)
	customConfirmed := s.peerConfirmed &&
		s.peerIdx == len(s.peerList)
	if customCursor {
		customPrefix = "▸"
		customStyle = theme.Action
	}
	if customConfirmed {
		customPrefix = "✓"
		customStyle = theme.Action
	}
	customLabel := "[Custom peer]"
	if s.customPubkey != "" {
		customLabel = fmt.Sprintf("[%s]",
			s.customAlias)
	}
	p.line(fmt.Sprintf(" %s %s",
		customPrefix,
		customStyle.Render(customLabel)))
	p.blank()

	// ── Amount presets ──
	p.line(" " + theme.Header.Render("Channel size:"))
	for i, amt := range amountPresets {
		prefix := " "
		style := theme.Value
		isCursor := isFocused &&
			s.focusZone == coZoneAmounts &&
			s.amountPreset == i
		isConfirmed := s.amountConfirmed &&
			s.amountPreset == i
		if isCursor {
			prefix = "▸"
			style = theme.Action
		}
		if isConfirmed {
			prefix = "✓"
			style = theme.Action
		}
		if amt == 0 && s.amountPreset == i {
			p.line(fmt.Sprintf(" %s %s",
				prefix, style.Render("Custom:")))
			inputW := w - 6
			if inputW > 20 {
				inputW = 20
			}
			ai := s.amountInput
			ai.SetWidth(inputW)
			p.line("   " + ai.View())
			continue
		}
		p.line(fmt.Sprintf(" %s %s",
			prefix, style.Render(presetLabel(amt))))
	}
	p.blank()

	// ── Toggles ──
	p.line(" " + theme.Header.Render("Channel type:"))
	toggleFocused := isFocused &&
		s.focusZone == coZoneToggles
	s.addToggles(p, toggleFocused)

	// ── Error ──
	p.appendError(s.error)

	// ── Buttons pinned to bottom ──
	btnFocused := isFocused &&
		s.focusZone == coZoneButtons
	return p.renderWithBottomButtons(
		[]string{"Clear", "Open Channel"},
		s.btnIdx, btnFocused, h)
}

func (s *ChannelOpenScreen) addToggles(
	p *paneBuilder, focused bool,
) {
	p.line(renderToggleSwitch(
		"Private", "Public",
		s.private,
		focused && s.toggleIdx == 0))
	p.line(renderToggleSwitch(
		"Taproot", "Legacy",
		s.taproot,
		focused && s.toggleIdx == 1))
}

// renderToggleSwitch renders a Sparrow-style toggle:
//
//	Private    ●━━○   (off)
//	Private    ○━━●   (on, highlighted with theme color)
func renderToggleSwitch(
	label string, altLabel string,
	on bool, focused bool,
) string {
	displayLabel := altLabel
	if on {
		displayLabel = label
	}
	padded := fmt.Sprintf("%-12s", displayLabel)

	var toggle string
	if on {
		knob := theme.Action.Render("●")
		track := theme.Action.Render("━━")
		dot := theme.Dim.Render("○")
		toggle = dot + track + knob
	} else {
		knob := theme.Value.Render("●")
		track := theme.Dim.Render("━━")
		dot := theme.Dim.Render("○")
		toggle = knob + track + dot
	}

	prefix := "  "
	labelStyle := theme.Value
	if focused {
		prefix = " " + theme.Action.Render("▸")
		labelStyle = theme.Action
	}

	return prefix + " " + labelStyle.Render(padded) + toggle
}

func (s *ChannelOpenScreen) viewCustomPeer(
	w int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Custom Peer")

	isFocused := s.ctx.ContentFocused

	p.input("Node Pubkey:",
		s.pubkeyInput,
		isFocused &&
			s.customZone == coCustomZonePubkey)
	p.blank()
	p.input("Host (host:port):",
		s.hostInput,
		isFocused &&
			s.customZone == coCustomZoneHost)

	p.appendError(s.error)

	p.blank()
	btnFocused := isFocused &&
		s.customZone == coCustomZoneButtons
	p.buttons(
		[]string{"Cancel", "Continue"},
		s.customBtnIdx, btnFocused)

	return p.render()
}

func (s *ChannelOpenScreen) viewConfirm(
	w int,
) string {
	p := newPane(w)
	p.title(theme.Warning, "Confirm Channel Open")

	p.field("Peer:    ", s.selectedAlias())
	p.field("Amount:  ",
		formatSats(s.amount)+" sats")

	chanType := "public"
	if s.private {
		chanType = "private"
	}
	if s.taproot {
		chanType += ", taproot"
	}
	p.field("Type:    ", chanType)
	p.blank()

	p.labelLine("Pubkey:")
	p.monoWrap(s.selectedPubkey())
	p.blank()
	p.warn("Spend " +
		formatSats(s.amount) + " sats?")

	p.appendError(s.error)

	p.blank()
	p.buttons(
		[]string{"Go Back", "Confirm"},
		s.confirmBtnIdx, s.ctx.ContentFocused)

	return p.render()
}

func (s *ChannelOpenScreen) viewOpening(
	w int,
) string {
	p := newPane(w)
	p.title(theme.Header, "Opening Channel...")
	p.line(" " + theme.Value.Render(
		"Connecting to peer and broadcasting tx."))
	p.blank()
	p.dim("May take up to 2 minutes over Tor.")
	p.dim("Do not close the terminal.")
	return p.render()
}

func (s *ChannelOpenScreen) viewResult(
	w int,
) string {
	p := newPane(w)

	if s.error != "" {
		p.title(theme.Warning, "Channel Open Failed")
		p.warnWrap(s.error)
	} else {
		p.title(theme.Success, "Channel Opening")
		p.line(" " + theme.Value.Render(
			"Funding tx broadcast successfully."))
		p.blank()
		p.field("Peer:   ", s.selectedAlias())
		p.field("Amount: ",
			formatSats(s.amount)+" sats")
		if s.txid != "" {
			p.blank()
			p.labelLine("TX ID:")
			p.monoWrap(s.txid)
		}
		p.blank()
		p.dim("Channel will appear as pending.")
	}

	return p.render()
}

// ── Helpbar bindings ───────────────────────────────────

func (s *ChannelOpenScreen) inputBindings() []key.Binding {
	switch s.focusZone {
	case coZonePeers:
		return s.peerListBindings()
	case coZoneAmounts:
		return s.amountListBindings()
	case coZoneToggles:
		return s.toggleBindings()
	case coZoneButtons:
		return s.buttonBindings()
	}
	return nil
}

func (s *ChannelOpenScreen) peerListBindings() []key.Binding {
	binds := []key.Binding{
		key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "select")),
		key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("⇥", "next section")),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm")),
		kSidebar,
	}
	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("shift+tab"),
				key.WithHelp("⇧⇥", "prev section")))
	}
	binds = append(binds, kQuit)
	return binds
}

func (s *ChannelOpenScreen) amountListBindings() []key.Binding {
	binds := []key.Binding{
		key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "select")),
		key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("⇥", "next section")),
		key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("⇧⇥", "prev section")),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm")),
		kSidebar,
	}
	binds = append(binds, kQuit)
	return binds
}

func (s *ChannelOpenScreen) toggleBindings() []key.Binding {
	binds := []key.Binding{
		key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "toggle")),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "toggle")),
		key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("⇥", "next section")),
		key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("⇧⇥", "prev section")),
	}
	binds = append(binds, kQuit)
	return binds
}

func (s *ChannelOpenScreen) buttonBindings() []key.Binding {
	var binds []key.Binding
	if s.btnIdx == 0 {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left"),
				key.WithHelp("←", "sidebar")),
			key.NewBinding(
				key.WithKeys("right"),
				key.WithHelp("→", "button")))
	} else {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left", "right"),
				key.WithHelp("←→", "buttons")))
	}
	binds = append(binds,
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")),
		key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("⇧⇥", "prev section")))
	binds = append(binds, kQuit)
	return binds
}

func (s *ChannelOpenScreen) customPeerBindings() []key.Binding {
	switch s.customZone {
	case coCustomZonePubkey, coCustomZoneHost:
		binds := []key.Binding{
			key.NewBinding(
				key.WithKeys("left", "right"),
				key.WithHelp("←→", "cursor")),
			key.NewBinding(
				key.WithKeys("tab"),
				key.WithHelp("⇥", "next field")),
			key.NewBinding(
				key.WithKeys("shift+tab"),
				key.WithHelp("⇧⇥", "prev field")),
			kSidebar,
		}
		binds = append(binds, kQuit)
		return binds
	case coCustomZoneButtons:
		var binds []key.Binding
		if s.customBtnIdx == 0 {
			binds = append(binds,
				key.NewBinding(
					key.WithKeys("left"),
					key.WithHelp("←", "sidebar")),
				key.NewBinding(
					key.WithKeys("right"),
					key.WithHelp("→", "button")))
		} else {
			binds = append(binds,
				key.NewBinding(
					key.WithKeys("left", "right"),
					key.WithHelp("←→", "buttons")))
		}
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "select")),
			key.NewBinding(
				key.WithKeys("shift+tab"),
				key.WithHelp("⇧⇥", "prev field")))
		binds = append(binds, kQuit)
		return binds
	}
	return nil
}

func (s *ChannelOpenScreen) confirmBindings() []key.Binding {
	var binds []key.Binding
	if s.confirmBtnIdx == 0 {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left"),
				key.WithHelp("←", "sidebar")),
			key.NewBinding(
				key.WithKeys("right"),
				key.WithHelp("→", "button")))
	} else {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("left", "right"),
				key.WithHelp("←→", "buttons")))
	}
	binds = append(binds,
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")),
		kBack)
	if s.ctx.HasTabs {
		binds = append(binds,
			key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑", "tab bar")))
	}
	binds = append(binds, kQuit)
	return binds
}

func (s *ChannelOpenScreen) openingBindings() []key.Binding {
	return []key.Binding{kQuit}
}

// ── channelOpenPeers ───────────────────────────────────

// amountPresets defines the channel size options.
// The last entry (0) represents the custom amount option.
var amountPresets = []int64{
	100000, 250000, 500000,
	1000000, 2000000,
	0, // custom
}

func presetLabel(sats int64) string {
	if sats == 0 {
		return "Custom amount"
	}
	return formatSats(sats) + " sats"
}

func channelOpenPeers() []peerOption {
	return []peerOption{
		{
			Alias:       "LNBig",
			Pubkey:      "034ea80f8b148c750463546bd999bf7321a0e6dfc60aaf84bd0400a2e8d376c0d5",
			Host:        "qimt6abvc2iuexwrtl5tzyrygnu7mshjahvresve5hdli6nstdg7elyd.onion:9735",
			TorOnly:     true,
			Curated:     true,
			MinChanSize: 500000,
		},
		{
			Alias:       "ACINQ",
			Pubkey:      "03864ef025fde8fb587d989186ce6a4a186895ee44a926bfc370e2c366597a3f8f",
			Host:        "3.33.236.230:9735",
			TorOnly:     false,
			Curated:     true,
			MinChanSize: 400000,
		},
		{
			Alias:       "Zeus",
			Pubkey:      "031b301307574bbe9b9ac7b79cbe1700e31e544513eae0b5d7497483083f99e581",
			Host:        "45.79.192.236:9735",
			TorOnly:     false,
			Curated:     true,
			MinChanSize: 150000,
		},
	}
}
