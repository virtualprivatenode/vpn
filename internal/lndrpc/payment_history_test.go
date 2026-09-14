package lndrpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type historyRPC struct {
	lnrpc.LightningClient
	invoices       *lnrpc.ListInvoiceResponse
	payments       *lnrpc.ListPaymentsResponse
	invoiceRequest *lnrpc.ListInvoiceRequest
	paymentRequest *lnrpc.ListPaymentsRequest
	contexts       []context.Context
	decode         func(context.Context) (*lnrpc.PayReq, error)
	alias          func(context.Context) (*lnrpc.NodeInfo, error)
}

func (r *historyRPC) ListInvoices(ctx context.Context, req *lnrpc.ListInvoiceRequest, _ ...grpc.CallOption) (*lnrpc.ListInvoiceResponse, error) {
	r.contexts = append(r.contexts, ctx)
	r.invoiceRequest = req
	return r.invoices, nil
}
func (r *historyRPC) ListPayments(ctx context.Context, req *lnrpc.ListPaymentsRequest, _ ...grpc.CallOption) (*lnrpc.ListPaymentsResponse, error) {
	r.contexts = append(r.contexts, ctx)
	r.paymentRequest = req
	return r.payments, nil
}
func (r *historyRPC) DecodePayReq(ctx context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
	r.contexts = append(r.contexts, ctx)
	return r.decode(ctx)
}
func (r *historyRPC) GetNodeInfo(ctx context.Context, _ *lnrpc.NodeInfoRequest, _ ...grpc.CallOption) (*lnrpc.NodeInfo, error) {
	r.contexts = append(r.contexts, ctx)
	return r.alias(ctx)
}

func TestHistoryRequestsIdentityAmountsAndStates(t *testing.T) {
	now := time.Now().Unix()
	rpc := &historyRPC{invoices: &lnrpc.ListInvoiceResponse{Invoices: []*lnrpc.Invoice{
		{AddIndex: 1, RHash: []byte{1}, Value: 1000, AmtPaidSat: 1200, State: lnrpc.Invoice_SETTLED},
		{AddIndex: 2, RHash: []byte{2}, Value: 0, AmtPaidSat: 900, State: lnrpc.Invoice_SETTLED},
		{AddIndex: 3, Value: 800, AmtPaidSat: 100, State: lnrpc.Invoice_OPEN, CreationDate: now, Expiry: 3600},
		{AddIndex: 4, Value: 700, State: lnrpc.Invoice_OPEN, CreationDate: now - 7200, Expiry: 3600},
		{AddIndex: 5, Value: 600, State: lnrpc.Invoice_ACCEPTED},
		{AddIndex: 6, Value: 500, State: lnrpc.Invoice_CANCELED},
	}}, payments: &lnrpc.ListPaymentsResponse{Payments: []*lnrpc.Payment{
		{PaymentIndex: 1, PaymentHash: "same-hash", ValueSat: 100, Status: lnrpc.Payment_IN_FLIGHT},
		{PaymentIndex: 2, PaymentHash: "same-hash", ValueSat: 200, FeeSat: 3, Status: lnrpc.Payment_SUCCEEDED, CreationTimeNs: 123000000000, PaymentPreimage: "proof"},
	}}}
	c := &Client{lightning: rpc, macaroonHex: "test-auth"}
	invoices, err := c.ListInvoicesContext(t.Context(), 50)
	if err != nil {
		t.Fatal(err)
	}
	amounts := []int64{1200, 900, 800, 700, 600, 500}
	states := []string{"SETTLED", "SETTLED", "OPEN", "EXPIRED", "ACCEPTED", "CANCELED"}
	if len(invoices) != len(amounts) {
		t.Fatalf("invoice count %d", len(invoices))
	}
	for i, e := range invoices {
		if e.Index != uint64(i+1) || !e.IsIncoming || e.AmountSats != amounts[i] || e.Status != states[i] {
			t.Fatalf("wrong invoice mapping: %+v", e)
		}
	}
	payments, err := c.ListPaymentsContext(t.Context(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(payments) != 2 || payments[0].Index != 1 || payments[0].Status != "IN_FLIGHT" || payments[1].Index != 2 || payments[1].IsIncoming || payments[1].FeeSats != 3 || payments[1].Preimage != "proof" || payments[1].CreationDate != 123 {
		t.Fatal("payment identity or meaning changed")
	}
	if rpc.invoiceRequest.NumMaxInvoices != 50 || !rpc.invoiceRequest.Reversed || rpc.invoiceRequest.PendingOnly || rpc.paymentRequest.MaxPayments != 50 || !rpc.paymentRequest.Reversed || !rpc.paymentRequest.IncludeIncomplete {
		t.Fatal("history window request changed")
	}
	for _, ctx := range rpc.contexts {
		md, _ := metadata.FromOutgoingContext(ctx)
		if got := md.Get("macaroon"); len(got) != 1 || got[0] != "test-auth" {
			t.Fatal("missing authentication")
		}
		if _, ok := ctx.Deadline(); !ok || ctx.Err() != context.Canceled {
			t.Fatal("missing deadline or released context")
		}
	}
}

func TestHistoryEnrichmentKeepsCallerCancellation(t *testing.T) {
	for _, phase := range []string{"alias", "memo"} {
		t.Run(phase, func(t *testing.T) {
			parent, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			pay := &lnrpc.Payment{PaymentIndex: 1, ValueSat: 100, Status: lnrpc.Payment_SUCCEEDED, PaymentRequest: "invoice", Htlcs: []*lnrpc.HTLCAttempt{{Status: lnrpc.HTLCAttempt_SUCCEEDED, Route: &lnrpc.Route{Hops: []*lnrpc.Hop{{PubKey: "peer"}}}}}}
			check := func(ctx context.Context) {
				t.Helper()
				pd, _ := parent.Deadline()
				cd, ok := ctx.Deadline()
				if !ok || cd.After(pd) {
					t.Fatal("enrichment reset caller deadline")
				}
			}
			rpc := &historyRPC{payments: &lnrpc.ListPaymentsResponse{Payments: []*lnrpc.Payment{pay}}}
			rpc.alias = func(ctx context.Context) (*lnrpc.NodeInfo, error) {
				check(ctx)
				return nil, errors.New("optional alias unavailable")
			}
			rpc.decode = func(ctx context.Context) (*lnrpc.PayReq, error) {
				check(ctx)
				return nil, errors.New("optional memo unavailable")
			}
			c := &Client{lightning: rpc}
			got, err := c.ListPaymentsContext(parent, 50)
			if err != nil || len(got) != 1 || got[0].Hops[0].PubKey != "peer" {
				t.Fatal("optional enrichment failure lost payment")
			}
			if phase == "alias" {
				rpc.alias = func(ctx context.Context) (*lnrpc.NodeInfo, error) { check(ctx); cancel(); return nil, ctx.Err() }
			} else {
				rpc.decode = func(ctx context.Context) (*lnrpc.PayReq, error) { check(ctx); cancel(); return nil, ctx.Err() }
			}
			if got, err := c.ListPaymentsContext(parent, 50); !errors.Is(err, context.Canceled) || got != nil {
				t.Fatal("canceled enrichment published successful partial history")
			}
		})
	}
}
