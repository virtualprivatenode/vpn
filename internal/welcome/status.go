// internal/welcome/status.go

package welcome

import (
	"sync"

	"github.com/ripsline/virtual-private-node/internal/bitcoin"
	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/lndrpc"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"

	tea "github.com/charmbracelet/bubbletea"
)

func fetchStatus(cfg *config.AppConfig, lndClient *lndrpc.Client) tea.Cmd {
	return func() tea.Msg {
		s := statusMsg{services: make(map[string]bool)}
		var wg sync.WaitGroup
		var mu sync.Mutex

		// Service checks (fast, run in parallel)
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

		// Bitcoin info (slow RPC, own goroutine)
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := bitcoin.GetBlockchainInfo()
			mu.Lock()
			s.btcResponding = info.Responding
			s.btcBlocks = info.Blocks
			s.btcHeaders = info.Headers
			s.btcProgress = info.Progress
			s.btcSynced = info.Synced
			mu.Unlock()
		}()

		// System info (disk, memory — moderate speed)
		wg.Add(1)
		go func() {
			defer wg.Done()
			disk := system.Disk("/")
			mem := system.Memory()
			btcSize := system.DirSize(paths.BitcoinDataDir)
			var lndSize string
			if cfg.HasLND() {
				lndSize = system.DirSize(paths.LNDDataDir)
			}
			mu.Lock()
			s.diskTotal = disk.Total
			s.diskUsed = disk.Used
			s.diskPct = disk.Percent
			s.ramTotal = mem.Total
			s.ramUsed = mem.Used
			s.ramPct = mem.Percent
			s.btcSize = btcSize
			s.lndSize = lndSize
			mu.Unlock()
		}()

		// LND info via gRPC (fast if connected)
		if cfg.HasLND() && lndClient != nil && lndClient.IsConnected() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				lndInfo, err := lndClient.GetInfo()
				mu.Lock()
				if err == nil {
					s.lndResponding = true
					s.lndPubkey = lndInfo.Pubkey
					s.lndChannels = lndInfo.Channels
					s.lndSyncedChain = lndInfo.SyncedChain
					s.lndSyncedGraph = lndInfo.SyncedGraph

					if !cfg.WalletExists() && lndInfo.Pubkey != "" {
						s.walletDetected = true
					}
				}
				mu.Unlock()
			}()

			// Wallet balance via gRPC
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
		}

		wg.Wait()

		// Cached public IP (no network call — uses ip route)
		if cfg.P2PMode == "hybrid" {
			s.publicIP = system.PublicIPv4()
		}

		s.rebootRequired = system.RebootRequired()

		return s
	}
}
