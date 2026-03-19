package welcome

import (
	"strings"
	"testing"
)

func TestButtonView(t *testing.T) {
	tests := []struct {
		name  string
		state ButtonState
		label string
	}{
		{"normal", ButtonNormal, "Test"},
		{"focused", ButtonFocused, "Test"},
		{"active", ButtonActive, "Test"},
		{"disabled", ButtonDisabled, "Test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			btn := Button{Label: tt.label, State: tt.state, Width: 16}
			view := btn.View()
			if view == "" {
				t.Error("button rendered empty string")
			}
			if !strings.Contains(view, tt.label) {
				t.Errorf("button view does not contain label %q", tt.label)
			}
		})
	}
}

func TestButtonGroupCreation(t *testing.T) {
	labels := []string{"One", "Two", "Three"}

	t.Run("horizontal", func(t *testing.T) {
		bg := NewButtonGroup(labels, Horizontal)
		if bg.FocusIndex != 0 {
			t.Errorf("expected FocusIndex 0, got %d", bg.FocusIndex)
		}
		if bg.ActiveIndex != -1 {
			t.Errorf("expected ActiveIndex -1, got %d", bg.ActiveIndex)
		}
		if bg.Gap != 0 {
			t.Errorf("horizontal should have Gap 0, got %d", bg.Gap)
		}
	})

	t.Run("vertical", func(t *testing.T) {
		bg := NewButtonGroup(labels, Vertical)
		if bg.Gap != 1 {
			t.Errorf("vertical should have Gap 1, got %d", bg.Gap)
		}
	})
}

func TestButtonGroupNavigation(t *testing.T) {
	labels := []string{"A", "B", "C", "D"}
	bg := NewButtonGroup(labels, Vertical)
	bg.Focus()

	if bg.FocusIndex != 0 {
		t.Fatalf("expected focus at 0, got %d", bg.FocusIndex)
	}

	bg.FocusNext()
	if bg.FocusIndex != 1 {
		t.Errorf("after FocusNext, expected 1, got %d", bg.FocusIndex)
	}

	bg.FocusNext()
	if bg.FocusIndex != 2 {
		t.Errorf("after second FocusNext, expected 2, got %d", bg.FocusIndex)
	}

	bg.FocusPrev()
	if bg.FocusIndex != 1 {
		t.Errorf("after FocusPrev, expected 1, got %d", bg.FocusIndex)
	}

	// Wrap forward
	bg.SetFocus(3)
	moved := bg.FocusNext()
	if !moved {
		t.Error("expected FocusNext to wrap around")
	}
	if bg.FocusIndex != 0 {
		t.Errorf("expected wrap to 0, got %d", bg.FocusIndex)
	}

	// Wrap backward
	bg.SetFocus(0)
	moved = bg.FocusPrev()
	if !moved {
		t.Error("expected FocusPrev to wrap around")
	}
	if bg.FocusIndex != 3 {
		t.Errorf("expected wrap to 3, got %d", bg.FocusIndex)
	}
}

func TestButtonGroupDisabled(t *testing.T) {
	labels := []string{"A", "B", "C", "D"}
	bg := NewButtonGroup(labels, Vertical)
	bg.Focus()
	bg.SetDisabled(1, true)
	bg.SetDisabled(2, true)

	bg.FocusNext()
	if bg.FocusIndex != 3 {
		t.Errorf("expected skip to 3, got %d", bg.FocusIndex)
	}

	bg.FocusPrev()
	if bg.FocusIndex != 0 {
		t.Errorf("expected skip back to 0, got %d", bg.FocusIndex)
	}

	// SetFocus on disabled should no-op
	bg.SetFocus(1)
	if bg.FocusIndex != 0 {
		t.Errorf("SetFocus on disabled should no-op, got %d", bg.FocusIndex)
	}
}

func TestButtonGroupActivate(t *testing.T) {
	labels := []string{"A", "B", "C"}
	bg := NewButtonGroup(labels, Horizontal)
	bg.Focus()

	bg.SetFocus(1)
	idx := bg.Activate()
	if idx != 1 {
		t.Errorf("expected activate index 1, got %d", idx)
	}
	if bg.ActiveIndex != 1 {
		t.Errorf("expected ActiveIndex 1, got %d", bg.ActiveIndex)
	}
}

func TestButtonGroupActivateDisabled(t *testing.T) {
	labels := []string{"A", "B"}
	bg := NewButtonGroup(labels, Vertical)
	bg.SetDisabled(0, true)
	bg.Focus()

	// Focus should have moved to 1 on Focus() call since 0 is disabled
	if bg.FocusIndex != 1 {
		t.Errorf("expected focus to skip to 1, got %d", bg.FocusIndex)
	}

	idx := bg.Activate()
	if idx != 1 {
		t.Errorf("expected activate index 1, got %d", idx)
	}
}

func TestButtonGroupHandleKeyPressVertical(t *testing.T) {
	bg := NewButtonGroup([]string{"A", "B", "C"}, Vertical)
	bg.Focus()

	handled, activated := bg.HandleKeyPress("down")
	if !handled {
		t.Error("down should be handled")
	}
	if activated != -1 {
		t.Error("down should not activate")
	}
	if bg.FocusIndex != 1 {
		t.Errorf("expected focus 1 after down, got %d", bg.FocusIndex)
	}

	handled, activated = bg.HandleKeyPress("up")
	if !handled {
		t.Error("up should be handled")
	}
	if activated != -1 {
		t.Error("up should not activate")
	}
	if bg.FocusIndex != 0 {
		t.Errorf("expected focus 0 after up, got %d", bg.FocusIndex)
	}

	bg.SetFocus(1)
	handled, activated = bg.HandleKeyPress("enter")
	if !handled {
		t.Error("enter should be handled")
	}
	if activated != 1 {
		t.Errorf("expected activation of 1, got %d", activated)
	}
}

func TestButtonGroupHandleKeyPressHorizontal(t *testing.T) {
	bg := NewButtonGroup([]string{"A", "B", "C"}, Horizontal)
	bg.Focus()

	handled, _ := bg.HandleKeyPress("right")
	if !handled {
		t.Error("right should be handled")
	}
	if bg.FocusIndex != 1 {
		t.Errorf("expected focus 1 after right, got %d", bg.FocusIndex)
	}

	handled, _ = bg.HandleKeyPress("left")
	if !handled {
		t.Error("left should be handled")
	}
	if bg.FocusIndex != 0 {
		t.Errorf("expected focus 0 after left, got %d", bg.FocusIndex)
	}
}

func TestButtonGroupUnfocusedIgnoresKeys(t *testing.T) {
	bg := NewButtonGroup([]string{"A", "B"}, Vertical)
	// Don't call Focus()

	handled, _ := bg.HandleKeyPress("down")
	if handled {
		t.Error("unfocused group should not handle keys")
	}
}

func TestButtonGroupViewContainsLabels(t *testing.T) {
	t.Run("horizontal", func(t *testing.T) {
		bg := NewButtonGroup([]string{"Alpha", "Beta", "Gamma"}, Horizontal)
		bg.Focus()
		bg.SetWidth(12)
		view := bg.View()
		for _, label := range bg.Labels {
			if !strings.Contains(view, label) {
				t.Errorf("horizontal view missing label %q", label)
			}
		}
	})

	t.Run("vertical", func(t *testing.T) {
		bg := NewButtonGroup([]string{"One", "Two", "Three"}, Vertical)
		bg.Focus()
		bg.ActivateIndex(0)
		bg.SetWidth(16)
		view := bg.View()
		for _, label := range bg.Labels {
			if !strings.Contains(view, label) {
				t.Errorf("vertical view missing label %q", label)
			}
		}
	})

	t.Run("disabled_renders", func(t *testing.T) {
		bg := NewButtonGroup([]string{"Active", "Disabled"}, Vertical)
		bg.SetDisabled(1, true)
		bg.SetWidth(16)
		view := bg.View()
		if !strings.Contains(view, "Disabled") {
			t.Error("disabled button should still render its label")
		}
	})
}

func TestButtonGroupActivateIndex(t *testing.T) {
	bg := NewButtonGroup([]string{"A", "B", "C"}, Vertical)
	bg.ActivateIndex(2)
	if bg.ActiveIndex != 2 {
		t.Errorf("expected ActiveIndex 2, got %d", bg.ActiveIndex)
	}

	// ActivateIndex on disabled should no-op
	bg.SetDisabled(0, true)
	bg.ActivateIndex(0)
	if bg.ActiveIndex != 2 {
		t.Errorf("ActivateIndex on disabled should no-op, got %d", bg.ActiveIndex)
	}
}

func TestButtonGroupVerticalVsHorizontalKeys(t *testing.T) {
	// Vertical should not respond to left/right
	vbg := NewButtonGroup([]string{"A", "B"}, Vertical)
	vbg.Focus()
	handled, _ := vbg.HandleKeyPress("right")
	if handled {
		t.Error("vertical group should not handle right key")
	}
	handled, _ = vbg.HandleKeyPress("left")
	if handled {
		t.Error("vertical group should not handle left key")
	}

	// Horizontal should not respond to up/down
	hbg := NewButtonGroup([]string{"A", "B"}, Horizontal)
	hbg.Focus()
	handled, _ = hbg.HandleKeyPress("up")
	if handled {
		t.Error("horizontal group should not handle up key")
	}
	handled, _ = hbg.HandleKeyPress("down")
	if handled {
		t.Error("horizontal group should not handle down key")
	}
}
