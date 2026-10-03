// prompt_test.go — untagged Tier-1 unit tests for composePrompt and modeRules.
// Assertions are stable substrings on load-bearing tokens, not full golden-file equality, so the
// template's prose can evolve without breaking this test.
// It also declares newTestStencilsDir, the package-local test helper every loomengine test uses to
// seed a hermetic stencils directory from the shipped stencils package defaults.

package loomengine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// newTestStencilsDir returns a t.TempDir() seeded with every registry stencil.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

// newMinimalStencilsDir returns a seeded stencils directory with the pattern-directive and friction-directive stencils removed, for tests that must prove a composer needs none of them to render its Tier-2-off / PATTERN-inactive path.
func newMinimalStencilsDir(t *testing.T) string {
	t.Helper()

	dir := stencilkit.Seed(t)
	stencilkit.Remove(t, dir,
		"pattern-directive-implementer",
		"pattern-directive-review-fix",
		"pattern-directive-orchestrator",
		"friction-directive-implementer",
		"friction-directive-review-fix",
		"friction-directive-orchestrator",
		"friction-directive-interview",
	)
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

			got, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, "", tt.autonomous)
			if err != nil {
				t.Fatalf("composePrompt(%q, %q, %q, %q, \"\", %v) = _, %v; want nil error", stencilsDir, slug, decisionRecordPath, supportLogPath, tt.autonomous, err)
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

	got, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, "", true)
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

	autonomousOut, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, "", true)
	if err != nil {
		t.Fatalf("composePrompt(autonomous=true) = _, %v; want nil error", err)
	}
	interactiveOut, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, "", false)
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

// TestComposePrompt_FrictionEnabled verifies that, given a real friction directive resolved via
// friction.NotePath/friction.Directive exactly as DiscussionSpec resolves one, composePrompt's
// rendered prompt contains the resolved absolute note path verbatim -- the property that catches a
// composer wiring the wrong path.
func TestComposePrompt_FrictionEnabled(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)
	frictionDir := filepath.Join(t.TempDir(), "friction")
	notePath := friction.NotePath(frictionDir, "Discussion-Write")
	if notePath == "" {
		t.Fatal("friction.NotePath(frictionDir, \"Discussion-Write\") = \"\"; want a resolved path")
	}
	directive, err := friction.Directive(notePath, stencilsDir, friction.RoleInterview)
	if err != nil {
		t.Fatalf("friction.Directive(...) = _, %v; want nil error", err)
	}

	got, err := composePrompt(stencilsDir, "add-json-flag", "/hub/repo/_lyx/discussion/decision-record.md", "/hub/repo/_lyx/discussion/support-log.md", directive, false)
	if err != nil {
		t.Fatalf("composePrompt(..., directive, false) = _, %v; want nil error", err)
	}

	if !strings.Contains(string(got), notePath) {
		t.Errorf("composePrompt(...) output does not contain the resolved friction note path %q verbatim", notePath)
	}
}

// TestComposePrompt_FrictionDisabled verifies that, with an empty frictionDirective (Tier 2 off) and
// a stencilsDir carrying no friction-directive stencil at all, composePrompt still renders
// successfully with no friction content -- proving composePrompt reads no friction stencil of its
// own, the same no-read guarantee internal/friction's own tests pin by pointing at a stencilsDir a
// read would fail against.
func TestComposePrompt_FrictionDisabled(t *testing.T) {
	stencilsDir := newMinimalStencilsDir(t)

	got, err := composePrompt(stencilsDir, "add-json-flag", "/hub/repo/_lyx/discussion/decision-record.md", "/hub/repo/_lyx/discussion/support-log.md", "", false)
	if err != nil {
		t.Fatalf("composePrompt(..., frictionDirective=\"\", false) = _, %v; want nil error", err)
	}

	rendered := string(got)
	if strings.Contains(rendered, "{{") {
		t.Errorf("composePrompt(..., frictionDirective=\"\", ...) output contains an unrendered marker token:\n%s", rendered)
	}
	if strings.Contains(rendered, "Friction note") {
		t.Error("composePrompt(..., frictionDirective=\"\", ...) output contains friction directive content; want none when Tier 2 is off")
	}
}

// TestComposePrompt_FrictionMarkerFreeTemplate verifies that a seeded loom-template-discussion stencil
// whose bytes carry no {{.friction_directive}} literal still composes successfully -- never an error
// -- while Tier 2 is enabled (a non-empty frictionDirective). This is the operator-edited-stencil and
// dev-build case: internal/stencilstore/reconcile.go never refreshes a StateEdited stencil, so it is
// reachable in a real worktree and must degrade to a warning rather than a failed run.
func TestComposePrompt_FrictionMarkerFreeTemplate(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)
	markerFree := bytes.ReplaceAll(stencils.LoomTemplateDiscussion, []byte("{{.friction_directive}}"), nil)
	discussionPath := filepath.Join(stencilsDir, "loom", "loom-template-discussion.md")
	if err := os.WriteFile(discussionPath, markerFree, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", discussionPath, err)
	}

	_, err := composePrompt(stencilsDir, "add-json-flag", "/hub/repo/_lyx/discussion/decision-record.md", "/hub/repo/_lyx/discussion/support-log.md", "some friction directive text", false)
	if err != nil {
		t.Fatalf("composePrompt(..., frictionDirective=<non-empty>, ...) with a marker-free template = _, %v; want nil error", err)
	}
}
