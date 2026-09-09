package app

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
)

type workflowSession struct {
	step   func(int) error
	endErr error
	closed bool
}

func (s *workflowSession) WaitStep(i int) error {
	if s.step != nil {
		return s.step(i)
	}
	return nil
}
func (s *workflowSession) Wait(any) error { return s.endErr }
func (s *workflowSession) Close()         { s.closed = true }

func TestHelperWorkflowResults(t *testing.T) {
	failure := errors.New("injected failure")
	for _, tc := range []struct {
		name                     string
		kind                     HelperWorkflowKind
		stepErr, endErr, loadErr error
		succeeded                bool
	}{
		{name: "syncthing", kind: SyncthingInstall, succeeded: true},
		{name: "p2p", kind: P2PUpgrade, succeeded: true},
		{name: "self update", kind: SelfUpdate, succeeded: true},
		{name: "partial change", kind: SyncthingInstall, stepErr: failure},
		{name: "missing success terminator", kind: SelfUpdate, endErr: failure},
		{name: "reload after success", kind: P2PUpgrade, loadErr: failure, succeeded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w := NewHelperWorkflows("2.1.1")
				defer w.Close()
				session := &workflowSession{endErr: tc.endErr, step: func(i int) error {
					if i == 1 {
						return tc.stepErr
					}
					return nil
				}}
				starts, loads := 0, 0
				w.start = func(ctx context.Context, verb string, params any) (helperSession, error) {
					starts++
					switch tc.kind {
					case SyncthingInstall:
						if verb != helper.VerbSyncthingInstall || params != nil {
							t.Error("wrong Syncthing request")
						}
					case P2PUpgrade:
						if verb != helper.VerbUpgradeP2PToHybrid || params != nil {
							t.Error("wrong P2P request")
						}
					case SelfUpdate:
						if verb != helper.VerbSelfUpdate || params != (helper.SelfUpdateParams{Version: "0.7.1"}) {
							t.Error("wrong reviewed release")
						}
					}
					return session, nil
				}
				w.loadConfig = func() (*config.AppConfig, error) { loads++; return config.Default(), tc.loadErr }
				var op *HelperOperation
				switch tc.kind {
				case SyncthingInstall:
					op = w.InstallSyncthing()
				case P2PUpgrade:
					op = w.UpgradeP2P(nil)
				case SelfUpdate:
					op = w.UpdateSelf("0.7.1")
				}
				// No progress consumer runs until the application has finished. A renderer
				// must not own the pace of mutation or leave a successful session open.
				synctest.Wait()
				if !session.closed {
					t.Fatal("consumer absence stranded the session")
				}
				count, terminal := 0, 0
				var result *HelperWorkflowResult
				for event := range op.Events() {
					if event.Index != count {
						t.Fatalf("event index %d, expected %d", event.Index, count)
					}
					if event.Result != nil {
						terminal++
						result = event.Result
					} else {
						count++
					}
				}
				if starts != 1 || terminal != 1 || result == nil {
					t.Fatalf("starts=%d terminal=%d", starts, terminal)
				}
				if result.HelperSucceeded != tc.succeeded {
					t.Fatalf("wrong mutation outcome: %+v", result)
				}
				wantErr := tc.stepErr != nil || tc.endErr != nil || tc.loadErr != nil
				if (result.Err != nil) != wantErr {
					t.Fatalf("wrong error: %v", result.Err)
				}
				wantLoad := tc.kind != SelfUpdate && tc.succeeded
				if (loads == 1) != wantLoad {
					t.Fatalf("config reads=%d", loads)
				}
				if (result.Config != nil) != (wantLoad && tc.loadErr == nil) {
					t.Fatal("invalid configuration publication")
				}
				if !wantErr && count != len(op.Steps()) {
					t.Fatal("success before all steps")
				}
			})
		})
	}
}

func TestHelperWorkflowShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := NewHelperWorkflows("2.1.1")
		starts := 0
		session := &workflowSession{}
		w.start = func(ctx context.Context, _ string, _ any) (helperSession, error) {
			starts++
			session.step = func(int) error { <-ctx.Done(); return ctx.Err() }
			return session, nil
		}
		first := w.InstallSyncthing()
		synctest.Wait()
		w.Close()
		if !session.closed {
			t.Fatal("shutdown left progress reader alive")
		}
		for event := range first.Events() {
			if event.Result == nil || !errors.Is(event.Result.Err, context.Canceled) || event.Result.HelperSucceeded {
				t.Fatal("disconnect fabricated success or rollback")
			}
		}
		after := w.UpdateSelf("0.7.1")
		for event := range after.Events() {
			if event.Result == nil || !errors.Is(event.Result.Err, context.Canceled) {
				t.Fatal("operation accepted after shutdown")
			}
		}
		if starts != 1 {
			t.Fatalf("shutdown started another request: %d", starts)
		}
	})
}

type waitingP2PConnection struct{ started, stopped bool }

func (c *waitingP2PConnection) ReconnectContext(ctx context.Context) {
	c.started = true
	<-ctx.Done()
	c.stopped = true
}

func TestP2PReconnectBelongsToWorkflowLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := NewHelperWorkflows("2.1.1")
		w.start = func(context.Context, string, any) (helperSession, error) { return &workflowSession{}, nil }
		w.loadConfig = func() (*config.AppConfig, error) { return config.Default(), nil }
		client := &waitingP2PConnection{}
		op := w.UpgradeP2P(client)
		synctest.Wait()
		if !client.started {
			t.Fatal("successful P2P change did not reconnect")
		}
		w.Close()
		if !client.stopped {
			t.Fatal("shutdown did not join the reconnect")
		}
		for range op.Events() {
		}
	})
}
