// internal/config/config.go

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/virtualprivatenode/vpn/internal/paths"
)

// Defaults — used by production code
const (
	DefaultDir  = paths.ConfigDir
	DefaultPath = paths.ConfigFile
)

// AppConfig holds the application state persisted to disk.
//
// Security note: passwords (SyncthingPassword) are stored in plaintext. This is a
// deliberate tradeoff for a single-user dedicated node. The config file
// has 0600 permissions, and the machine runs a single non-root user.
// Alternatives (OS keyring, encrypted vault) add complexity without
// meaningful security benefit on a dedicated node where the attacker
// model is remote access, not local privilege escalation.
type AppConfig struct {
	InstallComplete    bool              `json:"install_complete"`
	InstallVersion     string            `json:"install_version,omitempty"`
	Network            string            `json:"network"`
	Components         string            `json:"components"`
	PruneSize          int               `json:"prune_size"`
	P2PMode            string            `json:"p2p_mode"`
	AutoUnlock         bool              `json:"auto_unlock"`
	LNDInstalled       bool              `json:"lnd_installed"`
	WalletCreated      bool              `json:"wallet_created"`
	SyncthingInstalled bool              `json:"syncthing_installed"`
	SyncthingPassword  string            `json:"syncthing_password,omitempty"`
	SyncthingDevices   []SyncthingDevice `json:"syncthing_devices,omitempty"`
	Theme              string            `json:"theme,omitempty"`

	// DbCache is bitcoind's dbcache in MB, chosen at the
	// install hardware-fit step (ruling viii) from detected
	// RAM. Zero means "never set" — DbCacheMB() falls back
	// to the historical 512 so configs from older installs
	// keep their exact behavior.
	DbCache int `json:"dbcache,omitempty"`

	// SSHPorts holds sshd's OBSERVED listening ports,
	// recorded at install preflight (ruling xvi(b)). The
	// firewall allow-rules derive from this — never from a
	// hardcoded 22, which on a nonstandard-port box was a
	// delayed silent lockout hazard (deny-all enabled with
	// only 22 open, next connection refused). Empty means
	// "never observed" (pre-rename configs):
	// SSHPortsOrDefault falls back to 22, byte-identical to
	// the old behavior.
	SSHPorts []int `json:"ssh_ports,omitempty"`

	// KeyVerificationPending is set at install completion
	// and cleared when the TUI observes a real sshd login
	// for the admin user (journal evidence). While set, the
	// TUI shows the first-run banner asking the operator to
	// verify access from a SECOND terminal before trusting
	// the in-session handoff console as their only way in.
	KeyVerificationPending bool `json:"key_verification_pending,omitempty"`

	// SSHPasswordAuthDisabled mirrors the value
	// 00-vpn-hardening.conf writes for sshd's
	// PasswordAuthentication directive. False = password
	// auth enabled (matches debian default and the
	// bootstrap-written drop-in, which is silent on the
	// directive). True = password auth disabled by the
	// TUI's SSH Password Auth screen.
	SSHPasswordAuthDisabled bool `json:"ssh_password_auth_disabled,omitempty"`
}

type SyncthingDevice struct {
	Name     string `json:"name"`
	DeviceID string `json:"device_id"`
	PairedAt string `json:"paired_at"`
}

// Store handles reading/writing config to disk.
type Store struct {
	Dir  string
	Path string
}

func DefaultStore() *Store {
	return &Store{Dir: DefaultDir, Path: DefaultPath}
}

func Default() *AppConfig {
	return &AppConfig{
		Network:    "mainnet",
		Components: "bitcoin",
		PruneSize:  25,
		P2PMode:    "tor",
	}
}

func (s *Store) Load() (*AppConfig, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, err
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if err := ValidateNetwork(cfg.Network); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config to disk atomically.
// Writes to a temp file in the same directory, fsyncs, then renames.
// This ensures the config file is never partially written.
func (s *Store) Save(cfg *AppConfig) error {
	if err := os.MkdirAll(s.Dir, 0750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	tmp.Close()

	return os.Rename(tmpPath, s.Path)
}

func Load() (*AppConfig, error) {
	return DefaultStore().Load()
}

// RawFieldPresent reports whether the config file on disk
// contains the given JSON key at the top level. Needed where
// "absent" and "zero value" must be told apart on omitempty
// fields — the migration rule (commit-6 addendum 2026-07-17):
// an install seeds a field from observation ONLY when the
// operator's carried-over config never answered it; a present
// value is a prior answer and is never clobbered. A missing or
// unreadable file simply reports absent.
func RawFieldPresent(key string) bool {
	data, err := os.ReadFile(DefaultPath)
	if err != nil {
		return false
	}
	return rawFieldPresent(data, key)
}

// rawFieldPresent is the pure half of RawFieldPresent —
// unit-tested.
func rawFieldPresent(data []byte, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func Save(cfg *AppConfig) error {
	return DefaultStore().Save(cfg)
}

func (c *AppConfig) HasLND() bool {
	return c.LNDInstalled
}

// DbCacheMB returns the dbcache value for bitcoin.conf: the
// hardware-fit choice when one was recorded, else the
// historical default (512), so configs that predate the
// hardware-fit step render byte-identical bitcoin.conf.
func (c *AppConfig) DbCacheMB() int {
	if c.DbCache > 0 {
		return c.DbCache
	}
	return 512
}

// SSHPortsOrDefault returns the observed sshd ports for
// firewall allow-rules, falling back to 22 when no
// observation was ever recorded (pre-rename configs).
func (c *AppConfig) SSHPortsOrDefault() []int {
	if len(c.SSHPorts) > 0 {
		return c.SSHPorts
	}
	return []int{22}
}

func (c *AppConfig) IsMainnet() bool {
	return c.Network == "mainnet"
}

func (c *AppConfig) WalletExists() bool {
	return c.WalletCreated
}

func (c *AppConfig) NetworkConfig() *NetworkConfig {
	return NetworkConfigFromName(c.Network)
}

// ── State mutations ──────────────────────────────────────
//
// Named methods on *AppConfig for state transitions that
// callers otherwise inline. Each one encapsulates the
// "construct a record, append/update/remove in slice"
// operation so callers can't accidentally forget a field
// or reach into slice internals.
//
// These methods do NOT call Save — persistence is the
// caller's responsibility. The split keeps mutation and
// persistence composable (e.g. two mutations then one
// save, or a mutation applied speculatively then reverted
// without a disk write).
//
// See go-style-review.md Q4 and design-decisions.md for
// the rationale behind this pattern.

// AddSyncthingDevice appends a new device record with an
// auto-generated Name ("Device N" where N is the new
// device's 1-indexed position) and today's date.
func (c *AppConfig) AddSyncthingDevice(deviceID string) {
	c.SyncthingDevices = append(c.SyncthingDevices,
		SyncthingDevice{
			Name: fmt.Sprintf("Device %d",
				len(c.SyncthingDevices)+1),
			DeviceID: deviceID,
			PairedAt: time.Now().Format("2006-01-02"),
		})
}

// RemoveSyncthingDevice deletes the device with the given
// ID from the list. Returns true if a device was removed,
// false if no device had that ID. Caller uses the bool to
// decide whether to Save.
func (c *AppConfig) RemoveSyncthingDevice(deviceID string) bool {
	for i, d := range c.SyncthingDevices {
		if d.DeviceID == deviceID {
			c.SyncthingDevices = append(
				c.SyncthingDevices[:i],
				c.SyncthingDevices[i+1:]...)
			return true
		}
	}
	return false
}
