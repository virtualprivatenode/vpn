// internal/installer/sshkeys.go

package installer

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

// SSHKeyInfo holds parsed metadata for a single authorized key.
type SSHKeyInfo struct {
	Type        string // e.g. "ssh-ed25519", "ssh-rsa"
	Fingerprint string // "SHA256:..." base64-encoded hash
	Comment     string // trailing comment from the key line
	RawLine     string // original full line (for removal)
}

// Known SSH public key type prefixes.
var validKeyTypes = map[string]bool{
	"ssh-rsa":                            true,
	"ssh-ed25519":                        true,
	"ssh-dss":                            true,
	"ecdsa-sha2-nistp256":                true,
	"ecdsa-sha2-nistp384":                true,
	"ecdsa-sha2-nistp521":                true,
	"sk-ssh-ed25519@openssh.com":         true,
	"sk-ecdsa-sha2-nistp256@openssh.com": true,
}

// ParseSSHKey parses a single authorized_keys line into
// SSHKeyInfo. Lines with options prefixes (e.g. command="...")
// are not supported and will return an error.
func ParseSSHKey(line string) (SSHKeyInfo, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return SSHKeyInfo{}, errors.New("empty line")
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return SSHKeyInfo{},
			errors.New("invalid key: need type and data")
	}

	keyType := fields[0]
	if !validKeyTypes[keyType] {
		return SSHKeyInfo{},
			fmt.Errorf("unknown key type: %s", keyType)
	}

	keyData, err := base64.StdEncoding.DecodeString(
		fields[1])
	if err != nil {
		return SSHKeyInfo{},
			fmt.Errorf("invalid base64 key data: %w", err)
	}
	if len(keyData) == 0 {
		return SSHKeyInfo{},
			errors.New("empty key data")
	}

	hash := sha256.Sum256(keyData)
	fingerprint := "SHA256:" +
		base64.RawStdEncoding.EncodeToString(hash[:])

	var comment string
	if len(fields) >= 3 {
		comment = strings.Join(fields[2:], " ")
	}

	return SSHKeyInfo{
		Type:        keyType,
		Fingerprint: fingerprint,
		Comment:     comment,
		RawLine:     line,
	}, nil
}

// ValidateSSHKey checks whether line is a valid SSH public
// key. Returns nil on success.
func ValidateSSHKey(line string) error {
	_, err := ParseSSHKey(line)
	return err
}

// ListAuthorizedKeys reads and parses all keys from the
// admin user's authorized_keys file. Returns an empty
// slice (not an error) if the file does not exist.
//
// No privilege is involved anywhere in this file: the
// authorized_keys file belongs to the admin user, so the TUI
// (running as that user) reads and writes it directly, and
// the root-run installer can too. The one root-only piece of
// the key story — the effective password-auth answer that
// guards last-key removal — comes from
// EffectiveSSHPasswordAuth (sshd.go).
func ListAuthorizedKeys() ([]SSHKeyInfo, error) {
	data, err := os.ReadFile(paths.AuthorizedKeysFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"read authorized_keys: %w", err)
	}

	return parseAuthorizedKeys(string(data)), nil
}

// parseAuthorizedKeys splits file content into key entries,
// skipping blank lines, comments, and unparseable lines.
func parseAuthorizedKeys(content string) []SSHKeyInfo {
	var keys []SSHKeyInfo
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		info, err := ParseSSHKey(line)
		if err != nil {
			continue // skip unparseable lines
		}
		keys = append(keys, info)
	}
	return keys
}

// AppendAuthorizedKey validates and appends a public key
// to authorized_keys. Returns an error if the key is
// invalid or already present.
func AppendAuthorizedKey(line string) error {
	line = strings.TrimSpace(line)
	info, err := ParseSSHKey(line)
	if err != nil {
		return fmt.Errorf("invalid key: %w", err)
	}

	// Read existing keys to check for duplicates.
	existing, err := ListAuthorizedKeys()
	if err != nil {
		return err
	}
	for _, k := range existing {
		if k.Fingerprint == info.Fingerprint {
			return errors.New(
				"key already in authorized_keys")
		}
	}

	// Read raw file to preserve comments and formatting.
	var content string
	data, err := os.ReadFile(paths.AuthorizedKeysFile)
	if err == nil {
		content = string(data)
	}

	// Ensure trailing newline before appending.
	if content != "" &&
		!strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += line + "\n"

	return writeAuthorizedKeys([]byte(content))
}

// writeAuthorizedKeys writes the admin user's authorized_keys
// atomically: same-dir temp, chmod 0600, rename. When running
// as root (install-time), ownership is handed to the admin
// user afterwards; as the admin user the file is simply ours.
func writeAuthorizedKeys(content []byte) error {
	dir := filepath.Dir(paths.AuthorizedKeysFile)
	tmp, err := os.CreateTemp(dir, ".authorized_keys.tmp-")
	if err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	if err := os.Rename(tmpPath,
		paths.AuthorizedKeysFile); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	if os.Geteuid() == 0 {
		return system.SudoRun("chown",
			paths.AdminUser+":"+paths.AdminUser,
			paths.AuthorizedKeysFile)
	}
	return nil
}

// RemoveAuthorizedKey removes the key matching the given
// fingerprint from authorized_keys. Refuses to remove the
// last key when password authentication is not actually
// available — the invariant is "never leave the system with
// zero auth methods."
//
// Whether password auth is available is derived from sshd's
// EFFECTIVE configuration (EffectiveSSHPasswordAuth), never
// from a caller-supplied claim or this app's own config:
// those can diverge from reality (e.g. a provider's
// cloud-init drop-in disabled password auth before this app
// ever ran, while the app's config still says enabled). The
// query runs only when the last key is being removed, so
// ordinary removals cost no extra privileged call.
func RemoveAuthorizedKey(fingerprint string) error {
	data, err := os.ReadFile(paths.AuthorizedKeysFile)
	if err != nil {
		return fmt.Errorf(
			"read authorized_keys: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var kept []string
	found := false
	keyCount := 0

	// First pass: count valid keys and find the target.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" ||
			strings.HasPrefix(trimmed, "#") {
			continue
		}
		if info, err := ParseSSHKey(trimmed); err == nil {
			keyCount++
			if info.Fingerprint == fingerprint {
				found = true
			}
		}
	}

	if !found {
		return errors.New("key not found")
	}
	if keyCount <= 1 {
		pwAuthEnabled, err := EffectiveSSHPasswordAuth()
		if err != nil {
			return fmt.Errorf(
				"cannot verify password auth state "+
					"(%w) — refusing to remove the last "+
					"SSH key", err)
		}
		if !pwAuthEnabled {
			return errors.New(
				"cannot remove the last SSH key while " +
					"password authentication is disabled " +
					"in the effective sshd configuration " +
					"— enable password auth first")
		}
	}

	// Second pass: rebuild without the target line.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" &&
			!strings.HasPrefix(trimmed, "#") {
			if info, err := ParseSSHKey(trimmed); err == nil &&
				info.Fingerprint == fingerprint {
				continue
			}
		}
		kept = append(kept, line)
	}

	content := strings.Join(kept, "\n")

	return writeAuthorizedKeys([]byte(content))
}
