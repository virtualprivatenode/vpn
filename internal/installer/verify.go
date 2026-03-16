// internal/installer/verify.go

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/system"
)

// ── Trusted signing keys ─────────────────────────────────

var bitcoinCoreSigners = []struct {
	name        string
	fingerprint string
	keyURL      string
}{
	{
		name:        "fanquake",
		fingerprint: "E777299FC265DD04793070EB944D35F9AC3DB76A",
		keyURL:      "https://raw.githubusercontent.com/bitcoin-core/guix.sigs/main/builder-keys/fanquake.gpg",
	},
	{
		name:        "guggero",
		fingerprint: "FDE04B7075113BFB085020B57BBD8D4D95DB9F03",
		keyURL:      "https://raw.githubusercontent.com/bitcoin-core/guix.sigs/main/builder-keys/guggero.gpg",
	},
	{
		name:        "hebasto",
		fingerprint: "CBE89ED88EE8525FD8D79F1EDB56ADFD8B5EF498",
		keyURL:      "https://raw.githubusercontent.com/bitcoin-core/guix.sigs/main/builder-keys/hebasto.gpg",
	},
	{
		name:        "theStack",
		fingerprint: "9343A22960A50972CC1EFD7DB3B5CB8DB648B27F",
		keyURL:      "https://raw.githubusercontent.com/bitcoin-core/guix.sigs/main/builder-keys/theStack.gpg",
	},
	{
		name:        "willcl-ark",
		fingerprint: "A0083660F235A27000CD3C81CE6EC49945C17EA6",
		keyURL:      "https://raw.githubusercontent.com/bitcoin-core/guix.sigs/main/builder-keys/willcl-ark.gpg",
	},
}

var lndSigner = struct {
	name        string
	fingerprint string
	keyURL      string
}{
	name:        "roasbeef",
	fingerprint: "296212681AADF05656A2CDEE90525F7DEEE0AD86",
	keyURL:      "https://raw.githubusercontent.com/lightningnetwork/lnd/master/scripts/keys/roasbeef.asc",
}

// ── GPG setup ────────────────────────────────────────────

func ensureGPG() error {
	if _, err := exec.LookPath("gpg"); err == nil {
		return nil
	}
	return system.SudoRun("apt-get", "install", "-y", "-qq", "gnupg")
}

// ── Key import ───────────────────────────────────────────

func importBitcoinCoreKeys() error {
	logger.Verify("--- Bitcoin Core key import ---")
	imported := 0
	for _, signer := range bitcoinCoreSigners {
		keyFile := fmt.Sprintf("/tmp/btc-key-%s.gpg", signer.name)
		if err := system.DownloadRequireTor(signer.keyURL, keyFile); err != nil {
			logger.Verify("SKIP %s: download failed: %v", signer.name, err)
			continue
		}

		system.RunCombinedOutput("gpg", "--batch", "--import", keyFile)
		os.Remove(keyFile)

		if gpgHasFingerprint(signer.fingerprint) {
			imported++
			logger.Verify("OK %s: imported (fingerprint %s)", signer.name, signer.fingerprint)
		} else {
			logger.Verify("SKIP %s: fingerprint not found after import", signer.name)
		}
	}

	logger.Verify("Bitcoin Core keys imported: %d/%d", imported, len(bitcoinCoreSigners))
	if imported == 0 {
		logger.Verify("FAIL: no Bitcoin Core signing keys imported")
		return fmt.Errorf("could not import any Bitcoin Core signing keys")
	}
	return nil
}

func importLNDKey() error {
	logger.Verify("--- LND key import ---")
	keyFile := "/tmp/lnd-key-roasbeef.asc"
	if err := system.DownloadRequireTor(lndSigner.keyURL, keyFile); err != nil {
		logger.Verify("FAIL: download LND signing key: %v", err)
		return fmt.Errorf("download LND signing key: %w", err)
	}
	defer os.Remove(keyFile)

	output, err := system.RunCombinedOutput("gpg", "--batch", "--import", keyFile)
	if err != nil {
		logger.Verify("FAIL: import LND key: %s", output)
		return fmt.Errorf("import LND key: %w: %s", err, output)
	}

	if !gpgHasFingerprint(lndSigner.fingerprint) {
		logger.Verify("FAIL: LND key fingerprint mismatch (expected %s)", lndSigner.fingerprint)
		return fmt.Errorf("LND key fingerprint mismatch")
	}

	logger.Verify("OK roasbeef: imported (fingerprint %s)", lndSigner.fingerprint)
	return nil
}

// ── Signature verification ───────────────────────────────

func verifyBitcoinCoreSigs(minValid int) error {
	logger.Verify("--- Bitcoin Core signature verification ---")
	sumsFile := "/tmp/SHA256SUMS"
	sigFile := "/tmp/SHA256SUMS.asc"

	if _, err := os.Stat(sumsFile); err != nil {
		logger.Verify("FAIL: SHA256SUMS not found")
		return fmt.Errorf("SHA256SUMS not found")
	}
	if _, err := os.Stat(sigFile); err != nil {
		logger.Verify("FAIL: SHA256SUMS.asc not found")
		return fmt.Errorf("SHA256SUMS.asc not found")
	}

	outputStr, _ := system.RunCombinedOutput("gpg", "--batch", "--verify",
		"--status-fd", "1", sigFile, sumsFile)

	validCount := ParseGoodSigCount(outputStr)
	badCount := ParseBadSigCount(outputStr)

	for _, line := range strings.Split(outputStr, "\n") {
		if strings.Contains(line, "GOODSIG") {
			logger.Verify("GOODSIG: %s", strings.TrimSpace(line))
		}
		if strings.Contains(line, "BADSIG") {
			logger.Verify("BADSIG: %s", strings.TrimSpace(line))
		}
	}

	logger.Verify("Bitcoin Core valid signatures: %d/%d required (bad: %d)",
		validCount, minValid, badCount)

	if badCount > 0 {
		logger.Verify("FAIL: %d bad signatures detected", badCount)
		return fmt.Errorf("bad signatures detected: %d", badCount)
	}

	if validCount < minValid {
		logger.Verify("FAIL: insufficient valid signatures: got %d, need %d",
			validCount, minValid)
		return fmt.Errorf(
			"insufficient valid signatures: got %d, need %d",
			validCount, minValid)
	}

	logger.Verify("OK Bitcoin Core: %d valid signatures", validCount)
	return nil
}

func verifyLNDSig(version string) error {
	logger.Verify("--- LND signature verification ---")
	manifestFile := "/tmp/manifest.txt"
	sigFile := fmt.Sprintf("/tmp/manifest-roasbeef-v%s.sig", version)

	if _, err := os.Stat(manifestFile); err != nil {
		logger.Verify("FAIL: LND manifest not found")
		return fmt.Errorf("LND manifest not found at %s", manifestFile)
	}

	sigURL := fmt.Sprintf(
		"https://github.com/lightningnetwork/lnd/releases/download/v%s/manifest-roasbeef-v%s.sig",
		version, version)
	if err := system.DownloadRequireTor(sigURL, sigFile); err != nil {
		logger.Verify("FAIL: download LND signature: %v", err)
		return fmt.Errorf("download LND signature: %w", err)
	}
	defer os.Remove(sigFile)

	outputStr, _ := system.RunCombinedOutput("gpg", "--batch", "--verify",
		"--status-fd", "1", sigFile, manifestFile)

	if !strings.Contains(outputStr, "GOODSIG") {
		logger.Verify("FAIL: LND signature invalid: %s", outputStr)
		return fmt.Errorf("LND signature verification failed")
	}

	for _, line := range strings.Split(outputStr, "\n") {
		if strings.Contains(line, "GOODSIG") {
			logger.Verify("GOODSIG: %s", strings.TrimSpace(line))
		}
	}

	logger.Verify("OK LND: signature valid")
	return nil
}

// ── Checksum verification ────────────────────────────────

func verifyBitcoin() error {
	logger.Verify("--- Bitcoin Core checksum verification ---")
	// exec.Command used directly because sha256sum --check needs
	// working directory set to /tmp where the tarball was downloaded.
	cmd := exec.Command("sha256sum", "--ignore-missing", "--check", "SHA256SUMS")
	cmd.Dir = "/tmp"
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Verify("FAIL: Bitcoin Core checksum: %s", string(output))
		return fmt.Errorf("checksum failed: %w: %s", err, output)
	}
	logger.Verify("OK Bitcoin Core checksum: %s", strings.TrimSpace(string(output)))
	return nil
}

func verifyLND() error {
	logger.Verify("--- LND checksum verification ---")
	if _, err := os.Stat("/tmp/manifest.txt"); err != nil {
		logger.Verify("FAIL: LND manifest not found")
		return fmt.Errorf("LND manifest not found")
	}
	// exec.Command used directly because sha256sum --check needs
	// working directory set to /tmp where the tarball was downloaded.
	cmd := exec.Command("sha256sum", "--ignore-missing", "--check", "manifest.txt")
	cmd.Dir = "/tmp"
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Verify("FAIL: LND checksum: %s", string(output))
		return fmt.Errorf("checksum failed: %w: %s", err, output)
	}
	logger.Verify("OK LND checksum: %s", strings.TrimSpace(string(output)))
	return nil
}

// ── Helpers ──────────────────────────────────────────────

func gpgHasFingerprint(fingerprint string) bool {
	output, err := system.RunCombinedOutput("gpg", "--batch", "--list-keys",
		"--with-colons", fingerprint)
	if err != nil {
		return false
	}
	return strings.Contains(output, fingerprint)
}

// ParseGoodSigCount counts GOODSIG lines in GPG status output.
func ParseGoodSigCount(output string) int {
	return strings.Count(output, "GOODSIG")
}

// ParseBadSigCount counts BADSIG lines in GPG status output.
func ParseBadSigCount(output string) int {
	return strings.Count(output, "BADSIG")
}

// HasGoodSig checks if GPG output contains at least one GOODSIG.
func HasGoodSig(output string) bool {
	return strings.Contains(output, "GOODSIG")
}
