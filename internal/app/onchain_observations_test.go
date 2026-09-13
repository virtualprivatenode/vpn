package app

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/virtualprivatenode/vpn/internal/lndrpc"
)

type observationSource struct {
	coins func(context.Context, int32, int32) ([]lndrpc.UTXO, error)
	txs   func(context.Context) ([]lndrpc.OnChainTx, error)
}

func (s observationSource) ListUnspentContext(ctx context.Context, min, max int32) ([]lndrpc.UTXO, error) {
	return s.coins(ctx, min, max)
}
func (s observationSource) GetTransactionsContext(ctx context.Context) ([]lndrpc.OnChainTx, error) {
	return s.txs(ctx)
}

func TestOnChainReaderPartialObservationAndRetention(t *testing.T) {
	r := NewOnChainReader()
	t.Cleanup(r.Close)
	failure := errors.New("history read failed")
	result := r.Collect(observationSource{
		coins: func(ctx context.Context, min, max int32) ([]lndrpc.UTXO, error) {
			if min != 0 || max != 999999 {
				t.Error("confirmation range changed")
			}
			return nil, nil
		},
		txs: func(context.Context) ([]lndrpc.OnChainTx, error) {
			return []lndrpc.OnChainTx{{Txid: "partial"}}, failure
		},
	})
	previous := OnChainSnapshot{Utxos: observe([]lndrpc.UTXO{{Txid: "spent"}}, nil), OnChainTxs: observe([]lndrpc.OnChainTx{{Txid: "retained"}}, nil)}
	result = result.Retain(previous)
	if !result.Utxos.Fresh() || len(result.Utxos.Value) != 0 || result.OnChainTxs.Fresh() || !errors.Is(result.OnChainTxs.Err, failure) || result.OnChainTxs.Value[0].Txid != "retained" || result.OnChainTxs.ObservedAt != previous.OnChainTxs.ObservedAt {
		t.Fatalf("empty success or failed partial read handled incorrectly: %+v", result)
	}
	missing := r.Collect(nil)
	if missing.Utxos.Known() || missing.OnChainTxs.Known() || missing.Utxos.Err == nil || missing.OnChainTxs.Err == nil {
		t.Fatal("missing source became a successful empty snapshot")
	}
}

func TestOnChainReaderBoundsAndJoinsBothReads(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "close"}[shutdown], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := NewOnChainReader()
				defer r.Close()
				entered := make(chan context.Context, 2)
				block := func(ctx context.Context) error { entered <- ctx; <-ctx.Done(); return ctx.Err() }
				done := make(chan OnChainSnapshot, 1)
				go func() {
					done <- r.Collect(observationSource{
						coins: func(ctx context.Context, _, _ int32) ([]lndrpc.UTXO, error) { return nil, block(ctx) },
						txs:   func(ctx context.Context) ([]lndrpc.OnChainTx, error) { return nil, block(ctx) },
					})
				}()
				first, second := <-entered, <-entered
				for _, ctx := range []context.Context{first, second} {
					deadline, ok := ctx.Deadline()
					if !ok || time.Until(deadline) != 30*time.Second {
						t.Fatal("unbounded list read")
					}
				}
				want := context.DeadlineExceeded
				if shutdown {
					r.Close()
					want = context.Canceled
				}
				result := <-done
				if !errors.Is(result.Utxos.Err, want) || !errors.Is(result.OnChainTxs.Err, want) || first.Err() == nil || second.Err() == nil {
					t.Fatal("reader did not cancel and join both reads")
				}
				r.Close()
				late := r.Collect(observationSource{})
				if !errors.Is(late.Utxos.Err, context.Canceled) || !errors.Is(late.OnChainTxs.Err, context.Canceled) {
					t.Fatal("late command entered closed reader")
				}
			})
		})
	}
}
