// Every test here is in tier 0: it checks specs and model choices in memory and spawns nothing.

package shuttleengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckPromptEngine_RefusesAPromptRenderedForAnotherEngine covers each prompt-engine case a launching engine meets:
// its own name and a hand-written prompt pass;
// another engine's name and an unset one are refused, naming the engines and the model setting as the way forward.
func TestCheckPromptEngine_RefusesAPromptRenderedForAnotherEngine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		spec Spec
		// want lists the fragments the error carries; nil wants no error.
		want []string
	}{
		{"OwnEngine", Spec{Role: "r", PromptEngine: "claude"}, nil},
		{"HandWritten", Spec{Role: "r", HandWrittenPrompt: true}, nil},
		{"OtherEngine", Spec{Role: "r", PromptEngine: "codex"}, []string{`"codex"`, `"claude"`, "way forward", "model setting"}},
		{"Unset", Spec{Role: "r"}, []string{"names no prompt engine", `"claude"`, "way forward", "model setting"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := CheckPromptEngine(tt.spec, "claude")
			if tt.want == nil {
				if err != nil {
					t.Errorf("CheckPromptEngine(%+v, claude) = %v; want nil", tt.spec, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("CheckPromptEngine(%+v, claude) = nil; want an error", tt.spec)
			}
			for _, fragment := range tt.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("CheckPromptEngine(%+v, claude) = %q; want it to contain %q", tt.spec, err, fragment)
				}
			}
		})
	}
}

// TestEngineSetEngineName_EmptyChoiceTakesTheDefault covers a choice naming an engine and one naming none.
func TestEngineSetEngineName_EmptyChoiceTakesTheDefault(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	set := NewEngineSet("codex", nil)
	tests := []struct {
		name   string
		choice ModelChoice
		want   string
	}{
		{"NamesAnEngine", ModelChoice{Engine: "claude", Model: "opus"}, "claude"},
		{"NamesNone", ModelChoice{Model: "opus"}, "codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := set.EngineName(tt.choice); got != tt.want {
				t.Errorf("EngineName(%+v) = %q; want %q", tt.choice, got, tt.want)
			}
		})
	}
}
