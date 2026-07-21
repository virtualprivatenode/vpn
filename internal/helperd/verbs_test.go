// internal/helperd/verbs_test.go

package helperd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/virtualprivatenode/vpn/internal/helper"
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
		helper.VerbRebuildSSHConfig,
		helper.VerbRebuildTorConfig,
		helper.VerbPackageUpdate,
		helper.VerbSelfUpdate,
		helper.VerbSetP2PMode,
		helper.VerbSyncthingInstall,
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
	ctx := &verbCtx{}
	long := strings.Repeat("a", 20)
	cases := []helper.SetUserPasswordParams{
		{User: "root", Password: long},
		{User: "bitcoin", Password: long},
		{User: "", Password: long},
		{User: "vpn", Password: "short"}, // under the minimum
		{User: "vpn", Password: "with\nnl" + long},
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

func TestSetP2PModeValidation(t *testing.T) {
	ctx := &verbCtx{}
	for _, mode := range []string{"", "clearnet", "Hybrid", "both"} {
		if _, err := verbSetP2PMode(ctx, raw(t,
			helper.SetP2PModeParams{Mode: mode})); err == nil {
			t.Errorf("accepted mode=%q", mode)
		}
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
