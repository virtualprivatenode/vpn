// internal/helper/wire_test.go

package helper

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/virtualprivatenode/vpn/internal/autounlock"
)

func TestAutoUnlockResultJSONContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result autounlock.Result
		wire   string
	}{
		{"enabled", autounlock.Result{Outcome: autounlock.Enabled},
			`{"outcome":"enabled"}`},
		{"repair", autounlock.Result{Outcome: autounlock.RepairRequired, FailedStep: "restore normal restart policy"},
			`{"outcome":"repair_required","failed_step":"restore normal restart policy"}`},
		{"retry", autounlock.Result{Outcome: autounlock.VerificationFailed, Detail: "LND is locked"},
			`{"outcome":"verification_failed","detail":"LND is locked"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.result)
			if err != nil {
				t.Fatal(err)
			}
			var got, want map[string]string
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.wire), &want); err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(got, want) {
				t.Fatalf("encoded fields = %s, want %s", data, tc.wire)
			}
			var decoded autounlock.Result
			if err := json.Unmarshal([]byte(tc.wire), &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded != tc.result {
				t.Fatalf("decoded result = %+v, want %+v", decoded, tc.result)
			}
		})
	}
}

// The same-major gate is shared by the TUI's rendering and the
// helper's refusal — one function, both sides. These tables pin
// its behavior, including the refuse-don't-guess direction on
// anything that is not a plain release version.
func TestSameMajor(t *testing.T) {
	cases := []struct {
		current, target string
		same            bool
		wantErr         bool
	}{
		{"0.7.0", "0.7.1", true, false},
		{"0.7.0", "0.9.9", true, false},
		{"0.7.0", "1.0.0", false, false},
		{"1.2.3", "2.0.0", false, false},
		{"10.0.0", "1.0.0", false, false}, // "10" != "1"
		{"dev", "0.7.1", false, true},
		{"0.7.0", "dev", false, true},
		{"0.7.0", "", false, true},
		{"0.7.0", "v0.7.1", false, true},
		{"0.7.0", "0.7.1-rc1", false, true},
		{"0.7.0", "0.7", false, true},
		{"0.7.0", "0.7.1.2", false, true},
		{"0.7.0", "0.7.1;rm -rf /", false, true},
	}
	for _, c := range cases {
		same, err := SameMajor(c.current, c.target)
		if c.wantErr {
			if err == nil {
				t.Errorf("SameMajor(%q,%q): expected error",
					c.current, c.target)
			}
			continue
		}
		if err != nil {
			t.Errorf("SameMajor(%q,%q): %v", c.current, c.target, err)
			continue
		}
		if same != c.same {
			t.Errorf("SameMajor(%q,%q) = %v, want %v",
				c.current, c.target, same, c.same)
		}
	}
}

// Streaming step lists must never be empty — the client adapter
// keys its whole lifecycle on step indices existing.
func TestStepListsNonEmpty(t *testing.T) {
	lists := map[string][]string{
		"self-update":           SelfUpdateStepNames("0.7.1"),
		"package-update":        PackageUpdateStepNames(),
		"upgrade-p2p-to-hybrid": UpgradeP2PToHybridStepNames(),
		"syncthing-install":     SyncthingInstallStepNames("2.1.1"),
	}
	for name, l := range lists {
		if len(l) == 0 {
			t.Errorf("%s: empty step list", name)
		}
		for i, n := range l {
			if n == "" {
				t.Errorf("%s: step %d has no name", name, i)
			}
		}
	}
}
