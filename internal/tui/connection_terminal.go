package tui

import (
	"bufio"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
)

// The terminal owns this explicit credential display. No credential is placed
// in command arguments or a temporary file. Clear the display on every return;
// terminal emulators ultimately control scrollback retention.
type macaroonDisplay struct {
	text string
	in   io.Reader
	out  io.Writer
}

const clearCredentialDisplay = "\033[2J\033[3J\033[H"

func (d *macaroonDisplay) Run() (err error) {
	defer func() {
		_, clearErr := io.WriteString(d.out, clearCredentialDisplay)
		if err == nil {
			err = clearErr
		}
		d.text = ""
	}()
	if _, err = fmt.Fprintf(d.out, "%s\n%s\n\n  Press Enter...", clearCredentialDisplay, d.text); err != nil {
		return err
	}
	_, err = bufio.NewReader(d.in).ReadString('\n')
	return err
}
func (d *macaroonDisplay) SetStdin(in io.Reader)   { d.in = in }
func (d *macaroonDisplay) SetStdout(out io.Writer) { d.out = out }
func (d *macaroonDisplay) SetStderr(io.Writer)     {}

type connectionDisplayDoneMsg struct {
	request *connectionInfoRequest
	err     error
}

func showConnectionMacaroonCmd(request *connectionInfoRequest, text string) tea.Cmd {
	return tea.Exec(&macaroonDisplay{text: text}, func(err error) tea.Msg { return connectionDisplayDoneMsg{request: request, err: err} })
}
