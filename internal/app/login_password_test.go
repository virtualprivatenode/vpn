package app

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/virtualprivatenode/vpn/internal/loginpassword"
)

func TestLoginPasswordOutcomes(t *testing.T) {
	password, err := loginpassword.New("test password: with spaces")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		password loginpassword.Password
		failure  error
		want     LoginPasswordOutcome
		calls    int
	}{
		{"success", password, nil, LoginPasswordChanged, 1},
		{"lost response", password, errors.New("response lost"), LoginPasswordUnknown, 1},
		{"invalid", loginpassword.Password{}, nil, LoginPasswordNotChanged, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := NewLoginPasswordChanges()
			defer w.Close()
			calls := 0
			w.change = func(_ context.Context, got loginpassword.Password) error {
				calls++
				if got.Text() != password.Text() {
					t.Error("changed password input")
				}
				return tc.failure
			}
			results := w.Change(tc.password)
			result := <-results
			if result.Outcome != tc.want || (result.Err == nil) != (tc.want == LoginPasswordChanged) || calls != tc.calls {
				t.Fatalf("unexpected outcome: %+v, calls %d", result, calls)
			}
			if _, ok := <-results; ok {
				t.Fatal("duplicate result")
			}
		})
	}
}

func TestLoginPasswordDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := NewLoginPasswordChanges()
		defer w.Close()
		calls := 0
		w.change = func(ctx context.Context, _ loginpassword.Password) error {
			calls++
			<-ctx.Done()
			return ctx.Err()
		}
		password, _ := loginpassword.New("test password for timeout")
		started := time.Now()
		result := <-w.Change(password)
		if result.Outcome != LoginPasswordUnknown || !errors.Is(result.Err, context.DeadlineExceeded) || calls != 1 || time.Since(started) != 75*time.Second {
			t.Fatalf("request not bounded without retry: %+v", result)
		}
	})
}

func TestLoginPasswordCloseCancelsAndJoinsWithoutConsumer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := NewLoginPasswordChanges()
		release := make(chan struct{})
		w.change = func(ctx context.Context, _ loginpassword.Password) error {
			<-ctx.Done()
			<-release
			return ctx.Err()
		}
		password, _ := loginpassword.New("test password for shutdown")
		results := w.Change(password)
		synctest.Wait()
		closed := make(chan struct{})
		go func() { w.Close(); close(closed) }()
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("Close returned before its reader finished")
		default:
		}
		close(release)
		<-closed
		result := <-results
		if result.Outcome != LoginPasswordUnknown || !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("cancellation misreported: %+v", result)
		}
		if result := <-w.Change(password); result.Outcome != LoginPasswordNotChanged || !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("accepted change after shutdown: %+v", result)
		}
	})
}
