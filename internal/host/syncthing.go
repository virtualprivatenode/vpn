package host

import (
	"strings"
	"time"

	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// SyncthingDeviceID reads the certificate identity with the pinned daemon's
// read-only command. Configuration device order does not identify the local node.
// runuser supplies the service HOME required by Syncthing package initialization.
func SyncthingDeviceID() string {
	output, err := system.RunContext(10*time.Second,
		"runuser", "-u", "syncthing", "--", paths.SyncthingBinary,
		"device-id", "--config="+paths.SyncthingDir,
		"--data="+paths.SyncthingDataDir)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}
