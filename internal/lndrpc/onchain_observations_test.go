package lndrpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestOnChainReadRPCBoundary(t *testing.T) {
	for _, failure := range []string{"", "ListUnspent", "GetTransactions", "ListChannels", "PendingChannels", "ClosedChannels", "GetNodeInfo"} {
		t.Run("failure_"+failure, func(t *testing.T) {
			parent, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			parent = metadata.AppendToOutgoingContext(parent, "trace", "onchain", "macaroon", "obsolete")
			calls := map[string]int{}
			injected := errors.New("injected read failure")
			// Intercept generated requests before transport. No daemon or host
			// credentials are involved; cancellation still uses real contexts.
			conn, err := grpc.NewClient("passthrough:///onchain-test.invalid",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithUnaryInterceptor(func(ctx context.Context, method string, request, response any, _ *grpc.ClientConn, _ grpc.UnaryInvoker, _ ...grpc.CallOption) error {
					name := method[strings.LastIndex(method, "/")+1:]
					calls[name]++
					md, _ := metadata.FromOutgoingContext(ctx)
					if len(md.Get("macaroon")) != 1 || md.Get("macaroon")[0] != "test-only" || len(md.Get("trace")) != 1 {
						t.Error("authentication replaced caller metadata or retained obsolete credential")
					}
					deadline, ok := ctx.Deadline()
					limit, _ := parent.Deadline()
					if !ok || deadline.After(limit) {
						t.Error("enrichment extended caller deadline")
					}
					if name == failure {
						if name == "GetNodeInfo" {
							cancel()
							<-ctx.Done()
							return ctx.Err()
						}
						return injected
					}
					switch req := request.(type) {
					case *walletrpc.ListUnspentRequest:
						if req.MinConfs != 0 || req.MaxConfs != 999999 || req.Account != "" || req.UnconfirmedOnly {
							t.Error("UTXO request changed wallet scope or confirmation bounds")
						}
						response.(*walletrpc.ListUnspentResponse).Utxos = []*lnrpc.Utxo{{AmountSat: 10000, Confirmations: 2, Address: "address", Outpoint: &lnrpc.OutPoint{TxidBytes: append(make([]byte, 31), 1), OutputIndex: 3}}}
					case *lnrpc.GetTransactionsRequest:
						if req.StartHeight != 0 || req.EndHeight != 0 || req.Account != "" {
							t.Error("history request changed range or account")
						}
						response.(*lnrpc.TransactionDetails).Transactions = []*lnrpc.Transaction{{TxHash: "funding", Amount: -10000, TimeStamp: 1}, {TxHash: "receive", Amount: 20000, TimeStamp: 2}}
					case *lnrpc.ListChannelsRequest:
						response.(*lnrpc.ListChannelsResponse).Channels = []*lnrpc.Channel{{ChannelPoint: "funding:0", RemotePubkey: "peer"}}
					case *lnrpc.PendingChannelsRequest, *lnrpc.ClosedChannelsRequest:
					case *lnrpc.NodeInfoRequest:
						response.(*lnrpc.NodeInfo).Node = &lnrpc.LightningNode{Alias: "peer alias"}
					default:
						t.Errorf("unexpected RPC request %T", request)
					}
					return nil
				}))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			c := &Client{conn: conn, lightning: lnrpc.NewLightningClient(conn), macaroonHex: "test-only"}
			coins, coinErr := c.ListUnspentContext(parent, 0, 999999)
			if failure == "ListUnspent" {
				if !errors.Is(coinErr, injected) || coins != nil {
					t.Fatal("UTXO failure became empty success")
				}
				return
			}
			if coinErr != nil || len(coins) != 1 || coins[0].Txid != "01"+strings.Repeat("00", 31) || coins[0].Vout != 3 || coins[0].AmountSats != 10000 {
				t.Fatalf("UTXO conversion failed: %+v %v", coins, coinErr)
			}
			txs, txErr := c.GetTransactionsContext(parent)
			switch failure {
			case "":
				if txErr != nil || len(txs) != 2 || txs[0].Txid != "receive" || txs[1].TxType != "channel_open" || txs[1].ChannelPeer != "peer alias" {
					t.Fatalf("history conversion failed: %+v %v", txs, txErr)
				}
				for _, name := range []string{"GetTransactions", "ListChannels", "PendingChannels", "ClosedChannels", "GetNodeInfo"} {
					if calls[name] != 1 {
						t.Fatalf("%s calls=%d", name, calls[name])
					}
				}
			case "GetNodeInfo":
				if !errors.Is(txErr, context.Canceled) || txs != nil {
					t.Fatal("alias enrichment swallowed parent cancellation")
				}
			default:
				if !errors.Is(txErr, injected) || txs != nil {
					t.Fatalf("%s failure published partial history: %v", failure, txErr)
				}
			}
		})
	}
}
