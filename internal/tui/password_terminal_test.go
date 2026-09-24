package tui

import (
	"errors"
	"io"
	"os/exec"
	"testing"
)

func TestPasswordTerminalPreservesChildOutcome(t *testing.T) {
	for _, tc := range []struct {
		name, command    string
		started, changed bool
	}{
		{"success", "exit 0", true, true},
		{"authentication failure", "exit 1", true, false},
		{"interruption", "kill -TERM $$", true, false},
		{"could not start", "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			terminal := newPasswordTerminal()
			// Exercise process lifecycle without invoking passwd or touching accounts.
			terminal.cmd = exec.Command("/bin/sh", "-c", tc.command)
			if tc.command == "" {
				terminal.cmd = exec.Command("/no/such/password-test-command")
			}
			terminal.SetStdout(io.Discard)
			terminal.SetStderr(io.Discard)
			err := terminal.Run()
			result := terminal.result(err)
			if result.started != tc.started || result.changed != tc.changed || (result.err == nil) != tc.changed {
				t.Fatalf("outcome=%+v", result)
			}
			if tc.started && terminal.cmd.ProcessState == nil {
				t.Fatal("child not reaped")
			}
			restoreErr := errors.New("terminal unavailable")
			result = terminal.result(restoreErr)
			if result.changed != tc.changed || !errors.Is(result.err, restoreErr) {
				t.Fatal("terminal failure lost outcome")
			}
		})
	}
	terminal := newPasswordTerminal()
	result := terminal.result(errors.New("could not release terminal"))
	if result.started || result.changed || result.err == nil {
		t.Fatal("reported a change before command execution")
	}
}
