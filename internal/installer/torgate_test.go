// internal/installer/torgate_test.go

package installer

import (
	"errors"
	"testing"
	"time"
)

func TestParseBootstrapProgress(t *testing.T) {
	cases := []struct {
		name string
		line string
		want int
		ok   bool
	}{
		{"done",
			`250-status/bootstrap-phase=NOTICE BOOTSTRAP PROGRESS=100 TAG=done SUMMARY="Done"` + "\r\n",
			100, true},
		{"partial",
			`250-status/bootstrap-phase=NOTICE BOOTSTRAP PROGRESS=14 TAG=handshake SUMMARY="Handshaking with a relay"` + "\r\n",
			14, true},
		{"zero",
			`250-status/bootstrap-phase=NOTICE BOOTSTRAP PROGRESS=0 TAG=starting SUMMARY="Starting"`,
			0, true},
		{"no progress field", `250 OK`, 0, false},
		{"progress no digits", `PROGRESS=x`, 0, false},
		{"empty", ``, 0, false},
	}
	for _, c := range cases {
		got, ok := parseBootstrapProgress(c.line)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: parseBootstrapProgress(%q) = (%d,%v), want (%d,%v)",
				c.name, c.line, got, ok, c.want, c.ok)
		}
	}
}

func TestTorProbeVerdict(t *testing.T) {
	cases := []struct {
		name   string
		output string
		err    error
		want   torProbeResult
	}{
		{"tor exit", `{"IsTor":true,"IP":"185.220.101.1"}`, nil, probeTor},
		{"clearnet exit — the leak case",
			`{"IsTor":false,"IP":"203.0.113.7"}`, nil, probeClearnet},
		{"curl error", "", errors.New("exit status 6"), probeUnreachable},
		// An error must win even if output looks like success —
		// never confirm routing off a failed command.
		{"error with tor-looking output",
			`{"IsTor":true}`, errors.New("exit status 28"),
			probeUnreachable},
		{"captive portal / garbage", `<html>login required</html>`,
			nil, probeUnreachable},
		{"empty output", ``, nil, probeUnreachable},
	}
	for _, c := range cases {
		if got := torProbeVerdict(c.output, c.err); got != c.want {
			t.Errorf("%s: torProbeVerdict(%q, %v) = %v, want %v",
				c.name, c.output, c.err, got, c.want)
		}
	}
}

// ── keepWaitingForTor (the stall/ceiling clock rule) ─────

func TestKeepWaitingForTor(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stall := 90 * time.Second
	ceiling := 10 * time.Minute

	cases := []struct {
		name        string
		sinceStart  time.Duration
		sinceMove   time.Duration
		keepWaiting bool
	}{
		{"fresh start", 0, 0, true},
		{"advancing slowly, well under both",
			5 * time.Minute, 10 * time.Second, true},
		{"just under the stall window",
			2 * time.Minute, 89 * time.Second, true},
		{"stalled exactly at the window",
			2 * time.Minute, 90 * time.Second, false},
		{"stalled past the window",
			2 * time.Minute, 3 * time.Minute, false},
		{"ceiling reached while still advancing",
			10 * time.Minute, 5 * time.Second, false},
		{"past the ceiling",
			11 * time.Minute, 5 * time.Second, false},
	}
	for _, tt := range cases {
		now := base.Add(tt.sinceStart)
		lastAdvance := now.Add(-tt.sinceMove)
		got := keepWaitingForTor(now, base, lastAdvance,
			stall, ceiling)
		if got != tt.keepWaiting {
			t.Errorf("%s: got %v, want %v",
				tt.name, got, tt.keepWaiting)
		}
	}
}
