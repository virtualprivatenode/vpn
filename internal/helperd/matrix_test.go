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
	helper.VerbStageLNDCredentials: {
		paths.StateLNDTLSCert,
		paths.StateLNDMacaroon,
	},
	helper.VerbStageLNDMacaroon: {
		paths.StateLNDMacaroon,
	},
	helper.VerbUpgradeP2PToHybrid: {
		paths.StateLNDTLSCert,
	},
	helper.VerbSyncthingInstall: {
		paths.StateSyncthingAPIKey,
	},
	// Applied by the handler only for the lnd unit (start or
	// restart): LND can regenerate its TLS certificate during
	// startup, so the staged copy must follow.
	helper.VerbServiceAction: {
		paths.StateLNDTLSCert,
	},
	// Deliberately absent: rebuild-tor-config and
	// rebuild-ssh-config no longer touch any board fact —
	// onion addresses and the password-auth answer are
	// live-read (no copy exists to go stale).
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

// expectedBoardFiles restates the complete board INDEPENDENTLY
// of paths.go and matrix.go: the machine-cadence credential
// facts, and nothing else. Display facts (onion addresses, the
// Syncthing device ID, the SSH password-auth answer) are
// deliberately NOT here — they are live-read, with no copy.
var expectedBoardFiles = map[string]bool{
	paths.StateBitcoindRPCPass:      true,
	paths.StateLNDTLSCert:           true,
	paths.StateLNDMacaroon:          true,
	paths.StateSyncthingAPIKey:      true,
	paths.StateSyncthingWebPassword: true,
}

// Every fact in the matrix must have a stager, every verb in
// the matrix must exist on the verb menu, and every stager key
// must be a real board path.
func TestFreshnessMatrixIsClosed(t *testing.T) {
	boardFiles := expectedBoardFiles
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

// ── Freshness-declaration tests ──────────────────────────
//
// The rule: every board fact declares exactly one freshness
// story, and the live-read facts are pinned to menu verbs.
// Adding board fact number five without deciding how it stays
// fresh — or retiring a read verb without replacing the story
// it serves — fails here.

func TestEveryBoardFactDeclaresFreshness(t *testing.T) {
	for f := range expectedBoardFiles {
		story, ok := freshness[f]
		if !ok {
			t.Errorf("%s has no freshness declaration — how "+
				"does this fact stay fresh? Declare watched, "+
				"healed, live-read, or static-by-decision in "+
				"matrix.go", f)
			continue
		}
		switch story {
		case freshWatched, freshHealed, freshStatic:
		case freshLiveRead:
			t.Errorf("%s declares live-read but has a board "+
				"file — a live-read fact keeps no copy", f)
		default:
			t.Errorf("%s declares unknown story %q", f, story)
		}
	}
	for f := range freshness {
		if !expectedBoardFiles[f] {
			t.Errorf("freshness declares %s, which is not a "+
				"board file — update BOTH deliberately", f)
		}
	}
	for f := range stagers {
		if _, ok := freshness[f]; !ok {
			t.Errorf("stager registered for %s without a "+
				"freshness declaration", f)
		}
	}
}

// The watched and healed stories both depend on a stager
// existing (the path unit and the self-heal each end in a
// re-stage); a declaration without the mechanism is a lie.
func TestFreshnessStoriesHaveTheirMechanism(t *testing.T) {
	for f, story := range freshness {
		if story == freshWatched || story == freshHealed {
			if _, ok := stagers[f]; !ok {
				t.Errorf("%s declares %s but has no stager",
					f, story)
			}
		}
	}
}

// Live-read facts are served by verbs that must exist on the
// menu.
func TestLiveReadFactsServedByMenuVerbs(t *testing.T) {
	if len(liveReadFacts) == 0 {
		t.Fatal("no live-read facts declared")
	}
	for fact, verb := range liveReadFacts {
		if _, ok := verbs[verb]; !ok {
			t.Errorf("live-read fact %q names verb %s, which "+
				"is not on the menu", fact, verb)
		}
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

func TestUpgradeP2PToHybridStepNamesAligned(t *testing.T) {
	cfg := config.Default()
	cfg.P2PMode = "hybrid"
	server := stepNames(installer.UpgradeP2PToHybridSteps(
		cfg, "203.0.113.7"))
	// The verb reports two root-side completion steps after the installer
	// transition: staging only the regenerated TLS certificate, then
	// publishing authoritative desired state.
	server = append(server, "Restaging LND TLS certificate")
	server = append(server, "Publishing node configuration")
	assertNamesEqual(t, helper.VerbUpgradeP2PToHybrid,
		server, helper.UpgradeP2PToHybridStepNames())
}

func TestSyncthingInstallStepNamesAligned(t *testing.T) {
	cfg := config.Default()
	cfg.SyncthingEnabled = true
	steps, _, err := installer.SyncthingInstallSteps(cfg)
	if err != nil {
		t.Fatal(err)
	}
	server := stepNames(steps)
	// The verb reports one extra step: staging the new
	// component's facts.
	server = append(server, "Staging Syncthing facts")
	server = append(server, "Publishing node configuration")
	assertNamesEqual(t, helper.VerbSyncthingInstall,
		server, helper.SyncthingInstallStepNames(
			installer.SyncthingVersionStr()))
}
