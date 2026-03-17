// internal/welcome/welcome_test.go

package welcome

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

// testStore creates an isolated config store in a temp directory.
func testStore(t *testing.T) *config.Store {
	t.Helper()
	dir := t.TempDir()
	return &config.Store{
		Dir:  dir,
		Path: filepath.Join(dir, "config.json"),
	}
}

// testModelWithStore creates a model with an isolated config store.
func testModelWithStore(t *testing.T, cfg *config.AppConfig) Model {
	t.Helper()
	store := testStore(t)
	return NewTestModel(cfg, "0.0.0-test", store)
}

// ── Tab Navigation ───────────────────────────────────────

func TestInitialState(t *testing.T) {
	m := testModel()

	if m.activeTab != tabDashboard {
		t.Errorf("initial tab: got %d, want %d (dashboard)", m.activeTab, tabDashboard)
	}
	if m.subview != svNone {
		t.Errorf("initial subview: got %d, want %d (none)", m.subview, svNone)
	}
	if m.dashCard != cardServices {
		t.Errorf("initial card: got %d, want %d (services)", m.dashCard, cardServices)
	}
	if m.cardActive {
		t.Error("card should not be active initially")
	}
}

func TestTabForward(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24

	expected := []wTab{tabChannels, tabPairing, tabAddons, tabSettings, tabDashboard}
	for _, want := range expected {
		newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = newM.(Model)
		if m.activeTab != want {
			t.Errorf("after tab: got %d, want %d", m.activeTab, want)
		}
	}
}

func TestTabBackward(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = newM.(Model)
	if m.activeTab != tabSettings {
		t.Errorf("after shift+tab: got %d, want %d (settings)", m.activeTab, tabSettings)
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
		{"2", tabChannels},
		{"3", tabPairing},
		{"4", tabAddons},
		{"5", tabSettings},
	}
	for _, tt := range tests {
		newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
		result := newM.(Model)
		if result.activeTab != tt.want {
			t.Errorf("key %s: got tab %d, want %d", tt.key, result.activeTab, tt.want)
		}
	}
}

// ── Dashboard Navigation ─────────────────────────────────

func TestDashboardCardNavigation(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabDashboard
	m.dashCard = cardServices

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = newM.(Model)
	if m.dashCard != cardSystem {
		t.Errorf("right from services: got %d, want %d (system)", m.dashCard, cardSystem)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = newM.(Model)
	if m.dashCard != cardLightning {
		t.Errorf("down from system: got %d, want %d (lightning)", m.dashCard, cardLightning)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = newM.(Model)
	if m.dashCard != cardBitcoin {
		t.Errorf("left from lightning: got %d, want %d (bitcoin)", m.dashCard, cardBitcoin)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = newM.(Model)
	if m.dashCard != cardServices {
		t.Errorf("up from bitcoin: got %d, want %d (services)", m.dashCard, cardServices)
	}
}

// ── Card Activation ──────────────────────────────────────

func TestEnterActivatesServicesCard(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabDashboard
	m.dashCard = cardServices

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if !m.cardActive {
		t.Error("enter on services card should activate it")
	}
	if m.svcCursor != 0 {
		t.Errorf("service cursor should start at 0, got %d", m.svcCursor)
	}
}

func TestEnterActivatesSystemCard(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabDashboard
	m.dashCard = cardSystem

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if !m.cardActive {
		t.Error("enter on system card should activate it")
	}
}

func TestBackspaceDeactivatesCard(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabDashboard
	m.dashCard = cardServices
	m.cardActive = true

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.cardActive {
		t.Error("backspace should deactivate card")
	}
}

// ── Lightning Card Actions ───────────────────────────────

func TestLightningCardInstallLND(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabDashboard
	m.dashCard = cardLightning

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.shellAction != svLNDInstall {
		t.Errorf("enter on lightning without LND: got shellAction %d, want %d (svLNDInstall)",
			m.shellAction, svLNDInstall)
	}
}

func TestLightningCardWithLNDShowsDetail(t *testing.T) {
	m := testModelWithLND()
	m.width = 80
	m.height = 24
	m.activeTab = tabDashboard
	m.dashCard = cardLightning

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.shellAction != svWalletCreate {
		t.Errorf("enter on lightning with LND no wallet: got shellAction %d, want %d (svWalletCreate)",
			m.shellAction, svWalletCreate)
	}
}

// ── Subview Navigation ───────────────────────────────────

func TestSubviewBackspace(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.subview = svLightning

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("backspace from subview: got %d, want %d (none)", m.subview, svNone)
	}
}

func TestFullURLBackspaceReturnsToOrigin(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.subview = svFullURL
	m.urlReturnTo = svSyncthingDetail

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svSyncthingDetail {
		t.Errorf("backspace from full URL: got %d, want %d (syncthing detail)",
			m.subview, svSyncthingDetail)
	}
}

func TestFullURLBackspaceNoReturnTo(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.subview = svFullURL
	m.urlReturnTo = svNone

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("backspace from full URL no return: got %d, want %d (none)",
			m.subview, svNone)
	}
}

func TestQRBackspaceGoesToZeus(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.subview = svQR
	m.qrLabel = ""

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svZeus {
		t.Errorf("backspace from QR: got %d, want %d (zeus)", m.subview, svZeus)
	}
}

func TestQRBackspaceGoesToLndHubNewAccount(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.subview = svQR
	m.qrLabel = "Alice — Tor"

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svLndHubCreateAccount {
		t.Errorf("backspace from LndHub QR: got %d, want %d (create account)",
			m.subview, svLndHubCreateAccount)
	}
}

// ── Tab Switching Resets State ───────────────────────────

func TestTabSwitchResetsCardActive(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabDashboard
	m.cardActive = true
	m.svcConfirm = "restart"

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
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
		t.Errorf("base service count: got %d, want 2 (tor, bitcoind)", m.svcCount())
	}
}

func TestServiceCountWithLND(t *testing.T) {
	m := testModelWithLND()
	if m.svcCount() != 3 {
		t.Errorf("LND service count: got %d, want 3", m.svcCount())
	}
}

func TestServiceCountFullStack(t *testing.T) {
	m := testModelFullStack()
	if m.svcCount() != 5 {
		t.Errorf("full stack service count: got %d, want 5", m.svcCount())
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
		t.Errorf("full stack hybrid service count: got %d, want 6", m.svcCount())
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

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.shellAction == svSyncthingInstall {
		t.Error("syncthing install should not trigger without LND")
	}
}

// ── Settings ─────────────────────────────────────────────

func TestSettingsUpdateConfirm(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabSettings
	m.latestVersion = "9.9.9"

	m = handleSettingsKey(m, "enter")
	if !m.updateConfirm {
		t.Error("enter with new version available should set updateConfirm")
	}

	m = handleSettingsKey(m, "n")
	if m.updateConfirm {
		t.Error("pressing n should cancel updateConfirm")
	}
}

func TestSettingsNoUpdateWhenCurrent(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabSettings
	m.latestVersion = installer.GetVersion()

	m = handleSettingsKey(m, "enter")
	if m.updateConfirm {
		t.Error("should not confirm update when already on latest")
	}
}

// ── LndHub ───────────────────────────────────────────────

func TestAddonsLndHubRequiresLND(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabAddons
	m.addonFocus = 2

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.shellAction == svLndHubInstall {
		t.Error("LndHub install should not trigger without LND")
	}
}

func TestAddonsLndHubInstallWithLND(t *testing.T) {
	// LND service is not running in test environment,
	// so install should be blocked (shellAction stays svNone).
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabAddons
	m.addonFocus = 1

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.shellAction != svNone {
		t.Errorf("enter on LndHub with LND not running: got shellAction %d, want %d (svNone — blocked)",
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

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svLndHubManage {
		t.Errorf("enter on installed LndHub: got subview %d, want %d",
			m.subview, svLndHubManage)
	}
}

func TestAddonNavTwoCards(t *testing.T) {
	m := testModelFullStack()
	m.width = 80
	m.height = 24
	m.activeTab = tabAddons
	m.addonFocus = 0

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = newM.(Model)
	if m.addonFocus != 1 {
		t.Errorf("right from 0: got %d, want 1", m.addonFocus)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = newM.(Model)
	if m.addonFocus != 1 {
		t.Errorf("right from 1: got %d, want 1 (clamped)", m.addonFocus)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
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

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("backspace from lndhub manage: got %d, want %d", m.subview, svNone)
	}
}

func TestLndHubCreateNameBackspaceEmpty(t *testing.T) {
	m := testModelFullStack()
	m.width = 80
	m.height = 24
	m.subview = svLndHubCreateName
	m.hubNameInput = ""

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svLndHubManage {
		t.Errorf("backspace from empty name: got %d, want %d",
			m.subview, svLndHubManage)
	}
}

func TestLndHubCreateNameBackspaceWithText(t *testing.T) {
	m := testModelFullStack()
	m.width = 80
	m.height = 24
	m.subview = svLndHubCreateName
	m.hubNameInput = "Ali"

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svLndHubCreateName {
		t.Error("backspace with text should stay on name screen")
	}
	if m.hubNameInput != "Al" {
		t.Errorf("hubNameInput: got %q, want Al", m.hubNameInput)
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
	m.hubNameInput = "Alice"

	account := &installer.LndHubAccount{
		Login:    "abc123",
		Password: "def456",
	}
	newM, _ := m.Update(lndhubAccountCreatedMsg{account: account})
	m = newM.(Model)

	if m.subview != svLndHubCreateAccount {
		t.Errorf("after account created: got subview %d, want %d",
			m.subview, svLndHubCreateAccount)
	}
	if m.lastAccount == nil {
		t.Error("lastAccount should be set")
	}
	if len(m.cfg.LndHubAccounts) != 1 {
		t.Errorf("accounts: got %d, want 1", len(m.cfg.LndHubAccounts))
	}
	if m.cfg.LndHubAccounts[0].Label != "Alice" {
		t.Errorf("label: got %q, want Alice", m.cfg.LndHubAccounts[0].Label)
	}
}

func TestLndHubDeactivatedMsg(t *testing.T) {
	cfg := config.Default()
	cfg.LndHubInstalled = true
	cfg.LndHubAccounts = []config.LndHubAccount{
		{Label: "Alice", Login: "abc", CreatedAt: "2026-02-23", Active: true},
	}
	m := testModelWithStore(t, cfg)
	m.width = 80
	m.height = 24
	m.subview = svLndHubDeactivateConfirm
	m.hubCursor = 0

	newM, _ := m.Update(lndhubDeactivatedMsg{balance: "5000", err: nil})
	m = newM.(Model)

	if m.subview != svLndHubManage {
		t.Errorf("after deactivate: got subview %d, want %d",
			m.subview, svLndHubManage)
	}
	if m.cfg.LndHubAccounts[0].Active {
		t.Error("account should be deactivated")
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
	m.hubNameInput = "Bob"

	newM, _ := m.Update(lndhubAccountCreatedMsg{account: nil, err: fmt.Errorf("connection refused")})
	m = newM.(Model)

	if m.subview != svLndHubManage {
		t.Errorf("after error: got subview %d, want %d (manage)",
			m.subview, svLndHubManage)
	}
	if len(m.cfg.LndHubAccounts) != 0 {
		t.Error("should not have added account on error")
	}
}

// ── Hub Name Input Validation ────────────────────────────

func TestHubNameAllowedChars(t *testing.T) {
	allowed := []string{"a", "Z", "0", "9", " ", "-"}
	for _, key := range allowed {
		if !isAllowedHubNameChar(key) {
			t.Errorf("isAllowedHubNameChar(%q) should be true", key)
		}
	}
}

func TestHubNameRejectedChars(t *testing.T) {
	rejected := []string{";", "'", "\"", "/", "\\", "|", "&", "$", "`", "\n", "\t", ".", ",", "!", "@", "#"}
	for _, key := range rejected {
		if isAllowedHubNameChar(key) {
			t.Errorf("isAllowedHubNameChar(%q) should be false", key)
		}
	}
}

func TestHubNameMultiByteRejected(t *testing.T) {
	if isAllowedHubNameChar("ab") {
		t.Error("multi-byte input should be rejected")
	}
	if isAllowedHubNameChar("") {
		t.Error("empty input should be rejected")
	}
}

func TestHubNameMaxLength(t *testing.T) {
	m := testModelFullStack()
	m.width = 80
	m.height = 24
	m.subview = svLndHubCreateName
	m.hubNameInput = "abcdefghijklmnopqrstuvwxyz1234" // 30 chars

	// Try to add one more
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = newM.(Model)
	if len(m.hubNameInput) != 30 {
		t.Errorf("name length: got %d, want 30 (max)", len(m.hubNameInput))
	}
}

// ── Service Names Helper ─────────────────────────────────

func TestServiceNamesDefault(t *testing.T) {
	cfg := config.Default()
	names := serviceNames(cfg)
	if len(names) != 2 {
		t.Errorf("default: got %d services, want 2", len(names))
	}
	if names[0] != "tor" || names[1] != "bitcoind" {
		t.Errorf("default: got %v, want [tor bitcoind]", names)
	}
}

func TestServiceNamesWithLND(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	names := serviceNames(cfg)
	if len(names) != 3 {
		t.Errorf("with LND: got %d, want 3", len(names))
	}
	if names[2] != "lnd" {
		t.Errorf("third service: got %q, want lnd", names[2])
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
		t.Fatalf("full stack: got %d, want %d", len(names), len(expected))
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
			t.Error("tor mode should not include lndhub-proxy")
		}
	}
}

// ── Channels Tab ─────────────────────────────────────────

func TestChannelsTabRequiresLND(t *testing.T) {
	m := testModel()
	m.width = 80
	m.height = 24
	m.activeTab = tabChannels

	view := m.View()
	if !strings.Contains(view, "Install LND") {
		t.Error("channels tab without LND should show install message")
	}
}

func TestChannelsTabWithLNDNoWallet(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabChannels

	view := m.View()
	if !strings.Contains(view, "Create LND wallet") {
		t.Error("channels tab without wallet should show create message")
	}
}

func TestChannelsTabNoChannels(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabChannels
	m.status = &statusMsg{
		lndResponding: true,
		channels:      []channelInfo{},
		services:      map[string]bool{},
	}

	view := m.View()
	if !strings.Contains(view, "No channels yet") {
		t.Error("channels tab with no channels should show empty message")
	}
}

func TestChannelsTabWithChannels(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabChannels
	m.status = &statusMsg{
		lndResponding: true,
		channels: []channelInfo{
			{
				ChanID:        123,
				PeerAlias:     "ACINQ",
				RemotePubkey:  "03abc123",
				Capacity:      250000,
				LocalBalance:  150000,
				RemoteBalance: 100000,
				Active:        true,
			},
		},
		services: map[string]bool{},
	}

	view := m.View()
	if !strings.Contains(view, "ACINQ") {
		t.Error("channels tab should show peer alias")
	}
	if !strings.Contains(view, "1 active") {
		t.Error("channels tab should show active count")
	}
}

func TestChannelDetailSubview(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.WalletCreated = true
	m := NewModel(cfg, "0.0.0-test")
	m.width = 80
	m.height = 24
	m.activeTab = tabChannels
	m.chanCursor = 0
	m.status = &statusMsg{
		lndResponding: true,
		channels: []channelInfo{
			{
				ChanID:        123,
				PeerAlias:     "ACINQ",
				RemotePubkey:  "03abc123def456",
				Capacity:      250000,
				LocalBalance:  150000,
				RemoteBalance: 100000,
				Active:        true,
				Initiator:     true,
			},
		},
		services: map[string]bool{},
	}

	// Enter opens detail
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.subview != svChannelDetail {
		t.Errorf("enter should open channel detail, got subview %d", m.subview)
	}

	// Backspace returns
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.subview != svNone {
		t.Errorf("backspace should return to channels, got subview %d", m.subview)
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
	// 50/50 split
	bar := renderBalanceBar(500000, 500000, 1000000, 20)
	if len(bar) == 0 {
		t.Error("balance bar should not be empty")
	}

	// Zero capacity
	bar = renderBalanceBar(0, 0, 0, 20)
	if len(bar) == 0 {
		t.Error("zero capacity bar should not be empty")
	}
}

func TestChannelVisibleCount(t *testing.T) {
	m := testModel()
	count := m.channelVisibleCount()
	if count < 3 {
		t.Errorf("visible count too small: %d", count)
	}
}
