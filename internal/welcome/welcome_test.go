package welcome

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
)

func testModel() Model {
	cfg := config.Default()
	return NewModel(cfg, "0.0.0-test")
}

func testModelWithLND() Model {
	cfg := config.Default()
	cfg.LNDInstalled = true
	return NewModel(cfg, "0.0.0-test")
}

func testModelFullStack() Model {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.SyncthingInstalled = true
	cfg.LndHubInstalled = true
	return NewModel(cfg, "0.0.0-test")
}

func testStore(t *testing.T) *config.Store {
	t.Helper()
	dir := t.TempDir()
	return &config.Store{
		Dir:  dir,
		Path: filepath.Join(dir, "config.json"),
	}
}

func testModelWithStore(t *testing.T, cfg *config.AppConfig) Model {
	t.Helper()
	store := testStore(t)
	return NewTestModel(cfg, "0.0.0-test", store)
}

func testModelWalletReady() Model {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	return m
}

func testModelWalletWithStatus() Model {
	m := testModelWalletReady()
	m.status = &statusMsg{
		lndResponding:  true,
		lndBalance:     "1000000",
		lndPubkey:      "03abc123def456789012345678901234567890123456789012345678901234dead",
		lndSyncedChain: true,
		lndSyncedGraph: true,
		channels: []channelInfo{
			{
				ChanID: 123, PeerAlias: "ACINQ",
				RemotePubkey:  "03abc123def456",
				Capacity:      250000,
				LocalBalance:  150000,
				RemoteBalance: 100000,
				Active:        true,
				Initiator:     true,
			},
			{
				ChanID: 456, PeerAlias: "Zeus",
				RemotePubkey:  "02def789abc012",
				Capacity:      500000,
				LocalBalance:  200000,
				RemoteBalance: 300000,
				Active:        true,
			},
		},
		services:  map[string]bool{"tor": true, "bitcoind": true, "lnd": true},
		btcSynced: true,
	}
	return m
}

// ── Tab Navigation ───────────────────────────────────────

func TestInitialState(t *testing.T) {
	m := testModel()
	if m.activeTab != tabDashboard {
		t.Errorf("initial tab: got %d, want %d (dashboard)",
			m.activeTab, tabDashboard)
	}
	if m.subview != svNone {
		t.Errorf("initial subview: got %d, want %d (none)",
			m.subview, svNone)
	}
	if m.sysCard != cardServices {
		t.Errorf("initial card: got %d, want %d (services)",
			m.sysCard, cardServices)
	}
	if m.cardActive {
		t.Error("card should not be active initially")
	}
	if m.walletPaneFocused {
		t.Error("walletPaneFocused should be false initially")
	}
}

func TestInitialButtonGroups(t *testing.T) {
	m := testModel()

	// Tab bar
	if len(m.tabBar.Labels) != 5 {
		t.Errorf("tab bar labels: got %d, want 5", len(m.tabBar.Labels))
	}
	if m.tabBar.ActiveIndex != 0 {
		t.Errorf("tab bar active: got %d, want 0", m.tabBar.ActiveIndex)
	}
	if !m.tabBar.Focused {
		t.Error("tab bar should be focused initially")
	}

	// Wallet sidebar
	if len(m.walletSidebar.Labels) != 4 {
		t.Errorf("sidebar labels: got %d, want 4", len(m.walletSidebar.Labels))
	}
	if m.walletSidebar.ActiveIndex != walletSectionTransactions {
		t.Errorf("sidebar active: got %d, want %d",
			m.walletSidebar.ActiveIndex, walletSectionTransactions)
	}
	if m.walletSidebar.Focused {
		t.Error("sidebar should not be focused initially (dashboard is active)")
	}
	if !m.walletSidebar.Disabled[walletSectionOnChain] {
		t.Error("On-Chain button should be disabled")
	}
	if m.walletSidebar.Width != 18 {
		t.Errorf("sidebar width: got %d, want 18", m.walletSidebar.Width)
	}
}

func TestTabForward(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24

	expected := []wTab{tabWallet, tabPairing, tabAddons,
		tabSystem, tabDashboard}
	for _, want := range expected {
		newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = newM.(Model)
		if m.activeTab != want {
			t.Errorf("after tab: got %d, want %d",
				m.activeTab, want)
		}
	}
}

func TestTabBackward(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = newM.(Model)
	if m.activeTab != tabSystem {
		t.Errorf("after shift+tab: got %d, want %d (system)",
			m.activeTab, tabSystem)
	}
}

func TestNumberKeySwitchesTab(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24

	tests := []struct {
		key  string
		want wTab
	}{
		{"1", tabDashboard},
		{"2", tabWallet},
		{"3", tabPairing},
		{"4", tabAddons},
		{"5", tabSystem},
	}
	for _, tt := range tests {
		r := []rune(tt.key)
		newM, _ := m.Update(tea.KeyPressMsg{Code: r[0], Text: tt.key})
		result := newM.(Model)
		if result.activeTab != tt.want {
			t.Errorf("key %s: got tab %d, want %d",
				tt.key, result.activeTab, tt.want)
		}
	}
}

// ── Tab Switching Focus Management ───────────────────────

func TestSwitchToWalletFocusesSidebar(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabDashboard

	newM, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = newM.(Model)
	if m.activeTab != tabWallet {
		t.Fatalf("expected wallet tab, got %d", m.activeTab)
	}
	if !m.walletSidebar.Focused {
		t.Error("wallet sidebar should be focused when switching to wallet tab")
	}
	if m.walletPaneFocused {
		t.Error("wallet pane should not be focused when switching to wallet tab")
	}
}

func TestSwitchAwayFromWalletBlursSidebar(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabWallet
	m.walletSidebar.Focus()
	m.walletPaneFocused = true

	newM, _ := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m = newM.(Model)
	if m.walletSidebar.Focused {
		t.Error("sidebar should be blurred after leaving wallet tab")
	}
	if m.walletPaneFocused {
		t.Error("walletPaneFocused should be reset after leaving wallet tab")
	}
}

// ── System Tab Card Navigation ───────────────────────────

func TestSystemCardNavigation(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabSystem
	m.sysCard = cardServices

	// right: Services → SysStats
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = newM.(Model)
	if m.sysCard != cardSysStats {
		t.Errorf("right from services: got %d, want %d (sysstats)",
			m.sysCard, cardSysStats)
	}

	// down: SysStats → Update
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = newM.(Model)
	if m.sysCard != cardUpdate {
		t.Errorf("down from sysstats: got %d, want %d (update)",
			m.sysCard, cardUpdate)
	}

	// left: Update → Bitcoin
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = newM.(Model)
	if m.sysCard != cardBitcoin {
		t.Errorf("left from update: got %d, want %d (bitcoin)",
			m.sysCard, cardBitcoin)
	}

	// up: Bitcoin → Services
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = newM.(Model)
	if m.sysCard != cardServices {
		t.Errorf("up from bitcoin: got %d, want %d (services)",
			m.sysCard, cardServices)
	}
}

// ── Dashboard Navigation ─────────────────────────────────

func TestDashboardChannelNavigation(t *testing.T) {
	m := testModelWalletWithStatus()
	m.activeTab = tabDashboard
	m.chanCursor = 0

	// Down to second channel
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = newM.(Model)
	if m.chanCursor != 1 {
		t.Errorf("down: got cursor %d, want 1", m.chanCursor)
	}

	// Down again should clamp (only 2 channels)
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = newM.(Model)
	if m.chanCursor != 1 {
		t.Errorf("down clamped: got cursor %d, want 1", m.chanCursor)
	}

	// Up back to first
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = newM.(Model)
	if m.chanCursor != 0 {
		t.Errorf("up: got cursor %d, want 0", m.chanCursor)
	}

	// Up should clamp at 0
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = newM.(Model)
	if m.chanCursor != 0 {
		t.Errorf("up clamped: got cursor %d, want 0", m.chanCursor)
	}
}

func TestDashboardEnterOpensChannelDetail(t *testing.T) {
	m := testModelWalletWithStatus()
	m.activeTab = tabDashboard
	m.chanCursor = 0

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svChannelDetail {
		t.Errorf("enter on dashboard channel: got %d, want %d",
			m.subview, svChannelDetail)
	}
}

func TestDashboardEnterNoChannels(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabDashboard
	m.status = &statusMsg{
		lndResponding: true,
		channels:      []channelInfo{},
		services:      map[string]bool{},
	}

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("enter with no channels: got %d, want %d",
			m.subview, svNone)
	}
}

func TestDashboardOpenChannel(t *testing.T) {
	m := testModelWalletWithStatus()
	m.activeTab = tabDashboard

	newM, _ := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = newM.(Model)
	if m.subview != svChannelOpen {
		t.Errorf("o on dashboard: got %d, want %d",
			m.subview, svChannelOpen)
	}
}

func TestDashboardNoNavWithoutChannels(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabDashboard
	m.status = nil

	newM, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = newM.(Model)
	if m.chanCursor != 0 {
		t.Errorf("no channels, down should no-op: got %d", m.chanCursor)
	}
}

func TestDashboardShowsFullPubkey(t *testing.T) {
	m := testModelWalletWithStatus()
	m.activeTab = tabDashboard

	view := m.View()
	// Full pubkey should be displayed (not truncated)
	if !strings.Contains(view.Content, "03abc123def456789012345678901234567890123456789012345678901234dead") {
		t.Error("dashboard should show full pubkey")
	}
}

func TestDashboardShowsP2PMode(t *testing.T) {
	m := testModelWalletWithStatus()
	m.activeTab = tabDashboard

	view := m.View()
	if !strings.Contains(view.Content, "Tor only") {
		t.Error("dashboard should show P2P mode")
	}
}

// ── System Tab Card Activation ───────────────────────────

func TestEnterActivatesServicesCard(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabSystem
	m.sysCard = cardServices

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if !m.cardActive {
		t.Error("enter on services card should activate it")
	}
	if m.svcCursor != 0 {
		t.Errorf("service cursor should start at 0, got %d",
			m.svcCursor)
	}
}

func TestEnterActivatesSysStatsCard(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabSystem
	m.sysCard = cardSysStats

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if !m.cardActive {
		t.Error("enter on sysstats card should activate it")
	}
}

func TestBackspaceDeactivatesCard(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabSystem
	m.sysCard = cardServices
	m.cardActive = true

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.cardActive {
		t.Error("backspace should deactivate card")
	}
}

// ── Software Card → Install/Update Actions ───────────────

func TestSoftwareCardInstallLND(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabSystem
	m.sysCard = cardUpdate

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.shellAction != svLNDInstall {
		t.Errorf("enter on software without LND: got %d, want %d",
			m.shellAction, svLNDInstall)
	}
}

func TestSoftwareCardCreateWallet(t *testing.T) {
	m := testModelWithLND()
	m.width = 80
	m.height = 24
	m.activeTab = tabSystem
	m.sysCard = cardUpdate

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.shellAction != svWalletCreate {
		t.Errorf("enter on software no wallet: got %d, want %d",
			m.shellAction, svWalletCreate)
	}
}

func TestSoftwareCardUpdateConfirm(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabSystem
	m.sysCard = cardUpdate
	m.latestVersion = "9.9.9"

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if !m.updateConfirm {
		t.Error("enter with new version should set updateConfirm")
	}

	// Cancel with n
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = newM.(Model)
	if m.updateConfirm {
		t.Error("pressing n should cancel updateConfirm")
	}
}

func TestSoftwareCardUpdateConfirmYes(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabSystem
	m.sysCard = cardUpdate
	m.latestVersion = "9.9.9"
	m.updateConfirm = true

	newM, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = newM.(Model)
	if m.shellAction != svSelfUpdate {
		t.Errorf("y should trigger update: got %d, want %d",
			m.shellAction, svSelfUpdate)
	}
}

func TestSoftwareCardNoUpdateWhenCurrent(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabSystem
	m.sysCard = cardUpdate
	m.latestVersion = "0.0.0-test" // matches m.version

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.updateConfirm {
		t.Error("should not confirm when already on latest")
	}
}

// ── Wallet Tab — Sidebar Navigation ──────────────────────

func TestWalletSidebarUpDown(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletSidebar.Focus()

	// Start at Transactions (0), go down to Send (1)
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = newM.(Model)
	if m.walletSidebar.FocusIndex != walletSectionSend {
		t.Errorf("down from 0: got focus %d, want %d",
			m.walletSidebar.FocusIndex, walletSectionSend)
	}

	// Down to Receive (2)
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = newM.(Model)
	if m.walletSidebar.FocusIndex != walletSectionReceive {
		t.Errorf("down from 1: got focus %d, want %d",
			m.walletSidebar.FocusIndex, walletSectionReceive)
	}

	// Down again should skip On-Chain (3, disabled) and wrap to Transactions (0)
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = newM.(Model)
	if m.walletSidebar.FocusIndex != walletSectionTransactions {
		t.Errorf("down from 2 (skip disabled): got focus %d, want %d",
			m.walletSidebar.FocusIndex, walletSectionTransactions)
	}

	// Up from Transactions should skip On-Chain and go to Receive
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = newM.(Model)
	if m.walletSidebar.FocusIndex != walletSectionReceive {
		t.Errorf("up from 0 (skip disabled): got focus %d, want %d",
			m.walletSidebar.FocusIndex, walletSectionReceive)
	}
}

func TestWalletSidebarEnterTransactions(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletSidebar.Focus()
	m.walletSidebar.FocusIndex = walletSectionTransactions

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if !m.walletPaneFocused {
		t.Error("enter on Transactions should focus content pane")
	}
	if m.walletSidebar.Focused {
		t.Error("sidebar should be blurred after entering content pane")
	}
	if m.walletSidebar.ActiveIndex != walletSectionTransactions {
		t.Errorf("active index: got %d, want %d",
			m.walletSidebar.ActiveIndex, walletSectionTransactions)
	}
}

func TestWalletSidebarRightEntersPane(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletSidebar.Focus()
	m.walletSidebar.FocusIndex = walletSectionTransactions

	newM, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = newM.(Model)
	if !m.walletPaneFocused {
		t.Error("right arrow should enter content pane")
	}
}

func TestWalletSidebarEnterSendOpensSubview(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletSidebar.Focus()
	m.walletSidebar.SetFocus(walletSectionSend)

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svSend {
		t.Errorf("enter on Send: got subview %d, want %d",
			m.subview, svSend)
	}
}

func TestWalletSidebarEnterReceiveOpensSubview(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletSidebar.Focus()
	m.walletSidebar.SetFocus(walletSectionReceive)

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svReceive {
		t.Errorf("enter on Receive: got subview %d, want %d",
			m.subview, svReceive)
	}
}

func TestWalletSidebarEnterOnChainDisabled(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletSidebar.Focus()
	// Can't focus disabled button via SetFocus, so FocusIndex stays at 0
	m.walletSidebar.SetFocus(walletSectionOnChain)

	// Focus should not have moved to On-Chain since it's disabled
	if m.walletSidebar.FocusIndex == walletSectionOnChain {
		t.Error("should not be able to focus disabled On-Chain button")
	}
}

// ── Wallet Content Pane Navigation ───────────────────────

func TestWalletContentPaneBackToSidebar(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletPaneFocused = true
	m.walletSidebar.Blur()

	// Left arrow goes back to sidebar
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = newM.(Model)
	if m.walletPaneFocused {
		t.Error("left should return focus to sidebar")
	}
	if !m.walletSidebar.Focused {
		t.Error("sidebar should be focused after returning")
	}
}

func TestWalletContentPaneBackspaceToSidebar(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletPaneFocused = true
	m.walletSidebar.Blur()

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.walletPaneFocused {
		t.Error("backspace should return focus to sidebar")
	}
	if !m.walletSidebar.Focused {
		t.Error("sidebar should be focused after backspace")
	}
}

func TestWalletTransactionsPaneNavigation(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletPaneFocused = true
	m.walletSidebar.ActiveIndex = walletSectionTransactions
	m.payHistoryCursor = 0

	// Down with no history should be safe
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = newM.(Model)
	if m.payHistoryCursor != 0 {
		t.Errorf("down with no history: got %d, want 0", m.payHistoryCursor)
	}
}

// ── Send/Receive Return to Sidebar ───────────────────────

func TestSendEscReturnsToSidebar(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.subview = svSend
	m.sendInput = newSendPayReqInput()

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("esc from send: got subview %d, want %d", m.subview, svNone)
	}
	if m.walletPaneFocused {
		t.Error("should return to sidebar, not content pane")
	}
	if !m.walletSidebar.Focused {
		t.Error("sidebar should be focused after esc from send")
	}
}

func TestReceiveEscReturnsToSidebar(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.subview = svReceive
	m.recvAmountInput = newRecvAmountInput()
	m.recvMemoInput = newRecvMemoInput()

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("esc from receive: got subview %d, want %d", m.subview, svNone)
	}
	if m.walletPaneFocused {
		t.Error("should return to sidebar, not content pane")
	}
	if !m.walletSidebar.Focused {
		t.Error("sidebar should be focused after esc from receive")
	}
}

func TestSendResultEnterReturnsToSidebar(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.subview = svSendResult
	m.sendInput = newSendPayReqInput()

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("enter from send result: got %d, want %d", m.subview, svNone)
	}
	if !m.walletSidebar.Focused {
		t.Error("sidebar should be focused after returning from send result")
	}
}

func TestReceivePaidEnterReturnsToSidebar(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.subview = svReceivePaid
	m.recvAmountInput = newRecvAmountInput()
	m.recvMemoInput = newRecvMemoInput()

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("enter from receive paid: got %d, want %d", m.subview, svNone)
	}
	if !m.walletSidebar.Focused {
		t.Error("sidebar should be focused after returning from receive paid")
	}
}

func TestReceiveExpiredReturnsToSidebar(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.subview = svReceiveExpired
	m.recvAmountInput = newRecvAmountInput()
	m.recvMemoInput = newRecvMemoInput()

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("enter from receive expired: got %d, want %d", m.subview, svNone)
	}
	if !m.walletSidebar.Focused {
		t.Error("sidebar should be focused after returning from receive expired")
	}
}

func TestPaymentDetailBackReturnsToTransactionsPane(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.subview = svPaymentDetail
	m.walletSidebar.ActiveIndex = walletSectionTransactions

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("backspace from detail: got %d, want %d", m.subview, svNone)
	}
	if !m.walletPaneFocused {
		t.Error("should return to transactions pane, not sidebar")
	}
}

// ── Wallet Tab View Rendering ────────────────────────────

func TestWalletTabShowsSidebar(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletSidebar.Focus()

	view := m.View()
	if !strings.Contains(view.Content, "Transactions") {
		t.Error("wallet tab should show Transactions button")
	}
	if !strings.Contains(view.Content, "Send") {
		t.Error("wallet tab should show Send button")
	}
	if !strings.Contains(view.Content, "Receive") {
		t.Error("wallet tab should show Receive button")
	}
	if !strings.Contains(view.Content, "On-Chain") {
		t.Error("wallet tab should show On-Chain button")
	}
}

func TestWalletTabNoLND(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabWallet

	view := m.View()
	if !strings.Contains(view.Content, "Install LND") {
		t.Error("should show install message")
	}
}

func TestWalletTabNoWallet(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabWallet

	view := m.View()
	if !strings.Contains(view.Content, "create wallet") {
		t.Error("should show create wallet message")
	}
}

func TestWalletInfoBackspace(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.subview = svWalletInfo

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("backspace from wallet info: got %d, want %d",
			m.subview, svNone)
	}
}

// ── Subview Navigation ───────────────────────────────────

func TestFullURLBackspaceReturnsToOrigin(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.subview = svFullURL
	m.urlReturnTo = svSyncthingDetail

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svSyncthingDetail {
		t.Errorf("backspace from full URL: got %d, want %d",
			m.subview, svSyncthingDetail)
	}
}

func TestFullURLBackspaceNoReturnTo(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.subview = svFullURL
	m.urlReturnTo = svNone

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("backspace from full URL no return: got %d, want %d",
			m.subview, svNone)
	}
}

func TestQRBackspaceGoesToZeus(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.subview = svQR
	m.qrLabel = ""

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svZeus {
		t.Errorf("backspace from QR: got %d, want %d (zeus)",
			m.subview, svZeus)
	}
}

func TestQRBackspaceGoesToLndHubNewAccount(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.subview = svQR
	m.qrLabel = "Alice — Tor"

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svLndHubCreateAccount {
		t.Errorf("backspace from LndHub QR: got %d, want %d",
			m.subview, svLndHubCreateAccount)
	}
}

// ── Tab Switching Resets State ───────────────────────────

func TestTabSwitchResetsCardActive(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabSystem
	m.cardActive = true
	m.svcConfirm = "restart"

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = newM.(Model)
	if m.cardActive {
		t.Error("tab switch should deactivate card")
	}
	if m.svcConfirm != "" {
		t.Error("tab switch should clear svcConfirm")
	}
}

func TestTabSwitchResetsWalletPaneFocused(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet
	m.walletPaneFocused = true

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = newM.(Model)
	if m.walletPaneFocused {
		t.Error("tab switch should reset walletPaneFocused")
	}
}

// ── Service Count ────────────────────────────────────────

func TestServiceCountBase(t *testing.T) {
	m := testModel()
	if m.svcCount() != 2 {
		t.Errorf("base: got %d, want 2", m.svcCount())
	}
}

func TestServiceCountWithLND(t *testing.T) {
	m := testModelWithLND()
	if m.svcCount() != 3 {
		t.Errorf("with LND: got %d, want 3", m.svcCount())
	}
}

func TestServiceCountFullStack(t *testing.T) {
	m := testModelFullStack()
	if m.svcCount() != 5 {
		t.Errorf("full stack: got %d, want 5", m.svcCount())
	}
}

func TestServiceCountFullStackHybrid(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.SyncthingInstalled = true
	cfg.LndHubInstalled = true
	cfg.P2PMode = "hybrid"
	m := NewModel(cfg, "0.0.0-test")
	if m.svcCount() != 6 {
		t.Errorf("full stack hybrid: got %d, want 6", m.svcCount())
	}
}

// ── Service Names ────────────────────────────────────────

func TestServiceNames(t *testing.T) {
	m := testModelFullStack()
	expected := []string{"tor", "bitcoind", "lnd", "syncthing", "lndhub"}
	for i, want := range expected {
		got := m.svcName(i)
		if got != want {
			t.Errorf("svcName(%d): got %q, want %q", i, got, want)
		}
	}
}

func TestServiceNameOutOfBounds(t *testing.T) {
	m := testModel()
	got := m.svcName(99)
	if got != "" {
		t.Errorf("svcName(99): got %q, want empty", got)
	}
}

// ── Addons Navigation ────────────────────────────────────

func TestAddonsSyncthingRequiresLND(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabAddons
	m.addonFocus = 0

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.shellAction == svSyncthingInstall {
		t.Error("syncthing install should not trigger without LND")
	}
}

// ── LndHub ───────────────────────────────────────────────

func TestAddonsLndHubRequiresLND(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabAddons
	m.addonFocus = 2

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.shellAction == svLndHubInstall {
		t.Error("LndHub should not trigger without LND")
	}
}

func TestAddonsLndHubInstallWithLND(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabAddons
	m.addonFocus = 1

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.shellAction != svNone {
		t.Errorf("LND not running: got %d, want %d (blocked)",
			m.shellAction, svNone)
	}
}

func TestAddonsLndHubManageWhenInstalled(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	cfg.LndHubInstalled = true
	cfg.LndHubAdminToken = "test-token"
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabAddons
	m.addonFocus = 1

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svLndHubManage {
		t.Errorf("enter on installed LndHub: got %d, want %d",
			m.subview, svLndHubManage)
	}
}

func TestAddonNavTwoCards(t *testing.T) {
	m := testModelFullStack()
	m.width = 80
	m.height = 24
	m.activeTab = tabAddons
	m.addonFocus = 0

	newM, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = newM.(Model)
	if m.addonFocus != 1 {
		t.Errorf("right from 0: got %d, want 1", m.addonFocus)
	}

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = newM.(Model)
	if m.addonFocus != 1 {
		t.Errorf("right from 1: got %d, want 1 (clamped)",
			m.addonFocus)
	}

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = newM.(Model)
	if m.addonFocus != 0 {
		t.Errorf("left from 1: got %d, want 0", m.addonFocus)
	}
}

func TestLndHubManageBackspace(t *testing.T) {
	m := testModelFullStack()
	m.width = 80
	m.height = 24
	m.subview = svLndHubManage

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("backspace: got %d, want %d", m.subview, svNone)
	}
}

func TestLndHubCreateNameBackspaceEmpty(t *testing.T) {
	m := testModelFullStack()
	m.width = 80
	m.height = 24
	m.subview = svLndHubCreateName
	m.hubNameInput = newHubNameInput()

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = newM.(Model)
	if m.subview != svLndHubManage {
		t.Errorf("escape from create name: got %d, want %d",
			m.subview, svLndHubManage)
	}
}

func TestLndHubCreateNameBackspaceWithText(t *testing.T) {
	m := testModelFullStack()
	m.width = 80
	m.height = 24
	m.subview = svLndHubCreateName
	m.hubNameInput = newHubNameInput()
	m.hubNameInput.SetValue("Ali")

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svLndHubCreateName {
		t.Error("backspace with text should stay")
	}
	if m.hubNameInput.Value() != "Al" {
		t.Errorf("got %q, want Al", m.hubNameInput.Value())
	}
}

func TestLndHubAccountCreatedMsg(t *testing.T) {
	cfg := config.Default()
	cfg.LndHubInstalled = true
	cfg.LndHubAdminToken = "test"
	m := testModelWithStore(t, cfg)
	m.width = 80
	m.height = 24
	m.subview = svLndHubCreateName
	m.hubNameInput = newHubNameInput()
	m.hubNameInput.SetValue("Alice")

	account := &installer.LndHubAccount{
		Login: "abc123", Password: "def456",
	}
	newM, _ := m.Update(lndhubAccountCreatedMsg{account: account})
	m = newM.(Model)

	if m.subview != svLndHubCreateAccount {
		t.Errorf("after created: got %d, want %d",
			m.subview, svLndHubCreateAccount)
	}
	if m.lastAccount == nil {
		t.Error("lastAccount should be set")
	}
	if len(m.cfg.LndHubAccounts) != 1 {
		t.Errorf("accounts: got %d, want 1",
			len(m.cfg.LndHubAccounts))
	}
	if m.cfg.LndHubAccounts[0].Label != "Alice" {
		t.Errorf("label: got %q, want Alice",
			m.cfg.LndHubAccounts[0].Label)
	}
}

func TestLndHubDeactivatedMsg(t *testing.T) {
	cfg := config.Default()
	cfg.LndHubInstalled = true
	cfg.LndHubAccounts = []config.LndHubAccount{
		{Label: "Alice", Login: "abc",
			CreatedAt: "2026-02-23", Active: true},
	}
	m := testModelWithStore(t, cfg)
	m.width = 80
	m.height = 24
	m.subview = svLndHubDeactivateConfirm
	m.hubCursor = 0

	newM, _ := m.Update(lndhubDeactivatedMsg{
		balance: "5000", err: nil})
	m = newM.(Model)

	if m.subview != svLndHubManage {
		t.Errorf("after deactivate: got %d, want %d",
			m.subview, svLndHubManage)
	}
	if m.cfg.LndHubAccounts[0].Active {
		t.Error("should be deactivated")
	}
	if m.cfg.LndHubAccounts[0].BalanceOnDeactivate != "5000" {
		t.Errorf("balance: got %q, want 5000",
			m.cfg.LndHubAccounts[0].BalanceOnDeactivate)
	}
}

func TestLndHubAccountCreatedMsgError(t *testing.T) {
	cfg := config.Default()
	cfg.LndHubInstalled = true
	m := testModelWithStore(t, cfg)
	m.width = 80
	m.height = 24
	m.subview = svLndHubCreateName
	m.hubNameInput = newHubNameInput()
	m.hubNameInput.SetValue("Bob")

	newM, _ := m.Update(lndhubAccountCreatedMsg{
		account: nil, err: fmt.Errorf("refused")})
	m = newM.(Model)

	if m.subview != svLndHubManage {
		t.Errorf("after error: got %d, want %d",
			m.subview, svLndHubManage)
	}
	if len(m.cfg.LndHubAccounts) != 0 {
		t.Error("should not add on error")
	}
}

// ── Hub Name Input Validation ────────────────────────────

func TestHubNameAllowedChars(t *testing.T) {
	allowed := []string{"a", "Z", "0", "9", " ", "-"}
	for _, key := range allowed {
		if !isAllowedHubNameChar(key) {
			t.Errorf("should allow %q", key)
		}
	}
}

func TestHubNameRejectedChars(t *testing.T) {
	rejected := []string{";", "'", "\"", "/", "\\", "|",
		"&", "$", "`", "\n", "\t", ".", ",", "!", "@", "#"}
	for _, key := range rejected {
		if isAllowedHubNameChar(key) {
			t.Errorf("should reject %q", key)
		}
	}
}

func TestHubNameMultiByteRejected(t *testing.T) {
	if isAllowedHubNameChar("ab") {
		t.Error("multi-byte should be rejected")
	}
	if isAllowedHubNameChar("") {
		t.Error("empty should be rejected")
	}
}

func TestHubNameMaxLength(t *testing.T) {
	m := testModelFullStack()
	m.width = 80
	m.height = 24
	m.subview = svLndHubCreateName
	m.hubNameInput = newHubNameInput()
	m.hubNameInput.SetValue("abcdefghijklmnopqrstuvwxyz1234")

	newM, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = newM.(Model)
	if len(m.hubNameInput.Value()) != 30 {
		t.Errorf("length: got %d, want 30", len(m.hubNameInput.Value()))
	}
}

// ── Service Names Helper ─────────────────────────────────

func TestServiceNamesDefault(t *testing.T) {
	cfg := config.Default()
	names := serviceNames(cfg)
	if len(names) != 2 {
		t.Errorf("default: got %d, want 2", len(names))
	}
}

func TestServiceNamesWithLND(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	names := serviceNames(cfg)
	if len(names) != 3 {
		t.Errorf("with LND: got %d, want 3", len(names))
	}
}

func TestServiceNamesFullStack(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.SyncthingInstalled = true
	cfg.LndHubInstalled = true
	names := serviceNames(cfg)
	expected := []string{"tor", "bitcoind", "lnd", "syncthing", "lndhub"}
	if len(names) != len(expected) {
		t.Fatalf("got %d, want %d", len(names), len(expected))
	}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("[%d]: got %q, want %q", i, names[i], want)
		}
	}
}

func TestServiceNamesHybridProxy(t *testing.T) {
	cfg := config.Default()
	cfg.LndHubInstalled = true
	cfg.P2PMode = "hybrid"
	names := serviceNames(cfg)
	last := names[len(names)-1]
	if last != "lndhub-proxy" {
		t.Errorf("last: got %q, want lndhub-proxy", last)
	}
}

func TestServiceNamesNoProxyTorMode(t *testing.T) {
	cfg := config.Default()
	cfg.LndHubInstalled = true
	cfg.P2PMode = "tor"
	names := serviceNames(cfg)
	for _, n := range names {
		if n == "lndhub-proxy" {
			t.Error("tor mode should not include proxy")
		}
	}
}

// ── Channel Detail from Dashboard ────────────────────────

func TestChannelDetailFromDashboard(t *testing.T) {
	m := testModelWalletWithStatus()
	m.activeTab = tabDashboard
	m.chanCursor = 0

	// Enter opens detail
	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svChannelDetail {
		t.Errorf("enter should open detail, got %d", m.subview)
	}

	// View should show channel info
	view := m.View()
	if !strings.Contains(view.Content, "ACINQ") {
		t.Error("channel detail should show peer alias")
	}

	// Backspace returns
	newM, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("backspace should return, got %d", m.subview)
	}
}

func TestChannelDetailSecondChannel(t *testing.T) {
	m := testModelWalletWithStatus()
	m.activeTab = tabDashboard
	m.chanCursor = 1

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svChannelDetail {
		t.Errorf("enter should open detail, got %d", m.subview)
	}

	view := m.View()
	if !strings.Contains(view.Content, "Zeus") {
		t.Error("channel detail should show Zeus peer alias")
	}
}

// ── Formatting ───────────────────────────────────────────

func TestFormatSats(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{100, "100"},
		{1000, "1,000"},
		{1000000, "1,000,000"},
		{250000, "250,000"},
		{21000000000000, "21,000,000,000,000"},
	}
	for _, tt := range tests {
		got := formatSats(tt.input)
		if got != tt.want {
			t.Errorf("formatSats(%d): got %q, want %q",
				tt.input, got, tt.want)
		}
	}
}

func TestRenderBalanceBar(t *testing.T) {
	bar := renderBalanceBar(500000, 500000, 1000000, 20)
	if len(bar) == 0 {
		t.Error("should not be empty")
	}
	bar = renderBalanceBar(0, 0, 0, 20)
	if len(bar) == 0 {
		t.Error("zero capacity should not be empty")
	}
}

func TestParseBalance(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"0", 0},
		{"1000000", 1000000},
		{"", 0},
		{"abc", 0},
		{"12abc34", 1234},
	}
	for _, tt := range tests {
		got := parseBalance(tt.input)
		if got != tt.want {
			t.Errorf("parseBalance(%q): got %d, want %d",
				tt.input, got, tt.want)
		}
	}
}

// ── Poll Interval ────────────────────────────────────────

func TestPollIntervalNoStatus(t *testing.T) {
	m := testModel()
	m.status = nil
	if m.pollInterval().Seconds() != 3 {
		t.Errorf("no status: got %v, want 3s", m.pollInterval())
	}
}

func TestPollIntervalLNDNotResponding(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.status = &statusMsg{
		lndResponding: false,
		services:      map[string]bool{},
	}
	if m.pollInterval().Seconds() != 5 {
		t.Errorf("LND not responding: got %v, want 5s",
			m.pollInterval())
	}
}

func TestPollIntervalBitcoinSyncing(t *testing.T) {
	m := testModel()
	m.status = &statusMsg{
		btcSynced: false,
		services:  map[string]bool{},
	}
	if m.pollInterval().Seconds() != 15 {
		t.Errorf("syncing: got %v, want 15s", m.pollInterval())
	}
}

func TestPollIntervalStable(t *testing.T) {
	m := testModel()
	m.status = &statusMsg{
		btcSynced:     true,
		lndResponding: true,
		services:      map[string]bool{},
	}
	if m.pollInterval().Seconds() != 60 {
		t.Errorf("stable: got %v, want 60s", m.pollInterval())
	}
}

// ── Subview Classification ───────────────────────────────

func TestIsChannelSubview(t *testing.T) {
	channelViews := []wSubview{
		svChannelOpen, svChannelCustomPeer, svChannelAmountSelect,
		svChannelOpenConfirm, svChannelOpening, svChannelOpenResult,
		svChannelFundWallet,
	}
	for _, sv := range channelViews {
		if !isChannelSubview(sv) {
			t.Errorf("isChannelSubview(%d) should be true", sv)
		}
	}
	if isChannelSubview(svNone) {
		t.Error("svNone should not be channel subview")
	}
	if isChannelSubview(svZeus) {
		t.Error("svZeus should not be channel subview")
	}
}

func TestIsPairingSubview(t *testing.T) {
	if !isPairingSubview(svZeus) {
		t.Error("svZeus should be pairing")
	}
	if !isPairingSubview(svQR) {
		t.Error("svQR should be pairing")
	}
	if isPairingSubview(svNone) {
		t.Error("svNone should not be pairing")
	}
}

func TestIsAddonSubview(t *testing.T) {
	if !isAddonSubview(svSyncthingDetail) {
		t.Error("svSyncthingDetail should be addon")
	}
	if !isAddonSubview(svLndHubManage) {
		t.Error("svLndHubManage should be addon")
	}
	if isAddonSubview(svNone) {
		t.Error("svNone should not be addon")
	}
}

// ── Wallet Section Constants ─────────────────────────────

func TestWalletSectionConstants(t *testing.T) {
	if walletSectionTransactions != 0 {
		t.Errorf("walletSectionTransactions: got %d, want 0", walletSectionTransactions)
	}
	if walletSectionSend != 1 {
		t.Errorf("walletSectionSend: got %d, want 1", walletSectionSend)
	}
	if walletSectionReceive != 2 {
		t.Errorf("walletSectionReceive: got %d, want 2", walletSectionReceive)
	}
	if walletSectionOnChain != 3 {
		t.Errorf("walletSectionOnChain: got %d, want 3", walletSectionOnChain)
	}
}

// ── On-Chain Stub ────────────────────────────────────────

func TestOnChainPaneShowsComingSoon(t *testing.T) {
	m := testModelWalletReady()
	m.activeTab = tabWallet

	// Force On-Chain pane to render (even though button is disabled,
	// test the rendering function directly)
	pane := m.walletOnChainPane(50)
	if !strings.Contains(pane, "coming soon") {
		t.Error("On-Chain pane should show 'coming soon' message")
	}
}
