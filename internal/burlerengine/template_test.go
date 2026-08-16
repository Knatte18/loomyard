// template_test.go is the machine half of the Review Round Invariant (CONSTRAINTS.md): it pins each
// of the four shipped round-prompt assets' load-bearing statements as substring assertions — the
// orchestrator's sequencing statements, instruction 3's fix-everything/ never-push statements,
// instruction 2's cluster/origin statements — it proves each asset actually fills through stencil
// with its own required marker subset, and it guards that the orchestrator never carries a
// downstream instruction body back into itself.
// The four assets are read from the top-level stencils package's exported embedded defaults
// (stencils.BurlerTemplateRoundOrchestrator etc.) rather than this package's own now-deleted
// package-private vars — a cross-package import, not a rename, since composePrompt itself reads its
// four prompts from disk at call time via stencilstore.Read (see prompt.go).

package burlerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// TestTemplate_StatesRoundDiscipline asserts each asset's bytes carry the load-bearing
// round-discipline phrases in prose, so an edit that silently waters down the sequencing rule or
// the fix-everything rule fails this test rather than only a human review.
func TestTemplate_StatesRoundDiscipline(t *testing.T) {
	orchestrator := string(stencils.BurlerTemplateRoundOrchestrator)
	requireContains(t, orchestrator, "Sequencing rule")
	requireContains(t, orchestrator, "fully written to")
	requireContains(t, orchestrator, "before you touch")

	instruction3 := string(stencils.BurlerStep3Fix)
	requireContains(t, instruction3, "not whether it gets fixed")
	requireContains(t, instruction3, "never push")
	requireContains(t, instruction3, "nothing fixed")

	instruction2 := string(stencils.BurlerStep2Review)
	requireContains(t, instruction2, "origin")
}

// TestTemplate_HasClusterRulesSection asserts instruction 2's static bytes carry the "## Cluster
// rules" section and its {{.cluster_rules}} marker, so an edit that drops the section heading or
// renames the marker fails this test rather than only a human review.
func TestTemplate_HasClusterRulesSection(t *testing.T) {
	text := string(stencils.BurlerStep2Review)

	requireContains(t, text, "Cluster rules")
	requireContains(t, text, "{{.cluster_rules}}")
}

// TestTemplate_StatesClusterForkDiscipline pins the cluster round's load-bearing fork-discipline
// statements — the single-message fork spawn, the unnamed-fork rule, the fork's read-only/no-git
// discipline, that consolidation happens before job B, the origin labels, and the Rejected section
// — following this file's existing pin style but sourced from a full composePrompt render for a
// cluster profile rather than the static template bytes: this content is composed dynamically by
// clusterRulesBlock (prompt.go) into instruction 2, not baked into burler-step-2-review.md
// itself.
// An edit that silently waters any of these statements down fails this test rather than only a
// human review.
func TestTemplate_StatesClusterForkDiscipline(t *testing.T) {
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)
	p.ClusterFan = "standard"
	p.clusterLenses = []Lens{
		{Name: "style", Text: "pay extra attention to style"},
	}

	_, files, err := composePrompt(stencilsDir, &p, "", "/tmp/instruction-1-explore.md", "/tmp/instruction-2-review.md", "/tmp/instruction-3-fix.md")
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}

	got := files[1].Content

	requireContains(t, got, "SINGLE message")
	requireContains(t, got, "subagent_type")
	requireContains(t, got, "never pass a `name`")
	requireContains(t, got, "READ-ONLY")
	requireContains(t, got, "never run any git command")
	requireContains(t, got, "never call the Agent tool")
	requireContains(t, got, "HOLISTIC")
	requireContains(t, got, "before job B touches anything")
	requireContains(t, got, "origin:")
	requireContains(t, got, "Rejected")
}

// TestTemplate_OrchestratorExcludesDownstreamBodies guards that the orchestrator handed to the
// shuttle never carries a downstream instruction body back into itself.
// The five tokens below are disjoint from the orchestrator's retained two-jobs framing (including
// the retained job-B one-liner, "even if the verdict was APPROVED — non-blocking polish still gets
// fixed"): each appears only inside a downstream instruction file's body (instruction 3's
// fix-everything prose, instruction 2's review-file YAML keys and cluster fork-spawn prose), so a
// regression that inlines a downstream body back into the orchestrator trips this guard on the
// first offending token, without colliding with the orchestrator's legitimate bare-word "verdict"/
// "findings" usage.
func TestTemplate_OrchestratorExcludesDownstreamBodies(t *testing.T) {
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	orchestrator, _, err := composePrompt(stencilsDir, &p, "", "/tmp/instruction-1-explore.md", "/tmp/instruction-2-review.md", "/tmp/instruction-3-fix.md")
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}

	requireNotContains(t, orchestrator, "not whether it gets fixed")
	requireNotContains(t, orchestrator, "verdict:")
	requireNotContains(t, orchestrator, "findings:")
	requireNotContains(t, orchestrator, "SINGLE message")
	requireNotContains(t, orchestrator, "subagent_type")
}

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

// instruction1MarkerValues returns a values map with every one of
// instruction 1's four required top-level markers set to a non-empty
// placeholder, plus pattern_directive — the one optional marker, filled
// via stencil.FillOptional — set to a placeholder too, so tests can delete
// one key at a time to prove stencil.FillOptional's per-marker error.
func instruction1MarkerValues() map[string]string {
	return map[string]string{
		"pattern_directive": "## Constraints — do this before you judge or change anything\n\n- Read _lyx/PATTERN.md.",
		"target":            "target placeholder",
		"fasit":             "fasit placeholder",
		"rubric":            "rubric placeholder",
		"tool_use_rules":    "tool-use placeholder",
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

// TestTemplate_FillsWithAllMarkers asserts each of the four embedded assets fills through stencil
// when supplied its own full marker set (required markers plus, for instruction 1, the optional
// pattern_directive), and fails — naming the marker — when any single REQUIRED marker for that
// asset is absent.
// pattern_directive is deliberately excluded from instruction 1's deletion sweep: it is the one
// optional marker across all four assets, so deleting it must not error.
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
			requiredMarkers: []string{"instruction_1_path", "instruction_2_path", "instruction_3_path", "review_path"},
		},
		{
			name:            "instruction 1 (explore)",
			template:        stencils.BurlerStep1Explore,
			values:          instruction1MarkerValues(),
			optional:        []string{"pattern_directive"},
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

// TestTemplate_PatternDirectiveOptional asserts pattern_directive behaves as an optional marker on
// instruction 1: an empty value renders cleanly with no leftover `{{`, no orphan `## Constraints`
// heading, and no stray blank-line block where the directive would have sat, and a non-empty value
// places the directive block ahead of the first work instruction ("## What to review (the
// target)").
func TestTemplate_PatternDirectiveOptional(t *testing.T) {
	t.Run("empty pattern_directive renders cleanly", func(t *testing.T) {
		values := instruction1MarkerValues()
		values["pattern_directive"] = ""
		got, err := stencil.FillOptional(stencils.BurlerStep1Explore, values, []string{"pattern_directive"})
		if err != nil {
			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
		}
		text := string(got)
		if strings.Contains(text, "{{") {
			t.Errorf("rendered output contains leftover {{: %q", text)
		}
		if strings.Contains(text, "## Constraints") {
			t.Errorf("rendered output contains an orphan ## Constraints heading: %q", text)
		}
		if strings.Contains(text, "\n\n\n\n") {
			t.Errorf("rendered output contains a stray blank-line block: %q", text)
		}
	})

	t.Run("non-empty pattern_directive precedes the first work instruction", func(t *testing.T) {
		values := instruction1MarkerValues()
		got, err := stencil.FillOptional(stencils.BurlerStep1Explore, values, []string{"pattern_directive"})
		if err != nil {
			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
		}
		text := string(got)
		directiveIdx := strings.Index(text, values["pattern_directive"])
		workIdx := strings.Index(text, "## What to review (the target)")
		if directiveIdx == -1 || workIdx == -1 || directiveIdx >= workIdx {
			t.Errorf("pattern_directive (idx %d) does not precede the first work instruction (idx %d)", directiveIdx, workIdx)
		}
	})
}

// TestComposePrompt_ReadsEditedStencilFromDisk proves composePrompt reads a round prompt from
// stencilsDir on every call rather than from any compiled-in default: overwriting
// burler/burler-step-2-review.md on disk with a modified body, after building stencilsDir from the
// shipped defaults, must have that modified text — not the shipped default's own text — reach the
// composed instruction 2 file. This pins the runtime-read-not-embed Shared Decision at the
// burlerengine call site.
func TestComposePrompt_ReadsEditedStencilFromDisk(t *testing.T) {
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	const modifiedMarker = "MODIFIED-BY-TEST: this line does not exist in the shipped default"
	edited := append(append([]byte{}, stencils.BurlerStep2Review...), []byte("\n"+modifiedMarker+"\n")...)
	path := filepath.Join(stencilsDir, "burler", "burler-step-2-review.md")
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}

	_, files, err := composePrompt(stencilsDir, &p, "", "/tmp/instruction-1-explore.md", "/tmp/instruction-2-review.md", "/tmp/instruction-3-fix.md")
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}

	requireContains(t, files[1].Content, modifiedMarker)
}
