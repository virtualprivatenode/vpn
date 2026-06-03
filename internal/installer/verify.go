// internal/installer/verify.go

// Trust model
//
// All signing-key fingerprints are pinned in this file as the sole trust
// anchors. Keys are fetched fresh each run and imported into an ephemeral
// GPG home directory (os.MkdirTemp, 0700). Signatures are verified via
// gpg --status-fd 1; only [GNUPG:] VALIDSIG lines whose primary-key
// fingerprint (the LAST whitespace-delimited field) matches a pinned
// value are counted. Distinct signers are counted once (deduped by
// primary fingerprint). Any [GNUPG:] BADSIG line is a hard stop. The
// GPG exit code is never trusted.
//
// Three callers use verifyIsolated:
//   - verifySelfUpdate:      threshold 1 vs the rlvpn release key
//   - verifyBitcoinCoreSigs: threshold 2 distinct builder fingerprints
//   - verifyLNDSig:          threshold 1 vs roasbeef's fingerprint

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/system"
)

// ── Trust anchors ───────────────────────────────────────
//
// Each fingerprint was sourced and cross-checked as noted.
// To re-verify: download the key from the listed URL, import
// into an ephemeral keyring, and confirm the fingerprint with
// gpg --with-colons --list-keys.

// rlvpnReleaseFP is the primary fingerprint of the rlvpn
// release signing key.
// Source: generated locally; public key hosted at
// keys.openpgp.org. Cross-check: SIGNING_KEY_FP in
// virtual-private-node.sh (line 24).
const rlvpnReleaseFP = "AFA0EBACDC9A4C4AA7B0154AC97CE10F170BA5FE"

// bitcoinCoreSigners are the trusted Bitcoin Core builder keys.
// Source: github.com/bitcoin-core/guix.sigs/tree/main/builder-keys
// Cross-check: each key's fingerprint against the .gpg file at
// the listed URL. Threshold: 2 of 5 distinct signers required.
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

// lndSigner is the trusted LND release signer.
// Source: github.com/lightningnetwork/lnd/tree/master/scripts/keys
// Cross-check: roasbeef.asc at the listed URL.
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

// ── Isolated signature verification ─────────────────────

// verifyIsolated verifies a detached GPG signature inside an
// ephemeral keyring. It creates a temporary GPG home directory,
// imports the provided key files, and runs gpg --verify with
// --status-fd 1. Only VALIDSIG lines whose primary-key
// fingerprint (the LAST field) matches a pinned fingerprint are
// counted, and each primary fingerprint is counted at most once
// (distinct signers). Any BADSIG line sets badSig = true.
//
// The GPG exit code is intentionally ignored — the VALIDSIG and
// BADSIG parsing is the sole source of truth.
func verifyIsolated(
	keyFiles []string,
	sigFile, dataFile string,
	pinnedFPs map[string]bool,
) (distinctValidSigners int, badSig bool, err error) {
	// Ephemeral GPG home — 0700, random path, cleaned up on return.
	gpgHome, err := os.MkdirTemp("", "rlvpn-gpg-")
	if err != nil {
		return 0, false, fmt.Errorf(
			"create ephemeral gpg home: %w", err)
	}
	defer os.RemoveAll(gpgHome)

	// Import key files into the ephemeral keyring.
	for _, kf := range keyFiles {
		output, importErr := system.RunCombinedOutput(
			"gpg", "--homedir", gpgHome,
			"--batch", "--import", kf)
		if importErr != nil {
			logger.Verify("SKIP key import %s: %v: %s",
				filepath.Base(kf), importErr, output)
		}
	}

	// Verify — exit code intentionally discarded.
	output, _ := system.RunCombinedOutput(
		"gpg", "--homedir", gpgHome, "--batch", "--verify",
		"--status-fd", "1", sigFile, dataFile)

	// Parse status output.
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "[GNUPG:] BADSIG") {
			badSig = true
			logger.Verify("BADSIG: %s", trimmed)
		}

		if strings.HasPrefix(trimmed, "[GNUPG:] VALIDSIG") {
			fields := strings.Fields(trimmed)
			// VALIDSIG layout (after [GNUPG:] VALIDSIG):
			//   <signing-fpr> <date> <ts> <exp> <ver> <res>
			//   <pkalgo> <halgo> <sigclass> <primary-fpr>
			// The LAST field is the primary-key fingerprint.
			// When a subkey signs, the first field after
			// VALIDSIG is the subkey fingerprint — we must
			// match the LAST field (primary) against our pins.
			if len(fields) >= 3 {
				primaryFP := fields[len(fields)-1]
				if pinnedFPs[primaryFP] {
					if !seen[primaryFP] {
						seen[primaryFP] = true
						logger.Verify(
							"VALIDSIG pinned: %s", primaryFP)
					}
				} else {
					logger.Verify(
						"VALIDSIG unpinned (ignored): %s",
						primaryFP)
				}
			}
		}
	}

	return len(seen), badSig, nil
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

	distinct, hasBadSig, err := verifyIsolated(
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

	distinct, hasBadSig, err := verifyIsolated(
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
	// directory. verifyIsolated imports it into an ephemeral
	// GPG home — the shared keyring is never touched.
	keyFile := filepath.Join(workDir, "release-key.asc")
	keyURL := fmt.Sprintf(
		"https://keys.openpgp.org/vks/v1/by-fingerprint/%s",
		rlvpnReleaseFP)
	if err := system.DownloadRequireTor(
		keyURL, keyFile); err != nil {
		logger.Verify(
			"FAIL: download release signing key: %v", err)
		return fmt.Errorf(
			"download release signing key: %w", err)
	}

	pinnedFPs := map[string]bool{rlvpnReleaseFP: true}

	distinct, hasBadSig, err := verifyIsolated(
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

// ── Checksum verification ───────────────────────────────

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
