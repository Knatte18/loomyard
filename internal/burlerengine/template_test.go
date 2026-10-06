// template_test.go proves each of the four shipped round-prompt assets actually fills through stencil with its own required marker subset, that the three optional directives of instruction 1 render cleanly empty and placed ahead of its first work instruction, and that composePrompt reads a round prompt from disk at call time.
// The assets' load-bearing statements and the orchestrator's exclusion of downstream bodies are pinned from a full composePrompt render in prompt_test.go (PATTERN-review-round's machine half).
// The four assets are read from the top-level stencils package's exported embedded defaults (stencils.BurlerTemplateRoundOrchestrator etc.) rather than this package's own now-deleted package-private vars — a cross-package import, not a rename, since composePrompt itself reads its four prompts from disk at call time via stencilstore.Read (see prompt.go).

package burlerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// requireContains fails the test, naming the missing needle, if text does
// not contain it. Shared across this package's tests (prompt_test.go
// reuses it for composePrompt's rendered output).
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// orchestratorMarkerValues returns a values map with every one of the
// orchestrator's four required top-level markers set to a non-empty
// placeholder.
func orchestratorMarkerValues() map[string]string {
	return map[string]string{
		"instruction_1_path": "/tmp/instruction-1-explore.md",
		"instruction_2_path": "/tmp/instruction-2-review.md",
		"instruction_3_path": "/tmp/instruction-3-fix.md",
		"review_path":        "/tmp/review.md",
	}
}

// instruction1MarkerValues returns a values map with every one of instruction 1's four required top-level markers set to a non-empty placeholder, plus pattern_directive, friction_directive and focus_directive — the three optional markers, filled via stencil.FillOptional — set to a placeholder too, so tests can delete one key at a time to prove stencil.FillOptional's per-marker error.
func instruction1MarkerValues() map[string]string {
	return map[string]string{
		"pattern_directive":  "## Constraints — do this before you judge or change anything\n\n- Read the overview.",
		"friction_directive": "## Friction note — optional, only if something went wrong",
		"focus_directive":    "## Focus directive for this round",
		"target":             "target placeholder",
		"fasit":              "fasit placeholder",
		"rubric":             "rubric placeholder",
		"tool_use_rules":     "tool-use placeholder",
	}
}

// instruction2MarkerValues returns a values map with every one of
// instruction 2's three required top-level markers set to a non-empty
// placeholder.
func instruction2MarkerValues() map[string]string {
	return map[string]string{
		"cluster_rules": "cluster-rules placeholder",
		"review_path":   "/tmp/review.md",
		"prior_rounds":  "prior-rounds placeholder",
	}
}

// instruction3MarkerValues returns a values map with every one of
// instruction 3's three required top-level markers set to a non-empty
// placeholder.
func instruction3MarkerValues() map[string]string {
	return map[string]string{
		"fix_scope_rules":   "fix-scope placeholder",
		"review_path":       "/tmp/review.md",
		"fixer_report_path": "/tmp/fixer-report.md",
	}
}

// TestTemplate_FillsWithAllMarkers asserts each of the four embedded assets fills through stencil when supplied its own full marker set (required markers plus, for instruction 1, the optional pattern_directive, friction_directive and focus_directive), and fails — naming the marker — when any single REQUIRED marker for that asset is absent.
// pattern_directive, friction_directive and focus_directive are deliberately excluded from instruction 1's deletion sweep:
// they are the optional markers across all four assets, so deleting any of them must not error.
func TestTemplate_FillsWithAllMarkers(t *testing.T) {
	tests := []struct {
		name            string
		template        []byte
		values          map[string]string
		optional        []string
		requiredMarkers []string
	}{
		{
			name:            "orchestrator",
			template:        stencils.BurlerTemplateRoundOrchestrator,
			values:          orchestratorMarkerValues(),
			optional:        []string{"parent_directive"},
			requiredMarkers: []string{"instruction_1_path", "instruction_2_path", "instruction_3_path", "review_path"},
		},
		{
			name:            "instruction 1 (explore)",
			template:        stencils.BurlerStep1Explore,
			values:          instruction1MarkerValues(),
			optional:        []string{"pattern_directive", "friction_directive", "focus_directive"},
			requiredMarkers: []string{"target", "fasit", "rubric", "tool_use_rules"},
		},
		{
			name:            "instruction 2 (review)",
			template:        stencils.BurlerStep2Review,
			values:          instruction2MarkerValues(),
			requiredMarkers: []string{"cluster_rules", "review_path", "prior_rounds"},
		},
		{
			name:            "instruction 3 (fix)",
			template:        stencils.BurlerStep3Fix,
			values:          instruction3MarkerValues(),
			requiredMarkers: []string{"fix_scope_rules", "review_path", "fixer_report_path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("all markers supplied", func(t *testing.T) {
				if _, err := stencil.FillOptional(tt.template, tt.values, tt.optional); err != nil {
					t.Fatalf("stencil.FillOptional() = %v; want nil", err)
				}
			})

			for _, marker := range tt.requiredMarkers {
				t.Run("missing "+marker, func(t *testing.T) {
					// Copy the shared values map before deleting from it —
					// the same tt.values map backs every subtest in this
					// asset's table entry.
					values := make(map[string]string, len(tt.values))
					for k, v := range tt.values {
						values[k] = v
					}
					delete(values, marker)

					_, err := stencil.FillOptional(tt.template, values, tt.optional)
					if err == nil {
						t.Fatalf("stencil.FillOptional() with %q missing = nil error; want error naming the marker", marker)
					}
					if !strings.Contains(err.Error(), marker) {
						t.Errorf("stencil.FillOptional() error = %q; want it to name marker %q", err.Error(), marker)
					}
				})
			}
		})
	}
}

// TestTemplate_OptionalDirectives asserts pattern_directive, friction_directive and focus_directive each behave as an optional marker on instruction 1: an empty value renders cleanly with no leftover `{{`, no orphan heading and, for pattern_directive, no stray blank-line block where the directive would have sat, and a non-empty value places the directive block ahead of the first work instruction ("## What to review (the target)").
// A marker-free template (the literal {{.friction_directive}} stripped from the shipped bytes) still fills cleanly while a non-empty directive value is supplied -- the composer's optional-marker guarantee runs one direction only (see stencil.FillOptional's own doc comment), so a marker's absence from the template must never be an error.
func TestTemplate_OptionalDirectives(t *testing.T) {
	t.Parallel()

	optional := []string{"pattern_directive", "friction_directive", "focus_directive"}
	tests := []struct {
		marker            string
		orphanHeading     string
		noStrayBlankBlock bool
	}{
		{marker: "pattern_directive", orphanHeading: "## Constraints", noStrayBlankBlock: true},
		{marker: "friction_directive"},
		{marker: "focus_directive", orphanHeading: "## Focus directive"},
	}

	for _, tt := range tests {
		t.Run(tt.marker, func(t *testing.T) {
			t.Parallel()

			t.Run("empty value renders cleanly", func(t *testing.T) {
				t.Parallel()
				values := instruction1MarkerValues()
				values[tt.marker] = ""
				got, err := stencil.FillOptional(stencils.BurlerStep1Explore, values, optional)
				if err != nil {
					t.Fatalf("stencil.FillOptional() = %v; want nil", err)
				}
				text := string(got)
				if strings.Contains(text, "{{") {
					t.Errorf("rendered output contains leftover {{: %q", text)
				}
				if tt.orphanHeading != "" && strings.Contains(text, tt.orphanHeading) {
					t.Errorf("rendered output contains an orphan %s heading: %q", tt.orphanHeading, text)
				}
				if tt.noStrayBlankBlock && strings.Contains(text, "\n\n\n\n") {
					t.Errorf("rendered output contains a stray blank-line block: %q", text)
				}
			})

			t.Run("non-empty value precedes the first work instruction", func(t *testing.T) {
				t.Parallel()
				values := instruction1MarkerValues()
				got, err := stencil.FillOptional(stencils.BurlerStep1Explore, values, optional)
				if err != nil {
					t.Fatalf("stencil.FillOptional() = %v; want nil", err)
				}
				text := string(got)
				directiveIdx := strings.Index(text, values[tt.marker])
				workIdx := strings.Index(text, "## What to review (the target)")
				if directiveIdx == -1 || workIdx == -1 || directiveIdx >= workIdx {
					t.Errorf("%s (idx %d) does not precede the first work instruction (idx %d)", tt.marker, directiveIdx, workIdx)
				}
			})
		})
	}

	t.Run("marker-free template still fills while a directive value is supplied", func(t *testing.T) {
		t.Parallel()
		markerFree := strings.ReplaceAll(string(stencils.BurlerStep1Explore), "{{.friction_directive}}", "")
		values := instruction1MarkerValues()
		if _, err := stencil.FillOptional([]byte(markerFree), values, optional); err != nil {
			t.Fatalf("stencil.FillOptional() on a marker-free template = %v; want nil", err)
		}
	})
}

// TestComposePrompt_ReadsEditedStencilFromDisk proves composePrompt reads a round prompt from stencilsDir on every call rather than from any compiled-in default: overwriting burler/burler-step-2-review.md on disk with a modified body, after building stencilsDir from the shipped defaults, must have that modified text — not the shipped default's own text — reach the composed instruction 2 file.
// This pins the runtime-read-not-embed Shared Decision at the burlerengine call site.
//
//testtiming:keep pins that an edited on-disk instruction-2 body reaches the composed instruction 2 file, which the cluster test covering its blocks never edits
func TestComposePrompt_ReadsEditedStencilFromDisk(t *testing.T) {
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	const modifiedMarker = "MODIFIED-BY-TEST: this line does not exist in the shipped default"
	edited := append(append([]byte{}, stencils.BurlerStep2Review...), []byte("\n"+modifiedMarker+"\n")...)
	path := filepath.Join(stencilsDir, "burler", "burler-step-2-review.md")
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}

	_, files, err := composePrompt(stencilsDir, "", &p, "", "", "/tmp/instruction-1-explore.md", "/tmp/instruction-2-review.md", "/tmp/instruction-3-fix.md")
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}

	requireContains(t, files[1].Content, modifiedMarker)
}
