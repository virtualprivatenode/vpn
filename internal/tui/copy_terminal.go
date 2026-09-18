package tui

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Append Copy text to the normal terminal buffer. Bubble Tea restores its
// alternate screen on return; this display never requests page/history erasure.
type copyTextDisplay struct {
	text string
	in   io.Reader
	out  io.Writer
}

func (d *copyTextDisplay) Run() (err error) {
	defer func() {
		_, finishErr := io.WriteString(d.out, "\n")
		if err == nil {
			err = finishErr
		}
	}()
	if _, err = fmt.Fprintf(d.out, "\n%s\n\n  Press Enter...", d.text); err != nil {
		return err
	}
	_, err = bufio.NewReader(d.in).ReadString('\n')
	return err
}

func (d *copyTextDisplay) SetStdin(in io.Reader)   { d.in = in }
func (d *copyTextDisplay) SetStdout(out io.Writer) { d.out = out }
func (d *copyTextDisplay) SetStderr(io.Writer)     {}

func showCopyTextCmd(request *qrCopyDisplayRequest) tea.Cmd {
	return tea.Exec(&copyTextDisplay{text: request.text}, func(err error) tea.Msg {
		return qrCopyDisplayDoneMsg{request: request, err: err}
	})
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
