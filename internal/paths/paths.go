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

	// PasswordPendingMarker exists while an unattended install
	// has applied a generated admin password that was never
	// displayed (the identity step applies early; the print
	// happens only at the end of a completed run — a failure in
	// between would otherwise strand a credential nobody has
	// seen). Written by the identity step on the unattended
	// path; cleared when the password is finally printed, or
	// when the operator sets a password of their own from the
	// node console. Holds no secret — its presence is the fact.
	PasswordPendingMarker = "/etc/vpn/password-pending"

	BitcoinConf = "/etc/bitcoin/bitcoin.conf"
	BitcoinDir  = "/etc/bitcoin"

	// StateDir is the staging board: root-written files that
	// carry privileged facts (onion hostnames, staged
	// credentials) to the unprivileged admin user. The
	// directory is root:vpn 0750; each file root:vpn 0640.
	// Root (the installer and the helper) writes; the admin
	// user reads. Every file is re-written by whatever
	// operation changes the fact it carries — a reader that
	// finds a file missing or unreadable reports the feature
	// unavailable and logs why, never guesses.
	//
	// The board lives under /var/lib/vpn — NOT under /etc/vpn
	// — deliberately: /etc/vpn is owned by the admin user (the
	// TUI writes config.json there), and a directory owner can
	// replace a subdirectory with a symlink. Root-side board
	// writes under an admin-owned parent would hand a
	// compromised admin account a "make root chown an
	// arbitrary directory" primitive. Every ancestor of the
	// board is root-owned, so that class cannot arise.
	VarLibVPN = "/var/lib/vpn"
	StateDir  = VarLibVPN + "/state"

	// Staging board files. One fact per file.
	StateBitcoindRPCPass = StateDir + "/bitcoind-rpc.pass"
	StateLNDTLSCert      = StateDir + "/lnd-tls.cert"
	StateLNDMacaroon     = StateDir + "/lnd-admin.macaroon"
	StateOnionBitcoinP2P = StateDir + "/onion-bitcoin-p2p"
	StateOnionLNDGRPC    = StateDir + "/onion-lnd-grpc"
	StateOnionLNDREST    = StateDir + "/onion-lnd-rest"
	StateOnionSyncthing  = StateDir + "/onion-syncthing"
	StateSyncthingAPIKey = StateDir + "/syncthing-api-key"
	StateSyncthingDevID  = StateDir + "/syncthing-device-id"
	StateSSHPasswordAuth = StateDir + "/ssh-password-auth"

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

	// The root helper's socket-activated units. The socket
	// node's ownership and mode (root:vpn 0660, created by
	// systemd before the helper ever runs) ARE the
	// authentication for privileged operations; the service
	// is started by traffic and exits when idle.
	HelperSocket         = "/run/vpn-helperd.sock"
	HelperSocketUnit     = "/etc/systemd/system/vpn-helperd.socket"
	HelperServiceUnit    = "/etc/systemd/system/vpn-helperd.service"
	HelperSocketUnitName = "vpn-helperd.socket"
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

	// AdminSudoers is where older builds granted the admin
	// user NOPASSWD sudo. The install now DELETES this file
	// and writes no replacement: the admin user has no sudo
	// rights at all. Privileged operations go through the
	// root helper's socket instead (vpn helperd), which
	// serves a fixed menu of typed operations — not a shell.
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
