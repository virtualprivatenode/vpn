package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/loginpassword"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

type passwordChangesStub struct {
	passwords []loginpassword.Password
	results   chan app.LoginPasswordResult
}

func (f *passwordChangesStub) Change(p loginpassword.Password) <-chan app.LoginPasswordResult {
	f.passwords = append(f.passwords, p)
	return f.results
}
func (*passwordChangesStub) Close() {}

func TestLoginPasswordOwnsSubmissionAndHiddenResult(t *testing.T) {
	theme.Init(true)
	fake := &passwordChangesStub{results: make(chan app.LoginPasswordResult, 1)}
	ctx := &ScreenContext{LoginPasswords: fake}
	s := NewChangePasswordScreen(ctx)
	secret := "test password for submission"
	s.newInput.SetValue(secret)
	s.confInput.SetValue("different test password")
	if _, cmd := s.submit(); cmd != nil || len(fake.passwords) != 0 {
		t.Fatal("mismatch submitted")
	}
	s.confInput.SetValue(secret)
	_, cmd := s.submit()
	if cmd == nil || len(fake.passwords) != 1 || fake.passwords[0].Text() != secret || s.newInput.Value() != "" || s.confInput.Value() != "" {
		t.Fatal("submission did not freeze and release form secrets")
	}
	if _, cmd := s.submit(); cmd != nil || len(fake.passwords) != 1 {
		t.Fatal("duplicate submission")
	}
	other := NewChangePasswordScreen(ctx)
	other.step, other.attempt = changePwStepWorking, s.attempt
	m := Model{nav: NewNavSidebar(), screenCtx: ctx, tabs: []openTab{
		{Kind: tabSSHChangePassword, Section: secSystem, Screen: s},
		{Kind: tabSSHChangePassword, Section: secSystem, Screen: other},
	}}
	m.nav.ActiveItem = secSystem
	_, navigate := s.HandleKey("left", tea.KeyPressMsg{})
	if navigate == nil {
		t.Fatal("cannot leave working screen")
	}
	updated, _ := m.Update(navigate())
	m = updated.(Model)
	m.nav.Cursor = secWallet
	updated, _ = m.handleSidebarKey("enter")
	m = updated.(Model)
	m.Update(changePwDoneMsg{owner: s, attempt: s.attempt - 1})
	if !sshAccessBusy(s) {
		t.Fatal("stale result completed request")
	}
	fake.results <- app.LoginPasswordResult{Outcome: app.LoginPasswordChanged}
	result := cmd().(changePwDoneMsg)
	other.HandleMsg(result)
	m.Update(result)
	if s.step != changePwStepResult || s.result.Outcome != app.LoginPasswordChanged || !sshAccessBusy(other) {
		t.Fatal("hidden result lost or delivered to wrong owner")
	}
	result.result = app.LoginPasswordResult{Outcome: app.LoginPasswordUnknown, Err: errors.New("late failure")}
	m.Update(result)
	if s.result.Outcome != app.LoginPasswordChanged {
		t.Fatal("duplicate result changed outcome")
	}
	_, done := s.HandleKey("enter", tea.KeyPressMsg{})
	updated, _ = m.Update(done())
	m = updated.(Model)
	if len(m.tabs) != 1 || m.tabs[0].Screen != other {
		t.Fatal("Done closed wrong hidden tab")
	}
	for _, msg := range []tea.Msg{result, done()} {
		updated, _ = m.Update(msg)
		m = updated.(Model)
		if len(m.tabs) != 1 || m.tabs[0].Screen != other || !sshAccessBusy(other) {
			t.Fatal("closed-screen message changed or removed replacement")
		}
	}
}

func TestLoginPasswordUnknownOutcomeCopy(t *testing.T) {
	theme.Init(true)
	s := NewChangePasswordScreen(&ScreenContext{})
	s.step, s.attempt = changePwStepWorking, 1
	s.HandleMsg(changePwDoneMsg{owner: s, attempt: 1, result: app.LoginPasswordResult{Outcome: app.LoginPasswordUnknown, Err: errors.New("connection lost")}})
	view := s.View(67, 30)
	if !strings.Contains(view, "Password change not confirmed") || !strings.Contains(view, "may have changed") || strings.Contains(view, "successfully") {
		t.Fatal("uncertain outcome implies success or rollback")
	}
}

func TestLoginPasswordPasteIsNeverSilentlyChanged(t *testing.T) {
	for _, tc := range []struct{ name, value, want string }{
		{"password manager newline", "correct horse battery staple\n", "correct horse battery staple"},
		{"embedded newline", "correct horse\nbattery staple", ""},
		{"embedded tab", "correct horse\tbattery staple", ""},
		{"NUL", "correct horse\x00battery staple", ""},
		{"truncation", strings.Repeat("x", 257), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewChangePasswordScreen(&ScreenContext{})
			s.newInput.SetValue("previous password value")
			s.HandleMsg(tea.PasteMsg{Content: tc.value})
			if s.newInput.Value() != tc.want || (s.inputErr != "") != (tc.want == "") {
				t.Fatal("paste silently changed or incorrectly rejected")
			}
		})
	}
}

func TestLoginPasswordBusyReplacementAndDelayedCancel(t *testing.T) {
	ctx := &ScreenContext{LoginPasswords: &passwordChangesStub{results: make(chan app.LoginPasswordResult, 1)}}
	s := NewChangePasswordScreen(ctx)
	cancel := s.closeCommand()
	s.newInput.SetValue("correct horse battery staple")
	s.confInput.SetValue("correct horse battery staple")
	s.submit()
	m := Model{nav: NewNavSidebar(), screenCtx: ctx, tabs: []openTab{{Kind: tabSSHChangePassword, Section: secSystem, Screen: s}}}
	m.nav.ActiveItem = secSystem
	updated, _ := m.Update(openTabMsg{Kind: tabSSHChangePassword, Replace: true, Screen: NewChangePasswordScreen(ctx)})
	m = updated.(Model)
	if m.tabs[0].Screen != s {
		t.Fatal("busy password tab replaced")
	}
	m.Update(changePwDoneMsg{owner: s, attempt: s.attempt, result: app.LoginPasswordResult{Outcome: app.LoginPasswordChanged}})
	updated, _ = m.Update(cancel())
	m = updated.(Model)
	if len(m.tabs) != 1 {
		t.Fatal("old cancel closed completed newer attempt")
	}
}
