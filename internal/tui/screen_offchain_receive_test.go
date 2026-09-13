package tui

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/lndrpc"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

type screenInvoiceClient struct {
	requests []app.InvoiceRequest
	hashes   []string
	status   *lndrpc.Invoice
	err      error
}

func (c *screenInvoiceClient) AddInvoice(amount int64, memo string, blind bool) (*lndrpc.Invoice, error) {
	c.requests = append(c.requests, app.InvoiceRequest{AmountSats: amount, Memo: memo, Blinded: blind})
	return &lndrpc.Invoice{
		PaymentRequest: fmt.Sprintf("lntbs1invoice%d", len(c.requests)),
		PaymentHash:    fmt.Sprintf("%064x", len(c.requests)), AmountSats: amount,
	}, c.err
}

func (c *screenInvoiceClient) LookupInvoice(hash []byte) (*lndrpc.Invoice, error) {
	c.hashes = append(c.hashes, hex.EncodeToString(hash))
	return c.status, c.err
}

func receiveScreen(client *screenInvoiceClient) *ReceiveScreen {
	s := NewReceiveScreen(&ScreenContext{Cfg: config.Default(), State: &RuntimeState{WalletKnown: true, WalletExists: true}})
	s.invoices = client
	s.amountInput.SetSats(42)
	s.memoInput.SetValue("coffee")
	s.focusZone = recvZoneButtons
	return s
}

func TestReceiveCreatesOnceAndMonitorsTheCreatedInvoice(t *testing.T) {
	client := &screenInvoiceClient{}
	s := receiveScreen(client)
	_, create := s.HandleKey("enter", tea.KeyPressMsg{})
	if create == nil {
		t.Fatal("creation was not scheduled")
	}
	if _, duplicate := s.HandleKey("enter", tea.KeyPressMsg{}); duplicate != nil {
		t.Fatal("repeated Enter scheduled another invoice")
	}
	s.HandleMsg(tea.PasteMsg{Content: "999"})
	_, check := s.HandleMsg(create())
	if check == nil || len(client.requests) != 1 || client.requests[0] != (app.InvoiceRequest{AmountSats: 42, Memo: "coffee", Blinded: true}) {
		t.Fatalf("invoice submission changed: %+v", client.requests)
	}
	check()
	if len(client.hashes) != 1 || client.hashes[0] != fmt.Sprintf("%064x", 1) {
		t.Fatalf("monitored the wrong invoice: %v", client.hashes)
	}
}

func TestReceiveRejectsResultsFromReplacedScreen(t *testing.T) {
	client := &screenInvoiceClient{status: &lndrpc.Invoice{Settled: true}}
	old := receiveScreen(client)
	_, createOld := old.submitInvoice()
	oldCreated := createOld()
	_, checkOld := old.HandleMsg(oldCreated)
	current := receiveScreen(client)
	_, createCurrent := current.submitInvoice()
	if _, cmd := current.HandleMsg(oldCreated); cmd != nil || current.step != recvStepCreating {
		t.Fatal("old creation reached the replacement screen")
	}
	_, checkCurrent := current.HandleMsg(createCurrent())
	if _, cmd := current.HandleMsg(checkOld()); cmd != nil || current.step != recvStepWaiting {
		t.Fatal("old settlement completed the replacement invoice")
	}
	if _, cmd := current.HandleMsg(invoiceCheckMsg{attempt: old.attempt}); cmd != nil {
		t.Fatal("old timer scheduled another lookup")
	}
	current.HandleMsg(checkCurrent())
	if current.step != recvStepPaid {
		t.Fatal("current settlement was not accepted")
	}
}

func TestReceiveContinuesAfterPendingOrUnavailableStatus(t *testing.T) {
	theme.Init(true)
	client := &screenInvoiceClient{err: errors.New("no blinded routes to self")}
	s := receiveScreen(client)
	_, create := s.submitInvoice()
	s.HandleMsg(create())
	if s.step != recvStepInput || !strings.Contains(s.inputError, "turning off blinded paths") {
		t.Fatal("creation failure did not allow correcting the request")
	}
	client.err = nil
	_, create = s.submitInvoice()
	_, check := s.HandleMsg(create())
	for _, lookupError := range []error{errors.New("connection lost"), nil} {
		client.err = lookupError
		client.status = &lndrpc.Invoice{}
		_, next := s.HandleMsg(check())
		if s.step != recvStepWaiting || next == nil {
			t.Fatal("pending or unavailable status stopped monitoring")
		}
		view := s.View(82, 34)
		if strings.Contains(view, "Invoice Expired") || strings.Contains(view, "Payment Received") || strings.Contains(view, "Receive Failed") {
			t.Fatal("lookup invented a terminal outcome")
		}
		if strings.Contains(view, "connection lost") != (lookupError != nil) {
			t.Fatal("lookup warning did not reflect the latest check")
		}
		_, check = s.HandleMsg(next())
		if check == nil {
			t.Fatal("next lookup was not scheduled")
		}
		if _, duplicate := s.HandleMsg(invoiceCheckMsg{attempt: s.attempt}); duplicate != nil {
			t.Fatal("overlapping lookup scheduled")
		}
	}
	client.status = &lndrpc.Invoice{Settled: true}
	if _, next := s.HandleMsg(check()); next != nil || s.step != recvStepPaid {
		t.Fatal("settlement did not stop monitoring")
	}
}

func TestReceiveExpiryStopsMonitoring(t *testing.T) {
	client := &screenInvoiceClient{status: &lndrpc.Invoice{IsExpired: true}}
	s := receiveScreen(client)
	_, create := s.submitInvoice()
	_, check := s.HandleMsg(create())
	if _, next := s.HandleMsg(check()); next != nil || s.step != recvStepExpired {
		t.Fatal("expiry did not stop monitoring")
	}
	if _, next := s.HandleMsg(invoiceCheckMsg{attempt: s.attempt}); next != nil {
		t.Fatal("expired invoice restarted monitoring")
	}
}

func TestReceiveMonitoringSurvivesSectionChangeAndStopsOnClose(t *testing.T) {
	client := &screenInvoiceClient{status: &lndrpc.Invoice{}}
	s := receiveScreen(client)
	m := Model{nav: NewNavSidebar(), screenCtx: s.ctx,
		tabs: []openTab{{Kind: tabReceive, Section: secWallet, Screen: s}}}
	_, create := s.submitInvoice()
	// The default sidebar section is Channels, while this invoice belongs to Wallet.
	updated, check := m.Update(create())
	m = updated.(Model)
	if check == nil || s.step != recvStepWaiting {
		t.Fatal("hidden receive tab lost its creation result")
	}
	updated, next := m.Update(check())
	m = updated.(Model)
	if next == nil {
		t.Fatal("hidden receive tab stopped polling")
	}
	updated, inFlight := m.Update(invoiceCheckMsg{attempt: s.attempt})
	m = updated.(Model)
	if inFlight == nil {
		t.Fatal("hidden tab lost its next check")
	}
	m.nav.SetActive(secWallet)
	closed, _ := m.closeTab(1)
	m = closed.(Model)
	if _, cmd := m.Update(invoiceCheckMsg{attempt: s.attempt}); cmd != nil {
		t.Fatal("closed tab started another lookup")
	}
	if _, cmd := m.Update(inFlight()); cmd != nil {
		t.Fatal("lookup finishing after close restarted monitoring")
	}
	if len(client.hashes) != 2 {
		t.Fatal("unexpected lookup after close")
	}
}
