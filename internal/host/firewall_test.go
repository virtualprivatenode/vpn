package host

import (
	"reflect"
	"testing"
)

func withFirewallTestDeps(t *testing.T) {
	t.Helper()
	oldStatus, oldRun := readUFWStatusForFeature, runFirewallCommand
	t.Cleanup(func() { readUFWStatusForFeature, runFirewallCommand = oldStatus, oldRun })
}

func TestFeatureFirewallRefusesInactiveBeforeMutation(t *testing.T) {
	withFirewallTestDeps(t)
	readUFWStatusForFeature = func() (string, error) {
		return "Status: inactive\n", nil
	}
	mutated := false
	runFirewallCommand = func([]string) error {
		mutated = true
		return nil
	}
	if err := allowHybridP2PFirewallRules(); err == nil {
		t.Fatal("P2P rules accepted inactive UFW")
	}
	if mutated {
		t.Fatal("P2P rule path mutated inactive UFW")
	}
}

func TestHybridP2PFirewallAddsAndVerifiesOnlyOwnedRules(t *testing.T) {
	withFirewallTestDeps(t)
	statusCalls := 0
	readUFWStatusForFeature = func() (string, error) {
		statusCalls++
		if statusCalls == 1 {
			return "Status: active\n", nil
		}
		return "Status: active\n\n" +
			"9735/tcp                  ALLOW       Anywhere\n" +
			"8080/tcp                  ALLOW       Anywhere\n" +
			"22000/tcp                 ALLOW       Anywhere\n", nil
	}
	var got [][]string
	runFirewallCommand = func(args []string) error {
		got = append(got, append([]string(nil), args...))
		return nil
	}
	if err := allowHybridP2PFirewallRules(); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"ufw", "allow", "9735/tcp"},
		{"ufw", "allow", "8080/tcp"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands\n got: %#v\nwant: %#v", got, want)
	}
}

func TestSyncthingFirewallAddsAndVerifiesOnlyOwnedRule(t *testing.T) {
	withFirewallTestDeps(t)
	statusCalls := 0
	readUFWStatusForFeature = func() (string, error) {
		statusCalls++
		if statusCalls == 1 {
			return "Status: active\n", nil
		}
		return "Status: active\n\n" +
			"22000/tcp                 ALLOW       Anywhere\n", nil
	}
	var got [][]string
	runFirewallCommand = func(args []string) error {
		got = append(got, append([]string(nil), args...))
		return nil
	}
	if err := AllowSyncthingFirewallRule(); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"ufw", "allow", "22000/tcp"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFeatureFirewallFailsWhenLiveVerificationMissesRule(t *testing.T) {
	withFirewallTestDeps(t)
	readUFWStatusForFeature = func() (string, error) {
		return "Status: active\n", nil
	}
	runFirewallCommand = func([]string) error { return nil }
	if err := AllowSyncthingFirewallRule(); err == nil {
		t.Fatal("Syncthing rule reported success without live verification")
	}
}
