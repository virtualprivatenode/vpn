// internal/installer/sshd_test.go

package installer

import (
	"strings"
	"testing"

	"github.com/ripsline/virtual-private-node/internal/config"
)

// realistic excerpt of sshd -T output: lowercase keywords,
// one "keyword value" pair per line, target buried mid-list.
const sshdTFixture = `port 22
addressfamily any
listenaddress [::]:22
listenaddress 0.0.0.0:22
usepam yes
pubkeyauthentication yes
passwordauthentication no
kbdinteractiveauthentication no
permitrootlogin no
x11forwarding no
`

func TestParsePasswordAuth(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr bool
	}{
		{
			name:  "realistic -T output, no",
			input: sshdTFixture,
			want:  false,
		},
		{
			name: "realistic -T output, yes",
			input: strings.Replace(sshdTFixture,
				"passwordauthentication no",
				"passwordauthentication yes", 1),
			want: true,
		},
		{
			name:  "single line yes",
			input: "passwordauthentication yes\n",
			want:  true,
		},
		{
			name:  "single line no",
			input: "passwordauthentication no\n",
			want:  false,
		},
		{
			// sshd normalizes to lowercase; the parser
			// tolerates case anyway.
			name:  "mixed case tolerated",
			input: "PasswordAuthentication Yes\n",
			want:  true,
		},
		{
			// Unknown value is not "yes" → treated as
			// disabled (fail-safe direction).
			name:  "unexpected value treated as no",
			input: "passwordauthentication maybe\n",
			want:  false,
		},
		{
			name:    "directive absent errors",
			input:   "port 22\nusepam yes\n",
			wantErr: true,
		},
		{
			name:    "empty input errors",
			input:   "",
			wantErr: true,
		},
		{
			// Substring must not match: only an exact
			// two-field keyword line counts.
			name: "no substring false positive",
			input: "somepasswordauthentication yes\n" +
				"passwordauthenticationx yes\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePasswordAuth(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got "+
						"result %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v",
					got, tt.want)
			}
		})
	}
}

func TestBuildSSHHardeningConfig(t *testing.T) {
	cfg := config.Default()

	cfg.SSHPasswordAuthDisabled = false
	enabled := BuildSSHHardeningConfig(cfg)
	if !strings.Contains(enabled,
		"PasswordAuthentication yes\n") {
		t.Fatalf("enabled config missing "+
			"'PasswordAuthentication yes':\n%s", enabled)
	}

	cfg.SSHPasswordAuthDisabled = true
	disabled := BuildSSHHardeningConfig(cfg)
	if !strings.Contains(disabled,
		"PasswordAuthentication no\n") {
		t.Fatalf("disabled config missing "+
			"'PasswordAuthentication no':\n%s", disabled)
	}

	// Static hardening lines present in both states.
	for _, line := range []string{
		"PermitRootLogin no",
		"PubkeyAuthentication yes",
		"X11Forwarding no",
	} {
		if !strings.Contains(enabled, line) ||
			!strings.Contains(disabled, line) {
			t.Fatalf("static line %q missing", line)
		}
	}

	// Exactly one PasswordAuthentication directive —
	// a duplicate would silently lose first-match-wins.
	if n := strings.Count(disabled,
		"PasswordAuthentication"); n != 1 {
		t.Fatalf("want exactly 1 PasswordAuthentication "+
			"directive, got %d", n)
	}

	if !strings.HasSuffix(disabled, "\n") {
		t.Fatal("config must end with a newline")
	}
}
