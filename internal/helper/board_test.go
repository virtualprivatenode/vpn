// internal/helper/board_test.go

package helper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A missing board file must produce an error that tells the
// operator what to check — the fail-noisy contract: the reader
// never guesses, and the noise carries the diagnosis pointer.
func TestReadBoardMissingIsNoisy(t *testing.T) {
	_, err := ReadBoard(filepath.Join(t.TempDir(), "no-such-fact"))
	if err == nil {
		t.Fatal("missing board file: expected error")
	}
	msg := err.Error()
	for _, want := range []string{
		"no-such-fact", "journalctl -u vpn-helperd",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

func TestReadBoardString(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "fact")
	if err := os.WriteFile(p,
		[]byte("  value.onion\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBoardString(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "value.onion" {
		t.Errorf("got %q, want trimmed single value", got)
	}
}

// RemoveBoard treats a missing file as success (the fact is
// absent either way).
func TestRemoveBoardMissingOK(t *testing.T) {
	if err := RemoveBoard(
		filepath.Join(t.TempDir(), "gone")); err != nil {
		t.Errorf("remove of missing board file: %v", err)
	}
}
