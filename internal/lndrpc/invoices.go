package lndrpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── Data types ───────────────────────────────────────────

type Invoice struct {
	PaymentRequest string
	PaymentHash    string
	AmountSats     int64
	Memo           string
	Settled        bool
	Canceled       bool
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
	Index          uint64 // LND invoice add index or outgoing payment index.
	PaymentHash    string
	AmountSats     int64
	FeeSats        int64
	Status         string // Native payment/invoice state, with EXPIRED derived for open invoices.
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

// AddInvoice creates a new Lightning invoice. When blind
// is true, LND generates blinded paths that hide the node's
// pubkey from the sender. Requires at least one active
// channel with sufficient inbound capacity.
func (c *Client) AddInvoice(amountSats int64, memo string, blind bool) (*Invoice, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtx(defaultTimeout)
	defer cancel()

	resp, err := rpc.AddInvoice(ctx, &lnrpc.Invoice{
		Value:     amountSats,
		Memo:      memo,
		Private:   !blind,
		IsBlinded: blind,
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
	return c.decodePayReqContext(context.Background(), payReq)
}

func (c *Client) decodePayReqContext(parent context.Context, payReq string) (*DecodedPayReq, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtxFrom(parent, defaultTimeout)
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

// ErrInvoiceNotFound means LND has no record for the queried payment hash.
var ErrInvoiceNotFound = errors.New("invoice not found")

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
		if status.Code(err) == codes.NotFound {
			return nil, ErrInvoiceNotFound
		}
		c.handleError(err)
		return nil, err
	}

	// An overdue OPEN observation can precede LND's cancellation and cleanup.
	settled := resp.GetState() == lnrpc.Invoice_SETTLED
	isExpired := false
	if resp.GetState() == lnrpc.Invoice_OPEN {
		expiry := resp.GetExpiry()
		if expiry > 0 {
			now := time.Now().Unix()
			isExpired = (resp.GetCreationDate() + expiry) < now
		}
	}

	return &Invoice{
		PaymentRequest: resp.GetPaymentRequest(),
		PaymentHash:    fmt.Sprintf("%x", resp.GetRHash()),
		AmountSats:     resp.GetValue(),
		Memo:           resp.GetMemo(),
		Settled:        settled,
		Canceled:       resp.GetState() == lnrpc.Invoice_CANCELED,
		CreationDate:   resp.GetCreationDate(),
		SettleDate:     resp.GetSettleDate(),
		Expiry:         resp.GetExpiry(),
		IsExpired:      isExpired,
	}, nil
}

// ── Invoice listing ──────────────────────────────────────

// ListInvoicesContext returns the recent invoice window, including unsettled invoices.
func (c *Client) ListInvoicesContext(parent context.Context, limit uint64) ([]PaymentEntry, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtxFrom(parent, defaultTimeout)
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
		if inv.GetValue() == 0 &&
			inv.GetState() != lnrpc.Invoice_SETTLED {
			continue // skip zero-amount unsettled
		}

		var status string
		switch inv.GetState() {
		case lnrpc.Invoice_SETTLED:
			status = "SETTLED"
		case lnrpc.Invoice_CANCELED:
			status = "CANCELED"
		case lnrpc.Invoice_ACCEPTED:
			status = "ACCEPTED"
		case lnrpc.Invoice_OPEN:
			// Account for expiry before LND's watcher cancels and cleans up.
			status = "OPEN"
			now := time.Now().Unix()
			expiry := inv.GetExpiry()
			if expiry > 0 &&
				(inv.GetCreationDate()+expiry) < now {
				status = "EXPIRED"
			}
		default:
			status = inv.GetState().String()
		}

		amount := inv.GetValue()
		if inv.GetState() == lnrpc.Invoice_SETTLED {
			amount = inv.GetAmtPaidSat()
		}
		entries = append(entries, PaymentEntry{
			Index:        inv.GetAddIndex(),
			PaymentHash:  fmt.Sprintf("%x", inv.GetRHash()),
			AmountSats:   amount,
			Status:       status,
			CreationDate: inv.GetCreationDate(),
			IsIncoming:   true,
			Memo:         inv.GetMemo(),
		})
	}
	return entries, nil
}

// ── Payment listing ──────────────────────────────────────

// ListPaymentsContext returns recent outgoing payments, including incomplete attempts.
func (c *Client) ListPaymentsContext(parent context.Context, limit uint64) ([]PaymentEntry, error) {
	rpc := c.rpc()
	if rpc == nil {
		return nil, errNotConnected
	}
	ctx, cancel := c.callCtxFrom(parent, defaultTimeout)
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
			Index:          pay.GetPaymentIndex(),
			PaymentHash:    pay.GetPaymentHash(),
			AmountSats:     pay.GetValueSat(),
			FeeSats:        pay.GetFeeSat(),
			Status:         pay.GetStatus().String(),
			CreationDate:   pay.GetCreationTimeNs() / 1_000_000_000,
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
						FeeSats:  hop.GetFeeMsat() / 1000,
						AmtToFwd: hop.GetAmtToForwardMsat() / 1000,
					})
				}
				break // use first successful route
			}
		}
		for i := range entry.Hops {
			entry.Hops[i].Alias = c.getPeerAliasContext(ctx, entry.Hops[i].PubKey)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}

		// Decode payment request for memo
		if entry.PaymentRequest != "" {
			decoded, err := c.decodePayReqContext(ctx, entry.PaymentRequest)
			if err == nil {
				entry.Memo = decoded.Description
			}
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
