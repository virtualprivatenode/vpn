package tui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func openPasswordTestTerminal(t *testing.T) *os.File {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { master.Close() })
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { slave.Close() })
	return slave
}

func TestPasswordTerminalRestoresTTYState(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		changed       bool
	}{
		{"success", "stty -echo -icanon && exit 0", true},
		{"failure", "stty -echo -icanon && exit 1", false},
		{"killed", "stty -echo -icanon && kill -KILL $$", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tty := openPasswordTestTerminal(t)
			before, err := term.GetState(int(tty.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			terminal := newPasswordTerminal()
			terminal.cmd = exec.Command("/bin/sh", "-c", tc.command)
			terminal.SetStdin(tty)
			terminal.SetStdout(io.Discard)
			terminal.SetStderr(io.Discard)
			err = terminal.Run()
			result := terminal.result(err)
			if !result.started || result.changed != tc.changed || (result.err == nil) != tc.changed {
				t.Fatalf("outcome=%+v", result)
			}
			if terminal.cmd.ProcessState == nil {
				t.Fatal("child not reaped")
			}
			after, err := term.GetState(int(tty.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("child left terminal settings changed")
			}
		})
	}
}

// Close the parent's tty after the child starts, leaving its inherited fd valid.
type passwordTTYCloser struct {
	output bytes.Buffer
	tty    *os.File
}

func (w *passwordTTYCloser) Write(p []byte) (int, error) {
	n, err := w.output.Write(p)
	if w.tty != nil && strings.Contains(w.output.String(), "child-complete") {
		closeErr := w.tty.Close()
		w.tty = nil
		err = errors.Join(err, closeErr)
	}
	return n, err
}

func TestPasswordTerminalRestoreFailurePreservesSuccess(t *testing.T) {
	tty := openPasswordTestTerminal(t)
	terminal := newPasswordTerminal()
	terminal.cmd = exec.Command("/bin/sh", "-c", "stty -echo && printf child-complete")
	terminal.SetStdin(tty)
	terminal.SetStdout(&passwordTTYCloser{tty: tty})
	terminal.SetStderr(io.Discard)
	err := terminal.Run()
	result := terminal.result(err)
	if !result.started || !result.changed || !errors.Is(result.err, unix.EBADF) {
		t.Fatalf("lost confirmed child success or terminal restoration error: %+v", result)
	}
}

func TestPasswordTerminalStateFailureDoesNotStartChild(t *testing.T) {
	tty := openPasswordTestTerminal(t)
	if err := tty.Close(); err != nil {
		t.Fatal(err)
	}
	terminal := newPasswordTerminal()
	terminal.cmd = exec.Command("/bin/sh", "-c", "exit 0")
	terminal.SetStdin(tty)
	terminal.SetStdout(io.Discard)
	err := terminal.Run()
	result := terminal.result(err)
	if result.started || result.changed || result.err == nil || terminal.cmd.Process != nil {
		t.Fatalf("started child without saving terminal state: %+v", result)
	}
}
