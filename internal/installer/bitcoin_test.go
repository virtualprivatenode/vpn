//internal/installer/bitcoin_test.go

package installer

import (
	"strings"
	"testing"

	"github.com/virtualprivatenode/vpn/internal/bitcoin"
	"github.com/virtualprivatenode/vpn/internal/config"
)

func TestBitcoinConfigMainnet(t *testing.T) {
	cfg := config.Default()
	content := BuildBitcoinConfig(cfg, "")

	required := []string{
		"server=1",
		"disablewallet=1",
		"prune=25000",
		"proxy=127.0.0.1:9050",
		"rpcport=8332",
		"zmqpubrawblock=tcp://127.0.0.1:28332",
		"zmqpubrawtx=tcp://127.0.0.1:28333",
		"listen=1",
		"listenonion=1",
		"dbcache=512",
		"maxmempool=300",
		"bind=127.0.0.1",
		"rpcbind=127.0.0.1",
		"rpcallowip=127.0.0.1",
	}
	for _, req := range required {
		if !strings.Contains(content, req) {
			t.Errorf("mainnet config missing %q", req)
		}
	}

	if strings.Contains(content, "testnet4=1") {
		t.Error("mainnet config should not contain testnet4 flag")
	}
}

func TestBitcoinConfigTestnet4(t *testing.T) {
	cfg := &config.AppConfig{
		Network:   "testnet4",
		PruneSize: 25,
		P2PMode:   "tor",
	}
	content := BuildBitcoinConfig(cfg, "")

	required := []string{
		"testnet4=1",
		"prune=25000",
		"rpcport=48332",
		"zmqpubrawblock=tcp://127.0.0.1:28334",
		"zmqpubrawtx=tcp://127.0.0.1:28335",
		"[testnet4]",
	}
	for _, req := range required {
		if !strings.Contains(content, req) {
			t.Errorf("testnet4 config missing %q", req)
		}
	}
}

func TestBitcoinConfigAlwaysHasProxy(t *testing.T) {
	cfg := config.Default()
	content := BuildBitcoinConfig(cfg, "")
	if !strings.Contains(content, "proxy=127.0.0.1:9050") {
		t.Error("bitcoin config must always have Tor proxy")
	}
}

func TestBitcoinConfigAlwaysHasServer(t *testing.T) {
	cfg := config.Default()
	content := BuildBitcoinConfig(cfg, "")
	if !strings.Contains(content, "server=1") {
		t.Error("bitcoin config must always have server=1")
	}
}

func TestBitcoinConfigHeader(t *testing.T) {
	cfg := config.Default()
	content := BuildBitcoinConfig(cfg, "")
	if !strings.Contains(content, "Virtual Private Node") {
		t.Error("bitcoin config should have VPN header comment")
	}
}

func TestBitcoinConfigWalletDisabled(t *testing.T) {
	cfg := config.Default()
	content := BuildBitcoinConfig(cfg, "")
	if !strings.Contains(content, "disablewallet=1") {
		t.Error("bitcoin config must have disablewallet=1")
	}
}

// The rpcauth credential line must land in the GLOBAL section:
// auth options are not network-scoped, and on testnet4 a line
// appended at the end would fall inside [testnet4].
func TestBitcoinConfigRPCAuthPlacement(t *testing.T) {
	line := "rpcauth=vpn:aabb$ccdd"

	cfg := config.Default()
	if got := BuildBitcoinConfig(cfg, line); !strings.Contains(
		got, line+"\n") {
		t.Error("mainnet config missing rpcauth line")
	}

	tn := &config.AppConfig{
		Network: "testnet4", PruneSize: 25, P2PMode: "tor",
	}
	got := BuildBitcoinConfig(tn, line)
	authIdx := strings.Index(got, line)
	sectIdx := strings.Index(got, "[testnet4]")
	if authIdx == -1 || sectIdx == -1 {
		t.Fatalf("missing rpcauth (%d) or section (%d)",
			authIdx, sectIdx)
	}
	if authIdx > sectIdx {
		t.Error("rpcauth line landed inside the [testnet4] " +
			"section — it must be global")
	}
}

// The RPC identity the conf grants and the identity the client
// authenticates as are declared in two packages (import
// direction); this pins them together.
func TestBitcoindRPCUserAgreesWithClient(t *testing.T) {
	if BitcoindRPCUser != bitcoin.RPCUser {
		t.Errorf("installer says RPC user %q, client says %q",
			BitcoindRPCUser, bitcoin.RPCUser)
	}
}
