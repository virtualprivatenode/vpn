// Command factories connect screen messages to application and system operations.

package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/installer"
)

// ── Polling & version ────────────────────────────────────

func tickEveryCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchLatestVersionCmd() tea.Cmd {
	return func() tea.Msg {
		return latestVersionMsg(
			installer.CheckLatestVersion())
	}
}

// ── Syncthing actions ────────────────────────────────────

func fetchSyncthingDevicesCmd(owner *ScreenContext) tea.Cmd {
	owner.syncthingRevision++
	revision := owner.syncthingRevision
	runtime := owner.syncthing()
	return func() tea.Msg {
		devices, err := runtime.ListDevices()
		return syncthingDevicesMsg{owner: owner, revision: revision, devices: devices, err: err}
	}
}
func pairSyncthingDeviceCmd(owner *SyncthingPairScreen, deviceID string) tea.Cmd {
	runtime, attempt := owner.ctx.syncthing(), owner.attempt
	return func() tea.Msg {
		return syncthingPairedMsg{owner: owner, attempt: attempt, result: runtime.Pair(deviceID)}
	}
}
func removeSyncthingDeviceCmd(owner *SyncthingDeviceScreen) tea.Cmd {
	runtime, attempt, id := owner.ctx.syncthing(), owner.attempt, owner.device.DeviceID
	return func() tea.Msg { return syncthingRemovedMsg{owner: owner, attempt: attempt, result: runtime.Remove(id)} }
}
func closeSyncthingCmd(owner Screen, attempt uint64) tea.Cmd {
	return func() tea.Msg { return syncthingCloseMsg{owner: owner, attempt: attempt} }
}

// ── LND queries & fund-moving ────────────────────────────

type channelOpenAttempt struct {
	prepared app.PreparedChannelOpen
	alias    string
}

func openChannelCmd(client app.ChannelOpenClient, attempt *channelOpenAttempt) tea.Cmd {
	return func() tea.Msg {
		return channelOpenResultMsg{attempt: attempt, result: app.OpenChannel(client, attempt.prepared)}
	}
}

type channelCloseAttempt struct {
	prepared               app.PreparedChannelClose
	alias                  string
	capacity, localBalance int64
}

func closeChannelCmd(client app.ChannelCloseClient, attempt *channelCloseAttempt) tea.Cmd {
	return func() tea.Msg {
		return channelCloseResultMsg{attempt: attempt, result: app.CloseChannel(client, attempt.prepared)}
	}
}

func createInvoiceCmd(client app.LightningInvoiceClient, attempt *invoiceAttempt) tea.Cmd {
	return func() tea.Msg {
		invoice, err := app.CreateLightningInvoice(client, attempt.request)
		return invoiceCreatedMsg{attempt: attempt, invoice: invoice, err: err}
	}
}

func checkInvoiceCmd(client app.LightningInvoiceClient, attempt *invoiceAttempt, invoice app.LightningInvoice) tea.Cmd {
	return func() tea.Msg {
		state, err := app.CheckLightningInvoice(client, invoice)
		return invoiceStatusMsg{attempt: attempt, state: state, err: err}
	}
}

func scheduleInvoiceCheck(attempt *invoiceAttempt) tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return invoiceCheckMsg{attempt: attempt}
	})
}

func preparePaymentCmd(client app.LightningPaymentClient, attempt *paymentAttempt) tea.Cmd {
	return func() tea.Msg {
		payment, err := app.PrepareLightningPayment(client, attempt.request)
		return payReqDecodedMsg{attempt: attempt, payment: payment, err: err}
	}
}

func sendPaymentCmd(client app.LightningPaymentClient, attempt *paymentAttempt, payment app.PreparedPayment) tea.Cmd {
	return func() tea.Msg {
		result, err := app.SendLightningPayment(client, payment)
		return sendPaymentResultMsg{attempt: attempt, result: result, err: err}
	}
}

// ── On-chain queries & fund-moving ───────────────────────

func getNewAddressCmd(owner *OCReceiveScreen) tea.Cmd {
	client := owner.addresses
	if client == nil && owner.ctx.LndClient != nil {
		client = owner.ctx.LndClient
	}
	network, previous, attempt := owner.ctx.Cfg.Network, owner.address, owner.attempt
	return func() tea.Msg {
		address, err := app.CreateOnChainAddress(client, network, previous)
		return newAddressMsg{owner: owner, attempt: attempt, address: address, err: err}
	}
}

func sendCoinsCmd(client app.OnChainSendClient, attempt *onChainSendAttempt) tea.Cmd {
	prepared := attempt.prepared
	return func() tea.Msg {
		return sendCoinsResultMsg{attempt: attempt, result: app.SendOnChain(client, prepared)}
	}
}

// ── Transaction labeling ─────────────────────────────────

func labelTxCmd(owner *OnChainHomeScreen, client app.TransactionLabelClient, prepared app.PreparedTransactionLabel) tea.Cmd {
	attempt := owner.labelAttempt
	return func() tea.Msg {
		return labelTxMsg{owner: owner, attempt: attempt, result: app.SaveTransactionLabel(client, prepared)}
	}
}
