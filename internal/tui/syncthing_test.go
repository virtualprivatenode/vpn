package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/syncthing"
)

func syncModel(t *testing.T) (Model, *SyncthingDeviceScreen, *SyncthingDeviceScreen) {
	t.Helper()
	ctx := &ScreenContext{Cfg: config.Default(), State: &RuntimeState{}, Syncthing: app.NewSyncthing()}
	t.Cleanup(ctx.Syncthing.Close)
	a := NewSyncthingDeviceScreen(ctx, syncthing.Device{Name: "same", DeviceID: "A"})
	b := NewSyncthingDeviceScreen(ctx, syncthing.Device{Name: "same", DeviceID: "B"})
	m := Model{cfg: ctx.Cfg, state: ctx.State, screenCtx: ctx, nav: NewNavSidebar(), tabs: []openTab{
		{Kind: tabSyncthing, Section: secAddons, Screen: NewSyncthingDetailScreen(ctx)},
		{Kind: tabSyncthingDevice, Section: secAddons, Key: "A", Parent: tabSyncthing, Screen: a},
		{Kind: tabSyncthingDevice, Section: secAddons, Key: "B", Parent: tabSyncthing, Screen: b},
	}}
	m.nav.ActiveItem = secAddons
	return m, a, b
}
func TestSyncthingRemovalOwnsAttemptAndHiddenResult(t *testing.T) {
	m, a, b := syncModel(t)
	b.step = syncDeviceStepConfirm
	b.confirmIdx = 1
	_, first := b.HandleKey("enter", tea.KeyPressMsg{})
	_, second := b.HandleKey("enter", tea.KeyPressMsg{})
	if first == nil || second != nil {
		t.Fatal("repeated confirmation queued another removal")
	}
	m.nav.ActiveItem = secWallet
	result := syncthingRemovedMsg{owner: b, attempt: b.attempt, result: app.SyncthingResult{Outcome: app.SyncthingUnknown, Err: errors.New("removal unconfirmed")}}
	updated, _ := m.Update(result)
	m = updated.(Model)
	if b.removeError != "removal unconfirmed" || a.removeError != "" {
		t.Fatal("hidden result lost or misdelivered")
	}
	// A stale result cannot complete a later attempt.
	b.step = syncDeviceStepRemoving
	b.attempt++
	m.Update(result)
	if b.step != syncDeviceStepRemoving {
		t.Fatal("stale result changed new attempt")
	}
	result.attempt = b.attempt
	result.result = app.SyncthingResult{Outcome: app.SyncthingComplete}
	updated, _ = m.Update(result)
	m = updated.(Model)
	if b.step != syncDeviceStepRemoved {
		t.Fatal("hidden success lost")
	}
	if len(m.tabs) != 3 {
		t.Fatal("success silently removed result")
	}
	_, done := b.HandleKey("enter", tea.KeyPressMsg{})
	updated, _ = m.Update(done())
	m = updated.(Model)
	if len(m.tabs) != 2 || m.tabs[1].Screen != a || m.nav.ActiveSection() != secWallet {
		t.Fatal("Done closed another tab or changed section")
	}
}
func TestSyncthingDeviceTabsUseIDsAndProtectBusyParent(t *testing.T) {
	m, a, b := syncModel(t)
	other := NewSyncthingDeviceScreen(m.screenCtx, syncthing.Device{Name: "same", DeviceID: "C"})
	updated, _ := m.Update(openTabMsg{Kind: tabSyncthingDevice, Key: "C", Index: 0, Screen: other, Parent: tabSyncthing})
	m = updated.(Model)
	if len(m.tabs) != 4 {
		t.Fatal("row zero reused another device tab")
	}
	updated, _ = m.Update(openTabMsg{Kind: tabSyncthingDevice, Key: "B", Index: 9, Screen: b, Parent: tabSyncthing})
	m = updated.(Model)
	if len(m.tabs) != 4 {
		t.Fatal("reordered device duplicated tab")
	}
	a.step = syncDeviceStepRemoving
	for _, index := range []int{1, 2} {
		updated, _ = m.closeTab(index)
		if len(updated.(Model).tabs) != 4 {
			t.Fatal("busy device or parent closed")
		}
	}
}
func TestSyncthingPairPartialResultAndStaleDone(t *testing.T) {
	m, _, _ := syncModel(t)
	s := NewSyncthingPairScreen(m.screenCtx)
	s.step = syncPairStepPairing
	s.attempt = 1
	m.tabs = append(m.tabs, openTab{Kind: tabSyncthingPair, Section: secAddons, Parent: tabSyncthing, Screen: s})
	m.nav.ActiveItem = secWallet
	result := syncthingPairedMsg{owner: s, attempt: 1, result: app.SyncthingResult{Outcome: app.SyncthingPartial, Err: errors.New("device configured; share incomplete")}}
	updated, _ := m.Update(result)
	m = updated.(Model)
	if s.step != syncPairStepResult || s.pairError == "" {
		t.Fatal("partial pair returned to ordinary input")
	}
	oldDone := closeSyncthingCmd(s, s.attempt)()
	s.attempt++
	s.step = syncPairStepPairing
	m.Update(result)
	if s.step != syncPairStepPairing {
		t.Fatal("old result completed newer pair")
	}
	s.step = syncPairStepResult
	updated, _ = m.Update(oldDone)
	if len(updated.(Model).tabs) != 4 {
		t.Fatal("old Done closed newer completed attempt")
	}
}
func TestSyncthingRefreshKeepsIdentityAndRejectsOlderRead(t *testing.T) {
	m, _, _ := syncModel(t)
	detail := m.tabs[0].Screen.(*SyncthingDetailScreen)
	m.state.SyncthingDevicesKnown = true
	m.state.SyncthingDevices = []syncthing.Device{{DeviceID: "A"}, {DeviceID: "B"}}
	detail.focusZone = syncDetailZoneList
	detail.cursor = 1
	m.screenCtx.syncthingRevision = 2
	fresh := syncthingDevicesMsg{owner: m.screenCtx, revision: 2, devices: []syncthing.Device{{DeviceID: "B"}, {DeviceID: "A"}}}
	m.Update(fresh)
	if detail.cursor != 0 {
		t.Fatal("selection changed identity on reorder")
	}
	m.Update(syncthingDevicesMsg{owner: m.screenCtx, revision: 1, err: errors.New("late read")})
	if !m.state.SyncthingDevicesKnown || m.state.SyncthingDevices[0].DeviceID != "B" {
		t.Fatal("older read overwrote current list")
	}
	m.screenCtx.syncthingRevision++
	fresh.revision++
	fresh.devices = []syncthing.Device{{DeviceID: "A"}}
	m.Update(fresh)
	if detail.focusZone != syncDetailZoneButtons {
		t.Fatal("missing selection silently substituted another device")
	}
}
