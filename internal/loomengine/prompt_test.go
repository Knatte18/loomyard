// prompt_test.go — untagged Tier-1 unit tests for composePrompt and
// modeRules. Assertions are stable substrings on load-bearing tokens, not
// full golden-file equality, so the template's prose can evolve without
// breaking this test.

package loomengine

import (
	"strings"
	"testing"
)

// TestComposePrompt_RendersMarkers verifies, for both autonomous values,
// that the rendered prompt leaves no unrendered {{ marker token, contains
// the given slug and both given paths, and contains the board-read command
// the discussion agent must run first.
func TestComposePrompt_RendersMarkers(t *testing.T) {
	tests := []struct {
		name       string
		autonomous bool
	}{
		{"Interactive", false},
		{"Autonomous", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug := "add-json-flag"
			decisionRecordPath := "/hub/repo/_lyx/discussion/decision-record.md"
			supportLogPath := "/hub/repo/_lyx/discussion/support-log.md"

			got, err := composePrompt(slug, decisionRecordPath, supportLogPath, tt.autonomous)
			if err != nil {
				t.Fatalf("composePrompt(%q, %q, %q, %v) = _, %v; want nil error", slug, decisionRecordPath, supportLogPath, tt.autonomous, err)
			}
			rendered := string(got)

			if strings.Contains(rendered, "{{") {
				t.Errorf("composePrompt(...) output contains an unrendered marker token:\n%s", rendered)
			}
			if !strings.Contains(rendered, slug) {
				t.Errorf("composePrompt(...) output does not contain slug %q", slug)
			}
			if !strings.Contains(rendered, decisionRecordPath) {
				t.Errorf("composePrompt(...) output does not contain decision-record path %q", decisionRecordPath)
			}
			if !strings.Contains(rendered, supportLogPath) {
				t.Errorf("composePrompt(...) output does not contain support-log path %q", supportLogPath)
			}
			if !strings.Contains(rendered, "lyx board get") {
				t.Errorf("composePrompt(...) output does not contain the board-read command substring %q", "lyx board get")
			}
		})
	}
}

// TestComposePrompt_ModeLanguageDiffers verifies the autonomous=true
// rendering carries autonomous-mode language, the autonomous=false
// rendering carries interactive-mode language, and the two renderings are
// not identical.
func TestComposePrompt_ModeLanguageDiffers(t *testing.T) {
	slug := "add-json-flag"
	decisionRecordPath := "/hub/repo/_lyx/discussion/decision-record.md"
	supportLogPath := "/hub/repo/_lyx/discussion/support-log.md"

	autonomousOut, err := composePrompt(slug, decisionRecordPath, supportLogPath, true)
	if err != nil {
		t.Fatalf("composePrompt(autonomous=true) = _, %v; want nil error", err)
	}
	interactiveOut, err := composePrompt(slug, decisionRecordPath, supportLogPath, false)
	if err != nil {
		t.Fatalf("composePrompt(autonomous=false) = _, %v; want nil error", err)
	}

	if !strings.Contains(string(autonomousOut), "best-judgment") {
		t.Errorf("composePrompt(autonomous=true) output does not contain autonomous-mode language %q", "best-judgment")
	}
	if !strings.Contains(string(interactiveOut), "operator") {
		t.Errorf("composePrompt(autonomous=false) output does not contain interactive-mode language %q", "operator")
	}
	if string(autonomousOut) == string(interactiveOut) {
		t.Error("composePrompt(autonomous=true) and composePrompt(autonomous=false) rendered identically; want them to differ")
	}
}

// TestModeRules verifies modeRules(true) and modeRules(false) each return a
// non-empty string and the two are distinct.
func TestModeRules(t *testing.T) {
	autonomous := modeRules(true)
	interactive := modeRules(false)

	if autonomous == "" {
		t.Error("modeRules(true) = \"\"; want non-empty string")
	}
	if interactive == "" {
		t.Error("modeRules(false) = \"\"; want non-empty string")
	}
	if autonomous == interactive {
		t.Error("modeRules(true) == modeRules(false); want distinct strings")
	}
}
