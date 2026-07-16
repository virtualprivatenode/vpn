// internal/installer/ledger_test.go

package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLedgerValid(t *testing.T) {
	data := []byte(`{
  "schema": 1,
  "steps": {
    "tor.install": {
      "completed_at": "2026-07-16T14:02:11Z",
      "version": "0.6.3"
    }
  }
}`)
	l, err := parseLedger(data)
	if err != nil {
		t.Fatalf("parseLedger: %v", err)
	}
	if !l.done("tor.install") {
		t.Error("tor.install should be done")
	}
	if l.done("tor.gate") {
		t.Error("tor.gate should not be done")
	}
	e := l.Steps["tor.install"]
	if e.Version != "0.6.3" {
		t.Errorf("version = %q, want 0.6.3", e.Version)
	}
}

func TestParseLedgerCorrupt(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"garbage", "not json at all"},
		{"truncated", `{"schema":1,"steps":{`},
		{"wrong shape", `{"schema":"one"}`},
		{"empty", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseLedger([]byte(c.data)); err == nil {
				t.Errorf("parseLedger(%q) should error", c.data)
			}
		})
	}
}

// A ledger written by a NEWER binary has unknown semantics —
// it must be rejected (caller then treats it as empty; the
// fail-safe cost is a full re-run, never a wrong skip).
func TestParseLedgerFutureSchema(t *testing.T) {
	if _, err := parseLedger(
		[]byte(`{"schema": 2, "steps": {}}`)); err == nil {
		t.Error("future schema should be rejected")
	}
}

func TestParseLedgerNilStepsMap(t *testing.T) {
	l, err := parseLedger([]byte(`{"schema": 1}`))
	if err != nil {
		t.Fatalf("parseLedger: %v", err)
	}
	if l.Steps == nil {
		t.Fatal("Steps map must be non-nil after parse")
	}
	// Writable without panic.
	l.markDone("x", "test")
}

func TestLoadLedgerMissingFile(t *testing.T) {
	l := loadLedger(filepath.Join(t.TempDir(), "absent.json"))
	if l == nil {
		t.Fatal("loadLedger returned nil")
	}
	if len(l.Steps) != 0 {
		t.Errorf("missing file should load empty, got %d entries",
			len(l.Steps))
	}
}

func TestLoadLedgerCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(
		path, []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	l := loadLedger(path)
	if len(l.Steps) != 0 {
		t.Errorf("corrupt file should load empty, got %d entries",
			len(l.Steps))
	}
}

func TestLedgerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	l := newLedger()
	l.markDone("user.create", "0.6.3")
	l.markDone("tor.install", "0.6.3")
	if err := l.save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	got := loadLedger(path)
	for _, key := range []string{"user.create", "tor.install"} {
		if !got.done(key) {
			t.Errorf("%s lost in round trip", key)
		}
	}
	if got.done("btc.install") {
		t.Error("btc.install should not be done")
	}
	e := got.Steps["user.create"]
	if e.Version != "0.6.3" {
		t.Errorf("version = %q, want 0.6.3", e.Version)
	}
	if e.CompletedAt == "" {
		t.Error("completed_at empty")
	}
}

func TestLedgerSaveOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	l := newLedger()
	l.markDone("a", "1")
	if err := l.save(path); err != nil {
		t.Fatal(err)
	}
	l.markDone("b", "1")
	if err := l.save(path); err != nil {
		t.Fatal(err)
	}
	got := loadLedger(path)
	if !got.done("a") || !got.done("b") {
		t.Error("second save lost entries")
	}
	// No stray temp files left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected only the ledger in dir, found %d entries",
			len(entries))
	}
}
