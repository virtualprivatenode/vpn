package tui

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWalletTerminalCreationAndAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name, input  string
		exit         int
		created, ack bool
	}{
		{"acknowledged", "wrong\nI SAVED MY SEED\n", 0, true, true},
		{"EOF after creation", "", 0, true, false},
		{"lncli failure", "I SAVED MY SEED\n", 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			terminal := newWalletTerminal("signet")

			command := "exit 0"
			if tc.exit != 0 {
				command = "exit 1"
			}
			terminal.create = exec.CommandContext(ctx, "bash", "-c", command)
			terminal.acknowledge = exec.CommandContext(ctx, "bash", "-c", walletSeedAcknowledgement)
			path := filepath.Join(t.TempDir(), "input")
			if err := os.WriteFile(path, []byte(tc.input), 0600); err != nil {
				t.Fatal(err)
			}
			input, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			var output bytes.Buffer
			terminal.SetStdin(input)
			terminal.SetStdout(&output)
			terminal.SetStderr(&output)
			err = terminal.Run()
			if terminal.execution.Created != tc.created || terminal.execution.SeedAcknowledged != tc.ack || (err == nil) != tc.ack {
				t.Fatalf("result=%+v err=%v", terminal.execution, err)
			}
			if ctx.Err() != nil {
				t.Fatal("terminal wrapper hung")
			}
			if !tc.created && strings.Contains(output.String(), "Type I SAVED MY SEED") {
				t.Fatal("acknowledgement prompted after lncli failure")
			}
		})
	}
}
