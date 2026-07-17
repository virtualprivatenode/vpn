// internal/installer/observe_test.go

package installer

import (
	"strings"
	"testing"
)

// ── parseSSHObservation ──────────────────────────────────

func TestParseSSHObservationDefaultBox(t *testing.T) {
	out := "port 22\n" +
		"addressfamily any\n" +
		"listenaddress [::]:22\n" +
		"listenaddress 0.0.0.0:22\n" +
		"passwordauthentication yes\n" +
		"permitrootlogin without-password\n"
	obs, err := parseSSHObservation(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(obs.Ports) != 1 || obs.Ports[0] != 22 {
		t.Errorf("ports: got %v, want [22]", obs.Ports)
	}
	if !obs.PasswordAuth {
		t.Error("password auth: got disabled, want enabled")
	}
}

func TestParseSSHObservationNonstandardPort(t *testing.T) {
	out := "port 2222\n" +
		"listenaddress [::]:2222\n" +
		"listenaddress 0.0.0.0:2222\n" +
		"passwordauthentication no\n"
	obs, err := parseSSHObservation(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(obs.Ports) != 1 || obs.Ports[0] != 2222 {
		t.Errorf("ports: got %v, want [2222]", obs.Ports)
	}
	if obs.PasswordAuth {
		t.Error("password auth: got enabled, want disabled")
	}
}

// A ListenAddress with an explicit port makes sshd listen there
// even when it differs from the Port directive — the union is
// what the firewall must allow (the lockout direction).
func TestParseSSHObservationPortUnion(t *testing.T) {
	out := "port 22\n" +
		"listenaddress 0.0.0.0:2222\n" +
		"passwordauthentication yes\n"
	obs, err := parseSSHObservation(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(obs.Ports) != 2 || obs.Ports[0] != 22 ||
		obs.Ports[1] != 2222 {
		t.Errorf("ports: got %v, want [22 2222]", obs.Ports)
	}
}

func TestParseSSHObservationMultiplePortLines(t *testing.T) {
	out := "port 22\nport 2222\npasswordauthentication yes\n"
	obs, err := parseSSHObservation(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(obs.Ports) != 2 {
		t.Errorf("ports: got %v, want two ports", obs.Ports)
	}
}

// First match wins for passwordauthentication, like sshd's own
// config semantics.
func TestParseSSHObservationFirstPwAuthWins(t *testing.T) {
	out := "port 22\n" +
		"passwordauthentication no\n" +
		"passwordauthentication yes\n"
	obs, err := parseSSHObservation(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.PasswordAuth {
		t.Error("first-match: got enabled, want disabled")
	}
}

func TestParseSSHObservationRefusals(t *testing.T) {
	cases := []struct {
		name, out, wantIn string
	}{
		{"no port line",
			"passwordauthentication yes\n", "no listening port"},
		{"no passwordauthentication",
			"port 22\n", "passwordauthentication not present"},
		{"empty output", "", "no listening port"},
	}
	for _, tt := range cases {
		if _, err := parseSSHObservation(tt.out); err == nil {
			t.Errorf("%s: accepted, want error", tt.name)
		} else if !strings.Contains(err.Error(), tt.wantIn) {
			t.Errorf("%s: error %q does not mention %q",
				tt.name, err, tt.wantIn)
		}
	}
}

// ── portFromListenAddress ────────────────────────────────

func TestPortFromListenAddress(t *testing.T) {
	cases := []struct {
		addr string
		port int
		ok   bool
	}{
		{"0.0.0.0:22", 22, true},
		{"[::]:22", 22, true},
		{"[::]:2222", 2222, true},
		{"192.168.1.1:2022", 2022, true},
		{"0.0.0.0", 0, false},        // no port
		{"0.0.0.0:", 0, false},       // empty port
		{"0.0.0.0:notnum", 0, false}, // garbage
		{"0.0.0.0:0", 0, false},      // out of range
		{"0.0.0.0:70000", 0, false},  // out of range
		{"::1", 0, false},            // bare IPv6, no bracket
	}
	for _, tt := range cases {
		p, ok := portFromListenAddress(tt.addr)
		if ok != tt.ok || (ok && p != tt.port) {
			t.Errorf("portFromListenAddress(%q) = (%d,%v), "+
				"want (%d,%v)", tt.addr, p, ok, tt.port, tt.ok)
		}
	}
}
