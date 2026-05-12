// Package welcome — cmds.go
//
// tea.Cmd factories. Each function here returns a tea.Cmd
// (or a tea.Cmd-producing closure) that, when run by the
// Bubble Tea runtime, performs side effects and returns a
// message. These are intentionally separate from Model:
// they have no receiver, depend only on their typed
// arguments, and are thin wrappers over internal/lndrpc,
// internal/installer, and system shell-outs.
//
// Organization (by comment banner below):
//   - Polling & version
//   - Syncthing actions
//   - LND queries & fund-moving
//   - On-chain queries & fund-moving
//   - Fee estimation
//   - Transaction labeling
//   - Shell-out overlays
//   - System actions
//
// Behaviour note: fetchPaymentHistoryCmd uses separate
// err variables per RPC and rolls them up into a single
// message-level err. See design-decisions.md
// ("Multi-RPC fetch cmds must aggregate their errors").

package welcome

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/system"
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

func openChannelCmd(
	client *lndrpc.Client, pubkey, host string,
	amount int64, private bool, taproot bool,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return channelOpenResultMsg{
				err: fmt.Errorf("LND not connected")}
		}
		if host != "" {
			if err := client.ConnectPeer(
				pubkey, host); err != nil {
				logger.TUI(
					"Peer connect warning: %v", err)
			}
		}
		if err := client.WaitForPeer(
			pubkey, 60*time.Second); err != nil {
			return channelOpenResultMsg{
				err: fmt.Errorf(
					"could not connect: %v", err)}
		}
		result, err := client.OpenChannel(
			pubkey, amount, private, taproot)
		if err != nil {
			return channelOpenResultMsg{err: err}
		}
		return channelOpenResultMsg{
			txid: result.FundingTxID}
	}
}

func closeChannelCmd(
	client *lndrpc.Client,
	chanPoint string,
	force bool,
	satPerVbyte uint64,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return closeChannelMsg{
				err: fmt.Errorf("LND not connected")}
		}
		result, err := client.CloseChannel(
			chanPoint, force, satPerVbyte)
		if err != nil {
			return closeChannelMsg{err: err}
		}
		return closeChannelMsg{
			txid: result.ClosingTxid}
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

func createInvoiceCmd(
	client *lndrpc.Client, amount int64, memo string,
	blind bool,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return invoiceCreatedMsg{
				err: fmt.Errorf("LND not connected")}
		}
		inv, err := client.AddInvoice(amount, memo, blind)
		if err != nil {
			return invoiceCreatedMsg{err: err}
		}
		return invoiceCreatedMsg{
			payReq:      inv.PaymentRequest,
			paymentHash: inv.PaymentHash,
			amountSats:  inv.AmountSats,
		}
	}
}

func waitForInvoiceCmd(
	client *lndrpc.Client, paymentHash string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return invoiceSettledMsg{
				err: fmt.Errorf("LND not connected")}
		}
		hashBytes, err := hex.DecodeString(paymentHash)
		if err != nil {
			return invoiceSettledMsg{err: err}
		}
		inv, err := client.WaitForInvoiceSettlement(
			hashBytes, 3600*time.Second)
		if err != nil {
			return invoiceSettledMsg{err: err}
		}
		return invoiceSettledMsg{
			settled: inv.Settled,
			expired: inv.IsExpired,
		}
	}
}

func decodePayReqCmd(
	client *lndrpc.Client, payReq string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return payReqDecodedMsg{
				err: fmt.Errorf("LND not connected")}
		}
		decoded, err := client.DecodePayReq(payReq)
		if err != nil {
			return payReqDecodedMsg{err: err}
		}
		return payReqDecodedMsg{decoded: decoded}
	}
}

func sendPaymentCmd(
	client *lndrpc.Client, payReq string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return sendPaymentResultMsg{
				err: fmt.Errorf("LND not connected")}
		}
		result, err := client.SendPayment(payReq)
		if err != nil {
			return sendPaymentResultMsg{err: err}
		}
		return sendPaymentResultMsg{result: result}
	}
}

// fetchPaymentHistoryCmd merges ListInvoices and
// ListPayments into one paymentHistoryMsg. Each RPC's err
// is tracked independently and rolled up into rpcErr so
// the handler's `if msg.err == nil` partial-data guard
// doesn't overwrite last-good entries on a flaky fetch.
// See design-decisions.md ("Multi-RPC fetch cmds must
// aggregate their errors").
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

func sendCoinsCmd(
	client *lndrpc.Client, addr string,
	amount int64, feeRate int64, sendAll bool,
	outpoints []string,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return sendCoinsResultMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		result, err := client.SendCoins(
			addr, amount, feeRate, sendAll, outpoints)
		if err != nil {
			return sendCoinsResultMsg{err: err}
		}
		return sendCoinsResultMsg{txid: result.Txid}
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
	return func() tea.Msg {
		return fetchFeeTiers(cfg)
	}
}

func estimateTxFeeCmd(
	client *lndrpc.Client, addr string,
	amount int64, targetConf int32,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return feeEstimateMsg{err: fmt.Errorf(
				"LND not connected")}
		}
		est, err := client.EstimateFee(
			addr, amount, targetConf)
		if err != nil {
			return feeEstimateMsg{err: err}
		}
		return feeEstimateMsg{feeSats: est.FeeSats}
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

func showMacaroonCmd(cfg *config.AppConfig) tea.Cmd {
	mac := readMacaroonHex(cfg)
	if mac == "" {
		return nil
	}
	tmpFile, err := os.CreateTemp("", "rlvpn-macaroon-")
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
	tmpFile, err := os.CreateTemp("", "rlvpn-invoice-")
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
	tmpFile, err := os.CreateTemp("", "rlvpn-nodeuris-")
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
		system.SudoRun("systemctl", verb, svc)
		return svcActionDoneMsg{}
	}
}

func runUpdatePackagesCmd() tea.Cmd {
	// Non-interactive environment so beginner users
	// never see mid-upgrade prompts:
	//   - DEBIAN_FRONTEND=noninteractive suppresses
	//     debconf dialogs entirely
	//   - NEEDRESTART_MODE=a tells needrestart (default
	//     on Debian 13) to auto-restart services instead
	//     of showing its blue ncurses picker
	//   - --force-confdef + --force-confold keeps the
	//     existing config file on any conffile conflict,
	//     skipping the pink dpkg prompt
	// This matches the bootstrap script's Phase 1
	// upgrade command exactly.
	return func() tea.Msg {
		logger.Install("Update packages started")
		err := system.SudoRun("bash", "-c",
			"DEBIAN_FRONTEND=noninteractive "+
				"NEEDRESTART_MODE=a "+
				"apt-get update -qq && "+
				"DEBIAN_FRONTEND=noninteractive "+
				"NEEDRESTART_MODE=a "+
				"apt-get upgrade -y -qq "+
				"-o Dpkg::Options::=--force-confdef "+
				"-o Dpkg::Options::=--force-confold")
		if err != nil {
			logger.Install("Update packages failed: %v", err)
		} else {
			logger.Install("Update packages completed")
		}
		return pkgUpdateDoneMsg{}
	}
}

func runRebootCmd() tea.Cmd {
	c := exec.Command("sudo", "reboot")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return svcActionDoneMsg{}
	})
}
