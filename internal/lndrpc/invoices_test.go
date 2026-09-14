package lndrpc

import (
	"errors"
	"testing"
)

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
