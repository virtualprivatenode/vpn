// internal/installer/verify.go

// Trust model
//
// This file pins trusted signing-key fingerprints for Bitcoin Core, LND and
// VPN releases. Each workflow obtains verification inputs and enforces its
// signer threshold. artifact.VerifySignature owns isolated GPG verification;
// callers reject bad signatures and insufficient trusted signers.
// Syncthing provisioning keeps its release-specific trust policy in host.

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/artifact"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// ── Trust anchors ───────────────────────────────────────
//
// Each fingerprint was sourced and cross-checked as noted.
// To re-verify: download the key from the listed URL, import
// into an ephemeral keyring, and confirm the fingerprint with
// gpg --with-colons --list-keys.

// vpnReleaseFP is the primary fingerprint of the vpn
// release signing key.
// Source: generated locally; public key hosted at
// keys.openpgp.org. Cross-check: docs/verifying.md publishes the
// same fingerprint used for manual release verification.
const vpnReleaseFP = "AFA0EBACDC9A4C4AA7B0154AC97CE10F170BA5FE"

// bitcoinCoreSigners are the trusted Bitcoin Core builder keys.
// Source: github.com/bitcoin-core/guix.sigs/tree/main/builder-keys
// Cross-check: each key's fingerprint against the .gpg file at
// the listed URL. Verified against Bitcoin Core 29.3 SHA256SUMS.asc
// on a live install (June 4 2026). Primary-key fingerprints, not
// subkeys. Threshold: 2 of 5 distinct signers required.
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
		fingerprint: "F4FC70F07310028424EFC20A8E4256593F177720",
		keyURL:      "https://raw.githubusercontent.com/bitcoin-core/guix.sigs/main/builder-keys/guggero.gpg",
	},
	{
		name:        "hebasto",
		fingerprint: "D1DBF2C4B96F2DEBF4C16654410108112E7EA81F",
		keyURL:      "https://raw.githubusercontent.com/bitcoin-core/guix.sigs/main/builder-keys/hebasto.gpg",
	},
	{
		name:        "theStack",
		fingerprint: "6A8F9C266528E25AEB1D7731C2371D91CB716EA7",
		keyURL:      "https://raw.githubusercontent.com/bitcoin-core/guix.sigs/main/builder-keys/theStack.gpg",
	},
	{
		name:        "willcl-ark",
		fingerprint: "67AA5B46E7AF78053167FE343B8F814A784218F8",
		keyURL:      "https://raw.githubusercontent.com/bitcoin-core/guix.sigs/main/builder-keys/willcl-ark.gpg",
	},
}

// lndSigner is the trusted LND release signer.
// Source: github.com/lightningnetwork/lnd/tree/master/scripts/keys
// Cross-check: roasbeef.asc at the listed URL. Verified against
// the LND v0.21.2-beta manifest signature during release intake
// (August 28, 2026). Primary-key fingerprint — the signing
// subkey (2962...) is owned by this primary.
var lndSigner = struct {
	name        string
	fingerprint string
	keyURL      string
}{
	name:        "roasbeef",
	fingerprint: "A5B61896952D9FDA83BC054CDC42612E89237182",
	keyURL:      "https://raw.githubusercontent.com/lightningnetwork/lnd/master/scripts/keys/roasbeef.asc",
}

// ── GPG setup ────────────────────────────────────────────

func ensureGPG() error {
	if _, err := exec.LookPath("gpg"); err == nil {
		return nil
	}
	return system.SudoRun("apt-get", "install", "-y", "-qq", "gnupg")
}

// ── Bitcoin Core verification ───────────────────────────

func verifyBitcoinCoreSigs(workDir string, minValid int) error {
	logger.Verify("--- Bitcoin Core signature verification ---")

	sumsFile := filepath.Join(workDir, "SHA256SUMS")
	sigFile := filepath.Join(workDir, "SHA256SUMS.asc")

	if _, err := os.Stat(sumsFile); err != nil {
		logger.Verify("FAIL: SHA256SUMS not found")
		return fmt.Errorf("SHA256SUMS not found")
	}
	if _, err := os.Stat(sigFile); err != nil {
		logger.Verify("FAIL: SHA256SUMS.asc not found")
		return fmt.Errorf("SHA256SUMS.asc not found")
	}

	// Pin ALL fingerprints regardless of download success.
	// The pinned set is the trust anchor — it does not change
	// based on which keys we managed to download.
	pinnedFPs := make(map[string]bool)
	for _, signer := range bitcoinCoreSigners {
		pinnedFPs[signer.fingerprint] = true
	}

	// Download signing keys into the pipeline work directory.
	var keyFiles []string
	for _, signer := range bitcoinCoreSigners {
		keyFile := filepath.Join(workDir,
			fmt.Sprintf("btc-key-%s.gpg", signer.name))
		if err := system.DownloadRequireTor(
			signer.keyURL, keyFile); err != nil {
			logger.Verify("SKIP %s: download failed: %v",
				signer.name, err)
			continue
		}
		keyFiles = append(keyFiles, keyFile)
		logger.Verify("OK %s: key downloaded", signer.name)
	}

	if len(keyFiles) == 0 {
		logger.Verify("FAIL: no Bitcoin Core signing keys downloaded")
		return fmt.Errorf(
			"could not download any Bitcoin Core signing keys")
	}

	distinct, hasBadSig, err := artifact.VerifySignature(
		keyFiles, sigFile, sumsFile, pinnedFPs)
	if err != nil {
		return fmt.Errorf(
			"signature verification failed: %w", err)
	}

	if hasBadSig {
		logger.Verify("FAIL: bad signature detected")
		return fmt.Errorf(
			"bad signature detected — verification aborted")
	}

	logger.Verify(
		"Bitcoin Core valid pinned signatures: %d/%d required",
		distinct, minValid)

	if distinct < minValid {
		logger.Verify(
			"FAIL: insufficient valid signatures: got %d, need %d",
			distinct, minValid)
		return fmt.Errorf(
			"insufficient valid signatures: got %d, need %d",
			distinct, minValid)
	}

	logger.Verify("OK Bitcoin Core: %d valid pinned signatures",
		distinct)
	return nil
}

func verifyBitcoin(workDir string) error {
	logger.Verify("--- Bitcoin Core checksum verification ---")
	// exec.Command used directly because sha256sum --check needs
	// working directory set to where the tarball was downloaded.
	cmd := exec.Command("sha256sum",
		"--ignore-missing", "--check", "SHA256SUMS")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Verify("FAIL: Bitcoin Core checksum: %s",
			string(output))
		return fmt.Errorf("checksum failed: %w: %s", err, output)
	}
	logger.Verify("OK Bitcoin Core checksum: %s",
		strings.TrimSpace(string(output)))
	return nil
}

// ── LND verification ────────────────────────────────────

func verifyLNDSig(workDir string, version string) error {
	logger.Verify("--- LND signature verification ---")

	manifestFile := filepath.Join(workDir, "manifest.txt")
	if _, err := os.Stat(manifestFile); err != nil {
		logger.Verify("FAIL: LND manifest not found")
		return fmt.Errorf("LND manifest not found at %s",
			manifestFile)
	}

	// Download signature file.
	sigFile := filepath.Join(workDir,
		fmt.Sprintf("manifest-roasbeef-v%s.sig", version))
	sigURL := fmt.Sprintf(
		"https://github.com/lightningnetwork/lnd/releases/download/v%s/manifest-roasbeef-v%s.sig",
		version, version)
	if err := system.DownloadRequireTor(
		sigURL, sigFile); err != nil {
		logger.Verify("FAIL: download LND signature: %v", err)
		return fmt.Errorf("download LND signature: %w", err)
	}

	// Download signing key.
	keyFile := filepath.Join(workDir, "lnd-key-roasbeef.asc")
	if err := system.DownloadRequireTor(
		lndSigner.keyURL, keyFile); err != nil {
		logger.Verify("FAIL: download LND signing key: %v", err)
		return fmt.Errorf("download LND signing key: %w", err)
	}

	pinnedFPs := map[string]bool{lndSigner.fingerprint: true}

	distinct, hasBadSig, err := artifact.VerifySignature(
		[]string{keyFile}, sigFile, manifestFile, pinnedFPs)
	if err != nil {
		return fmt.Errorf(
			"LND signature verification failed: %w", err)
	}

	if hasBadSig {
		logger.Verify("FAIL: bad LND signature detected")
		return fmt.Errorf(
			"bad LND signature detected — verification aborted")
	}

	if distinct < 1 {
		logger.Verify(
			"FAIL: LND signature not valid against pinned fingerprint")
		return fmt.Errorf("LND signature verification failed")
	}

	logger.Verify(
		"OK LND: signature valid (roasbeef, pinned fingerprint)")
	return nil
}

func verifyLND(workDir string) error {
	logger.Verify("--- LND checksum verification ---")
	manifestFile := filepath.Join(workDir, "manifest.txt")
	if _, err := os.Stat(manifestFile); err != nil {
		logger.Verify("FAIL: LND manifest not found")
		return fmt.Errorf("LND manifest not found")
	}
	// exec.Command used directly because sha256sum --check needs
	// working directory set to where the tarball was downloaded.
	cmd := exec.Command("sha256sum",
		"--ignore-missing", "--check", "manifest.txt")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Verify("FAIL: LND checksum: %s",
			string(output))
		return fmt.Errorf("checksum failed: %w: %s", err, output)
	}
	logger.Verify("OK LND checksum: %s",
		strings.TrimSpace(string(output)))
	return nil
}

// ── Self-update verification ────────────────────────────

func verifySelfUpdate(workDir string) error {
	logger.Verify("--- Self-update signature verification ---")

	sumsFile := filepath.Join(workDir, "SHA256SUMS")
	sigFile := filepath.Join(workDir, "SHA256SUMS.asc")

	if _, err := os.Stat(sumsFile); err != nil {
		logger.Verify("FAIL: SHA256SUMS not found")
		return fmt.Errorf("SHA256SUMS not found")
	}
	if _, err := os.Stat(sigFile); err != nil {
		logger.Verify("FAIL: SHA256SUMS.asc not found")
		return fmt.Errorf("SHA256SUMS.asc not found")
	}

	// Download the release signing key fresh into the work
	// directory. artifact.VerifySignature imports it into an ephemeral
	// GPG home; the shared keyring is never touched.
	keyFile := filepath.Join(workDir, "release-key.asc")
	keyURL := fmt.Sprintf(
		"https://keys.openpgp.org/vks/v1/by-fingerprint/%s",
		vpnReleaseFP)
	if err := system.DownloadRequireTor(
		keyURL, keyFile); err != nil {
		logger.Verify(
			"FAIL: download release signing key: %v", err)
		return fmt.Errorf(
			"download release signing key: %w", err)
	}

	pinnedFPs := map[string]bool{vpnReleaseFP: true}

	distinct, hasBadSig, err := artifact.VerifySignature(
		[]string{keyFile}, sigFile, sumsFile, pinnedFPs)
	if err != nil {
		return fmt.Errorf(
			"signature verification failed: %w", err)
	}

	if hasBadSig {
		logger.Verify("FAIL: bad signature detected")
		return fmt.Errorf(
			"bad signature detected — verification aborted")
	}

	if distinct < 1 {
		logger.Verify(
			"FAIL: signature not from the release signing key")
		return fmt.Errorf(
			"signature not from the release signing key")
	}

	logger.Verify("OK self-update: signature valid " +
		"(release key, pinned fingerprint)")
	return nil
}
