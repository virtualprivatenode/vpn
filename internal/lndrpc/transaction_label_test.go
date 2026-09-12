package lndrpc

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestTransactionLabelRPCBoundary(t *testing.T) {
	var request *walletrpc.LabelTransactionRequest
	var rpcContext context.Context
	calls := 0
	var rpcErr error
	// Intercept before transport so this tests the real generated WalletKit
	// request and authenticated context without contacting a daemon.
	conn, err := grpc.NewClient("passthrough:///label-test.invalid",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, _ *grpc.ClientConn, _ grpc.UnaryInvoker, _ ...grpc.CallOption) error {
			if method != "/walletrpc.WalletKit/LabelTransaction" {
				t.Errorf("unexpected method %q", method)
			}
			calls++
			rpcContext, request = ctx, req.(*walletrpc.LabelTransactionRequest)
			return rpcErr
		}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	client := &Client{conn: conn, macaroonHex: "test-only"}
	txid, err := chainhash.NewHashFromStr("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatal(err)
	}
	input := TransactionLabelRequest{
		Txid:  *txid,
		Label: " exact label ",
	}
	started := time.Now()
	result := client.SetTransactionLabel(input)
	if result.Err != nil || !result.Submitted || calls != 1 || request.Label != input.Label || !request.Overwrite {
		t.Fatalf("result=%+v request=%v calls=%d", result, request, calls)
	}
	if got := hex.EncodeToString(request.Txid); got != "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100" {
		t.Fatalf("incorrect transaction byte order: %s", got)
	}
	deadline, ok := rpcContext.Deadline()
	if !ok || deadline.Before(started.Add(30*time.Second)) || deadline.After(time.Now().Add(30*time.Second)) {
		t.Fatal("RPC lacks its 30-second deadline")
	}
	md, _ := metadata.FromOutgoingContext(rpcContext)
	if got := md.Get("macaroon"); len(got) != 1 || got[0] != "test-only" || rpcContext.Err() != context.Canceled {
		t.Fatal("authentication or completed-context release missing")
	}
	rpcErr = errors.New("acknowledgement lost")
	result = client.SetTransactionLabel(input)
	if !result.Submitted || !errors.Is(result.Err, rpcErr) || calls != 2 {
		t.Fatal("attempted failure lost its classification or was retried")
	}
	for _, unavailable := range []*Client{nil, {}} {
		result = unavailable.SetTransactionLabel(input)
		if result.Submitted || !errors.Is(result.Err, errNotConnected) {
			t.Fatal("missing connection was not a pre-submission refusal")
		}
	}
}
