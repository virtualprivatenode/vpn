// internal/installer/staging.go

package installer

// Staging-board writers. Each function refreshes ONE fact under
// /var/lib/vpn/state from current reality, running as root (the
// installer during `vpn install`, the helper afterwards). The
// helper's freshness matrix (internal/helperd/matrix.go) maps
// operations to the facts they invalidate and calls these; the
// install's staging step calls StageBoardAll once so a finished
// install always leaves a complete, current board.

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
)

// StageLNDTLSCert copies LND's TLS certificate (the public
// half only — tls.key never moves) to the board. LND rewrites
// the cert during startup when its parameters change, so this
// polls until the file parses as a stable certificate: two
// consecutive reads, half a second apart, must agree and parse.
func StageLNDTLSCert() error {
	deadline := time.Now().Add(60 * time.Second)
	var prev []byte
	for {
		data, err := os.ReadFile(paths.LNDTLSCert)
		if err == nil && certParses(data) {
			if prev != nil && bytes.Equal(prev, data) {
				return helper.WriteBoard(
					paths.StateLNDTLSCert, data)
			}
			prev = data
		} else {
			prev = nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf(
				"LND TLS certificate at %s not readable/stable "+
					"after 60s", paths.LNDTLSCert)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// certParses reports whether data holds a parseable PEM
// certificate — the guard against copying a half-written file.
func certParses(data []byte) bool {
	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}
	_, err := x509.ParseCertificate(block.Bytes)
	return err == nil
}

// StageLNDMacaroon copies the admin macaroon to the board. It
// requires the macaroon to exist: the callers are the moments
// that create it (wallet creation) or that follow its known
// existence. For the install-time case where no wallet exists
// yet, see StageBoardAll, which skips it.
func StageLNDMacaroon() error {
	network, err := macaroonNetworkDir()
	if err != nil {
		return err
	}
	src := paths.LNDMacaroon(network)
	deadline := time.Now().Add(30 * time.Second)
	for {
		data, err := os.ReadFile(src)
		if err == nil && len(data) > 0 {
			return helper.WriteBoard(paths.StateLNDMacaroon, data)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf(
				"admin macaroon at %s not readable after 30s "+
					"(%v)", src, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// macaroonNetworkDir resolves the network directory in LND's
// macaroon path from the node's config.
func macaroonNetworkDir() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("read node config: %w", err)
	}
	profile, err := cfg.NetworkConfig()
	if err != nil {
		return "", fmt.Errorf("resolve node network profile: %w", err)
	}
	return profile.LNDNetwork, nil
}

// StageSyncthingAPIKey extracts the GUI API key from
// Syncthing's config and stages it. With the key on the board,
// every runtime device operation (pair, unpair, folder
// sharing) is plain localhost REST with no privilege at all.
func StageSyncthingAPIKey() error {
	key, err := getSyncthingAPIKey()
	if err != nil {
		return fmt.Errorf("read Syncthing API key: %w", err)
	}
	return helper.WriteBoard(
		paths.StateSyncthingAPIKey, []byte(key+"\n"))
}

// StageBoardAll builds the complete board at install time:
// every fact that exists on this box right now. Facts whose
// source does not exist yet are SKIPPED, not failed — a fresh
// box has no wallet macaroon until the operator creates the
// wallet (that moment stages it), and no Syncthing facts until
// Syncthing is installed.
func StageBoardAll() error {
	if err := helper.FixBoardOwnership(); err != nil {
		return err
	}

	type fact struct {
		name string
		fn   func() error
		skip func() bool // true = source legitimately absent
	}
	cfg, cfgErr := config.Load()
	haveSyncthing := cfgErr == nil && cfg.SyncthingEnabled
	haveWallet := false
	if network, err := macaroonNetworkDir(); err == nil {
		_, statErr := os.Stat(paths.LNDMacaroon(network))
		haveWallet = statErr == nil
	}

	facts := []fact{
		{"lnd-tls-cert", StageLNDTLSCert, nil},
		{"lnd-admin-macaroon", StageLNDMacaroon,
			func() bool { return !haveWallet }},
		{"syncthing-api-key", StageSyncthingAPIKey,
			func() bool { return !haveSyncthing }},
	}
	for _, f := range facts {
		if f.skip != nil && f.skip() {
			logger.Install(
				"staging: %s skipped (source not present yet)",
				f.name)
			continue
		}
		if err := f.fn(); err != nil {
			return fmt.Errorf("stage %s: %w", f.name, err)
		}
	}
	logger.Install("staging board complete at %s", paths.StateDir)
	return nil
}
