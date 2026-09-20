// internal/installer/bitcoin.go

package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/virtualprivatenode/vpn/internal/bitcoin"
	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/host"
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
// state. Pure logic — no side effects. Non-empty rpcauthLines
// are the salted-hash credential lines for the TUI and LND
// (see rpcauth.go); they are placed in the GLOBAL
// section deliberately — auth options are not network-scoped,
// and on testnet4 an appended line would land inside the
// [testnet4] section.
func BuildBitcoinConfig(
	cfg *config.AppConfig, rpcauthLines ...string,
) (string, error) {
	net, err := cfg.NetworkConfig()
	if err != nil {
		return "", err
	}
	pruneMB := cfg.PruneSize * 1000

	var auth string
	for _, line := range rpcauthLines {
		if line != "" {
			auth += line + "\n"
		}
	}

	var b strings.Builder
	b.WriteString("# Virtual Private Node — Bitcoin Core\n")
	b.WriteString("server=1\n")
	b.WriteString("disablewallet=1\n")
	// VPN has two explicit rpcauth identities. Keeping Core's independent
	// session cookie would add a third credential that no supported client
	// consumes.
	b.WriteString("norpccookiefile=1\n")
	if net.BitcoinFlag != "" {
		b.WriteString(net.BitcoinFlag + "\n")
	}
	fmt.Fprintf(&b, "prune=%d\n", pruneMB)
	fmt.Fprintf(&b, "dbcache=%d\n", cfg.DbCacheMB())
	b.WriteString("maxmempool=300\n")
	b.WriteString("proxy=127.0.0.1:9050\n")
	b.WriteString("listen=1\n")
	b.WriteString("listenonion=1\n")
	b.WriteString(auth)
	if net.CoreNetwork != "main" {
		fmt.Fprintf(&b, "\n[%s]\n", net.CoreNetwork)
	}
	b.WriteString("bind=127.0.0.1\n")
	b.WriteString("rpcbind=127.0.0.1\n")
	fmt.Fprintf(&b, "rpcport=%d\n", net.RPCPort)
	b.WriteString("rpcallowip=127.0.0.1\n")
	fmt.Fprintf(&b, "zmqpubrawblock=tcp://127.0.0.1:%d\n", net.ZMQBlockPort)
	fmt.Fprintf(&b, "zmqpubrawtx=tcp://127.0.0.1:%d\n", net.ZMQTxPort)
	return b.String(), nil
}

// writeBitcoinConfig rotates both local RPC identities and
// writes both consumers' configurations in one resumable
// install step. The TUI cleartext is staged on the board; the
// LND cleartext is written only to root:lnd lnd.conf. A failure
// between writes is detectable authentication failure and the
// incomplete btc group reruns the whole operation.
func writeBitcoinConfig(cfg *config.AppConfig) error {
	creds, err := writeRPCAuthCredentials()
	if err != nil {
		return err
	}
	content, err := BuildBitcoinConfig(cfg, creds.lines...)
	if err != nil {
		return err
	}
	if err := system.SudoWriteFile(paths.BitcoinConf, []byte(content), 0640); err != nil {
		return err
	}
	if err := system.SudoRun(
		"chown", "root:"+bitcoinUser, paths.BitcoinConf); err != nil {
		return err
	}
	return host.WriteLNDConfigWithRPCPassword(cfg, "", creds.lndPassword)
}

func bitcoindServiceUnit(username string) string {
	return fmt.Sprintf(`[Unit]
Description=Bitcoin Core
After=network-online.target tor.service
Wants=network-online.target

[Service]
Type=simple
User=%s
Group=%s
SupplementaryGroups=debian-tor
UMask=0077
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
}

func writeBitcoindService(username string) error {
	return system.SudoWriteFile(paths.BitcoindService,
		[]byte(bitcoindServiceUnit(username)), 0644)
}

// startBitcoind enables and starts Bitcoin Core. `systemctl
// restart` rather than `start`, deliberately: start is a no-op
// on a service that is already running during an interrupted-install resume.
// Restart makes the unit and config already written by this lifecycle the ones
// actually in effect; on a fresh pass the two commands are equivalent.
func startBitcoind(cfg *config.AppConfig) error {
	if err := system.SudoRun("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "enable", "bitcoind"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "restart", "bitcoind"); err != nil {
		return err
	}
	profile, err := cfg.NetworkConfig()
	if err != nil {
		return err
	}
	return waitForBitcoinIdentity(
		profile, bitcoin.GetBlockchainIdentity, time.Sleep, 60, 2*time.Second)
}

func validateBitcoinIdentity(
	profile *config.NetworkConfig, identity bitcoin.BlockchainIdentity,
) error {
	if identity.Chain != profile.CoreNetwork {
		return fmt.Errorf("Bitcoin Core reports chain %q, want %q for profile %q",
			identity.Chain, profile.CoreNetwork, profile.Name)
	}
	if identity.Genesis != profile.ExpectedGenesis {
		return fmt.Errorf("Bitcoin Core genesis is %q, want %q for profile %q",
			identity.Genesis, profile.ExpectedGenesis, profile.Name)
	}
	if identity.SignetChallenge != profile.ExpectedSignetChallenge {
		return fmt.Errorf(
			"Bitcoin Core signet challenge is %q, want %q for profile %q",
			identity.SignetChallenge, profile.ExpectedSignetChallenge,
			profile.Name)
	}
	return nil
}

func waitForBitcoinIdentity(
	profile *config.NetworkConfig,
	probe func(int) (bitcoin.BlockchainIdentity, error),
	sleep func(time.Duration), attempts int, interval time.Duration,
) error {
	if attempts < 1 {
		return fmt.Errorf("Bitcoin Core identity verification has no attempts")
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		identity, err := probe(profile.RPCPort)
		if err == nil {
			if err := validateBitcoinIdentity(profile, identity); err != nil {
				return err
			}
			return nil
		}
		lastErr = err
		if i+1 < attempts {
			sleep(interval)
		}
	}
	return fmt.Errorf(
		"Bitcoin Core did not expose verifiable %s identity on RPC port %d: %w",
		profile.Name, profile.RPCPort, lastErr)
}
