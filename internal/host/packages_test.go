package host

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestPackageUpdateCommandsAndFailureBoundaries(t *testing.T) {
	failure := errors.New("injected command failure")
	for _, tc := range []struct {
		name      string
		fail      int
		audit     string
		wantCalls int
		wantSteps []int
	}{
		{name: "complete", wantCalls: 3, wantSteps: []int{0, 1}},
		{name: "refresh fails", fail: 1, wantCalls: 1},
		{name: "upgrade fails", fail: 2, wantCalls: 2, wantSteps: []int{0}},
		{name: "audit command fails", fail: 3, wantCalls: 3, wantSteps: []int{0, 1}},
		{name: "audit reports inconsistency", audit: " package is only half configured\n", wantCalls: 3, wantSteps: []int{0, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			var steps []int
			run := func(name string, args ...string) error {
				calls = append(calls, append([]string{name}, args...))
				if len(calls) == tc.fail {
					return failure
				}
				return nil
			}
			err := updatePackages(run, func(timeout time.Duration, name string, args ...string) (string, error) {
				if timeout != 10*time.Second {
					t.Errorf("read-only audit bound: %v", timeout)
				}
				return tc.audit, run(name, args...)
			}, func(i int) { steps = append(steps, i) })
			want := [][]string{
				{"apt-get", "update", "--error-on=any", "-qq"},
				{"apt-get", "upgrade", "-y", "-qq", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"},
				{"dpkg", "--audit"},
			}
			if !reflect.DeepEqual(calls, want[:tc.wantCalls]) || !reflect.DeepEqual(steps, tc.wantSteps) {
				t.Fatalf("commands %v, completed stages %v", calls, steps)
			}
			if (err != nil) != (tc.fail != 0 || tc.audit != "") || (tc.fail != 0 && !errors.Is(err, failure)) {
				t.Fatalf("lost or invented failure: %v", err)
			}
		})
	}
}
