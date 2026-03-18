// internal/welcome/peers.go

package welcome

// curatedPeers returns well-known, reliable Lightning nodes.
func curatedPeers() []peerOption {
	return []peerOption{
		{
			Alias:       "ACINQ",
			Pubkey:      "03864ef025fde8fb587d989186ce6a4a186895ee44a926bfc370e2c366597a3f8f",
			Host:        "3.33.236.230:9735",
			TorOnly:     false,
			Curated:     true,
			MinChanSize: 400000,
		},
		{
			Alias:       "Zeus",
			Pubkey:      "031b301307574bbe9b9ac7b79cbe1700e31e544513eae0b5d7497483083f99e581",
			Host:        "45.79.192.236:9735",
			TorOnly:     false,
			Curated:     true,
			MinChanSize: 150000,
			Note:        "Unannounced channels only from new nodes",
		},
		{
			Alias:   "WalletOfSatoshi",
			Pubkey:  "035e4ff418fc8b5554c5d9eea66396c227bd429a3251c8cbc711002ba215bfc226",
			Host:    "170.75.163.209:9735",
			TorOnly: false,
			Curated: true,
			Note:    "Clearnet only — may not accept Tor",
		},
		{
			Alias:   "Kraken 🐙⚡",
			Pubkey:  "02f1a8c87607f415c8f22c00593002775941dea48869ce23096af27b0cfdcc0b69",
			Host:    "52.13.118.208:9735",
			TorOnly: false,
			Curated: true,
		},
		{
			Alias:   "Boltz",
			Pubkey:  "026165850492521f4ac8abd9bd8088123446d126f648ca35e60f88177dc149ceb2",
			Host:    "24.199.120.64:9735",
			TorOnly: false,
			Curated: true,
			Note:    "Clearnet only — may not accept Tor",
		},
		{
			Alias:       "LNBig (Tor)",
			Pubkey:      "034ea80f8b148c750463546bd999bf7321a0e6dfc60aaf84bd0400a2e8d376c0d5",
			Host:        "qimt6abvc2iuexwrtl5tzyrygnu7mshjahvresve5hdli6nstdg7elyd.onion:9735",
			TorOnly:     true,
			Curated:     true,
			MinChanSize: 500000,
		},
	}
}
