// internal/installer/syncthing.go

package installer

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
)

// ── Syncthing XML config types ───────────────────────────

type syncthingConfig struct {
	XMLName xml.Name      `xml:"configuration"`
	GUI     syncthingGUI  `xml:"gui"`
	Options syncthingOpts `xml:"options"`
	Rest    []byte        `xml:",innerxml"`
}

type syncthingGUI struct {
	Enabled               string `xml:"enabled,attr"`
	TLS                   string `xml:"tls,attr"`
	Address               string `xml:"address"`
	User                  string `xml:"user,omitempty"`
	Password              string `xml:"password,omitempty"`
	InsecureSkipHostcheck bool   `xml:"insecureSkipHostcheck"`
	APIKey                string `xml:"apikey,omitempty"`
	Theme                 string `xml:"theme,omitempty"`
}

type syncthingOpts struct {
	ListenAddresses       []string `xml:"listenAddress"`
	GlobalAnnounceEnabled bool     `xml:"globalAnnounceEnabled"`
	LocalAnnounceEnabled  bool     `xml:"localAnnounceEnabled"`
	RelaysEnabled         bool     `xml:"relaysEnabled"`
	NATEnabled            bool     `xml:"natEnabled"`
	Rest                  []byte   `xml:",innerxml"`
}

func installSyncthingRepo() error {
	system.SudoRun("mkdir", "-p", "/etc/apt/keyrings")

	tmpKey := "/tmp/syncthing-release-key.gpg"
	if err := system.DownloadRequireTor(
		"https://syncthing.net/release-key.gpg", tmpKey); err != nil {
		return err
	}
	defer os.Remove(tmpKey)
	if err := system.SudoRun("cp", tmpKey, paths.SyncthingKeyring); err != nil {
		return err
	}

	repoLine := `deb [signed-by=` + paths.SyncthingKeyring +
		`] https://apt.syncthing.net/ syncthing stable-v2`
	return system.SudoWriteFile(paths.SyncthingSourceList,
		[]byte(repoLine+"\n"), 0644)
}

func installSyncthingPackage() error {
	if err := system.SudoRun("apt-get", "update", "-qq"); err != nil {
		return err
	}
	return system.SudoRun("apt-get", "install", "-y", "-qq", "syncthing")
}

func createSyncthingDirs() error {
	dirs := []struct {
		path  string
		owner string
		mode  os.FileMode
	}{
		{paths.SyncthingDir, systemUser + ":" + systemUser, 0750},
		{paths.SyncthingDataDir, systemUser + ":" + systemUser, 0750},
		{paths.SyncthingBackup, systemUser + ":" + systemUser, 0750},
	}
	for _, d := range dirs {
		if err := system.SudoRun("mkdir", "-p", d.path); err != nil {
			return err
		}
		if err := system.SudoRun("chown", d.owner, d.path); err != nil {
			return err
		}
		if err := system.SudoRun("chmod",
			fmt.Sprintf("%o", d.mode), d.path); err != nil {
			return err
		}
	}
	return nil
}

func writeSyncthingService() error {
	content := fmt.Sprintf(`[Unit]
Description=Syncthing File Synchronization
After=network-online.target tor.service
Wants=network-online.target

[Service]
Type=simple
User=%s
Group=%s
ExecStart=/usr/bin/syncthing serve --no-browser --no-restart --config=/etc/syncthing --data=/var/lib/syncthing
Restart=on-failure
RestartSec=10
SuccessExitStatus=3 4
RestartForceExitStatus=3 4

[Install]
WantedBy=multi-user.target
`, systemUser, systemUser)
	return system.SudoWriteFile(paths.SyncthingService,
		[]byte(content), 0644)
}

func configureSyncthingAuth(password string) error {
	system.SudoRunSilent("chown",
		systemUser+":"+systemUser, paths.SyncthingDir)

	if err := system.SudoRun("-u", systemUser, "syncthing",
		"generate", "--home="+paths.SyncthingDir); err != nil {
		return fmt.Errorf("syncthing generate: %w", err)
	}

	configPath := paths.SyncthingConfigXML
	output, err := system.SudoRunOutput("cat", configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var cfg syncthingConfig
	if err := xml.Unmarshal([]byte(output), &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Configure GUI
	cfg.GUI.Address = "127.0.0.1:8384"
	cfg.GUI.User = "admin"
	cfg.GUI.Password = string(hash)
	cfg.GUI.InsecureSkipHostcheck = true

	// Disable discovery and relays — connections are direct only
	cfg.Options.GlobalAnnounceEnabled = false
	cfg.Options.LocalAnnounceEnabled = false
	cfg.Options.RelaysEnabled = false
	cfg.Options.NATEnabled = false

	// Listen on all interfaces for clearnet sync connections.
	// Syncthing uses mutual TLS — only pre-approved Device IDs
	// can establish a connection.
	cfg.Options.ListenAddresses = []string{"tcp://0.0.0.0:22000"}
	cfg.Options.Rest = nil

	// Marshal back
	xmlOutput, err := xml.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	xmlHeader := []byte(xml.Header)
	xmlOutput = append(xmlHeader, xmlOutput...)

	if err := system.SudoWriteFile(configPath, xmlOutput, 0640); err != nil {
		return err
	}
	return system.SudoRun("chown",
		systemUser+":"+systemUser, configPath)
}

func setupChannelBackupWatcher(cfg *config.AppConfig) error {
	network := cfg.Network
	if cfg.IsMainnet() {
		network = "mainnet"
	}
	backupSource := paths.ChannelBackup(network)
	backupDest := paths.SyncthingBackup + "/channel.backup"

	pathUnit := fmt.Sprintf(`[Unit]
Description=Watch LND channel backup

[Path]
PathChanged=%s
Unit=lnd-backup-copy.service

[Install]
WantedBy=multi-user.target
`, backupSource)
	if err := system.SudoWriteFile(paths.BackupWatchPath,
		[]byte(pathUnit), 0644); err != nil {
		return err
	}

	copyService := fmt.Sprintf(`[Unit]
Description=Copy LND channel backup

[Service]
Type=oneshot
User=%s
ExecStart=/bin/cp %s %s
`, systemUser, backupSource, backupDest)
	if err := system.SudoWriteFile(paths.BackupCopyService,
		[]byte(copyService), 0644); err != nil {
		return err
	}

	if err := system.SudoRun("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "enable",
		"lnd-backup-watch.path"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "start",
		"lnd-backup-watch.path"); err != nil {
		return err
	}

	// Copy existing backup if present
	system.SudoRunSilent("cp", backupSource, backupDest)
	system.SudoRunSilent("chown",
		systemUser+":"+systemUser, backupDest)
	return nil
}

func startSyncthing() error {
	if err := system.SudoRun("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "enable", "syncthing"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "start", "syncthing"); err != nil {
		return err
	}

	// Wait for Syncthing to become ready (lenient — continue regardless)
	for i := 0; i < 30; i++ {
		output, err := system.RunContext(3*time.Second,
			"curl", "-s", "-o", "/dev/null",
			"-w", "%{http_code}",
			"http://127.0.0.1:8384/rest/system/status")
		if err == nil && strings.TrimSpace(output) == "200" {
			break
		}
		time.Sleep(1 * time.Second)
	}

	return nil
}

// ── Syncthing Backup Folder Registration ─────────────────

// registerBackupFolder adds the lnd-backup directory as a
// Send Only folder in Syncthing so it can be shared with
// paired devices.
func registerBackupFolder() error {
	apiKey, err := getSyncthingAPIKey()
	if err != nil {
		return fmt.Errorf("get API key: %w", err)
	}

	// Check if folder already exists
	existing, err := syncthingAPIGet(apiKey,
		"/rest/config/folders")
	if err == nil && strings.Contains(existing, "lnd-backup") {
		return nil
	}

	// Get local device ID to include in folder config
	localID := GetSyncthingDeviceID()
	if localID == "" {
		return fmt.Errorf("cannot determine local device ID")
	}

	folder := fmt.Sprintf(`{
        "id": "lnd-backup",
        "label": "LND Channel Backup",
        "path": "%s",
        "type": "sendonly",
        "rescanIntervalS": 10,
        "fsWatcherEnabled": true,
        "fsWatcherDelayS": 1,
        "devices": [{"deviceID": %q}]
    }`, paths.SyncthingBackup, localID)

	if err := syncthingAPIPost(apiKey,
		"/rest/config/folders", folder); err != nil {
		return fmt.Errorf("register folder: %w", err)
	}

	logger.Install("Registered lnd-backup folder in Syncthing")
	return nil
}

// ── Syncthing Device Pairing ─────────────────────────────

// GetSyncthingDeviceID returns the VPS Syncthing Device ID
// by parsing the config XML. This works even when Syncthing
// is stopped since the ID is derived from the TLS certificate
// and stored in the config file.
func GetSyncthingDeviceID() string {
	output, err := system.SudoRunOutput("cat",
		paths.SyncthingConfigXML)
	if err != nil {
		return ""
	}

	type device struct {
		ID   string `xml:"id,attr"`
		Name string `xml:"name,attr"`
	}
	type syncCfg struct {
		XMLName xml.Name `xml:"configuration"`
		Devices []device `xml:"device"`
	}

	var c syncCfg
	if xml.Unmarshal([]byte(output), &c) != nil {
		return ""
	}

	if len(c.Devices) > 0 {
		return c.Devices[0].ID
	}
	return ""
}

// PairSyncthingDevice adds a remote device to Syncthing and
// shares the lnd-backup folder with it via the REST API.
func PairSyncthingDevice(deviceID string) error {
	apiKey, err := getSyncthingAPIKey()
	if err != nil {
		return fmt.Errorf("get API key: %w", err)
	}

	// Add the device
	devicePayload := fmt.Sprintf(`{
        "deviceID": %q,
        "name": "local-backup",
        "addresses": ["dynamic"],
        "autoAcceptFolders": true
    }`, deviceID)

	if err := syncthingAPIPost(apiKey,
		"/rest/config/devices", devicePayload); err != nil {
		return fmt.Errorf("add device: %w", err)
	}

	// Share the backup folder with the new device
	folderConfig, err := syncthingAPIGet(apiKey,
		"/rest/config/folders")
	if err != nil {
		return fmt.Errorf("get folders: %w", err)
	}

	if err := addDeviceToBackupFolder(apiKey,
		folderConfig, deviceID); err != nil {
		return fmt.Errorf("share folder: %w", err)
	}

	logger.Install("Paired Syncthing device: %s...",
		deviceID[:min(16, len(deviceID))])
	return nil
}

func getSyncthingAPIKey() (string, error) {
	output, err := system.SudoRunOutput("cat",
		paths.SyncthingConfigXML)
	if err != nil {
		return "", err
	}

	var cfg syncthingConfig
	if err := xml.Unmarshal([]byte(output), &cfg); err != nil {
		return "", err
	}
	if cfg.GUI.APIKey == "" {
		return "", fmt.Errorf("no API key found")
	}
	return cfg.GUI.APIKey, nil
}

func syncthingAPIPost(apiKey, endpoint, body string) error {
	_, err := system.RunContext(10*time.Second,
		"curl", "-s",
		"-X", "POST",
		"-H", "X-API-Key: "+apiKey,
		"-H", "Content-Type: application/json",
		"-d", body,
		"http://127.0.0.1:8384"+endpoint)
	return err
}

func syncthingAPIPut(apiKey, endpoint, body string) error {
	_, err := system.RunContext(10*time.Second,
		"curl", "-s",
		"-X", "PUT",
		"-H", "X-API-Key: "+apiKey,
		"-H", "Content-Type: application/json",
		"-d", body,
		"http://127.0.0.1:8384"+endpoint)
	return err
}

func syncthingAPIDelete(apiKey, endpoint string) error {
	_, err := system.RunContext(10*time.Second,
		"curl", "-s",
		"-X", "DELETE",
		"-H", "X-API-Key: "+apiKey,
		"http://127.0.0.1:8384"+endpoint)
	return err
}

// UnpairSyncthingDevice removes a device from Syncthing
// via the REST API. The folder sharing is dropped
// automatically when the device is removed.
func UnpairSyncthingDevice(deviceID string) error {
	apiKey, err := getSyncthingAPIKey()
	if err != nil {
		return fmt.Errorf("get API key: %w", err)
	}

	if err := syncthingAPIDelete(apiKey,
		"/rest/config/devices/"+deviceID); err != nil {
		return fmt.Errorf("remove device: %w", err)
	}

	logger.Install("Removed Syncthing device: %s...",
		deviceID[:min(16, len(deviceID))])
	return nil
}

func syncthingAPIGet(apiKey, endpoint string) (string, error) {
	return system.RunContext(10*time.Second,
		"curl", "-s",
		"-H", "X-API-Key: "+apiKey,
		"http://127.0.0.1:8384"+endpoint)
}

func addDeviceToBackupFolder(
	apiKey, foldersJSON, deviceID string,
) error {
	type folderDevice struct {
		DeviceID     string `json:"deviceID"`
		IntroducedBy string `json:"introducedBy,omitempty"`
	}
	type folder struct {
		ID      string         `json:"id"`
		Path    string         `json:"path"`
		Devices []folderDevice `json:"devices"`
	}

	var folders []folder
	if err := json.Unmarshal(
		[]byte(foldersJSON), &folders); err != nil {
		return err
	}

	for i, f := range folders {
		if f.Path == paths.SyncthingBackup ||
			f.Path == paths.SyncthingBackup+"/" {
			// Check if device already added
			for _, d := range f.Devices {
				if d.DeviceID == deviceID {
					return nil
				}
			}
			folders[i].Devices = append(
				folders[i].Devices,
				folderDevice{DeviceID: deviceID},
			)

			updated, err := json.Marshal(folders[i])
			if err != nil {
				return err
			}
			return syncthingAPIPut(apiKey,
				"/rest/config/folders/"+f.ID,
				string(updated))
		}
	}

	return fmt.Errorf("backup folder not found")
}
