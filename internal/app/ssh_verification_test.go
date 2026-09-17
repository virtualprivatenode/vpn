package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestSSHVerificationReplyAuthorityAndAddressIndependence(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantAddress string
		helperErr, addressErr   bool
		wantPending, wantError  bool
		wantAddressReads        int
	}{
		{name: "pending", body: `{"pending":true,"verified":false}`, wantPending: true, wantAddress: "203.0.113.7", wantAddressReads: 1},
		{name: "verified", body: `{"pending":false,"verified":true}`},
		{name: "already clear", body: `{"pending":false,"verified":false}`},
		{name: "address unavailable", body: `{"pending":true,"verified":false}`, addressErr: true, wantPending: true, wantAddressReads: 1},
		{name: "helper unavailable", helperErr: true, wantError: true},
		{name: "missing body", wantError: true},
		{name: "null body", body: `null`, wantError: true},
		{name: "empty body", body: `{}`, wantError: true},
		{name: "missing pending", body: `{"verified":false}`, wantError: true},
		{name: "missing verified", body: `{"pending":false}`, wantError: true},
		{name: "inconsistent", body: `{"pending":true,"verified":true}`, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewSSHVerificationReader()
			defer r.Close()
			calls, addresses := 0, 0
			failure := errors.New("unavailable")
			r.call = func(_ context.Context, verb string, params, result any) error {
				calls++
				if verb != "verify-admin-login" || params != nil {
					t.Fatalf("unexpected helper request: %s %v", verb, params)
				}
				if tc.helperErr {
					return failure
				}
				if tc.body == "" {
					return nil
				} // Success frame without a result body.
				return json.Unmarshal([]byte(tc.body), result)
			}
			r.address = func(context.Context) (string, error) {
				addresses++
				if tc.addressErr {
					return "203.0.113.99", failure
				}
				return "203.0.113.7", nil
			}
			got := r.Read()
			if got.Pending != tc.wantPending || (got.Err != nil) != tc.wantError || got.Address != tc.wantAddress {
				t.Fatalf("result=%+v", got)
			}
			if tc.helperErr && !errors.Is(got.Err, failure) {
				t.Fatalf("helper error lost: %v", got.Err)
			}
			if calls != 1 || addresses != tc.wantAddressReads {
				t.Fatalf("helper calls=%d address reads=%d", calls, addresses)
			}
		})
	}
}

func TestSSHVerificationDeadlineAndCloseJoin(t *testing.T) {
	for _, phase := range []string{"deadline", "close helper", "close address"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := NewSSHVerificationReader()
				defer r.Close()
				calls := 0
				release := make(chan struct{})
				block := func(ctx context.Context) error {
					<-ctx.Done()
					if phase != "deadline" {
						<-release
					}
					return errors.New("transport closed after cancellation")
				}
				r.call = func(ctx context.Context, _ string, _, result any) error {
					calls++
					if phase == "close address" {
						return json.Unmarshal([]byte(`{"pending":true,"verified":false}`), result)
					}
					return block(ctx)
				}
				r.address = func(ctx context.Context) (string, error) {
					if phase != "close address" {
						t.Error("address read after cancellation")
					}
					return "", block(ctx)
				}
				started := time.Now()
				done := make(chan SSHVerification, 1)
				go func() { done <- r.Read() }()
				want := context.DeadlineExceeded
				if phase != "deadline" {
					want = context.Canceled
					synctest.Wait()
					closed := make(chan struct{})
					go func() { r.Close(); close(closed) }()
					synctest.Wait()
					select {
					case <-closed:
						t.Fatal("Close returned before observation finished")
					default:
					}
					close(release)
					<-closed
				}
				got := <-done
				if !errors.Is(got.Err, want) || got.Pending || got.Address != "" {
					t.Fatalf("canceled observation published data: %+v", got)
				}
				if phase == "deadline" && time.Since(started) != 30*time.Second {
					t.Fatal("local deadline changed")
				}
				r.Close()
				if got := r.Read(); !errors.Is(got.Err, context.Canceled) || calls != 1 {
					t.Fatal("work admitted after close or hidden retry")
				}
			})
		})
	}
}
