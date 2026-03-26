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
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"))
	kBack = key.NewBinding(
		key.WithKeys("backspace"),
		key.WithHelp("⌫", "back"))
	kEnter = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"))
	kUpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑↓", "navigate"))
	kLeftRight = key.NewBinding(
		key.WithKeys("left", "right"),
		key.WithHelp("←→", "navigate"))
	kSidebar = key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←", "sidebar"))
	kTabBar = key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑", "tab bar"))
	kConfirm = key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "confirm"))
	kCancel = key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "cancel"))
	kTab = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch field"))
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
			key.WithKeys("up", "k"),
			key.WithHelp("↑↓", "navigate")),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("", "")),
		Enter: key.NewBinding(
			key.WithKeys("enter", "right", "l"),
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
	Close     key.Binding
	Back      key.Binding
	Quit      key.Binding
}

func newTabBarBindings() tabBarBindings {
	return tabBarBindings{
		LeftRight: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "tabs")),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓", "content")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")),
		Close: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("✕", "close tab")),
		Back: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("⌫", "sidebar")),
		Quit: kQuit,
	}
}

func (b tabBarBindings) ShortHelp() []key.Binding {
	return []key.Binding{
		b.LeftRight, b.Down, b.Enter,
		b.Close, b.Back, b.Quit,
	}
}

func (b tabBarBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Channels home bindings ───────────────────────────────

type channelsHomeBindings struct {
	UpDown    key.Binding
	LeftRight key.Binding
	Enter     key.Binding
	Sidebar   key.Binding
	TabBar    key.Binding
	Quit      key.Binding
}

func newChannelsHomeBindings(
	hasTabs bool, onButton bool,
) channelsHomeBindings {
	b := channelsHomeBindings{
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "channels")),
		LeftRight: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "buttons")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "details")),
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	if onButton {
		b.Enter = key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"))
		b.UpDown = key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "channels"))
	} else {
		b.LeftRight.SetEnabled(false)
	}
	return b
}

func (b channelsHomeBindings) ShortHelp() []key.Binding {
	var binds []key.Binding
	binds = append(binds, b.UpDown)
	if b.LeftRight.Enabled() {
		binds = append(binds, b.LeftRight)
	}
	binds = append(binds, b.Enter, b.Sidebar)
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b channelsHomeBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Wallet home bindings ─────────────────────────────────

type walletHomeBindings struct {
	LeftRight key.Binding
	UpDown    key.Binding
	Enter     key.Binding
	Sidebar   key.Binding
	TabBar    key.Binding
	Quit      key.Binding
}

func newWalletHomeBindings(
	hasTabs bool, onButtons bool,
) walletHomeBindings {
	b := walletHomeBindings{
		LeftRight: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "buttons")),
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "payments")),
		Enter:   kEnter,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	if !onButtons {
		b.LeftRight.SetEnabled(false)
	}
	return b
}

func (b walletHomeBindings) ShortHelp() []key.Binding {
	var binds []key.Binding
	if b.LeftRight.Enabled() {
		binds = append(binds, b.LeftRight)
	}
	binds = append(binds, b.UpDown, b.Enter, b.Sidebar)
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b walletHomeBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Addons home bindings ─────────────────────────────────

type addonsHomeBindings struct {
	UpDown  key.Binding
	Enter   key.Binding
	Sidebar key.Binding
	TabBar  key.Binding
	Quit    key.Binding
}

func newAddonsHomeBindings(hasTabs bool) addonsHomeBindings {
	b := addonsHomeBindings{
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "select")),
		Enter:   kEnter,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b addonsHomeBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.UpDown, b.Enter, b.Sidebar,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b addonsHomeBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── System home bindings ─────────────────────────────────

type systemHomeBindings struct {
	UpDown    key.Binding
	LeftRight key.Binding
	Restart   key.Binding
	Stop      key.Binding
	Start     key.Binding
	Enter     key.Binding
	Sidebar   key.Binding
	TabBar    key.Binding
	Quit      key.Binding
}

func newSystemHomeBindings(
	hasTabs bool, onServices bool,
) systemHomeBindings {
	b := systemHomeBindings{
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "services")),
		LeftRight: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "buttons")),
		Restart: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "restart")),
		Stop: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "stop")),
		Start: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "start")),
		Enter:   kEnter,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	if !onServices {
		b.Restart.SetEnabled(false)
		b.Stop.SetEnabled(false)
		b.Start.SetEnabled(false)
		b.UpDown = key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "services"))
	} else {
		b.LeftRight.SetEnabled(false)
		b.Enter.SetEnabled(false)
	}
	return b
}

func (b systemHomeBindings) ShortHelp() []key.Binding {
	var binds []key.Binding
	binds = append(binds, b.UpDown)
	if b.LeftRight.Enabled() {
		binds = append(binds, b.LeftRight)
	}
	if b.Restart.Enabled() {
		binds = append(binds, b.Restart,
			b.Stop, b.Start)
	}
	if b.Enter.Enabled() {
		binds = append(binds, b.Enter)
	}
	binds = append(binds, b.Sidebar)
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b systemHomeBindings) FullHelp() [][]key.Binding {
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
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit")),
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
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit")),
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

// ── Send confirm / channel confirm bindings ──────────────

type payConfirmBindings struct {
	Yes     key.Binding
	Back    key.Binding
	Sidebar key.Binding
	TabBar  key.Binding
	Quit    key.Binding
}

func newPayConfirmBindings(hasTabs bool) payConfirmBindings {
	b := payConfirmBindings{
		Yes: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "confirm")),
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b payConfirmBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{b.Yes, b.Back, b.Sidebar}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b payConfirmBindings) FullHelp() [][]key.Binding {
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
			key.WithKeys("up", "k"),
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
	Enter key.Binding
	Back  key.Binding
	Quit  key.Binding
}

func newResultBindings() resultBindings {
	return resultBindings{
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "return")),
		Back: kBack,
		Quit: kQuit,
	}
}

func (b resultBindings) ShortHelp() []key.Binding {
	return []key.Binding{b.Enter, b.Quit}
}

func (b resultBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Receive waiting bindings ─────────────────────────────

type recvWaitingBindings struct {
	LeftRight key.Binding
	Enter     key.Binding
	Back      key.Binding
	Sidebar   key.Binding
	TabBar    key.Binding
	Quit      key.Binding
}

func newRecvWaitingBindings(
	hasTabs bool,
) recvWaitingBindings {
	b := recvWaitingBindings{
		LeftRight: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "buttons")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")),
		Back: kBack,
		Sidebar: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("←", "sidebar")),
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b recvWaitingBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.LeftRight, b.Enter, b.Back,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b recvWaitingBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── On-chain overview bindings ───────────────────────────

type onChainHomeBindings struct {
	LeftRight key.Binding
	UpDown    key.Binding
	Space     key.Binding
	Enter     key.Binding
	Back      key.Binding
	Sidebar   key.Binding
	TabBar    key.Binding
	Quit      key.Binding
}

func newOnChainHomeBindings(
	hasTabs bool, focus int,
) onChainHomeBindings {
	b := onChainHomeBindings{
		LeftRight: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "buttons")),
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "navigate")),
		Space: key.NewBinding(
			key.WithKeys("space"),
			key.WithHelp("space", "select")),
		Enter:   kEnter,
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	b.Space.SetEnabled(false)
	if focus == 0 {
		b.UpDown = key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "UTXOs"))
	} else if focus == 1 {
		b.UpDown = key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "UTXOs"))
		b.Enter = key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "details"))
		b.LeftRight.SetEnabled(false)
		b.Space.SetEnabled(true)
	} else {
		b.UpDown = key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "transactions"))
		b.Enter = key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "tx details"))
		b.LeftRight.SetEnabled(false)
	}
	return b
}

func (b onChainHomeBindings) ShortHelp() []key.Binding {
	var binds []key.Binding
	if b.LeftRight.Enabled() {
		binds = append(binds, b.LeftRight)
	}
	binds = append(binds, b.UpDown)
	if b.Space.Enabled() {
		binds = append(binds, b.Space)
	}
	binds = append(binds, b.Enter,
		b.Back, b.Sidebar)
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b onChainHomeBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── On-chain send amount bindings ────────────────────────

type ocSendAmountBindings struct {
	UpDown    key.Binding
	LeftRight key.Binding
	TabToggle key.Binding
	Enter     key.Binding
	Back      key.Binding
	Sidebar   key.Binding
	TabBar    key.Binding
	Quit      key.Binding
}

func newOCSendAmountBindings(
	hasTabs bool,
) ocSendAmountBindings {
	b := ocSendAmountBindings{
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "fields")),
		LeftRight: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "fee tier")),
		TabToggle: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "continue")),
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit")),
	}
	b.TabToggle.SetEnabled(false)
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b ocSendAmountBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.UpDown, b.LeftRight, b.TabToggle,
		b.Enter, b.Back, b.Sidebar,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b ocSendAmountBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Fullscreen (QR, URL) bindings ────────────────────────

type fullscreenBindings struct {
	Back key.Binding
	Quit key.Binding
}

func newFullscreenBindings() fullscreenBindings {
	return fullscreenBindings{
		Back: kBack,
		Quit: kQuit,
	}
}

func (b fullscreenBindings) ShortHelp() []key.Binding {
	return []key.Binding{b.Back, b.Quit}
}

func (b fullscreenBindings) FullHelp() [][]key.Binding {
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
			key.WithKeys("up", "k"),
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
			key.WithKeys("up", "k"),
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
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit")),
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
			key.WithKeys("up", "k"),
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

// ── Receive input bindings (up/down fields) ──────────────

type recvInputBindings struct {
	UpDown  key.Binding
	Enter   key.Binding
	Back    key.Binding
	Sidebar key.Binding
	TabBar  key.Binding
	Quit    key.Binding
}

func newRecvInputBindings(
	hasTabs bool,
) recvInputBindings {
	b := recvInputBindings{
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "switch field")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "create invoice")),
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit")),
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b recvInputBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.UpDown, b.Enter, b.Back, b.Sidebar,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b recvInputBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Send input bindings (with cursor) ────────────────────

type sendInputBindings struct {
	Cursor  key.Binding
	Enter   key.Binding
	Back    key.Binding
	Sidebar key.Binding
	TabBar  key.Binding
	Quit    key.Binding
}

func newSendInputBindings(
	hasTabs bool,
) sendInputBindings {
	b := sendInputBindings{
		Cursor: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←→", "cursor")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "decode")),
		Back:    kBack,
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit")),
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b sendInputBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{
		b.Cursor, b.Enter, b.Back, b.Sidebar,
	}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b sendInputBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Close type select bindings ───────────────────────────

type closeTypeBindings struct {
	UpDown  key.Binding
	Enter   key.Binding
	Back    key.Binding
	Sidebar key.Binding
	Quit    key.Binding
}

func newCloseTypeBindings() closeTypeBindings {
	return closeTypeBindings{
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "close type")),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select")),
		Back:    kBack,
		Sidebar: kSidebar,
		Quit:    kQuit,
	}
}

func (b closeTypeBindings) ShortHelp() []key.Binding {
	return []key.Binding{
		b.UpDown, b.Enter, b.Back,
		b.Sidebar, b.Quit,
	}
}

func (b closeTypeBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}

// ── Channel history bindings ─────────────────────────────

type channelHistoryBindings struct {
	UpDown  key.Binding
	Back    key.Binding
	Sidebar key.Binding
	TabBar  key.Binding
	Quit    key.Binding
}

func newChannelHistoryBindings(
	hasTabs bool,
) channelHistoryBindings {
	b := channelHistoryBindings{
		UpDown: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "channels")),
		Back: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("⌫", "close tab")),
		Sidebar: kSidebar,
		TabBar: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "tab bar")),
		Quit: kQuit,
	}
	if !hasTabs {
		b.TabBar.SetEnabled(false)
	}
	return b
}

func (b channelHistoryBindings) ShortHelp() []key.Binding {
	binds := []key.Binding{b.UpDown, b.Back, b.Sidebar}
	if b.TabBar.Enabled() {
		binds = append(binds, b.TabBar)
	}
	binds = append(binds, b.Quit)
	return binds
}

func (b channelHistoryBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{b.ShortHelp()}
}
