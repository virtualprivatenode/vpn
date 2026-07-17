// internal/paths/paths.go

// Package paths centralizes all filesystem paths used by vpn.
// Every hardcoded path in the project should be defined here.
package paths

import "fmt"

// ── Configuration ────────────────────────────────────────

const (
	ConfigDir  = "/etc/vpn"
	ConfigFile = "/etc/vpn/config.json"

	// InstallStateFile is the per-step install ledger: which
	// install steps have completed, keyed by stable step key
	// (installer/ledger.go). A SEPARATE file from config.json
	// so a config load failure cannot erase install history,
	// and so its ownership flips to root with root-dispatched
	// install without touching config's story.
	InstallStateFile = "/etc/vpn/install-state.json"

	BitcoinConf = "/etc/bitcoin/bitcoin.conf"
	BitcoinDir  = "/etc/bitcoin"

	LNDConf = "/etc/lnd/lnd.conf"
	LNDDir  = "/etc/lnd"

	SyncthingDir = "/etc/syncthing"
)

// ── Data ─────────────────────────────────────────────────

const (
	BitcoinDataDir   = "/var/lib/bitcoin"
	LNDDataDir       = "/var/lib/lnd"
	SyncthingDataDir = "/var/lib/syncthing"
	SyncthingBackup  = "/var/lib/syncthing/lnd-backup"
)

// ── LND files ────────────────────────────────────────────

const (
	LNDTLSCert        = "/var/lib/lnd/tls.cert"
	LNDTLSKey         = "/var/lib/lnd/tls.key"
	LNDWalletPassword = "/var/lib/lnd/wallet_password"
)

// LNDMacaroon returns the path to the admin macaroon for a given network.
func LNDMacaroon(network string) string {
	return fmt.Sprintf("/var/lib/lnd/data/chain/bitcoin/%s/admin.macaroon", network)
}

// LNDCookiePath returns the cookie path relative to bitcoin datadir.
func LNDCookiePath(cookieSuffix string) string {
	return fmt.Sprintf("%s/%s", BitcoinDataDir, cookieSuffix)
}

// ChannelBackup returns the path to the channel backup for a given network.
func ChannelBackup(network string) string {
	return fmt.Sprintf("/var/lib/lnd/data/chain/bitcoin/%s/channel.backup", network)
}

// ── Tor ──────────────────────────────────────────────────

const (
	Torrc                = "/etc/tor/torrc"
	TorBitcoinP2P        = "/var/lib/tor/bitcoin-p2p"
	TorLNDGRPC           = "/var/lib/tor/lnd-grpc"
	TorLNDREST           = "/var/lib/tor/lnd-rest"
	TorLNDRESTHostname   = "/var/lib/tor/lnd-rest/hostname"
	TorSyncthing         = "/var/lib/tor/syncthing"
	TorSyncthingHostname = "/var/lib/tor/syncthing/hostname"
	TorSyncthingSync     = "/var/lib/tor/syncthing-sync"
)

// ── Systemd ──────────────────────────────────────────────

const (
	BitcoindService   = "/etc/systemd/system/bitcoind.service"
	LNDService        = "/etc/systemd/system/lnd.service"
	SyncthingService  = "/etc/systemd/system/syncthing.service"
	BackupWatchPath   = "/etc/systemd/system/lnd-backup-watch.path"
	BackupCopyService = "/etc/systemd/system/lnd-backup-copy.service"
)

// ── Logs ─────────────────────────────────────────────────

const (
	LogFile = "/var/log/vpn.log"
)

// ── System ───────────────────────────────────────────────

const (
	OSRelease   = "/etc/os-release"
	SudoersFile = "/etc/sudoers"
	SudoersDir  = "/etc/sudoers.d"

	SyncthingConfigXML = "/etc/syncthing/config.xml"
	UFWDefault         = "/etc/default/ufw"
	SSHDConfig         = "/etc/ssh/sshd_config"
	// SSHDDropIn uses a 00- prefix so it is parsed before
	// other drop-ins (notably 50-cloud-init.conf which
	// declares PasswordAuthentication yes on cloud
	// images). sshd's first-match-wins semantics mean
	// loading first = winning.
	SSHDDropIn = "/etc/ssh/sshd_config.d/00-vpn-hardening.conf"

	// OldSSHDDropIn is the drop-in filename from before the
	// rlvpn → vpn rename. On a migrated box the stale file
	// would sort BEFORE SSHDDropIn (r < v) and win every
	// contested directive under first-match-wins, so the
	// install SSH step deletes it — the ONLY old-name
	// artifact the installer removes (ruling xv: everything
	// else old survives until the operator's verified
	// teardown). Ordering is binding: observe → write new →
	// delete old → validate → restart, because a
	// TUI-disabled PasswordAuthentication lives in THIS
	// file until the observed value is carried into the
	// new one.
	OldSSHDDropIn = "/etc/ssh/sshd_config.d/00-rlvpn-hardening.conf"

	Fail2banJail       = "/etc/fail2ban/jail.local"
	AutoUpgrades       = "/etc/apt/apt.conf.d/20auto-upgrades"
	UnattendedUpgrades = "/etc/apt/apt.conf.d/50unattended-upgrades"
	DisableIPv6Conf    = "/etc/sysctl.d/99-disable-ipv6.conf"
)

// ── User ─────────────────────────────────────────────────

const (
	// AdminUser is the node's admin login — same name as the
	// binary, one name to know (ruling vi: clean break from
	// the old ripsline user; migrated boxes retire the old
	// user via MIGRATION.md's operator-run teardown, never
	// via this binary).
	AdminUser          = "vpn"
	AdminHome          = "/home/" + AdminUser
	AdminBashrc        = AdminHome + "/.bashrc"
	AdminBashProfile   = AdminHome + "/.bash_profile"
	AuthorizedKeysFile = AdminHome + "/.ssh/authorized_keys"

	// AdminSudoers is the NOPASSWD rule the installer writes
	// for the admin user. Commit 7 (root helper) deletes it —
	// nothing replaces it (ruling iii: zero sudoers residue).
	AdminSudoers = "/etc/sudoers.d/" + AdminUser

	// BinaryPath is where the installer places the running
	// binary (and where self-update installs new ones).
	BinaryPath = "/usr/local/bin/vpn"
)

// ── Cache ────────────────────────────────────────────────

const (
	VersionCacheDir  = AdminHome + "/.cache/vpn"
	VersionCacheFile = VersionCacheDir + "/latest-version"
)
