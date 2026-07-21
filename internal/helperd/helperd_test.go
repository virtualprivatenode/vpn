// internal/helperd/helperd_test.go

package helperd

import (
	"os"
	"strconv"
	"testing"
)

// activationListener must refuse to run outside socket
// activation, and must refuse environment addressed to a
// different process — consuming another process's fds would be
// wrong in both directions.
func TestActivationListenerRefusals(t *testing.T) {
	unset := func() {
		os.Unsetenv("LISTEN_PID")
		os.Unsetenv("LISTEN_FDS")
		os.Unsetenv("LISTEN_FDNAMES")
	}
	unset()
	t.Cleanup(unset)

	if _, err := activationListener(); err == nil {
		t.Error("no LISTEN_PID: expected error")
	}

	os.Setenv("LISTEN_PID", "1") // not us
	os.Setenv("LISTEN_FDS", "1")
	if _, err := activationListener(); err == nil {
		t.Error("foreign LISTEN_PID: expected error")
	}
	// The refusal path must still have scrubbed the env so no
	// child of ours can mis-detect activation.
	if os.Getenv("LISTEN_PID") != "" {
		t.Error("LISTEN_PID not scrubbed")
	}

	os.Setenv("LISTEN_PID", strconv.Itoa(os.Getpid()))
	os.Setenv("LISTEN_FDS", "2") // wrong count
	if _, err := activationListener(); err == nil {
		t.Error("two fds: expected error")
	}

	os.Setenv("LISTEN_PID", strconv.Itoa(os.Getpid()))
	os.Setenv("LISTEN_FDS", "junk")
	if _, err := activationListener(); err == nil {
		t.Error("junk LISTEN_FDS: expected error")
	}
}

// Serve refuses to run unprivileged with a message that names
// the intended path (started by systemd), not a bare failure.
func TestServeRefusesUnprivileged(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	if err := Serve("0.7.0"); err == nil {
		t.Error("unprivileged Serve: expected error")
	}
}
