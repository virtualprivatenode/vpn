package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/virtualprivatenode/vpn/internal/lndrpc"
)

type OnChainSource interface {
	ListUnspentContext(context.Context, int32, int32) ([]lndrpc.UTXO, error)
	GetTransactionsContext(context.Context) ([]lndrpc.OnChainTx, error)
}

type OnChainSnapshot struct {
	Utxos      Observation[[]lndrpc.UTXO]
	OnChainTxs Observation[[]lndrpc.OnChainTx]
}

func (s OnChainSnapshot) Retain(previous OnChainSnapshot) OnChainSnapshot {
	s.Utxos = retain(s.Utxos, previous.Utxos)
	s.OnChainTxs = retain(s.OnChainTxs, previous.OnChainTxs)
	return s
}

func (s OnChainSnapshot) Unavailable(err error) OnChainSnapshot {
	s.Utxos.Err, s.OnChainTxs.Err = err, err
	return s
}

// OnChainReader owns local read lifetimes. Scheduling and same-wallet retention
// belong to its caller; the two reads do not form an atomic wallet snapshot.
type OnChainReader struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
	calls  sync.WaitGroup
}

func NewOnChainReader() *OnChainReader {
	ctx, cancel := context.WithCancel(context.Background())
	return &OnChainReader{ctx: ctx, cancel: cancel}
}

// Close cancels and joins local reads. Shared LND reconnect mutex waits can
// still delay the join; this owner does not change connection recovery.
func (r *OnChainReader) Close() {
	r.mu.Lock()
	r.closed = true
	r.cancel()
	r.mu.Unlock()
	r.calls.Wait()
}

func (r *OnChainReader) Collect(source OnChainSource) OnChainSnapshot {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return OnChainSnapshot{}.Unavailable(context.Canceled)
	}
	r.calls.Add(1)
	r.mu.Unlock()
	defer r.calls.Done()
	if source == nil {
		return OnChainSnapshot{}.Unavailable(errors.New("lnd not connected"))
	}
	ctx, cancel := context.WithTimeout(r.ctx, 30*time.Second)
	defer cancel()
	var result OnChainSnapshot
	var reads sync.WaitGroup
	reads.Go(func() {
		result.Utxos = observe(source.ListUnspentContext(ctx, 0, 999999))
	})
	reads.Go(func() {
		result.OnChainTxs = observe(source.GetTransactionsContext(ctx))
	})
	reads.Wait()
	return result
}
