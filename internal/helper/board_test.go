// internal/helper/board_test.go

package helper

import (
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
