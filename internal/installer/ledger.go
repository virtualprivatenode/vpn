// internal/installer/ledger.go

package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ripsline/virtual-private-node/internal/logger"
)

// ── Install ledger ───────────────────────────────────────
//
// The ledger is the durable record of which install steps have
// completed. It answers exactly one question — "did this step
// ever complete?" — and that answer gates resume skipping and
// the InstallComplete flag. It deliberately does NOT claim the
// box is currently correct: a recorded outcome can drift after
// it is written (a package removed by hand, a snapshot
// restored). Load-bearing drift surfaces as a later step
// failing; leaf drift may not surface during a run at all. The
// present-tense question belongs to a doctor-style health
// check, not to resume-time probes.
//
// An entry is written ONLY after a step's Fn returned nil, so
// process death at any instant — ctrl+c, connection drop,
// power — leaves either a truthful entry or no entry.
// Crash-safety comes from that write ordering, not from
// shutdown handling.
//
// Failure direction on load: a missing file is a fresh
// install; an unreadable or corrupt file is treated as EMPTY
// with a logged warning. The worst case of an empty ledger is
// redoing work; the worst case of a trusted-but-wrong ledger
// would be wrongly skipping it. Empty is the fail-safe side.
//
// The ledger lives in its own file (paths.InstallStateFile),
// not in config.json: a config load failure must not erase
// install history, and the file's ownership story is
// independent of config's (root-owned once install runs under
// root dispatch).

// ledgerSchema is the current on-disk schema version. A file
// carrying a HIGHER schema was written by a newer binary; its
// semantics are unknown to us, so it is treated as empty
// (fail-safe: full re-run, never a wrong skip).
const ledgerSchema = 1

type ledgerEntry struct {
	CompletedAt string `json:"completed_at"`
	Version     string `json:"version"`
}

type installLedger struct {
	Schema int                    `json:"schema"`
	Steps  map[string]ledgerEntry `json:"steps"`
}

func newLedger() *installLedger {
	return &installLedger{
		Schema: ledgerSchema,
		Steps:  map[string]ledgerEntry{},
	}
}

// parseLedger interprets raw ledger bytes. Pure function —
// separated from file I/O so every corrupt-input shape is
// unit-testable. Any defect in the bytes yields an error; the
// caller substitutes an empty ledger.
func parseLedger(data []byte) (*installLedger, error) {
	var l installLedger
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if l.Schema > ledgerSchema {
		return nil, fmt.Errorf(
			"schema %d is newer than supported %d",
			l.Schema, ledgerSchema)
	}
	if l.Steps == nil {
		l.Steps = map[string]ledgerEntry{}
	}
	l.Schema = ledgerSchema
	return &l, nil
}

// loadLedger reads the ledger at path. It never fails: missing
// means fresh install, unreadable/corrupt means empty with a
// logged warning (see the failure-direction note above).
func loadLedger(path string) *installLedger {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Install(
				"WARNING: install ledger unreadable, "+
					"treating as empty (full run): %v", err)
		}
		return newLedger()
	}
	l, err := parseLedger(data)
	if err != nil {
		logger.Install(
			"WARNING: install ledger corrupt, "+
				"treating as empty (full run): %v", err)
		return newLedger()
	}
	return l
}

func (l *installLedger) done(key string) bool {
	_, ok := l.Steps[key]
	return ok
}

func (l *installLedger) markDone(key, version string) {
	l.Steps[key] = ledgerEntry{
		CompletedAt: time.Now().UTC().Format(time.RFC3339),
		Version:     version,
	}
}

// save writes the ledger atomically: same-dir temp file, chmod,
// sync, rename — the config.Save pattern, so the file is never
// partially written.
func (l *installLedger) save(path string) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".install-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp ledger: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	tmp.Close()

	return os.Rename(tmpPath, path)
}
