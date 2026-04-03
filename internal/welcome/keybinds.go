package welcome

import (
	"charm.land/bubbles/v2/key"
)

// ── Key binding definitions ──────────────────────────────
// Each binding has a set of keys and a help description.
// The help description appears in the footer bar.

// Common bindings reused across states
var (
	kQuit = key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"))
	kBack = key.NewBinding(
		key.WithKeys("backspace"),
		key.WithHelp("⌫", "back"))
	kEnter = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"))
	kSidebar = key.NewBinding(
		key.WithKeys("left"),
		key.WithHelp("←", "sidebar"))
	kConfirm = key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "confirm"))
	kCancel = key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "cancel"))
)

// ── Sidebar bindings ─────────────────────────────────────

type sidebarBindings struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Quit  key.Binding
}

func newSidebarBindings() sidebarBindings {
	return sidebarBindings{
		Up: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑↓", "navigate")),
		Down: key.NewBinding(
			key.WithKeys("down", "tab"),
			key.WithHelp("", "")),
		Enter: key.NewBinding(
			key.WithKeys("enter", "right"),
			key.WithHelp("enter", "select")),
		Quit: kQuit,
	}
}

func (b sidebarBindings) ShortHelp() []key.Binding {
	return []key.Binding{b.Up, b.Enter, b.Quit}
}

func (b sidebarBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{{b.Up, b.Enter, b.Quit}}
}

// ── Tab bar bindings ─────────────────────────────────────

type tabBarBindings struct {
	LeftRight key.Binding
	Down      key.Binding
	Enter     key.Binding
	Back      key.Binding
	Quit      key.Binding
}

func newTabBarBindings(
	viewOnly bool, onTab bool,
) tabBarBindings {
	b := tabBarBindings{
		LeftRight: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "tabs")),
		Down: key.NewBinding(
			key.WithKeys("down", "tab"),
			key.WithHelp("↓", "content")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")),
		Back: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("⌫", "sidebar")),
		Quit: kQuit,
	}
	if viewOnly {
		b.Down.SetEnabled(false)
		b.Enter.SetEnabled(false)
	}
	if onTab {
		b.Back = key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("⌫", "close tab"))
	}
	return b
}

func (b tabBarBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{b.LeftRight}
	if b.Down.Enabled() {
		binds = append(binds, b.Down)
	}
	if b.Enter.Enabled() {
		binds = append(binds, b.Enter)
	}
	binds = append(binds, b.Back, b.Quit)
	return binds
}

func (b tabBarBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Confirm dialog bindings ──────────────────────────────

type confirmBindings struct {
	Yes key.Binding
	No  key.Binding
}

func newConfirmBindings() confirmBindings {
	return confirmBindings{
		Yes: kConfirm,
		No:  kCancel,
	}
}

func (b confirmBindings) ShortHelp() []key.Binding {
	return []key.Binding{b.Yes, b.No}
}

func (b confirmBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Text input flow bindings ─────────────────────────────

type textInputBindings struct {
	Enter   key.Binding
	Back    key.Binding
	Sidebar key.Binding
	TabBar  key.Binding
	Quit    key.Binding
}

func newTextInputBindings(hasTabs bool) textInputBindings {
	b := textInputBindings{
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "continue")),
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b textInputBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{b.Enter, b.Back, b.Sidebar}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b textInputBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Two-field input bindings (receive, custom peer) ──────

type twoFieldBindings struct {
	Tab     key.Binding
	Enter   key.Binding
	Back    key.Binding
	Sidebar key.Binding
	TabBar  key.Binding
	Quit    key.Binding
}

func newTwoFieldBindings(hasTabs bool) twoFieldBindings {
	b := twoFieldBindings{
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch field")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "continue")),
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b twoFieldBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.Tab, b.Enter, b.Back, b.Sidebar,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b twoFieldBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Channel confirm with toggle ──────────────────────────

type chanConfirmBindings struct {
	Yes     key.Binding
	Toggle  key.Binding
	Back    key.Binding
	Sidebar key.Binding
	TabBar  key.Binding
	Quit    key.Binding
}

func newChanConfirmBindings(
	hasTabs bool,
) chanConfirmBindings {
	b := chanConfirmBindings{
		Yes: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "confirm")),
		Toggle: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "toggle private")),
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b chanConfirmBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.Yes, b.Toggle, b.Back, b.Sidebar,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b chanConfirmBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── In-flight / waiting bindings ─────────────────────────

type waitingBindings struct {
	Quit key.Binding
}

func newWaitingBindings() waitingBindings {
	return waitingBindings{Quit: kQuit}
}

func (b waitingBindings) ShortHelp() []key.Binding {
	return []key.Binding{b.Quit}
}

func (b waitingBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Result screen bindings ───────────────────────────────

type resultBindings struct {
	Close key.Binding
	Quit  key.Binding
}

func newResultBindings() resultBindings {
	return resultBindings{
		Close: key.NewBinding(
			key.WithKeys("enter", "backspace"),
			key.WithHelp("enter/⌫", "close")),
		Quit: kQuit,
	}
}

func (b resultBindings) ShortHelp() []key.Binding {
	return []key.Binding{b.Close, b.Quit}
}

func (b resultBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── On-chain send confirm bindings ──────────────────────

type ocSendConfirmBindings struct {
	LeftRight key.Binding
	Enter     key.Binding
	Back      key.Binding
	TabBar    key.Binding
	Quit      key.Binding
}

func newOCSendConfirmBindings(
	hasTabs bool,
) ocSendConfirmBindings {
	b := ocSendConfirmBindings{
		LeftRight: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "buttons")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")),
		Back: kBack,
		TabBar: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b ocSendConfirmBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.LeftRight, b.Enter, b.Back,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b ocSendConfirmBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Addon detail bindings ────────────────────────────────

type addonDetailBindings struct {
	LeftRight key.Binding
	UpDown    key.Binding
	Enter     key.Binding
	Back      key.Binding
	Sidebar   key.Binding
	TabBar    key.Binding
	Quit      key.Binding
}

func newAddonDetailBindings(
	hasTabs bool,
) addonDetailBindings {
	b := addonDetailBindings{
		LeftRight: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "buttons")),
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "items")),
		Enter:   kEnter,
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b addonDetailBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.LeftRight, b.UpDown, b.Enter,
		b.Back, b.Sidebar,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b addonDetailBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Peer select bindings ─────────────────────────────────

type peerSelectBindings struct {
	UpDown  key.Binding
	Enter   key.Binding
	Back    key.Binding
	Sidebar key.Binding
	TabBar  key.Binding
	Quit    key.Binding
}

func newPeerSelectBindings(
	hasTabs bool,
) peerSelectBindings {
	b := peerSelectBindings{
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "peers")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select peer")),
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b peerSelectBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.UpDown, b.Enter, b.Back, b.Sidebar,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b peerSelectBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Amount select bindings ───────────────────────────────

type amountSelectBindings struct {
	UpDown  key.Binding
	Enter   key.Binding
	Back    key.Binding
	Sidebar key.Binding
	TabBar  key.Binding
	Quit    key.Binding
}

func newAmountSelectBindings(
	hasTabs bool,
) amountSelectBindings {
	b := amountSelectBindings{
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "amounts")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "continue")),
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b amountSelectBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.UpDown, b.Enter, b.Back, b.Sidebar,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b amountSelectBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Detail tab bindings (view-only) ──────────────────────

type detailTabBindings struct {
	Up      key.Binding
	Sidebar key.Binding
	Back    key.Binding
	Quit    key.Binding
}

func newDetailTabBindings(hasTabs bool) detailTabBindings {
	b := detailTabBindings{
		Up: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "tab bar")),
		Sidebar: kSidebar,
		Back: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("⌫", "close tab")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.Up.SetEnabled(false)
	}
	return b
}

func (b detailTabBindings) ShortHelp() []key.Binding {
	var binds []key.Binding
	if b.Up.Enabled() {
		binds = append(binds, b.Up)
	}
	binds = append(binds, b.Sidebar, b.Back, b.Quit)
	return binds
}

func (b detailTabBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}
