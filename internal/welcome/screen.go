package welcome

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/lndrpc"
)

// ── Screen interface ────────────────────────────────────
// Each tab's content implements this interface.
// The three routing tables (key dispatch, view dispatch,
// helpbar dispatch) collapse into single interface calls.

type Screen interface {
	// HandleKey processes keyboard input.
	// Returns the (possibly new) screen and a command.
	HandleKey(key string, msg tea.KeyPressMsg) (Screen, tea.Cmd)

	// HandleMsg processes async results (e.g.
	// invoiceSettledMsg), paste messages, and any
	// future non-key message types.
	HandleMsg(msg tea.Msg) (Screen, tea.Cmd)

	// View renders the screen content.
	// Width and height are passed as parameters since
	// they're only needed at render time.
	View(w, h int) string

	// HelpBindings returns the current key bindings
	// for the helpbar.
	HelpBindings() []key.Binding

	// Init returns any initial command (e.g.
	// createInvoiceCmd for receive flow after input).
	// Called once when the screen is mounted to a tab.
	Init() tea.Cmd
}

// ── ScreenContext ────────────────────────────────────────
// Pointer semantics — screens always see current data.
// Model owns the single instance; screens store a
// *ScreenContext on creation. When Model updates
// m.status on a new statusMsg, every screen's View()
// automatically sees current data through the pointer
// chain — zero refresh plumbing.

type ScreenContext struct {
	Cfg            *config.AppConfig
	LndClient      *lndrpc.Client
	Status         *statusMsg
	HasTabs        bool // varies by section; Model sets before calling View/HelpBindings
	ContentFocused bool // true when content pane has focus (not tab bar, not sidebar)
}

// ── Screen-to-Model messages ────────────────────────────
// Screens emit these via tea.Cmd. They flow through the
// Bubble Tea event loop and arrive in Model's Update like
// any other message. Model handles them in its main
// switch — no synchronous cmd inspection needed.

// closeTabMsg tells Model to close the active tab.
type closeTabMsg struct{}

// openTabMsg tells Model to open a new tab with the
// given screen.
type openTabMsg struct {
	Kind   tabKind
	Label  string
	Screen Screen
}

// focusSidebarMsg tells Model to move focus to the
// sidebar.
type focusSidebarMsg struct{}

// focusTabBarMsg tells Model to move focus to the tab
// bar.
type focusTabBarMsg struct{}

// showQRMsg tells Model to show the fullscreen QR view.
type showQRMsg struct {
	URL      string
	Label    string
	ReturnTo wSubview
}

// refreshStatusMsg tells Model to re-fetch node status.
// Distinct from statusMsg which carries actual status
// data.
type refreshStatusMsg struct{}

// ── Message emitters ────────────────────────────────────
// Screens use these as tea.Cmd values. Each is a
// func() tea.Msg that returns instantly — the message
// flows through the Bubble Tea runtime on the next tick.

func emitCloseTab() tea.Msg {
	return closeTabMsg{}
}

func emitFocusSidebar() tea.Msg {
	return focusSidebarMsg{}
}

func emitFocusTabBar() tea.Msg {
	return focusTabBarMsg{}
}

func emitRefreshStatus() tea.Msg {
	return refreshStatusMsg{}
}
