// internal/system/exec.go

package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/virtualprivatenode/vpn/internal/logger"
)

// Run executes a command and returns an error with output on failure.
func Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %s: %s", name, strings.Join(args, " "), err, output)
	}
	return nil
}

// requireRoot guards every privileged wrapper below. These
// wrappers run commands DIRECTLY, with the privilege the process
// already has — there is no sudo on this box to borrow (the
// admin user has no sudo rights at all). Two process shapes are
// allowed to call them: `sudo vpn install` (root by dispatch)
// and `vpn helperd` (root via its systemd unit). Any other
// caller is a programming error: an unprivileged code path that
// should have gone through the helper client instead. Failing
// loudly here — instead of quietly prefixing sudo and hoping —
// is deliberate: it makes "no generic root escape from the TUI"
// a property the binary enforces, not a convention.
func requireRoot(name string) error {
	if os.Geteuid() == 0 {
		return nil
	}
	return fmt.Errorf(
		"%s requires root: this operation must go through "+
			"the node's root helper (vpn helperd), not run "+
			"directly — this is a bug worth reporting", name)
}

// SudoRun executes a command that needs root. The name is
// historical (kept so 100+ call sites read unchanged): it no
// longer ever invokes sudo — it requires the process itself to
// be root and refuses otherwise.
func SudoRun(name string, args ...string) error {
	if err := requireRoot(name); err != nil {
		return err
	}
	return Run(name, args...)
}

// RunOutput executes a command and returns stdout as a string.
func RunOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = nil
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output)), nil
}

// SudoRunOutput executes a root-requiring command and returns
// stdout.
func SudoRunOutput(name string, args ...string) (string, error) {
	if err := requireRoot(name); err != nil {
		return "", err
	}
	return RunOutput(name, args...)
}

// RunContext executes a command with a timeout.
func RunContext(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = nil
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output)), nil
}

// SudoRunContext executes a root-requiring command with a
// timeout.
func SudoRunContext(timeout time.Duration, name string, args ...string) (string, error) {
	if err := requireRoot(name); err != nil {
		return "", err
	}
	return RunContext(timeout, name, args...)
}

// RunCombinedOutput executes a command and returns combined stdout+stderr.
// Used for GPG commands which write status to stderr.
func RunCombinedOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output)), nil
}

// SudoRunCombinedOutput executes a root-requiring command and
// returns combined stdout+stderr.
func SudoRunCombinedOutput(name string, args ...string) (string, error) {
	if err := requireRoot(name); err != nil {
		return "", err
	}
	return RunCombinedOutput(name, args...)
}

// RunSilent executes a command and discards all output.
func RunSilent(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// SudoRunSilent executes a root-requiring command and discards
// all output.
func SudoRunSilent(name string, args ...string) error {
	if err := requireRoot(name); err != nil {
		return err
	}
	return RunSilent(name, args...)
}

// SudoWriteFile atomically writes content to a root-owned path
// (directly as root, via sudo otherwise):
// stages in a dest-dir temp (install -m), then rename(2) onto the path, so the
// canonical path only ever holds the final mode and a crash never leaves it
// partial or world-readable. os.CreateTemp's O_EXCL guards the staging file
// against symlink attacks. Write failures are logged centrally for support.
func SudoWriteFile(path string, content []byte, perm os.FileMode) error {
	tmpFile, err := os.CreateTemp("", "vpn-write-")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	tmpDest := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := SudoRun("install", "-m", fmt.Sprintf("%04o", perm),
		tmpPath, tmpDest); err != nil {
		SudoRunSilent("rm", "-f", tmpDest)
		logger.System("write %s: install: %v", path, err)
		return err
	}
	if err := SudoRun("mv", tmpDest, path); err != nil {
		SudoRunSilent("rm", "-f", tmpDest)
		logger.System("write %s: mv: %v", path, err)
		return err
	}
	return nil
}

// Download fetches a URL to a local path.
// Uses torsocks if available, but does not require it.
// Used only for downloads before Tor is installed (apt keys, etc.).
func Download(url, dest string) error {
	return doDownload(url, dest, false)
}

// DownloadRequireTor fetches a URL and fails if torsocks is not available.
// Retries up to 3 times to handle intermittent Tor DNS resolution failures
// (each attempt carries its own tool-level bounds too — see doDownload —
// so the total is bounded either way).
func DownloadRequireTor(url, dest string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		lastErr = doDownload(url, dest, true)
		if lastErr == nil {
			return nil
		}
		if attempt < 2 {
			time.Sleep(2 * time.Second)
		}
	}
	return lastErr
}

// doDownload runs one fetch with explicit time and retry bounds.
// The bounds exist for the helper's sake: verb-reachable
// downloads (self-update, the Syncthing install) run
// synchronously inside the serialized root helper, and the
// helper's per-operation deadline bounds only its socket I/O —
// a subprocess must bound itself. Left at their defaults the
// tools do not: wget retries up to 20 times with a 900 second
// read timeout, and curl has no overall cap at all once
// connected.
//
//   - wget: --timeout applies per phase (DNS, connect, read),
//     so a black-holed transfer dies within a minute instead of
//     hanging; --tries bounds attempts while still absorbing
//     transient failures — a single Tor exit-side resolution
//     failure is common enough that a single-shot fetch
//     abandons runs a retry would have saved.
//   - curl (fallback): --connect-timeout bounds setup and
//     --max-time caps the whole transfer, sized generously for
//     a large release download over a slow Tor circuit.
func doDownload(url, dest string, requireTor bool) error {
	wrapper := torWrapper()
	if requireTor && wrapper == "" {
		return fmt.Errorf("torsocks not available — cannot download over Tor")
	}
	if _, err := exec.LookPath("wget"); err == nil {
		wgetArgs := []string{"--timeout=60", "--tries=3",
			"-q", "-O", dest, url}
		if wrapper != "" {
			return Run(wrapper, append([]string{"wget"},
				wgetArgs...)...)
		}
		return Run("wget", wgetArgs...)
	}
	curlArgs := []string{"-sL", "--connect-timeout", "60",
		"--max-time", "1800", "-o", dest, url}
	if wrapper != "" {
		return Run(wrapper, append([]string{"curl"},
			curlArgs...)...)
	}
	return Run("curl", curlArgs...)
}

// torWrapper returns "torsocks" if available, empty string otherwise.
func torWrapper() string {
	if _, err := exec.LookPath("torsocks"); err == nil {
		return "torsocks"
	}
	return ""
}

// SudoReadFile reads a root-readable file. It preserves
// os.IsNotExist in the error for callers that treat a missing
// file as empty. Like every wrapper above, it requires the
// process to already be root: the unprivileged read path for
// privileged facts is the staging board (/var/lib/vpn/state),
// not a privileged copy staged on demand.
func SudoReadFile(path string) ([]byte, error) {
	if os.Geteuid() == 0 {
		return os.ReadFile(path)
	}
	return nil, requireRoot("read " + path)
}
