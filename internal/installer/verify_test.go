// internal/installer/verify_test.go

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ── Test helpers ────────────────────────────────────────

func gpgAvailable() bool {
	_, err := exec.LookPath("gpg")
	return err == nil
}

// testGenKey generates a GPG key in the given homedir and
// returns the primary-key fingerprint. If addSigningSubkey
// is true, the primary key has certify-only usage and a
// separate signing subkey is added.
func testGenKey(
	t *testing.T, gpgHome, name string,
	addSigningSubkey bool,
) string {
	t.Helper()

	var params string
	if addSigningSubkey {
		params = fmt.Sprintf(`%%no-protection
Key-Type: RSA
Key-Length: 2048
Key-Usage: cert
Subkey-Type: RSA
Subkey-Length: 2048
Subkey-Usage: sign
Name-Real: %s
Name-Email: %s@test.local
%%commit
`, name, name)
	} else {
		params = fmt.Sprintf(`%%no-protection
Key-Type: RSA
Key-Length: 2048
Key-Usage: sign
Name-Real: %s
Name-Email: %s@test.local
%%commit
`, name, name)
	}

	paramFile := filepath.Join(gpgHome, "params-"+name)
	if err := os.WriteFile(
		paramFile, []byte(params), 0600); err != nil {
		t.Fatalf("write key params for %s: %v", name, err)
	}

	output, err := exec.Command(
		"gpg", "--homedir", gpgHome,
		"--batch", "--gen-key", paramFile,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("generate key %s: %v\n%s", name, err, output)
	}

	return testGetFingerprint(t, gpgHome, name)
}

// testGetFingerprint extracts the primary-key fingerprint for
// the named key from the given GPG homedir.
func testGetFingerprint(
	t *testing.T, gpgHome, name string,
) string {
	t.Helper()

	output, err := exec.Command(
		"gpg", "--homedir", gpgHome,
		"--batch", "--with-colons",
		"--list-keys", name,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("list key %s: %v\n%s", name, err, output)
	}

	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "fpr:") {
			fields := strings.Split(line, ":")
			if len(fields) >= 10 && fields[9] != "" {
				return fields[9]
			}
		}
	}
	t.Fatalf("fingerprint not found for %s in:\n%s",
		name, output)
	return ""
}

// testExportKey exports the public key for the given fingerprint
// to a file.
func testExportKey(
	t *testing.T, gpgHome, fingerprint, destFile string,
) {
	t.Helper()

	output, err := exec.Command(
		"gpg", "--homedir", gpgHome,
		"--batch", "--armor",
		"--export", fingerprint,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("export key %s: %v", fingerprint, err)
	}
	if err := os.WriteFile(destFile, output, 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
}

// testSign creates an armored detached signature of dataFile
// using the key identified by fingerprint.
func testSign(
	t *testing.T, gpgHome, fingerprint,
	dataFile, sigFile string,
) {
	t.Helper()

	output, err := exec.Command(
		"gpg", "--homedir", gpgHome,
		"--batch", "--armor",
		"--local-user", fingerprint,
		"--detach-sign",
		"--output", sigFile,
		dataFile,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("sign with %s: %v\n%s",
			fingerprint, err, output)
	}
}

// ── Hermetic test suite ─────────────────────────────────

func TestVerifyIsolated(t *testing.T) {
	if !gpgAvailable() {
		t.Skip("gpg not available — skipping")
	}

	// Set up a key-generation homedir with two distinct keys
	// and one subkey-only key.
	genHome, err := os.MkdirTemp("", "rlvpn-test-gen-")
	if err != nil {
		t.Fatalf("create gen home: %v", err)
	}
	defer os.RemoveAll(genHome)

	fpAlpha := testGenKey(t, genHome, "Alpha", false)
	fpBeta := testGenKey(t, genHome, "Beta", false)
	fpSubkey := testGenKey(t, genHome, "SubkeySigner", true)

	// Create test data file.
	dataDir, err := os.MkdirTemp("", "rlvpn-test-data-")
	if err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	defer os.RemoveAll(dataDir)

	dataFile := filepath.Join(dataDir, "testdata.txt")
	if err := os.WriteFile(
		dataFile, []byte("test data content\n"), 0600); err != nil {
		t.Fatalf("write test data: %v", err)
	}

	// Create a tampered data file (for case 2).
	tamperedFile := filepath.Join(dataDir, "tampered.txt")
	if err := os.WriteFile(
		tamperedFile,
		[]byte("tampered data content\n"), 0600); err != nil {
		t.Fatalf("write tampered data: %v", err)
	}

	// Export all keys to files (for verifyIsolated to import).
	keyAlpha := filepath.Join(dataDir, "alpha.asc")
	keyBeta := filepath.Join(dataDir, "beta.asc")
	keySubkey := filepath.Join(dataDir, "subkey.asc")
	testExportKey(t, genHome, fpAlpha, keyAlpha)
	testExportKey(t, genHome, fpBeta, keyBeta)
	testExportKey(t, genHome, fpSubkey, keySubkey)

	// Sign test data with each key.
	sigAlpha := filepath.Join(dataDir, "sig-alpha.asc")
	sigBeta := filepath.Join(dataDir, "sig-beta.asc")
	sigSubkey := filepath.Join(dataDir, "sig-subkey.asc")
	testSign(t, genHome, fpAlpha, dataFile, sigAlpha)
	testSign(t, genHome, fpBeta, dataFile, sigBeta)
	testSign(t, genHome, fpSubkey, dataFile, sigSubkey)

	// Create combined signatures for multi-signer tests.
	sigAlphaData, _ := os.ReadFile(sigAlpha)
	sigBetaData, _ := os.ReadFile(sigBeta)

	// Alpha + Beta combined (for case 5).
	sigAlphaBeta := filepath.Join(dataDir, "sig-alpha-beta.asc")
	if err := os.WriteFile(
		sigAlphaBeta,
		append(sigAlphaData, sigBetaData...),
		0600); err != nil {
		t.Fatalf("write combined sig: %v", err)
	}

	// Alpha + Alpha combined (for case 4).
	sigAlphaAlpha := filepath.Join(dataDir,
		"sig-alpha-alpha.asc")
	if err := os.WriteFile(
		sigAlphaAlpha,
		append(sigAlphaData, sigAlphaData...),
		0600); err != nil {
		t.Fatalf("write double sig: %v", err)
	}

	// ── Case 1: Good sig from a pinned key → accept ────
	t.Run("pinned_key_accepts", func(t *testing.T) {
		pinned := map[string]bool{fpAlpha: true}
		distinct, bad, err := verifyIsolated(
			[]string{keyAlpha}, sigAlpha, dataFile, pinned)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bad {
			t.Fatal("unexpected BADSIG")
		}
		if distinct != 1 {
			t.Fatalf("distinct = %d, want 1", distinct)
		}
	})

	// ── Case 2: Tampered file → BADSIG → reject ────────
	t.Run("tampered_file_badsig", func(t *testing.T) {
		pinned := map[string]bool{fpAlpha: true}
		_, bad, err := verifyIsolated(
			[]string{keyAlpha}, sigAlpha, tamperedFile, pinned)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bad {
			t.Fatal("expected BADSIG for tampered file")
		}
	})

	// ── Case 3: Good sig from UNPINNED key → reject ────
	t.Run("unpinned_key_rejects", func(t *testing.T) {
		// Alpha signed, but only Beta is pinned.
		pinned := map[string]bool{fpBeta: true}
		distinct, bad, err := verifyIsolated(
			[]string{keyAlpha}, sigAlpha, dataFile, pinned)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bad {
			t.Fatal("unexpected BADSIG")
		}
		if distinct != 0 {
			t.Fatalf("distinct = %d, want 0 "+
				"(signature valid but not from pinned key)",
				distinct)
		}
	})

	// ── Case 4: Same key twice → counts as 1 ───────────
	//     → fails threshold 2
	t.Run("same_key_twice_counts_once", func(t *testing.T) {
		pinned := map[string]bool{fpAlpha: true}
		distinct, bad, err := verifyIsolated(
			[]string{keyAlpha},
			sigAlphaAlpha, dataFile, pinned)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bad {
			t.Fatal("unexpected BADSIG")
		}
		if distinct != 1 {
			t.Fatalf("distinct = %d, want 1 "+
				"(same key signing twice should count once)",
				distinct)
		}
		// Would fail threshold 2.
		if distinct >= 2 {
			t.Fatal("same key twice should not clear threshold 2")
		}
	})

	// ── Case 5: Two different pinned keys → clears 2 ───
	t.Run("two_pinned_keys_clears_threshold", func(t *testing.T) {
		pinned := map[string]bool{
			fpAlpha: true,
			fpBeta:  true,
		}
		distinct, bad, err := verifyIsolated(
			[]string{keyAlpha, keyBeta},
			sigAlphaBeta, dataFile, pinned)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bad {
			t.Fatal("unexpected BADSIG")
		}
		if distinct != 2 {
			t.Fatalf("distinct = %d, want 2", distinct)
		}
	})

	// ── Case 6: Subkey-signed → matches primary FP ─────
	t.Run("subkey_signed_matches_primary", func(t *testing.T) {
		// fpSubkey is the PRIMARY fingerprint. The signing
		// subkey has a different fingerprint. verifyIsolated
		// must match the VALIDSIG last field (primary), not
		// the first field (subkey).
		pinned := map[string]bool{fpSubkey: true}
		distinct, bad, err := verifyIsolated(
			[]string{keySubkey},
			sigSubkey, dataFile, pinned)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bad {
			t.Fatal("unexpected BADSIG")
		}
		if distinct != 1 {
			t.Fatalf("distinct = %d, want 1 "+
				"(subkey-signed should match primary FP)",
				distinct)
		}
	})
}

// ── Anchor validation ───────────────────────────────────

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

func TestReleaseKeyFingerprint(t *testing.T) {
	if len(rlvpnReleaseFP) != 40 {
		t.Errorf("release key fingerprint length %d, want 40",
			len(rlvpnReleaseFP))
	}

	// Cross-check: must match the value baked into
	// virtual-private-node.sh SIGNING_KEY_FP (line 24).
	expected := "AFA0EBACDC9A4C4AA7B0154AC97CE10F170BA5FE"
	if rlvpnReleaseFP != expected {
		t.Errorf("release FP = %s, want %s",
			rlvpnReleaseFP, expected)
	}
}
