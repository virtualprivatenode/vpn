// internal/bitcoin/client.go

// Package bitcoin talks to the local bitcoind over its JSON-RPC
// interface, authenticating with the node's own staged RPC
// credential (see internal/installer/rpcauth.go). Plain HTTP to
// 127.0.0.1 with basic auth — bitcoind's RPC is loopback-only
// on this box — and no privileged operation anywhere: the
// password is read from the staging board, which the admin
// user can read by group.
package bitcoin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/paths"
)

// RPCUser must match the rpcauth line the installer writes
// into bitcoin.conf (installer.BitcoindRPCUser; duplicated
// here as a plain constant because a bitcoin→installer import
// would be backwards). A unit test in the installer package
// asserts the two constants agree.
const RPCUser = "vpn"

type BlockchainInfo struct {
	Blocks     int
	Headers    int
	Progress   float64
	Synced     bool
	Responding bool
	// SizeOnDisk is bitcoind's own measure of its data
	// footprint in bytes — which is why no privileged
	// du of the data dir is needed for the status screen.
	SizeOnDisk int64
}

type blockchainInfoResponse struct {
	Blocks               int     `json:"blocks"`
	Headers              int     `json:"headers"`
	VerificationProgress float64 `json:"verificationprogress"`
	InitialBlockDownload bool    `json:"initialblockdownload"`
	SizeOnDisk           int64   `json:"size_on_disk"`
}

// rpcCall performs one JSON-RPC call against loopback. The
// credential never touches argv or the environment: it is read
// from the board and sent as HTTP basic auth.
func rpcCall(rpcPort int, method string,
	params []any, result any) error {
	pass, err := helper.ReadBoardString(paths.StateBitcoindRPCPass)
	if err != nil {
		return err
	}
	if params == nil {
		params = []any{}
	}
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "1.0",
		"id":      "vpn",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/", rpcPort),
		bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.SetBasicAuth(RPCUser, pass)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("bitcoind unreachable: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		// The staged password and the conf's rpcauth line are
		// replaced together at install; disagreement means one
		// of them was changed outside this app.
		return fmt.Errorf(
			"bitcoind rejected the staged RPC credential — " +
				"bitcoin.conf and the staged password disagree")
	case http.StatusForbidden:
		return fmt.Errorf(
			"bitcoind denied method %s for user %s", method, RPCUser)
	}
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("%s: rpc error %d: %s",
			method, rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if result != nil {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return nil
}

// GetBlockchainInfo probes chain status. Never errors — the
// status screen renders Responding=false as "not responding",
// which covers bitcoind being down, starting up, or the staged
// credential being unavailable (each already logged closer to
// its source).
func GetBlockchainInfo(rpcPort int) *BlockchainInfo {
	var resp blockchainInfoResponse
	if err := rpcCall(rpcPort,
		"getblockchaininfo", nil, &resp); err != nil {
		return &BlockchainInfo{Responding: false}
	}
	return &BlockchainInfo{
		Blocks:     resp.Blocks,
		Headers:    resp.Headers,
		Progress:   resp.VerificationProgress,
		Synced:     !resp.InitialBlockDownload,
		Responding: true,
		SizeOnDisk: resp.SizeOnDisk,
	}
}

// EstimateSmartFee returns the fee estimate for the target in
// sat/vB (minimum 1), or an error when bitcoind has no
// estimate yet.
func EstimateSmartFee(rpcPort, target int) (float64, error) {
	var resp struct {
		FeeRate float64  `json:"feerate"`
		Errors  []string `json:"errors"`
	}
	if err := rpcCall(rpcPort, "estimatesmartfee",
		[]any{target}, &resp); err != nil {
		return 0, err
	}
	if resp.FeeRate <= 0 || len(resp.Errors) > 0 {
		return 0, fmt.Errorf("no estimate for target %d (%s)",
			target, strings.Join(resp.Errors, "; "))
	}
	// bitcoind returns BTC/kvB; sat/vB = BTC/kvB × 1e8 / 1000.
	satPerVB := resp.FeeRate * 100000
	if satPerVB < 1 {
		satPerVB = 1
	}
	return satPerVB, nil
}

func FormatProgress(progress float64) string {
	return fmt.Sprintf("%.2f%%", progress*100)
}

// FormatSize renders a byte count the way the status screen
// shows data-directory sizes ("12.3 GB").
func FormatSize(bytes int64) string {
	const gb = 1 << 30
	const mb = 1 << 20
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/gb)
	case bytes >= mb:
		return fmt.Sprintf("%.0f MB", float64(bytes)/mb)
	case bytes > 0:
		return fmt.Sprintf("%d B", bytes)
	default:
		return "N/A"
	}
}
