package host

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/virtualprivatenode/vpn/internal/sshkeys"
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
		{"realistic no", sshdTFixture, false, false},
		{"realistic yes", strings.Replace(sshdTFixture, "passwordauthentication no", "passwordauthentication yes", 1), true, false},
		{"single line yes", "passwordauthentication yes\n", true, false},
		{"single line no", "passwordauthentication no\n", false, false},
		{"mixed case", "PasswordAuthentication Yes\n", true, false},
		{"unknown value", "passwordauthentication maybe\n", false, true},
		{"absent", "port 22\nusepam yes\n", false, true},
		{"empty", "", false, true},
		{"substring", "somepasswordauthentication yes\npasswordauthenticationx yes\n", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePasswordAuth(tt.input)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("got %v, error %v; want %v, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestBuildSSHHardeningConfig(t *testing.T) {
	for _, setting := range []string{"yes", "no", ""} {
		t.Run("password="+setting, func(t *testing.T) {
			out := buildHardeningDropIn(setting)
			// Unknown observations omit the directive; explicit choices emit it once.
			wantCount := 0
			if setting != "" {
				wantCount = 1
				if !strings.Contains(out, "PasswordAuthentication "+setting+"\n") {
					t.Fatalf("missing requested password setting:\n%s", out)
				}
			}
			if n := strings.Count(out, "PasswordAuthentication"); n != wantCount {
				t.Fatalf("got %d password directives, want %d", n, wantCount)
			}
			for _, line := range []string{
				"PermitRootLogin no", "PubkeyAuthentication yes",
				"ChallengeResponseAuthentication no", "KbdInteractiveAuthentication no",
				"X11Forwarding no",
			} {
				if !strings.Contains(out, line+"\n") {
					t.Errorf("missing hardening directive %q", line)
				}
			}
			if !strings.HasSuffix(out, "\n") {
				t.Error("drop-in must end with a newline")
			}
		})
	}
}

func TestSSHApplyValidatesRestoresAndDoesNotRestartOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name                                                     string
		wantEvents                                               string
		exists                                                   bool
		readErr, writeErr, validationErr, restoreErr, restartErr error
	}{
		{name: "success", wantEvents: "read,write,validate,restart", exists: true},
		{name: "read failure", wantEvents: "read", exists: true, readErr: errors.New("denied")},
		{name: "write failure", wantEvents: "read,write", exists: true, writeErr: errors.New("disk full")},
		{name: "restore previous", wantEvents: "read,write,validate,restore", exists: true, validationErr: errors.New("bad config")},
		{name: "remove new", wantEvents: "read,write,validate,remove", validationErr: errors.New("bad config")},
		{name: "restore failure", wantEvents: "read,write,validate,restore", exists: true, validationErr: errors.New("bad config"), restoreErr: errors.New("denied")},
		{name: "restart failure", wantEvents: "read,write,validate,restart", exists: true, restartErr: errors.New("restart failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			ops := sshdOps{
				read: func() ([]byte, error) {
					events = append(events, "read")
					if tc.readErr != nil {
						return nil, tc.readErr
					}
					if !tc.exists {
						return nil, os.ErrNotExist
					}
					return []byte("previous"), nil
				},
				write: func(data []byte) error {
					if string(data) == "previous" {
						events = append(events, "restore")
						return tc.restoreErr
					}
					events = append(events, "write")
					return tc.writeErr
				},
				remove: func() error { events = append(events, "remove"); return tc.restoreErr },
				validate: func() (string, error) {
					events = append(events, "validate")
					return "config rejected", tc.validationErr
				},
				restart: func() error { events = append(events, "restart"); return tc.restartErr },
			}
			err := applySSHHardening("no", ops)
			if strings.Join(events, ",") != tc.wantEvents {
				t.Fatalf("operations %v, want %s", events, tc.wantEvents)
			}
			if (err != nil) != (tc.name != "success") {
				t.Fatalf("unexpected result %v", err)
			}
			if tc.restoreErr != nil && !strings.Contains(err.Error(), "restoring") {
				t.Fatal("restore failure hidden")
			}
		})
	}
}

func TestSSHDisableGuardPrecedesHostWrites(t *testing.T) {
	dir := t.TempDir()
	store := sshkeys.Store{Path: filepath.Join(dir, "authorized_keys")}
	valid := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEB"
	zeroModulusRSA := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAAA==\n"
	for _, content := range []string{"", "ssh-ed25519 YQ==\n", zeroModulusRSA, "command=\"false\" " + valid + "\n", valid + "\n"} {
		if err := os.WriteFile(store.Path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		applied := false
		err := rebuildSSHConfig(true, store, func(value string) error {
			applied = true
			if value != "no" {
				t.Fatal("wrong desired setting")
			}
			if err := store.WithLock(func() error { t.Fatal("key edit overlapped disabling"); return nil }); err == nil {
				t.Fatal("guard did not hold lock")
			}
			return nil
		}, func() (bool, error) { return false, nil })
		want := content == valid+"\n"
		if applied != want || (err == nil) != want {
			t.Fatalf("content %q applied %v error %v", content, applied, err)
		}
	}
	if err := rebuildSSHConfig(false, store, func(string) error { return nil }, func() (bool, error) { return false, nil }); err == nil {
		t.Fatal("effective mismatch reported success")
	}
	if err := rebuildSSHConfig(false, store, func(string) error { return nil }, func() (bool, error) { return false, errors.New("observation failed") }); err == nil {
		t.Fatal("failed observation reported success")
	}
}
