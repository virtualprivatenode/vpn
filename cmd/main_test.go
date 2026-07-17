// cmd/main_test.go

package main

import "testing"

// Explicit dispatch: the command line alone decides the mode.
func TestParseArgs(t *testing.T) {
	cmd, _, err := parseArgs(nil)
	if err != nil || cmd != cmdConsole {
		t.Errorf("no args: got (%v,%v), want console", cmd, err)
	}

	cmd, opts, err := parseArgs([]string{"install"})
	if err != nil || cmd != cmdInstall {
		t.Fatalf("install: got (%v,%v)", cmd, err)
	}
	if opts.Network != "" || opts.Unattended || opts.UntilBake {
		t.Errorf("install: unexpected opts %+v", opts)
	}

	cmd, opts, err = parseArgs(
		[]string{"install", "--testnet4", "--unattended"})
	if err != nil || cmd != cmdInstall {
		t.Fatalf("install flags: got (%v,%v)", cmd, err)
	}
	if opts.Network != "testnet4" || !opts.Unattended {
		t.Errorf("install flags: got %+v", opts)
	}

	if _, _, err := parseArgs(
		[]string{"install", "--bogus"}); err == nil {
		t.Error("unknown install flag accepted")
	}
	if _, _, err := parseArgs([]string{"bogus"}); err == nil {
		t.Error("unknown command accepted")
	}

	for _, v := range []string{"version", "--version", "-v"} {
		if cmd, _, err := parseArgs([]string{v}); err != nil ||
			cmd != cmdVersion {
			t.Errorf("%s: got (%v,%v), want version", v, cmd, err)
		}
	}
}
