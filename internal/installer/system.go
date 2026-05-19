// internal/installer/system.go

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
)

func checkOS() error {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return fmt.Errorf("cannot read /etc/os-release")
	}
	content := string(data)
	if !strings.Contains(content, "ID=debian") {
		return fmt.Errorf("requires Debian 13+")
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "VERSION_ID=") {
			ver := strings.Trim(
				strings.TrimPrefix(line, "VERSION_ID="), `"`)
			verNum, err := strconv.Atoi(ver)
			if err != nil {
				return fmt.Errorf(
					"cannot parse Debian version: %s", ver)
			}
			if verNum < 13 {
				return fmt.Errorf(
					"requires Debian 13+, found %s", ver)
			}
			return nil
		}
	}
	return fmt.Errorf("cannot determine Debian version")
}

func createSystemUser(username string) error {
	if _, err := user.Lookup(username); err == nil {
		return nil
	}
	return system.SudoRun("adduser",
		"--system", "--group",
		"--home", paths.BitcoinDataDir,
		"--shell", "/usr/sbin/nologin",
		username)
}

func createBitcoinDirs(username string) error {
	dirs := []struct {
		path  string
		owner string
		mode  os.FileMode
	}{
		{paths.BitcoinDir, "root:" + username, 0750},
		{paths.BitcoinDataDir, username + ":" + username, 0750},
	}
	for _, d := range dirs {
		if err := system.SudoRun("mkdir", "-p", d.path); err != nil {
			return fmt.Errorf("mkdir %s: %w", d.path, err)
		}
		if err := system.SudoRun("chown", d.owner, d.path); err != nil {
			return err
		}
		if err := system.SudoRun("chmod",
			fmt.Sprintf("%o", d.mode), d.path); err != nil {
			return fmt.Errorf("chmod %s: %w", d.path, err)
		}
	}
	return nil
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

func configureFirewall(cfg *config.AppConfig) error {
	if err := system.SudoRun("apt-get", "install",
		"-y", "-qq", "ufw"); err != nil {
		return err
	}

	ufwDefault, err := system.SudoRunOutput("cat", paths.UFWDefault)
	if err == nil {
		content := strings.ReplaceAll(
			ufwDefault, "IPV6=yes", "IPV6=no")
		system.SudoWriteFile(
			paths.UFWDefault, []byte(content), 0644)
	}

	commands := [][]string{
		{"ufw", "default", "deny", "incoming"},
		{"ufw", "default", "allow", "outgoing"},
		{"ufw", "allow", "22/tcp"},
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
	if cfg.SyncthingInstalled {
		commands = append(commands,
			[]string{"ufw", "allow", "22000/tcp"})
	}

	commands = append(commands,
		[]string{"ufw", "--force", "enable"})

	for _, args := range commands {
		if err := system.SudoRun(
			args[0], args[1:]...); err != nil {
			return err
		}
	}
	return nil
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

// logTorStatus verifies torsocks is available and Tor is routing
// traffic correctly. Logs the result for post-install audit.
// Returns nil even on failure — this is informational, not blocking.
func logTorStatus() error {
	if _, err := exec.LookPath("torsocks"); err != nil {
		logger.Install("WARNING: torsocks not found — downloads may use clearnet")
		return nil
	}
	logger.Install("torsocks available — downloads will route through Tor")

	confirmed := false
	for attempt := 0; attempt < 3; attempt++ {
		output, err := system.RunContext(15*time.Second,
			"torsocks", "curl", "-s", "--max-time", "10",
			"https://check.torproject.org/api/ip")
		if err == nil && strings.Contains(output, `"IsTor":true`) {
			logger.Install("Tor routing CONFIRMED via check.torproject.org")
			confirmed = true
			break
		}
		if attempt < 2 {
			time.Sleep(3 * time.Second)
		}
	}
	if !confirmed {
		logger.Install("WARNING: Tor routing check timed out after 3 attempts — verify with: torsocks curl -s https://check.torproject.org/api/ip")
	}
	return nil
}

// configureAptTor sets up apt to route all package downloads through
// Tor's SOCKS proxy. This ensures apt-get install/upgrade commands
// (GPG, PostgreSQL, Syncthing, fail2ban, unattended-upgrades) don't
// leak the server's IP to Debian mirrors or third-party repositories.
func configureAptTor() error {
	content := `Acquire::http::Proxy "socks5h://127.0.0.1:9050";
Acquire::https::Proxy "socks5h://127.0.0.1:9050";
`
	return system.SudoWriteFile("/etc/apt/apt.conf.d/99-tor-proxy",
		[]byte(content), 0644)
}
