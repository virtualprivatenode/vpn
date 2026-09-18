package tui

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Copy views use direct terminal I/O without shell commands or temporary files.
// Clear before display and on every return, including errors. Terminal emulators
// ultimately control scrollback retention; clearing is not secure erasure.
type copyTextDisplay struct {
	text string
	in   io.Reader
	out  io.Writer
}

const clearCopyDisplay = "\033[2J\033[3J\033[H"

func (d *copyTextDisplay) Run() (err error) {
	defer func() {
		_, clearErr := io.WriteString(d.out, clearCopyDisplay)
		if err == nil {
			err = clearErr
		}
		d.text = ""
	}()
	if _, err = fmt.Fprintf(d.out, "%s\n%s\n\n  Press Enter...", clearCopyDisplay, d.text); err != nil {
		return err
	}
	_, err = bufio.NewReader(d.in).ReadString('\n')
	return err
}

func (d *copyTextDisplay) SetStdin(in io.Reader)   { d.in = in }
func (d *copyTextDisplay) SetStdout(out io.Writer) { d.out = out }
func (d *copyTextDisplay) SetStderr(io.Writer)     {}

func showCopyTextCmd(text string, done func(error) tea.Msg) tea.Cmd {
	return tea.Exec(&copyTextDisplay{text: text}, done)
}

func nodeURIText(uris []string) string {
	var b strings.Builder
	b.WriteString("  Node URIs\n  =========\n\n")
	clearnet, tor := classifyURIs(uris)
	for _, group := range []struct {
		label string
		uris  []string
	}{{"Clearnet", clearnet}, {"Tor", tor}} {
		if len(group.uris) == 0 {
			continue
		}
		fmt.Fprintf(&b, "  %s:\n", group.label)
		for _, uri := range group.uris {
			fmt.Fprintf(&b, "  %s\n", uri)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
