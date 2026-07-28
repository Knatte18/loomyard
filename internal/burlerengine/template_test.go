// template_test.go is the machine half of the Review Round Invariant
// (CONSTRAINTS.md): it pins the embedded prompt template's load-bearing
// round-discipline statements as substring assertions, and separately
// proves the template actually fills through stencil with all nine
// required markers.

package burlerengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// TestTemplate_StatesRoundDiscipline asserts the embedded template's bytes
// carry the load-bearing round-discipline phrases in prose, so an edit that
// silently waters down the sequencing rule or the fix-everything rule fails
// this test rather than only a human review.
func TestTemplate_StatesRoundDiscipline(t *testing.T) {
	text := string(reviewPromptTemplate)

	requireContains(t, text, "Sequencing rule")
	requireContains(t, text, "fully written to")
	requireContains(t, text, "before you touch")
	requireContains(t, text, "not whether it gets fixed")
	requireContains(t, text, "never push")
	requireContains(t, text, "nothing fixed")
	requireContains(t, text, "origin")
}

// TestTemplate_HasClusterRulesSection asserts the embedded template's
// static bytes carry the new "## Cluster rules" section and its
// {{.cluster_rules}} marker, so an edit that drops the section heading or
// renames the marker fails this test rather than only a human review.
func TestTemplate_HasClusterRulesSection(t *testing.T) {
	text := string(reviewPromptTemplate)

	requireContains(t, text, "Cluster rules")
	requireContains(t, text, "{{.cluster_rules}}")
}

// TestTemplate_StatesClusterForkDiscipline pins the cluster round's
// load-bearing fork-discipline statements — the single-message fork spawn,
// the unnamed-fork rule, the fork's read-only/no-git discipline, that
// consolidation happens before job B, the origin labels, and the Rejected
// section — following this file's existing pin style but sourced from a
// full composePrompt render for a cluster profile rather than the static
// template bytes: this content is composed dynamically by
// clusterRulesBlock (prompt.go), not baked into
// review-prompt-template.md itself. An edit that silently waters any of
// these statements down fails this test rather than only a human review.
func TestTemplate_StatesClusterForkDiscipline(t *testing.T) {
	p := newComposableProfile(t)
	p.ClusterFan = "standard"
	p.clusterLenses = []Lens{
		{Name: "style", Text: "pay extra attention to style"},
	}

	got, err := composePrompt(&p, "")
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}

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

// requireContains fails the test, naming the missing needle, if text does
// not contain it. Shared across this package's tests (prompt_test.go
// reuses it for composePrompt's rendered output).
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// allMarkerValues returns a values map with every one of the template's
// nine required top-level markers set to a non-empty placeholder, plus
// pattern_directive — the one optional marker, filled via
// stencil.FillOptional — set to a placeholder too, so tests can delete one
// key at a time to prove stencil.FillOptional's per-marker error.
func allMarkerValues() map[string]string {
	return map[string]string{
		"target":            "target placeholder",
		"fasit":             "fasit placeholder",
		"rubric":            "rubric placeholder",
		"fix_scope_rules":   "fix-scope placeholder",
		"tool_use_rules":    "tool-use placeholder",
		"prior_rounds":      "prior-rounds placeholder",
		"cluster_rules":     "cluster-rules placeholder",
		"review_path":       "/tmp/review.md",
		"fixer_report_path": "/tmp/fixer-report.md",
		"pattern_directive": "## Constraints — do this before you judge or change anything\n\n- Read _pattern/PATTERN.md.",
	}
}

// TestTemplate_FillsWithAllMarkers asserts stencil.FillOptional succeeds
// when every one of the nine required markers plus the optional
// pattern_directive marker is supplied, and fails — naming the marker —
// when any single REQUIRED one is absent. pattern_directive is
// deliberately excluded from this deletion sweep: it is the one optional
// marker (see the template's own banner comment), so deleting it must not
// error.
func TestTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.FillOptional(reviewPromptTemplate, allMarkerValues(), []string{"pattern_directive"}); err != nil {
			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
		}
	})

	for _, marker := range []string{
		"target", "fasit", "rubric", "fix_scope_rules", "tool_use_rules",
		"prior_rounds", "cluster_rules", "review_path", "fixer_report_path",
	} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := allMarkerValues()
			delete(values, marker)
			_, err := stencil.FillOptional(reviewPromptTemplate, values, []string{"pattern_directive"})
			if err == nil {
				t.Fatalf("stencil.FillOptional() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.FillOptional() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestTemplate_PatternDirectiveOptional asserts pattern_directive behaves
// as an optional marker: an empty value renders cleanly with no leftover
// `{{`, no orphan `## Constraints` heading, and no stray blank-line block
// where the directive would have sat, and a non-empty value places the
// directive block ahead of the first work instruction ("## What to review
// (the target)").
func TestTemplate_PatternDirectiveOptional(t *testing.T) {
	t.Run("empty pattern_directive renders cleanly", func(t *testing.T) {
		values := allMarkerValues()
		values["pattern_directive"] = ""
		got, err := stencil.FillOptional(reviewPromptTemplate, values, []string{"pattern_directive"})
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
		values := allMarkerValues()
		got, err := stencil.FillOptional(reviewPromptTemplate, values, []string{"pattern_directive"})
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
