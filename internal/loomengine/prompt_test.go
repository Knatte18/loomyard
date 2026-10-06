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

// TestComposePrompt_RendersMarkers verifies the rendered prompt of each mode has no unrendered markers, contains the slug, the paths and the board-read command, and names no skill (skills load from the spec, not the stencil), that the modes carry different language (and modeRules returns distinct non-empty strings for them), and that the autonomous output and its mode rules name no nonexistent `--auto` flag.
func TestComposePrompt_RendersMarkers(t *testing.T) {
	t.Parallel()
	const (
		slug               = "add-json-flag"
		decisionRecordPath = "/hub/repo/_lyx/discussion/decision-record.md"
		supportLogPath     = "/hub/repo/_lyx/discussion/support-log.md"
	)
	stencilsDir := newTestStencilsDir(t)
	render := func(t *testing.T, autonomous bool) string {
		t.Helper()
		got, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, "", "", "", autonomous)
		if err != nil {
			t.Fatalf("composePrompt(..., autonomous=%v) = _, %v; want nil error", autonomous, err)
		}
		return string(got)
	}

	tests := []struct {
		name         string
		autonomous   bool
		wantLanguage string
	}{
		{"Interactive", false, "operator"},
		{"Autonomous", true, "best-judgment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rendered := render(t, tt.autonomous)

			if strings.Contains(rendered, "{{") {
				t.Errorf("composePrompt(...) output contains an unrendered marker token:\n%s", rendered)
			}
			for _, want := range []string{
				slug,
				decisionRecordPath,
				supportLogPath,
				"lyx board get",
				"lyx loom validate-discussion",
				"MUST NOT gather exact signatures",
				tt.wantLanguage,
			} {
				if !strings.Contains(rendered, want) {
					t.Errorf("composePrompt(..., autonomous=%v) output does not contain %q", tt.autonomous, want)
				}
			}
			for _, skill := range discussionSkills {
				if strings.Contains(rendered, skill) {
					t.Errorf("composePrompt(...) output names the skill %q; skills load from the spec, not the stencil", skill)
				}
			}
			if rules := modeRules(tt.autonomous); rules == "" {
				t.Errorf("modeRules(%v) = \"\"; want non-empty string", tt.autonomous)
			}
			if tt.autonomous {
				if strings.Contains(rendered, "--auto") || strings.Contains(modeRules(true), "--auto") {
					t.Errorf("autonomous output or modeRules(true) contains %q; want no reference to the nonexistent flag", "--auto")
				}
			}
		})
	}

	if modeRules(true) == modeRules(false) {
		t.Error("modeRules(true) == modeRules(false); want distinct strings")
	}
	if render(t, true) == render(t, false) {
		t.Error("composePrompt(autonomous=true) and composePrompt(autonomous=false) rendered identically; want them to differ")
	}
}

// TestCompose_FrictionDirective verifies, for the discussion and the plan composer, that a real friction directive resolved via friction.NotePath/friction.Directive exactly as the spec builders resolve one lands its resolved absolute note path verbatim in the prompt (the property that catches a composer wiring the wrong path); that an empty directive (Tier 2 off) over a stencilsDir carrying no friction-directive stencil still renders with no friction content (the composer reads no friction stencil of its own); and that a seeded template whose bytes carry no {{.friction_directive}} literal still composes while Tier 2 is enabled -- the operator-edited-stencil and dev-build case, since internal/stencilstore/reconcile.go never refreshes a StateEdited stencil, so it must degrade to a warning rather than a failed run.
func TestCompose_FrictionDirective(t *testing.T) {
	t.Parallel()
	const (
		decisionRecordPath = "/hub/repo/_lyx/discussion/decision-record.md"
		supportLogPath     = "/hub/repo/_lyx/discussion/support-log.md"
	)
	composers := []struct {
		name         string
		producerRow  string
		role         friction.Role
		templateFile string
		templateBody []byte
		compose      func(t *testing.T, stencilsDir, frictionDirective string) (string, error)
	}{
		{
			name:         "discussion",
			producerRow:  "Discussion-Write",
			role:         friction.RoleInterview,
			templateFile: "loom-template-discussion.md",
			templateBody: stencils.LoomTemplateDiscussion,
			compose: func(t *testing.T, stencilsDir, frictionDirective string) (string, error) {
				got, err := composePrompt(stencilsDir, "add-json-flag", decisionRecordPath, supportLogPath, "", frictionDirective, "", false)
				return string(got), err
			},
		},
		{
			name:         "plan",
			producerRow:  "Plan-Write",
			role:         friction.RoleImplementer,
			templateFile: "loom-template-plan.md",
			templateBody: stencils.LoomTemplatePlan,
			compose: func(t *testing.T, stencilsDir, frictionDirective string) (string, error) {
				got, err := composePlanPrompt(stencilsDir, newTestSpecsDir(t), decisionRecordPath, "/hub/repo/_lyx/plan", "/hub/repo/_lyx/plan/00-overview.md", "", frictionDirective, "")
				return string(got), err
			},
		},
	}
	for _, c := range composers {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			t.Run("enabled", func(t *testing.T) {
				t.Parallel()
				stencilsDir := newTestStencilsDir(t)
				notePath := friction.NotePath(filepath.Join(t.TempDir(), "friction"), c.producerRow)
				if notePath == "" {
					t.Fatalf("friction.NotePath(frictionDir, %q) = \"\"; want a resolved path", c.producerRow)
				}
				directive, err := friction.Directive(notePath, stencilsDir, c.role)
				if err != nil {
					t.Fatalf("friction.Directive(...) = _, %v; want nil error", err)
				}

				got, err := c.compose(t, stencilsDir, directive)
				if err != nil {
					t.Fatalf("compose(..., directive) = _, %v; want nil error", err)
				}
				if !strings.Contains(got, notePath) {
					t.Errorf("compose(...) output does not contain the resolved friction note path %q verbatim", notePath)
				}
			})

			t.Run("disabled", func(t *testing.T) {
				t.Parallel()
				got, err := c.compose(t, newMinimalStencilsDir(t), "")
				if err != nil {
					t.Fatalf("compose(..., frictionDirective=\"\") = _, %v; want nil error", err)
				}
				if strings.Contains(got, "{{") {
					t.Errorf("compose(..., frictionDirective=\"\") output contains an unrendered marker token:\n%s", got)
				}
				if strings.Contains(got, "Friction note") {
					t.Error("compose(..., frictionDirective=\"\") output contains friction directive content; want none when Tier 2 is off")
				}
			})

			t.Run("marker-free template", func(t *testing.T) {
				t.Parallel()
				stencilsDir := newTestStencilsDir(t)
				markerFree := bytes.ReplaceAll(c.templateBody, []byte("{{.friction_directive}}"), nil)
				templatePath := filepath.Join(stencilsDir, "loom", c.templateFile)
				if err := os.WriteFile(templatePath, markerFree, 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", templatePath, err)
				}

				if _, err := c.compose(t, stencilsDir, "some friction directive text"); err != nil {
					t.Fatalf("compose(..., frictionDirective=<non-empty>) with a marker-free template = _, %v; want nil error", err)
				}
			})
		})
	}
}
