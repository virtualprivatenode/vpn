package lndrpc

import (
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
)

// ── Data types ───────────────────────────────────────────

type UTXO struct {
	Txid          string
	Vout          int
	AmountSats    int64
	Confirmations int64
	Address       string
	PkScript      string
}

type SendCoinsResult struct {
	Txid string
}

type FeeEstimate struct {
	FeeSats     int64
	SatPerVbyte uint64
}

// ── List UTXOs ───────────────────────────────────────────

func (c *Client) ListUnspent(
	minConfs, maxConfs int32,
) ([]UTXO, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return nil, errNotConnected
	}

	walletClient := walletrpc.NewWalletKitClient(conn)

	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := walletClient.ListUnspent(ctx,
		&walletrpc.ListUnspentRequest{
			MinConfs: minConfs,
			MaxConfs: maxConfs,
		})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	var utxos []UTXO
	for _, u := range resp.GetUtxos() {
		op := u.GetOutpoint()
		txid := ""
		vout := 0
		if op != nil {
			txid = fmt.Sprintf("%x", op.GetTxidBytes())
			// Reverse txid bytes for display
			txidBytes := op.GetTxidBytes()
			if len(txidBytes) == 32 {
				reversed := make([]byte, 32)
				for i := 0; i < 32; i++ {
					reversed[i] = txidBytes[31-i]
				}
				txid = fmt.Sprintf("%x", reversed)
			}
			vout = int(op.GetOutputIndex())
		}
		utxos = append(utxos, UTXO{
			Txid:          txid,
			Vout:          vout,
			AmountSats:    u.GetAmountSat(),
			Confirmations: u.GetConfirmations(),
			Address:       u.GetAddress(),
			PkScript:      fmt.Sprintf("%x", u.GetPkScript()),
		})
	}
	return utxos, nil
}

// ── Send on-chain ────────────────────────────────────────

func (c *Client) SendCoins(
	address string, amountSats int64,
	satPerVbyte int64, sendAll bool,
) (*SendCoinsResult, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(60 * time.Second)
	defer cancel()

	req := &lnrpc.SendCoinsRequest{
		Addr:        address,
		Amount:      amountSats,
		SatPerVbyte: uint64(satPerVbyte),
		SendAll:     sendAll,
	}

	resp, err := rpc.SendCoins(ctx, req)
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	return &SendCoinsResult{
		Txid: resp.GetTxid(),
	}, nil
}

// ── Fee estimation ───────────────────────────────────────

func (c *Client) EstimateFee(
	address string, amountSats int64,
	targetConf int32,
) (*FeeEstimate, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	addrToAmount := map[string]int64{
		address: amountSats,
	}

	resp, err := rpc.EstimateFee(ctx,
		&lnrpc.EstimateFeeRequest{
			AddrToAmount: addrToAmount,
			TargetConf:   targetConf,
		})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	return &FeeEstimate{
		FeeSats:     resp.GetFeeSat(),
		SatPerVbyte: resp.GetSatPerVbyte(),
	}, nil
}
