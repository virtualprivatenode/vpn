// internal/installer/tor_test.go

package installer

import (
	"strings"
	"testing"

	"github.com/ripsline/virtual-private-node/internal/config"
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
		// ControlPort is deliberately NOT forbidden here: it is
		// unconditional since the install routing gate consumes it
		// (see TestTorConfigControlPortAlways).
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

func TestTorConfigControlPortAlways(t *testing.T) {
	// The install-path routing gate (torgate.go) reads bootstrap
	// progress from the control port unconditionally, so every
	// generated torrc must include it — LND or not.
	cfg := config.Default()
	content := BuildTorConfig(cfg)

	if !strings.Contains(content, "ControlPort 9051") {
		t.Error("ControlPort must be present in every config (install gate depends on it)")
	}
	if !strings.Contains(content, "CookieAuthentication 1") {
		t.Error("control port must require cookie auth")
	}
}
