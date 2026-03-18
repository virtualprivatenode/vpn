package lndrpc

import (
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
)

// ── Data types ───────────────────────────────────────────

type Invoice struct {
	PaymentRequest string
	PaymentHash    string
	AmountSats     int64
	Memo           string
	Settled        bool
	CreationDate   int64
	SettleDate     int64
	Expiry         int64
	IsExpired      bool
}

type DecodedPayReq struct {
	Destination string
	AmountSats  int64
	Description string
	PaymentHash string
	Expiry      int64
	Timestamp   int64
	IsExpired   bool
}

type PaymentEntry struct {
	PaymentHash    string
	AmountSats     int64
	FeeSats        int64
	Status         string // SUCCEEDED, FAILED, IN_FLIGHT
	CreationDate   int64
	Preimage       string
	PaymentRequest string
	IsIncoming     bool
	Memo           string
	Hops           []RouteHop
}

type RouteHop struct {
	PubKey   string
	Alias    string
	ChanID   uint64
	FeeSats  int64
	AmtToFwd int64
}

// ── Invoice creation ─────────────────────────────────────

// AddInvoice creates a new Lightning invoice.
func (c *Client) AddInvoice(amountSats int64, memo string) (*Invoice, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.AddInvoice(ctx, &lnrpc.Invoice{
		Value:   amountSats,
		Memo:    memo,
		Private: true,
	})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	return &Invoice{
		PaymentRequest: resp.GetPaymentRequest(),
		PaymentHash:    fmt.Sprintf("%x", resp.GetRHash()),
		AmountSats:     amountSats,
		Memo:           memo,
	}, nil
}

// ── Invoice decoding ─────────────────────────────────────

// DecodePayReq decodes a bolt11 payment request without paying it.
func (c *Client) DecodePayReq(payReq string) (*DecodedPayReq, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.DecodePayReq(ctx, &lnrpc.PayReqString{
		PayReq: payReq,
	})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	now := time.Now().Unix()
	isExpired := (resp.GetTimestamp() + resp.GetExpiry()) < now

	return &DecodedPayReq{
		Destination: resp.GetDestination(),
		AmountSats:  resp.GetNumSatoshis(),
		Description: resp.GetDescription(),
		PaymentHash: resp.GetPaymentHash(),
		Expiry:      resp.GetExpiry(),
		Timestamp:   resp.GetTimestamp(),
		IsExpired:   isExpired,
	}, nil
}

// ── Invoice lookup ───────────────────────────────────────

// LookupInvoice checks the status of an invoice by payment hash.
func (c *Client) LookupInvoice(paymentHash []byte) (*Invoice, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.LookupInvoice(ctx, &lnrpc.PaymentHash{
		RHash: paymentHash,
	})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	now := time.Now().Unix()
	isExpired := !resp.GetSettled() &&
		(resp.GetCreationDate()+resp.GetExpiry()) < now

	return &Invoice{
		PaymentRequest: resp.GetPaymentRequest(),
		PaymentHash:    fmt.Sprintf("%x", resp.GetRHash()),
		AmountSats:     resp.GetValue(),
		Memo:           resp.GetMemo(),
		Settled:        resp.GetSettled(),
		CreationDate:   resp.GetCreationDate(),
		SettleDate:     resp.GetSettleDate(),
		Expiry:         resp.GetExpiry(),
		IsExpired:      isExpired,
	}, nil
}

// ── Invoice listing ──────────────────────────────────────

// ListInvoices returns recent invoices (received payments).
func (c *Client) ListInvoices(limit uint64) ([]PaymentEntry, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.ListInvoices(ctx, &lnrpc.ListInvoiceRequest{
		NumMaxInvoices: limit,
		Reversed:       true,
	})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	var entries []PaymentEntry
	for _, inv := range resp.GetInvoices() {
		if inv.GetValue() == 0 && !inv.GetSettled() {
			continue // skip zero-amount unsettled
		}
		status := "OPEN"
		if inv.GetSettled() {
			status = "SETTLED"
		} else {
			now := time.Now().Unix()
			if (inv.GetCreationDate() + inv.GetExpiry()) < now {
				status = "EXPIRED"
			}
		}
		entries = append(entries, PaymentEntry{
			PaymentHash:  fmt.Sprintf("%x", inv.GetRHash()),
			AmountSats:   inv.GetValue(),
			Status:       status,
			CreationDate: inv.GetCreationDate(),
			IsIncoming:   true,
			Memo:         inv.GetMemo(),
		})
	}
	return entries, nil
}

// ── Payment listing ──────────────────────────────────────

// ListPayments returns recent outgoing payments.
func (c *Client) ListPayments(limit uint64) ([]PaymentEntry, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.ListPayments(ctx, &lnrpc.ListPaymentsRequest{
		MaxPayments:       limit,
		Reversed:          true,
		IncludeIncomplete: true,
	})
	if err != nil {
		c.handleError(err)
		return nil, err
	}

	var entries []PaymentEntry
	for _, pay := range resp.GetPayments() {
		entry := PaymentEntry{
			PaymentHash:    pay.GetPaymentHash(),
			AmountSats:     pay.GetValueSat(),
			FeeSats:        pay.GetFeeSat(),
			Status:         pay.GetStatus().String(),
			CreationDate:   pay.GetCreationDate(),
			Preimage:       pay.GetPaymentPreimage(),
			PaymentRequest: pay.GetPaymentRequest(),
			IsIncoming:     false,
		}

		// Extract route hops from the successful HTLC
		for _, htlc := range pay.GetHtlcs() {
			if htlc.GetStatus() == lnrpc.HTLCAttempt_SUCCEEDED &&
				htlc.GetRoute() != nil {
				for _, hop := range htlc.GetRoute().GetHops() {
					entry.Hops = append(entry.Hops, RouteHop{
						PubKey:   hop.GetPubKey(),
						ChanID:   hop.GetChanId(),
						FeeSats:  hop.GetFee(),
						AmtToFwd: hop.GetAmtToForwardMsat() / 1000,
					})
				}
				break // use first successful route
			}
		}

		// Decode payment request for memo
		if entry.PaymentRequest != "" && c.rpc() != nil {
			decoded, err := c.DecodePayReq(entry.PaymentRequest)
			if err == nil {
				entry.Memo = decoded.Description
			}
		}

		entries = append(entries, entry)
	}
	return entries, nil
}
