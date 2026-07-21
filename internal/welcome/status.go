package welcome

import (
	"strings"
	"sync"
	"time"

	"github.com/virtualprivatenode/vpn/internal/bitcoin"
	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/lndrpc"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/system"

	tea "charm.land/bubbletea/v2"
)

func fetchStatus(cfg *config.AppConfig, lndClient *lndrpc.Client) tea.Cmd {
	return func() tea.Msg {
		s := statusMsg{services: make(map[string]bool)}
		var wg sync.WaitGroup
		var mu sync.Mutex

		for _, name := range serviceNames(cfg) {
			wg.Add(1)
			go func(n string) {
				defer wg.Done()
				active := system.IsServiceActive(n)
				mu.Lock()
				s.services[n] = active
				mu.Unlock()
			}(name)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			info := bitcoin.GetBlockchainInfo(
				cfg.NetworkConfig().RPCPort)
			mu.Lock()
			s.btcResponding = info.Responding
			s.btcBlocks = info.Blocks
			s.btcHeaders = info.Headers
			s.btcProgress = info.Progress
			s.btcSynced = info.Synced
			// bitcoind reports its own data footprint
			// (size_on_disk) — no privileged measurement of
			// the data dir is needed for this card.
			if info.Responding {
				s.btcSize = bitcoin.FormatSize(info.SizeOnDisk)
			} else {
				s.btcSize = "N/A"
			}
			mu.Unlock()
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			disk := system.Disk("/")
			mem := system.Memory()
			var lndSize string
			if cfg.HasLND() {
				lndSize = cachedLNDSize()
			}
			mu.Lock()
			s.diskTotal = disk.Total
			s.diskUsed = disk.Used
			s.diskPct = disk.Percent
			s.ramTotal = mem.Total
			s.ramUsed = mem.Used
			s.ramPct = mem.Percent
			s.lndSize = lndSize
			mu.Unlock()
		}()

		// LND owns its TLS cert lifecycle via
		// tlsautorefresh=1 in lnd.conf, so the cert
		// is always present on disk when LND is up.
		// The status fetcher attempts RPCs whenever
		// the wallet exists; if LND is down or its
		// gRPC connection is stale, the RPC fails
		// with "Unavailable" and handleError triggers
		// Reconnect() automatically.
		if cfg.HasLND() && lndClient != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				lndInfo, err := lndClient.GetInfo()
				mu.Lock()
				if err == nil {
					s.lndResponding = true
					s.lndPubkey = lndInfo.Pubkey
					s.lndAlias = lndInfo.Alias
					s.lndURIs = lndInfo.URIs
					// LND reports Version as e.g.
					// "0.20.0-beta commit=v0.20.0-beta".
					// Strip the commit=... suffix so
					// the user-facing display shows just
					// the semver. Any downstream code
					// that needs the full string can
					// call GetInfo directly.
					if fields := strings.Fields(
						lndInfo.Version); len(fields) > 0 {
						s.lndVersion = fields[0]
					}
					s.lndPeers = lndInfo.Peers
					s.lndChannels = lndInfo.Channels
					s.lndSyncedChain = lndInfo.SyncedChain
					s.lndSyncedGraph = lndInfo.SyncedGraph
					if !cfg.WalletExists() && lndInfo.Pubkey != "" {
						s.walletDetected = true
					}
				}
				mu.Unlock()
			}()

			if cfg.WalletExists() {
				wg.Add(1)
				go func() {
					defer wg.Done()
					bal, err := lndClient.GetWalletBalance()
					mu.Lock()
					if err == nil && bal.TotalBalance != "" {
						s.lndBalance = bal.TotalBalance
					}
					mu.Unlock()
				}()
			}

			// IMPORTANT: fetch open+pending channels together to avoid flicker/races.
			wg.Add(1)
			go func() {
				defer wg.Done()

				channels, errCh := lndClient.ListChannels()
				pending, errPend := lndClient.GetPendingChannels()

				merged := make([]channelInfo, 0, len(channels)+8)

				if errCh == nil {
					for _, ch := range channels {
						merged = append(merged, channelInfo{
							ChanID:         ch.ChanID,
							ChannelPoint:   ch.ChannelPoint,
							PeerAlias:      ch.PeerAlias,
							RemotePubkey:   ch.RemotePubkey,
							Capacity:       ch.Capacity,
							LocalBalance:   ch.LocalBalance,
							RemoteBalance:  ch.RemoteBalance,
							Active:         ch.Active,
							Private:        ch.Private,
							Initiator:      ch.Initiator,
							CommitmentType: ch.CommitmentType,
						})
					}
				}

				if errPend == nil {
					for _, pc := range pending.PendingOpenChannels {
						merged = append(merged, channelInfo{
							RemotePubkey: pc.RemotePubkey,
							PeerAlias:    pc.PeerAlias,
							Capacity:     pc.Capacity,
							LocalBalance: pc.LocalBalance,
							Pending:      true,
						})
					}
				}

				mu.Lock()
				s.channels = merged
				if errPend == nil {
					s.pendingOpen = pending.PendingOpen
					s.pendingForceClose = pending.ForceClose
					s.pendingForceCloseChannels =
						pending.PendingForceCloseChannels
					s.waitingCloseChannels =
						pending.WaitingCloseChannels
				}
				mu.Unlock()
			}()
		}

		wg.Wait()

		if cfg.P2PMode == "hybrid" {
			s.publicIP = system.PublicIPv4()
		}
		s.rebootRequired = system.RebootRequired()

		return s
	}
}

// ── LND data-dir size (helper-measured, cached) ─────────
//
// The LND data dir belongs to the service user, so its size is
// measured by the root helper (dir-size operation). Directory
// sizes change slowly and the status poll is frequent, so the
// answer — including a failed answer — is cached for five
// minutes: the display stays fresh enough while the helper
// isn't woken on every poll tick.

var (
	lndSizeMu  sync.Mutex
	lndSizeVal string
	lndSizeAt  time.Time
)

func cachedLNDSize() string {
	lndSizeMu.Lock()
	defer lndSizeMu.Unlock()
	if !lndSizeAt.IsZero() &&
		time.Since(lndSizeAt) < 5*time.Minute {
		return lndSizeVal
	}
	var res helper.DirSizeResult
	if err := helper.Call(helper.VerbDirSize,
		helper.DirSizeParams{Which: "lnd"}, &res); err != nil {
		logger.Status("lnd dir size: %v", err)
		lndSizeVal = "N/A"
	} else {
		lndSizeVal = res.Size
	}
	lndSizeAt = time.Now()
	return lndSizeVal
}
