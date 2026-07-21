// internal/installer/syncthing.go

package installer

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// downloadSyncthing fetches the pinned release tarball and its
// clearsigned checksum file from GitHub over Tor.
func downloadSyncthing(version, workDir string) error {
	filename := fmt.Sprintf(
		"syncthing-linux-amd64-v%s.tar.gz", version)
	url := fmt.Sprintf(
		"https://github.com/syncthing/syncthing/releases/download/v%s/%s",
		version, filename)
	ascURL := fmt.Sprintf(
		"https://github.com/syncthing/syncthing/releases/download/v%s/sha256sum.txt.asc",
		version)
	if err := system.DownloadRequireTor(
		url, filepath.Join(workDir, filename)); err != nil {
		return err
	}
	if err := system.DownloadRequireTor(ascURL,
		filepath.Join(workDir, "sha256sum.txt.asc")); err != nil {
		return fmt.Errorf("download Syncthing checksums: %w", err)
	}
	return nil
}

// extractAndInstallSyncthing unpacks the verified tarball and
// installs the binary to /usr/local/bin (LND pattern).
// Tarball layout (verified June 9 2026):
// syncthing-linux-amd64-v<ver>/syncthing
func extractAndInstallSyncthing(version, workDir string) error {
	filename := fmt.Sprintf(
		"syncthing-linux-amd64-v%s.tar.gz", version)
	if err := system.Run("tar", "-xzf",
		filepath.Join(workDir, filename),
		"-C", workDir); err != nil {
		return err
	}
	src := filepath.Join(workDir,
		fmt.Sprintf("syncthing-linux-amd64-v%s", version),
		"syncthing")
	return system.SudoRun("install", "-m", "0755",
		"-o", "root", "-g", "root",
		src, "/usr/local/bin/")
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

// writeSyncthingService writes the systemd unit for the pinned
// Syncthing binary. STNOUPGRADE=1 disables the binary's
// self-upgrader (the GitHub release binary is NOT built with
// [noupgrade], verified June 9 2026 — this env var plus
// autoUpgradeIntervalH=0 in the config are the two controls).
// STNODEFAULTFOLDER=1 prevents creation of the default ~/Sync
// folder on first run. --no-restart + Restart=on-failure keeps
// lifecycle ownership with systemd (existing posture).
func writeSyncthingService() error {
	content := fmt.Sprintf(`[Unit]
Description=Syncthing File Synchronization
After=network-online.target tor.service
Wants=network-online.target

[Service]
Type=simple
User=%s
Group=%s
Environment=STNOUPGRADE=1
Environment=STNODEFAULTFOLDER=1
ExecStart=/usr/local/bin/syncthing serve --no-browser --no-restart --config=/etc/syncthing --data=/var/lib/syncthing
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

// configureSyncthingAuth provisions Syncthing's identity and
// writes the complete authored config BEFORE first daemon start.
//
// Finding H history: the previous implementation round-tripped
// the generated config through Go structs carrying `,innerxml`,
// which re-emitted captured raw XML alongside the typed fields —
// duplicate <gui>/<options> blocks whose last-wins resolution
// kept the generate defaults, silently leaving discovery and
// relays ENABLED on every install. Struct round-trips are
// unsalvageable here (dedup either duplicates or drops unmodeled
// elements); the fix is to author the entire file from a
// template tied to the pinned Syncthing version, then verify it
// before the daemon ever starts.
func configureSyncthingAuth(password string) error {
	system.SudoRunSilent("chown",
		systemUser+":"+systemUser, paths.SyncthingDir)

	// 1. Crypto identity only: TLS cert/key + device ID. The
	//    generated config.xml is read for its identity values,
	//    then overwritten by the authored template. Explicit
	//    binary path — never PATH resolution — so a leftover
	//    apt-installed /usr/bin/syncthing can never be the one
	//    that generates the identity. runuser (util-linux)
	//    drops from root to the service user; this box has no
	//    sudo rules to borrow.
	if err := system.SudoRun("runuser", "-u", systemUser, "--",
		"/usr/local/bin/syncthing",
		"generate", "--home="+paths.SyncthingDir); err != nil {
		return fmt.Errorf("syncthing generate: %w", err)
	}

	// 2. Extract device ID, device name, and API key from the
	//    generated config. Read by exact path — `generate` can
	//    leave its own .syncthing.tmp.* scratch alongside.
	output, err := system.SudoRunOutput("cat",
		paths.SyncthingConfigXML)
	if err != nil {
		return fmt.Errorf("read generated config: %w", err)
	}

	type genDevice struct {
		ID   string `xml:"id,attr"`
		Name string `xml:"name,attr"`
	}
	type genGUI struct {
		APIKey string `xml:"apikey"`
	}
	type genConfig struct {
		XMLName xml.Name    `xml:"configuration"`
		Devices []genDevice `xml:"device"`
		GUI     genGUI      `xml:"gui"`
	}
	var gen genConfig
	if err := xml.Unmarshal([]byte(output), &gen); err != nil {
		return fmt.Errorf("parse generated config: %w", err)
	}
	if len(gen.Devices) == 0 || gen.Devices[0].ID == "" {
		return fmt.Errorf("generated config has no device ID")
	}
	if gen.GUI.APIKey == "" {
		return fmt.Errorf("generated config has no API key")
	}

	// 3. Hash the GUI password.
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// 4. Author the complete config and write it atomically.
	rendered := renderSyncthingConfig(
		gen.Devices[0].ID, gen.Devices[0].Name,
		gen.GUI.APIKey, string(hash))
	if err := system.SudoWriteFile(paths.SyncthingConfigXML,
		[]byte(rendered), 0640); err != nil {
		return err
	}
	if err := system.SudoRun("chown",
		systemUser+":"+systemUser,
		paths.SyncthingConfigXML); err != nil {
		return err
	}

	// 5. Self-verify gate — the daemon must never start with a
	//    config we have not verified.
	return verifySyncthingConfig()
}

// verifySyncthingConfig is the pre-start self-verify gate.
// Gate (a) — version tripwire: the installed binary must be the
// pinned version and the written config must carry the pinned
// schema version. When the pinned version is bumped in a future
// release, this fails until the template is re-reviewed against
// the new version's generate output — deliberately.
// Gate (b) — field check: every privacy field must be PRESENT
// with its exact intended value, with single <gui>/<options>
// blocks and a single listen address. An absent field means the
// schema assumption broke; refusing to start converts a silent
// leak into a loud install failure.
func verifySyncthingConfig() error {
	// (a) binary version
	verOut, err := system.RunContext(10*time.Second,
		"/usr/local/bin/syncthing", "--version")
	if err != nil {
		return fmt.Errorf("syncthing --version: %w", err)
	}
	if !strings.Contains(verOut, "syncthing v"+syncthingVersion+" ") {
		return fmt.Errorf(
			"version tripwire: installed Syncthing is not the "+
				"pinned v%s: %q", syncthingVersion,
			strings.TrimSpace(verOut))
	}

	// (b) written config
	content, err := system.SudoRunOutput("cat",
		paths.SyncthingConfigXML)
	if err != nil {
		return fmt.Errorf("re-read config: %w", err)
	}

	if !strings.Contains(content,
		`<configuration version="`+syncthingConfigSchema+`"`) {
		return fmt.Errorf(
			"version tripwire: config schema does not match "+
				"the pinned schema version %s",
			syncthingConfigSchema)
	}

	required := []string{
		"<globalAnnounceEnabled>false</globalAnnounceEnabled>",
		"<localAnnounceEnabled>false</localAnnounceEnabled>",
		"<relaysEnabled>false</relaysEnabled>",
		"<natEnabled>false</natEnabled>",
		"<announceLANAddresses>false</announceLANAddresses>",
		"<crashReportingEnabled>false</crashReportingEnabled>",
		"<autoUpgradeIntervalH>0</autoUpgradeIntervalH>",
		"<urAccepted>-1</urAccepted>",
		"<listenAddress>tcp://0.0.0.0:22000</listenAddress>",
	}
	for _, want := range required {
		if !strings.Contains(content, want) {
			return fmt.Errorf(
				"config self-verify failed: %s missing or wrong "+
					"— refusing to start Syncthing", want)
		}
	}

	for tag, n := range map[string]int{
		"<gui ":           1,
		"<options>":       1,
		"<listenAddress>": 1,
	} {
		if got := strings.Count(content, tag); got != n {
			return fmt.Errorf(
				"config self-verify failed: %d %s blocks, want %d "+
					"— refusing to start Syncthing", got, tag, n)
		}
	}

	logger.Install("Syncthing config self-verify passed " +
		"(pinned version, schema, privacy fields)")
	return nil
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

// stopSyncthingFailSafe stops AND disables Syncthing. Used on
// the fail-safe paths (readiness timeout, privacy-confirm
// failure) so that a later reboot — including the automatic
// reboot from unattended-upgrades — cannot silently restart a
// daemon whose privacy state we could not verify. A retried
// install re-enables via startSyncthing.
func stopSyncthingFailSafe() {
	system.SudoRunSilent("systemctl", "stop", "syncthing")
	system.SudoRunSilent("systemctl", "disable", "syncthing")
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

	// Readiness probe. /rest/noauth/health is the documented
	// unauthenticated endpoint; the previous probe hit
	// /rest/system/status without an API key, got 403 on every
	// try, and spun the full 30s. Fail-safe: if the daemon never
	// readies, stop it and fail the install.
	ready := false
	for i := 0; i < 30; i++ {
		output, err := system.RunContext(3*time.Second,
			"curl", "-s", "-o", "/dev/null",
			"-w", "%{http_code}",
			"http://127.0.0.1:8384/rest/noauth/health")
		if err == nil && strings.TrimSpace(output) == "200" {
			ready = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !ready {
		stopSyncthingFailSafe()
		return fmt.Errorf(
			"syncthing did not become ready — stopped")
	}

	// Post-start confirmation (defense in depth): the RUNNING
	// daemon must report the privacy settings off. This is the
	// exact check that exposed finding H. Fail-safe on mismatch.
	return confirmSyncthingPrivacy()
}

// confirmSyncthingPrivacy reads the effective options from the
// running daemon and hard-fails (stopping Syncthing) if any
// announce/relay setting is enabled.
func confirmSyncthingPrivacy() error {
	apiKey, err := getSyncthingAPIKey()
	if err != nil {
		stopSyncthingFailSafe()
		return fmt.Errorf("privacy confirm: get API key: %w", err)
	}
	resp, err := syncthingAPIGet(apiKey, "/rest/config/options")
	if err != nil {
		stopSyncthingFailSafe()
		return fmt.Errorf("privacy confirm: get options: %w", err)
	}

	var opts struct {
		GlobalAnnounceEnabled bool `json:"globalAnnounceEnabled"`
		LocalAnnounceEnabled  bool `json:"localAnnounceEnabled"`
		RelaysEnabled         bool `json:"relaysEnabled"`
		NATEnabled            bool `json:"natEnabled"`
	}
	if err := json.Unmarshal([]byte(resp), &opts); err != nil {
		stopSyncthingFailSafe()
		return fmt.Errorf("privacy confirm: parse options: %w", err)
	}
	if opts.GlobalAnnounceEnabled || opts.LocalAnnounceEnabled ||
		opts.RelaysEnabled || opts.NATEnabled {
		stopSyncthingFailSafe()
		return fmt.Errorf(
			"privacy confirm FAILED: running daemon reports "+
				"announce/relay enabled (global=%t local=%t "+
				"relays=%t nat=%t) — Syncthing stopped",
			opts.GlobalAnnounceEnabled, opts.LocalAnnounceEnabled,
			opts.RelaysEnabled, opts.NATEnabled)
	}
	logger.Install("Syncthing privacy confirmed on running daemon")
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

// GetSyncthingDeviceID returns this node's Syncthing device
// ID. As root it parses Syncthing's config XML (works even
// with the daemon stopped — the ID derives from the TLS cert);
// unprivileged it reads the staged copy on the board, which
// the install (and any reinstall) refreshes.
func GetSyncthingDeviceID() string {
	if os.Geteuid() != 0 {
		id, err := helper.ReadBoardString(
			paths.StateSyncthingDevID)
		if err != nil {
			logger.Status("syncthing device ID: %v", err)
			return ""
		}
		return id
	}

	output, err := os.ReadFile(paths.SyncthingConfigXML)
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
	if xml.Unmarshal(output, &c) != nil {
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
        "autoAcceptFolders": false
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

// getSyncthingAPIKey returns the REST API key. As root it
// parses Syncthing's config; unprivileged it reads the staged
// board copy — which is what makes every runtime device
// operation (pair, unpair, folder share) plain localhost REST
// with no privilege involved.
func getSyncthingAPIKey() (string, error) {
	if os.Geteuid() != 0 {
		return helper.ReadBoardString(paths.StateSyncthingAPIKey)
	}

	output, err := os.ReadFile(paths.SyncthingConfigXML)
	if err != nil {
		return "", err
	}

	type guiKey struct {
		APIKey string `xml:"apikey"`
	}
	type cfgFile struct {
		XMLName xml.Name `xml:"configuration"`
		GUI     guiKey   `xml:"gui"`
	}
	var cfg cfgFile
	if err := xml.Unmarshal(output, &cfg); err != nil {
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

func syncthingAPIPatch(apiKey, endpoint, body string) error {
	_, err := system.RunContext(10*time.Second,
		"curl", "-s",
		"-X", "PATCH",
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

			// PATCH only the devices array. The previous PUT sent
			// a 3-field object (id/path/devices) with no "type" —
			// Syncthing's PUT replaces the WHOLE folder, so
			// type:"sendonly" reverted to "sendreceive" on every
			// pairing (finding P, reproduced live).
			//
			// WARNING: PATCH replaces child arrays WHOLESALE (v2
			// Config Endpoints docs) — this MUST send the complete
			// merged devices array (existing + new), never a
			// single-element array, or the local device is wiped
			// and the folder de-shared.
			devices, err := json.Marshal(folders[i].Devices)
			if err != nil {
				return err
			}
			return syncthingAPIPatch(apiKey,
				"/rest/config/folders/"+f.ID,
				`{"devices":`+string(devices)+`}`)
		}
	}

	return fmt.Errorf("backup folder not found")
}
