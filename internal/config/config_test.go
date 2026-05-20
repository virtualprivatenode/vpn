// internal/config/config_test.go

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultValues(t *testing.T) {
	cfg := Default()

	if cfg.Network != "mainnet" {
		t.Errorf("Network: got %q, want %q", cfg.Network, "mainnet")
	}
	if cfg.Components != "bitcoin" {
		t.Errorf("Components: got %q, want %q", cfg.Components, "bitcoin")
	}
	if cfg.PruneSize != 25 {
		t.Errorf("PruneSize: got %d, want %d", cfg.PruneSize, 25)
	}
	if cfg.P2PMode != "tor" {
		t.Errorf("P2PMode: got %q, want %q", cfg.P2PMode, "tor")
	}
	if cfg.AutoUnlock {
		t.Error("AutoUnlock: expected false")
	}
	if cfg.LNDInstalled {
		t.Error("LNDInstalled: expected false")
	}
	if cfg.SyncthingInstalled {
		t.Error("SyncthingInstalled: expected false")
	}
	if cfg.InstallComplete {
		t.Error("InstallComplete: expected false")
	}
	if cfg.InstallVersion != "" {
		t.Error("InstallVersion: expected empty")
	}
	if cfg.WalletCreated {
		t.Error("WalletCreated: expected false")
	}
}

func TestHasLND(t *testing.T) {
	cfg := Default()
	if cfg.HasLND() {
		t.Error("default config should not have LND")
	}
	cfg.LNDInstalled = true
	if !cfg.HasLND() {
		t.Error("config with LNDInstalled=true should have LND")
	}
}

func TestIsMainnet(t *testing.T) {
	tests := []struct {
		network string
		want    bool
	}{
		{"mainnet", true},
		{"testnet4", false},
		{"", false},
		{"signet", false},
	}
	for _, tt := range tests {
		cfg := &AppConfig{Network: tt.network}
		if got := cfg.IsMainnet(); got != tt.want {
			t.Errorf("IsMainnet(%q): got %v, want %v",
				tt.network, got, tt.want)
		}
	}
}

func TestNetworkConfigRouting(t *testing.T) {
	mainnet := &AppConfig{Network: "mainnet"}
	if mainnet.NetworkConfig().RPCPort != 8332 {
		t.Errorf("mainnet RPCPort: got %d, want 8332",
			mainnet.NetworkConfig().RPCPort)
	}

	testnet := &AppConfig{Network: "testnet4"}
	if testnet.NetworkConfig().RPCPort != 48332 {
		t.Errorf("testnet4 RPCPort: got %d, want 48332",
			testnet.NetworkConfig().RPCPort)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := &AppConfig{
		InstallComplete:    true,
		InstallVersion:     "0.2.2",
		Network:            "testnet4",
		Components:         "bitcoin+lnd",
		PruneSize:          50,
		P2PMode:            "hybrid",
		AutoUnlock:         true,
		LNDInstalled:       true,
		WalletCreated:      true,
		SyncthingInstalled: true,
		SyncthingPassword:  "def456",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	loaded := &AppConfig{}
	if err := json.Unmarshal(data, loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if loaded.Network != original.Network {
		t.Errorf("Network: got %q, want %q",
			loaded.Network, original.Network)
	}
	if loaded.PruneSize != original.PruneSize {
		t.Errorf("PruneSize: got %d, want %d",
			loaded.PruneSize, original.PruneSize)
	}
	if loaded.SyncthingPassword != original.SyncthingPassword {
		t.Errorf("SyncthingPassword: got %q, want %q",
			loaded.SyncthingPassword, original.SyncthingPassword)
	}
	if !loaded.AutoUnlock {
		t.Error("AutoUnlock: expected true")
	}
}

func TestOmitEmptyPasswords(t *testing.T) {
	cfg := Default()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	raw := make(map[string]interface{})
	json.Unmarshal(data, &raw)

	if _, exists := raw["syncthing_password"]; exists {
		t.Error("empty SyncthingPassword should be omitted from JSON")
	}
}

func TestStoreSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{
		Dir:  tmpDir,
		Path: filepath.Join(tmpDir, "config.json"),
	}

	cfg := Default()
	cfg.PruneSize = 75
	cfg.LNDInstalled = true
	cfg.Network = "testnet4"

	if err := store.Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.PruneSize != 75 {
		t.Errorf("PruneSize: got %d, want 75", loaded.PruneSize)
	}
	if !loaded.LNDInstalled {
		t.Error("LNDInstalled: expected true")
	}
	if loaded.Network != "testnet4" {
		t.Errorf("Network: got %q, want testnet4", loaded.Network)
	}
}

func TestStoreLoadMissingFile(t *testing.T) {
	store := &Store{
		Dir:  t.TempDir(),
		Path: "/nonexistent/config.json",
	}
	_, err := store.Load()
	if err == nil {
		t.Error("expected error loading nonexistent file")
	}
}

func TestStoreLoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "config.json")
	os.WriteFile(tmpPath, []byte("not json"), 0600)

	store := &Store{Dir: tmpDir, Path: tmpPath}
	_, err := store.Load()
	if err == nil {
		t.Error("expected error loading invalid JSON")
	}
}

func TestStoreLoadInvalidNetwork(t *testing.T) {
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "config.json")
	badCfg := `{"network": "bogus", "components": "bitcoin", "prune_size": 25, "p2p_mode": "tor"}`
	os.WriteFile(tmpPath, []byte(badCfg), 0600)

	store := &Store{Dir: tmpDir, Path: tmpPath}
	_, err := store.Load()
	if err == nil {
		t.Error("expected error loading config with invalid network")
	}
}

func TestStoreSaveFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{
		Dir:  tmpDir,
		Path: filepath.Join(tmpDir, "config.json"),
	}

	if err := store.Save(Default()); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(store.Path)
	if err != nil {
		t.Fatal(err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions: got %o, want 0600", perm)
	}
}
