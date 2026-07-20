package lndrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"

	"github.com/virtualprivatenode/vpn/internal/logger"
)

// SendPaymentResult holds the outcome of a payment attempt.
type SendPaymentResult struct {
	Preimage string
	FeeSats  int64
	Status   string
	Hops     []RouteHop
	Error    string
}

// SendPayment sends a payment using SendPaymentV2 (streaming).
// Blocks until the payment succeeds, fails, or times out.
// Returns route hops on success for visualization.
func (c *Client) SendPayment(payReq string) (*SendPaymentResult, error) {
	if c.rpc() == nil {
		return nil, errNotConnected
	}
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return nil, errNotConnected
	}

	routerClient := routerrpc.NewRouterClient(conn)

	ctx, cancel := context.WithTimeout(c.macaroonCtx(), 120*time.Second)
	defer cancel()

	stream, err := routerClient.SendPaymentV2(ctx,
		&routerrpc.SendPaymentRequest{
			PaymentRequest:    payReq,
			TimeoutSeconds:    60,
			FeeLimitSat:       1000,
			MaxParts:          16,
			NoInflightUpdates: true,
		})
	if err != nil {
		return nil, fmt.Errorf("send payment: %w", err)
	}

	for {
		payment, err := stream.Recv()
		if err != nil {
			return nil, fmt.Errorf("payment stream: %w", err)
		}

		switch payment.GetStatus() {
		case lnrpc.Payment_SUCCEEDED:
			result := &SendPaymentResult{
				Preimage: payment.GetPaymentPreimage(),
				FeeSats:  payment.GetFeeSat(),
				Status:   "SUCCEEDED",
			}
			for _, htlc := range payment.GetHtlcs() {
				if htlc.GetStatus() == lnrpc.HTLCAttempt_SUCCEEDED &&
					htlc.GetRoute() != nil {
					for _, hop := range htlc.GetRoute().GetHops() {
						result.Hops = append(result.Hops, RouteHop{
							PubKey:   hop.GetPubKey(),
							ChanID:   hop.GetChanId(),
							FeeSats:  hop.GetFeeMsat() / 1000,
							AmtToFwd: hop.GetAmtToForwardMsat() / 1000,
						})
					}
					break
				}
			}
			for i := range result.Hops {
				result.Hops[i].Alias = c.getPeerAlias(
					result.Hops[i].PubKey)
			}
			logger.TUI("Payment succeeded: fee=%d sats, hops=%d",
				result.FeeSats, len(result.Hops))
			return result, nil

		case lnrpc.Payment_FAILED:
			reason := payment.GetFailureReason().String()
			logger.TUI("Payment failed: %s", reason)
			return &SendPaymentResult{
				Status: "FAILED",
				Error:  reason,
			}, nil

		case lnrpc.Payment_IN_FLIGHT:
			continue

		default:
			continue
		}
	}
}

// WaitForInvoiceSettlement polls an invoice until settled or expired.
// Uses re-subscribe pattern (Approach 1) — polls every 2 seconds.
func (c *Client) WaitForInvoiceSettlement(
	paymentHash []byte, expiry time.Duration,
) (*Invoice, error) {
	deadline := time.Now().Add(expiry)

	for time.Now().Before(deadline) {
		inv, err := c.LookupInvoice(paymentHash)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if inv.Settled {
			return inv, nil
		}
		if inv.IsExpired {
			return inv, nil
		}
		time.Sleep(2 * time.Second)
	}

	inv, err := c.LookupInvoice(paymentHash)
	if err != nil {
		return nil, fmt.Errorf("invoice lookup: %w", err)
	}
	return inv, nil
}
