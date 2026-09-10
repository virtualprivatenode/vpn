package tui

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/lndrpc"
)

func (m Model) routeWalletCreation(owner *WalletCreateScreen, msg tea.Msg) (Model, tea.Cmd) {
	if owner == nil {
		return m, nil
	}
	for _, tab := range m.tabs {
		if tab.Screen == owner {
			_, cmd := owner.HandleMsg(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) finishWalletCreation(msg walletFinalizedMsg) (Model, tea.Cmd) {
	s := msg.owner
	if s == nil || !s.acceptsFinalization(msg) {
		return m, nil
	}
	found := false
	for _, tab := range m.tabs {
		if tab.Screen == s {
			found = true
			break
		}
	}
	if !found {
		return m, nil
	}
	s.HandleMsg(msg)
	m.screenCtx.walletRevision++
	m.state.WalletKnown = msg.result.Presence != app.WalletUnknown
	if m.state.WalletKnown {
		m.state.WalletExists = msg.result.Presence == app.WalletPresent
	}
	s.clientErr = nil
	if msg.result.CredentialsStaged && m.lndClient == nil {
		open := m.screenCtx.openWalletClient
		if open == nil {
			open = lndrpc.NewStagedClient
		}
		client, err := open()
		if err != nil {
			s.clientErr = fmt.Errorf("wallet credentials were staged, but the TUI client could not be initialized: %w", err)
			s.result.Err = errors.Join(s.result.Err, s.clientErr)
		} else {
			m.lndClient, m.screenCtx.LndClient = client, client
		}
	}
	if s.result.Err == nil && s.canContinue() {
		return m.continueWalletCreation(s)
	}
	return m, nil
}

func (m Model) continueWalletCreation(owner *WalletCreateScreen) (Model, tea.Cmd) {
	for i, tab := range m.tabs {
		if tab.Screen != owner {
			continue
		}
		m.releaseWalletCreation(owner)
		screen := NewAutoUnlockScreen(m.screenCtx)
		m.tabs[i].Kind, m.tabs[i].Label, m.tabs[i].Screen = tabAutoUnlock, "Auto-Unlock", screen
		return m, tea.Batch(fetchStatus(m.cfg, m.state, m.lndClient), screen.Init())
	}
	return m, nil
}

func (m *Model) releaseWalletCreation(screen Screen) {
	if screen == m.screenCtx.walletCreationOwner {
		m.screenCtx.walletCreationOwner = nil
		m.screenCtx.walletRevision++
	}
}
