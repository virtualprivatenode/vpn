// internal/installer/tor_test.go

package installer

import (
	"strings"
	"testing"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/paths"
)

func TestTorConfigBitcoinOnly(t *testing.T) {
	cfg := config.Default()
	content := BuildTorConfig(cfg)

	required := []string{
		"SOCKSPort 9050",
		"bitcoin-p2p",
		"HiddenServicePort 8333",
	}
	for _, req := range required {
		if !strings.Contains(content, req) {
			t.Errorf("missing %q in bitcoin-only torrc", req)
		}
	}

	forbidden := []string{
		"ControlPort",
		"bitcoin-rpc",
		"lnd-rest",
		"lnd-grpc",
		"syncthing",
	}
	for _, f := range forbidden {
		if strings.Contains(content, f) {
			t.Errorf("bitcoin-only torrc should not contain %q", f)
		}
	}
}

func TestTorConfigWithLND(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	content := BuildTorConfig(cfg)

	required := []string{
		"SOCKSPort 9050",
		"ControlPort 9051",
		"CookieAuthentication 1",
		"CookieAuthFileGroupReadable 1",
		"bitcoin-p2p",
		"lnd-grpc",
		"lnd-rest",
		"HiddenServicePort 10009",
		"HiddenServicePort 8080",
	}
	for _, req := range required {
		if !strings.Contains(content, req) {
			t.Errorf("missing %q in LND torrc", req)
		}
	}
}

func TestTorConfigWithSyncthing(t *testing.T) {
	cfg := config.Default()
	cfg.SyncthingInstalled = true
	content := BuildTorConfig(cfg)

	// Web UI still accessible over Tor
	if !strings.Contains(content, "syncthing") {
		t.Error("missing syncthing hidden service")
	}
	if !strings.Contains(content, "HiddenServicePort 8384") {
		t.Error("missing Syncthing web UI port")
	}

	// Sync protocol goes over clearnet — no hidden service
	if strings.Contains(content, "syncthing-sync") {
		t.Error("should not have syncthing-sync hidden service")
	}
	if strings.Contains(content, "HiddenServicePort 22000") {
		t.Error("should not have port 22000 hidden service")
	}
}

func TestTorConfigNoSyncthingWithoutInstall(t *testing.T) {
	cfg := config.Default()
	content := BuildTorConfig(cfg)

	if strings.Contains(content, "syncthing") {
		t.Error("should not have syncthing without install")
	}
}

func TestTorConfigFullStack(t *testing.T) {
	cfg := &config.AppConfig{
		Network:            "mainnet",
		LNDInstalled:       true,
		SyncthingInstalled: true,
	}
	content := BuildTorConfig(cfg)

	required := []string{
		"SOCKSPort 9050",
		"ControlPort 9051",
		"bitcoin-p2p",
		"lnd-grpc",
		"lnd-rest",
		"syncthing",
		"HiddenServicePort 8384",
	}
	for _, req := range required {
		if !strings.Contains(content, req) {
			t.Errorf("full stack torrc missing %q", req)
		}
	}

	// Sync protocol over clearnet, not Tor
	if strings.Contains(content, "syncthing-sync") {
		t.Error("full stack should not have syncthing-sync hidden service")
	}
}

func TestTorConfigMainnetPorts(t *testing.T) {
	cfg := config.Default()
	content := BuildTorConfig(cfg)

	if !strings.Contains(content, "HiddenServicePort 8333") {
		t.Error("mainnet torrc should use port 8333 for P2P")
	}
	if strings.Contains(content, "HiddenServicePort 8332") {
		t.Error("mainnet torrc should not have RPC hidden service")
	}
}

func TestTorConfigTestnet4Ports(t *testing.T) {
	cfg := &config.AppConfig{Network: "testnet4"}
	content := BuildTorConfig(cfg)

	if !strings.Contains(content, "HiddenServicePort 48333") {
		t.Error("testnet4 torrc should use port 48333 for P2P")
	}
	if strings.Contains(content, "HiddenServicePort 48332") {
		t.Error("testnet4 torrc should not have RPC hidden service")
	}
}

func TestTorConfigNoControlPortWithoutLND(t *testing.T) {
	cfg := config.Default()
	content := BuildTorConfig(cfg)

	if strings.Contains(content, "ControlPort") {
		t.Error("should not have ControlPort without LND")
	}
}

func TestTorConfigWithLndHub(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	cfg.LndHubInstalled = true
	content := BuildTorConfig(cfg)

	required := []string{
		"lndhub",
		"HiddenServicePort " + paths.LndHubExternalPort,
	}
	for _, req := range required {
		if !strings.Contains(content, req) {
			t.Errorf("missing %q in LndHub torrc", req)
		}
	}

	// Verify Tor points to internal port
	if !strings.Contains(content,
		"127.0.0.1:"+paths.LndHubInternalPort) {
		t.Error("LndHub hidden service should point to internal port " +
			paths.LndHubInternalPort)
	}
}

func TestTorConfigNoLndHubWithoutInstall(t *testing.T) {
	cfg := config.Default()
	cfg.LNDInstalled = true
	content := BuildTorConfig(cfg)

	if strings.Contains(content, "lndhub") {
		t.Error("should not have lndhub without install")
	}
}

func TestTorConfigFullStackWithLndHub(t *testing.T) {
	cfg := &config.AppConfig{
		Network:            "mainnet",
		LNDInstalled:       true,
		SyncthingInstalled: true,
		LndHubInstalled:    true,
	}
	content := BuildTorConfig(cfg)

	required := []string{
		"SOCKSPort 9050",
		"ControlPort 9051",
		"bitcoin-p2p",
		"lnd-grpc",
		"lnd-rest",
		"syncthing",
		"HiddenServicePort 8384",
		"lndhub",
		"HiddenServicePort " + paths.LndHubExternalPort,
		"127.0.0.1:" + paths.LndHubInternalPort,
	}
	for _, req := range required {
		if !strings.Contains(content, req) {
			t.Errorf("full stack with lndhub torrc missing %q", req)
		}
	}

	// No sync hidden service
	if strings.Contains(content, "syncthing-sync") {
		t.Error("should not have syncthing-sync hidden service")
	}
}

func TestTorConfigLndHubInternalPort(t *testing.T) {
	cfg := &config.AppConfig{
		Network:         "mainnet",
		LNDInstalled:    true,
		LndHubInstalled: true,
	}
	content := BuildTorConfig(cfg)

	// Should NOT point directly to external port on localhost
	if strings.Contains(content,
		"127.0.0.1:"+paths.LndHubExternalPort) {
		t.Error("Tor should point to internal port, not external port")
	}
}
