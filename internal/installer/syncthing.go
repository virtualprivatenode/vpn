// internal/installer/syncthing.go

// Package installer provisions VPN and coordinates fresh or resumed installation.
package installer

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/host"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/syncthing"
	"github.com/virtualprivatenode/vpn/internal/system"
)

var syncthingResiduePaths = []string{
	paths.SyncthingBinary,
	paths.SyncthingDir,
	paths.SyncthingDataDir,
	paths.SyncthingService,
	paths.StateSyncthingAPIKey,
	paths.StateSyncthingWebPassword,
	paths.TorSyncthing,
	paths.TorSyncthingSync,
	paths.BackupWatchPath,
	paths.BackupExportService,
	paths.LNDBackupStage,
	paths.LNDBackupExport,
}

// SyncthingResiduePresent conservatively detects known add-on artifacts before
// a fresh install attempt. It does not classify, adopt, clean, or repair them;
// any evidence causes the helper to refuse without mutation so ADDON-001 can
// define recovery later.
func SyncthingResiduePresent() (bool, error) {
	for _, path := range syncthingResiduePaths {
		if _, err := os.Lstat(path); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, fmt.Errorf("inspect Syncthing residue %s: %w", path, err)
		}
	}
	if _, err := user.Lookup(syncthingUser); err == nil {
		return true, nil
	} else if _, ok := err.(user.UnknownUserError); !ok {
		return false, fmt.Errorf("inspect Syncthing user: %w", err)
	}
	if _, err := user.LookupGroup(backupGroup); err == nil {
		return true, nil
	} else if _, ok := err.(user.UnknownGroupError); !ok {
		return false, fmt.Errorf("inspect Syncthing backup group: %w", err)
	}
	return false, nil
}

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

type syncthingDirSpec struct {
	path  string
	owner string
	mode  os.FileMode
}

func syncthingDirSpecs() []syncthingDirSpec {
	return []syncthingDirSpec{
		{paths.SyncthingDir,
			syncthingUser + ":" + syncthingUser, 0700},
		{paths.SyncthingDataDir,
			syncthingUser + ":" + syncthingUser, 0700},
		// The project-owned parent is the only cross-service
		// boundary. Syncthing receives group traversal here, never
		// through its own private state.
		{paths.ExportDir,
			"root:" + backupGroup, 0750},
		// Only the lnd-run publisher can enter private staging.
		{paths.LNDBackupStage,
			lndUser + ":" + lndUser, 0700},
		// The publisher owns the final send-only folder. Syncthing
		// can traverse and read through the backup group but has no
		// write bit on the directory or published file.
		{paths.LNDBackupExport,
			lndUser + ":" + backupGroup, 0750},
		// Syncthing requires a marker in every folder it serves. A
		// custom, installer-owned marker lets it validate this folder
		// without receiving the write access used to create .stfolder.
		{paths.LNDBackupExportMarker,
			"root:" + backupGroup, 0750},
	}
}

func createSyncthingDirs() error {
	for _, d := range syncthingDirSpecs() {
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
func syncthingServiceUnit() string {
	return fmt.Sprintf(`[Unit]
Description=Syncthing File Synchronization
After=network-online.target tor.service
Wants=network-online.target

[Service]
Type=simple
User=%s
Group=%s
SupplementaryGroups=%s
Environment=STNOUPGRADE=1
Environment=STNODEFAULTFOLDER=1
ExecStart=/usr/local/bin/syncthing serve --no-browser --no-restart --config=/etc/syncthing --data=/var/lib/syncthing
Restart=on-failure
RestartSec=10
SuccessExitStatus=3 4
RestartForceExitStatus=3 4

[Install]
WantedBy=multi-user.target
`, syncthingUser, syncthingUser, backupGroup)
}

func writeSyncthingService() error {
	return system.SudoWriteFile(paths.SyncthingService,
		[]byte(syncthingServiceUnit()), 0644)
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
		syncthingUser+":"+syncthingUser, paths.SyncthingDir)

	// 1. Crypto identity only: TLS cert/key + device ID. The
	//    generated config.xml is read for its identity values,
	//    then overwritten by the authored template. Explicit
	//    binary path — never PATH resolution — so a leftover
	//    apt-installed /usr/bin/syncthing can never be the one
	//    that generates the identity. runuser (util-linux)
	//    drops from root to the service user; this box has no
	//    sudo rules to borrow.
	if err := system.SudoRun("runuser", "-u", syncthingUser, "--",
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
		syncthingUser+":"+syncthingUser,
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
	// (a) binary version. Probed through runuser as the service
	//     user, exactly like `generate` above: not for privilege
	//     but for environment. Syncthing v2 panics during
	//     package init when $HOME is undefined
	//     (lib/locations.userHomeDir), exiting 2 before any
	//     argument is parsed, and the root helper that runs this
	//     step is a socket-activated systemd service, which sets
	//     no $HOME. runuser sets $HOME from the passwd entry, so
	//     the probe works in any environment that can run the
	//     daemon itself.
	verOut, err := system.RunContext(10*time.Second,
		"runuser", "-u", syncthingUser, "--",
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
	profile, err := config.NetworkConfigFromName(network)
	if err != nil {
		return fmt.Errorf("configure LND backup publisher: %w", err)
	}

	pathUnit, exportService, err := channelBackupUnits(network)
	if err != nil {
		return err
	}
	if err := system.SudoWriteFile(paths.BackupWatchPath,
		[]byte(pathUnit), 0644); err != nil {
		return err
	}

	if err := system.SudoWriteFile(paths.BackupExportService,
		[]byte(exportService), 0644); err != nil {
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

	// If a backup already exists, copy it through the same
	// least-privilege unit that handles future changes. A node
	// without a wallet/channel backup yet is a normal no-op.
	if _, err := os.Stat(paths.ChannelBackup(profile.LNDNetwork)); err == nil {
		if err := system.SudoRun("systemctl", "start",
			"lnd-backup-export.service"); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect LND channel backup: %w", err)
	}
	return nil
}

// channelBackupUnits renders the watcher and the only service
// allowed to cross from LND state into the project-owned export.
// The normal lnd.service has no backup-group access, and Syncthing
// has no access to /var/lib/lnd or the private staging directory.
// The service accepts only the closed network selector; every
// filesystem path is fixed inside the publisher implementation.
func channelBackupUnits(network string) (
	pathUnit, exportService string, err error,
) {
	profile, err := config.NetworkConfigFromName(network)
	if err != nil {
		return "", "", fmt.Errorf(
			"render LND backup units: %w", err)
	}
	backupSource := paths.ChannelBackup(profile.LNDNetwork)
	pathUnit = fmt.Sprintf(`[Unit]
Description=Watch LND channel backup

[Path]
PathChanged=%s
Unit=lnd-backup-export.service

[Install]
WantedBy=multi-user.target
`, backupSource)

	exportService = fmt.Sprintf(`[Unit]
Description=Export LND channel backup

[Service]
Type=oneshot
User=%s
Group=%s
SupplementaryGroups=%s
UMask=0027
ExecStart=%s publish-lnd-backup %s
`, lndUser, lndUser, backupGroup, paths.BinaryPath, network)
	return pathUnit, exportService, nil
}

var syncthingServiceCommand = system.SudoRun

// stopSyncthingFailSafe attempts both stop and disable so an unverified daemon
// cannot silently return on reboot. Failures remain explicit; retained residue
// is not repaired by retrying installation.
func stopSyncthingFailSafe() error {
	stopErr := syncthingServiceCommand("systemctl", "stop", "syncthing")
	disableErr := syncthingServiceCommand("systemctl", "disable", "syncthing")
	return errors.Join(stopErr, disableErr)
}

func failSyncthingPrivacy(cause error) error {
	if err := stopSyncthingFailSafe(); err != nil {
		return errors.Join(cause, fmt.Errorf("could not confirm Syncthing stop/disable; administrator action is required: %w", err))
	}
	return fmt.Errorf("syncthing stopped and disabled: %w", cause)
}

func startSyncthing() error {
	if err := system.SudoRun("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "enable", "syncthing"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "start", "syncthing"); err != nil {
		return failSyncthingPrivacy(err)
	}

	// Health and privacy reads use the same loopback-only transport as runtime.
	ready := false
	probe := syncthing.NewClient("")
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, err := probe.Request(ctx, http.MethodGet, "/rest/noauth/health", "")
		cancel()
		if err == nil {
			ready = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !ready {
		return failSyncthingPrivacy(errors.New("syncthing did not become ready"))
	}

	// Verify the running daemon accepted the authored privacy settings.
	return confirmSyncthingPrivacy()
}

// confirmSyncthingPrivacy verifies effective network, reporting and GUI settings
// after startup. Missing or mismatched values trigger stop and disable.
func confirmSyncthingPrivacy() error {
	apiKey, err := getSyncthingAPIKey()
	if err == nil {
		err = syncthing.NewClient(apiKey).ConfirmPrivacy(context.Background())
	}
	if err != nil {
		return failSyncthingPrivacy(fmt.Errorf("privacy confirmation failed: %w", err))
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
	client := syncthing.NewClient(apiKey)

	// Check for the exact folder ID. Searching the raw JSON for
	// a substring can confuse an unrelated label or longer ID
	// for the required folder.
	existing, err := client.Request(context.Background(), http.MethodGet, "/rest/config/folders", "")
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}
	registered, err := backupFolderRegistered(existing)
	if err != nil {
		return fmt.Errorf("parse folders: %w", err)
	}
	if registered {
		return nil
	}

	// Get local device ID to include in folder config
	localID := host.SyncthingDeviceID()
	if localID == "" {
		return fmt.Errorf("cannot determine local device ID")
	}

	folder := renderBackupFolderConfig(localID)

	if _, err := client.Request(context.Background(), http.MethodPost, "/rest/config/folders", folder); err != nil {
		return fmt.Errorf("register folder: %w", err)
	}

	logger.Install("Registered lnd-backup folder in Syncthing")
	return nil
}

func renderBackupFolderConfig(localID string) string {
	return fmt.Sprintf(`{
        "id": "lnd-backup",
        "label": "LND Channel Backup",
        "path": %q,
        "type": "sendonly",
        "markerName": %q,
        "rescanIntervalS": 10,
        "fsWatcherEnabled": true,
        "fsWatcherDelayS": 1,
        "devices": [{"deviceID": %q}]
    }`, paths.LNDBackupExport, paths.ExportReadyMarkerName, localID)
}

func backupFolderRegistered(foldersJSON string) (bool, error) {
	var folders []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(foldersJSON), &folders); err != nil {
		return false, err
	}
	for _, folder := range folders {
		if folder.ID == "lnd-backup" {
			return true, nil
		}
	}
	return false, nil
}

// getSyncthingAPIKey reads the private configuration for root-side provisioning.
// Runtime workflows read their staged credentials in the application layer.
func getSyncthingAPIKey() (string, error) {
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
