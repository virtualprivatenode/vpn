package welcome

import (
	"fmt"
	"os/exec"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── WalletCreateScreen ──────────────────────────────────
// Three-step wallet creation flow that runs entirely
// inside the TUI:
//
//   walletConfirm — privacy + seed warnings, Cancel /
//                   Proceed buttons. The user reads the
//                   warnings and explicitly opts in.
//
//   walletWaiting — "Waiting for LND..." centered in the
//                   pane while WaitForLND polls the REST
//                   API. Async, can fail with a 120s
//                   timeout.
//
//   walletExec    — tea.ExecProcess hands the terminal
//                   to a single bash command that runs
//                   `lncli create` interactively, then
//                   prompts the user to type
//                   "I SAVED MY SEED" to confirm. SIGINT
//                   is trapped after lncli succeeds so
//                   ctrl+c cannot escape the seed
//                   confirmation loop.
//
// On success, the screen emits walletCreatedMsg. The
// Model handles that message by transforming this tab
// in place into an AutoUnlockScreen — the user goes
// straight from "I SAVED MY SEED" into auto-unlock
// configuration without any tab juggling. See the
// walletCreatedMsg case in update.go.
//
// On error (timeout, lncli failure, user ctrl+c during
// lncli create itself), the screen transitions to an
// error view with a Done button. No config changes
// occur on the failure path.

type walletCreateStep int

const (
	walletConfirm walletCreateStep = iota
	walletWaiting
	walletExec
	walletErr
)

// Messages emitted by the wallet creation flow.
// All three are unique to this screen so they don't
// collide with anything else in the routing table.

// walletLNDReadyMsg is delivered when WaitForLND
// returns. The screen uses err to decide whether to
// proceed to the bash exec or show the timeout error.
type walletLNDReadyMsg struct{ err error }

// walletExecDoneMsg is delivered by the tea.ExecProcess
// callback when the bash wrapper exits. err is nil on
// success (lncli created the wallet AND the user typed
// "I SAVED MY SEED"). Non-nil if lncli failed, the user
// ctrl+c'd during lncli create, or the SSH session was
// killed.
type walletExecDoneMsg struct{ err error }

// walletCreatedMsg tells Model that wallet creation
// finished successfully. Model creates the lndClient
// (it didn't exist before this point) and transforms
// this tab in place into an AutoUnlockScreen. See
// update.go for the handler.
type walletCreatedMsg struct{}

type WalletCreateScreen struct {
	ctx  *ScreenContext
	step walletCreateStep

	// Confirm step — 0=Cancel, 1=Proceed
	btnIdx int

	// Error captured from a failed step
	resultErr error
}

func NewWalletCreateScreen(
	ctx *ScreenContext,
) *WalletCreateScreen {
	return &WalletCreateScreen{
		ctx:    ctx,
		step:   walletConfirm,
		btnIdx: 1, // default focus on Proceed
	}
}

// ── Screen interface ────────────────────────────────────

func (s *WalletCreateScreen) Init() tea.Cmd {
	return nil
}

func (s *WalletCreateScreen) HandleKey(
	keyStr string, msg tea.KeyPressMsg,
) (Screen, tea.Cmd) {
	// Error state — Done button only
	if s.step == walletErr {
		switch keyStr {
		case "ctrl+c":
			return s, tea.Quit
		case "enter", "backspace":
			return s, emitCloseTab
		case "left":
			return s, emitFocusSidebar
		case "up":
			if s.ctx.HasTabs {
				return s, emitFocusTabBar
			}
		}
		return s, nil
	}

	// Waiting and exec steps — block all keys.
	// During walletWaiting we're polling LND and there's
	// nothing meaningful to interact with. During
	// walletExec the bash command owns the terminal and
	// the TUI isn't even visible — but ctrl+c here would
	// quit the entire TUI, which we don't want either.
	if s.step == walletWaiting || s.step == walletExec {
		if keyStr == "ctrl+c" {
			return s, tea.Quit
		}
		return s, nil
	}

	// Confirm step
	switch keyStr {
	case "ctrl+c":
		return s, tea.Quit
	case "left":
		if s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.btnIdx < 1 {
			s.btnIdx++
		}
		return s, nil
	case "up":
		if s.ctx.HasTabs {
			return s, emitFocusTabBar
		}
		return s, nil
	case "backspace":
		return s, emitCloseTab
	case "enter":
		if s.btnIdx == 0 {
			return s, emitCloseTab
		}
		return s.startWaitingForLND()
	}
	return s, nil
}

func (s *WalletCreateScreen) startWaitingForLND() (
	Screen, tea.Cmd,
) {
	s.step = walletWaiting
	return s, waitForLNDCmd()
}

func waitForLNDCmd() tea.Cmd {
	return func() tea.Msg {
		err := installer.WaitForLND()
		return walletLNDReadyMsg{err: err}
	}
}

// ── HandleMsg ───────────────────────────────────────────

func (s *WalletCreateScreen) HandleMsg(
	msg tea.Msg,
) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case walletLNDReadyMsg:
		if m.err != nil {
			s.step = walletErr
			s.resultErr = fmt.Errorf(
				"LND did not respond in time. "+
					"Make sure bitcoind is running "+
					"and try again. (%v)", m.err)
			return s, nil
		}
		// LND is ready — kick off the bash wrapper
		s.step = walletExec
		return s, s.startExecProcess()

	case walletExecDoneMsg:
		if m.err != nil {
			s.step = walletErr
			s.resultErr = fmt.Errorf(
				"wallet creation failed: %v", m.err)
			return s, nil
		}
		// Success. Model handles walletCreatedMsg by
		// creating the lndClient and transforming this
		// tab into the auto-unlock screen.
		return s, func() tea.Msg {
			return walletCreatedMsg{}
		}
	}
	return s, nil
}

// startExecProcess builds and returns the bash wrapper
// as a tea.ExecProcess. The wrapper:
//  1. Runs `lncli create` interactively. The user types
//     a wallet password, confirms it, chooses 'n' for
//     a new seed, optionally adds a cipher seed
//     passphrase, and sees the 24 words.
//  2. If lncli exits 0, sets `trap ” INT` to ignore
//     ctrl+c, then loops prompting for "I SAVED MY
//     SEED" until the user types it exactly.
//  3. Exits 0 only after the user types the exact
//     confirmation phrase.
//
// ── Asymmetric ctrl+c handling is deliberate ────────
// Step 1 allows ctrl+c. Step 2 blocks it. This mirrors
// the atomicity boundary inside lncli itself: until
// GenSeed completes, no seed exists and no wallet file
// has been written, so a canceled context leaves the
// node in a clean state and the user can retry. Once
// InitWallet has run, the wallet file is on disk and
// the only remaining risk is a user who aborts before
// writing down their seed — which would be
// unrecoverable. Blocking SIGINT in step 2 removes
// that exit path entirely.
//
// A subtlety worth knowing: lncli doesn't make a gRPC
// call during the password / seed-type / passphrase
// prompts. Those are local readline-style inputs.
// The first (and only, for seed generation) RPC is
// GenSeed, which fires after the cipher passphrase
// prompt is submitted. If the user ctrl+c's during
// the earlier prompts, lncli's SIGINT handler arms
// cancellation on a context that doesn't exist yet,
// and the terminal appears to swallow the signal. The
// cancellation then fires the instant GenSeed is
// called, and LND returns:
//
//	unable to generate seed: rpc error:
//	code = Canceled desc = context canceled
//
// This looks like an error but is actually the
// intended cancellation path — no wallet is written,
// no seed is displayed, the user is safe to retry.
// Do not "fix" this by blocking SIGINT in step 1:
// that would remove a legitimate escape hatch that
// depends on lncli's atomicity guarantee for its
// correctness.
//
// If the user ctrl+c's during step 2, the trap
// ignores the signal and the loop continues — the
// only escape is typing the exact phrase or killing
// the SSH session.
func (s *WalletCreateScreen) startExecProcess() tea.Cmd {
	net := s.ctx.Cfg.NetworkConfig()
	script := `clear
echo
echo "  ==================================================="
echo "    Lightning Wallet Creation"
echo "  ==================================================="
echo
echo "  You're about to create the Lightning wallet that"
echo "  will hold your Bitcoin on this node."
echo
echo "  In a moment you'll be asked to:"
echo "    1. Choose a wallet password (you'll use this"
echo "       to unlock the wallet)"
echo "    2. Confirm the password"
echo "    3. Press 'n' to generate a new 24-word seed"
echo "    4. Skip the cipher seed passphrase by pressing"
echo "       Enter (most users do)"
echo
echo "  Your seed phrase will appear in this terminal."
echo "  Make sure nobody is looking over your shoulder."
echo
sudo -u bitcoin lncli ` +
		`--lnddir=/var/lib/lnd ` +
		`--network=` + net.LNCLINetwork + ` create && {
  trap '' INT
  echo
  echo "  ==================================================="
  echo "  Your 24-word seed is displayed above."
  echo "  Write it down NOW."
  echo
  echo "  Storage options:"
  echo "    * Pen and paper, kept somewhere safe"
  echo "    * An offline password manager (e.g. KeePass)"
  echo
  echo "  Digital copies (screenshots, photos, cloud"
  echo "  storage) are NOT safe."
  echo
  while true; do
    printf "  Type I SAVED MY SEED: "
    read line
    [ "$line" = "I SAVED MY SEED" ] && break
    echo "  Please type exactly: I SAVED MY SEED"
  done
  echo
  echo "  Seed confirmed. Returning to TUI..."
  sleep 1
  printf '\033[2J\033[3J\033[H'
}`

	cmd := exec.Command("bash", "-c", script)
	return tea.ExecProcess(cmd,
		func(err error) tea.Msg {
			return walletExecDoneMsg{err: err}
		})
}

// ── View ────────────────────────────────────────────────

func (s *WalletCreateScreen) View(
	w, h int,
) string {
	switch s.step {
	case walletWaiting:
		return renderWaitingForLND(w, h)
	case walletExec:
		// During exec, bash owns the terminal and the
		// TUI isn't visible. If something does render
		// briefly during the handoff, show a neutral
		// placeholder.
		return renderWaitingForLND(w, h)
	case walletErr:
		return s.viewError(w, h)
	}
	return s.viewConfirm(w, h)
}

func (s *WalletCreateScreen) viewConfirm(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused
	p := newPane(w)

	p.title(theme.Header,
		"Create Your LND Lightning Wallet")

	p.line(" " + theme.Warning.Render(
		"IMPORTANT — read carefully before"+
			" you proceed."))
	p.blank()
	p.line(" " + theme.Value.Render(
		"LND will display your 24-word seed"+
			" phrase ONCE."))
	p.line(" " + theme.Value.Render(
		"It cannot be shown again. This seed"+
			" is the ONLY way"))
	p.line(" " + theme.Value.Render(
		"to recover your funds if anything"+
			" happens to this"))
	p.line(" " + theme.Value.Render(
		"server. No one can help you if you"+
			" lose it."))
	p.blank()
	p.line(" " + theme.Value.Render(
		"Before you proceed:"))
	p.blank()
	p.line(" " + theme.Value.Render(
		"  • Make sure you are in a private area"))
	p.line(" " + theme.Value.Render(
		"  • Have pen and paper ready, OR"))
	p.line(" " + theme.Value.Render(
		"  • Have an offline password manager"+
			" ready (e.g. KeePass)"))
	p.blank()
	p.line(" " + theme.Value.Render(
		"The wallet creation process will ask"+
			" you to:"))
	p.blank()
	p.line(" " + theme.Value.Render(
		"  • Set a wallet password"))
	p.line(" " + theme.Value.Render(
		"  • Confirm the password"))
	p.line(" " + theme.Value.Render(
		"  • Press 'n' to generate a new seed"))
	p.line(" " + theme.Value.Render(
		"  • Skip the cipher seed passphrase"+
			" (press Enter)"))
	p.line(" " + theme.Value.Render(
		"  • WRITE DOWN your 24 words"))
	p.blank()
	p.dim(
		"Once you press Proceed, this screen will be")
	p.dim(
		"replaced by the wallet creation prompts.")

	return p.renderWithBottomButtons(
		[]string{"Cancel", "Proceed"},
		s.btnIdx, isFocused, h)
}

func (s *WalletCreateScreen) viewError(
	w, h int,
) string {
	isFocused := s.ctx.ContentFocused
	p := newPane(w)

	p.title(theme.Header,
		"Wallet Creation Failed")
	p.blank()
	if s.resultErr != nil {
		p.warnWrap(s.resultErr.Error())
	}
	p.blank()
	p.line(" " + theme.Value.Render(
		"You can close this and try again."))

	return p.renderWithBottomButtons(
		[]string{"Done"}, 0, isFocused, h)
}

// ── HelpBindings ────────────────────────────────────────

func (s *WalletCreateScreen) HelpBindings() []key.Binding {
	if s.step == walletWaiting || s.step == walletExec {
		return []key.Binding{kQuit}
	}

	if s.step == walletErr {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "close")),
			kSidebar,
			kQuit,
		}
	}

	// Confirm step
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
