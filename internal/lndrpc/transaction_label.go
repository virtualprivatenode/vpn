package lndrpc

import (
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
)

// TransactionLabelRequest carries app-validated intent in wallet byte order.
type TransactionLabelRequest struct {
	Txid  chainhash.Hash
	Label string
}

// TransactionLabelResult distinguishes a local refusal from an attempted RPC.
// Submitted alone does not confirm a wallet write.
type TransactionLabelResult struct {
	Submitted bool
	Err       error
}

func (c *Client) SetTransactionLabel(input TransactionLabelRequest) TransactionLabelResult {
	if c == nil {
		return TransactionLabelResult{Err: errNotConnected}
	}
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return TransactionLabelResult{Err: errNotConnected}
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()
	_, err := walletrpc.NewWalletKitClient(conn).LabelTransaction(ctx,
		&walletrpc.LabelTransactionRequest{Txid: input.Txid[:], Label: input.Label, Overwrite: true})
	if err != nil {
		c.handleError(err)
	}
	return TransactionLabelResult{Submitted: true, Err: err}
}
