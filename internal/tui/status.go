package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/lndrpc"
)

type statusReader interface {
	Collect(config.AppConfig, bool, app.StatusLND) app.StatusSnapshot
	Close()
}

type statusScope struct {
	config                    config.AppConfig
	walletKnown, walletExists bool
	walletRevision            uint64
	client                    *lndrpc.Client
}

type statusRequest struct{ scope statusScope }
type statusResultMsg struct {
	request  *statusRequest
	snapshot app.StatusSnapshot
}

func requestStatusCmd() tea.Msg { return refreshStatusMsg{} }

func (m Model) currentStatusScope() statusScope {
	return statusScope{config: *m.cfg, walletKnown: m.state.WalletKnown,
		walletExists: m.state.WalletExists, walletRevision: m.screenCtx.walletRevision,
		client: m.lndClient}
}

// Every trigger reaches this gate on the event loop. Overlap requests one
// follow-up read, never a concurrent collector or a queue of redundant reads.
func (m *Model) admitStatus() tea.Cmd {
	current := m.currentStatusScope()
	if m.statusScope != current {
		m.screenCtx.Status = nil
	}
	if m.statusActive != nil {
		m.statusPending = true
		return nil
	}
	request := &statusRequest{scope: current}
	m.statusActive = request
	collector := m.statusCollector
	return func() tea.Msg {
		var client app.StatusLND
		if request.scope.client != nil {
			client = request.scope.client
		}
		return statusResultMsg{request: request, snapshot: collector.Collect(
			request.scope.config, request.scope.walletKnown && request.scope.walletExists, client)}
	}
}

func (m *Model) completeStatus(msg statusResultMsg) tea.Cmd {
	if msg.request == nil || msg.request != m.statusActive {
		return nil
	}
	m.statusActive = nil
	current := m.currentStatusScope()
	if msg.request.scope == current {
		if m.screenCtx.Status != nil && m.statusScope == current {
			msg.snapshot = msg.snapshot.Retain(*m.screenCtx.Status)
		}
		m.screenCtx.Status = &msg.snapshot
		m.statusScope = current
	} else {
		m.statusPending = true
	}
	if m.statusPending {
		m.statusPending = false
		return m.admitStatus()
	}
	return nil
}
