package updateui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"xchat/internal/update"
)

func updatePrompt(decision update.Decision, retry bool) update.Prompt {
	current, _ := update.ParseVersion("0.3.2")
	latest, _ := update.ParseVersion("0.4.0")
	return update.Prompt{Current: current, Latest: latest, Notes: "online update", Decision: decision, Retry: retry}
}

func TestConsoleOffersOnlyActionsAllowedByPolicy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		prompt   update.Prompt
		expected update.Action
	}{
		{name: "optional update", input: "u\n", prompt: updatePrompt(update.DecisionOptional, false), expected: update.ActionUpdate},
		{name: "optional skip", input: "s\n", prompt: updatePrompt(update.DecisionOptional, false), expected: update.ActionSkip},
		{name: "required update", input: "u\n", prompt: updatePrompt(update.DecisionRequired, false), expected: update.ActionUpdate},
		{name: "required exit", input: "e\n", prompt: updatePrompt(update.DecisionRequired, false), expected: update.ActionExit},
		{name: "retry", input: "r\n", prompt: updatePrompt(update.DecisionRequired, true), expected: update.ActionRetry},
		{name: "retry exit", input: "e\n", prompt: updatePrompt(update.DecisionRequired, true), expected: update.ActionExit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := new(bytes.Buffer)
			console := New(strings.NewReader(test.input), output)
			if actual := console.Choose(test.prompt); actual != test.expected {
				t.Fatalf("Choose = %v, want %v; output=%q", actual, test.expected, output.String())
			}
		})
	}
}

func TestConsoleRepeatsInvalidChoiceAndReportsProgressSafely(t *testing.T) {
	output := new(bytes.Buffer)
	console := New(strings.NewReader("x\nu\n"), output)
	if action := console.Choose(updatePrompt(update.DecisionOptional, false)); action != update.ActionUpdate {
		t.Fatalf("Choose = %v", action)
	}
	if !strings.Contains(output.String(), "无效选择") {
		t.Fatalf("invalid choice was not explained: %q", output.String())
	}
	console.Progress(5, 0)
	console.Progress(5, 10)
	failure := errors.New("too large")
	console.ShowError(failure)
	if !strings.Contains(output.String(), "5/10") || !strings.Contains(output.String(), failure.Error()) {
		t.Fatalf("progress/error missing: %q", output.String())
	}
}
