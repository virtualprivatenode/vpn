// internal/installer/identity.go

package installer

// Installer owns initial key-source inventory, confirmed decisions and generated
// password delivery. Host owns account, authorized_keys and login-shell writes.

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/host"
	"github.com/virtualprivatenode/vpn/internal/loginpassword"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/sshkeys"
)

// KeySource is one authorized_keys file found on the box.
type KeySource struct {
	User     string // owning login ("root", "debian", "ripsline", …)
	Path     string
	Keys     []sshkeys.Key
	Excluded int // malformed or unsupported key-bearing lines, including options
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
// parse as a supported bare "type base64 [comment]" line, including
// malformed keys and lines with options. The cloud-init
// forced-command decoy is a common example (`no-port-forwarding,...,command="echo
// 'Please login as ...'" ssh-rsa AAAA...`): copying it verbatim
// would grant its key access under OUR user with the provider's
// message semantics stripped of context (the IA-3-E decoy trap).
// Counting exclusions — instead of dropping them silently — keeps
// the screen honest about what was on the box.
func classifyAuthorizedKeys(content string) ([]sshkeys.Key, int) {
	var keys []sshkeys.Key
	excluded := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		info, err := sshkeys.Parse(line)
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
// authorized_keys line names a recognized key type, including malformed
// bare keys and option-prefixed keys, as opposed to plain junk.
// Pure — unit-tested.
func lineCarriesKeyMaterial(line string) bool {
	for _, field := range strings.Fields(line) {
		if sshkeys.RecognizedType(field) {
			return true
		}
	}
	return false
}

// DedupeKeys flattens sources into a fingerprint-unique key list,
// preserving first-seen order (root first, then /home in ReadDir
// order). Pure — unit-tested.
func DedupeKeys(sources []KeySource) []sshkeys.Key {
	seen := map[string]bool{}
	var out []sshkeys.Key
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
	Keys []sshkeys.Key
	// Password is validated using the initial login-password policy.
	Password loginpassword.Password
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
}

// applyIdentityAccess coordinates access provisioning and password delivery.
// Record successful password application before the marker and shell setup so
// a later failure cannot obscure the credential that was already applied.
func applyIdentityAccess(dec *InstallDecisions) error {
	return provisionIdentityAccess(dec, identityAccessOps{
		create: host.CreateOperatorAccess, password: host.SetLoginPassword,
		markPending: markPasswordPending, sudo: host.ConfigureOperatorSudo,
		autoLaunch: host.ConfigureOperatorAutoLaunch,
	})
}

// Keep password application and its delivery marker ahead of sudo and shell
// setup. A failed password operation must never reach the sudo grant.
type identityAccessOps struct {
	create                        func([]sshkeys.Key) error
	password                      func(loginpassword.Password) error
	markPending, sudo, autoLaunch func() error
}

func provisionIdentityAccess(dec *InstallDecisions, ops identityAccessOps) error {
	if err := ops.create(dec.Keys); err != nil {
		return err
	}
	if err := ops.password(dec.Password); err != nil {
		return fmt.Errorf("set owner password: %w", err)
	}
	dec.PasswordApplied = true
	if dec.GeneratedPassword != "" {
		if err := ops.markPending(); err != nil {
			return fmt.Errorf("record password-pending marker: %w", err)
		}
	}
	if err := ops.sudo(); err != nil {
		return err
	}
	return ops.autoLaunch()
}

// generateAdminPassword returns a random alphanumeric password
// for the --unattended fallback: 25 characters from a 62-symbol
// alphabet (~148 bits). Uses rejection sampling for uniformity.
func generateAdminPassword() string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 25
	out := make([]byte, 0, length)
	buf := make([]byte, 64)
	for len(out) < length {
		rand.Read(buf)
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
	return string(out)
}

// SortKeySources orders sources root-first then by user name for
// stable display.
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
