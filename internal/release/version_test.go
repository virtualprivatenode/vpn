package release

import "testing"

// The same-major gate is shared by the TUI's rendering and the
// helper's refusal. These tables pin
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
