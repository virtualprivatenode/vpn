// internal/installer/strand_test.go

package installer

import "testing"

// The unattended zero-key strand guard: refuse exactly when
// completing would leave no SSH way in and nobody consented to
// that by name. Notably NOT refused: password auth on (the
// password is a network credential then), any key present, or
// explicit --allow-console-only consent.
func TestStrandsBox(t *testing.T) {
	cases := []struct {
		keys                           int
		passwordAuth, allowConsoleOnly bool
		want                           bool
	}{
		{0, false, false, true},  // the strand
		{0, false, true, false},  // consented by name
		{0, true, false, false},  // password is a way in
		{1, false, false, false}, // a key is a way in
		{3, true, true, false},
	}
	for _, c := range cases {
		got := strandsBox(c.keys, c.passwordAuth,
			c.allowConsoleOnly)
		if got != c.want {
			t.Errorf("strandsBox(%d,%v,%v) = %v, want %v",
				c.keys, c.passwordAuth, c.allowConsoleOnly,
				got, c.want)
		}
	}
}
