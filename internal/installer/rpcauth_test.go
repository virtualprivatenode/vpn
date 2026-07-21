// internal/installer/rpcauth_test.go

package installer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
)

// The rpcauth line must be exactly what bitcoind's reference
// generator produces: user:salt$hmac, 32-hex-char salt, 64-hex
// HMAC keyed by the hex salt STRING over the password bytes.
// The recomputation here is the cross-check — a keying mistake
// (raw salt bytes, or salt/password swapped) yields a
// well-formed line that never authenticates, which no format
// check alone would catch.
func TestGenerateRPCAuth(t *testing.T) {
	line, password, err := generateRPCAuth("vpn")
	if err != nil {
		t.Fatal(err)
	}

	shape := regexp.MustCompile(
		`^rpcauth=vpn:([0-9a-f]{32})\$([0-9a-f]{64})$`)
	m := shape.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("line %q does not match the rpcauth shape", line)
	}
	saltHex, hmacHex := m[1], m[2]

	mac := hmac.New(sha256.New, []byte(saltHex))
	mac.Write([]byte(password))
	if want := hex.EncodeToString(mac.Sum(nil)); want != hmacHex {
		t.Errorf("HMAC mismatch: line %s, recomputed %s",
			hmacHex, want)
	}

	// token_urlsafe(32): 43 chars, URL-safe alphabet, no
	// padding.
	if len(password) != 43 {
		t.Errorf("password length %d, want 43", len(password))
	}
	if strings.ContainsAny(password, "+/=\n") {
		t.Errorf("password %q not URL-safe/unpadded", password)
	}

	// Two generations must not repeat.
	line2, password2, err := generateRPCAuth("vpn")
	if err != nil {
		t.Fatal(err)
	}
	if line == line2 || password == password2 {
		t.Error("two generations produced identical output")
	}
}
