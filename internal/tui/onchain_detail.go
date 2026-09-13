package tui

import "github.com/virtualprivatenode/vpn/internal/app"

// A detail keeps its identity and opening scope, never a second record cache.
// Rendering follows the same retained observations as the list.
type onChainDetail struct {
	ctx   *ScreenContext
	owner *OnChainContext
	scope onChainScope
	key   string
}

func newOnChainDetail(ctx *ScreenContext, key string) onChainDetail {
	return onChainDetail{ctx: ctx, owner: ctx.OnChain, scope: ctx.onChainScope(), key: key}
}

func (d onChainDetail) current() bool {
	return d.owner != nil && d.owner == d.ctx.OnChain &&
		d.scope == d.ctx.onChainScope() && d.scope == d.owner.scope
}

const previousOnChainDetail = "This detail belongs to a previous wallet session. Reopen it from the current list."

func onChainDetailRecord[T any](observation app.Observation[[]T], match func(T) bool) (T, string, bool) {
	if observation.Known() {
		for _, record := range observation.Value {
			if match(record) {
				notice := ""
				if observation.Err != nil {
					notice = "Showing stale data. Retrying..."
				}
				return record, notice, true
			}
		}
	}
	var zero T
	if observation.Err != nil {
		return zero, "List unavailable. Retrying...", false
	}
	if !observation.Known() {
		return zero, "Loading...", false
	}
	return zero, "Not in the latest successful list.", false
}

func onChainDetailKey(screen Screen, kind tabKind) string {
	switch s := screen.(type) {
	case *UtxoDetailScreen:
		if kind == tabUtxoDetail && s.current() {
			return s.key
		}
	case *OnChainTxScreen:
		if kind == tabOnChainTx && s.current() {
			return s.key
		}
	}
	return ""
}

func onChainDetailLabel(screen Screen) string {
	var label string
	switch s := screen.(type) {
	case *UtxoDetailScreen:
		label = s.key
		if u, _, found := s.record(); found {
			label = u.Address
		}
	case *OnChainTxScreen:
		label = s.key
		if tx, _, found := s.record(); found && tx.Label != "" {
			label = tx.Label
		}
	}
	if len(label) > 14 {
		label = label[:12] + ".."
	}
	return label
}
