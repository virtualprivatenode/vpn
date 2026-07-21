// internal/installer/preflight.go

package installer

// Install preflight (sprint principle 3): before the install engine
// mutates anything, assert the environmental bets this binary makes
// and refuse to proceed if the box is not one we can trust. All
// checks run and every failure is reported at once, so the operator
// fixes everything in one round trip.
//
// Placement: called at the top of RunInstall(), under root dispatch
// (`sudo vpn install`), before the wizard opens. A refused box is
// untouched — no user created, no config written, no service
// restarted.
//
// Re-homed under root dispatch (ruling xvi(c)):
//   - the commit-4 `sudo -n` scaffolding check is DELETED — install
//     no longer depends on any sudo rule (it IS root), and commit 7
//     deletes NOPASSWD entirely;
//   - the torsocks assertion RE-SEQUENCED out of preflight: the
//     engine installs tor/torsocks itself now, so asserting it here
//     would refuse every fresh box. The post-Tor-install,
//     pre-first-download assertion lives in the torgate step
//     (torgate.go LookPaths torsocks before gating; retires with
//     the Go-native Tor client, standalone queue #6);
//   - the sudoers scan reads /etc/sudoers.d directly — root needs
//     no `sudo find` workaround, and the failed-as-skipped
//     plumbing that coupled it to the deleted sudo check is gone;
//   - NEW: read-only sshd port/auth observation (observe.go),
//     REFUSE on failure — the firewall rules and the drop-in seed
//     derive from it, and there is deliberately no guessing path.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// PreflightResult is one check's outcome. Err == nil means pass.
type PreflightResult struct {
	Name string
	Err  error
}

// Check names double as report labels — short, present-tense facts.
const (
	checkNameDebian    = "Debian version is exactly 13"
	checkNameIOLogging = "sudo I/O logging is disabled"
	checkNameDpkg      = "package system is healthy (dpkg --audit)"
	checkNameUfw       = "ufw is installable"
	checkNameSSHState  = "ssh daemon state is observable"
)

// RunPreflight runs every check, prints a full report to stderr on
// any failure (and mirrors the failures to the log file, so the log
// trail never just stops), and returns a summary error that aborts
// the install before its first step. On success it returns the sshd
// observation for the wizard copy, the firewall rules, and the
// config seed — observed once, consumed everywhere (the SSH
// hardening step re-observes seconds before its own write).
func RunPreflight() (SSHObservation, error) {
	results, obs := runPreflightChecks()

	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
		}
	}
	// Warnings never refuse; they are printed and logged so the
	// operator can judge them.
	for _, w := range preflightWarnings() {
		fmt.Fprintf(os.Stderr, "\n  WARNING: %s\n", w)
		logger.Install("Preflight WARNING: %s", w)
	}

	if failed == 0 {
		logger.Install("Preflight passed (%d/%d checks)",
			len(results), len(results))
		return obs, nil
	}

	fmt.Fprint(os.Stderr, "\n"+FormatPreflightReport(results))
	for _, r := range results {
		if r.Err != nil {
			logger.Install("Preflight FAIL: %s: %v", r.Name, r.Err)
		}
	}
	return obs, fmt.Errorf(
		"preflight: %d of %d checks failed — nothing was changed",
		failed, len(results))
}

// runPreflightChecks executes the checks in order. All five run
// unconditionally — root dispatch removed the sudo dependency that
// used to force failed-as-skipped coupling between checks.
func runPreflightChecks() ([]PreflightResult, SSHObservation) {
	results := []PreflightResult{
		{checkNameDebian, checkDebian13()},
		{checkNameIOLogging, checkSudoersIOLogging()},
		{checkNameDpkg, checkDpkgAudit()},
		{checkNameUfw, checkUfwInstallable()},
	}
	obs, obsErr := ObserveSSHState()
	results = append(results,
		PreflightResult{checkNameSSHState, obsErr})
	return results, obs
}

// preflightWarnings collects conditions worth telling the
// operator about that are not grounds for refusal.
func preflightWarnings() []string {
	var ws []string
	// A world-readable /etc/sudoers.d discloses the box's sudo
	// POLICY to any local user (Debian 13 ships it 750). It
	// holds no credentials, and this node grants no sudo rules
	// anyway — a disclosure note, never a refusal.
	if fi, err := os.Stat(paths.SudoersDir); err == nil &&
		fi.Mode().Perm()&0o004 != 0 {
		ws = append(ws, fmt.Sprintf(
			"%s is world-readable (%o) — local users can read "+
				"sudo policy; Debian ships it 0750",
			paths.SudoersDir, fi.Mode().Perm()))
	}
	return ws
}

// ── Check 1: OS ──────────────────────────────────────────

// checkDebian13 asserts /etc/os-release says ID=debian AND
// VERSION_ID exactly 13 (ruling ix). Exactly, not at-least:
// every external interface this binary invokes (sshd -T -C flags,
// chpasswd, dpkg, apt) is [LIVE]-verified against the package
// versions frozen in Debian 13; a future Debian 14 ships new
// versions that may falsify those contracts (precedent: the
// commit-2 -G/-C flag rejection). Debian 14 support = re-verify
// the interface inventory, then widen this check in a release.
func checkDebian13() error {
	data, err := os.ReadFile(paths.OSRelease)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", paths.OSRelease, err)
	}
	return checkOSRelease(string(data))
}

// checkOSRelease parses os-release content. Pure function —
// unit-tested. Line-anchored prefix parsing, not substring search,
// so ID_LIKE=debian (Ubuntu and friends) can never match.
func checkOSRelease(content string) error {
	id, versionID := "", ""
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if v, ok := strings.CutPrefix(line, "ID="); ok {
			id = strings.Trim(v, `"`)
		}
		if v, ok := strings.CutPrefix(line, "VERSION_ID="); ok {
			versionID = strings.Trim(v, `"`)
		}
	}
	if id != "debian" {
		return fmt.Errorf(
			"this installer supports Debian only (found ID=%q)", id)
	}
	if versionID != "13" {
		return fmt.Errorf(
			"this release is verified for Debian 13 only "+
				"(found VERSION_ID=%q); newer Debian releases are "+
				"refused until their system commands are re-verified",
			versionID)
	}
	return nil
}

// ── Check 2: sudo I/O logging (IA-3-H) ───────────────────

// checkSudoersIOLogging asserts no sudoers file enables sudo's
// input/output recording. Scope: /etc/sudoers plus every file
// sudo's @includedir would parse in /etc/sudoers.d. Residual
// (documented, accepted): a nonstandard @includedir pointing
// elsewhere is not followed.
//
// This app itself no longer runs anything through sudo (the
// admin user has no sudo rights; privileged operations go
// through the root helper), so nothing of OURS can be captured
// by sudo I/O recording anymore. The check stays because a box
// where someone silently records terminal input is a box this
// installer should not set a node up on — the assertion is
// about the environment's hygiene, cheap to keep.
func checkSudoersIOLogging() error {
	files := []string{paths.SudoersFile}
	dropIns, err := listSudoersDropIns()
	if err != nil {
		return err
	}
	files = append(files, dropIns...)

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("cannot read %s: %w", f, err)
		}
		if directive, found :=
			sudoersEnablesIOLogging(string(data)); found {
			return fmt.Errorf(
				"%s enables sudo I/O recording (%s) — it would "+
					"write the admin password to disk when this app "+
					"sets it; remove that setting and try again",
				f, directive)
		}
	}
	return nil
}

// listSudoersDropIns enumerates the files sudo's @includedir would
// parse. Running as root, the directory is read directly — the
// commit-4 `sudo find` workaround (needed because /etc/sudoers.d
// is 750 root:root on Debian 13 [LIVE] and the check then ran
// unprivileged) is gone. A missing directory means nothing to
// scan; any other failure refuses. Subdirectories are skipped —
// sudo's includedir reads regular files only.
func listSudoersDropIns() ([]string, error) {
	entries, err := os.ReadDir(paths.SudoersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"cannot list %s: %w", paths.SudoersDir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !sudoersFileIncluded(e.Name()) {
			continue
		}
		files = append(files, paths.SudoersDir+"/"+e.Name())
	}
	return files, nil
}

// sudoersFileIncluded mirrors sudo's @includedir rule: files whose
// names contain a '.' or end in '~' are ignored by sudo, so a
// finding in them would be a false refusal. Pure — unit-tested.
func sudoersFileIncluded(name string) bool {
	return !strings.Contains(name, ".") &&
		!strings.HasSuffix(name, "~")
}

// sudoersEnablesIOLogging reports whether sudoers content enables
// I/O recording, via either mechanism:
//
//   - a Defaults option: `Defaults log_input` / `Defaults
//     log_output` (negation-aware: an odd number of leading '!'
//     disables; comments and @include/#include lines cannot enable
//     anything and are skipped)
//   - a command tag in a user spec: `LOG_INPUT:` / `LOG_OUTPUT:`
//     (uppercase, colon-suffixed; NOLOG_* tags disable and do not
//     match)
//
// Direction on ambiguity: over-refusal. Pure — unit-tested.
func sudoersEnablesIOLogging(content string) (string, bool) {
	// Join backslash line continuations first.
	content = strings.ReplaceAll(content, "\\\n", " ")

	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		for _, f := range strings.Fields(line) {
			if f == "LOG_INPUT:" || f == "LOG_OUTPUT:" {
				return f, true
			}
		}

		if !strings.HasPrefix(line, "Defaults") {
			continue
		}
		rest := line[len("Defaults"):]
		for _, tok := range strings.FieldsFunc(rest,
			func(r rune) bool {
				return r == ' ' || r == '\t' ||
					r == ',' || r == '='
			}) {
			neg := 0
			for strings.HasPrefix(tok, "!") {
				tok = tok[1:]
				neg++
			}
			if (tok == "log_input" || tok == "log_output") &&
				neg%2 == 0 {
				return tok, true
			}
		}
	}
	return "", false
}

// ── Check 3: dpkg ────────────────────────────────────────

// checkDpkgAudit asserts the package database has no packages stuck
// in broken states — a wedged dpkg would otherwise kill the install
// midway through the engine's own apt operations. Read-only; no
// lock taken.
func checkDpkgAudit() error {
	out, err := system.RunContext(60*time.Second, "dpkg", "--audit")
	return dpkgAuditVerdict(out, err)
}

// dpkgAuditVerdict: clean means rc 0 AND empty output — no bet on
// dpkg's exit-code behavior across versions (principle 2). Pure —
// unit-tested.
func dpkgAuditVerdict(output string, err error) error {
	if err != nil {
		return fmt.Errorf("dpkg --audit failed: %v", err)
	}
	if s := strings.TrimSpace(output); s != "" {
		first := strings.SplitN(s, "\n", 2)[0]
		return fmt.Errorf(
			"dpkg reports packages in a broken state (%s ...) — "+
				"fix with: dpkg --configure -a && "+
				"apt-get -f install", first)
	}
	return nil
}

// ── Check 4: ufw ─────────────────────────────────────────

// checkUfwInstallable asserts apt's on-disk package index can
// deliver ufw when the engine's base-package step apt-installs it.
// Offline; installs nothing; passes instantly when ufw is already
// present (re-runs, migrated boxes).
//
// LOAD-BEARING since the script absorption (commit 6): nothing
// pre-proves apt anymore — the binary's own base-package step is
// the first apt operation run for us on this box, and the firewall
// step immediately after it depends on ufw having arrived.
func checkUfwInstallable() error {
	if _, err := exec.LookPath("ufw"); err == nil {
		return nil
	}
	out, err := runAptCachePolicy("ufw", 30*time.Second)
	if err != nil {
		return fmt.Errorf("apt-cache policy ufw failed: %v", err)
	}
	return ufwCandidateVerdict(out)
}

// runAptCachePolicy runs `apt-cache policy <pkg>` with LC_ALL=C so
// the Candidate line is stable across locales. Local exec (not the
// system wrappers) purely for the env override.
func runAptCachePolicy(
	pkg string, timeout time.Duration,
) (string, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "apt-cache", "policy", pkg)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("apt-cache policy %s: %w", pkg, err)
	}
	return string(out), nil
}

// ufwCandidateVerdict parses apt-cache policy output: installable
// means a Candidate line with a real version. Pure — unit-tested.
func ufwCandidateVerdict(output string) error {
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if v, ok := strings.CutPrefix(line, "Candidate:"); ok {
			v = strings.TrimSpace(v)
			if v != "" && v != "(none)" {
				return nil
			}
			return errors.New(
				"apt has no installable ufw candidate — package " +
					"lists may be broken; run: apt-get update")
		}
	}
	return errors.New(
		"no Candidate line in apt-cache policy output — apt " +
			"package lists may be missing; run: apt-get update")
}

// ── Report formatting ────────────────────────────────────

// FormatPreflightReport renders the full check list with every
// failure's reason. Pure — unit-tested.
func FormatPreflightReport(results []PreflightResult) string {
	var b strings.Builder
	b.WriteString("  Preflight — checking the environment " +
		"before changing anything:\n\n")
	failed := 0
	for _, r := range results {
		if r.Err == nil {
			fmt.Fprintf(&b, "    [ OK ] %s\n", r.Name)
			continue
		}
		failed++
		fmt.Fprintf(&b, "    [FAIL] %s\n", r.Name)
		for _, line := range wrapText(r.Err.Error(), 56) {
			fmt.Fprintf(&b, "           %s\n", line)
		}
	}
	if failed > 0 {
		b.WriteString("\n  Refusing to install. Nothing on this " +
			"system has been changed.\n")
	}
	return b.String()
}

// wrapText word-wraps s to width columns. Words longer than width
// get their own line, unbroken. Pure — unit-tested.
func wrapText(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) <= width {
			cur += " " + w
			continue
		}
		lines = append(lines, cur)
		cur = w
	}
	return append(lines, cur)
}
