// internal/installer/preflight_test.go

package installer

import (
	"errors"
	"strings"
	"testing"
)

// ── checkOSRelease ───────────────────────────────────────

func TestCheckOSReleaseAccepts(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"debian 13 quoted",
			"PRETTY_NAME=\"Debian GNU/Linux 13 (trixie)\"\n" +
				"NAME=\"Debian GNU/Linux\"\n" +
				"VERSION_ID=\"13\"\n" +
				"VERSION=\"13 (trixie)\"\n" +
				"VERSION_CODENAME=trixie\n" +
				"ID=debian\n"},
		{"debian 13 unquoted",
			"ID=debian\nVERSION_ID=13\n"},
		{"leading whitespace tolerated",
			"  ID=debian\n  VERSION_ID=\"13\"\n"},
	}
	for _, tt := range cases {
		if err := checkOSRelease(tt.content); err != nil {
			t.Errorf("%s: unexpected refusal: %v", tt.name, err)
		}
	}
}

func TestCheckOSReleaseRefuses(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantIn  string
	}{
		{"debian 12",
			"ID=debian\nVERSION_ID=\"12\"\n",
			"Debian 13 only"},
		{"debian 14",
			"ID=debian\nVERSION_ID=\"14\"\n",
			"Debian 13 only"},
		{"ubuntu with ID_LIKE=debian",
			"ID=ubuntu\nID_LIKE=debian\nVERSION_ID=\"24.04\"\n",
			"Debian only"},
		{"debian sid, no VERSION_ID",
			"ID=debian\nVERSION_CODENAME=sid\n",
			"Debian 13 only"},
		{"empty content", "", "Debian only"},
	}
	for _, tt := range cases {
		err := checkOSRelease(tt.content)
		if err == nil {
			t.Errorf("%s: accepted, want refusal", tt.name)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantIn) {
			t.Errorf("%s: error %q does not mention %q",
				tt.name, err, tt.wantIn)
		}
	}
}

// The old checkOS accepted 13-or-newer via substring matching;
// this guards the two deliberate tightenings (exactly 13, and
// line-anchored ID parsing) against regression.
func TestCheckOSReleaseExactlyThirteen(t *testing.T) {
	for _, ver := range []string{"14", "15", "12", "11"} {
		content := "ID=debian\nVERSION_ID=\"" + ver + "\"\n"
		if err := checkOSRelease(content); err == nil {
			t.Errorf("VERSION_ID=%s accepted, want refusal", ver)
		}
	}
}

// ── sudoersFileIncluded ──────────────────────────────────

func TestSudoersFileIncluded(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"ripsline", true},
		{"README", true},
		{"99-custom", true},
		{"backup.bak", false},    // contains '.'
		{"50-cloud.conf", false}, // contains '.'
		{"ripsline~", false},     // editor backup
	}
	for _, tt := range cases {
		if got := sudoersFileIncluded(tt.name); got != tt.want {
			t.Errorf("sudoersFileIncluded(%q) = %v, want %v",
				tt.name, got, tt.want)
		}
	}
}

// ── sudoersEnablesIOLogging ──────────────────────────────

func TestSudoersIOLoggingDetected(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"plain log_input", "Defaults log_input\n"},
		{"plain log_output", "Defaults log_output\n"},
		{"user-scoped", "Defaults:ripsline log_input\n"},
		{"comma list", "Defaults env_reset, log_input\n"},
		{"double negation enables", "Defaults !!log_input\n"},
		{"tab separated", "Defaults\tlog_output\n"},
		{"line continuation",
			"Defaults env_reset, \\\n    log_input\n"},
		{"LOG_INPUT tag",
			"ripsline ALL=(ALL) LOG_INPUT: /usr/bin/foo\n"},
		{"LOG_OUTPUT tag",
			"ripsline ALL=(ALL) NOPASSWD: LOG_OUTPUT: ALL\n"},
		{"buried among safe lines",
			"# comment\nDefaults env_reset\n" +
				"ripsline ALL=(ALL) NOPASSWD:ALL\n" +
				"Defaults log_input\n"},
	}
	for _, tt := range cases {
		if _, found := sudoersEnablesIOLogging(tt.content); !found {
			t.Errorf("%s: not detected", tt.name)
		}
	}
}

func TestSudoersIOLoggingClean(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"bootstrap rule only",
			"ripsline ALL=(ALL) NOPASSWD:ALL\n"},
		{"stock debian",
			"Defaults env_reset\n" +
				"Defaults mail_badpass\n" +
				"Defaults use_pty\n" + // no disk sink by itself
				"root ALL=(ALL:ALL) ALL\n" +
				"@includedir /etc/sudoers.d\n"},
		{"commented out", "#Defaults log_input\n"},
		{"commented with space", "# Defaults log_input\n"},
		{"explicitly negated", "Defaults !log_input\n"},
		{"negated both",
			"Defaults !log_input\nDefaults !log_output\n"},
		{"iolog_dir alone does not enable",
			"Defaults iolog_dir=/var/log/sudo-io\n"},
		{"NOLOG tags disable",
			"ripsline ALL=(ALL) NOLOG_INPUT: NOLOG_OUTPUT: ALL\n"},
		{"similar option names",
			"Defaults log_year\nDefaults logfile=/var/log/sudo\n"},
	}
	for _, tt := range cases {
		if d, found := sudoersEnablesIOLogging(tt.content); found {
			t.Errorf("%s: false positive on %q", tt.name, d)
		}
	}
}

// ── dpkgAuditVerdict ─────────────────────────────────────

func TestDpkgAuditVerdict(t *testing.T) {
	if err := dpkgAuditVerdict("", nil); err != nil {
		t.Errorf("clean audit refused: %v", err)
	}
	if err := dpkgAuditVerdict("  \n ", nil); err != nil {
		t.Errorf("whitespace-only output refused: %v", err)
	}
	if err := dpkgAuditVerdict(
		"The following packages are only half configured:\n"+
			" somepkg\n", nil); err == nil {
		t.Error("broken-state output accepted")
	}
	if err := dpkgAuditVerdict("", errors.New("exit status 2")); err == nil {
		t.Error("nonzero exit accepted")
	}
}

// ── ufwCandidateVerdict ──────────────────────────────────

func TestUfwCandidateVerdict(t *testing.T) {
	good := "ufw:\n" +
		"  Installed: (none)\n" +
		"  Candidate: 0.36.2-9\n" +
		"  Version table:\n" +
		"     0.36.2-9 500\n"
	if err := ufwCandidateVerdict(good); err != nil {
		t.Errorf("real candidate refused: %v", err)
	}

	none := "ufw:\n  Installed: (none)\n  Candidate: (none)\n"
	if err := ufwCandidateVerdict(none); err == nil {
		t.Error("Candidate (none) accepted")
	}

	if err := ufwCandidateVerdict(""); err == nil {
		t.Error("empty output accepted")
	}
}

// ── report formatting ────────────────────────────────────

func TestFormatPreflightReportAllPass(t *testing.T) {
	out := FormatPreflightReport([]PreflightResult{
		{Name: "check one", Err: nil},
		{Name: "check two", Err: nil},
	})
	if !strings.Contains(out, "[ OK ] check one") ||
		!strings.Contains(out, "[ OK ] check two") {
		t.Errorf("missing OK lines:\n%s", out)
	}
	if strings.Contains(out, "Refusing to install") {
		t.Errorf("refusal line present with zero failures:\n%s", out)
	}
}

func TestFormatPreflightReportWithFailure(t *testing.T) {
	out := FormatPreflightReport([]PreflightResult{
		{Name: "check one", Err: nil},
		{Name: "check two", Err: errors.New(
			"something specific went wrong and here is a " +
				"longer explanation that should wrap across lines")},
	})
	if !strings.Contains(out, "[FAIL] check two") {
		t.Errorf("missing FAIL line:\n%s", out)
	}
	if !strings.Contains(out, "something specific went wrong") {
		t.Errorf("missing failure reason:\n%s", out)
	}
	if !strings.Contains(out,
		"Nothing on this system has been changed") {
		t.Errorf("missing untouched-box line:\n%s", out)
	}
}

// ── wrapText ─────────────────────────────────────────────

func TestWrapText(t *testing.T) {
	lines := wrapText("aa bb cc dd", 5)
	want := []string{"aa bb", "cc dd"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines %v, want %v", len(lines), lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, lines[i], want[i])
		}
	}
	if got := wrapText("", 10); got != nil {
		t.Errorf("empty input: got %v, want nil", got)
	}
	// A word longer than the width gets its own unbroken line.
	long := wrapText("short averyverylongword end", 6)
	found := false
	for _, l := range long {
		if l == "averyverylongword" {
			found = true
		}
	}
	if !found {
		t.Errorf("long word split or lost: %v", long)
	}
}

// ── check names are stable report labels ─────────────────

func TestPreflightCheckNamesNonEmpty(t *testing.T) {
	for _, n := range []string{
		checkNameDebian, checkNameSudo, checkNameTorsocks,
		checkNameIOLogging, checkNameDpkg, checkNameUfw,
	} {
		if strings.TrimSpace(n) == "" {
			t.Error("empty check name")
		}
	}
}
