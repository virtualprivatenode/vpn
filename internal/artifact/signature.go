// Package artifact verifies signatures using caller-selected trust anchors.
package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// VerifySignature verifies a detached GPG signature inside an
// ephemeral keyring. It creates a temporary GPG home directory,
// imports the provided key files, and runs gpg --verify with
// --status-fd 1. Only VALIDSIG lines whose primary-key
// fingerprint (the LAST field) matches a pinned fingerprint are
// counted, and each primary fingerprint is counted at most once
// (distinct signers). Any BADSIG line sets badSig = true.
//
// GPG may exit unsuccessfully when another signature uses an unavailable key,
// as in Syncthing v2.1.1's dual-signed release. Callers must require their pinned
// signer threshold and reject badSig, rather than infer trust from exit status.
//
// dataFile == "" means sigFile is CLEARSIGNED (data and
// signature in one file, e.g. Syncthing's sha256sum.txt.asc);
// gpg is invoked with the single file argument. Otherwise the
// signature is detached and gpg gets both arguments.
func VerifySignature(
	keyFiles []string,
	sigFile, dataFile string,
	pinnedFPs map[string]bool,
) (distinctValidSigners int, badSig bool, err error) {
	// Ephemeral GPG home: 0700, random path, cleaned up on return.
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

	// Verify: exit code intentionally discarded.
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
			// VALIDSIG is the subkey fingerprint: we must
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
