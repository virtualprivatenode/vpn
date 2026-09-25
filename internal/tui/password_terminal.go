package tui

import (
	"errors"
	"fmt"
	"io"
	"os/exec"

	"golang.org/x/term"
)

type passwordExecution struct {
	started, changed bool
	err              error
}

// Store the child outcome separately: a later terminal restoration failure
// must not turn a confirmed password change into an unconfirmed one.
type passwordTerminal struct {
	cmd       *exec.Cmd
	execution passwordExecution
}

func newPasswordTerminal() *passwordTerminal {
	return &passwordTerminal{cmd: exec.Command("/usr/bin/passwd")}
}
func (t *passwordTerminal) SetStdin(r io.Reader)  { t.cmd.Stdin = r }
func (t *passwordTerminal) SetStdout(w io.Writer) { t.cmd.Stdout = w }
func (t *passwordTerminal) SetStderr(w io.Writer) { t.cmd.Stderr = w }
func (t *passwordTerminal) Run() (err error) {
	defer func() { t.execution.err = err }()
	if input, ok := t.cmd.Stdin.(interface{ Fd() uintptr }); ok {
		fd := int(input.Fd())
		state, stateErr := term.GetState(fd)
		if stateErr != nil {
			return fmt.Errorf("save password terminal state: %w", stateErr)
		}
		// passwd may exit without restoring echo. Restore before tea.Exec
		// reacquires the terminal and saves its next restoration state.
		defer func() {
			if restoreErr := term.Restore(fd, state); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("restore password terminal state: %w", restoreErr))
			}
		}()
	}
	out := t.cmd.Stdout
	if out == nil {
		out = io.Discard
	}
	if _, err := fmt.Fprint(out, "To cancel at a password prompt: Ctrl+U, then Ctrl+D.\nCtrl+U clears your input; Ctrl+D ends input.\n\n"); err != nil {
		return fmt.Errorf("show password instructions: %w", err)
	}
	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("start passwd: %w", err)
	}
	t.execution.started = true
	if err := t.cmd.Wait(); err != nil {
		return fmt.Errorf("passwd ended without confirmed success: %w", err)
	}
	t.execution.changed = true
	return nil
}
func (t *passwordTerminal) result(terminalErr error) passwordExecution {
	result := t.execution
	if terminalErr != nil && !errors.Is(terminalErr, result.err) {
		result.err = errors.Join(result.err, fmt.Errorf("terminal session: %w", terminalErr))
	}
	return result
}
