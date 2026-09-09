package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
)

type P2PConnection interface{ ReconnectContext(context.Context) }

type HelperWorkflowKind int

const (
	SyncthingInstall HelperWorkflowKind = iota
	P2PUpgrade
	SelfUpdate
)

// HelperWorkflowResult distinguishes successful mutation from a failed local
// configuration read. An error never establishes that earlier changes were undone.
type HelperWorkflowResult struct {
	HelperSucceeded bool
	Config          *config.AppConfig
	Err             error
}

// HelperProgress reports one completed step, or the terminal result at Index.
// The terminal event occupies the next index and is emitted exactly once.
type HelperProgress struct {
	Index  int
	Result *HelperWorkflowResult
}

type HelperOperation struct {
	kind   HelperWorkflowKind
	names  []string
	events chan HelperProgress
}

func (o *HelperOperation) Kind() HelperWorkflowKind      { return o.kind }
func (o *HelperOperation) Steps() []string               { return append([]string(nil), o.names...) }
func (o *HelperOperation) Events() <-chan HelperProgress { return o.events }

type helperSession interface {
	WaitStep(int) error
	Wait(any) error
	Close()
}

// HelperWorkflows owns this client's streaming operations. Close cancels local
// observation and waits for its readers to release their connections. The helper
// retains authority to finish accepted operations, including after this TUI exits.
type HelperWorkflows struct {
	syncthingVersion string
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.Mutex
	workers          sync.WaitGroup
	start            func(context.Context, string, any) (helperSession, error)
	loadConfig       func() (*config.AppConfig, error)
}

func NewHelperWorkflows(syncthingVersion string) *HelperWorkflows {
	ctx, cancel := context.WithCancel(context.Background())
	return &HelperWorkflows{
		ctx: ctx, cancel: cancel, syncthingVersion: syncthingVersion,
		start: func(ctx context.Context, verb string, params any) (helperSession, error) {
			return helper.StartContext(ctx, verb, params)
		},
		loadConfig: config.Load,
	}
}

func (w *HelperWorkflows) Close() {
	w.mu.Lock()
	w.cancel()
	w.mu.Unlock()
	w.workers.Wait()
}

func (w *HelperWorkflows) InstallSyncthing() *HelperOperation {
	return w.begin(SyncthingInstall, helper.VerbSyncthingInstall, nil,
		helper.SyncthingInstallStepNames(w.syncthingVersion), nil)
}

func (w *HelperWorkflows) UpgradeP2P(client P2PConnection) *HelperOperation {
	return w.begin(P2PUpgrade, helper.VerbUpgradeP2PToHybrid, nil,
		helper.UpgradeP2PToHybridStepNames(), client)
}

func (w *HelperWorkflows) UpdateSelf(version string) *HelperOperation {
	return w.begin(SelfUpdate, helper.VerbSelfUpdate,
		helper.SelfUpdateParams{Version: version}, helper.SelfUpdateStepNames(version), nil)
}

func (w *HelperWorkflows) begin(kind HelperWorkflowKind, verb string, params any, names []string, client P2PConnection) *HelperOperation {
	rootSteps := len(names)
	if kind != SelfUpdate {
		names = append(names, "Reloading node configuration")
	}
	// Every possible event fits even if the consumer disappears. Rendering never
	// controls helper execution, and a stopped TUI cannot strand the reader.
	op := &HelperOperation{kind: kind, names: names, events: make(chan HelperProgress, len(names)+1)}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ctx.Err(); err != nil {
		op.events <- HelperProgress{Result: &HelperWorkflowResult{Err: err}}
		close(op.events)
		return op
	}
	w.workers.Go(func() {
		defer close(op.events)
		result, index := w.run(op, verb, params, rootSteps)
		if result.Err == nil && client != nil && w.ctx.Err() == nil {
			client.ReconnectContext(w.ctx)
		}
		op.events <- HelperProgress{Index: index, Result: &result}
	})
	return op
}

func (w *HelperWorkflows) run(op *HelperOperation, verb string, params any, rootSteps int) (HelperWorkflowResult, int) {
	var result HelperWorkflowResult
	if err := w.ctx.Err(); err != nil {
		result.Err = err
		return result, 0
	}
	session, err := w.start(w.ctx, verb, params)
	if err != nil {
		result.Err = err
		return result, 0
	}
	defer session.Close()
	for i := range rootSteps {
		if err := session.WaitStep(i); err != nil {
			result.Err = err
			return result, i
		}
		// A final step event is not the operation's success terminator.
		if i == rootSteps-1 {
			if err := session.Wait(nil); err != nil {
				result.Err = err
				return result, i
			}
			result.HelperSucceeded = true
		}
		op.events <- HelperProgress{Index: i}
	}
	if op.kind != SelfUpdate {
		if err := w.ctx.Err(); err != nil {
			result.Err = err
			return result, rootSteps
		}
		fresh, err := w.loadConfig()
		if err != nil {
			result.Err = fmt.Errorf("reload node configuration: %w", err)
			return result, rootSteps
		}
		result.Config = fresh
		op.events <- HelperProgress{Index: rootSteps}
	}
	return result, len(op.names)
}
