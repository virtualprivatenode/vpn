// internal/installer/userpassword_test.go

package installer

import (
	"fmt"
	"strings"
	"testing"
)

func TestNewLoginPassword(t *testing.T) {
	long := strings.Repeat("a", MinLoginPasswordLen)

	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty", "", true},
		{"one below minimum",
			long[:MinLoginPasswordLen-1], true},
		{"exactly minimum", long, false},
		{"above minimum", long + "extra", false},
		{"embedded newline",
			long + "\n" + long, true},
		{"trailing newline", long + "\n", true},
		{"colon is allowed (only the username " +
			"side of chpasswd's line is delimited)",
			"with:colon:" + long, false},
		{"spaces allowed",
			"correct horse battery staple", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewLoginPassword(tc.in)
			if tc.wantErr && err == nil {
				t.Errorf("NewLoginPassword(%q): "+
					"expected error, got nil", tc.in)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("NewLoginPassword(%q): "+
					"unexpected error: %v", tc.in, err)
			}
		})
	}
}

// The password must never leak through the fmt verbs that
// end up in logs and error chains.
func TestLoginPasswordRedaction(t *testing.T) {
	secret := strings.Repeat("s3cret!!", 4)
	pw, err := NewLoginPassword(secret)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	for _, verb := range []string{
		"%v", "%s", "%#v", "%+v",
	} {
		got := fmt.Sprintf(verb, pw)
		if strings.Contains(got, secret) {
			t.Errorf("fmt %s leaked the password: %q",
				verb, got)
		}
		if got != "[redacted]" {
			t.Errorf("fmt %s: got %q, want %q",
				verb, got, "[redacted]")
		}
	}

	wrapped := fmt.Errorf("op failed: %v", pw)
	if strings.Contains(wrapped.Error(), secret) {
		t.Errorf("error wrapping leaked the password: %q",
			wrapped.Error())
	}
}

// SetUserPassword's own guards fire before any exec, so
// they are testable without sudo/chpasswd present.
func TestSetUserPasswordRejectsBeforeExec(t *testing.T) {
	valid, err := NewLoginPassword(
		strings.Repeat("a", MinLoginPasswordLen))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := SetUserPassword("", valid); err == nil {
		t.Error("empty username: expected error")
	}
	if err := SetUserPassword(
		"bad:user", valid); err == nil {
		t.Error("username with colon: expected error")
	}
	if err := SetUserPassword(
		"bad\nuser", valid); err == nil {
		t.Error("username with newline: expected error")
	}
	if err := SetUserPassword(
		"someuser", LoginPassword{}); err == nil {
		t.Error("zero-value LoginPassword: " +
			"expected error")
	}
}
