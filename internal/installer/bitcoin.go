// internal/installer/bitcoin.go

package installer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

func downloadBitcoin(version, workDir string) error {
	filename := fmt.Sprintf("bitcoin-%s-x86_64-linux-gnu.tar.gz", version)
	baseURL := fmt.Sprintf("https://bitcoincore.org/bin/bitcoin-core-%s", version)
	if err := system.DownloadRequireTor(
		baseURL+"/"+filename,
		filepath.Join(workDir, filename)); err != nil {
		return err
	}
	if err := system.DownloadRequireTor(
		baseURL+"/SHA256SUMS",
		filepath.Join(workDir, "SHA256SUMS")); err != nil {
		return err
	}
	return system.DownloadRequireTor(
		baseURL+"/SHA256SUMS.asc",
		filepath.Join(workDir, "SHA256SUMS.asc"))
}

func extractAndInstallBitcoin(version, workDir string) error {
	filename := fmt.Sprintf("bitcoin-%s-x86_64-linux-gnu.tar.gz", version)
	if err := system.Run("tar", "-xzf",
		filepath.Join(workDir, filename),
		"-C", workDir); err != nil {
		return err
	}
	extractDir := filepath.Join(workDir,
		fmt.Sprintf("bitcoin-%s", version), "bin")
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}
	for _, entry := range entries {
		src := filepath.Join(extractDir, entry.Name())
		if err := system.SudoRun("install", "-m", "0755",
			"-o", "root", "-g", "root",
			src, "/usr/local/bin/"); err != nil {
			return err
		}
	}
	return nil
}

// BuildBitcoinConfig generates bitcoin.conf content from config
// state. Pure logic — no side effects. rpcauthLine, when
// non-empty, is the salted-hash credential line for this node's
// own tooling (see rpcauth.go); it is placed in the GLOBAL
// section deliberately — auth options are not network-scoped,
// and on testnet4 an appended line would land inside the
// [testnet4] section.
func BuildBitcoinConfig(cfg *config.AppConfig, rpcauthLine string) string {
	net := cfg.NetworkConfig()
	pruneMB := cfg.PruneSize * 1000

	auth := ""
	if rpcauthLine != "" {
		auth = rpcauthLine + "\n"
	}

	if net.Name == "testnet4" {
		return fmt.Sprintf(`# Virtual Private Node — Bitcoin Core
server=1
disablewallet=1
%s
prune=%d
dbcache=%d
maxmempool=300
proxy=127.0.0.1:9050
listen=1
listenonion=1
%s
[testnet4]
bind=127.0.0.1
rpcbind=127.0.0.1
rpcport=%d
rpcallowip=127.0.0.1
zmqpubrawblock=tcp://127.0.0.1:%d
zmqpubrawtx=tcp://127.0.0.1:%d
`, net.BitcoinFlag, pruneMB, cfg.DbCacheMB(), auth,
			net.RPCPort, net.ZMQBlockPort, net.ZMQTxPort)
	}

	return fmt.Sprintf(`# Virtual Private Node — Bitcoin Core
server=1
disablewallet=1
prune=%d
dbcache=%d
maxmempool=300
proxy=127.0.0.1:9050
listen=1
listenonion=1
%s
bind=127.0.0.1
rpcbind=127.0.0.1
rpcport=%d
rpcallowip=127.0.0.1
zmqpubrawblock=tcp://127.0.0.1:%d
zmqpubrawtx=tcp://127.0.0.1:%d
`, pruneMB, cfg.DbCacheMB(), auth,
		net.RPCPort, net.ZMQBlockPort, net.ZMQTxPort)
}

// writeBitcoinConfig writes bitcoin.conf with a FRESH rpcauth
// credential, staging the matching password on the board in
// the same operation — the hashed line and the cleartext are
// only ever replaced together, so they cannot drift apart.
func writeBitcoinConfig(cfg *config.AppConfig) error {
	rpcauthLine, err := writeRPCAuthCredential()
	if err != nil {
		return err
	}
	content := BuildBitcoinConfig(cfg, rpcauthLine)
	if err := system.SudoWriteFile(paths.BitcoinConf, []byte(content), 0640); err != nil {
		return err
	}
	return system.SudoRun("chown", "root:"+systemUser, paths.BitcoinConf)
}

func writeBitcoindService(username string) error {
	content := fmt.Sprintf(`[Unit]
Description=Bitcoin Core
After=network-online.target tor.service
Wants=network-online.target

[Service]
Type=simple
User=%s
Group=%s
ExecStart=/usr/local/bin/bitcoind -conf=/etc/bitcoin/bitcoin.conf -datadir=/var/lib/bitcoin
Restart=on-failure
RestartSec=30
TimeoutStopSec=600
PrivateTmp=true
ProtectSystem=full
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`, username, username)
	return system.SudoWriteFile(paths.BitcoindService, []byte(content), 0644)
}

func startBitcoind() error {
	if err := system.SudoRun("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "enable", "bitcoind"); err != nil {
		return err
	}
	return system.SudoRun("systemctl", "start", "bitcoind")
}
