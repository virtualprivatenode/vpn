package installer

import (
	"errors"
	"testing"

	"github.com/lightningnetwork/lnd/lnrpc"

	"github.com/virtualprivatenode/vpn/internal/paths"
)

func TestWalletExistsFromState(t *testing.T) {
	tests := []struct {
		name   string
		state  lnrpc.WalletState
		exists bool
		known  bool
	}{
		{"non-existing", lnrpc.WalletState_NON_EXISTING, false, true},
		{"locked", lnrpc.WalletState_LOCKED, true, true},
		{"unlocked", lnrpc.WalletState_UNLOCKED, true, true},
		{"rpc active", lnrpc.WalletState_RPC_ACTIVE, true, true},
		{"server active", lnrpc.WalletState_SERVER_ACTIVE, true, true},
		{"waiting", lnrpc.WalletState_WAITING_TO_START, false, false},
		{"unknown", lnrpc.WalletState(99), false, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exists, err := walletExistsFromState(test.state, nil)
			if test.known && err != nil {
				t.Fatalf("known state returned an error: %v", err)
			}
			if !test.known && err == nil {
				t.Fatal("unknown state reported a wallet fact")
			}
			if exists != test.exists {
				t.Fatalf("exists=%v, want %v", exists, test.exists)
			}
		})
	}

	probeErr := errors.New("state RPC unavailable")
	if exists, err := walletExistsFromState(
		lnrpc.WalletState_NON_EXISTING, probeErr,
	); err == nil || exists || !errors.Is(err, probeErr) {
		t.Fatalf("RPC error: exists=%v err=%v", exists, err)
	}
}

func TestWalletExistsRejectsUnknownProfileBeforeObservation(t *testing.T) {
	if _, err := WalletExists("signet"); err == nil {
		t.Fatal("raw signet profile reached live wallet observation")
	}
}

func TestSyncthingResidueIncludesStagedCredentials(t *testing.T) {
	seen := make(map[string]bool)
	for _, path := range syncthingResiduePaths {
		seen[path] = true
	}
	for _, path := range []string{
		paths.StateSyncthingAPIKey,
		paths.StateSyncthingWebPassword,
	} {
		if !seen[path] {
			t.Errorf("staged credential %s is not install residue", path)
		}
	}
}
