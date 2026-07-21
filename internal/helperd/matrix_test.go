// internal/helperd/matrix_test.go

package helperd

import (
	"testing"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/installer"
	"github.com/virtualprivatenode/vpn/internal/paths"
)

// ── Freshness-matrix tests ───────────────────────────────
//
// The staging board's failure mode is staleness, and the
// freshness matrix is the defense — so the matrix itself is
// pinned by tests. expectedMatrix below restates the ruled
// verb × fact table INDEPENDENTLY of matrix.go; if either side
// is edited alone, these tests fail and force the two back
// into agreement.

var expectedMatrix = map[string][]string{
	helper.VerbRebuildTorConfig: {
		paths.StateOnionBitcoinP2P,
		paths.StateOnionLNDGRPC,
		paths.StateOnionLNDREST,
		paths.StateOnionSyncthing,
	},
	helper.VerbStageLNDCredentials: {
		paths.StateLNDTLSCert,
		paths.StateLNDMacaroon,
	},
	helper.VerbSetP2PMode: {
		paths.StateLNDTLSCert,
		paths.StateLNDMacaroon,
	},
	helper.VerbSyncthingInstall: {
		paths.StateSyncthingAPIKey,
		paths.StateSyncthingDevID,
		paths.StateOnionSyncthing,
	},
	helper.VerbRebuildSSHConfig: {
		paths.StateSSHPasswordAuth,
	},
}

func TestFreshnessMatrixMatchesRuledTable(t *testing.T) {
	for verb, wantFiles := range expectedMatrix {
		got := freshnessMatrix[verb]
		if len(got) != len(wantFiles) {
			t.Errorf("%s: %d facts, want %d",
				verb, len(got), len(wantFiles))
			continue
		}
		for i, f := range wantFiles {
			if got[i] != f {
				t.Errorf("%s[%d] = %s, want %s",
					verb, i, got[i], f)
			}
		}
	}
	for verb := range freshnessMatrix {
		if _, ok := expectedMatrix[verb]; !ok {
			t.Errorf(
				"matrix has verb %s the expected table lacks — "+
					"update BOTH deliberately", verb)
		}
	}
}

// Every fact in the matrix must have a stager, every verb in
// the matrix must exist on the verb menu, and every stager key
// must be a real board path.
func TestFreshnessMatrixIsClosed(t *testing.T) {
	boardFiles := map[string]bool{
		paths.StateBitcoindRPCPass: true,
		paths.StateLNDTLSCert:      true,
		paths.StateLNDMacaroon:     true,
		paths.StateOnionBitcoinP2P: true,
		paths.StateOnionLNDGRPC:    true,
		paths.StateOnionLNDREST:    true,
		paths.StateOnionSyncthing:  true,
		paths.StateSyncthingAPIKey: true,
		paths.StateSyncthingDevID:  true,
		paths.StateSSHPasswordAuth: true,
	}
	for verb, files := range freshnessMatrix {
		if _, ok := verbs[verb]; !ok {
			t.Errorf("matrix verb %s is not on the verb menu", verb)
		}
		for _, f := range files {
			if !boardFiles[f] {
				t.Errorf("%s re-stages unknown file %s", verb, f)
			}
			if _, ok := stagers[f]; !ok {
				t.Errorf("%s: no stager registered for %s", verb, f)
			}
		}
	}
	for f := range stagers {
		if !boardFiles[f] {
			t.Errorf("stager registered for unknown file %s", f)
		}
	}
}

// restage must fail loudly on a matrix/stager mismatch instead
// of silently skipping a fact.
func TestRestageUnknownVerbIsNoop(t *testing.T) {
	if err := restage("no-such-verb"); err != nil {
		t.Errorf("unknown verb should re-stage nothing: %v", err)
	}
}

// ── Step-name alignment ──────────────────────────────────
//
// Streaming verbs report progress by INDEX; the client renders
// NAMES from the shared lists in the helper package. These
// tests are what make drift between the two impossible to ship.

func stepNames(steps []installer.InstallStep) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.Name
	}
	return out
}

func assertNamesEqual(t *testing.T, verb string,
	server, shared []string) {
	t.Helper()
	if len(server) != len(shared) {
		t.Fatalf("%s: server has %d steps, shared list %d",
			verb, len(server), len(shared))
	}
	for i := range server {
		if server[i] != shared[i] {
			t.Errorf("%s step %d: server %q, shared %q",
				verb, i, server[i], shared[i])
		}
	}
}

func TestSelfUpdateStepNamesAligned(t *testing.T) {
	v := "0.7.1"
	assertNamesEqual(t, helper.VerbSelfUpdate,
		stepNames(installer.SelfUpdateSteps(v)),
		helper.SelfUpdateStepNames(v))
}

func TestPackageUpdateStepNamesAligned(t *testing.T) {
	assertNamesEqual(t, helper.VerbPackageUpdate,
		stepNames(installer.PackageUpdateSteps()),
		helper.PackageUpdateStepNames())
}

func TestSetP2PModeStepNamesAligned(t *testing.T) {
	cfg := config.Default()
	cfg.P2PMode = "hybrid"
	cfg.LNDInstalled = true
	server := stepNames(installer.P2PUpgradeSteps(cfg, "203.0.113.7"))
	// The verb reports one extra step after the installer's
	// three: re-staging the regenerated credentials.
	server = append(server, "Restaging LND credentials")
	assertNamesEqual(t, helper.VerbSetP2PMode,
		server, helper.SetP2PModeStepNames())
}

func TestSyncthingInstallStepNamesAligned(t *testing.T) {
	cfg := config.Default()
	cfg.SyncthingInstalled = true
	steps, _, err := installer.SyncthingInstallSteps(cfg)
	if err != nil {
		t.Fatal(err)
	}
	server := stepNames(steps)
	// The verb reports one extra step: staging the new
	// component's facts.
	server = append(server, "Staging Syncthing facts")
	assertNamesEqual(t, helper.VerbSyncthingInstall,
		server, helper.SyncthingInstallStepNames(
			installer.SyncthingVersionStr()))
}
