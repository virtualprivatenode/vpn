package tui

// A detail keeps its identity and opening scope, never a second record cache.
// Rendering follows the same retained observations as the list.
type onChainDetail struct {
	ctx   *ScreenContext
	owner *OnChainContext
	scope walletObservationScope
	key   string
}

func newOnChainDetail(ctx *ScreenContext, key string) onChainDetail {
	return onChainDetail{ctx: ctx, owner: ctx.OnChain, scope: ctx.walletObservationScope(), key: key}
}

func (d onChainDetail) current() bool {
	return d.owner != nil && d.owner == d.ctx.OnChain &&
		d.scope == d.ctx.walletObservationScope() && d.scope == d.owner.scope
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
