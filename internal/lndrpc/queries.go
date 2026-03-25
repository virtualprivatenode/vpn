// internal/lndrpc/queries.go

package lndrpc

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/ripsline/virtual-private-node/internal/logger"
)

const defaultTimeout = 30 * time.Second

// ── Data types ───────────────────────────────────────────

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

type WalletBalance struct {
	TotalBalance       string
	ConfirmedBalance   string
	UnconfirmedBalance string
}

type Channel struct {
	ChanID        uint64
	ChannelPoint  string // "txid:index"
	RemotePubkey  string
	Capacity      int64
	LocalBalance  int64
	RemoteBalance int64
	Active        bool
	Private       bool
	Initiator     bool
	PeerAlias     string
}

type PendingChannelInfo struct {
	PendingOpen               int
	ForceClose                int
	WaitingClose              int
	PendingOpenChannels       []PendingChannel
	PendingForceCloseChannels []PendingForceCloseChannel
	WaitingCloseChannels      []WaitingCloseChannel
}

type PendingChannel struct {
	RemotePubkey string
	Capacity     int64
	LocalBalance int64
	PeerAlias    string
}

type PendingForceCloseChannel struct {
	RemotePubkey     string
	ChannelPoint     string
	Capacity         int64
	LocalBalance     int64
	LimboBalance     int64
	RecoveredBalance int64
	ClosingTxid      string
	MaturityHeight   int32
	BlocksRemaining  int32
	PeerAlias        string
}

type WaitingCloseChannel struct {
	RemotePubkey string
	ChannelPoint string
	Capacity     int64
	LocalBalance int64
	LimboBalance int64
	ClosingTxid  string
	PeerAlias    string
}

type ClosedChannel struct {
	ChannelPoint string
	RemotePubkey string
	Capacity     int64
	CloseType    string
	ClosingTxid  string
	PeerAlias    string
	SettledBal   int64
	CloseHeight  int32
}

type OnChainAddress struct {
	Address string
}

type ChannelOpenResult struct {
	FundingTxID string
}

type PeerInfo struct {
	PubKey  string
	Address string
	Inbound bool
	SatSent int64
	SatRecv int64
}

// ── Read queries ─────────────────────────────────────────

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

	channels := make([]Channel, 0, len(resp.GetChannels()))
	for _, ch := range resp.GetChannels() {
		channels = append(channels, Channel{
			ChanID:        ch.GetChanId(),
			ChannelPoint:  ch.GetChannelPoint(),
			RemotePubkey:  ch.GetRemotePubkey(),
			Capacity:      ch.GetCapacity(),
			LocalBalance:  ch.GetLocalBalance(),
			RemoteBalance: ch.GetRemoteBalance(),
			Active:        ch.GetActive(),
			Private:       ch.GetPrivate(),
			Initiator:     ch.GetInitiator(),
		})
	}
	for i := range channels {
		channels[i].PeerAlias = c.getPeerAlias(channels[i].RemotePubkey)
	}
	return channels, nil
}

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

	var pendingChans []PendingChannel
	for _, pc := range resp.GetPendingOpenChannels() {
		ch := pc.GetChannel()
		if ch != nil {
			alias := c.getPeerAlias(ch.GetRemoteNodePub())
			pendingChans = append(pendingChans, PendingChannel{
				RemotePubkey: ch.GetRemoteNodePub(),
				Capacity:     ch.GetCapacity(),
				LocalBalance: ch.GetLocalBalance(),
				PeerAlias:    alias,
			})
		}
	}

	var forceCloseChans []PendingForceCloseChannel
	for _, fc := range resp.GetPendingForceClosingChannels() {
		ch := fc.GetChannel()
		if ch != nil {
			alias := c.getPeerAlias(
				ch.GetRemoteNodePub())
			forceCloseChans = append(forceCloseChans,
				PendingForceCloseChannel{
					RemotePubkey:     ch.GetRemoteNodePub(),
					ChannelPoint:     ch.GetChannelPoint(),
					Capacity:         ch.GetCapacity(),
					LocalBalance:     ch.GetLocalBalance(),
					LimboBalance:     fc.GetLimboBalance(),
					RecoveredBalance: fc.GetRecoveredBalance(),
					ClosingTxid:      fc.GetClosingTxid(),
					MaturityHeight:   int32(fc.GetMaturityHeight()),
					BlocksRemaining:  fc.GetBlocksTilMaturity(),
					PeerAlias:        alias,
				})
		}
	}

	var waitingCloseChans []WaitingCloseChannel
	for _, wc := range resp.GetWaitingCloseChannels() {
		ch := wc.GetChannel()
		if ch != nil {
			alias := c.getPeerAlias(
				ch.GetRemoteNodePub())
			waitingCloseChans = append(
				waitingCloseChans,
				WaitingCloseChannel{
					RemotePubkey: ch.GetRemoteNodePub(),
					ChannelPoint: ch.GetChannelPoint(),
					Capacity:     ch.GetCapacity(),
					LocalBalance: ch.GetLocalBalance(),
					LimboBalance: wc.GetLimboBalance(),
					ClosingTxid:  wc.GetClosingTxid(),
					PeerAlias:    alias,
				})
		}
	}

	return &PendingChannelInfo{
		PendingOpen:               len(resp.GetPendingOpenChannels()),
		ForceClose:                len(resp.GetPendingForceClosingChannels()),
		WaitingClose:              len(resp.GetWaitingCloseChannels()),
		PendingOpenChannels:       pendingChans,
		PendingForceCloseChannels: forceCloseChans,
		WaitingCloseChannels:      waitingCloseChans,
	}, nil
}

// ListClosedChannels returns all historically closed
// channels with close type and peer info.
func (c *Client) ListClosedChannels() (
	[]ClosedChannel, error,
) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.ClosedChannels(ctx,
		&lnrpc.ClosedChannelsRequest{})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	var channels []ClosedChannel
	for _, ch := range resp.GetChannels() {
		closeType := "unknown"
		switch ch.GetCloseType() {
		case lnrpc.ChannelCloseSummary_COOPERATIVE_CLOSE:
			closeType = "cooperative"
		case lnrpc.ChannelCloseSummary_LOCAL_FORCE_CLOSE:
			closeType = "force"
		case lnrpc.ChannelCloseSummary_REMOTE_FORCE_CLOSE:
			closeType = "force"
		case lnrpc.ChannelCloseSummary_BREACH_CLOSE:
			closeType = "breach"
		case lnrpc.ChannelCloseSummary_FUNDING_CANCELED:
			closeType = "canceled"
		case lnrpc.ChannelCloseSummary_ABANDONED:
			closeType = "abandoned"
		}

		alias := c.getPeerAlias(
			ch.GetRemotePubkey())
		if alias == "" {
			pk := ch.GetRemotePubkey()
			if len(pk) > 12 {
				alias = pk[:12] + ".."
			} else {
				alias = pk
			}
		}

		channels = append(channels, ClosedChannel{
			ChannelPoint: ch.GetChannelPoint(),
			RemotePubkey: ch.GetRemotePubkey(),
			Capacity:     ch.GetCapacity(),
			CloseType:    closeType,
			ClosingTxid:  ch.GetClosingTxHash(),
			PeerAlias:    alias,
			SettledBal:   ch.GetSettledBalance(),
			CloseHeight:  int32(ch.GetCloseHeight()),
		})
	}

	return channels, nil
}

func (c *Client) GetNewAddress() (*OnChainAddress, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.NewAddress(ctx, &lnrpc.NewAddressRequest{
		Type: lnrpc.AddressType_TAPROOT_PUBKEY,
	})
	if err != nil {
		c.handleError(err)
		return nil, err
	}
	return &OnChainAddress{Address: resp.GetAddress()}, nil
}

// ListPeers returns currently connected peers.
func (c *Client) ListPeers() ([]PeerInfo, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.ListPeers(ctx, &lnrpc.ListPeersRequest{})
	if err != nil {
		c.handleError(err)
		return nil, err
	}
	var peers []PeerInfo
	for _, p := range resp.GetPeers() {
		peers = append(peers, PeerInfo{
			PubKey:  p.GetPubKey(),
			Address: p.GetAddress(),
			Inbound: p.GetInbound(),
			SatSent: p.GetSatSent(),
			SatRecv: p.GetSatRecv(),
		})
	}
	return peers, nil
}

// ── Channel operations (fund-moving) ─────────────────────

// ConnectPeer connects to a Lightning peer. Uses perm=true for
// persistent connection. Does not fail if already connected.
func (c *Client) ConnectPeer(pubkey, host string) error {
	rpc := c.rpc()
	if rpc == nil {
		return errNotConnected
	}
	ctx, cancel := c.callCtx(60 * time.Second)
	defer cancel()

	_, err := rpc.ConnectPeer(ctx, &lnrpc.ConnectPeerRequest{
		Addr: &lnrpc.LightningAddress{
			Pubkey: pubkey,
			Host:   host,
		},
		Perm: true,
	})
	if err != nil {
		errStr := err.Error()
		if contains(errStr, "already connected") {
			return nil
		}
		return err
	}
	return nil
}

// WaitForPeer polls ListPeers until the given pubkey appears or
// timeout is reached. Returns nil if peer connected, error if timeout.
func (c *Client) WaitForPeer(pubkey string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		peers, err := c.ListPeers()
		if err == nil {
			for _, p := range peers {
				if p.PubKey == pubkey {
					return nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("peer did not connect within %s", timeout)
}

// OpenChannel opens a channel to a peer. This is a fund-moving operation.
// The caller MUST verify the peer is connected and show a confirmation
// dialog before calling this.
func (c *Client) OpenChannel(pubkey string, localAmount int64, private bool) (*ChannelOpenResult, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}

	pubkeyBytes, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %w", err)
	}

	ctx, cancel := c.callCtx(120 * time.Second)
	defer cancel()

	resp, err := rpc.OpenChannelSync(ctx, &lnrpc.OpenChannelRequest{
		NodePubkey:         pubkeyBytes,
		LocalFundingAmount: localAmount,
		Private:            private,
		MinConfs:           1,
		SpendUnconfirmed:   false,
	})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	txidBytes := resp.GetFundingTxidBytes()
	txid := fmt.Sprintf("%x", txidBytes)
	if len(txidBytes) == 32 {
		reversed := make([]byte, 32)
		for i := 0; i < 32; i++ {
			reversed[i] = txidBytes[31-i]
		}
		txid = fmt.Sprintf("%x", reversed)
	}
	return &ChannelOpenResult{FundingTxID: txid}, nil
}

// ── Internal helpers ─────────────────────────────────────

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

func (c *Client) handleError(err error) {
	errStr := err.Error()
	if contains(errStr, "DeadlineExceeded") || contains(errStr, "context deadline") {
		return
	}
	if contains(errStr, "starting up") || contains(errStr, "not yet ready") {
		return
	}
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

func satStr(sats int64) string {
	return fmt.Sprintf("%d", sats)
}

var errNotConnected = fmt.Errorf("LND gRPC not connected")
