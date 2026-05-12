package lndrpc

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
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

type OnChainTx struct {
	Txid           string
	Amount         int64
	Fee            int64
	Confirmations  int32
	BlockHeight    int32
	Timestamp      int64
	Label          string
	DestAddresses  []string
	RawTxHex       string
	TxType         string // "send", "receive", "channel_open", "channel_close"
	ChannelPeer    string // peer alias for channel open/close
	Inputs         []TxInput
	Outputs        []TxOutput
	TotalInputSats int64
	IsAnchorSweep  bool // abandoned anchor sweep (0 confs, ≤330 sats)
}

type TxInput struct {
	Outpoint string // "txid:vout" format
	PrevTxid string
	PrevVout uint32
	IsOurs   bool
	Amount   int64
}

type TxOutput struct {
	Address string
	Amount  int64
	IsLocal bool   // true if address belongs to our wallet
	Label   string // "destination", "change", "channel"
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
	outpoints []string,
) (*SendCoinsResult, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(60 * time.Second)
	defer cancel()

	req := &lnrpc.SendCoinsRequest{
		Addr:             address,
		Amount:           amountSats,
		SatPerVbyte:      uint64(satPerVbyte),
		SendAll:          sendAll,
		MinConfs:         0,
		SpendUnconfirmed: true,
	}

	// Coin control: restrict inputs to selected UTXOs
	for _, op := range outpoints {
		parts := strings.SplitN(op, ":", 2)
		if len(parts) != 2 {
			continue
		}
		txid := parts[0]
		var idx uint32
		for _, c := range parts[1] {
			if c >= '0' && c <= '9' {
				idx = idx*10 + uint32(c-'0')
			}
		}
		req.Outpoints = append(req.Outpoints,
			&lnrpc.OutPoint{
				TxidStr:     txid,
				OutputIndex: idx,
			})
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

// ── Get on-chain transactions ────────────────────────────

func (c *Client) GetTransactions() ([]OnChainTx, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.GetTransactions(ctx,
		&lnrpc.GetTransactionsRequest{})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	// Build set of our wallet addresses from UTXOs
	// for identifying change outputs
	walletAddrs := c.getWalletAddresses()

	// Get channel funding txids for labeling
	chanTxids := c.getChannelFundingTxids()

	// Get closed channel txids
	closeTxids := c.getClosedChannelTxids()

	var txs []OnChainTx
	for _, t := range resp.GetTransactions() {
		txid := t.GetTxHash()
		amount := t.GetAmount()
		fee := t.GetTotalFees()

		// Parse inputs from previous outpoints
		var inputs []TxInput
		for _, po := range t.GetPreviousOutpoints() {
			outpoint := po.GetOutpoint()
			prevTxid := ""
			var prevVout uint32
			if idx := strings.LastIndex(
				outpoint, ":"); idx >= 0 {
				prevTxid = outpoint[:idx]
				for _, c := range outpoint[idx+1:] {
					if c >= '0' && c <= '9' {
						prevVout = prevVout*10 +
							uint32(c-'0')
					}
				}
			}
			inputs = append(inputs, TxInput{
				Outpoint: outpoint,
				PrevTxid: prevTxid,
				PrevVout: prevVout,
				IsOurs:   po.GetIsOurOutput(),
			})
		}

		// Parse outputs from output details
		var outputs []TxOutput
		for _, od := range t.GetOutputDetails() {
			addr := od.GetAddress()
			if addr == "" {
				// Skip unparseable outputs
				continue
			}
			outputs = append(outputs, TxOutput{
				Address: addr,
				Amount:  od.GetAmount(),
				IsLocal: od.GetIsOurAddress(),
			})
		}

		// Fallback: if no output details, use
		// dest addresses (older LND versions)
		if len(outputs) == 0 {
			for _, addr := range t.GetDestAddresses() {
				isLocal := walletAddrs[addr]
				outputs = append(outputs, TxOutput{
					Address: addr,
					IsLocal: isLocal,
				})
			}
		}

		// Determine transaction type
		txType := "receive"
		channelPeer := ""
		label := t.GetLabel()

		if peerAlias, ok := chanTxids[txid]; ok {
			txType = "channel_open"
			channelPeer = peerAlias
		} else if peerAlias, ok :=
			closeTxids[txid]; ok {
			txType = "channel_close"
			channelPeer = peerAlias
		} else if amount < 0 {
			txType = "send"
		}

		// Label outputs based on type
		for i := range outputs {
			if txType == "send" {
				if outputs[i].IsLocal {
					outputs[i].Label = "change"
				} else {
					outputs[i].Label = "destination"
				}
			} else if txType == "channel_open" {
				if !outputs[i].IsLocal {
					outputs[i].Label = "channel"
				} else {
					outputs[i].Label = "change"
				}
			} else if txType == "channel_close" {
				if outputs[i].IsLocal {
					outputs[i].Label = "received"
				}
			} else {
				// receive
				if outputs[i].IsLocal {
					outputs[i].Label = "received"
				}
			}
		}

		// If label from LND is present, use it
		if label == "" {
			switch txType {
			case "channel_open":
				label = "Channel Open"
				if channelPeer != "" {
					label += ": " + channelPeer
				}
			case "channel_close":
				label = "Channel Close"
				if channelPeer != "" {
					label += ": " + channelPeer
				}
			case "send":
				label = "On-chain Send"
			case "receive":
				label = "On-chain Receive"
			}
		}

		tx := OnChainTx{
			Txid:          txid,
			Amount:        amount,
			Fee:           fee,
			Confirmations: t.GetNumConfirmations(),
			BlockHeight:   t.GetBlockHeight(),
			Timestamp:     t.GetTimeStamp(),
			Label:         label,
			DestAddresses: t.GetDestAddresses(),
			RawTxHex:      t.GetRawTxHex(),
			TxType:        txType,
			ChannelPeer:   channelPeer,
			Inputs:        inputs,
			Outputs:       outputs,
		}

		// Detect abandoned anchor sweeps:
		// 0 confirmations, total amount ≤ 330 sats
		// (BOLT 3 anchor output value), typically
		// associated with a force close. The sweep
		// tx is abandoned because the fee exceeds
		// the 330-sat anchor value.
		if tx.Confirmations == 0 {
			absAmt := tx.Amount
			if absAmt < 0 {
				absAmt = -absAmt
			}
			if absAmt <= 330 &&
				(tx.TxType == "channel_close" ||
					strings.Contains(
						strings.ToLower(tx.Label),
						"anchor")) {
				tx.IsAnchorSweep = true
				tx.Label = "Anchor Sweep"
			}
		}

		txs = append(txs, tx)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(txs, func(i, j int) bool {
		return txs[i].Timestamp > txs[j].Timestamp
	})

	return txs, nil
}

// ── Label transaction ───────────────────────────────────

func (c *Client) LabelTransaction(
	txid string, label string, overwrite bool,
) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return errNotConnected
	}

	// Convert hex txid to reversed bytes
	// (LND expects internal byte order)
	txidBytes, err := hex.DecodeString(txid)
	if err != nil {
		return fmt.Errorf("invalid txid: %w", err)
	}
	if len(txidBytes) == 32 {
		for i, j := 0, 31; i < j; i, j = i+1, j-1 {
			txidBytes[i], txidBytes[j] =
				txidBytes[j], txidBytes[i]
		}
	}

	walletClient := walletrpc.NewWalletKitClient(conn)
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	_, err = walletClient.LabelTransaction(ctx,
		&walletrpc.LabelTransactionRequest{
			Txid:      txidBytes,
			Label:     label,
			Overwrite: overwrite,
		})
	if err != nil {
		c.handleError(err)
		return err
	}
	return nil
}

// ── Helpers for transaction labeling ─────────────────────

// getWalletAddresses returns a set of known wallet
// addresses from current UTXOs.
func (c *Client) getWalletAddresses() map[string]bool {
	addrs := make(map[string]bool)

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return addrs
	}

	walletClient := walletrpc.NewWalletKitClient(conn)
	ctx, cancel := c.callCtx(10 * time.Second)
	defer cancel()

	resp, err := walletClient.ListUnspent(ctx,
		&walletrpc.ListUnspentRequest{
			MinConfs: 0,
			MaxConfs: 999999,
		})
	if err != nil {
		return addrs
	}

	for _, u := range resp.GetUtxos() {
		if u.GetAddress() != "" {
			addrs[u.GetAddress()] = true
		}
	}

	// Also get a few recent addresses from the
	// address manager. We look at dest addresses of
	// recent receive transactions since UTXOs may
	// have been spent already.
	rpc := c.rpc()
	if rpc != nil {
		ctx2, cancel2 := c.callCtx(10 * time.Second)
		defer cancel2()
		txResp, err := rpc.GetTransactions(ctx2,
			&lnrpc.GetTransactionsRequest{})
		if err == nil {
			for _, t := range txResp.GetTransactions() {
				if t.GetAmount() > 0 {
					for _, addr := range t.GetDestAddresses() {
						addrs[addr] = true
					}
				}
			}
		}
	}

	return addrs
}

// getChannelFundingTxids returns a map of funding txid →
// peer alias for all open and pending channels.
func (c *Client) getChannelFundingTxids() map[string]string {
	result := make(map[string]string)
	rpc := c.rpc()
	if rpc == nil {
		return result
	}

	// Open channels
	ctx, cancel := c.callCtx(10 * time.Second)
	defer cancel()
	chResp, err := rpc.ListChannels(ctx,
		&lnrpc.ListChannelsRequest{})
	if err == nil {
		for _, ch := range chResp.GetChannels() {
			cp := ch.GetChannelPoint()
			txid := chanPointTxid(cp)
			if txid != "" {
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
				result[txid] = alias
			}
		}
	}

	// Pending channels
	ctx2, cancel2 := c.callCtx(10 * time.Second)
	defer cancel2()
	pendResp, err := rpc.PendingChannels(ctx2,
		&lnrpc.PendingChannelsRequest{})
	if err == nil {
		for _, pc := range pendResp.
			GetPendingOpenChannels() {
			ch := pc.GetChannel()
			if ch != nil {
				cp := ch.GetChannelPoint()
				txid := chanPointTxid(cp)
				if txid != "" {
					alias := c.getPeerAlias(
						ch.GetRemoteNodePub())
					if alias == "" {
						pk := ch.GetRemoteNodePub()
						if len(pk) > 12 {
							alias = pk[:12] + ".."
						} else {
							alias = pk
						}
					}
					result[txid] = alias
				}
			}
		}
	}

	return result
}

// getClosedChannelTxids returns a map of closing txid →
// peer alias for all closed channels.
func (c *Client) getClosedChannelTxids() map[string]string {
	result := make(map[string]string)
	rpc := c.rpc()
	if rpc == nil {
		return result
	}

	ctx, cancel := c.callCtx(10 * time.Second)
	defer cancel()

	resp, err := rpc.ClosedChannels(ctx,
		&lnrpc.ClosedChannelsRequest{})
	if err != nil {
		return result
	}

	for _, ch := range resp.GetChannels() {
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

		closeTxid := ch.GetClosingTxHash()
		if closeTxid != "" {
			result[closeTxid] = alias
		}

		// Also record the funding tx as channel_open
		// (handled by getChannelFundingTxids for
		// open channels, but closed channels won't
		// appear there anymore)
		cp := ch.GetChannelPoint()
		fundingTxid := chanPointTxid(cp)
		if fundingTxid != "" {
			result[fundingTxid] = alias
		}
	}

	return result
}

// chanPointTxid extracts the txid from a channel point
// string "txid:index".
func chanPointTxid(cp string) string {
	for i := len(cp) - 1; i >= 0; i-- {
		if cp[i] == ':' {
			return cp[:i]
		}
	}
	return cp
}
