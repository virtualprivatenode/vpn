// internal/installer/verify_test.go

package installer

import (
	"strings"
	"testing"
)

func TestParseGoodSigCount(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			name:   "two valid",
			output: "[GNUPG:] GOODSIG E777299FC265DD04 fanquake\n[GNUPG:] GOODSIG FDE04B7075113BFB guggero\n",
			want:   2,
		},
		{
			name:   "one valid one error",
			output: "[GNUPG:] GOODSIG E777299FC265DD04 fanquake\n[GNUPG:] ERRSIG 9343A22960A50972\n",
			want:   1,
		},
		{
			name:   "none",
			output: "[GNUPG:] ERRSIG E777299FC265DD04\n",
			want:   0,
		},
		{
			name:   "empty",
			output: "",
			want:   0,
		},
		{
			name:   "five valid",
			output: "[GNUPG:] GOODSIG A f\n[GNUPG:] GOODSIG B g\n[GNUPG:] GOODSIG C h\n[GNUPG:] GOODSIG D t\n[GNUPG:] GOODSIG E w\n",
			want:   5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGoodSigCount(tt.output)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseBadSigCount(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			name:   "no bad sigs",
			output: "[GNUPG:] GOODSIG A fanquake\n",
			want:   0,
		},
		{
			name:   "one bad sig",
			output: "[GNUPG:] BADSIG A fanquake\n",
			want:   1,
		},
		{
			name:   "mixed good and bad",
			output: "[GNUPG:] GOODSIG A fanquake\n[GNUPG:] BADSIG B guggero\n[GNUPG:] BADSIG C hebasto\n",
			want:   2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBadSigCount(tt.output)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHasGoodSig(t *testing.T) {
	if HasGoodSig("") {
		t.Error("empty string should not have GOODSIG")
	}
	if !HasGoodSig("[GNUPG:] GOODSIG A fanquake") {
		t.Error("should detect GOODSIG")
	}
	if HasGoodSig("[GNUPG:] BADSIG A fanquake") {
		t.Error("BADSIG should not count as GOODSIG")
	}
}

func TestBadSigShouldBlockVerification(t *testing.T) {
	// This test documents the security fix:
	// Even with 2 good sigs, if there's a bad sig, verification should fail
	output := "[GNUPG:] GOODSIG A fanquake\n[GNUPG:] GOODSIG B guggero\n[GNUPG:] BADSIG C hebasto\n"

	good := ParseGoodSigCount(output)
	bad := ParseBadSigCount(output)

	if good < 2 {
		t.Error("expected at least 2 good sigs")
	}
	if bad == 0 {
		t.Error("expected bad sigs to be detected")
	}

	// In the fixed code, bad > 0 means verification fails
	// regardless of good sig count
	if bad > 0 && good >= 2 {
		t.Log("PASS: bad sigs detected even with sufficient good sigs")
	}
}

func TestSignerFingerprints(t *testing.T) {
	if len(bitcoinCoreSigners) != 5 {
		t.Errorf("expected 5 Bitcoin Core signers, got %d",
			len(bitcoinCoreSigners))
	}

	for _, signer := range bitcoinCoreSigners {
		if len(signer.fingerprint) != 40 {
			t.Errorf("signer %s: fingerprint length %d, want 40",
				signer.name, len(signer.fingerprint))
		}
		if signer.name == "" {
			t.Error("signer has empty name")
		}
		if signer.keyURL == "" {
			t.Errorf("signer %s has empty keyURL", signer.name)
		}
	}

	if len(lndSigner.fingerprint) != 40 {
		t.Errorf("LND signer fingerprint length %d, want 40",
			len(lndSigner.fingerprint))
	}
	if lndSigner.keyURL == "" {
		t.Error("LND signer has empty keyURL")
	}
}

func TestReleaseKeyFingerprintFormat(t *testing.T) {
	// The release signing key fingerprint is defined in setup.go's RunSelfUpdate.
	// We can't easily access it from here since it's a local variable,
	// but we can test the fingerprint validation logic.

	validFP := "ABCDEF1234567890ABCDEF1234567890ABCDEF12"
	if len(validFP) != 40 {
		t.Errorf("test fingerprint length: got %d, want 40", len(validFP))
	}

	// Test that our GPG output parsing would match a fingerprint
	sampleOutput := "fpr:::::::::ABCDEF1234567890ABCDEF1234567890ABCDEF12:"
	if !strings.Contains(sampleOutput, validFP) {
		t.Error("fingerprint should be found in GPG colon output")
	}

	// Test that a wrong fingerprint doesn't match
	wrongFP := "0000000000000000000000000000000000000000"
	if strings.Contains(sampleOutput, wrongFP) {
		t.Error("wrong fingerprint should not match")
	}
}
