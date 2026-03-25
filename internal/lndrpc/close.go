package lndrpc

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
)

// CloseChannelResult holds the outcome of a channel
// close request.
type CloseChannelResult struct {
	ClosingTxid string
}

// CloseChannel initiates a channel close. If force is
// true, a unilateral close is performed. The function
// blocks until the closing transaction is broadcast
// (ClosePending update) and returns the closing txid.
func (c *Client) CloseChannel(
	channelPoint string,
	force bool,
	satPerVbyte uint64,
) (*CloseChannelResult, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}

	// Parse "txid:index" into components
	parts := strings.SplitN(channelPoint, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf(
			"invalid channel point: %s",
			channelPoint)
	}
	txidHex := parts[0]
	var outputIndex uint32
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return nil, fmt.Errorf(
				"invalid output index: %s",
				parts[1])
		}
		outputIndex = outputIndex*10 +
			uint32(c-'0')
	}

	// Convert txid hex to reversed bytes
	// (LND expects internal byte order)
	txidBytes, err := hex.DecodeString(txidHex)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid txid hex: %w", err)
	}
	if len(txidBytes) == 32 {
		for i, j := 0, 31; i < j; i, j = i+1, j-1 {
			txidBytes[i], txidBytes[j] =
				txidBytes[j], txidBytes[i]
		}
	}

	req := &lnrpc.CloseChannelRequest{
		ChannelPoint: &lnrpc.ChannelPoint{
			FundingTxid: &lnrpc.ChannelPoint_FundingTxidBytes{
				FundingTxidBytes: txidBytes,
			},
			OutputIndex: outputIndex,
		},
		Force: force,
	}
	if satPerVbyte > 0 {
		req.SatPerVbyte = satPerVbyte
	}

	// Use a long timeout — force closes can take
	// time and cooperative closes need peer
	// communication over Tor
	ctx, cancel := c.callCtx(120 * time.Second)
	defer cancel()

	stream, err := rpc.CloseChannel(ctx, req)
	if err != nil {
		c.handleError(err)
		return nil, fmt.Errorf(
			"close channel: %w", err)
	}

	// Wait for the first update (ClosePending)
	// which contains the closing transaction txid
	for {
		update, err := stream.Recv()
		if err != nil {
			return nil, fmt.Errorf(
				"close stream: %w", err)
		}

		switch u := update.Update.(type) {
		case *lnrpc.CloseStatusUpdate_ClosePending:
			txid := u.ClosePending.GetTxid()
			// Reverse bytes for display
			txidDisplay := fmt.Sprintf("%x", txid)
			if len(txid) == 32 {
				reversed := make([]byte, 32)
				for i := 0; i < 32; i++ {
					reversed[i] = txid[31-i]
				}
				txidDisplay = fmt.Sprintf(
					"%x", reversed)
			}
			return &CloseChannelResult{
				ClosingTxid: txidDisplay,
			}, nil

		case *lnrpc.CloseStatusUpdate_ChanClose:
			// Channel fully resolved — shouldn't
			// happen before ClosePending but handle
			// it gracefully
			return &CloseChannelResult{
				ClosingTxid: "resolved",
			}, nil
		}
	}
}
