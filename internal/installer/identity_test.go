// internal/installer/identity_test.go

package installer

import (
	"strings"
	"testing"
)

const (
	testKeyA = "ssh-ed25519 QUFBQUFBQUFBQQ== alice@laptop"
	testKeyB = "ssh-rsa QkJCQkJCQkJCQg== bob@desktop"
	// The wild cloud-init decoy shape: an options prefix with a
	// forced command ahead of real key material.
	testDecoy = `no-port-forwarding,no-agent-forwarding,` +
		`command="echo 'Please login as the user \"admin\" ` +
		`rather than the user \"root\".'" ` +
		`ssh-ed25519 QUFBQUFBQUFBQQ== provisioning`
)

// ── classifyAuthorizedKeys ───────────────────────────────

func TestClassifyAuthorizedKeysPlain(t *testing.T) {
	keys, excluded := classifyAuthorizedKeys(
		testKeyA + "\n" + testKeyB + "\n")
	if len(keys) != 2 || excluded != 0 {
		t.Fatalf("got %d keys %d excluded, want 2/0",
			len(keys), excluded)
	}
	if keys[0].Comment != "alice@laptop" {
		t.Errorf("comment: got %q", keys[0].Comment)
	}
}

// The decoy trap (IA-3-E context, ruling vii): a forced-command
// provider line must be EXCLUDED and COUNTED — never copied
// verbatim, never silently dropped.
func TestClassifyAuthorizedKeysExcludesDecoy(t *testing.T) {
	keys, excluded := classifyAuthorizedKeys(
		testDecoy + "\n" + testKeyA + "\n")
	if len(keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(keys))
	}
	if keys[0].Comment != "alice@laptop" {
		t.Errorf("wrong key survived: %+v", keys[0])
	}
	if excluded != 1 {
		t.Errorf("excluded: got %d, want 1", excluded)
	}
}

func TestClassifyAuthorizedKeysSkipsNoise(t *testing.T) {
	keys, excluded := classifyAuthorizedKeys(
		"# a comment\n\n   \n" + testKeyA + "\n" +
			"complete garbage line\n")
	if len(keys) != 1 {
		t.Errorf("got %d keys, want 1", len(keys))
	}
	// Garbage without key material is noise, not an exclusion —
	// counting it would overstate what was on the box.
	if excluded != 0 {
		t.Errorf("excluded: got %d, want 0", excluded)
	}
}

// ── lineCarriesKeyMaterial ───────────────────────────────

func TestLineCarriesKeyMaterial(t *testing.T) {
	if !lineCarriesKeyMaterial(testDecoy) {
		t.Error("decoy line: got false, want true")
	}
	if lineCarriesKeyMaterial("complete garbage line") {
		t.Error("garbage line: got true, want false")
	}
	// A bare valid key line starts WITH the type (index 0) —
	// this helper only classifies unparseable lines, which by
	// construction have a prefix before the type.
	if lineCarriesKeyMaterial(testKeyA) {
		t.Error("bare key line: got true, want false")
	}
}

// ── DedupeKeys ───────────────────────────────────────────

func TestDedupeKeys(t *testing.T) {
	a, _ := ParseSSHKey(testKeyA)
	b, _ := ParseSSHKey(testKeyB)
	sources := []KeySource{
		{User: "root", Keys: []SSHKeyInfo{a, b}},
		{User: "debian", Keys: []SSHKeyInfo{a}}, // duplicate
	}
	out := DedupeKeys(sources)
	if len(out) != 2 {
		t.Fatalf("got %d keys, want 2", len(out))
	}
	if out[0].Fingerprint != a.Fingerprint ||
		out[1].Fingerprint != b.Fingerprint {
		t.Error("order not first-seen")
	}
}

// ── SortKeySources ───────────────────────────────────────

func TestSortKeySourcesRootFirst(t *testing.T) {
	in := []KeySource{
		{User: "debian"}, {User: "admin"}, {User: "root"},
	}
	out := SortKeySources(in)
	if out[0].User != "root" || out[1].User != "admin" ||
		out[2].User != "debian" {
		t.Errorf("order: got %v", []string{
			out[0].User, out[1].User, out[2].User})
	}
	// Input untouched.
	if in[0].User != "debian" {
		t.Error("input mutated")
	}
}

// ── generateAdminPassword ────────────────────────────────

func TestGenerateAdminPassword(t *testing.T) {
	pw, err := generateAdminPassword()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pw) != 25 {
		t.Errorf("length: got %d, want 25", len(pw))
	}
	for _, c := range pw {
		if !strings.ContainsRune(
			"ABCDEFGHIJKLMNOPQRSTUVWXYZ"+
				"abcdefghijklmnopqrstuvwxyz0123456789", c) {
			t.Errorf("character %q outside alphabet", c)
		}
	}
	// The generated fallback must satisfy the same policy the
	// interactive prompt enforces.
	if _, err := NewLoginPassword(pw); err != nil {
		t.Errorf("generated password fails validation: %v", err)
	}
	// Two draws must differ (sanity, not a randomness test).
	pw2, _ := generateAdminPassword()
	if pw == pw2 {
		t.Error("two generated passwords identical")
	}
}
