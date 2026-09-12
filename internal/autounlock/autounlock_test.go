package autounlock

import (
	"fmt"
	"strings"
	"testing"
)

func TestWalletPasswordContract(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		valid      bool
	}{
		{"empty", "", false},
		{"existing short password", "short", true},
		{"spaces preserved", "  wallet password  ", true},
		{"byte boundary", strings.Repeat("x", 512), true},
		{"too many bytes", strings.Repeat("x", 513), false},
		{"Unicode byte boundary", strings.Repeat("é", 256), true},
		{"Unicode over byte boundary", strings.Repeat("é", 257), false},
		{"newline", "wallet\npassword", false},
		{"carriage return", "wallet\rpassword", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			password, err := NewPassword(tc.text)
			if (err == nil) != tc.valid || (tc.valid && password.Text() != tc.text) {
				t.Fatal("wallet password contract changed")
			}
			if tc.valid && (fmt.Sprint(password) != "[redacted]" || fmt.Sprintf("%#v", password) != "[redacted]") {
				t.Fatal("formatted password exposes secret")
			}
		})
	}
}
