// internal/installer/bitcoin.go

package installer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
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

// BuildBitcoinConfig generates bitcoin.conf content from config state.
// Pure logic — no side effects.
func BuildBitcoinConfig(cfg *config.AppConfig) string {
	net := cfg.NetworkConfig()
	pruneMB := cfg.PruneSize * 1000

	if net.Name == "testnet4" {
		return fmt.Sprintf(`# Virtual Private Node — Bitcoin Core
server=1
disablewallet=1
%s
prune=%d
dbcache=512
maxmempool=300
proxy=127.0.0.1:9050
listen=1
listenonion=1

[testnet4]
bind=127.0.0.1
rpcbind=127.0.0.1
rpcport=%d
rpcallowip=127.0.0.1
zmqpubrawblock=tcp://127.0.0.1:%d
zmqpubrawtx=tcp://127.0.0.1:%d
`, net.BitcoinFlag, pruneMB,
			net.RPCPort, net.ZMQBlockPort, net.ZMQTxPort)
	}

	return fmt.Sprintf(`# Virtual Private Node — Bitcoin Core
server=1
disablewallet=1
prune=%d
dbcache=512
maxmempool=300
proxy=127.0.0.1:9050
listen=1
listenonion=1

bind=127.0.0.1
rpcbind=127.0.0.1
rpcport=%d
rpcallowip=127.0.0.1
zmqpubrawblock=tcp://127.0.0.1:%d
zmqpubrawtx=tcp://127.0.0.1:%d
`, pruneMB,
		net.RPCPort, net.ZMQBlockPort, net.ZMQTxPort)
}

func writeBitcoinConfig(cfg *config.AppConfig) error {
	content := BuildBitcoinConfig(cfg)
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
