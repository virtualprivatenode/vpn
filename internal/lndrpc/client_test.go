// internal/lndrpc/client_test.go

package lndrpc

import (
	"testing"
)

func TestNodeInfoFields(t *testing.T) {
	info := &NodeInfo{
		Pubkey:      "02abc123",
		Alias:       "mynode",
		Channels:    5,
		Peers:       10,
		BlockHeight: 850000,
		SyncedChain: true,
		SyncedGraph: true,
		Version:     "0.20.0-beta",
	}

	if info.Pubkey != "02abc123" {
		t.Errorf("Pubkey: got %q", info.Pubkey)
	}
	if info.Channels != 5 {
		t.Errorf("Channels: got %d, want 5", info.Channels)
	}
	if !info.SyncedChain {
		t.Error("SyncedChain: expected true")
	}
}

func TestWalletBalanceFields(t *testing.T) {
	bal := &WalletBalance{
		TotalBalance:       "1000000",
		ConfirmedBalance:   "900000",
		UnconfirmedBalance: "100000",
	}

	if bal.TotalBalance != "1000000" {
		t.Errorf("TotalBalance: got %q", bal.TotalBalance)
	}
}

func TestChannelFields(t *testing.T) {
	ch := Channel{
		ChanID:        123456,
		RemotePubkey:  "02abc",
		Capacity:      1000000,
		LocalBalance:  600000,
		RemoteBalance: 400000,
		Active:        true,
		Private:       false,
		Initiator:     true,
		PeerAlias:     "ACINQ",
	}

	if ch.Capacity != 1000000 {
		t.Errorf("Capacity: got %d", ch.Capacity)
	}
	if ch.LocalBalance != 600000 {
		t.Errorf("LocalBalance: got %d", ch.LocalBalance)
	}
	if !ch.Active {
		t.Error("Active: expected true")
	}
	if ch.PeerAlias != "ACINQ" {
		t.Errorf("PeerAlias: got %q", ch.PeerAlias)
	}
}

func TestPendingChannelInfoFields(t *testing.T) {
	info := &PendingChannelInfo{
		PendingOpen:  2,
		PendingClose: 1,
		ForceClose:   0,
		WaitingClose: 0,
	}

	if info.PendingOpen != 2 {
		t.Errorf("PendingOpen: got %d", info.PendingOpen)
	}
}

func TestSatStr(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{1000000, "1000000"},
		{-500, "-500"},
		{2100000000000000, "2100000000000000"},
	}
	for _, tt := range tests {
		got := satStr(tt.input)
		if got != tt.want {
			t.Errorf("satStr(%d): got %q, want %q",
				tt.input, got, tt.want)
		}
	}
}

func TestContains(t *testing.T) {
	if !contains("connection refused", "refused") {
		t.Error("should contain refused")
	}
	if contains("connection refused", "timeout") {
		t.Error("should not contain timeout")
	}
	if contains("", "something") {
		t.Error("empty string should not contain anything")
	}
}

func TestNilClientSafety(t *testing.T) {
	// A client that failed to connect should return errors, not panic
	c := &Client{}

	_, err := c.GetInfo()
	if err == nil {
		t.Error("GetInfo on disconnected client should return error")
	}

	_, err = c.GetWalletBalance()
	if err == nil {
		t.Error("GetWalletBalance on disconnected client should return error")
	}

	_, err = c.ListChannels()
	if err == nil {
		t.Error("ListChannels on disconnected client should return error")
	}

	_, err = c.GetPendingChannels()
	if err == nil {
		t.Error("GetPendingChannels on disconnected client should return error")
	}
}

func TestIsConnectedDefault(t *testing.T) {
	c := &Client{}
	if c.IsConnected() {
		t.Error("new client should not be connected")
	}
}
