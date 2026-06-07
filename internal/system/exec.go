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

	"github.com/ripsline/virtual-private-node/internal/logger"
)

// Run executes a command and returns an error with output on failure.
func Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %s: %s", name, strings.Join(args, " "), err, output)
	}
	return nil
}

// SudoRun executes a command via sudo.
func SudoRun(name string, args ...string) error {
	sudoArgs := append([]string{name}, args...)
	return Run("sudo", sudoArgs...)
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

// SudoRunOutput executes a command via sudo and returns stdout.
func SudoRunOutput(name string, args ...string) (string, error) {
	sudoArgs := append([]string{name}, args...)
	return RunOutput("sudo", sudoArgs...)
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

// SudoRunContext executes a command via sudo with a timeout.
func SudoRunContext(timeout time.Duration, name string, args ...string) (string, error) {
	sudoArgs := append([]string{name}, args...)
	return RunContext(timeout, "sudo", sudoArgs...)
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

// SudoRunCombinedOutput executes a command via sudo and returns combined stdout+stderr.
func SudoRunCombinedOutput(name string, args ...string) (string, error) {
	sudoArgs := append([]string{name}, args...)
	return RunCombinedOutput("sudo", sudoArgs...)
}

// RunSilent executes a command and discards all output.
func RunSilent(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// SudoRunSilent executes a command via sudo and discards all output.
func SudoRunSilent(name string, args ...string) error {
	sudoArgs := append([]string{name}, args...)
	return RunSilent("sudo", sudoArgs...)
}

// SudoWriteFile atomically writes content to a root-owned path via sudo:
// stages in a dest-dir temp (install -m), then rename(2) onto the path, so the
// canonical path only ever holds the final mode and a crash never leaves it
// partial or world-readable. os.CreateTemp's O_EXCL guards the staging file
// against symlink attacks. Write failures are logged centrally for support.
func SudoWriteFile(path string, content []byte, perm os.FileMode) error {
	tmpFile, err := os.CreateTemp("", "rlvpn-write-")
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
// Retries up to 3 times to handle intermittent Tor DNS resolution failures.
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

func doDownload(url, dest string, requireTor bool) error {
	wrapper := torWrapper()
	if requireTor && wrapper == "" {
		return fmt.Errorf("torsocks not available — cannot download over Tor")
	}
	if _, err := exec.LookPath("wget"); err == nil {
		if wrapper != "" {
			return Run(wrapper, "wget", "-q", "-O", dest, url)
		}
		return Run("wget", "-q", "-O", dest, url)
	}
	if wrapper != "" {
		return Run(wrapper, "curl", "-sL", "-o", dest, url)
	}
	return Run("curl", "-sL", "-o", dest, url)
}

// torWrapper returns "torsocks" if available, empty string otherwise.
func torWrapper() string {
	if _, err := exec.LookPath("torsocks"); err == nil {
		return "torsocks"
	}
	return ""
}

// SudoReadFile reads a file that requires root/sudo access.
// Uses os.CreateTemp for secure temp file creation.
func SudoReadFile(path string) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "rlvpn-read-")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := SudoRun("cp", path, tmpPath); err != nil {
		return nil, fmt.Errorf("sudo cp %s: %w", path, err)
	}
	if err := SudoRun("chmod", "0600", tmpPath); err != nil {
		return nil, fmt.Errorf("chmod tmp: %w", err)
	}
	if err := SudoRun("chown",
		fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), tmpPath); err != nil {
		return nil, fmt.Errorf("chown tmp: %w", err)
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("read tmp: %w", err)
	}
	return data, nil
}
