// internal/installer/passwordpending_test.go

package installer

import "testing"

// The four run shapes that matter, by name. The one that found
// this (live-run finding): a resume after a mid-run failure must
// re-apply, because the earlier pass's password was applied but
// never displayed. The one that must NOT fire: a re-run after a
// completed install — the operator saved (or later changed) the
// standing password, and clobbering it would invalidate it.
func TestNeedsPasswordReapply(t *testing.T) {
	cases := []struct {
		name      string
		generated string
		applied   bool
		pending   bool
		want      bool
	}{
		{"fresh unattended run, identity applied this pass",
			"pw", true, true, false},
		{"resume after mid-run failure", "pw", false, true, true},
		{"re-run after completed install", "pw", false, false,
			false},
		{"interactive pass generates no password", "", false,
			true, false},
	}
	for _, tt := range cases {
		got := needsPasswordReapply(
			tt.generated, tt.applied, tt.pending)
		if got != tt.want {
			t.Errorf("%s: got %v, want %v",
				tt.name, got, tt.want)
		}
	}
}
