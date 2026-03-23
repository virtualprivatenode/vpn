package welcome

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

func readOnion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		output, err := system.SudoRunOutput("cat", path)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(output)
	}
	return strings.TrimSpace(string(data))
}

func readMacaroonHex(cfg *config.AppConfig) string {
	network := cfg.Network
	if cfg.IsMainnet() {
		network = "mainnet"
	}
	path := paths.LNDMacaroon(network)

	data, err := os.ReadFile(path)
	if err != nil {
		data, err = system.SudoReadFile(path)
		if err != nil {
			logger.Status("Warning: failed to read macaroon: %v", err)
			return ""
		}
	}
	return hex.EncodeToString(data)
}

func getSyncthingVersion() string {
	output, err := system.RunContext(3*time.Second, "syncthing", "--version")
	if err != nil {
		return "unknown"
	}
	fields := strings.Fields(output)
	if len(fields) >= 2 {
		return fields[1]
	}
	return "unknown"
}

// ── Fee estimation via bitcoin-cli ───────────────────────

type smartFeeResponse struct {
	FeeRate float64  `json:"feerate"`
	Errors  []string `json:"errors"`
	Blocks  int      `json:"blocks"`
}

func fetchFeeTiers(cfg *config.AppConfig) feeTiersMsg {
	targets := [4]int{1, 3, 6, 25}
	labels := [4]string{"~1 blk", "~3 blk", "~6 blk", "~25 blk"}
	var tiers [4]feeTier

	cliName := "bitcoin-cli"
	if cfg.Network == "testnet" {
		cliName = "bitcoin-cli"
	}

	for i, target := range targets {
		tiers[i] = feeTier{
			Target: target,
			Label:  labels[i],
		}

		output, err := system.RunContext(
			5*time.Second,
			"sudo", "-u", "bitcoin",
			cliName,
			fmt.Sprintf("-conf=%s", paths.BitcoinConf),
			"estimatesmartfee",
			fmt.Sprintf("%d", target),
		)
		if err != nil {
			continue
		}

		var resp smartFeeResponse
		if err := json.Unmarshal(
			[]byte(strings.TrimSpace(output)),
			&resp,
		); err != nil {
			continue
		}

		if resp.FeeRate > 0 && len(resp.Errors) == 0 {
			// bitcoin-cli returns BTC/kB
			// Convert to sat/vB:
			// BTC/kB × 100,000,000 / 1000 = sat/vB
			satPerVB := resp.FeeRate * 100000
			if satPerVB < 1 {
				satPerVB = 1
			}
			tiers[i].SatPerVB = satPerVB
		}
	}

	// Check if we got at least one valid tier
	anyValid := false
	for _, t := range tiers {
		if t.SatPerVB > 0 {
			anyValid = true
			break
		}
	}
	if !anyValid {
		return feeTiersMsg{
			err: fmt.Errorf("no fee estimates available"),
		}
	}

	return feeTiersMsg{tiers: tiers}
}

// isValidOnChainAddr does a basic prefix check.
// LND will do full validation on send.
func isValidOnChainAddr(addr string, network string) bool {
	if len(addr) < 14 {
		return false
	}
	switch network {
	case "mainnet":
		return strings.HasPrefix(addr, "bc1") ||
			strings.HasPrefix(addr, "1") ||
			strings.HasPrefix(addr, "3")
	case "testnet":
		return strings.HasPrefix(addr, "tb1") ||
			strings.HasPrefix(addr, "2") ||
			strings.HasPrefix(addr, "m") ||
			strings.HasPrefix(addr, "n")
	case "regtest":
		return strings.HasPrefix(addr, "bcrt1")
	case "signet":
		return strings.HasPrefix(addr, "tb1") ||
			strings.HasPrefix(addr, "sb1")
	}
	return true // unknown network, let LND validate
}

// renderViewport creates a local viewport, sets content,
// auto-scrolls to keep cursorLine visible, and returns the
// rendered view with scroll indicators overlaid.
//
// Parameters:
//
//	content    - fully rendered string of all lines
//	w          - viewport width
//	vpH        - viewport height (visible lines)
//	cursorLine - which line the cursor is on (0-based)
//	totalLines - total number of lines in content
//	active     - whether auto-scroll should be applied
func renderViewport(
	content string, w, vpH, cursorLine int,
	totalLines int, active bool,
) string {
	vp := viewport.New(
		viewport.WithWidth(w),
		viewport.WithHeight(vpH),
	)
	vp.FillHeight = true
	vp.SetContent(content)

	// Auto-scroll to keep cursor visible
	if active && totalLines > vpH {
		offset := vp.YOffset()
		if cursorLine < offset {
			vp.SetYOffset(cursorLine)
		}
		if cursorLine >= offset+vpH {
			vp.SetYOffset(cursorLine - vpH + 1)
		}
	}

	vpView := vp.View()
	vpLines := strings.Split(vpView, "\n")

	hasAbove := vp.YOffset() > 0
	hasBelow := vp.YOffset()+vpH < totalLines

	if hasAbove && len(vpLines) > 0 {
		indicator := strings.Repeat(" ", w-4) +
			theme.Dim.Render(" ▲")
		vpLines[0] = indicator
	}
	if hasBelow && len(vpLines) > 0 {
		indicator := strings.Repeat(" ", w-4) +
			theme.Dim.Render(" ▼")
		vpLines[len(vpLines)-1] = indicator
	}

	return strings.Join(vpLines, "\n")
}
