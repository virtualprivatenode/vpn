// internal/config/rawfield_test.go

package config

import "testing"

// rawFieldPresent tells "absent" apart from "zero value" on
// omitempty fields — the migration seed rule's mechanism.
func TestRawFieldPresent(t *testing.T) {
	data := []byte(`{
  "install_complete": true,
  "ssh_password_auth_disabled": false,
  "network": "mainnet"
}`)
	if !rawFieldPresent(data, "ssh_password_auth_disabled") {
		t.Error("present false value reported absent")
	}
	if rawFieldPresent(data, "dbcache") {
		t.Error("absent key reported present")
	}
	if rawFieldPresent([]byte("not json"), "network") {
		t.Error("corrupt JSON reported present")
	}
	if rawFieldPresent(nil, "network") {
		t.Error("nil data reported present")
	}
}
