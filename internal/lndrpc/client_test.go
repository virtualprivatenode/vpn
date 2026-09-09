// internal/lndrpc/client_test.go

package lndrpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc/metadata"
)

func TestWalletStateNames(t *testing.T) {
	for state, want := range map[lnrpc.WalletState]WalletState{
		lnrpc.WalletState_LOCKED:        WalletStateLocked,
		lnrpc.WalletState_SERVER_ACTIVE: WalletStateActive,
		lnrpc.WalletState_UNLOCKED:      "UNLOCKED",
	} {
		if got := walletStateName(state); got != want {
			t.Errorf("walletStateName(%s) = %s, want %s", state, got, want)
		}
	}
}

func TestSatStr(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"}, {1000000, "1000000"}, {-500, "-500"},
	}
	for _, tt := range tests {
		if got := satStr(tt.input); got != tt.want {
			t.Errorf("satStr(%d): got %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNilClientSafety(t *testing.T) {
	c := &Client{}
	if _, err := c.GetState(); err == nil {
		t.Error("should error")
	}
	if _, err := c.GetInfo(); err == nil {
		t.Error("should error")
	}
	if _, err := c.GetWalletBalance(); err == nil {
		t.Error("should error")
	}
	if _, err := c.ListChannels(); err == nil {
		t.Error("should error")
	}
	if _, err := c.GetPendingChannels(); err == nil {
		t.Error("should error")
	}
	if _, err := c.GetNewAddress(); err == nil {
		t.Error("should error")
	}
	if _, err := c.ListPeers(); err == nil {
		t.Error("should error")
	}
	if err := c.ConnectPeer("a", "b"); err == nil {
		t.Error("should error")
	}
	if result := c.OpenChannel(ChannelOpenRequest{}); result.Err == nil || result.Submitted {
		t.Error("should error")
	}
	if _, err := c.SendPayment("lnbc1"); err == nil {
		t.Error("should error")
	}
	if result := c.CloseChannel(ChannelCloseRequest{ChannelPoint: strings.Repeat("ab", 32) + ":0"}); result.Err == nil || result.Submitted {
		t.Error("should error")
	}
}

// macaroonCtx attaches the credential metadata to every call
// while Reconnect can rewrite the credential field on another
// goroutine. This test runs both sides at once so the race
// detector, not code review, arbitrates the lock discipline:
// readers hammer macaroonCtx while a writer rewrites the field
// under the same write lock the redial path holds. Run under
// go test -race; without the detector it still verifies that
// every context carries exactly one non-empty macaroon value.
func TestMacaroonCtxConcurrentRewrite(t *testing.T) {
	c := &Client{macaroonHex: "00"}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				md, ok := metadata.FromOutgoingContext(
					c.macaroonCtx())
				if !ok {
					t.Error("context carries no metadata")
					return
				}
				vals := md.Get("macaroon")
				if len(vals) != 1 || vals[0] == "" {
					t.Errorf("macaroon metadata: %q", vals)
					return
				}
			}
		}()
	}
	for i := 0; i < 500; i++ {
		c.mu.Lock()
		c.macaroonHex = fmt.Sprintf("%04x", i+1)
		c.mu.Unlock()
	}
	close(stop)
	wg.Wait()
}

// Both fund-moving coin-control call sites route through
// parseOutpoint; anything that does not parse must be an error,
// never a silently narrowed or retargeted selection.
func TestParseOutpoint(t *testing.T) {
	txid := strings.Repeat("ab", 32)
	op, err := parseOutpoint(txid + ":7")
	if err != nil || op.TxidStr != txid || op.OutputIndex != 7 {
		t.Fatalf("valid outpoint: got (%v, %v)", op, err)
	}
	if _, err := parseOutpoint(txid + ":0"); err != nil {
		t.Errorf("index 0 rejected: %v", err)
	}
	bad := []string{
		"",
		txid,
		txid + ":",
		txid + ":x",
		txid + ":1x2",
		txid + ":-1",
		txid + ":4294967296",
		"ab:0",
		"nothex" + strings.Repeat("a", 58) + ":0",
	}
	for _, s := range bad {
		if _, err := parseOutpoint(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

// The routing fee limit scales with the amount: half a percent
// with a 30 sat floor, and a fixed cap when the amount is
// unknown (zero-amount invoice).
func TestPaymentFeeLimit(t *testing.T) {
	tests := []struct{ amt, want int64 }{
		{0, 1000},
		{-5, 1000},
		{1, 30},
		{50, 30},
		{6000, 30},
		{6200, 31},
		{200000, 1000},
		{10000000, 50000},
	}
	for _, tt := range tests {
		if got := paymentFeeLimitSat(tt.amt); got != tt.want {
			t.Errorf("paymentFeeLimitSat(%d): got %d, want %d",
				tt.amt, got, tt.want)
		}
	}
}

// The RPC error classifier routes failures: transport loss
// reconnects, credential rejection reconnects under the
// re-stage limiter's damping (both land at the dial-time
// repair, which re-stages the LND credentials when they were
// the problem); in-progress states and application answers
// over a working connection do neither. The dial-time repair
// itself deliberately has NO classifier — it keys on the
// absence of a successful test call, because enumerating
// heal-worthy failure texts is a whitelist and a whitelist
// misses.
func TestClassifyRPCError(t *testing.T) {
	transport := []string{
		"rpc error: code = Unavailable desc = connection refused",
		// A TLS verification failure against a stale staged
		// certificate surfaces with code Unavailable — the
		// reconnect routes it to the dial-time repair.
		"rpc error: code = Unavailable desc = connection error: " +
			"desc = \"transport: authentication handshake failed: " +
			"tls: failed to verify certificate: x509: " +
			"certificate signed by unknown authority\"",
		// Observed in production when an old build dialed LND by
		// the name localhost on a box with IPv6 disabled. The
		// client now dials by literal address so this shape
		// should not recur, but if an address-family failure
		// ever does, reconnecting is the right response to it.
		"rpc error: code = Unavailable desc = connection error: " +
			"desc = \"transport: Error while dialing: dial tcp " +
			"[::1]:10009: connect: cannot assign requested " +
			"address\"",
	}
	for _, msg := range transport {
		if classifyRPCError(msg) != rpcErrTransport {
			t.Errorf("not classified transport: %q", msg)
		}
	}
	credential := []string{
		// A stale staged macaroon fails closed HERE. This class
		// was treated as terminal before, which made macaroon
		// staleness permanent for the process's lifetime.
		"rpc error: code = Unauthenticated desc = verification " +
			"failed: signature mismatch after caveat verification",
		"rpc error: code = PermissionDenied desc = permission " +
			"denied",
	}
	for _, msg := range credential {
		if classifyRPCError(msg) != rpcErrCredential {
			t.Errorf("not classified credential: %q", msg)
		}
	}
	other := []string{
		// In-progress states: reconnecting cannot help and
		// would churn the connection.
		"context deadline exceeded",
		"rpc error: code = DeadlineExceeded desc = deadline",
		"the RPC server is still starting up",
		"chain notifier RPC is still syncing, not yet ready",
		// Application answers over a working connection.
		"rpc error: code = InvalidArgument desc = invalid " +
			"payment request",
		"insufficient funds available to construct transaction",
	}
	for _, msg := range other {
		if classifyRPCError(msg) != rpcErrOther {
			t.Errorf("wrongly classified: %q", msg)
		}
	}
}

// The re-stage limiter is process-wide and bounds the heal to
// one request per interval — with the heal keyed on any failed
// test call, this is what keeps a long outage cheap.
func TestRestageDue(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	if !restageDue(time.Time{}, now) {
		t.Error("first request should be due")
	}
	if restageDue(now.Add(-credRestageInterval+time.Second), now) {
		t.Error("due inside the interval")
	}
	if !restageDue(now.Add(-credRestageInterval), now) {
		t.Error("not due at exactly the interval")
	}
	if !restageDue(now.Add(-time.Hour), now) {
		t.Error("not due long after the interval")
	}
}

func TestCancelledReconnectDoesNotReadCredentialsOrRestage(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := &Client{}
	if err := client.dial(ctx, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled dial reached filesystem or RPC work: %v", err)
	}
	credRestageMu.Lock()
	before := credRestageAt
	credRestageMu.Unlock()
	if requestCredentialRestage(ctx) {
		t.Fatal("cancelled reconnect requested credential mutation")
	}
	credRestageMu.Lock()
	after := credRestageAt
	credRestageMu.Unlock()
	if !after.Equal(before) {
		t.Fatal("cancelled reconnect consumed the restage allowance")
	}
}
