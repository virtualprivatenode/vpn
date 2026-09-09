// internal/lndrpc/client.go

// Package lndrpc provides a gRPC client for LND.
//
// The client reads the TLS certificate and admin macaroon from
// the staging board — root-staged copies the admin user reads
// directly, no privileged operation on the read path — and
// holds the macaroon in memory for the duration of the process.
// The macaroon is injected into every gRPC call as metadata.
//
// Connection uses TLS to the loopback address. The macaroon
// never crosses the network. When the TUI process exits, the
// macaroon is gone from memory.
//
// Read queries and fund-moving RPCs share this one client;
// every fund-moving call sits behind an explicit confirmation
// flow in its caller.
package lndrpc

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"

	"github.com/lightningnetwork/lnd/lnrpc"

	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
)

// Client wraps an LND gRPC connection with macaroon authentication.
type Client struct {
	conn        *grpc.ClientConn
	lightning   lnrpc.LightningClient
	state       lnrpc.StateClient
	macaroonHex string
	mu          sync.RWMutex
}

// New creates a new LND gRPC client. It reads the TLS certificate
// and admin macaroon, establishes the connection, and verifies it
// with a GetInfo call.
//
// Returns a client even if LND is not available — RPC methods
// check for a live connection internally and return
// errNotConnected if the connection is nil.
func New() *Client {
	c := &Client{}
	if err := c.connect(); err != nil {
		logger.Status("LND gRPC not available: %v", err)
		return c
	}
	return c
}

func (c *Client) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dial(context.Background(), true)
}

// dial establishes the connection; the caller holds c.mu.
//
// allowHeal enables the staged-credential self-heal: LND can
// regenerate its TLS certificate OUTSIDE any helper operation
// (an automatic restart after a crash, a reboot, or a typed
// configuration change), and its macaroon can be
// recreated the same way — and the staged board copies then no
// longer match what LND accepts. That staleness reliably
// surfaces exactly here, as a failed test call. The heal keys
// on the ABSENCE OF SUCCESS, not on recognizing the failure's
// text: enumerating the failure messages worth healing is a
// whitelist, and a whitelist misses (a dial error this client
// once produced was worded in a way no pattern anticipated).
// On any test-call failure, the client asks the helper to
// re-stage the LND credentials — rate-limited process-wide, so
// failures with nothing to heal (LND down, wallet locked) cost
// at most one re-stage per interval — and dials again ONCE
// with the refreshed board copies.
func (c *Client) dial(parent context.Context, allowHeal bool) error {
	if err := parent.Err(); err != nil {
		return err
	}
	// Read the staged TLS cert copy (fail-noisy: a missing
	// staged fact names itself and points at the journal).
	certData, err := helper.ReadBoard(paths.StateLNDTLSCert)
	if err != nil {
		return fmt.Errorf("read TLS cert: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(certData) {
		return fmt.Errorf("failed to parse TLS cert")
	}

	tlsCreds := credentials.NewClientTLSFromCert(certPool, "")

	// Read the staged admin macaroon copy (staged at wallet
	// creation; re-staged whenever an operation invalidates it).
	macBytes, err := helper.ReadBoard(paths.StateLNDMacaroon)
	if err != nil {
		return fmt.Errorf("read macaroon: %w", err)
	}
	c.macaroonHex = hex.EncodeToString(macBytes)

	// Connect. Keepalive bounds how long a silently dead TCP
	// connection (LND restarted underneath the TUI, a network
	// blip) can block calls and streams instead of blocking
	// forever; LND disconnects clients that ping more often
	// than every five seconds, and one minute stays far clear
	// of that floor with pings only while calls are active.
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(50 * 1024 * 1024)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    time.Minute,
			Timeout: 20 * time.Second,
		}),
	}

	// Dial by the shared loopback constant — the same value
	// lnd.conf binds to. Never by the name localhost: with
	// IPv6 disabled on the node, that name can resolve to an
	// IPv6 address the connection cannot use.
	conn, err := grpc.NewClient(paths.LNDGRPCEndpoint, opts...)
	if err != nil {
		return fmt.Errorf("grpc connect: %w", err)
	}

	c.conn = conn
	c.lightning = lnrpc.NewLightningClient(conn)
	c.state = lnrpc.NewStateClient(conn)

	// Test the connection with a longer timeout.
	// During IBD, LND's GetInfo queries Bitcoin Core which can
	// be slow. The probe context is built inline rather than
	// through macaroonCtx: dial runs under the write lock and
	// macaroonCtx takes the read lock.
	md := metadata.New(map[string]string{"macaroon": c.macaroonHex})
	probeCtx := metadata.NewOutgoingContext(parent, md)
	ctx, cancel := context.WithTimeout(probeCtx, 30*time.Second)
	defer cancel()

	_, err = c.lightning.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err == nil {
		logger.Status("LND gRPC connected and ready")
		return nil
	}
	if parent.Err() != nil {
		conn.Close()
		c.conn, c.lightning, c.state = nil, nil, nil
		return parent.Err()
	}
	if allowHeal && requestCredentialRestage(parent) {
		logger.Status("LND gRPC test call failed (%v) — staged "+
			"credentials re-staged in case they were stale, "+
			"reconnecting once", err)
		c.conn.Close()
		c.conn = nil
		c.lightning = nil
		c.state = nil
		return c.dial(parent, false)
	}
	if parent.Err() != nil {
		conn.Close()
		c.conn, c.lightning, c.state = nil, nil, nil
		return parent.Err()
	}
	logger.Status("LND gRPC connected, waiting for RPC ready: %v", err)
	return nil
}

// Credential self-heal state: at most one re-stage request per
// interval, PROCESS-WIDE — with the heal keyed on any failed
// test call, this limiter is what keeps a long LND outage from
// turning every reconnect poll into helper traffic and journal
// noise.
var (
	credRestageMu sync.Mutex
	credRestageAt time.Time
)

const credRestageInterval = 5 * time.Minute

// restageDue reports whether enough time has passed since the
// last re-stage request for another one. Pure — unit-tested.
func restageDue(last, now time.Time) bool {
	return last.IsZero() || now.Sub(last) >= credRestageInterval
}

// credRestageWorthwhile reports whether a re-stage request made
// now would pass the limiter, WITHOUT consuming it. The
// credential-rejection path in handleError checks this before
// reconnecting: when no repair can happen, no reconnect can
// help, and staying quiet caps a permanently rejected
// credential at one reconnect-and-repair attempt (and one log
// line) per limiter interval instead of one per failing call.
func credRestageWorthwhile() bool {
	credRestageMu.Lock()
	defer credRestageMu.Unlock()
	return restageDue(credRestageAt, time.Now())
}

// requestCredentialRestage asks the helper to refresh the staged
// LND credentials from current reality. Reports whether a
// re-stage actually happened (rate limit passed and the helper
// succeeded), so the caller only re-dials when the board copies
// could have changed.
func requestCredentialRestage(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	credRestageMu.Lock()
	defer credRestageMu.Unlock()
	if !restageDue(credRestageAt, time.Now()) {
		return false
	}
	credRestageAt = time.Now()
	session, err := helper.StartContext(ctx, helper.VerbStageLNDCredentials, nil)
	if err == nil {
		err = session.Wait(nil)
	}
	if err != nil {
		logger.Status("stage-lnd-credentials: %v", err)
		return false
	}
	return true
}

// Reconnect attempts to re-establish the gRPC connection.
// Called when an RPC fails, indicating LND may have restarted.
func (c *Client) Reconnect() {
	c.ReconnectContext(context.Background())
}

// ReconnectContext observes the caller's lifetime for the connection probe and
// credential restaging. A cancelled caller cannot schedule a later reconnect.
func (c *Client) ReconnectContext(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	if c.conn != nil {
		c.conn.Close()
	}
	c.conn, c.lightning, c.state = nil, nil, nil
	if err := c.dial(ctx, true); err != nil {
		logger.Status("LND gRPC reconnect failed: %v", err)
	}
}

// Close shuts down the gRPC connection.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.lightning = nil
		c.state = nil
	}
}

// macaroonCtx returns a context with the macaroon injected as
// gRPC metadata. The field is read under the lock — Reconnect
// rewrites it on another goroutine.
func (c *Client) macaroonCtx() context.Context {
	c.mu.RLock()
	macHex := c.macaroonHex
	c.mu.RUnlock()
	md := metadata.New(map[string]string{
		"macaroon": macHex,
	})
	return metadata.NewOutgoingContext(context.Background(), md)
}

// callCtx returns a context with macaroon and a timeout.
func (c *Client) callCtx(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.macaroonCtx(), timeout)
}

// rpc returns the Lightning client, or nil if not connected.
func (c *Client) rpc() lnrpc.LightningClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lightning
}

func (c *Client) stateRPC() lnrpc.StateClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}
