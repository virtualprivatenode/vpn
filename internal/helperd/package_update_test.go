package helperd

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestPackageUpdateDispatchRejectsParamsAndPreservesFailure(t *testing.T) {
	previous := updatePackages
	t.Cleanup(func() { updatePackages = previous })
	failure := errors.New("injected package inconsistency")
	calls := 0
	updatePackages = func(func(int)) error {
		calls++
		return failure
	}
	handler := verbs["package-update"].handler
	ctx := &verbCtx{}
	for _, params := range []string{`{"command":"anything"}`, `[]`, `1`} {
		if _, err := handler(ctx, json.RawMessage(params)); err == nil || calls != 0 {
			t.Fatalf("untrusted parameters reached host: %s, %v", params, err)
		}
	}
	if _, err := handler(ctx, nil); !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("package failure lost: %v, calls %d", err, calls)
	}
}
