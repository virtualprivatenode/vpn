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
	if m.walletFocus != 0 {
		t.Errorf("initial walletFocus: got %d, want 0",
			m.walletFocus)
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

// ── Dashboard is Read-Only ───────────────────────────────

func TestDashboardNoCardNavigation(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabDashboard

	// Arrow keys should not change any card state on Dashboard
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = newM.(Model)
	if m.cardActive {
		t.Error("dashboard should not activate cards")
	}

	// Enter should not activate anything on Dashboard
	newM, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.cardActive {
		t.Error("enter on dashboard should not activate cards")
	}
	if m.subview != svNone {
		t.Errorf("enter on dashboard: got subview %d, want %d",
			m.subview, svNone)
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

// ── Wallet Tab Navigation ────────────────────────────────

func TestWalletTabLeftRight(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabWallet
	m.walletFocus = 0

	newM, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = newM.(Model)
	if m.walletFocus != 1 {
		t.Errorf("right from 0: got %d, want 1",
			m.walletFocus)
	}

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = newM.(Model)
	if m.walletFocus != 1 {
		t.Errorf("right from 1: got %d, want 1 (clamped)",
			m.walletFocus)
	}

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = newM.(Model)
	if m.walletFocus != 0 {
		t.Errorf("left from 1: got %d, want 0",
			m.walletFocus)
	}
}

func TestWalletCardEnter(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabWallet
	m.walletFocus = 1

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svWalletInfo {
		t.Errorf("enter on wallet card: got %d, want %d",
			m.subview, svWalletInfo)
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
	// Correct method to retrieve the value from textinput.Model
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

// ── Wallet Tab ───────────────────────────────────────────

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
	if !strings.Contains(view.Content, "Create") {
		t.Error("should show create wallet message")
	}
}

func TestWalletTabWithChannels(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabWallet
	m.status = &statusMsg{
		lndResponding: true,
		lndBalance:    "1000000",
		channels: []channelInfo{
			{
				ChanID: 123, PeerAlias: "ACINQ",
				RemotePubkey: "03abc123",
				Capacity:     250000, LocalBalance: 150000,
				RemoteBalance: 100000, Active: true,
			},
		},
		services: map[string]bool{},
	}

	view := m.View()
	if !strings.Contains(view.Content, "ACINQ") {
		t.Error("should show peer alias")
	}
	if !strings.Contains(view.Content, "Wallet") {
		t.Error("should show wallet card")
	}
}

func TestChannelDetailSubview(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabWallet
	m.walletFocus = 0
	m.chanCursor = 0
	m.status = &statusMsg{
		lndResponding: true,
		channels: []channelInfo{
			{
				ChanID: 123, PeerAlias: "ACINQ",
				RemotePubkey: "03abc123def456",
				Capacity:     250000, LocalBalance: 150000,
				RemoteBalance: 100000, Active: true,
				Initiator: true,
			},
		},
		services: map[string]bool{},
	}

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svChannelDetail {
		t.Errorf("enter should open detail, got %d", m.subview)
	}

	newM, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("backspace should return, got %d", m.subview)
	}
}

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
