// internal/lndrpc/client_test.go

package lndrpc

import (
	"testing"
)

func TestNodeInfoFields(t *testing.T) {
	info := &NodeInfo{
		Pubkey: "02abc123", Alias: "mynode", Channels: 5,
		Peers: 10, BlockHeight: 850000, SyncedChain: true,
		SyncedGraph: true, Version: "0.20.0-beta",
	}
	if info.Channels != 5 {
		t.Errorf("Channels: got %d", info.Channels)
	}
}

func TestWalletBalanceFields(t *testing.T) {
	bal := &WalletBalance{TotalBalance: "1000000"}
	if bal.TotalBalance != "1000000" {
		t.Errorf("TotalBalance: got %q", bal.TotalBalance)
	}
}

func TestChannelFields(t *testing.T) {
	ch := Channel{Capacity: 1000000, LocalBalance: 600000, Active: true, PeerAlias: "ACINQ"}
	if ch.Capacity != 1000000 {
		t.Errorf("Capacity: got %d", ch.Capacity)
	}
	if ch.PeerAlias != "ACINQ" {
		t.Errorf("PeerAlias: got %q", ch.PeerAlias)
	}
}

func TestPendingChannelInfoFields(t *testing.T) {
	info := &PendingChannelInfo{PendingOpen: 2}
	if info.PendingOpen != 2 {
		t.Errorf("PendingOpen: got %d", info.PendingOpen)
	}
}

func TestSatStr(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"}, {1000000, "1000000"}, {-500, "-500"},
	}
	for _, tt := range tests {
		if got := satStr(tt.input); got != tt.want {
			t.Errorf("satStr(%d): got %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestContains(t *testing.T) {
	if !contains("connection refused", "refused") {
		t.Error("should contain")
	}
	if contains("", "something") {
		t.Error("empty should not contain")
	}
}

func TestNilClientSafety(t *testing.T) {
	c := &Client{}
	if _, err := c.GetInfo(); err == nil {
		t.Error("should error")
	}
	if _, err := c.GetWalletBalance(); err == nil {
		t.Error("should error")
	}
	if _, err := c.ListChannels(); err == nil {
		t.Error("should error")
	}
	if _, err := c.GetPendingChannels(); err == nil {
		t.Error("should error")
	}
	if _, err := c.GetNewAddress(); err == nil {
		t.Error("should error")
	}
	if _, err := c.ListPeers(); err == nil {
		t.Error("should error")
	}
	if err := c.ConnectPeer("a", "b"); err == nil {
		t.Error("should error")
	}
	if _, err := c.OpenChannel("a", 100000, false, false); err == nil {
		t.Error("should error")
	}
}
