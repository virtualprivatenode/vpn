package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/loginpassword"
	"github.com/virtualprivatenode/vpn/internal/paths"
)

type LoginPasswordOutcome int

const (
	LoginPasswordUnknown LoginPasswordOutcome = iota
	LoginPasswordNotChanged
	LoginPasswordChanged
)

type LoginPasswordResult struct {
	Outcome LoginPasswordOutcome
	Err     error
}

// LoginPasswordChanges owns local password-change requests for one TUI.
// Close cancels and joins its readers, not mutations accepted by the helper.
type LoginPasswordChanges struct {
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	workers sync.WaitGroup
	change  func(context.Context, loginpassword.Password) error
}

func NewLoginPasswordChanges() *LoginPasswordChanges {
	ctx, cancel := context.WithCancel(context.Background())
	return &LoginPasswordChanges{ctx: ctx, cancel: cancel, change: func(ctx context.Context, password loginpassword.Password) error {
		session, err := helper.StartContext(ctx, helper.VerbSetUserPassword,
			helper.SetUserPasswordParams{User: paths.AdminUser, Password: password.Text()})
		if err != nil {
			return err
		}
		return session.Wait(nil)
	}}
}

func (w *LoginPasswordChanges) Close() {
	w.mu.Lock()
	w.cancel()
	w.mu.Unlock()
	w.workers.Wait()
}

// Change starts immediately. The buffered result is delivered even if rendering
// stops. A transport error cannot prove whether the helper accepted the request.
func (w *LoginPasswordChanges) Change(password loginpassword.Password) <-chan LoginPasswordResult {
	results := make(chan LoginPasswordResult, 1)
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.ctx.Err()
	if password.Text() == "" {
		err = errors.New("password was not validated")
	}
	if err != nil {
		results <- LoginPasswordResult{Outcome: LoginPasswordNotChanged, Err: err}
		close(results)
		return results
	}
	// Include time queued behind another helper verb. This is a local observation
	// bound, not a promise that an accepted root operation has stopped.
	ctx, cancel := context.WithTimeout(w.ctx, 75*time.Second)
	w.workers.Go(func() {
		defer cancel()
		defer close(results)
		if err := ctx.Err(); err != nil {
			results <- LoginPasswordResult{Outcome: LoginPasswordNotChanged, Err: err}
			return
		}
		err := w.change(ctx, password)
		outcome := LoginPasswordChanged
		if err != nil {
			outcome = LoginPasswordUnknown
		}
		results <- LoginPasswordResult{Outcome: outcome, Err: err}
	})
	return results
}
