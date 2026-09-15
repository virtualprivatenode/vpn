package lndrpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestClosedChannelReadBoundary(t *testing.T) {
	for _, failure := range []string{"", "list", "alias", "cancel alias"} {
		t.Run("failure_"+failure, func(t *testing.T) {
			parent, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			parent = metadata.AppendToOutgoingContext(parent, "trace", "history", "macaroon", "obsolete")
			injected := errors.New("injected failure")
			conn, err := grpc.NewClient("passthrough:///history-test.invalid",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithUnaryInterceptor(func(ctx context.Context, _ string, request, response any, _ *grpc.ClientConn, _ grpc.UnaryInvoker, _ ...grpc.CallOption) error {
					md, _ := metadata.FromOutgoingContext(ctx)
					deadline, bounded := ctx.Deadline()
					limit, _ := parent.Deadline()
					if !bounded || deadline.After(limit) || strings.Join(md.Get("macaroon"), ",") != "test-only" || strings.Join(md.Get("trace"), ",") != "history" {
						t.Error("list or alias read lost caller lifetime, metadata or authentication")
					}
					switch req := request.(type) {
					case *lnrpc.ClosedChannelsRequest:
						if req.Cooperative || req.LocalForce || req.RemoteForce || req.Breach || req.FundingCanceled || req.Abandoned {
							t.Error("history unexpectedly filters closing types")
						}
						if failure == "list" {
							return injected
						}
						response.(*lnrpc.ClosedChannelsResponse).Channels = []*lnrpc.ChannelCloseSummary{{ChannelPoint: "funding:7", RemotePubkey: "peer", CloseType: lnrpc.ChannelCloseSummary_COOPERATIVE_CLOSE}}
					case *lnrpc.NodeInfoRequest:
						if req.PubKey != "peer" {
							t.Error("alias lookup used another channel identity")
						}
						if failure == "cancel alias" {
							cancel()
							return ctx.Err()
						}
						if failure == "alias" {
							return injected
						}
						response.(*lnrpc.NodeInfo).Node = &lnrpc.LightningNode{Alias: "peer alias"}
					default:
						t.Errorf("unexpected request %T", req)
					}
					return nil
				}))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			client := &Client{conn: conn, lightning: lnrpc.NewLightningClient(conn), macaroonHex: "test-only"}
			rows, err := client.ListClosedChannelsContext(parent)
			switch failure {
			case "list":
				if !errors.Is(err, injected) || rows != nil {
					t.Fatal("failed list became empty success")
				}
			case "cancel alias":
				if !errors.Is(err, context.Canceled) || rows != nil {
					t.Fatal("enrichment swallowed cancellation and published partial history")
				}
			default:
				if err != nil || len(rows) != 1 || rows[0].ChannelPoint != "funding:7" || rows[0].RemotePubkey != "peer" || rows[0].CloseType != "cooperative" {
					t.Fatalf("lost closed-channel identity or lifecycle: %+v %v", rows, err)
				}
				if failure == "alias" && rows[0].PeerAlias != "peer" || failure == "" && rows[0].PeerAlias != "peer alias" {
					t.Fatal("optional alias failure changed required channel evidence")
				}
			}
		})
	}
}
