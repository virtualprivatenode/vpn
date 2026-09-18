// Command factories connect screen messages to application and system operations.

package tui

import (
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/helper"
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

// ── Live-read node facts ─────────────────────────────────

func fetchWalletStateCmd(owner *ScreenContext) tea.Cmd {
	owner.walletRevision++
	revision := owner.walletRevision
	return func() tea.Msg {
		var state helper.WalletStateResult
		err := helper.Call(helper.VerbReadWalletState, nil, &state)
		return walletStateMsg{owner: owner, revision: revision, state: state, err: err}
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

// ── Shell-out overlays ───────────────────────────────────
// Hand the terminal to a subprocess for display. The TUI
// pauses; the subprocess prints to the user's terminal; the
// user presses Enter; the TUI resumes. Used where the user
// wants to select/copy text with their terminal's native
// mechanism rather than via the TUI's monoWrap/QR overlays.

func showInvoiceCmd(invoice string) tea.Cmd {
	if invoice == "" {
		return nil
	}
	tmpFile, err := os.CreateTemp("", "vpn-invoice-")
	if err != nil {
		return nil
	}
	tmpPath := tmpFile.Name()
	_, _ = tmpFile.WriteString(invoice)
	_ = tmpFile.Close()
	// Plain clear at end — invoice isn't sensitive
	// (the user generated it and likely copied it),
	// so preserving scrollback is fine.
	c := exec.Command("bash", "-c",
		"clear && echo && cat "+tmpPath+
			" && echo && echo && echo "+
			"'  Press Enter...' && read && rm -f "+
			tmpPath+
			" && clear")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		_ = os.Remove(tmpPath)
		return systemRefreshMsg{}
	})
}

// showNodeURIsCmd hands the terminal to a shell that
// displays the node's advertised URIs (clearnet first,
// then Tor) so the user can select and copy them with
// their terminal's native copy mechanism. Same pattern
// as showInvoiceCmd — non-sensitive data, no scrollback
// wipe. Preserving scrollback is a feature here: a user
// who returns to the TUI and later wants the URI again
// can pull it from their SSH scrollback without
// reopening the screen.
func showNodeURIsCmd(uris []string) tea.Cmd {
	if len(uris) == 0 {
		return nil
	}
	// Format with section labels. Clearnet first to
	// match the Node Info screen's button order and
	// LND's typical advertisement order.
	var b strings.Builder
	b.WriteString("\n  Node URIs\n")
	b.WriteString("  =========\n\n")
	var clearnet, tor []string
	for _, u := range uris {
		if strings.Contains(u, ".onion:") {
			tor = append(tor, u)
		} else {
			clearnet = append(clearnet, u)
		}
	}
	if len(clearnet) > 0 {
		b.WriteString("  Clearnet:\n")
		for _, u := range clearnet {
			b.WriteString("  ")
			b.WriteString(u)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(tor) > 0 {
		b.WriteString("  Tor:\n")
		for _, u := range tor {
			b.WriteString("  ")
			b.WriteString(u)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	tmpFile, err := os.CreateTemp("", "vpn-nodeuris-")
	if err != nil {
		return nil
	}
	tmpPath := tmpFile.Name()
	_, _ = tmpFile.WriteString(b.String())
	_ = tmpFile.Close()
	c := exec.Command("bash", "-c",
		"clear && cat "+tmpPath+
			" && echo && echo "+
			"'  Press Enter...' && read && rm -f "+
			tmpPath+
			" && clear")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		_ = os.Remove(tmpPath)
		return systemRefreshMsg{}
	})
}
