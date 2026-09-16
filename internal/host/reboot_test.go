package host

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
)

func TestRebootCommandAcceptanceFailureAndBound(t *testing.T) {
	for _, mode := range []string{"success", "fail", "block"} {
		t.Run(mode, func(t *testing.T) {
			var child *exec.Cmd
			timeout := 5 * time.Second
			if mode == "block" {
				timeout = 200 * time.Millisecond
			}
			err := requestReboot(timeout, func(ctx context.Context, name string, args ...string) *exec.Cmd {
				if name != "systemctl" || !slices.Equal(args, []string{"reboot"}) {
					t.Fatalf("unexpected reboot command: %s %v", name, args)
				}
				// Reuse the safe child fixture; never invoke the host's systemctl.
				child = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestServiceControlProcess$")
				child.Env = append(os.Environ(), "VPN_SERVICE_CONTROL_TEST="+mode)
				return child
			})
			if (err == nil) != (mode == "success") {
				t.Fatalf("request: %v", err)
			}
			if child.ProcessState == nil {
				t.Fatal("command was not reaped")
			}
			if mode == "block" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("deadline lost: %v", err)
			}
		})
	}
}
