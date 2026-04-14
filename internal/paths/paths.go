// internal/paths/paths.go

// Package paths centralizes all filesystem paths used by rlvpn.
// Every hardcoded path in the project should be defined here.
package paths

import "fmt"

// ── Configuration ────────────────────────────────────────

const (
	ConfigDir  = "/etc/rlvpn"
	ConfigFile = "/etc/rlvpn/config.json"

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
	BitcoindService    = "/etc/systemd/system/bitcoind.service"
	LNDService         = "/etc/systemd/system/lnd.service"
	SyncthingService   = "/etc/systemd/system/syncthing.service"
	BackupWatchPath    = "/etc/systemd/system/lnd-backup-watch.path"
	BackupCopyService  = "/etc/systemd/system/lnd-backup-copy.service"
	LndHubProxyService = "/etc/systemd/system/lndhub-proxy.service"
)

// ── Logs ─────────────────────────────────────────────────

const (
	LogFile = "/var/log/rlvpn.log"
)

// ── System ───────────────────────────────────────────────

const (
	SyncthingConfigXML = "/etc/syncthing/config.xml"
	UFWDefault         = "/etc/default/ufw"
	SSHDConfig         = "/etc/ssh/sshd_config"
	// SSHDDropIn uses a 00- prefix so it is parsed before
	// other drop-ins (notably 50-cloud-init.conf which
	// declares PasswordAuthentication yes on cloud
	// images). sshd's first-match-wins semantics mean
	// loading first = winning.
	SSHDDropIn          = "/etc/ssh/sshd_config.d/00-rlvpn-hardening.conf"
	Fail2banJail        = "/etc/fail2ban/jail.local"
	AutoUpgrades        = "/etc/apt/apt.conf.d/20auto-upgrades"
	UnattendedUpgrades  = "/etc/apt/apt.conf.d/50unattended-upgrades"
	DisableIPv6Conf     = "/etc/sysctl.d/99-disable-ipv6.conf"
	SyncthingKeyring    = "/etc/apt/keyrings/syncthing-archive-keyring.gpg"
	SyncthingSourceList = "/etc/apt/sources.list.d/syncthing.list"
)

// ── LndHub ───────────────────────────────────────────────

const (
	LndHubDir          = "/etc/lndhub"
	LndHubEnv          = "/etc/lndhub/lndhub.env"
	LndHubDataDir      = "/var/lib/lndhub"
	LndHubService      = "/etc/systemd/system/lndhub.service"
	LndHubMacaroon     = "/var/lib/lnd/lndhub.macaroon"
	TorLndHub          = "/var/lib/tor/lndhub"
	TorLndHubHostname  = "/var/lib/tor/lndhub/hostname"
	LndHubInternalPort = "3004"
	LndHubExternalPort = "3000"
)

// ── LndHub Proxy ─────────────────────────────────────────

const (
	LndHubProxyCert = "/etc/lndhub/proxy-tls.cert"
	LndHubProxyKey  = "/etc/lndhub/proxy-tls.key"
)

// ── User ─────────────────────────────────────────────────

const (
	AdminUser          = "ripsline"
	AdminBashrc        = "/home/ripsline/.bashrc"
	AuthorizedKeysFile = "/home/" + AdminUser + "/.ssh/authorized_keys"
)

// ── Cache ────────────────────────────────────────────────

const (
	VersionCacheDir  = "/home/ripsline/.cache/rlvpn"
	VersionCacheFile = "/home/ripsline/.cache/rlvpn/latest-version"
)
