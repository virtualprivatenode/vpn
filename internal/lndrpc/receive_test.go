package lndrpc

import (
	"context"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type addressRPC struct {
	lnrpc.LightningClient
	ctx     context.Context
	request *lnrpc.NewAddressRequest
}

func (r *addressRPC) NewAddress(ctx context.Context, request *lnrpc.NewAddressRequest, _ ...grpc.CallOption) (*lnrpc.NewAddressResponse, error) {
	r.ctx, r.request = ctx, request
	return &lnrpc.NewAddressResponse{Address: "daemon-address"}, nil
}

func TestNewAddressUsesFreshTaprootDefaultAccountAndBoundedAuthenticatedRPC(t *testing.T) {
	rpc := &addressRPC{}
	client := &Client{lightning: rpc, macaroonHex: "test-credential"}
	started := time.Now()
	result, err := client.GetNewAddress()
	if err != nil || result.Address != "daemon-address" {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if rpc.request.Type != lnrpc.AddressType_TAPROOT_PUBKEY || rpc.request.Account != "" {
		t.Fatalf("unexpected derivation request: %v", rpc.request)
	}
	deadline, ok := rpc.ctx.Deadline()
	if !ok || !deadline.After(started) || deadline.After(time.Now().Add(30*time.Second)) {
		t.Fatal("RPC lacks the 30-second deadline")
	}
	md, _ := metadata.FromOutgoingContext(rpc.ctx)
	if got := md.Get("macaroon"); len(got) != 1 || got[0] != "test-credential" {
		t.Fatal("RPC lacks the staged authentication metadata")
	}
	if rpc.ctx.Err() != context.Canceled {
		t.Fatal("completed RPC did not release its context")
	}
}
