package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/lndrpc"
)

type statusReadCall struct {
	cfg    config.AppConfig
	result chan app.StatusSnapshot
}
type controlledStatusReader struct {
	calls chan statusReadCall
	done  <-chan struct{}
}

func (r *controlledStatusReader) Collect(cfg config.AppConfig, _ bool, _ app.StatusLND) app.StatusSnapshot {
	call := statusReadCall{cfg: cfg, result: make(chan app.StatusSnapshot, 1)}
	r.calls <- call
	select {
	case result := <-call.result:
		return result
	case <-r.done:
		return app.StatusSnapshot{}
	}
}
func (*controlledStatusReader) Close() {}
func statusModelFixture(t *testing.T) (Model, *controlledStatusReader) {
	t.Helper()
	cfg := config.Default()
	ctx := &ScreenContext{Cfg: cfg, State: &RuntimeState{WalletKnown: true, WalletExists: true}}
	r := &controlledStatusReader{calls: make(chan statusReadCall, 4), done: t.Context().Done()}
	return Model{cfg: cfg, state: ctx.State, screenCtx: ctx, statusCollector: r, nav: NewNavSidebar()}, r
}
func startStatusCommand(t *testing.T, cmd tea.Cmd, r *controlledStatusReader) (statusReadCall, <-chan tea.Msg) {
	t.Helper()
	if cmd == nil {
		t.Fatal("refresh was not scheduled")
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case call := <-r.calls:
		return call, result
	case <-time.After(time.Second):
		t.Fatal("collector not reached")
		return statusReadCall{}, nil
	}
}
func statusUpdate(m *Model, msg tea.Msg) tea.Cmd {
	updated, cmd := m.Update(msg)
	*m = updated.(Model)
	return cmd
}
func freshStatus[T any](v T) app.Observation[T] {
	return app.Observation[T]{Value: v, ObservedAt: time.Now()}
}

func TestStatusCoalescesAndRejectsReplayedCompletion(t *testing.T) {
	m, r := statusModelFixture(t)
	first, firstDone := startStatusCommand(t, statusUpdate(&m, refreshStatusMsg{}), r)
	for range 5 {
		if statusUpdate(&m, refreshStatusMsg{}) != nil {
			t.Fatal("overlapping refresh admitted a reader")
		}
	}
	first.result <- app.StatusSnapshot{Balance: freshStatus(lndrpc.WalletBalance{TotalBalance: "10000"})}
	old := <-firstDone
	next := statusUpdate(&m, old)
	second, secondDone := startStatusCommand(t, next, r)
	if statusUpdate(&m, old) != nil || m.screenCtx.Status.Balance.Value.TotalBalance != "10000" {
		t.Fatal("duplicate completion changed publication")
	}
	// A replay must not clear the second read's admission guard.
	if statusUpdate(&m, refreshStatusMsg{}) != nil {
		t.Fatal("old completion cleared the current guard")
	}
	second.result <- app.StatusSnapshot{Balance: freshStatus(lndrpc.WalletBalance{TotalBalance: "20000"})}
	thirdCmd := statusUpdate(&m, <-secondDone)
	third, thirdDone := startStatusCommand(t, thirdCmd, r)
	third.result <- app.StatusSnapshot{Balance: freshStatus(lndrpc.WalletBalance{TotalBalance: "30000"})}
	if statusUpdate(&m, <-thirdDone) != nil {
		t.Fatal("coalesced requests became an unbounded queue")
	}
	statusUpdate(&m, old)
	if m.screenCtx.Status.Balance.Value.TotalBalance != "30000" {
		t.Fatal("older completion replaced current balance")
	}
}

func TestStatusSchedulingSnapshotAndScopeChange(t *testing.T) {
	m, r := statusModelFixture(t)
	cmd := statusUpdate(&m, refreshStatusMsg{})
	m.cfg.SyncthingEnabled = true
	first, done := startStatusCommand(t, cmd, r)
	if first.cfg.SyncthingEnabled {
		t.Fatal("command read mutable configuration after scheduling")
	}
	first.result <- app.StatusSnapshot{Balance: freshStatus(lndrpc.WalletBalance{TotalBalance: "900"})}
	secondCmd := statusUpdate(&m, <-done)
	if m.screenCtx.Status != nil {
		t.Fatal("obsolete configuration result was published")
	}
	second, done := startStatusCommand(t, secondCmd, r)
	if !second.cfg.SyncthingEnabled {
		t.Fatal("follow-up did not use current configuration")
	}
	second.result <- app.StatusSnapshot{Balance: freshStatus(lndrpc.WalletBalance{TotalBalance: "1000"})}
	statusUpdate(&m, <-done)
	// A new wallet generation cannot inherit old-wallet last-good balances.
	m.screenCtx.walletRevision++
	third, done := startStatusCommand(t, statusUpdate(&m, refreshStatusMsg{}), r)
	if m.screenCtx.Status != nil {
		t.Fatal("old-wallet balance remained visible during replacement read")
	}
	third.result <- app.StatusSnapshot{Balance: app.Observation[lndrpc.WalletBalance]{Err: errors.New("unavailable")}}
	statusUpdate(&m, <-done)
	if m.screenCtx.Status.Balance.Known() {
		t.Fatal("retained balance from an obsolete wallet generation")
	}
}

func TestStatusFailureRenderingAndRecovery(t *testing.T) {
	m, r := statusModelFixture(t)
	oc := &OnChainContext{OnChainTxs: []lndrpc.OnChainTx{{Amount: 10000}, {Amount: 10000}}}
	onchain := NewOnChainHomeScreen(m.screenCtx, oc)
	channels := NewChannelsHomeScreen(m.screenCtx)
	offchain := NewWalletHomeScreen(m.screenCtx)
	offchain.entries = []lndrpc.PaymentEntry{{IsIncoming: true, Status: "SETTLED", AmountSats: 1000}}
	failed := errors.New("injected read failure")
	publish := func(s app.StatusSnapshot) {
		t.Helper()
		call, done := startStatusCommand(t, statusUpdate(&m, refreshStatusMsg{}), r)
		call.result <- s
		statusUpdate(&m, <-done)
	}
	publish(app.StatusSnapshot{Node: freshStatus(lndrpc.NodeInfo{}), Balance: app.Observation[lndrpc.WalletBalance]{Err: failed}, Channels: app.Observation[app.ChannelStatus]{Err: failed}})
	view := ansi.Strip(onchain.View(90, 42))
	if !strings.Contains(view, "unavailable") || strings.Contains(view, "Balance:  0 sats") || onchain.computeTxBalances() != nil || offchain.computeBalances() != nil {
		t.Fatalf("unavailable total became a fabricated balance:\n%s", view)
	}
	if !strings.Contains(view, "N/A") || !strings.Contains(ansi.Strip(offchain.View(90, 42)), "N/A") {
		t.Fatal("history rows fabricated running balances")
	}
	channelView := ansi.Strip(channels.View(90, 42))
	if !strings.Contains(channelView, "Channels unavailable") || strings.Contains(channelView, "No channels yet") {
		t.Fatalf("failed channel read became empty:\n%s", channelView)
	}
	publish(app.StatusSnapshot{Node: freshStatus(lndrpc.NodeInfo{}), Balance: freshStatus(lndrpc.WalletBalance{TotalBalance: "20000"}), Channels: freshStatus(app.ChannelStatus{Channels: []app.StatusChannel{{Channel: lndrpc.Channel{PeerAlias: "retained peer", LocalBalance: 5000}}}})})
	publish(app.StatusSnapshot{Node: freshStatus(lndrpc.NodeInfo{}), Balance: app.Observation[lndrpc.WalletBalance]{Err: failed}, Channels: app.Observation[app.ChannelStatus]{Err: failed}})
	view = ansi.Strip(onchain.View(90, 42))
	if !strings.Contains(view, "20,000 sats (stale)") || onchain.computeTxBalances() != nil {
		t.Fatalf("last-good balance was not explicitly stale:\n%s", view)
	}
	channelView = ansi.Strip(channels.View(90, 42))
	if !strings.Contains(channelView, "Channels stale") || !strings.Contains(channelView, "retained peer") {
		t.Fatalf("last-good channels disappeared or lost stale marker:\n%s", channelView)
	}
	publish(app.StatusSnapshot{Node: freshStatus(lndrpc.NodeInfo{}), Balance: freshStatus(lndrpc.WalletBalance{TotalBalance: "0"}), Channels: freshStatus(app.ChannelStatus{})})
	if !strings.Contains(ansi.Strip(onchain.View(90, 42)), "0 sats") || !strings.Contains(ansi.Strip(channels.View(90, 42)), "No channels yet") || !m.screenCtx.Status.Balance.Fresh() {
		t.Fatal("successful empty observation failed to recover")
	}
}
