// internal/lndrpc/queries.go

package lndrpc

import (
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/ripsline/virtual-private-node/internal/logger"
)

const defaultTimeout = 30 * time.Second

// ── Data types returned to the TUI ───────────────────────

// NodeInfo contains basic node information.
type NodeInfo struct {
	Pubkey      string
	Alias       string
	Channels    int
	Peers       int
	BlockHeight int
	SyncedChain bool
	SyncedGraph bool
	Version     string
}

// WalletBalance contains on-chain wallet balances.
type WalletBalance struct {
	TotalBalance       string
	ConfirmedBalance   string
	UnconfirmedBalance string
}

// Channel contains information about a single channel.
type Channel struct {
	ChanID        uint64
	RemotePubkey  string
	Capacity      int64
	LocalBalance  int64
	RemoteBalance int64
	Active        bool
	Private       bool
	Initiator     bool
	PeerAlias     string
}

// PendingChannelInfo contains summary of pending channels.
type PendingChannelInfo struct {
	PendingOpen  int
	PendingClose int
	ForceClose   int
	WaitingClose int
}

// ── Query methods ────────────────────────────────────────

// GetInfo returns basic node information.
func (c *Client) GetInfo() (*NodeInfo, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}

	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	return &NodeInfo{
		Pubkey:      resp.GetIdentityPubkey(),
		Alias:       resp.GetAlias(),
		Channels:    int(resp.GetNumActiveChannels()),
		Peers:       int(resp.GetNumPeers()),
		BlockHeight: int(resp.GetBlockHeight()),
		SyncedChain: resp.GetSyncedToChain(),
		SyncedGraph: resp.GetSyncedToGraph(),
		Version:     resp.GetVersion(),
	}, nil
}

// GetWalletBalance returns on-chain wallet balances.
func (c *Client) GetWalletBalance() (*WalletBalance, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}

	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.WalletBalance(ctx, &lnrpc.WalletBalanceRequest{})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	return &WalletBalance{
		TotalBalance:       satStr(resp.GetTotalBalance()),
		ConfirmedBalance:   satStr(resp.GetConfirmedBalance()),
		UnconfirmedBalance: satStr(resp.GetUnconfirmedBalance()),
	}, nil
}

// ListChannels returns all active and inactive channels.
func (c *Client) ListChannels() ([]Channel, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}

	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.ListChannels(ctx, &lnrpc.ListChannelsRequest{})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	channels := make([]Channel, 0, len(resp.Channels))
	for _, ch := range resp.GetChannels() {
		channels = append(channels, Channel{
			ChanID:        ch.GetChanId(),
			RemotePubkey:  ch.GetRemotePubkey(),
			Capacity:      ch.GetCapacity(),
			LocalBalance:  ch.GetLocalBalance(),
			RemoteBalance: ch.GetRemoteBalance(),
			Active:        ch.GetActive(),
			Private:       ch.GetPrivate(),
			Initiator:     ch.GetInitiator(),
		})
	}

	// Resolve peer aliases
	for i := range channels {
		alias := c.getPeerAlias(channels[i].RemotePubkey)
		channels[i].PeerAlias = alias
	}

	return channels, nil
}

// GetPendingChannels returns a summary of pending channel states.
func (c *Client) GetPendingChannels() (*PendingChannelInfo, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}

	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.PendingChannels(ctx, &lnrpc.PendingChannelsRequest{})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	return &PendingChannelInfo{
		PendingOpen:  len(resp.GetPendingOpenChannels()),
		PendingClose: len(resp.GetPendingClosingChannels()),
		ForceClose:   len(resp.GetPendingForceClosingChannels()),
		WaitingClose: len(resp.GetWaitingCloseChannels()),
	}, nil
}

// ── Internal helpers ─────────────────────────────────────

// getPeerAlias resolves a pubkey to a node alias from the graph.
// Returns empty string if unavailable (non-critical).
func (c *Client) getPeerAlias(pubkey string) string {
	rpc := c.rpc()
	if rpc == nil {
		return ""
	}

	ctx, cancel := c.callCtx(3 * time.Second)
	defer cancel()

	resp, err := rpc.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          pubkey,
		IncludeChannels: false,
	})
	if err != nil {
		return ""
	}
	if resp.GetNode() != nil {
		return resp.GetNode().GetAlias()
	}
	return ""
}

// handleError logs RPC errors and triggers reconnect for connection failures.
func (c *Client) handleError(err error) {
	errStr := err.Error()
	// Timeout during IBD or startup — LND is alive but slow.
	// Don't reconnect, just let the next poll retry.
	if contains(errStr, "DeadlineExceeded") || contains(errStr, "context deadline") {
		return
	}
	// "starting up" means LND is alive but not ready — don't reconnect.
	if contains(errStr, "starting up") || contains(errStr, "not yet ready") {
		return
	}
	// gRPC Unavailable means LND is down or restarting
	if contains(errStr, "Unavailable") || contains(errStr, "connection refused") {
		logger.Status("LND connection lost, will reconnect: %v", err)
		go c.Reconnect()
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// satStr formats an int64 satoshi amount as a string.
func satStr(sats int64) string {
	return fmt.Sprintf("%d", sats)
}

var errNotConnected = fmt.Errorf("LND gRPC not connected")
