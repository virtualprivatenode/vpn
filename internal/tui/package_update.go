package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/logger"
)

type packageAttempt struct {
	results <-chan app.PackageUpdateResult
}

type packageUpdateRequestMsg struct {
	owner   *SystemHomeScreen
	attempt *packageAttempt
}

type packageUpdateResultMsg struct {
	owner   *SystemHomeScreen
	attempt *packageAttempt
	result  app.PackageUpdateResult
}

func packageUpdateCmd(owner *SystemHomeScreen, attempt *packageAttempt) tea.Cmd {
	return func() tea.Msg { return packageUpdateRequestMsg{owner: owner, attempt: attempt} }
}

func (m *Model) startPackageUpdate(msg packageUpdateRequestMsg) tea.Cmd {
	if msg.owner == nil || m.sectionScreens[secSystem] != msg.owner ||
		msg.attempt == nil || msg.owner.pkgPending != msg.attempt || msg.attempt.results != nil {
		return nil
	}
	// Invalidate on the event loop before the helper can accept the update.
	m.invalidatePackageStatus()
	results := m.screenCtx.packageUpdates().Update()
	msg.attempt.results = results
	return func() tea.Msg {
		return packageUpdateResultMsg{owner: msg.owner, attempt: msg.attempt, result: <-results}
	}
}

func (m *Model) completePackageUpdate(msg packageUpdateResultMsg) tea.Cmd {
	if msg.owner == nil || m.sectionScreens[secSystem] != msg.owner ||
		msg.attempt == nil || msg.owner.pkgPending != msg.attempt || msg.attempt.results == nil {
		return nil
	}
	msg.owner.pkgPending = nil
	switch msg.result.Outcome {
	case app.PackageUpdateCompleted:
		msg.owner.pkgResult = "Package update completed."
	case app.PackageUpdateNotStarted:
		msg.owner.pkgResult = "Package update was not started. Reopen the TUI to try again."
	default:
		msg.owner.pkgResult = "Package update outcome unconfirmed. The host may still be working. " +
			"Ask your administrator to check package state and logs before retrying."
	}
	if msg.result.Err != nil {
		logger.TUI("Package update: %v", msg.result.Err)
	}
	m.invalidatePackageStatus()
	return m.admitStatus()
}

func (m *Model) invalidatePackageStatus() {
	m.statusRevision++
	if status := m.screenCtx.Status; status != nil {
		// Package configuration can restart services and change resource usage.
		// Preserve last-good values and wallet identity while new reads recover.
		err := errors.New("package update may have changed host state")
		for name, observation := range status.Services {
			observation.Err = err
			status.Services[name] = observation
		}
		status.Reboot.Err, status.Disk.Err, status.Memory.Err = err, err, err
		status.Bitcoin.Err = err
		*status = status.WalletUnavailable(err)
	}
}
