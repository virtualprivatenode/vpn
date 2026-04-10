// internal/lndrpc/client.go

// Package lndrpc provides a gRPC client for LND.
//
// The client reads the admin macaroon once at startup via sudo
// (using a temp file that's immediately deleted) and holds it
// in memory for the duration of the process. The macaroon is
// injected into every gRPC call as metadata.
//
// Connection uses TLS to localhost. The macaroon never crosses
// the network. When the TUI process exits, the macaroon is gone
// from memory.
//
// This package only performs read operations. Fund-moving RPCs
// (SendPayment, OpenChannel, etc.) are added in later changes
// with explicit confirmation flows.
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
	"google.golang.org/grpc/metadata"

	"github.com/lightningnetwork/lnd/lnrpc"

	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
)

// Client wraps an LND gRPC connection with macaroon authentication.
type Client struct {
	conn        *grpc.ClientConn
	lightning   lnrpc.LightningClient
	macaroonHex string
	network     string
	mu          sync.RWMutex
}

// New creates a new LND gRPC client. It reads the TLS certificate
// and admin macaroon, establishes the connection, and verifies it
// with a GetInfo call.
//
// Returns nil with a logged warning if LND is not available.
// The TUI should handle a nil client gracefully (show "waiting for LND").
func New(network string) *Client {
	c := &Client{network: network}
	if err := c.connect(); err != nil {
		logger.Status("LND gRPC not available: %v", err)
		return c
	}
	return c
}

func (c *Client) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Read TLS cert
	certData, err := system.SudoReadFile(paths.LNDTLSCert)
	if err != nil {
		return fmt.Errorf("read TLS cert: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(certData) {
		return fmt.Errorf("failed to parse TLS cert")
	}

	tlsCreds := credentials.NewClientTLSFromCert(certPool, "")

	// Read macaroon
	macaroonPath := paths.LNDMacaroon(c.networkDir())
	macBytes, err := system.SudoReadFile(macaroonPath)
	if err != nil {
		return fmt.Errorf("read macaroon: %w", err)
	}
	c.macaroonHex = hex.EncodeToString(macBytes)

	// Connect
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(50 * 1024 * 1024)),
	}

	conn, err := grpc.NewClient("localhost:10009", opts...)
	if err != nil {
		return fmt.Errorf("grpc connect: %w", err)
	}

	c.conn = conn
	c.lightning = lnrpc.NewLightningClient(conn)

	// Test the connection with a longer timeout.
	// During IBD, LND's GetInfo queries Bitcoin Core which can be slow.
	ctx, cancel := context.WithTimeout(c.macaroonCtx(), 30*time.Second)
	defer cancel()

	_, err = c.lightning.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		logger.Status("LND gRPC connected, waiting for RPC ready: %v", err)
	} else {
		logger.Status("LND gRPC connected and ready")
	}

	return nil
}

// Reconnect attempts to re-establish the gRPC connection.
// Called when an RPC fails, indicating LND may have restarted.
func (c *Client) Reconnect() {
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.conn = nil
	c.lightning = nil
	c.mu.Unlock()

	if err := c.connect(); err != nil {
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
	}
}

// macaroonCtx returns a context with the macaroon injected as gRPC metadata.
func (c *Client) macaroonCtx() context.Context {
	md := metadata.New(map[string]string{
		"macaroon": c.macaroonHex,
	})
	return metadata.NewOutgoingContext(context.Background(), md)
}

// callCtx returns a context with macaroon and a timeout.
func (c *Client) callCtx(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.macaroonCtx(), timeout)
}

// networkDir returns the macaroon network directory name.
// LND uses "mainnet" not "mainnet" in the path for mainnet.
func (c *Client) networkDir() string {
	if c.network == "" {
		return "mainnet"
	}
	return c.network
}

// rpc returns the Lightning client, or nil if not connected.
func (c *Client) rpc() lnrpc.LightningClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lightning
}
