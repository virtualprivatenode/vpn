// internal/installer/handoff_test.go

package installer

import "testing"

// ── journalShowsAdminLogin ───────────────────────────────

func TestJournalShowsAdminLogin(t *testing.T) {
	accepted := []struct {
		name, journal string
	}{
		{"publickey",
			"Accepted publickey for vpn from 203.0.113.7 " +
				"port 51234 ssh2: ED25519 SHA256:abc\n"},
		{"password",
			"Accepted password for vpn from 203.0.113.7 " +
				"port 51234 ssh2\n"},
		{"buried among noise",
			"Connection closed by 198.51.100.9\n" +
				"Failed password for vpn from 198.51.100.9 port 1\n" +
				"Accepted publickey for vpn from 203.0.113.7 " +
				"port 2 ssh2: ED25519 SHA256:abc\n"},
	}
	for _, tt := range accepted {
		if !journalShowsAdminLogin(tt.journal) {
			t.Errorf("%s: not detected", tt.name)
		}
	}

	rejected := []struct {
		name, journal string
	}{
		{"empty", ""},
		{"failed attempt only",
			"Failed password for vpn from 198.51.100.9 port 1\n"},
		{"different user",
			"Accepted publickey for root from 203.0.113.7 " +
				"port 2 ssh2\n"},
		{"prefix user must not match",
			"Accepted publickey for vpnadmin from 203.0.113.7 " +
				"port 2 ssh2\n"},
		{"invalid user probe",
			"Invalid user vpn from 198.51.100.9 port 1\n"},
	}
	for _, tt := range rejected {
		if journalShowsAdminLogin(tt.journal) {
			t.Errorf("%s: false positive", tt.name)
		}
	}
}
