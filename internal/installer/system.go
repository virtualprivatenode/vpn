// internal/installer/system.go

package installer

import (
	"fmt"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/host"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// The OS check formerly here (checkOS, Debian 13-or-newer) is
// superseded by the preflight's exactly-13 assertion (ruling ix).
// See preflight.go.

// ensureRootOwnedVarLibVPN revalidates the lifecycle-owned ancestor before a
// later install step uses it. Creation belongs exclusively to lifecycle
// initialization; resume never normalizes an unsafe object.
func ensureRootOwnedVarLibVPN() error {
	return validateRootDir(paths.VarLibVPN, 0o755)
}

// createBaseServiceIdentities establishes the fresh-install
// daemon identity and data-directory boundary. Lifecycle initialization is
// intentionally earlier than this ledger step.
func createBaseServiceIdentities() error {
	if err := ensureRootOwnedVarLibVPN(); err != nil {
		return err
	}
	return host.CreateBaseDaemonIdentities()
}

func disableIPv6() error {
	content := `# Virtual Private Node — disable IPv6
net.ipv6.conf.all.disable_ipv6 = 1
net.ipv6.conf.default.disable_ipv6 = 1
net.ipv6.conf.lo.disable_ipv6 = 1
`
	if err := system.SudoWriteFile(
		paths.DisableIPv6Conf, []byte(content), 0644); err != nil {
		return err
	}
	return system.SudoRunSilent("sysctl", "--system")
}

var (
	observeSSHForFirewall = ObserveSSHState
	installUFWForFirewall = func() error {
		return system.SudoRun("apt-get", "install", "-y", "-qq", "ufw")
	}
	readUFWDefaultForFirewall = func() (string, error) {
		return system.SudoRunOutput("cat", paths.UFWDefault)
	}
	writeUFWDefaultForFirewall = func(data []byte) error {
		return system.SudoWriteFile(paths.UFWDefault, data, 0o644)
	}
	runFirewallCommand = func(args []string) error {
		return system.SudoRun(args[0], args[1:]...)
	}
)

// configureInitialFirewall establishes the node's global UFW baseline during
// initial installation. Post-install features must not call it: they add and
// verify only the rules they own through the narrow helpers below.
//
// The initial operation re-observes sshd immediately before any firewall
// mutation. Observation failure refuses the rewrite; there is no cached-port
// or port-22 fallback.
func configureInitialFirewall(cfg *config.AppConfig) error {
	if _, err := observeSSHForFirewall(); err != nil {
		return fmt.Errorf("observe SSH ports before firewall preparation: %w", err)
	}
	if err := installUFWForFirewall(); err != nil {
		return err
	}
	// Package installation can take long enough for sshd configuration to
	// change. Re-observe at the last responsible moment; only this fresh
	// answer is allowed to shape the rewrite below.
	obs, err := observeSSHForFirewall()
	if err != nil {
		return fmt.Errorf("observe SSH ports before firewall rewrite: %w", err)
	}

	ufwDefault, err := readUFWDefaultForFirewall()
	if err == nil {
		content := strings.ReplaceAll(
			ufwDefault, "IPV6=yes", "IPV6=no")
		if err := writeUFWDefaultForFirewall([]byte(content)); err != nil {
			return err
		}
	}

	commands := buildInitialFirewallCommands(cfg, obs.Ports)
	for _, args := range commands {
		if err := runFirewallCommand(args); err != nil {
			return err
		}
	}
	return nil
}

func buildInitialFirewallCommands(
	cfg *config.AppConfig, sshPorts []int,
) [][]string {
	commands := [][]string{
		{"ufw", "default", "deny", "incoming"},
		{"ufw", "default", "allow", "outgoing"},
	}
	for _, p := range sshPorts {
		commands = append(commands,
			[]string{"ufw", "allow",
				fmt.Sprintf("%d/tcp", p)})
	}

	if cfg.HasLND() && cfg.P2PMode == "hybrid" {
		commands = append(commands,
			[]string{"ufw", "allow", "9735/tcp"})
		commands = append(commands,
			[]string{"ufw", "allow", "8080/tcp"})
	}

	// Syncthing sync protocol — clearnet direct connection.
	// Mutual TLS with explicit device approval ensures only
	// paired devices can connect.
	if cfg.SyncthingEnabled {
		commands = append(commands,
			[]string{"ufw", "allow", "22000/tcp"})
	}

	commands = append(commands,
		[]string{"ufw", "--force", "enable"})

	return commands
}

func installUnattendedUpgrades() error {
	return system.SudoRun("apt-get", "install", "-y", "-qq",
		"unattended-upgrades", "apt-listchanges")
}

func configureUnattendedUpgrades() error {
	autoConf := `APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
APT::Periodic::AutocleanInterval "7";
`
	if err := system.SudoWriteFile(paths.AutoUpgrades,
		[]byte(autoConf), 0644); err != nil {
		return err
	}

	upgradeConf := `// Virtual Private Node — Unattended Upgrades
Unattended-Upgrade::Allowed-Origins {
    "${distro_id}:${distro_codename}-security";
};
Unattended-Upgrade::Automatic-Reboot "true";
Unattended-Upgrade::Automatic-Reboot-Time "04:00";
Unattended-Upgrade::Remove-Unused-Kernel-Packages "true";
Unattended-Upgrade::Remove-Unused-Dependencies "true";
`
	return system.SudoWriteFile(paths.UnattendedUpgrades,
		[]byte(upgradeConf), 0644)
}

func installFail2ban() error {
	return system.SudoRun("apt-get", "install",
		"-y", "-qq", "fail2ban")
}

func configureFail2ban() error {
	content := `# Virtual Private Node — Fail2ban
[sshd]
enabled = true
mode = aggressive
port = ssh
maxretry = 5
findtime = 600
bantime = 600
`
	if err := system.SudoWriteFile(paths.Fail2banJail,
		[]byte(content), 0644); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "enable",
		"fail2ban"); err != nil {
		return err
	}
	return system.SudoRun("systemctl", "restart", "fail2ban")
}

// The Tor routing check formerly here (logTorStatus, warn-only) is
// superseded by the hard-gate install step in torgate.go (IA-2-K).

// configureAptTor sets up apt to route all package downloads through
// Tor's SOCKS proxy. This ensures apt-get install/upgrade commands
// (GPG, PostgreSQL, Syncthing, fail2ban, unattended-upgrades) don't
// leak the server's IP to Debian mirrors or third-party repositories.
//
// The Acquire timeout and retry lines bound apt's DOWNLOAD phase —
// its kill-safe phase — which matters most over Tor, where a stalled
// circuit could otherwise hold a package operation (and with it the
// root helper's serialized queue) indefinitely. The INSTALL phase is
// deliberately left unbounded here: interrupting dpkg mid-transaction
// corrupts package state, which is exactly why the package-update
// budget is generous. (The pre-Tor apt operations run before this
// file exists and rely on apt's stock 120-second timeout default.)
func configureAptTor() error {
	content := `Acquire::http::Proxy "socks5h://127.0.0.1:9050";
Acquire::https::Proxy "socks5h://127.0.0.1:9050";
Acquire::http::Timeout "60";
Acquire::https::Timeout "60";
Acquire::Retries "3";
`
	return system.SudoWriteFile(paths.AptTorProxy,
		[]byte(content), 0644)
}
