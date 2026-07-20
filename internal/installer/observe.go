// internal/installer/observe.go

package installer

// Read-only sshd observation for the install path (ruling
// xvi(b)). One privileged query answers the two questions the
// installer must not guess at:
//
//   - which port(s) sshd actually listens on — the firewall
//     allow-rules derive from this. Guessing 22 was the one
//     real lockout hazard in the old script order: enabling
//     deny-all with only 22 open while sshd listens elsewhere
//     is a DELAYED silent lockout (the current connection
//     survives; the next one is refused).
//   - whether password authentication is effectively enabled —
//     the ruling-vii seed for cfg.SSHPasswordAuthDisabled (only
//     when the field is absent, never clobbering a carried-over
//     answer), the state-aware wizard copy, and the explicit
//     PasswordAuthentication directive in the install drop-in
//     (ruling xvi(a): explicit-from-observed).
//
// The observation runs in preflight and REFUSES the install on
// failure — a running-sshd box always parses, so failure means
// a broken environment, and there is deliberately no guessing
// path (xvi(b)). The SSH hardening step re-observes seconds
// before its write; only THAT later observation may degrade
// (to directive omission, xvi(a)) because by then refusing
// would strand a half-installed box.
//
// The query simulates a connection as root — the admin user
// does not exist yet at observation time (the identity step
// creates it later in this same install), and the question
// asked here is the box-global one ("is password auth on,
// today, before we changed anything"), not the per-user
// question the TUI's EffectiveSSHPasswordAuth answers after
// install. Same [LIVE]-verified `sshd -T -C` invocation shape
// as commit 2 (openssh-server 1:10.0p1-7+deb13u4).

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/system"
)

// SSHObservation is the read-only snapshot of the running
// sshd's effective configuration taken before any mutation.
type SSHObservation struct {
	// Ports are the listening ports, deduplicated, sorted.
	Ports []int
	// PasswordAuth reports whether password authentication is
	// effectively enabled.
	PasswordAuth bool
}

// ObserveSSHState queries the running sshd's effective
// configuration. Callers on the preflight path treat an error
// as refuse-to-install; the SSH step treats it as
// omit-directive-and-warn (ruling xvi(a)/(b) asymmetry).
func ObserveSSHState() (SSHObservation, error) {
	out, err := system.SudoRunOutput("sshd", "-T",
		"-C", "user=root,host=localhost,addr=127.0.0.1")
	if err != nil {
		return SSHObservation{}, fmt.Errorf(
			"query effective sshd config: %w", err)
	}
	return parseSSHObservation(out)
}

// parseSSHObservation extracts ports and password-auth state
// from sshd -T output (one "keyword value..." pair per line,
// keywords lowercased by sshd). Pure — unit-tested.
//
// Ports are the UNION of every `port` line and every
// `listenaddress host:port` line: a ListenAddress with an
// explicit port makes sshd listen there even when it differs
// from the Port directive. The union direction is fail-safe
// for a firewall allow-list — allowing a port sshd does not
// listen on wastes a rule; missing one it does listen on is
// the lockout.
func parseSSHObservation(sshdOutput string) (SSHObservation, error) {
	portSet := map[int]bool{}
	pwSeen := false
	pw := false

	for _, line := range strings.Split(sshdOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "port":
			if p, err := strconv.Atoi(fields[1]); err == nil &&
				p > 0 && p < 65536 {
				portSet[p] = true
			}
		case "listenaddress":
			if p, ok := portFromListenAddress(fields[1]); ok {
				portSet[p] = true
			}
		case "passwordauthentication":
			// First match wins, like sshd itself.
			if !pwSeen {
				pw = strings.EqualFold(fields[1], "yes")
				pwSeen = true
			}
		}
	}

	if len(portSet) == 0 {
		return SSHObservation{}, errors.New(
			"no listening port found in sshd -T output")
	}
	if !pwSeen {
		return SSHObservation{}, errors.New(
			"passwordauthentication not present in sshd -T output")
	}

	ports := make([]int, 0, len(portSet))
	for p := range portSet {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return SSHObservation{Ports: ports, PasswordAuth: pw}, nil
}

// portFromListenAddress extracts the port from a
// ListenAddress value as printed by sshd -T: "0.0.0.0:22",
// "[::]:22", or an address with no port (no port to extract).
// Pure — unit-tested.
func portFromListenAddress(addr string) (int, bool) {
	i := strings.LastIndex(addr, ":")
	if i < 0 || i == len(addr)-1 {
		return 0, false
	}
	// Bare IPv6 without brackets has colons but no port
	// separator we can trust; only accept a numeric suffix
	// that follows either a bracket or a dotted/hostname
	// form ("]:" or a single-colon value).
	if strings.Count(addr, ":") > 1 && !strings.Contains(addr, "]:") {
		return 0, false
	}
	p, err := strconv.Atoi(addr[i+1:])
	if err != nil || p <= 0 || p >= 65536 {
		return 0, false
	}
	return p, true
}
