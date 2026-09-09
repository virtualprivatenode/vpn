// Command factories connect screen messages to application and system operations.

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/installer"
	"github.com/virtualprivatenode/vpn/internal/lndrpc"
	"github.com/virtualprivatenode/vpn/internal/logger"
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

func fetchWalletStateCmd() tea.Cmd {
	return func() tea.Msg {
		var state helper.WalletStateResult
		err := helper.Call(helper.VerbReadWalletState, nil, &state)
		return walletStateMsg{state: state, err: err}
	}
}

func fetchKeyVerificationStateCmd() tea.Cmd {
	return func() tea.Msg {
		var state helper.KeyVerificationStateResult
		err := helper.Call(
			helper.VerbReadKeyVerificationState, nil, &state)
		return keyVerificationStateMsg{state: state, err: err}
	}
}

// fetchNodeAddressesCmd asks the helper's read-node-addresses
// operation for the node's onion hostnames and Syncthing
// device ID, live. Screens that display these run this at
// entry (Init, and again on tabActivatedMsg) and render the
// answer from their own state — never from a stored copy that
// could outlive the truth. tab routes the answer back to the
// requesting screen.
func fetchNodeAddressesCmd(tab tabKind) tea.Cmd {
	return func() tea.Msg {
		var res helper.NodeAddressesResult
		err := helper.Call(
			helper.VerbReadNodeAddresses, nil, &res)
		if err != nil {
			logger.Status("read node addresses: %v", err)
		}
		return nodeAddressesMsg{tab: tab, addrs: res, err: err}
	}
}

// ── Syncthing actions ────────────────────────────────────

func fetchSyncthingDevicesCmd() tea.Cmd {
	return func() tea.Msg {
		devices, err := installer.ListSyncthingDevices()
		if err != nil {
			logger.Status("read Syncthing devices: %v", err)
		}
		return syncthingDevicesMsg{devices: devices, err: err}
	}
}

func pairSyncthingDeviceCmd(
	deviceID string,
) tea.Cmd {
	return func() tea.Msg {
		err := installer.PairSyncthingDevice(deviceID)
		return syncthingPairedMsg{
			deviceID: deviceID, err: err}
	}
}

func removeSyncthingDeviceCmd(
	deviceID string,
) tea.Cmd {
	return func() tea.Msg {
		err := installer.UnpairSyncthingDevice(deviceID)
		return syncthingRemovedMsg{
			deviceID: deviceID, err: err}
	}
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

type channelCloseFeesMsg struct {
	screen *ChannelCloseScreen
	tiers  [4]feeTier
	err    error
}

func closeChannelCmd(client app.ChannelCloseClient, attempt *channelCloseAttempt) tea.Cmd {
	return func() tea.Msg {
		return channelCloseResultMsg{attempt: attempt, result: app.CloseChannel(client, attempt.prepared)}
	}
}

func closeFeeTiersCmd(screen *ChannelCloseScreen) tea.Cmd {
	fetch := fetchFeeTiersCmd(screen.ctx.Cfg)
	return func() tea.Msg {
		msg := fetch().(feeTiersMsg)
		return channelCloseFeesMsg{screen: screen, tiers: msg.tiers, err: msg.err}
	}
}

func fetchClosedChannelsCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return closedChannelsMsg{
				err: fmt.Errorf("LND not connected")}
		}
		channels, err := client.ListClosedChannels()
		return closedChannelsMsg{
			channels: channels, err: err}
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

// fetchPaymentHistoryCmd merges ListInvoices and
// ListPayments into one paymentHistoryMsg. Each RPC's err
// is tracked independently and rolled up into rpcErr so
// the handler's `if msg.err == nil` partial-data guard
// doesn't overwrite last-good entries on a flaky fetch.
func fetchPaymentHistoryCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return paymentHistoryMsg{
				err: fmt.Errorf("LND not connected")}
		}
		invoices, invErr := client.ListInvoices(50)
		if invErr != nil {
			logger.TUI("ListInvoices: %v", invErr)
		}
		payments, payErr := client.ListPayments(50)
		if payErr != nil {
			logger.TUI("ListPayments: %v", payErr)
		}
		var all []lndrpc.PaymentEntry
		all = append(all, invoices...)
		all = append(all, payments...)
		sort.Slice(all, func(i, j int) bool {
			return all[i].CreationDate >
				all[j].CreationDate
		})
		var rpcErr error
		switch {
		case invErr != nil && payErr != nil:
			rpcErr = fmt.Errorf(
				"invoices and payments: %v", invErr)
		case invErr != nil:
			rpcErr = fmt.Errorf("invoices: %v", invErr)
		case payErr != nil:
			rpcErr = fmt.Errorf("payments: %v", payErr)
		}
		return paymentHistoryMsg{
			entries: all, err: rpcErr}
	}
}

// ── On-chain queries & fund-moving ───────────────────────

func getNewAddressCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return newAddressMsg{
				err: fmt.Errorf("LND not connected")}
		}
		addr, err := client.GetNewAddress()
		if err != nil {
			return newAddressMsg{err: err}
		}
		return newAddressMsg{address: addr.Address}
	}
}

func listUnspentCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return utxoListMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		utxos, err := client.ListUnspent(0, 999999)
		return utxoListMsg{utxos: utxos, err: err}
	}
}

func sendCoinsCmd(client app.OnChainSendClient, attempt *onChainSendAttempt) tea.Cmd {
	prepared := attempt.prepared
	return func() tea.Msg {
		return sendCoinsResultMsg{attempt: attempt, result: app.SendOnChain(client, prepared)}
	}
}

func fetchOnChainTxCmd(
	client *lndrpc.Client,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return onChainTxMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		txs, err := client.GetTransactions()
		return onChainTxMsg{txs: txs, err: err}
	}
}

// ── Fee estimation ───────────────────────────────────────

func fetchFeeTiersCmd(
	cfg *config.AppConfig,
) tea.Cmd {
	snapshot := *cfg
	return func() tea.Msg {
		return fetchFeeTiers(&snapshot)
	}
}

// ── Transaction labeling ─────────────────────────────────

func labelTxCmd(
	client *lndrpc.Client, txid, label string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return labelTxMsg{
				err: fmt.Errorf("LND not connected")}
		}
		err := client.LabelTransaction(
			txid, label, true)
		return labelTxMsg{err: err}
	}
}

// ── Shell-out overlays ───────────────────────────────────
// Hand the terminal to a subprocess for display. The TUI
// pauses; the subprocess prints to the user's terminal; the
// user presses Enter; the TUI resumes. Used where the user
// wants to select/copy text with their terminal's native
// mechanism rather than via the TUI's monoWrap/QR overlays.

func showMacaroonCmd() tea.Cmd {
	mac := readMacaroonHex()
	if mac == "" {
		return nil
	}
	tmpFile, err := os.CreateTemp("", "vpn-macaroon-")
	if err != nil {
		return nil
	}
	tmpPath := tmpFile.Name()
	_, _ = tmpFile.WriteString(mac)
	_ = tmpFile.Close()
	// Macaroon hex is a credential — wipe scrollback
	// on exit so it doesn't sit in the user's terminal
	// history after they return to the TUI.
	c := exec.Command("bash", "-c",
		"clear && echo && cat "+tmpPath+
			" && echo && echo && echo "+
			"'  Press Enter...' && read && rm -f "+
			tmpPath+
			` && printf '\033[2J\033[3J\033[H'`)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		_ = os.Remove(tmpPath)
		return svcActionDoneMsg{}
	})
}

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
		return svcActionDoneMsg{}
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
		return svcActionDoneMsg{}
	})
}

// ── System actions ───────────────────────────────────────

// runSvcActionCmd requests a service start/stop/restart from
// the root helper. The helper validates the unit and action
// against closed sets and verifies the unit's state afterward;
// this side just reports the outcome.
func runSvcActionCmd(action, svc string) tea.Cmd {
	var verb string
	switch action {
	case "Restart":
		verb = "restart"
	case "Stop":
		verb = "stop"
	case "Start":
		verb = "start"
	default:
		return nil
	}
	return func() tea.Msg {
		if err := helper.Call(helper.VerbServiceAction,
			helper.ServiceActionParams{
				Unit: svc, Action: verb,
			}, nil); err != nil {
			logger.Install("%s %s: %v", verb, svc, err)
		}
		return svcActionDoneMsg{}
	}
}

// runUpdatePackagesCmd requests the helper's package-update
// operation (apt refresh + upgrade, non-interactive, with a
// dpkg consistency check after). The helper streams step
// progress; this button's UX is a single busy state, so the
// call simply blocks until the terminator.
func runUpdatePackagesCmd() tea.Cmd {
	return func() tea.Msg {
		logger.Install("Update packages started")
		err := helper.Call(helper.VerbPackageUpdate, nil, nil)
		if err != nil {
			logger.Install("Update packages failed: %v", err)
		} else {
			logger.Install("Update packages completed")
		}
		return pkgUpdateDoneMsg{}
	}
}

func runRebootCmd() tea.Cmd {
	return func() tea.Msg {
		// The helper answers, then reboots — the connection
		// outliving the box is not expected, so any error here
		// is only logged.
		if err := helper.Call(helper.VerbReboot, nil, nil); err != nil {
			logger.Install("reboot: %v", err)
		}
		return svcActionDoneMsg{}
	}
}
