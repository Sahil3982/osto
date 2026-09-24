package cli

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
)

func TestRegisterUsesMaskedPasswordPrompt(t *testing.T) {
	model := New(nil, time.Minute).(*model)
	model.input.SetValue("register alice")

	if cmd := model.submit(); cmd != nil {
		t.Fatal("register username should not start a command yet")
	}
	if model.mode != modeRegisterPassword {
		t.Fatalf("mode = %q, want register-password", model.mode)
	}
	if model.input.EchoMode != textinput.EchoPassword {
		t.Fatal("registration password prompt is not masked")
	}
}

func TestLoginWithoutUsernamePromptsSafely(t *testing.T) {
	model := New(nil, time.Minute).(*model)
	model.input.SetValue("login")

	if cmd := model.submit(); cmd != nil {
		t.Fatal("login command should first open the username prompt")
	}
	if model.mode != modeLoginUsername || model.input.EchoMode != textinput.EchoNormal {
		t.Fatalf("unexpected login prompt state: mode=%q echo=%v", model.mode, model.input.EchoMode)
	}
}

func TestSensitiveCommandsAreNotSavedInHistory(t *testing.T) {
	model := New(nil, time.Minute).(*model)
	model.input.SetValue("login alice")
	model.submit()
	if len(model.history) != 1 {
		t.Fatalf("history length = %d, want one non-sensitive command", len(model.history))
	}
	if model.history[0] != "login alice" {
		t.Fatalf("history entry = %q, want login alice", model.history[0])
	}
}
