package lndrpc

import (
	"testing"
)

func TestInvoiceFields(t *testing.T) {
	inv := &Invoice{
		AmountSats: 1000,
		Memo:       "test payment",
		Settled:    false,
	}
	if inv.AmountSats != 1000 {
		t.Errorf("AmountSats: got %d", inv.AmountSats)
	}
	if inv.Memo != "test payment" {
		t.Errorf("Memo: got %q", inv.Memo)
	}
	if inv.Settled {
		t.Error("should not be settled")
	}
}

func TestDecodedPayReqFields(t *testing.T) {
	d := &DecodedPayReq{
		Destination: "03abc...",
		AmountSats:  50000,
		Description: "Coffee",
		IsExpired:   false,
	}
	if d.AmountSats != 50000 {
		t.Errorf("AmountSats: got %d", d.AmountSats)
	}
	if d.Description != "Coffee" {
		t.Errorf("Description: got %q", d.Description)
	}
	if d.IsExpired {
		t.Error("should not be expired")
	}
}

func TestPaymentEntryFields(t *testing.T) {
	e := PaymentEntry{
		PaymentHash: "abc123",
		AmountSats:  10000,
		FeeSats:     15,
		Status:      "SUCCEEDED",
		IsIncoming:  false,
		Preimage:    "preimage123",
		Memo:        "sent to friend",
		Hops: []RouteHop{
			{PubKey: "03aaa", Alias: "ACINQ", FeeSats: 10},
			{PubKey: "03bbb", Alias: "Dest", FeeSats: 5},
		},
	}
	if len(e.Hops) != 2 {
		t.Errorf("Hops: got %d, want 2", len(e.Hops))
	}
	if e.Hops[0].Alias != "ACINQ" {
		t.Errorf("Hop[0].Alias: got %q", e.Hops[0].Alias)
	}
	if e.FeeSats != 15 {
		t.Errorf("FeeSats: got %d", e.FeeSats)
	}
}

func TestRouteHopFields(t *testing.T) {
	hop := RouteHop{
		PubKey:   "03abc123",
		Alias:    "TestNode",
		ChanID:   839201214501,
		FeeSats:  10,
		AmtToFwd: 50000,
	}
	if hop.Alias != "TestNode" {
		t.Errorf("Alias: got %q", hop.Alias)
	}
	if hop.AmtToFwd != 50000 {
		t.Errorf("AmtToFwd: got %d", hop.AmtToFwd)
	}
}

func TestSendPaymentResultFields(t *testing.T) {
	r := &SendPaymentResult{
		Preimage: "abc123",
		FeeSats:  25,
		Status:   "SUCCEEDED",
		Hops: []RouteHop{
			{Alias: "Hop1"},
			{Alias: "Hop2"},
			{Alias: "Dest"},
		},
	}
	if r.Status != "SUCCEEDED" {
		t.Errorf("Status: got %q", r.Status)
	}
	if len(r.Hops) != 3 {
		t.Errorf("Hops: got %d, want 3", len(r.Hops))
	}
}

func TestSendPaymentResultFailed(t *testing.T) {
	r := &SendPaymentResult{
		Status: "FAILED",
		Error:  "FAILURE_REASON_NO_ROUTE",
	}
	if r.Error == "" {
		t.Error("failed result should have error")
	}
}

func TestNilClientInvoiceMethods(t *testing.T) {
	c := &Client{}
	if _, err := c.AddInvoice(1000, "test"); err == nil {
		t.Error("should error")
	}
	if _, err := c.DecodePayReq("lnbc..."); err == nil {
		t.Error("should error")
	}
	if _, err := c.LookupInvoice([]byte{1, 2, 3}); err == nil {
		t.Error("should error")
	}
	if _, err := c.ListInvoices(10); err == nil {
		t.Error("should error")
	}
	if _, err := c.ListPayments(10); err == nil {
		t.Error("should error")
	}
	if _, err := c.SendPayment("lnbc..."); err == nil {
		t.Error("should error")
	}
}

func TestPaymentEntryIncoming(t *testing.T) {
	e := PaymentEntry{IsIncoming: true, Status: "SETTLED"}
	if !e.IsIncoming {
		t.Error("should be incoming")
	}
}

func TestPaymentEntryOutgoing(t *testing.T) {
	e := PaymentEntry{IsIncoming: false, Status: "SUCCEEDED"}
	if e.IsIncoming {
		t.Error("should be outgoing")
	}
}
