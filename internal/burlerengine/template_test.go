// template_test.go pins that each shipped half-prompt asset fills through stencil with its required marker subset.
// It also pins that the explore step's two optional directives render empty and sit ahead of its first work instruction.
// It pins that the compose functions read a half prompt from disk at call time.
// prompt_test.go pins the Go-built blocks and the seat stencils' cluster-block exclusion in full compose renders.
// The assets are the exported embedded defaults of the top-level stencils package.

package burlerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// requireContains fails the test, naming the missing needle, if text does not contain it.
// The package's tests share it.
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// reviewerMarkerValues returns a values map with a placeholder for each required marker of the reviewer seat stencil.
func reviewerMarkerValues() map[string]string {
	return map[string]string{
		"instruction_1_path": "/tmp/instruction-1-explore.md",
		"instruction_2_path": "/tmp/instruction-2-review.md",
		"review_path":        "/tmp/review.md",
		"edit_directive":     "edit directive placeholder",
	}
}

// fixerMarkerValues returns a values map with a placeholder for each required marker of the fixer seat stencil.
func fixerMarkerValues() map[string]string {
	return map[string]string{
		"review_path":          "/tmp/review.md",
		"review_format_path":   "/tmp/review-format.md",
		"instruction_fix_path": "/tmp/instruction-fix.md",
		"edit_directive":       "edit directive placeholder",
	}
}

// reviewStepMarkerValues returns a values map with a placeholder for each required marker of the review step.
func reviewStepMarkerValues() map[string]string {
	return map[string]string{
		"cluster_rules":      "cluster-rules placeholder",
		"review_path":        "/tmp/review.md",
		"review_format_path": "/tmp/review-format.md",
		"prior_rounds":       "prior-rounds placeholder",
	}
}

// fixStepMarkerValues returns a values map with a placeholder for each top-level marker of the fix step.
// It includes the optional markers friction_directive and self_check.
func fixStepMarkerValues() map[string]string {
	return map[string]string{
		"friction_directive": "## Friction note placeholder",
		"self_check":         "## Self-check placeholder\n\n",
		"work":               "work placeholder",
		"fix_scope_rules":    "fix-scope placeholder",
		"review_path":        "/tmp/review.md",
		"fixer_report_path":  "/tmp/fixer-report.md",
	}
}

// instruction1MarkerValues returns a values map with a placeholder for each marker of the explore step.
// The optional markers pattern_directive, focus_directive and fork_reading are included too.
// Tests delete one key at a time to prove the per-marker error of stencil.FillOptional.
func instruction1MarkerValues() map[string]string {
	return map[string]string{
		"pattern_directive": "## Constraints — do this before you judge anything\n\n- Read the overview.",
		"focus_directive":   "## Focus directive for this round",
		"fork_reading":      "\n\n## Reading for the fork reviewers",
		"target":            "target placeholder",
		"fasit":             "fasit placeholder",
		"rubric":            "rubric placeholder",
		"tool_use_rules":    "tool-use placeholder",
	}
}

// TestTemplate_FillsWithAllMarkers asserts each embedded asset fills through stencil when given its full marker set.
// For the explore step that set includes the optional pattern_directive and focus_directive.
// The test fails, naming the marker, when any single required marker of an asset is absent.
// pattern_directive and focus_directive are excluded from the explore step's deletion sweep.
// They are optional markers, so deleting either must not error.
func TestTemplate_FillsWithAllMarkers(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name            string
		template        []byte
		values          map[string]string
		optional        []string
		requiredMarkers []string
	}{
		{
			name:            "reviewer seat",
			template:        stencils.BurlerTemplateReviewer,
			values:          reviewerMarkerValues(),
			optional:        []string{"parent_directive"},
			requiredMarkers: []string{"instruction_1_path", "instruction_2_path", "review_path"},
		},
		{
			name:            "fixer seat",
			template:        stencils.BurlerTemplateFixer,
			values:          fixerMarkerValues(),
			optional:        []string{"parent_directive"},
			requiredMarkers: []string{"review_path", "review_format_path", "instruction_fix_path"},
		},
		{
			name:            "review step",
			template:        stencils.BurlerStepReview,
			values:          reviewStepMarkerValues(),
			optional:        []string{assistRulesMarker, parentRulesMarker},
			requiredMarkers: []string{"cluster_rules", "review_path", "review_format_path", "prior_rounds"},
		},
		{
			name:            "fix step",
			template:        stencils.BurlerStepFix,
			values:          fixStepMarkerValues(),
			optional:        []string{"friction_directive", "self_check"},
			requiredMarkers: []string{"work", "fix_scope_rules", "review_path", "fixer_report_path"},
		},
		{
			name:     "review format",
			template: stencils.BurlerReviewFormat,
			values:   map[string]string{},
		},
		{
			name:            "explore step",
			template:        stencils.BurlerStep1Explore,
			values:          instruction1MarkerValues(),
			optional:        []string{"pattern_directive", "focus_directive", "fork_reading"},
			requiredMarkers: []string{"target", "fasit", "rubric", "tool_use_rules"},
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
					// Copy the shared values map before deleting from it.
					// Every subtest in the asset's table entry shares tt.values.
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

// TestTemplate_OptionalDirectives asserts the explore step's optional markers render cleanly when empty.
// They are pattern_directive, focus_directive and fork_reading.
// An empty value leaves no leftover `{{`, no orphan heading and, for pattern_directive, no stray blank-line block.
// A non-empty value places the two directives ahead of the first work instruction.
// It places the fork reading after the tool-use rules at the end of the stencil.
// The fix step's friction_directive and self_check each render an empty value cleanly too.
func TestTemplate_OptionalDirectives(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	optional := []string{"pattern_directive", "focus_directive", "fork_reading"}
	tests := []struct {
		marker            string
		orphanHeading     string
		noStrayBlankBlock bool
		placement         string
		wantBefore        bool
		anchor            string
	}{
		{marker: "pattern_directive", orphanHeading: "## Constraints", noStrayBlankBlock: true, placement: "precedes the first work instruction", wantBefore: true, anchor: "## What to review (the target)"},
		{marker: "focus_directive", orphanHeading: "## Focus directive", placement: "precedes the first work instruction", wantBefore: true, anchor: "## What to review (the target)"},
		{marker: "fork_reading", orphanHeading: "## Reading for the fork reviewers", placement: "follows the tool-use rules", anchor: "## Tool-use rules — how you gather evidence"},
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

			t.Run("non-empty value is placed", func(t *testing.T) {
				t.Parallel()
				values := instruction1MarkerValues()
				got, err := stencil.FillOptional(stencils.BurlerStep1Explore, values, optional)
				if err != nil {
					t.Fatalf("stencil.FillOptional() = %v; want nil", err)
				}
				text := string(got)
				valueIdx := strings.Index(text, strings.TrimSpace(values[tt.marker]))
				anchorIdx := strings.Index(text, tt.anchor)
				if valueIdx == -1 || anchorIdx == -1 || (valueIdx < anchorIdx) != tt.wantBefore {
					t.Errorf("%s (idx %d) does not satisfy %q against %q (idx %d)", tt.marker, valueIdx, tt.placement, tt.anchor, anchorIdx)
				}
			})
		})
	}

	t.Run("fix step renders an empty optional marker cleanly", func(t *testing.T) {
		t.Parallel()
		for _, marker := range []string{"friction_directive", "self_check"} {
			values := fixStepMarkerValues()
			values[marker] = ""
			got, err := stencil.FillOptional(stencils.BurlerStepFix, values, []string{"friction_directive", "self_check"})
			if err != nil {
				t.Fatalf("stencil.FillOptional() with %s empty = %v; want nil", marker, err)
			}
			if strings.Contains(string(got), "{{") {
				t.Errorf("rendered output with %s empty contains leftover {{: %q", marker, got)
			}
		}
	})

}

// TestComposePrompt_ReadsEditedStencilFromDisk proves the review prompt reads its stencil from disk.
// It is not taken from a compiled-in default.
// Overwriting burler/burler-step-review.md in stencilsDir with a modified body must reach the composed review step.
//
//testtiming:keep pins that an edited on-disk review-step body reaches the composed review step file, which the cluster test covering its blocks never edits
func TestComposePrompt_ReadsEditedStencilFromDisk(t *testing.T) {
	tiergate.Require(t, 0)
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	const modifiedMarker = "MODIFIED-BY-TEST: this line does not exist in the shipped default"
	edited := append(append([]byte{}, stencils.BurlerStepReview...), []byte("\n"+modifiedMarker+"\n")...)
	path := filepath.Join(stencilsDir, "burler", "burler-step-review.md")
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}

	prompt := composeReview(t, stencilsDir, &p, "")

	requireContains(t, prompt.Files[reviewStepFile].Content, modifiedMarker)
}

// TestComposePrompt_RendersEngineBlocksForTheHalfsEngine checks both halves.
// An engine block that an instruction stencil includes renders the copy of the engine the half runs on.
func TestComposePrompt_RendersEngineBlocksForTheHalfsEngine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	stencilsDir := newTestStencilsDir(t)
	seedEngineWords(t, stencilsDir, map[string][]byte{
		"burler-step-1-explore": stencils.BurlerStep1Explore,
		"burler-step-review":    stencils.BurlerStepReview,
		"burler-step-fix":       stencils.BurlerStepFix,
	})

	tests := []struct {
		engine string
		want   string
		absent string
	}{
		{"claude", "Claude words.", "Codex words."},
		{"codex", "Codex words.", "Claude words."},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			t.Parallel()
			p := newComposableProfile(t)
			review, err := composeReviewPrompt(stencilsDir, testSpecsDir, tt.engine, &p, "", "", "", testReviewFiles)
			if err != nil {
				t.Fatalf("composeReviewPrompt(%s) = %v; want nil error", tt.engine, err)
			}
			fix, err := composeFixPrompt(stencilsDir, testSpecsDir, tt.engine, &p, "", fixWork(p.ReviewPath, "", false), testFixFiles)
			if err != nil {
				t.Fatalf("composeFixPrompt(%s) = %v; want nil error", tt.engine, err)
			}
			for file, content := range map[string]string{"explore step": review.Files[reviewExploreFile].Content, "review step": review.Files[reviewStepFile].Content, "fix step": fix.Files[fixStepFile].Content} {
				if !strings.Contains(content, tt.want) || strings.Contains(content, tt.absent) {
					t.Errorf("%s for engine %s = %q; want it to hold %q and not %q", file, tt.engine, content, tt.want, tt.absent)
				}
			}
		})
	}
}
