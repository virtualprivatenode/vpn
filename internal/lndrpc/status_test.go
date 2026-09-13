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

type statusQueryRPC struct {
	lnrpc.LightningClient
	read func(context.Context) error
}

func (r statusQueryRPC) GetInfo(ctx context.Context, _ *lnrpc.GetInfoRequest, _ ...grpc.CallOption) (*lnrpc.GetInfoResponse, error) {
	return nil, r.read(ctx)
}
func (r statusQueryRPC) WalletBalance(ctx context.Context, _ *lnrpc.WalletBalanceRequest, _ ...grpc.CallOption) (*lnrpc.WalletBalanceResponse, error) {
	return nil, r.read(ctx)
}
func (r statusQueryRPC) ListChannels(ctx context.Context, _ *lnrpc.ListChannelsRequest, _ ...grpc.CallOption) (*lnrpc.ListChannelsResponse, error) {
	return nil, r.read(ctx)
}
func (r statusQueryRPC) PendingChannels(ctx context.Context, _ *lnrpc.PendingChannelsRequest, _ ...grpc.CallOption) (*lnrpc.PendingChannelsResponse, error) {
	return nil, r.read(ctx)
}

func TestStatusQueriesKeepCallerCancellation(t *testing.T) {
	for _, name := range []string{"info", "balance", "channels", "pending", "state"} {
		t.Run(name, func(t *testing.T) {
			parent, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			parent = metadata.AppendToOutgoingContext(parent, "trace", "status", "macaroon", "old-test-macaroon")
			entered := make(chan struct{})
			read := func(ctx context.Context) error {
				md, _ := metadata.FromOutgoingContext(ctx)
				if name != "state" && (len(md.Get("macaroon")) != 1 || md.Get("macaroon")[0] != "test-macaroon" || len(md.Get("trace")) != 1) {
					t.Error("credential attachment lost metadata")
				}
				deadline, ok := ctx.Deadline()
				parentDeadline, _ := parent.Deadline()
				if !ok || deadline.After(parentDeadline) {
					t.Error("query extended caller deadline")
				}
				close(entered)
				<-ctx.Done()
				return ctx.Err()
			}
			c := &Client{lightning: statusQueryRPC{read: read}, macaroonHex: "test-macaroon",
				state: setupStateClient{read: func(ctx context.Context) (*lnrpc.GetStateResponse, error) { return nil, read(ctx) }}}
			done := make(chan error, 1)
			go func() {
				var err error
				switch name {
				case "info":
					_, err = c.GetInfoContext(parent)
				case "balance":
					_, err = c.GetWalletBalanceContext(parent)
				case "channels":
					_, err = c.ListChannelsContext(parent)
				case "pending":
					_, err = c.GetPendingChannelsContext(parent)
				case "state":
					_, err = c.GetStateContext(parent)
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("read did not reach RPC")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancelled query remained blocked")
			}
		})
	}
}
