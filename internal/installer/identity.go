// internal/installer/identity.go

package installer

// The identity/access install step (ruling vii + the xvi
// refinements). Replaces the retired bootstrap script's silent
// key-guess cascade ($SUDO_USER → logname → who → /root) with an
// interactive step: enumerate EVERY candidate authorized_keys
// source on the box, show what was found — fingerprints and
// comments, with provider decoy lines recognized and excluded
// rather than copied verbatim — and let the operator confirm the
// copy or paste a key instead. Keys that already logged into this
// box are stronger evidence than a fresh paste, so confirmation
// is by fingerprint, never by re-pasting (ruling xvi).
//
// The login password is PROMPTED and non-skippable: with password
// auth off it is the console-recovery credential, not a network
// credential — post-commit-7 the admin user is the box's only
// interactive identity, and no password + broken SSH would leave
// rescue mode as the only way in. Random generation survives only
// as the --unattended fallback (the image path; the first-boot
// wizard replaces it).
//
// This file owns enumeration (pure parts unit-tested) and the
// step's apply function; the wizard screens that collect the
// operator's decisions live in wizard.go.

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/logger"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// KeySource is one authorized_keys file found on the box.
type KeySource struct {
	User     string // owning login ("root", "debian", "ripsline", …)
	Path     string
	Keys     []SSHKeyInfo
	Excluded int // key-bearing lines excluded (options/decoy lines)
}

// EnumerateKeySources scans every candidate authorized_keys
// location: /root plus each directory under /home — except the
// admin user's own (that file is this step's DESTINATION; on a
// re-run it must not enumerate itself as a source). Unreadable or
// missing files simply contribute nothing: enumeration informs a
// decision the operator confirms on screen, so an empty result is
// visible, not silent.
func EnumerateKeySources() []KeySource {
	type candidate struct{ user, path string }
	cands := []candidate{
		{"root", "/root/.ssh/authorized_keys"},
	}
	if entries, err := os.ReadDir("/home"); err == nil {
		for _, e := range entries {
			if !e.IsDir() || e.Name() == paths.AdminUser {
				continue
			}
			cands = append(cands, candidate{
				e.Name(),
				filepath.Join("/home", e.Name(),
					".ssh", "authorized_keys"),
			})
		}
	}

	var sources []KeySource
	for _, c := range cands {
		data, err := os.ReadFile(c.path)
		if err != nil {
			continue
		}
		keys, excluded := classifyAuthorizedKeys(string(data))
		if len(keys) == 0 && excluded == 0 {
			continue
		}
		sources = append(sources, KeySource{
			User: c.user, Path: c.path,
			Keys: keys, Excluded: excluded,
		})
	}
	return sources
}

// classifyAuthorizedKeys splits authorized_keys content into
// parseable keys and EXCLUDED key-bearing lines. Pure —
// unit-tested.
//
// An excluded line is one that carries key material but does not
// parse as a bare "type base64 [comment]" line — i.e. it has an
// options prefix. The canonical wild instance is the cloud-init
// forced-command decoy (`no-port-forwarding,...,command="echo
// 'Please login as ...'" ssh-rsa AAAA...`): copying it verbatim
// would grant its key access under OUR user with the provider's
// message semantics stripped of context (the IA-3-E decoy trap).
// Counting exclusions — instead of dropping them silently — keeps
// the screen honest about what was on the box.
func classifyAuthorizedKeys(content string) ([]SSHKeyInfo, int) {
	var keys []SSHKeyInfo
	excluded := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		info, err := ParseSSHKey(line)
		if err == nil {
			keys = append(keys, info)
			continue
		}
		if lineCarriesKeyMaterial(line) {
			excluded++
		}
	}
	return keys, excluded
}

// lineCarriesKeyMaterial reports whether an unparseable
// authorized_keys line still contains an SSH key (an options
// prefix ahead of a known key type) as opposed to plain junk.
// Pure — unit-tested.
func lineCarriesKeyMaterial(line string) bool {
	for t := range validKeyTypes {
		idx := strings.Index(line, t+" ")
		if idx > 0 {
			return true
		}
	}
	return false
}

// DedupeKeys flattens sources into a fingerprint-unique key list,
// preserving first-seen order (root first, then /home in ReadDir
// order). Pure — unit-tested.
func DedupeKeys(sources []KeySource) []SSHKeyInfo {
	seen := map[string]bool{}
	var out []SSHKeyInfo
	for _, s := range sources {
		for _, k := range s.Keys {
			if seen[k.Fingerprint] {
				continue
			}
			seen[k.Fingerprint] = true
			out = append(out, k)
		}
	}
	return out
}

// ── Applying the decisions ───────────────────────────────

// InstallDecisions carries the wizard's answers into the engine
// steps. Collected before the steps run (interactively or by the
// unattended defaults) and read by step closures at execution.
type InstallDecisions struct {
	// Keys are written to the admin user's authorized_keys.
	// Empty means password-only access (operator's explicit
	// choice, or nothing found under --unattended).
	Keys []SSHKeyInfo
	// Password is the admin login password (validated at
	// construction; 16-char minimum from commit 3).
	Password LoginPassword
	// GeneratedPassword is set only on the --unattended path
	// (ruling vii: random generation survives only there);
	// printed once at the end of the run, never logged.
	GeneratedPassword string
	// PasswordApplied is set by the identity step after
	// chpasswd succeeded. The generated password is printed
	// ONLY when this is set: on a resume that ledger-skips the
	// identity step (or an --until=bake run that filters it
	// out), this pass generated a password that was never
	// applied — printing it would hand the operator a
	// credential that does not work.
	PasswordApplied bool
	// DbCacheMB is the hardware-fit step's confirmed dbcache
	// (ruling viii).
	DbCacheMB int
	// Obs is the preflight sshd observation (wizard copy +
	// config seed; the SSH step re-observes before writing).
	Obs SSHObservation
}

// applyIdentityAccess is the identity.access step: create the
// admin user, grant NOPASSWD sudo (commit 7 deletes this — the
// TUI still runs on sudo until the root helper lands), write the
// confirmed keys, set the login password, and configure the
// SSH-login auto-launch. Idempotent — safe to re-run after an
// interrupt (adduser is guarded, writes overwrite, chpasswd
// resets to the same password).
func applyIdentityAccess(dec *InstallDecisions) error {
	if _, err := user.Lookup(paths.AdminUser); err != nil {
		if err := system.SudoRun("adduser",
			"--disabled-password",
			"--gecos", "Virtual Private Node",
			paths.AdminUser); err != nil {
			return fmt.Errorf("create admin user: %w", err)
		}
	}

	sudoers := paths.AdminUser + " ALL=(ALL) NOPASSWD:ALL\n"
	if err := system.SudoWriteFile(
		paths.AdminSudoers, []byte(sudoers), 0440); err != nil {
		return fmt.Errorf("write sudoers rule: %w", err)
	}

	if len(dec.Keys) > 0 {
		sshDir := paths.AdminHome + "/.ssh"
		if err := system.SudoRun(
			"mkdir", "-p", sshDir); err != nil {
			return fmt.Errorf("mkdir %s: %w", sshDir, err)
		}
		var b strings.Builder
		for _, k := range dec.Keys {
			b.WriteString(k.RawLine)
			b.WriteString("\n")
		}
		if err := system.SudoWriteFile(paths.AuthorizedKeysFile,
			[]byte(b.String()), 0600); err != nil {
			return fmt.Errorf("write authorized_keys: %w", err)
		}
		owner := paths.AdminUser + ":" + paths.AdminUser
		if err := system.SudoRun(
			"chown", "-R", owner, sshDir); err != nil {
			return err
		}
		if err := system.SudoRun(
			"chmod", "700", sshDir); err != nil {
			return err
		}
		logger.Install("admin access: %d key(s) written for %s",
			len(dec.Keys), paths.AdminUser)
	} else {
		logger.Install(
			"admin access: no SSH keys configured (password login)")
	}

	if err := SetUserPassword(
		paths.AdminUser, dec.Password); err != nil {
		return fmt.Errorf("set admin password: %w", err)
	}
	dec.PasswordApplied = true

	return writeAdminAutoLaunch()
}

// writeAdminAutoLaunch installs the .bash_profile that opens the
// TUI on SSH login — the retired bootstrap's auto-launch,
// verbatim in behavior: only on SSH sessions with a tty, and
// .bashrc (where shellenv puts the cli wrappers) is sourced
// after.
func writeAdminAutoLaunch() error {
	profile := `# Virtual Private Node — auto-launch
if [ -n "$SSH_CONNECTION" ] && [ -t 0 ]; then
    ` + paths.BinaryPath + `
fi

# Source .bashrc after the TUI exits (cli wrappers live there)
[ -f ~/.bashrc ] && source ~/.bashrc
`
	if err := system.SudoWriteFile(paths.AdminBashProfile,
		[]byte(profile), 0644); err != nil {
		return fmt.Errorf("write .bash_profile: %w", err)
	}
	return system.SudoRun("chown",
		paths.AdminUser+":"+paths.AdminUser,
		paths.AdminBashProfile)
}

// installSelfBinary is the binary.install step: place the running
// executable at paths.BinaryPath if it is not already running
// from there — the auto-launch profile and the handoff both
// depend on that path existing. A source build run as
// `sudo ./vpn install` gets installed; a binary already at the
// canonical path (the MIGRATION.md flow installs it in step 1)
// no-ops.
func installSelfBinary() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate own binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	if self == paths.BinaryPath {
		return nil
	}
	if err := system.SudoRun("install", "-m", "755",
		self, paths.BinaryPath); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}
	logger.Install("binary installed to %s (from %s)",
		paths.BinaryPath, self)
	return nil
}

// generateAdminPassword returns a random alphanumeric password
// for the --unattended fallback (ruling vii). 25 characters from
// a 62-symbol alphabet (~148 bits) — same shape the retired
// bootstrap printed. Uses rejection sampling for uniformity.
func generateAdminPassword() (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 25
	out := make([]byte, 0, length)
	buf := make([]byte, 64)
	for len(out) < length {
		if _, err := randRead(buf); err != nil {
			return "", fmt.Errorf("generate password: %w", err)
		}
		for _, b := range buf {
			// Reject bytes that would bias the modulus.
			if int(b) >= 248 { // 248 = 4*62
				continue
			}
			out = append(out, alphabet[int(b)%62])
			if len(out) == length {
				break
			}
		}
	}
	return string(out), nil
}

// SortKeySources orders sources root-first then by user name for
// stable display. Pure — unit-tested.
func SortKeySources(sources []KeySource) []KeySource {
	out := make([]KeySource, len(sources))
	copy(out, sources)
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].User == "root") != (out[j].User == "root") {
			return out[i].User == "root"
		}
		return out[i].User < out[j].User
	})
	return out
}
