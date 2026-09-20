// internal/installer/lnd.go

package installer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/host"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

func downloadLND(version, workDir string) error {
	filename := fmt.Sprintf("lnd-linux-amd64-v%s.tar.gz", version)
	url := fmt.Sprintf(
		"https://github.com/lightningnetwork/lnd/releases/download/v%s/%s",
		version, filename)
	manifestURL := fmt.Sprintf(
		"https://github.com/lightningnetwork/lnd/releases/download/v%s/manifest-v%s.txt",
		version, version)
	if err := system.DownloadRequireTor(
		url, filepath.Join(workDir, filename)); err != nil {
		return err
	}
	if err := system.DownloadRequireTor(
		manifestURL,
		filepath.Join(workDir, "manifest.txt")); err != nil {
		return fmt.Errorf("download LND manifest: %w", err)
	}
	return nil
}

func extractAndInstallLND(version, workDir string) error {
	filename := fmt.Sprintf("lnd-linux-amd64-v%s.tar.gz", version)
	if err := system.Run("tar", "-xzf",
		filepath.Join(workDir, filename),
		"-C", workDir); err != nil {
		return err
	}
	extractDir := filepath.Join(workDir,
		fmt.Sprintf("lnd-linux-amd64-v%s", version))
	for _, bin := range []string{"lnd", "lncli"} {
		src := filepath.Join(extractDir, bin)
		if err := system.SudoRun("install", "-m", "0755",
			"-o", "root", "-g", "root",
			src, "/usr/local/bin/"); err != nil {
			return err
		}
	}
	return nil
}

func createLNDDirs(username string) error {
	dirs := []struct {
		path  string
		owner string
		mode  os.FileMode
	}{
		{paths.LNDDir, "root:" + username, 0750},
		{paths.LNDDataDir, username + ":" + username, 0750},
	}
	for _, d := range dirs {
		if err := system.SudoRun("mkdir", "-p", d.path); err != nil {
			return err
		}
		if err := system.SudoRun("chown", d.owner, d.path); err != nil {
			return err
		}
		if err := system.SudoRun("chmod", fmt.Sprintf("%o", d.mode), d.path); err != nil {
			return err
		}
	}
	return nil
}

// writeLNDService writes the LND unit in the requested variant.
func writeLNDService(username string, withUnlock bool) error {
	return system.SudoWriteFile(paths.LNDService,
		[]byte(host.LNDServiceUnit(username, withUnlock)), 0644)
}

// writeLNDServiceFromConfig writes the LND unit that matches the
// node's desired state: the unlock variant when the config says
// auto-unlock is enabled AND the wallet password file is actually
// present, the plain variant otherwise. Requiring the file too is deliberate: a
// unit pointing at a missing password file would keep LND from
// starting at all.
func writeLNDServiceFromConfig(cfg *config.AppConfig, username string) error {
	withUnlock := cfg.AutoUnlock && walletPasswordFileExists()
	if cfg.AutoUnlock && !withUnlock {
		logger.Install(
			"auto_unlock is enabled in the config but %s is "+
				"missing — writing the LND unit without the unlock "+
				"flag; re-enable auto-unlock from the node TUI",
			paths.LNDWalletPassword)
	}
	return writeLNDService(username, withUnlock)
}

func walletPasswordFileExists() bool {
	_, err := os.Stat(paths.LNDWalletPassword)
	return err == nil
}

// startLND enables and starts LND. `systemctl restart` rather
// than `start`, deliberately: start is a no-op on a service that
// is already running during an interrupted-install resume. Restart makes the
// unit and config already written by this lifecycle the ones actually in
// effect; on a fresh pass the two commands are equivalent.
func startLND() error {
	if err := system.SudoRun("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "enable", "lnd"); err != nil {
		return err
	}
	return system.SudoRun("systemctl", "restart", "lnd")
}
