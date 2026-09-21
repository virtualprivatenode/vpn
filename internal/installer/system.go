package installer

import (
	"github.com/virtualprivatenode/vpn/internal/host"
	"github.com/virtualprivatenode/vpn/internal/paths"
)

// createBaseServiceIdentities revalidates the lifecycle-owned ancestor before
// provisioning. Only lifecycle initialization creates it; resume never repairs
// an unsafe object.
func createBaseServiceIdentities() error {
	if err := validateRootDir(paths.VarLibVPN, 0o755); err != nil {
		return err
	}
	return host.CreateBaseDaemonIdentities()
}
