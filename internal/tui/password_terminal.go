package tui

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
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
