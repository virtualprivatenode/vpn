package lndrpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type invoiceLookupRPC struct {
	lnrpc.LightningClient
	invoice *lnrpc.Invoice
	err     error
	hash    []byte
}

func (r *invoiceLookupRPC) LookupInvoice(_ context.Context, req *lnrpc.PaymentHash, _ ...grpc.CallOption) (*lnrpc.Invoice, error) {
	r.hash = append([]byte(nil), req.RHash...)
	return r.invoice, r.err
}

func TestInvoiceLookupDistinguishesAbsenceFromFailure(t *testing.T) {
	hash := bytes.Repeat([]byte{7}, 32)
	for _, tc := range []struct {
		name    string
		err     error
		missing bool
	}{
		{"not found", status.Error(codes.NotFound, "gone"), true},
		{"wrapped not found", fmt.Errorf("lookup: %w", status.Error(codes.NotFound, "gone")), true},
		{"deadline", status.Error(codes.DeadlineExceeded, "lookup timed out"), false},
		{"canceled lookup", status.Error(codes.Canceled, "lookup canceled"), false},
		{"untyped message", errors.New("NotFound: invoice not found"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rpc := &invoiceLookupRPC{err: tc.err}
			client := &Client{lightning: rpc}
			invoice, err := client.LookupInvoice(hash)
			if invoice != nil || !bytes.Equal(rpc.hash, hash) || errors.Is(err, ErrInvoiceNotFound) != tc.missing {
				t.Fatalf("lookup lost identity or misclassified failure: %v", err)
			}
			if !tc.missing && err != tc.err {
				t.Fatal("lookup discarded the original failure")
			}
		})
	}
}

func TestInvoiceLookupPreservesCanceledAndSettledOutcomesPastExpiry(t *testing.T) {
	for _, tc := range []struct {
		state                      lnrpc.Invoice_InvoiceState
		settled, canceled, expired bool
	}{
		{lnrpc.Invoice_OPEN, false, false, true},
		{lnrpc.Invoice_ACCEPTED, false, false, false},
		{lnrpc.Invoice_CANCELED, false, true, false},
		{lnrpc.Invoice_SETTLED, true, false, false},
	} {
		t.Run(tc.state.String(), func(t *testing.T) {
			rpc := &invoiceLookupRPC{invoice: &lnrpc.Invoice{
				State: tc.state, CreationDate: time.Now().Add(-time.Hour).Unix(), Expiry: 60,
			}}
			client := &Client{lightning: rpc}
			invoice, err := client.LookupInvoice(bytes.Repeat([]byte{7}, 32))
			if err != nil || invoice.Settled != tc.settled || invoice.Canceled != tc.canceled || invoice.IsExpired != tc.expired {
				t.Fatalf("native outcome replaced by inferred expiry: invoice=%+v err=%v", invoice, err)
			}
		})
	}
}

func TestDisconnectedClientInvoiceMethods(t *testing.T) {
	c := &Client{}
	if _, err := c.AddInvoice(1000, "test", false); err == nil {
		t.Error("should error")
	}
	if _, err := c.DecodePayReq("lnbc..."); err == nil {
		t.Error("should error")
	}
	if _, err := c.LookupInvoice([]byte{1, 2, 3}); err == nil {
		t.Error("should error")
	}
	if entries, err := c.ListInvoicesContext(t.Context(), 10); !errors.Is(err, errNotConnected) || entries != nil {
		t.Error("disconnected invoice read must be unavailable")
	}
	if entries, err := c.ListPaymentsContext(t.Context(), 10); !errors.Is(err, errNotConnected) || entries != nil {
		t.Error("disconnected payment read must be unavailable")
	}
	if _, err := c.SendPayment("lnbc..."); err == nil {
		t.Error("should error")
	}
}
