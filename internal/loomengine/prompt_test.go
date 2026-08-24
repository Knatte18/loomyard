// prompt_test.go — untagged Tier-1 unit tests for composePrompt and modeRules.
// Assertions are stable substrings on load-bearing tokens, not full golden-file equality, so the
// template's prose can evolve without breaking this test.
// It also declares newTestStencilsDir, the package-local test helper every loomengine test uses to
// seed a hermetic stencils directory from the shipped stencils package defaults.

package loomengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
)

// newTestStencilsDir builds a t.TempDir() seeded with loom's two stencils plus the three
// pattern-directive stencils, all copied byte-for-byte from the stencils package's embedded
// defaults, and returns the directory to pass as stencilsDir.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	loomDir := filepath.Join(dir, "loom")
	if err := os.MkdirAll(loomDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", loomDir, err)
	}
	if err := os.WriteFile(filepath.Join(loomDir, "loom-template-discussion.md"), stencils.LoomTemplateDiscussion, 0o644); err != nil {
		t.Fatalf("WriteFile(loom-template-discussion.md) = %v; want nil", err)
	}
	if err := os.WriteFile(filepath.Join(loomDir, "loom-template-plan.md"), stencils.LoomTemplatePlan, 0o644); err != nil {
		t.Fatalf("WriteFile(loom-template-plan.md) = %v; want nil", err)
	}

	patternDir := filepath.Join(dir, "pattern")
	if err := os.MkdirAll(patternDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", patternDir, err)
	}
	if err := os.WriteFile(filepath.Join(patternDir, "pattern-directive-implementer.md"), stencils.PatternDirectiveImplementer, 0o644); err != nil {
		t.Fatalf("WriteFile(pattern-directive-implementer.md) = %v; want nil", err)
	}
	if err := os.WriteFile(filepath.Join(patternDir, "pattern-directive-review-fix.md"), stencils.PatternDirectiveReviewFix, 0o644); err != nil {
		t.Fatalf("WriteFile(pattern-directive-review-fix.md) = %v; want nil", err)
	}
	if err := os.WriteFile(filepath.Join(patternDir, "pattern-directive-orchestrator.md"), stencils.PatternDirectiveOrchestrator, 0o644); err != nil {
		t.Fatalf("WriteFile(pattern-directive-orchestrator.md) = %v; want nil", err)
	}
	return dir
}

// TestComposePrompt_RendersMarkers verifies the rendered prompt has no unrendered markers, contains
// the slug and paths, and contains the board-read command.
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
			stencilsDir := newTestStencilsDir(t)
			slug := "add-json-flag"
			decisionRecordPath := "/hub/repo/_lyx/discussion/decision-record.md"
			supportLogPath := "/hub/repo/_lyx/discussion/support-log.md"

			got, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, tt.autonomous)
			if err != nil {
				t.Fatalf("composePrompt(%q, %q, %q, %q, %v) = _, %v; want nil error", stencilsDir, slug, decisionRecordPath, supportLogPath, tt.autonomous, err)
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
			if !strings.Contains(rendered, "scribe:prose") {
				t.Errorf("composePrompt(...) output does not contain the Step 0 skill name %q", "scribe:prose")
			}
			if !strings.Contains(rendered, "scribe:conversation") {
				t.Errorf("composePrompt(...) output does not contain the Step 0 skill name %q", "scribe:conversation")
			}
			if !strings.Contains(rendered, "lyx loom validate-discussion") {
				t.Errorf("composePrompt(...) output does not contain the Step 6 self-check command %q", "lyx loom validate-discussion")
			}
			if !strings.Contains(rendered, "MUST NOT gather exact signatures") {
				t.Errorf("composePrompt(...) output does not contain the exploration bound's MUST NOT clause")
			}
		})
	}
}

// TestComposePrompt_AutonomousOutputHasNoAutoFlag verifies the rendered autonomous output does not
// name the nonexistent `--auto` flag.
func TestComposePrompt_AutonomousOutputHasNoAutoFlag(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)
	slug := "add-json-flag"
	decisionRecordPath := "/hub/repo/_lyx/discussion/decision-record.md"
	supportLogPath := "/hub/repo/_lyx/discussion/support-log.md"

	got, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, true)
	if err != nil {
		t.Fatalf("composePrompt(..., autonomous=true) = _, %v; want nil error", err)
	}

	if strings.Contains(string(got), "--auto") {
		t.Errorf("composePrompt(..., autonomous=true) output contains %q; want no reference to the nonexistent flag", "--auto")
	}
}

// TestComposePrompt_ModeLanguageDiffers verifies the mode renderings carry different language and
// are not identical.
func TestComposePrompt_ModeLanguageDiffers(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)
	slug := "add-json-flag"
	decisionRecordPath := "/hub/repo/_lyx/discussion/decision-record.md"
	supportLogPath := "/hub/repo/_lyx/discussion/support-log.md"

	autonomousOut, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, true)
	if err != nil {
		t.Fatalf("composePrompt(autonomous=true) = _, %v; want nil error", err)
	}
	interactiveOut, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, false)
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

// TestModeRules verifies modeRules returns distinct non-empty strings for each mode.
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
	if strings.Contains(autonomous, "--auto") {
		t.Errorf("modeRules(true) contains %q; want no reference to the nonexistent flag", "--auto")
	}
}
