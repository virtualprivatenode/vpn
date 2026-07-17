// internal/installer/tor.go

package installer

import (
	"fmt"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

func installTor() error {
	return system.SudoRun("apt-get", "install", "-y", "-qq", "tor", "torsocks")
}

// BuildTorConfig generates the complete torrc content from config state.
// Pure logic — no side effects.
// Note: HiddenServiceDir paths are hardcoded strings because they are
// torrc config content read by Tor, not Go logic paths.
func BuildTorConfig(cfg *config.AppConfig) string {
	net := cfg.NetworkConfig()

	var b strings.Builder
	b.WriteString("# Virtual Private Node — Tor Configuration\n")
	b.WriteString("SOCKSPort 9050\n")

	// Control port: always emitted. Two consumers — the install-path
	// Tor routing gate (torgate.go reads bootstrap progress here,
	// unconditionally) and LND's P2P onion management. Loopback-only,
	// cookie-authenticated; emitting it without LND adds no exposure.
	b.WriteString("\n# Control port (install routing gate + LND onion management)\n")
	b.WriteString("ControlPort 9051\n")
	b.WriteString("CookieAuthentication 1\n")
	b.WriteString("CookieAuthFileGroupReadable 1\n")

	b.WriteString(fmt.Sprintf(`
# Bitcoin Core P2P (static onion address for peers)
HiddenServiceDir /var/lib/tor/bitcoin-p2p/
HiddenServicePort %d 127.0.0.1:%d
`, net.P2PPort, net.P2PPort))

	if cfg.HasLND() {
		b.WriteString(`
# LND gRPC (wallet connections over Tor)
HiddenServiceDir /var/lib/tor/lnd-grpc/
HiddenServicePort 10009 127.0.0.1:10009

# LND REST (wallet connections over Tor)
HiddenServiceDir /var/lib/tor/lnd-rest/
HiddenServicePort 8080 127.0.0.1:8080
`)
	}

	if cfg.SyncthingInstalled {
		b.WriteString(`
# Syncthing web UI (Tor only, HTTP)
HiddenServiceDir /var/lib/tor/syncthing/
HiddenServicePort 8384 127.0.0.1:8384
`)
		// Sync protocol (port 22000) goes over clearnet.
		// No hidden service needed — Syncthing uses mutual TLS
		// with explicit device approval for authentication.
	}

	return b.String()
}

// RebuildTorConfig writes the torrc to disk.
func RebuildTorConfig(cfg *config.AppConfig) error {
	content := BuildTorConfig(cfg)
	if err := system.SudoWriteFile(
		paths.Torrc, []byte(content), 0640); err != nil {
		return err
	}
	return system.SudoRun("chown", "root:debian-tor", paths.Torrc)
}

func addUserToTorGroup(username string) error {
	return system.SudoRun("usermod", "-aG", "debian-tor", username)
}

func restartTor() error {
	if err := system.SudoRun("systemctl", "enable", "tor"); err != nil {
		return err
	}
	return system.SudoRun("systemctl", "restart", "tor")
}

// RestartTor is the exported wrapper for use by the welcome
// package during install rollback.
func RestartTor() error { return restartTor() }
