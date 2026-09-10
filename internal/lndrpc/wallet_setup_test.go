package lndrpc

import (
	"context"
	"errors"
	"testing"

	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
)

type setupStateClient struct {
	lnrpc.StateClient
	read func(context.Context) (*lnrpc.GetStateResponse, error)
}

func (s setupStateClient) GetState(ctx context.Context, _ *lnrpc.GetStateRequest, _ ...grpc.CallOption) (*lnrpc.GetStateResponse, error) {
	return s.read(ctx)
}

func TestWalletSetupStateDoesNotInventAbsence(t *testing.T) {
	failure := errors.New("state unavailable")
	for _, tc := range []struct {
		response *lnrpc.GetStateResponse
		err      error
		want     WalletState
	}{
		{nil, nil, WalletStateUnknown},
		{nil, failure, WalletStateUnknown},
		{&lnrpc.GetStateResponse{State: lnrpc.WalletState_NON_EXISTING}, nil, "NON_EXISTING"},
		{&lnrpc.GetStateResponse{State: lnrpc.WalletState_WAITING_TO_START}, nil, "WAITING_TO_START"},
		{&lnrpc.GetStateResponse{State: lnrpc.WalletState(99)}, nil, "99"},
	} {
		state, err := readWalletSetupState(t.Context(), setupStateClient{read: func(context.Context) (*lnrpc.GetStateResponse, error) { return tc.response, tc.err }})
		if state != tc.want || (err == nil) != (tc.response != nil && tc.err == nil) {
			t.Fatalf("state=%q err=%v", state, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	state, err := ReadWalletSetupState(ctx)
	if state != WalletStateUnknown || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled probe: %q %v", state, err)
	}
}
