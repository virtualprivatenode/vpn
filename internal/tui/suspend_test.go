package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHandoffDisablesSuspendButPreservesExit(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		m := Model{disableSuspend: disabled, nav: NewNavSidebar()}
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
		if disabled {
			if cmd != nil {
				t.Fatal("handoff requested suspension without a shell to resume it")
			}
		} else {
			if cmd == nil {
				t.Fatal("normal shell launch lost suspension")
			}
			if _, ok := cmd().(tea.SuspendMsg); !ok {
				t.Fatal("normal shell launch did not request suspension")
			}
		}
		_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("lost normal exit")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("normal exit did not request quit")
		}
	}
}
