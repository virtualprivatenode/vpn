// internal/installer/lnd.go

package installer

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
)

func downloadLND(version string) error {
	filename := fmt.Sprintf("lnd-linux-amd64-v%s.tar.gz", version)
	url := fmt.Sprintf("https://github.com/lightningnetwork/lnd/releases/download/v%s/%s",
		version, filename)
	manifestURL := fmt.Sprintf("https://github.com/lightningnetwork/lnd/releases/download/v%s/manifest-v%s.txt",
		version, version)
	if err := system.DownloadRequireTor(url, "/tmp/"+filename); err != nil {
		return err
	}
	if err := system.DownloadRequireTor(manifestURL, "/tmp/manifest.txt"); err != nil {
		return fmt.Errorf("download LND manifest: %w", err)
	}
	return nil
}

func extractAndInstallLND(version string) error {
	filename := fmt.Sprintf("lnd-linux-amd64-v%s.tar.gz", version)
	if err := system.Run("tar", "-xzf", "/tmp/"+filename, "-C", "/tmp"); err != nil {
		return err
	}
	extractDir := fmt.Sprintf("/tmp/lnd-linux-amd64-v%s", version)
	for _, bin := range []string{"lnd", "lncli"} {
		src := fmt.Sprintf("%s/%s", extractDir, bin)
		if err := system.SudoRun("install", "-m", "0755", "-o", "root", "-g", "root", src, "/usr/local/bin/"); err != nil {
			return err
		}
	}
	os.Remove("/tmp/" + filename)
	os.Remove("/tmp/manifest.txt")
	os.RemoveAll(extractDir)
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

func writeLNDConfig(cfg *config.AppConfig, publicIPv4 string) error {
	net := cfg.NetworkConfig()
	restOnion := strings.TrimSpace(readFileOrDefault(paths.TorLNDRESTHostname, ""))

	listenLine := "listen=localhost:9735"
	restListenLine := "restlisten=localhost:8080"
	externalLine := ""
	tlsExtraIP := ""
	if cfg.P2PMode == "hybrid" && publicIPv4 != "" {
		listenLine = "listen=0.0.0.0:9735"
		restListenLine = "restlisten=0.0.0.0:8080"
		externalLine = fmt.Sprintf("externalhosts=%s:9735", publicIPv4)
		tlsExtraIP = fmt.Sprintf("tlsextraip=%s", publicIPv4)
	}

	tlsExtraDomain := ""
	if restOnion != "" {
		tlsExtraDomain = fmt.Sprintf("tlsextradomain=%s", restOnion)
	}

	cookiePath := paths.LNDCookiePath(net.CookiePath)

	content := fmt.Sprintf(`# Virtual Private Node — LND
[Application Options]
lnddir=/var/lib/lnd
%s
rpclisten=localhost:10009
%s
debuglevel=info
%s
%s
%s

# Let LND own its TLS cert lifecycle. tlsautorefresh
# regenerates the cert when its parameters change
# (e.g. tlsextraip is added during a P2P upgrade) or
# when it's near expiry. tlsdisableautofill keeps the
# cert deterministic — it contains only what we set
# explicitly here, not autodetected interface IPs.
# This is the same pattern used by Raspiblitz.
tlsautorefresh=1
tlsdisableautofill=1

[Bitcoin]
%s
bitcoin.node=bitcoind

[Bitcoind]
bitcoind.dir=/var/lib/bitcoin
bitcoind.config=/etc/bitcoin/bitcoin.conf
bitcoind.rpccookie=%s
bitcoind.rpchost=127.0.0.1:%d
bitcoind.zmqpubrawblock=tcp://127.0.0.1:%d
bitcoind.zmqpubrawtx=tcp://127.0.0.1:%d

[Tor]
tor.active=true
tor.socks=127.0.0.1:9050
tor.control=127.0.0.1:9051
tor.targetipaddress=127.0.0.1
tor.v3=true
tor.streamisolation=true
`, listenLine, restListenLine, externalLine, tlsExtraDomain, tlsExtraIP,
		net.LNDBitcoinFlag, cookiePath,
		net.RPCPort, net.ZMQBlockPort, net.ZMQTxPort)

	if err := system.SudoWriteFile(paths.LNDConf, []byte(content), 0640); err != nil {
		return err
	}
	return system.SudoRun("chown", "root:"+systemUser, paths.LNDConf)
}

func writeLNDServiceInitial(username string) error {
	content := fmt.Sprintf(`[Unit]
Description=LND Lightning Network Daemon
After=bitcoind.service tor.service
Wants=bitcoind.service

[Service]
Type=simple
User=%s
Group=%s
ExecStart=/usr/local/bin/lnd --configfile=/etc/lnd/lnd.conf
Restart=on-failure
RestartSec=30
TimeoutStopSec=300
PrivateTmp=true
ProtectSystem=full
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`, username, username)
	return system.SudoWriteFile(paths.LNDService, []byte(content), 0644)
}

func startLND() error {
	if err := system.SudoRun("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "enable", "lnd"); err != nil {
		return err
	}
	return system.SudoRun("systemctl", "start", "lnd")
}

func setupAutoUnlock(password string) error {
	// Write password to a secure temp file, then sudo move it.
	// os.CreateTemp uses O_EXCL to prevent symlink attacks.
	tmpFile, err := os.CreateTemp("", "rlvpn-wallet-pw-")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPw := tmpFile.Name()
	if _, err := tmpFile.Write([]byte(password)); err != nil {
		tmpFile.Close()
		os.Remove(tmpPw)
		return err
	}
	tmpFile.Close()
	defer os.Remove(tmpPw)

	passwordFile := paths.LNDWalletPassword
	if err := system.SudoRun("cp", tmpPw, passwordFile); err != nil {
		return err
	}
	if err := system.SudoRun("chmod", "0400", passwordFile); err != nil {
		return err
	}
	if err := system.SudoRunSilent("chown", systemUser+":"+systemUser, passwordFile); err != nil {
		logger.System("Warning: chown wallet password: %v", err)
	}

	content := fmt.Sprintf(`[Unit]
Description=LND Lightning Network Daemon
After=bitcoind.service tor.service
Wants=bitcoind.service

[Service]
Type=simple
User=%s
Group=%s
ExecStart=/usr/local/bin/lnd --configfile=/etc/lnd/lnd.conf --wallet-unlock-password-file=/var/lib/lnd/wallet_password
Restart=on-failure
RestartSec=30
TimeoutStopSec=300
PrivateTmp=true
ProtectSystem=full
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`, systemUser, systemUser)

	if err := system.SudoWriteFile(paths.LNDService, []byte(content), 0644); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "daemon-reload"); err != nil {
		return err
	}
	return system.SudoRun("systemctl", "restart", "lnd")
}

// disableAutoUnlock removes the wallet password file
// and rewrites the LND systemd service back to its
// initial (no auto-unlock) form, then restarts LND.
// After this returns successfully, LND will require
// manual unlock (e.g. `lncli unlock`) on next startup.
func disableAutoUnlock() error {
	// SudoRunSilent because the file may not exist if
	// called from an inconsistent state — that's fine,
	// we just want it gone.
	system.SudoRunSilent(
		"rm", "-f", paths.LNDWalletPassword)

	if err := writeLNDServiceInitial(systemUser); err != nil {
		return fmt.Errorf("rewrite service: %w", err)
	}
	if err := system.SudoRun(
		"systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	return system.SudoRun(
		"systemctl", "restart", "lnd")
}

func waitForLND() error {
	for i := 0; i < 60; i++ {
		client := buildLNDClient()
		resp, err := client.Get("https://localhost:8080/v1/state")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("LND did not respond after 120 seconds")
}

// ── Exported wrappers for the welcome package ───────────
// These wrap the unexported helpers so the welcome
// package can call them from screens without leaking
// the rest of the installer package.

// WaitForLND blocks until LND's REST API responds, or
// returns an error after 120 seconds. Safe to call as a
// tea.Cmd from a screen.
func WaitForLND() error {
	return waitForLND()
}

// SetupAutoUnlock writes the wallet password to a
// permission-locked file and rewrites the LND systemd
// service to start LND with --wallet-unlock-password-file.
// LND is restarted as the final step.
func SetupAutoUnlock(password string) error {
	return setupAutoUnlock(password)
}

// DisableAutoUnlock removes the wallet password file and
// rewrites the LND systemd service back to its initial
// (no-auto-unlock) form. LND is restarted as the final
// step. After this call, LND will require manual unlock
// (e.g. `lncli unlock`) on next startup.
func DisableAutoUnlock() error {
	return disableAutoUnlock()
}

func buildLNDClient() *http.Client {
	tlsConfig := &tls.Config{}
	// Try direct read first, fall back to sudo
	certData, err := os.ReadFile(paths.LNDTLSCert)
	if err != nil {
		output, sudoErr := system.SudoRunOutput("cat", paths.LNDTLSCert)
		if sudoErr == nil {
			certData = []byte(output)
			err = nil
		}
	}
	if err == nil {
		pool := x509.NewCertPool()
		if pool.AppendCertsFromPEM(certData) {
			tlsConfig.RootCAs = pool
		}
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
		Timeout:   5 * time.Second,
	}
}
