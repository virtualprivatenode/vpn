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
// Four callers use verifyIsolated:
//   - verifySelfUpdate:      threshold 1 vs the vpn release key
//   - verifyBitcoinCoreSigs: threshold 2 distinct builder fingerprints
//   - verifyLNDSig:          threshold 1 vs roasbeef's fingerprint
//   - verifySyncthingSig:    threshold 1 vs the Syncthing release key
//                            (CLEARSIGNED — dataFile == "")

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
// keys.openpgp.org. Cross-check: the fingerprint in
// MIGRATION.md (Step 1) — the SAME key signed every release
// under the old name.
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
// LND v0.20.0-beta manifest signature on a live install (June 4
// 2026). Primary-key fingerprint — the signing subkey (2962...)
// is owned by this primary.
var lndSigner = struct {
	name        string
	fingerprint string
	keyURL      string
}{
	name:        "roasbeef",
	fingerprint: "A5B61896952D9FDA83BC054CDC42612E89237182",
	keyURL:      "https://raw.githubusercontent.com/lightningnetwork/lnd/master/scripts/keys/roasbeef.asc",
}

// syncthingSigner is the trusted Syncthing release signer.
// Source: https://syncthing.net/release-key.txt, linked from
// https://syncthing.net/security/ ("Release Signatures").
// Cross-checked June 9 2026 against an independent temporal
// channel: the apt keyring on a months-old production install
// (/etc/apt/keyrings/syncthing-archive-keyring.gpg) holds the
// identical primary fingerprint. Empirically bound: the v2.1.1
// sha256sum.txt.asc VALIDSIG primary fingerprint (last field)
// matches this pin on a live verification.
//
// NOTE: Syncthing dual-signs releases during key rotation — the
// v2.1.1 checksum file carries a second signature from the
// pre-rotation key (ends D26E6ED000654A3E), which our keyring
// cannot check (ERRSIG/NO_PUBKEY) and which makes gpg exit
// non-zero even on a genuine release. This is why exit-code
// trust is unusable here and VALIDSIG parsing is the sole
// source of truth (the v0.6.1 finding A/B design).
var syncthingSigner = struct {
	name        string
	fingerprint string
	keyURL      string
}{
	name:        "Syncthing Release Management",
	fingerprint: "FBA2E162F2F44657B38F0309E5665F9BD5970C47",
	keyURL:      "https://syncthing.net/release-key.txt",
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
// BADSIG parsing is the sole source of truth. (Empirically
// necessary: Syncthing's dual-signed v2.1.1 release makes gpg
// exit non-zero on a genuine artifact — see syncthingSigner.)
//
// dataFile == "" means sigFile is CLEARSIGNED (data and
// signature in one file, e.g. Syncthing's sha256sum.txt.asc);
// gpg is invoked with the single file argument. Otherwise the
// signature is detached and gpg gets both arguments.
func verifyIsolated(
	keyFiles []string,
	sigFile, dataFile string,
	pinnedFPs map[string]bool,
) (distinctValidSigners int, badSig bool, err error) {
	// Ephemeral GPG home — 0700, random path, cleaned up on return.
	gpgHome, err := os.MkdirTemp("", "vpn-gpg-")
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
	// Clearsigned input (dataFile == "") takes one file argument.
	args := []string{"--homedir", gpgHome, "--batch",
		"--verify", "--status-fd", "1", sigFile}
	if dataFile != "" {
		args = append(args, dataFile)
	}
	output, _ := system.RunCombinedOutput("gpg", args...)

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

// ── Syncthing verification ──────────────────────────────

// verifySyncthingSig verifies the CLEARSIGNED checksum file
// (sha256sum.txt.asc) against the pinned release fingerprint.
// Unlike Bitcoin Core and LND (detached signatures: signature
// and data in separate files), Syncthing ships the checksum
// list and its signature in ONE file. Must run BEFORE
// verifySyncthingChecksum — the checksums inside the file are
// untrusted until the signature over them validates.
func verifySyncthingSig(workDir string) error {
	logger.Verify("--- Syncthing signature verification ---")

	ascFile := filepath.Join(workDir, "sha256sum.txt.asc")
	if _, err := os.Stat(ascFile); err != nil {
		logger.Verify("FAIL: sha256sum.txt.asc not found")
		return fmt.Errorf("sha256sum.txt.asc not found")
	}

	keyFile := filepath.Join(workDir, "syncthing-release-key.txt")
	if err := system.DownloadRequireTor(
		syncthingSigner.keyURL, keyFile); err != nil {
		logger.Verify("FAIL: download Syncthing signing key: %v", err)
		return fmt.Errorf("download Syncthing signing key: %w", err)
	}

	pinnedFPs := map[string]bool{syncthingSigner.fingerprint: true}

	// dataFile "" → clearsigned, single-argument verify.
	distinct, hasBadSig, err := verifyIsolated(
		[]string{keyFile}, ascFile, "", pinnedFPs)
	if err != nil {
		return fmt.Errorf(
			"Syncthing signature verification failed: %w", err)
	}

	if hasBadSig {
		logger.Verify("FAIL: bad Syncthing signature detected")
		return fmt.Errorf(
			"bad Syncthing signature detected — verification aborted")
	}

	if distinct < 1 {
		logger.Verify(
			"FAIL: Syncthing signature not valid against pinned fingerprint")
		return fmt.Errorf("Syncthing signature verification failed")
	}

	logger.Verify(
		"OK Syncthing: signature valid (release key, pinned fingerprint)")
	return nil
}

// verifySyncthingChecksum checks the tarball against the
// now-trusted clearsigned checksum file. Same sha256sum
// pattern as verifyBitcoin/verifyLND — the only difference is
// that the checksum source is the clearsigned .asc itself:
// sha256sum skips the PGP armor lines (reported as "improperly
// formatted" warnings) and matches the real checksum lines.
func verifySyncthingChecksum(workDir string) error {
	logger.Verify("--- Syncthing checksum verification ---")
	// exec.Command used directly because sha256sum --check needs
	// working directory set to where the tarball was downloaded.
	cmd := exec.Command("sha256sum",
		"--ignore-missing", "--check", "sha256sum.txt.asc")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Verify("FAIL: Syncthing checksum: %s", string(output))
		return fmt.Errorf("checksum failed: %w: %s", err, output)
	}
	logger.Verify("OK Syncthing checksum: %s",
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
	// directory. verifyIsolated imports it into an ephemeral
	// GPG home — the shared keyring is never touched.
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
