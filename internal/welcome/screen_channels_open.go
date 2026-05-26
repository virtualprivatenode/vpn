package welcome

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Channel open screen steps ──────────────────────────

type chanOpenStep int

const (
	coStepInput       chanOpenStep = iota // peer + amount + fee + toggles + buttons
	coStepCustomPeer                      // pubkey + host fields + Go Back/Continue
	coStepCoinControl                     // UTXO table + Go Back / Confirm
	coStepConfirm                         // summary + Go Back / Confirm
	coStepOpening                         // in-flight
	coStepResult                          // success or error
)

// ── Focus zones for coStepInput ────────────────────────

const (
	coZonePeers   = 0
	coZoneAmounts = 1
	coZoneFee     = 2
	coZoneToggles = 3
	coZoneButtons = 4
)

// ── Focus zones for coStepCustomPeer ───────────────────

const (
	coCustomZonePubkey  = 0
	coCustomZoneHost    = 1
	coCustomZoneButtons = 2
)

// ── Focus zones for coStepCoinControl ─────────────────

const (
	coCCZoneList    = 0
	coCCZoneButtons = 1
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
	amountIdx   int // 0=coin control btn, 1=amount
	amountInput AmountInput
	amount      int64
	fundMax     bool

	// Fee rate
	feeInput AmountInput
	feeTiers [4]feeTier

	// Toggles
	private   bool
	taproot   bool
	toggleIdx int

	// UTXO selection (coin control)
	utxos             []lndrpc.UTXO
	txs               []lndrpc.OnChainTx
	utxoSelected      map[int]bool
	utxoSelectedTotal int64
	utxoOutpoints     []string
	utxoCursor        int
	utxoFetched       bool // lazy fetch guard
	ccZone            int  // sub-step focus zone
	ccBtnIdx          int  // sub-step button index

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
	s := &ChannelOpenScreen{
		ctx:          ctx,
		step:         coStepInput,
		peerList:     channelOpenPeers(),
		amountInput:  NewAmountInput(),
		feeInput:     NewFeeInput(),
		private:      true,
		taproot:      true,
		btnIdx:       1,
		pubkeyInput:  newChanPubkeyInput(),
		hostInput:    newChanHostInput(),
		customBtnIdx: 1,
		utxoSelected: make(map[int]bool),
	}
	return s
}

// ── Screen interface ────────────────────────────────────

func (s *ChannelOpenScreen) Init() tea.Cmd {
	return tea.Batch(
		fetchChannelUtxosCmd(s.ctx.LndClient),
		fetchChannelTxsCmd(s.ctx.LndClient),
		fetchFeeTiersCmd(s.ctx.Cfg))
}

func (s *ChannelOpenScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch s.step {
	case coStepInput:
		return s.handleInputKey(keyStr, msg)
	case coStepCustomPeer:
		return s.handleCustomPeerKey(keyStr, msg)
	case coStepCoinControl:
		return s.handleCoinControlKey(keyStr)
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
	case tabActivatedMsg:
		// Always refetch on tab re-focus so data
		// reflects any changes since last viewed,
		// matching channel history's pattern.
		return s, tea.Batch(
			fetchChannelUtxosCmd(s.ctx.LndClient),
			fetchChannelTxsCmd(s.ctx.LndClient),
			fetchFeeTiersCmd(s.ctx.Cfg))
	case tea.PasteMsg:
		return s.handlePaste(msg)
	case channelOpenResultMsg:
		return s.handleOpenResult(msg)
	case coUtxoListMsg:
		return s.handleUtxoList(msg)
	case coTxListMsg:
		return s.handleTxList(msg)
	case feeTiersMsg:
		return s.handleFeeTiers(msg)
	}
	return s, nil
}

func (s *ChannelOpenScreen) View(w, h int) string {
	switch s.step {
	case coStepInput:
		return s.viewInput(w, h)
	case coStepCustomPeer:
		return s.viewCustomPeer(w, h)
	case coStepCoinControl:
		return s.viewCoinControl(w, h)
	case coStepConfirm:
		return s.viewConfirm(w, h)
	case coStepOpening:
		return s.viewOpening(w, h)
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
	case coStepCoinControl:
		return s.coinControlBindings()
	case coStepConfirm:
		return actionButtonBindings(
			s.confirmBtnIdx, s.ctx.HasTabs)
	case coStepOpening:
		return inFlightBindings()
	case coStepResult:
		return resultBindings(s.ctx.HasTabs)
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
	case coZoneFee:
		return s.handleFeeZoneKey(keyStr, msg)
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
		return s, nil
	case "backspace":
		return s, emitFocusParent
	}
	return s, nil
}

func (s *ChannelOpenScreen) handleAmountListKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	onCoinCtrl := s.amountIdx == 0
	editing := s.amountIdx == 1 &&
		s.amountInput.Focused()
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if editing && !s.amountInput.Empty() {
			cmd := s.amountInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusSidebar
	case "right":
		if editing {
			cmd := s.amountInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil
	case "up":
		if onCoinCtrl {
			s.enterPeersBackward()
		} else {
			// Amount → coin control
			if editing {
				s.amountInput.Blur()
			}
			s.amountIdx = 0
		}
		return s, nil
	case "down":
		if onCoinCtrl {
			// Coin control → amount
			s.amountIdx = 1
			if !s.amountConfirmed {
				s.amountInput.Focus()
			}
			return s, nil
		}
		// Amount: advance to fee
		if editing && !s.amountInput.Empty() {
			if !s.confirmAmountAndAdvance() {
				return s, nil
			}
			return s, nil
		}
		if editing {
			s.amountInput.Blur()
		}
		s.focusZone = coZoneFee
		s.feeInput.Focus()
		return s, nil
	case "tab":
		if editing && !s.amountInput.Empty() {
			if !s.confirmAmountAndAdvance() {
				return s, nil
			}
			return s, nil
		}
		if editing {
			s.amountInput.Blur()
		}
		s.focusZone = coZoneFee
		s.feeInput.Focus()
		return s, nil
	case "shift+tab":
		if editing {
			s.amountInput.Blur()
		}
		s.enterPeersBackward()
		return s, nil
	case "backspace":
		if editing {
			cmd := s.amountInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusParent
	case "enter":
		if onCoinCtrl {
			// Open coin control sub-step
			s.ccZone = coCCZoneList
			s.ccBtnIdx = 1
			s.error = ""
			s.step = coStepCoinControl
			return s, nil
		}
		if s.amountConfirmed && !editing {
			// Unlock editing on auto-confirmed amount
			s.amountConfirmed = false
			s.amountInput.Focus()
			return s, nil
		}
		if editing {
			if !s.confirmAmountAndAdvance() {
				return s, nil
			}
			return s, nil
		}
		return s, nil
	}
	if editing {
		cmd := s.amountInput.Update(tea.Msg(msg))
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
			s.enterFeeBackward()
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
		s.enterFeeBackward()
		return s, nil
	case "enter":
		s.focusZone = coZoneButtons
		return s, nil
	case "space":
		switch s.toggleIdx {
		case 0:
			s.private = !s.private
		case 1:
			s.taproot = !s.taproot
		}
		return s, nil
	case "backspace":
		return s, emitFocusParent
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
		s.enterTogglesBackward()
		return s, nil
	case "tab":
		return s, nil
	case "shift+tab":
		s.enterTogglesBackward()
		return s, nil
	case "enter":
		switch s.btnIdx {
		case 0: // Clear
			return s.clearForm(), nil
		case 1: // Open Channel
			return s.submitOpenChannel()
		}
		return s, nil
	case "backspace":
		return s, emitFocusParent
	}
	return s, nil
}

// ── Fee zone key handling ─────────────────────────────

func (s *ChannelOpenScreen) handleFeeZoneKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if !s.feeInput.Empty() {
			cmd := s.feeInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, emitFocusSidebar
	case "right":
		if !s.feeInput.Empty() {
			cmd := s.feeInput.Update(tea.Msg(msg))
			return s, cmd
		}
		return s, nil
	case "up":
		s.feeInput.Blur()
		s.enterAmountsBackward()
		return s, nil
	case "down":
		s.feeInput.Blur()
		s.focusZone = coZoneToggles
		s.toggleIdx = 0
		return s, nil
	case "tab":
		s.feeInput.Blur()
		s.focusZone = coZoneToggles
		s.toggleIdx = 0
		return s, nil
	case "shift+tab":
		s.feeInput.Blur()
		s.enterAmountsBackward()
		return s, nil
	case "backspace":
		cmd := s.feeInput.Update(tea.Msg(msg))
		return s, cmd
	case "enter":
		s.feeInput.Blur()
		s.focusZone = coZoneToggles
		s.toggleIdx = 0
		return s, nil
	default:
		cmd := s.feeInput.Update(tea.Msg(msg))
		return s, cmd
	}
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
		s.amountIdx == 1 &&
		s.amountInput.Focused() {
		cmd := s.amountInput.Update(msg)
		return s, cmd
	}
	if s.step == coStepInput &&
		s.focusZone == coZoneFee {
		cmd := s.feeInput.Update(msg)
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
	if msg.err == nil {
		return s, emitRefreshStatus
	}
	return s, nil
}

func (s *ChannelOpenScreen) handleFeeTiers(
	msg feeTiersMsg,
) (Screen, tea.Cmd) {
	if msg.err != nil {
		return s, nil
	}
	s.feeTiers = msg.tiers
	// Pre-fill fee input if still empty
	if s.feeInput.Empty() &&
		msg.tiers[0].SatPerVB > 0 {
		s.feeInput.SetSats(
			int64(msg.tiers[0].SatPerVB))
	}
	return s, nil
}

// ── Form actions ───────────────────────────────────────

// validateCustomAmount checks the custom amount field is
// non-empty and within the 20,000 — 1,000,000,000 sats
// channel-size bounds. The upper bound matches LND's wumbo
// channel limit (10 BTC) enabled via protocol.wumbo-channels
// in lnd.conf.
func (s *ChannelOpenScreen) validateCustomAmount() (int64, string) {
	if s.amountInput.Empty() {
		return 0, "empty amount"
	}
	n := s.amountInput.Sats()
	if n < 20000 {
		return 0, "min 20,000 sats"
	}
	if n > 1000000000 {
		return 0, "max 1,000,000,000 sats (10 BTC)"
	}
	return n, ""
}

// confirmAmountAndAdvance validates the amount input,
// commits it, detects FundMax, and advances to fee zone.
// Returns false on validation failure.
func (s *ChannelOpenScreen) confirmAmountAndAdvance() bool {
	n, errMsg := s.validateCustomAmount()
	if errMsg != "" {
		s.error = errMsg
		return false
	}
	s.error = ""
	s.amount = n
	// FundMax when amount matches selected UTXO total
	s.fundMax = len(s.utxoSelected) > 0 &&
		n == s.utxoSelectedTotal
	s.amountConfirmed = true
	s.amountInput.Blur()
	s.focusZone = coZoneFee
	s.feeInput.Focus()
	return true
}

// ── Backward-entry helpers ─────────────────────────────
//
// When focus returns to a zone from forward (via
// shift+tab or up), that zone's "confirmed" state is
// dropped — the user is re-entering to potentially make
// changes. The cursor lands on the same item they
// previously committed (so they can see where they were),
// but there's no checkmark. Committing requires explicit
// forward navigation (enter/tab/down).
//
// Each helper owns one zone's reset logic. Handlers in
// other zones call these when their keystrokes cause a
// backward transition, rather than inlining the reset.
// See design-decisions.md: "Backward-entry helpers for
// multi-zone form screens."

// enterPeersBackward: focus returned to peers from the
// amounts zone. Decommits the peer while keeping the
// cursor on the previously-selected row.
func (s *ChannelOpenScreen) enterPeersBackward() {
	s.focusZone = coZonePeers
	s.peerConfirmed = false
	s.error = ""
}

// enterAmountsBackward: focus returned to amounts from
// the fee zone. Decommits the amount but preserves the
// value in the input for review/editing. If there's a
// committed amount, populates the AmountInput so the
// user can arrow left/right through the number.
func (s *ChannelOpenScreen) enterAmountsBackward() {
	s.focusZone = coZoneAmounts
	s.amountConfirmed = false
	s.error = ""
	s.amountIdx = 1
	s.amountInput.Focus()
}

// enterFeeBackward: focus returned to fee from the
// toggles zone. Refocuses the fee input for editing.
func (s *ChannelOpenScreen) enterFeeBackward() {
	s.focusZone = coZoneFee
	s.feeInput.Focus()
	s.error = ""
}

// enterTogglesBackward: focus returned to toggles from
// the buttons zone. Toggles have no "draft vs committed"
// distinction — each toggle is its own commit — so the
// only state to reset is the error message. Included for
// symmetry so the navigation handlers in the buttons zone
// don't inline the focus assignment.
func (s *ChannelOpenScreen) enterTogglesBackward() {
	s.focusZone = coZoneToggles
	s.error = ""
}

func (s *ChannelOpenScreen) clearForm() *ChannelOpenScreen {
	s.peerIdx = 0
	s.peerConfirmed = false
	s.customPubkey = ""
	s.customHost = ""
	s.customAlias = ""
	s.amountIdx = 0
	s.amountConfirmed = false
	s.amountInput.Clear()
	s.amount = 0
	s.fundMax = false
	s.feeInput = NewFeeInput()
	if s.feeTiers[0].SatPerVB > 0 {
		s.feeInput.SetSats(
			int64(s.feeTiers[0].SatPerVB))
	}
	s.private = true
	s.taproot = true
	s.toggleIdx = 0
	s.utxoSelected = make(map[int]bool)
	s.utxoSelectedTotal = 0
	s.utxoOutpoints = nil
	s.utxoCursor = 0
	s.utxoFetched = false
	s.txs = nil
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

	// Mandatory coin control
	if len(s.utxoSelected) == 0 {
		s.error = "Select UTXOs in Coin control first"
		return s, nil
	}

	// Validate amount confirmed
	if !s.amountConfirmed {
		s.error = "Select a channel size first"
		return s, nil
	}

	// Validate amount value
	if !s.fundMax {
		n, errMsg := s.validateCustomAmount()
		if errMsg != "" {
			s.error = errMsg
			return s, nil
		}
		s.amount = n
	}

	// FundMax minimum check
	if s.fundMax && s.utxoSelectedTotal < 20000 {
		s.error = "min 20,000 sats"
		return s, nil
	}

	// Custom amount vs selected UTXOs check
	feeRate := s.feeInput.Sats()
	if !s.fundMax {
		estFee := estimateSimpleFee(
			len(s.utxoSelected), 2, feeRate)
		if s.amount+estFee > s.utxoSelectedTotal {
			s.error = "Amount plus fee exceeds " +
				"selected UTXOs"
			return s, nil
		}
	}

	s.error = ""
	s.confirmBtnIdx = 1
	s.step = coStepConfirm
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
		balText = formatSats(
			parseBalance(s.ctx.Status.lndBalance)) +
			" sats"
	}
	p.field("On-Chain Balance: ", balText)
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
			prefix = theme.NavActive.Render("▸")
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
		customPrefix = theme.NavActive.Render("▸")
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

	// ── Channel size: coin control + amount ──
	p.line(" " + theme.Header.Render("Channel size:"))

	amtFocused := isFocused &&
		s.focusZone == coZoneAmounts

	// Coin control button
	ccPrefix := " "
	ccStyle := theme.Value
	if amtFocused && s.amountIdx == 0 {
		ccPrefix = theme.NavActive.Render("▸")
		ccStyle = theme.Action
	}
	var ccLabel string
	if len(s.utxoSelected) > 0 {
		ccLabel = fmt.Sprintf(
			"[Coin control: %d UTXO (%s sats)]",
			len(s.utxoSelected),
			formatSats(s.utxoSelectedTotal))
	} else {
		ccLabel = "[Coin control]"
	}
	p.line(fmt.Sprintf(" %s %s",
		ccPrefix, ccStyle.Render(ccLabel)))

	// Amount line
	amtPrefix := " "
	amtStyle := theme.Value
	amtCursor := amtFocused && s.amountIdx == 1
	if amtCursor {
		amtPrefix = theme.NavActive.Render("▸")
		amtStyle = theme.Action
	}
	if s.amountConfirmed {
		amtPrefix = "✓"
		amtStyle = theme.Action
	}

	hasCoinCtrl := len(s.utxoSelected) > 0
	if s.amountConfirmed && !s.amountInput.Focused() {
		// Auto-confirmed or committed state
		annotation := ""
		if hasCoinCtrl &&
			s.amount == s.utxoSelectedTotal {
			annotation = theme.Dim.Render(
				"  full UTXO(s), no change")
		}
		p.line(fmt.Sprintf(" %s %s%s",
			amtPrefix,
			amtStyle.Render(
				formatSats(s.amount)+" sats"),
			annotation))
	} else {
		// Editing state with input field
		inputW := w - 14
		if inputW > 20 {
			inputW = 20
		}
		s.amountInput.SetWidth(inputW)
		amtLine := fmt.Sprintf(" %s %s %s",
			amtPrefix,
			amtStyle.Render("Amount:"),
			s.amountInput.View())
		// Change annotation
		if hasCoinCtrl && !s.amountInput.Empty() {
			typed := s.amountInput.Sats()
			if typed == s.utxoSelectedTotal {
				amtLine += theme.Dim.Render(
					"  full UTXO(s), no change")
			} else if typed < s.utxoSelectedTotal {
				change := s.utxoSelectedTotal - typed
				amtLine += theme.Warning.Render(
					fmt.Sprintf("  ~%s sats change",
						formatSats(change)))
			}
		}
		p.line(amtLine)
	}
	p.blank()

	// ── Fee rate ──
	feeActive := isFocused &&
		s.focusZone == coZoneFee
	feeMarker := " "
	if feeActive {
		feeMarker = theme.NavActive.Render("▸")
	}
	p.line(" " + theme.Header.Render(
		"Fee Rate (sat/vB):"))
	p.line(" " + feeMarker + " " + s.feeInput.View())
	hints := formatFeeHints(s.feeTiers)
	if hints != "" {
		p.line("  " + theme.Dim.Render(hints))
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

// renderToggleSwitch renders a bracket-style toggle with
// both options visible. The knob slides left/right inside
// the housing to indicate the active selection.
//
//	Public [ ━━● ] Private   (on=true, Private selected)
//	Legacy [ ●━━ ] Taproot   (on=false, Legacy selected)
func renderToggleSwitch(
	label string, altLabel string,
	on bool, focused bool,
) string {
	leftW := 8 // right-align left labels for alignment

	activeStyle := theme.Value
	if focused {
		activeStyle = theme.Action
	}

	var leftStyled, rightStyled, toggle string
	bracket := theme.Dim
	if on {
		leftStyled = theme.Dim.Render(
			fmt.Sprintf("%*s", leftW, altLabel))
		rightStyled = activeStyle.Render(label)
		toggle = bracket.Render("[ ") +
			theme.Dim.Render("━━") +
			activeStyle.Render("●") +
			bracket.Render(" ]")
	} else {
		leftStyled = activeStyle.Render(
			fmt.Sprintf("%*s", leftW, altLabel))
		rightStyled = theme.Dim.Render(label)
		toggle = bracket.Render("[ ") +
			activeStyle.Render("●") +
			theme.Dim.Render("━━") +
			bracket.Render(" ]")
	}

	prefix := "  "
	if focused {
		prefix = " " + theme.NavActive.Render("▸")
	}

	return prefix + leftStyled + " " +
		toggle + " " + rightStyled
}

// ── Helpbar bindings ───────────────────────────────────

func (s *ChannelOpenScreen) inputBindings() []key.Binding {
	switch s.focusZone {
	case coZonePeers:
		return s.peerListBindings()
	case coZoneAmounts:
		return s.amountListBindings()
	case coZoneFee:
		return s.feeZoneBindings()
	case coZoneToggles:
		return s.toggleBindings()
	case coZoneButtons:
		return s.buttonBindings()
	}
	return nil
}

func (s *ChannelOpenScreen) peerListBindings() []key.Binding {
	binds := []key.Binding{
		kUpDownSelect, kTabNext, kEnterConfirm,
		kSidebar,
	}
	if s.ctx.HasTabs {
		binds = append(binds, kShiftTabBar)
	}
	binds = append(binds, kBack, kQuit)
	return binds
}

func (s *ChannelOpenScreen) amountListBindings() []key.Binding {
	return []key.Binding{
		kUpDownSelect, kTabNext,
		bind("⇧tab", "peers", "shift+tab"),
		kEnterConfirm, kSidebar, kBack, kQuit,
	}
}

func (s *ChannelOpenScreen) toggleBindings() []key.Binding {
	return []key.Binding{
		kUpDownSelect,
		bind("space", "toggle", "space"),
		kEnterNext,
		bind("⇧tab", "fee", "shift+tab"),
		kBack, kQuit,
	}
}

func (s *ChannelOpenScreen) feeZoneBindings() []key.Binding {
	return []key.Binding{
		kLeftRightCursor, kTabNext,
		bind("⇧tab", "amount", "shift+tab"),
		kSidebar, kBack, kQuit,
	}
}

func (s *ChannelOpenScreen) buttonBindings() []key.Binding {
	binds := buttonNav(s.btnIdx)
	binds = append(binds, kEnter,
		bind("⇧tab", "toggles", "shift+tab"),
		kBack, kQuit)
	return binds
}

// ── channelOpenPeers ───────────────────────────────────

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
			Alias:       "Zeus",
			Pubkey:      "031b301307574bbe9b9ac7b79cbe1700e31e544513eae0b5d7497483083f99e581",
			Host:        "r46dwvxcdri754hf6n3rwexmc53h5x4natg5g6hidnxfzejm5xrqn2id.onion:9735",
			TorOnly:     true,
			Curated:     true,
			MinChanSize: 150000,
		},
	}
}
