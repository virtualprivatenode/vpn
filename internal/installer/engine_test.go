// internal/installer/engine_test.go

package installer

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

// testSteps builds a miniature install list with the shapes
// that matter: plain mutations, a gate, and a 3-member
// pipeline group. Fns count executions into calls.
func testSteps(calls map[string]int) []InstallStep {
	fn := func(key string) func() error {
		return func() error {
			calls[key]++
			return nil
		}
	}
	return []InstallStep{
		{Key: "user.create", Name: "Creating user",
			Fn: fn("user.create")},
		{Key: "tor.install", Name: "Installing Tor",
			Fn: fn("tor.install")},
		{Key: "tor.gate", Name: "Verifying Tor routing",
			Kind: StepGate, Fn: fn("tor.gate")},
		{Key: "btc.download", Group: "btc", Name: "Downloading",
			Fn: fn("btc.download")},
		{Key: "btc.verify", Group: "btc", Name: "Verifying",
			Fn: fn("btc.verify")},
		{Key: "btc.install", Group: "btc", Name: "Installing",
			Fn: fn("btc.install")},
		{Key: "shellenv", Name: "Shell environment",
			Fn: fn("shellenv")},
	}
}

func keysToRun(steps []InstallStep, plan []stepPlan) []string {
	var out []string
	for i, p := range plan {
		if p.Run {
			out = append(out, steps[i].Key)
		}
	}
	return out
}

func sameKeys(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestValidateSteps(t *testing.T) {
	ok := testSteps(map[string]int{})
	if err := validateSteps(ok); err != nil {
		t.Errorf("valid steps rejected: %v", err)
	}

	dup := testSteps(map[string]int{})
	dup[1].Key = "user.create"
	if err := validateSteps(dup); err == nil {
		t.Error("duplicate key accepted")
	}

	empty := testSteps(map[string]int{})
	empty[2].Key = ""
	if err := validateSteps(empty); err == nil {
		t.Error("empty key accepted")
	}
}

func TestPlanScenarios(t *testing.T) {
	all := []string{"user.create", "tor.install", "tor.gate",
		"btc.download", "btc.verify", "btc.install", "shellenv"}

	cases := []struct {
		name    string
		done    []string // pre-recorded ledger keys
		wantRun []string
	}{
		{
			name:    "fresh install runs everything",
			done:    nil,
			wantRun: all,
		},
		{
			name:    "all done: only the gate re-runs",
			done:    all,
			wantRun: []string{"tor.gate"},
		},
		{
			name: "resume after step 2: gate re-runs, rest forward",
			done: []string{"user.create", "tor.install"},
			wantRun: []string{"tor.gate", "btc.download",
				"btc.verify", "btc.install", "shellenv"},
		},
		{
			// THE group test: download+verify recorded but the
			// terminal (btc.install) is not — the whole group
			// re-runs (the workdir died with the old process).
			name: "incomplete group re-runs whole",
			done: []string{"user.create", "tor.install",
				"btc.download", "btc.verify"},
			wantRun: []string{"tor.gate", "btc.download",
				"btc.verify", "btc.install", "shellenv"},
		},
		{
			// Terminal recorded: the group is complete even if
			// member entries are absent (only the terminal is
			// consulted for group members).
			name: "group judged by terminal only",
			done: []string{"user.create", "tor.install",
				"btc.install"},
			wantRun: []string{"tor.gate", "shellenv"},
		},
		{
			name: "unknown ledger keys ignored",
			done: []string{"user.create", "no.such.step",
				"another.ghost"},
			wantRun: []string{"tor.install", "tor.gate",
				"btc.download", "btc.verify", "btc.install",
				"shellenv"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			led := newLedger()
			for _, k := range c.done {
				led.markDone(k, "0.6.3")
			}
			steps := testSteps(map[string]int{})
			plan := planRun(steps, led)
			got := keysToRun(steps, plan)
			if !sameKeys(got, c.wantRun) {
				t.Errorf("run set = %v, want %v", got, c.wantRun)
			}
		})
	}
}

// The gate must re-run even when its own key is recorded —
// a recorded gate means "it held on some earlier pass," which
// is exactly what a gate must never rely on.
func TestPlanGateIgnoresOwnLedgerEntry(t *testing.T) {
	led := newLedger()
	led.markDone("tor.gate", "0.6.3")
	steps := testSteps(map[string]int{})
	plan := planRun(steps, led)
	if !plan[2].Run {
		t.Fatal("gate was ledger-skipped")
	}
}

func TestFilterPhase(t *testing.T) {
	steps := []InstallStep{
		{Key: "a", Phase: PhaseBake},
		{Key: "b", Phase: PhaseFirstBoot},
		{Key: "c", Phase: PhaseBake},
	}
	bake := FilterPhase(steps, PhaseBake)
	if len(bake) != 2 || bake[0].Key != "a" || bake[1].Key != "c" {
		t.Errorf("bake filter = %v", bake)
	}
	allSteps := FilterPhase(steps, PhaseFirstBoot)
	if len(allSteps) != 3 {
		t.Errorf("first-boot filter kept %d of 3", len(allSteps))
	}
}

func TestRunnerSkipDoesNotExecute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	led := newLedger()
	led.markDone("user.create", "0.6.3")
	if err := led.save(path); err != nil {
		t.Fatal(err)
	}

	calls := map[string]int{}
	r, err := newStepRunner(testSteps(calls), "0.6.3", path)
	if err != nil {
		t.Fatal(err)
	}
	skipped, err := r.runIndex(0)
	if err != nil {
		t.Fatalf("runIndex: %v", err)
	}
	if !skipped {
		t.Error("recorded step not skipped")
	}
	if calls["user.create"] != 0 {
		t.Error("skipped step's Fn executed")
	}
}

func TestRunnerRecordsAfterSuccessOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	calls := map[string]int{}
	steps := testSteps(calls)
	boom := errors.New("boom")
	steps[1].Fn = func() error { return boom }

	r, err := newStepRunner(steps, "0.6.3", path)
	if err != nil {
		t.Fatal(err)
	}

	// Success: entry lands on disk.
	if _, err := r.runIndex(0); err != nil {
		t.Fatalf("step 0: %v", err)
	}
	if !loadLedger(path).done("user.create") {
		t.Error("completed step not recorded on disk")
	}

	// Failure: no entry, error surfaced.
	_, err = r.runIndex(1)
	if !errors.Is(err, boom) {
		t.Fatalf("step 1 err = %v, want boom", err)
	}
	if loadLedger(path).done("tor.install") {
		t.Error("failed step recorded as done")
	}
}

func TestClassifyRun(t *testing.T) {
	steps := testSteps(map[string]int{})
	total := len(steps)

	res := classifyRun(steps, true, false, total)
	if res.Outcome != RunComplete {
		t.Errorf("complete run classified %v", res.Outcome)
	}

	failed := testSteps(map[string]int{})
	failed[3].Status = StepFailed
	failed[3].Err = errors.New("download failed")
	res = classifyRun(failed, true, true, 3)
	if res.Outcome != RunFailed {
		t.Errorf("failed run classified %v", res.Outcome)
	}
	if res.StepNum != 4 || res.StepName != "Downloading" {
		t.Errorf("failure attributed to %d/%q",
			res.StepNum, res.StepName)
	}
	if res.Err == nil {
		t.Error("failed run lost its error")
	}

	// Quit mid-run: done=false → interrupted, at the current
	// step — never complete.
	res = classifyRun(steps, false, false, 2)
	if res.Outcome != RunInterrupted {
		t.Errorf("interrupted run classified %v", res.Outcome)
	}
	if res.StepNum != 3 || res.StepName != "Verifying Tor routing" {
		t.Errorf("interrupt attributed to %d/%q",
			res.StepNum, res.StepName)
	}

	// Failed flag with no failed step: fail-safe, never complete.
	res = classifyRun(steps, true, true, total)
	if res.Outcome != RunFailed {
		t.Errorf("inconsistent state classified %v (must fail safe)",
			res.Outcome)
	}
}

func TestRunInstallUnattendedCompleteAndResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	// First pass: everything runs, outcome complete.
	calls := map[string]int{}
	res, err := RunInstallUnattended(testSteps(calls), "0.6.3", path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != RunComplete {
		t.Fatalf("first pass outcome %v", res.Outcome)
	}
	for k, n := range calls {
		if n != 1 {
			t.Errorf("first pass ran %s %d times", k, n)
		}
	}

	// Second pass over the same ledger: only the gate runs.
	calls2 := map[string]int{}
	res, err = RunInstallUnattended(testSteps(calls2), "0.6.3", path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != RunComplete {
		t.Fatalf("second pass outcome %v", res.Outcome)
	}
	for k, n := range calls2 {
		want := 0
		if k == "tor.gate" {
			want = 1
		}
		if n != want {
			t.Errorf("second pass ran %s %d times, want %d",
				k, n, want)
		}
	}
}

func TestRunInstallUnattendedFailureStops(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	calls := map[string]int{}
	steps := testSteps(calls)
	steps[2].Fn = func() error {
		calls["tor.gate"]++
		return fmt.Errorf("tor not routing")
	}

	res, err := RunInstallUnattended(steps, "0.6.3", path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != RunFailed {
		t.Fatalf("outcome %v, want RunFailed", res.Outcome)
	}
	if res.StepNum != 3 {
		t.Errorf("failed at %d, want 3", res.StepNum)
	}
	// Nothing after the failure ran.
	for _, k := range []string{"btc.download", "btc.verify",
		"btc.install", "shellenv"} {
		if calls[k] != 0 {
			t.Errorf("%s ran after the failure", k)
		}
	}
	// Steps before it are recorded; the gate is not.
	led := loadLedger(path)
	if !led.done("user.create") || !led.done("tor.install") {
		t.Error("pre-failure steps not recorded")
	}
	if led.done("tor.gate") {
		t.Error("failed gate recorded as done")
	}
}
