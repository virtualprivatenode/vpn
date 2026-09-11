// internal/helperd/verbs_test.go

package helperd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/installer"
	"github.com/virtualprivatenode/vpn/internal/loginpassword"
)

// Every verb on the menu carries a deadline: an operation with
// no ceiling could hold the serialized queue forever.
func TestEveryVerbHasDeadline(t *testing.T) {
	if len(verbs) == 0 {
		t.Fatal("empty verb menu")
	}
	for name, def := range verbs {
		if def.deadline <= 0 {
			t.Errorf("%s: no deadline", name)
		}
		if def.handler == nil {
			t.Errorf("%s: no handler", name)
		}
		if def.deadline > time.Hour {
			t.Errorf("%s: deadline %v is implausibly long",
				name, def.deadline)
		}
	}
}

func TestServiceActionDeadlineCoversLNDStopAndNotifiedStart(t *testing.T) {
	// The installed LND unit permits 300 seconds to stop and 1200 seconds
	// to notify readiness during an ordinary restart. The helper socket must
	// remain available for that complete systemd transaction.
	if got := verbs[helper.VerbServiceAction].deadline; got < 25*time.Minute {
		t.Fatalf("service-action deadline = %s, want at least 25m", got)
	}
}

// The menu is closed: exactly the ruled verbs, nothing else.
// Adding a verb must be a deliberate act that updates this
// list too.
func TestVerbMenuIsExactlyTheRuledSet(t *testing.T) {
	want := []string{
		helper.VerbServiceAction,
		helper.VerbReboot,
		helper.VerbDirSize,
		helper.VerbSetUserPassword,
		helper.VerbStageWalletPassword,
		helper.VerbRemoveWalletPassword,
		helper.VerbStageLNDCredentials,
		helper.VerbStageLNDMacaroon,
		helper.VerbRebuildSSHConfig,
		helper.VerbPackageUpdate,
		helper.VerbSelfUpdate,
		helper.VerbUpgradeP2PToHybrid,
		helper.VerbSyncthingInstall,
		// Live-read verbs have no parameters or mutation. The verification
		// verb also has no parameters but may clear its private marker. They
		// serve the live-read display facts (onion addresses,
		// the Syncthing device ID, the SSH password-auth
		// answer), which keep no board copy.
		helper.VerbReadNodeAddresses,
		helper.VerbReadSSHAuth,
		helper.VerbReadWalletState,
		helper.VerbReadKeyVerificationState,
		helper.VerbVerifyAdminLogin,
	}
	if len(verbs) != len(want) {
		t.Errorf("verb menu has %d entries, want %d",
			len(verbs), len(want))
	}
	for _, v := range want {
		if _, ok := verbs[v]; !ok {
			t.Errorf("menu is missing %s", v)
		}
	}
}

func withConfigVerbTestDeps(t *testing.T) {
	t.Helper()
	oldLoad := loadSystemConfig
	oldSave := saveSystemConfig
	oldSetup := setupAutoUnlock
	oldDisable := disableAutoUnlock
	oldIP := publicIPv4
	oldP2P := p2pUpgradeSteps
	oldSync := syncthingInstallSteps
	oldResidue := syncthingResiduePresent
	oldPrerequisites := verifySyncthingPrerequisites
	oldWallet := walletExists
	oldKeyPending := keyVerificationPending
	oldVerifyLogin := verifyAdminLogin
	oldStagePassword := stageSyncthingWebPassword
	oldRestage := restageFacts
	verifySyncthingPrerequisites = func(*config.AppConfig) error { return nil }
	t.Cleanup(func() {
		loadSystemConfig = oldLoad
		saveSystemConfig = oldSave
		setupAutoUnlock = oldSetup
		disableAutoUnlock = oldDisable
		publicIPv4 = oldIP
		p2pUpgradeSteps = oldP2P
		syncthingInstallSteps = oldSync
		syncthingResiduePresent = oldResidue
		verifySyncthingPrerequisites = oldPrerequisites
		walletExists = oldWallet
		keyVerificationPending = oldKeyPending
		verifyAdminLogin = oldVerifyLogin
		stageSyncthingWebPassword = oldStagePassword
		restageFacts = oldRestage
	})
}

func TestAutoUnlockVerbsReturnStructuredTransitionResults(t *testing.T) {
	withConfigVerbTestDeps(t)
	wantEnable := installer.AutoUnlockResult{
		Outcome: installer.AutoUnlockVerificationFailed,
	}
	setupAutoUnlock = func(password string) (installer.AutoUnlockResult, error) {
		if password != "correct horse" {
			t.Fatalf("password = %q", password)
		}
		return wantEnable, nil
	}
	wantDisable := installer.AutoUnlockResult{
		Outcome: installer.AutoUnlockStillEnabled,
	}
	disableAutoUnlock = func() (installer.AutoUnlockResult, error) {
		return wantDisable, nil
	}

	got, err := verbStageWalletPassword(&verbCtx{}, raw(t,
		helper.StageWalletPasswordParams{Password: "correct horse"}))
	if err != nil || got != wantEnable {
		t.Fatalf("enable result = %#v, %v", got, err)
	}
	got, err = verbRemoveWalletPassword(&verbCtx{}, nil)
	if err != nil || got != wantDisable {
		t.Fatalf("disable result = %#v, %v", got, err)
	}
}

func TestP2PPersistenceFailureIsOperationFailure(t *testing.T) {
	withConfigVerbTestDeps(t)
	wantErr := errors.New("injected config save failure")
	loadSystemConfig = func() (*config.AppConfig, error) { return config.Default(), nil }
	saveSystemConfig = func(*config.AppConfig) error { return wantErr }
	publicIPv4 = func() string { return "203.0.113.7" }
	p2pUpgradeSteps = func(*config.AppConfig, string) []installer.InstallStep {
		return []installer.InstallStep{{Name: "apply", Fn: func() error { return nil }}}
	}
	restageFacts = func(string) error { return nil }
	if _, err := verbUpgradeP2PToHybrid(
		&verbCtx{}, nil); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want injected save failure", err)
	}
}

func TestSyncthingStagesPasswordButNeverReturnsIt(t *testing.T) {
	withConfigVerbTestDeps(t)
	loadSystemConfig = func() (*config.AppConfig, error) { return config.Default(), nil }
	saveSystemConfig = func(cfg *config.AppConfig) error {
		if !cfg.SyncthingEnabled {
			t.Fatal("published config does not enable Syncthing")
		}
		return nil
	}
	syncthingResiduePresent = func() (bool, error) { return false, nil }
	syncthingInstallSteps = func(*config.AppConfig) ([]installer.InstallStep, string, error) {
		return []installer.InstallStep{{Name: "apply", Fn: func() error { return nil }}}, "secret-web-password", nil
	}
	restageFacts = func(string) error { return nil }
	staged := ""
	stageSyncthingWebPassword = func(password string) error { staged = password; return nil }
	result, err := verbSyncthingInstall(&verbCtx{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if staged != "secret-web-password" {
		t.Fatalf("staged %q", staged)
	}
	if result != nil {
		t.Fatalf("helper returned secret-bearing result: %#v", result)
	}
}

func TestSyncthingRefusesResidueBeforeMutation(t *testing.T) {
	withConfigVerbTestDeps(t)
	loadSystemConfig = func() (*config.AppConfig, error) { return config.Default(), nil }
	syncthingResiduePresent = func() (bool, error) { return true, nil }
	mutated := false
	syncthingInstallSteps = func(*config.AppConfig) ([]installer.InstallStep, string, error) {
		mutated = true
		return nil, "", nil
	}
	if _, err := verbSyncthingInstall(&verbCtx{}, nil); err == nil ||
		!strings.Contains(err.Error(), "residue") {
		t.Fatalf("residue refusal error = %v", err)
	}
	if mutated {
		t.Fatal("Syncthing mutation began despite residue")
	}
}

func TestSyncthingRefusesFailedPrerequisiteBeforeMutation(t *testing.T) {
	withConfigVerbTestDeps(t)
	loadSystemConfig = func() (*config.AppConfig, error) {
		return config.Default(), nil
	}
	syncthingResiduePresent = func() (bool, error) { return false, nil }
	wantErr := errors.New("Tor is inactive")
	verifySyncthingPrerequisites = func(*config.AppConfig) error {
		return wantErr
	}
	mutated := false
	syncthingInstallSteps = func(
		*config.AppConfig,
	) ([]installer.InstallStep, string, error) {
		mutated = true
		return nil, "", nil
	}
	if _, err := verbSyncthingInstall(
		&verbCtx{}, nil); !errors.Is(err, wantErr) {
		t.Fatalf("prerequisite error=%v want=%v", err, wantErr)
	}
	if mutated {
		t.Fatal("Syncthing mutation began despite failed prerequisite")
	}
}

func TestSyncthingFinalSaveFailureLeavesResidueAndReportsFailure(t *testing.T) {
	withConfigVerbTestDeps(t)
	wantErr := errors.New("injected final save failure")
	loadSystemConfig = func() (*config.AppConfig, error) { return config.Default(), nil }
	saveSystemConfig = func(*config.AppConfig) error { return wantErr }
	syncthingResiduePresent = func() (bool, error) { return false, nil }
	installed := false
	syncthingInstallSteps = func(*config.AppConfig) ([]installer.InstallStep, string, error) {
		return []installer.InstallStep{{Name: "apply", Fn: func() error {
			installed = true
			return nil
		}}}, "secret-web-password", nil
	}
	restageFacts = func(string) error { return nil }
	staged := false
	stageSyncthingWebPassword = func(string) error { staged = true; return nil }
	if _, err := verbSyncthingInstall(&verbCtx{}, nil); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want final save failure", err)
	}
	if !installed || !staged {
		t.Fatalf("residue not preserved: installed=%v staged=%v", installed, staged)
	}
}

func TestWalletAndVerificationReadsFailIndependently(t *testing.T) {
	withConfigVerbTestDeps(t)
	cfg := config.Default()
	cfg.Network = "testnet4"
	loadSystemConfig = func() (*config.AppConfig, error) { return cfg, nil }
	walletExists = func(network string) (bool, error) {
		if network != "testnet4" {
			t.Fatalf("wallet observed for %q", network)
		}
		return true, nil
	}
	keyVerificationPending = func() (bool, error) {
		return false, errors.New("marker unreadable")
	}
	result, err := verbReadWalletState(&verbCtx{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := result.(helper.WalletStateResult)
	if !got.WalletExists {
		t.Fatalf("live wallet state = %+v", got)
	}
	walletErr := errors.New("LND state RPC unavailable")
	walletExists = func(string) (bool, error) {
		return false, walletErr
	}
	if _, err := verbReadWalletState(&verbCtx{}, nil); !errors.Is(err, walletErr) {
		t.Fatalf("unknown wallet fact was not preserved: %v", err)
	}
	if _, err := verbReadKeyVerificationState(
		&verbCtx{}, nil); err == nil {
		t.Fatal("unreadable verification marker reported a state")
	}

	keyVerificationPending = func() (bool, error) { return true, nil }
	result, err = verbReadKeyVerificationState(&verbCtx{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	verification := result.(helper.KeyVerificationStateResult)
	if !verification.Pending {
		t.Fatalf("verification state = %+v", verification)
	}

	verifyAdminLogin = func() (bool, bool, error) { return false, true, nil }
	result, err = verbVerifyAdminLogin(&verbCtx{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	verified := result.(helper.VerifyAdminLoginResult)
	if verified.Pending || !verified.Verified {
		t.Fatalf("verification result = %+v", verified)
	}
}

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Parameter validation refuses everything outside the closed
// sets, before any side effect. (Only refusal paths run here —
// success paths mutate a system and belong to the live run.)
func TestServiceActionValidation(t *testing.T) {
	ctx := &verbCtx{}
	cases := []helper.ServiceActionParams{
		{Unit: "sshd", Action: "restart"},     // not a managed unit
		{Unit: "bitcoind", Action: "disable"}, // not an allowed action
		{Unit: "", Action: ""},
		{Unit: "bitcoind; rm -rf /", Action: "start"},
	}
	for _, c := range cases {
		if _, err := verbServiceAction(ctx, raw(t, c)); err == nil {
			t.Errorf("accepted %+v", c)
		}
	}
	// Unknown fields refuse too (strict decoding).
	if _, err := verbServiceAction(ctx, json.RawMessage(
		`{"unit":"tor","action":"restart","extra":1}`)); err == nil {
		t.Error("accepted unknown params field")
	}
}

func TestDirSizeValidation(t *testing.T) {
	ctx := &verbCtx{}
	for _, which := range []string{
		"bitcoin", "", "/etc", "../lnd", "lnd/..",
	} {
		if _, err := verbDirSize(ctx, raw(t,
			helper.DirSizeParams{Which: which})); err == nil {
			t.Errorf("accepted which=%q", which)
		}
	}
}

func TestSetUserPasswordValidation(t *testing.T) {
	old := setLoginPassword
	t.Cleanup(func() { setLoginPassword = old })
	setLoginPassword = func(loginpassword.Password) error { t.Fatal("invalid request reached host operation"); return nil }
	ctx := &verbCtx{}
	long := strings.Repeat("a", 20)
	cases := []helper.SetUserPasswordParams{
		{User: "root", Password: long},
		{User: "bitcoin", Password: long},
		{User: "", Password: long},
		{User: "vpn", Password: "short"}, // under the minimum
		{User: "vpn", Password: "with\nnl" + long},
		{User: "vpn", Password: long + "\x00suffix"},
		{User: "vpn", Password: ""},
	}
	for _, c := range cases {
		if _, err := verbSetUserPassword(ctx, raw(t, c)); err == nil {
			t.Errorf("accepted user=%q pwlen=%d",
				c.User, len(c.Password))
		}
	}
}

func TestWalletPasswordValidation(t *testing.T) {
	if err := validateWalletPassword(""); err == nil {
		t.Error("accepted empty wallet password")
	}
	if err := validateWalletPassword(
		strings.Repeat("x", 513)); err == nil {
		t.Error("accepted oversized wallet password")
	}
	if err := validateWalletPassword("a\nb"); err == nil {
		t.Error("accepted wallet password with newline")
	}
	if err := validateWalletPassword("correct horse"); err != nil {
		t.Errorf("rejected a normal wallet password: %v", err)
	}
}

func TestP2PUpgradeRefusesAnythingButTorCurrentMode(t *testing.T) {
	withConfigVerbTestDeps(t)
	cfg := config.Default()
	cfg.P2PMode = "hybrid"
	loadSystemConfig = func() (*config.AppConfig, error) { return cfg, nil }
	called := false
	publicIPv4 = func() string { called = true; return "203.0.113.7" }
	p2pUpgradeSteps = func(*config.AppConfig, string) []installer.InstallStep {
		called = true
		return nil
	}
	if _, err := verbUpgradeP2PToHybrid(&verbCtx{}, nil); err == nil {
		t.Fatal("hybrid-to-hybrid transition accepted")
	}
	if called {
		t.Fatal("P2P mutation preparation began before current-mode refusal")
	}
}

func TestP2PUpgradeAcceptsNoCallerSelectedMode(t *testing.T) {
	withConfigVerbTestDeps(t)
	loaded := false
	loadSystemConfig = func() (*config.AppConfig, error) {
		loaded = true
		return config.Default(), nil
	}
	if _, err := verbUpgradeP2PToHybrid(
		&verbCtx{}, raw(t, map[string]string{
			"mode": "tor",
		})); err == nil {
		t.Fatal("one-way P2P upgrade accepted caller-selected mode")
	}
	if loaded {
		t.Fatal("P2P request with parameters reached authoritative config read")
	}
}

// The self-update gate refuses before any network or disk
// activity: bad target shapes, cross-major targets, and a
// non-release running version all stop at the boundary.
func TestSelfUpdateGate(t *testing.T) {
	ctx := &verbCtx{version: "0.7.0"}
	for _, target := range []string{
		"", "dev", "v0.7.1", "0.7", "1.0.0", "2.3.4",
		"0.7.1-rc1", "0.7.1;curl evil",
	} {
		if _, err := verbSelfUpdate(ctx, raw(t,
			helper.SelfUpdateParams{
				Version: target,
			})); err == nil {
			t.Errorf("gate passed target %q", target)
		}
	}
	// A dev build refuses everything (cannot prove same-major).
	dev := &verbCtx{version: "dev"}
	if _, err := verbSelfUpdate(dev, raw(t,
		helper.SelfUpdateParams{Version: "0.7.1"})); err == nil {
		t.Error("dev build accepted a self-update")
	}
}

// decode is strict: unknown fields and malformed JSON refuse.
func TestDecodeStrict(t *testing.T) {
	var p helper.RebuildSSHConfigParams
	if err := decode(json.RawMessage(
		`{"password_auth_disabled":true,"bonus":1}`), &p); err == nil {
		t.Error("accepted unknown field")
	}
	if err := decode(json.RawMessage(`{`), &p); err == nil {
		t.Error("accepted malformed JSON")
	}
	if err := decode(json.RawMessage(
		`{"password_auth_disabled":true}`), &p); err != nil {
		t.Errorf("rejected valid params: %v", err)
	}
}

func TestLoginPasswordMarkerFollowsConfirmedChange(t *testing.T) {
	oldSet, oldClear := setLoginPassword, clearPasswordPendingMarker
	t.Cleanup(func() { setLoginPassword, clearPasswordPendingMarker = oldSet, oldClear })
	for _, tc := range []struct {
		name             string
		setErr, clearErr error
		cleared          bool
	}{
		{"success", nil, nil, true},
		{"unconfirmed change", errors.New("process failed"), nil, false},
		{"cleanup failure after success", nil, errors.New("remove failed"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleared := false
			setLoginPassword = func(p loginpassword.Password) error {
				if p.Text() != "test password for marker" {
					t.Error("password altered at helper boundary")
				}
				return tc.setErr
			}
			clearPasswordPendingMarker = func() error { cleared = true; return tc.clearErr }
			_, err := verbSetUserPassword(&verbCtx{}, raw(t, helper.SetUserPasswordParams{User: "vpn", Password: "test password for marker"}))
			if (err != nil) != (tc.setErr != nil) || cleared != tc.cleared {
				t.Fatalf("changed password and marker outcome conflated: %v", err)
			}
		})
	}
}
