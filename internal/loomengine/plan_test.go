// plan_test.go — untagged Tier-1 unit tests for PlanSpec. Pure Go over an
// in-memory Config and a temp-dir modelspec registry; no live hub, reed, or
// network involved.

package loomengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

// TestPlanSpec verifies PlanSpec's field mapping against a hand-built
// Layout, an in-memory Config, and the built-in modelspec registry (no
// models.yaml present).
func TestPlanSpec(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	wantOutputFiles := []string{
		filepath.Join(worktreeRoot, "_lyx", "plan", "00-overview.md"),
	}
	wantTimeout := 120 * time.Minute

	spec, err := PlanSpec(layout, cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}

	if len(spec.OutputFiles) != len(wantOutputFiles) {
		t.Fatalf("PlanSpec(...).OutputFiles = %v; want %v", spec.OutputFiles, wantOutputFiles)
	}
	for i, want := range wantOutputFiles {
		if spec.OutputFiles[i] != want {
			t.Errorf("PlanSpec(...).OutputFiles[%d] = %q; want %q", i, spec.OutputFiles[i], want)
		}
	}
	if spec.Interactive != false {
		t.Errorf("PlanSpec(...).Interactive = %v; want false", spec.Interactive)
	}
	if spec.Role != "plan" {
		t.Errorf("PlanSpec(...).Role = %q; want %q", spec.Role, "plan")
	}
	if spec.Model == "" {
		t.Error("PlanSpec(...).Model = \"\"; want non-empty")
	}
	if spec.Effort != "high" {
		t.Errorf("PlanSpec(...).Effort = %q; want %q", spec.Effort, "high")
	}
	if spec.Timeout != wantTimeout {
		t.Errorf("PlanSpec(...).Timeout = %s; want %s", spec.Timeout, wantTimeout)
	}
	if spec.Prompt == "" {
		t.Error("PlanSpec(...).Prompt = \"\"; want non-empty")
	}
}

// TestPlanSpec_PromptFilled verifies the rendered prompt contains every
// resolved marker value and no leftover "{{", proving stencil.Fill filled
// every marker in plan-template.md rather than silently leaving one blank.
func TestPlanSpec_PromptFilled(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	spec, err := PlanSpec(layout, cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}

	decisionRecordPath := layout.DiscussionDecisionRecord()
	planDir := layout.PlanDir()
	overviewPath := layout.PlanOverview()

	for _, want := range []string{decisionRecordPath, planDir, overviewPath} {
		if !strings.Contains(spec.Prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q", want)
		}
	}
	if strings.Contains(spec.Prompt, "{{") {
		t.Error("PlanSpec(...).Prompt contains a leftover \"{{\" marker; want every marker filled")
	}
}

// TestPlanSpec_PatternDirectiveOptional proves pattern_directive behaves as
// an optional marker driven all the way through PlanSpec: an empty
// directive (the common case — PATTERN inactive) renders with no leftover
// "{{", no orphan "## Constraints" heading, and no stray blank-line block,
// while a non-empty directive (PATTERN active) appears ahead of "## Step
// 1". The two cases deliberately use DIFFERENT Layout fixtures. Every other
// test in this file builds its Layout from a path that never exists on
// disk (filepath.Join("home", "user", "repo")), which is fine for pure
// string-shape assertions — but pattern.Directive performs a real
// os.Stat on _pattern/PATTERN.md, so reusing that fake Layout here would
// always render the directive empty and the non-empty case's placement
// assertion would pass vacuously, proving nothing. The non-empty case
// instead builds its Layout on a t.TempDir() with a real _pattern/PATTERN.md
// seeded on disk. This is the one test in this file that touches the
// filesystem — cards 24 through 27 inject pattern_directive directly as a
// stencil value and never stat anything, since their templates are
// exercised through stencil.FillOptional rather than through a
// Layout-taking entry point; PlanSpec is Layout-taking, so this test is
// the one place in the whole batch that must actually exercise
// pattern.Directive's own os.Stat. t.TempDir() is not a banned token under
// the Test Tier Purity Invariant, so this file stays untagged.
func TestPlanSpec_PatternDirectiveOptional(t *testing.T) {
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	t.Run("empty pattern_directive (PATTERN inactive) renders cleanly", func(t *testing.T) {
		// filepath.Join("home", "user", "repo") never exists on disk, so
		// pattern.Directive's os.Stat always resolves "not exist" here —
		// PATTERN is inactive by construction.
		layout := &hubgeometry.Layout{WorktreeRoot: filepath.Join("home", "user", "repo")}

		reg, err := modelspec.LoadRegistry(t.TempDir())
		if err != nil {
			t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
		}
		spec, err := PlanSpec(layout, cfg, reg)
		if err != nil {
			t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
		}

		prompt := spec.Prompt
		if strings.Contains(prompt, "{{") {
			t.Errorf("PlanSpec(...).Prompt contains a leftover {{: %q", prompt)
		}
		if strings.Contains(prompt, "## Constraints") {
			t.Errorf("PlanSpec(...).Prompt contains an orphan ## Constraints heading: %q", prompt)
		}
		if strings.Contains(prompt, "\n\n\n\n") {
			t.Errorf("PlanSpec(...).Prompt contains a stray blank-line block: %q", prompt)
		}
	})

	t.Run("non-empty pattern_directive (PATTERN active) precedes Step 1", func(t *testing.T) {
		worktreeRoot := t.TempDir()
		patternDir := filepath.Join(worktreeRoot, "_pattern")
		if err := os.MkdirAll(patternDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", patternDir, err)
		}
		if err := os.WriteFile(filepath.Join(patternDir, "PATTERN.md"), []byte("# PATTERN\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
		}
		layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}

		reg, err := modelspec.LoadRegistry(t.TempDir())
		if err != nil {
			t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
		}
		spec, err := PlanSpec(layout, cfg, reg)
		if err != nil {
			t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
		}

		prompt := spec.Prompt
		directiveIdx := strings.Index(prompt, "## Constraints")
		stepIdx := strings.Index(prompt, "## Step 1")
		if directiveIdx == -1 || stepIdx == -1 || directiveIdx >= stepIdx {
			t.Errorf("pattern_directive (idx %d) does not precede ## Step 1 (idx %d) in prompt: %q", directiveIdx, stepIdx, prompt)
		}
	})
}

// TestPlanSpec_PromptStatesCardCriteria verifies the rendered prompt
// carries plan-format-v3's card-granularity contract ("What a card is"),
// not just the field format: a live run against a template without these
// criteria produced a card introducing new behavior with no bundled test
// (proven live, round fable-r1), so their presence is pinned here.
func TestPlanSpec_PromptStatesCardCriteria(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"What a card is",
		"smallest change",
		"independently committable",
		"Bundles its own test",
		"not a substitute for a bundled test",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the card-granularity contract must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesRootResolution verifies the rendered prompt
// explains the frontmatter `root:` key it advertises — the `<root>/<path>`
// join and the `//` worktree-root escape — rather than naming the key with
// no resolution rules (an agent electing to set `root:` could not otherwise
// write conformant escaped paths; see docs/reference/plan-format-v3.md's
// "Card path resolution" section).
func TestPlanSpec_PromptStatesRootResolution(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"`root:` is optional",
		"`<root>/<path>`",
		"worktree-root-relative",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; root:/// resolution rules must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesContextSemantics verifies the rendered prompt
// defines `Context:` (read-but-not-change, advisory) and the per-card
// field mutual-exclusivity rule, matching plan-format-v3.md's
// card-field-overlap contract — a template that names the five fields
// without their semantics leaves an agent free to misuse them.
func TestPlanSpec_PromptStatesContextSemantics(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"read but not change",
		"ONE of the five fields",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; Context:/exclusivity semantics must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesMoveRedundantRule verifies the rendered prompt
// carries plan-format-v3.md's move-redundant rule (a Moves: endpoint never
// also in Creates:/Deletes: of the same plan) and the rename-plus-extraction
// shape (one Moves: pair plus a separate Creates: entry).
func TestPlanSpec_PromptStatesMoveRedundantRule(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"must not also appear",
		"split-out file is a separate plain `Creates:` entry",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the move-redundant rule must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesVerifyIsRunnable verifies the rendered prompt
// pins verify: values to runnable shell commands: a live run against a
// template without this clause produced a plan-level `## verify:` of prose
// acceptance criteria a mechanical consumer cannot run (proven live, round
// fable-r1).
func TestPlanSpec_PromptStatesVerifyIsRunnable(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"runnable shell commands",
		"never prose",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the runnable-verify rule must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesMovedFileNotInEdits verifies the rendered prompt
// reconciles the Rename mechanic's "make surgical edits to the moved file"
// instruction with the per-card field-exclusivity rule: the moved file's
// surgical edits are covered by its Moves: entry, so a moved file must never
// also appear in the same card's Edits:. A live rename-plus-extraction run
// (round opus-r2) intermittently produced a card declaring the moved file in
// both Edits: and as its Moves: destination — a card-field-overlap the
// exclusivity sentence alone did not prevent under the mechanic's pull.
func TestPlanSpec_PromptStatesMovedFileNotInEdits(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"already declared by its `Moves:` entry",
		"either endpoint — in that same card's `Edits:`",
		"same card-field-overlap contradiction",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the moved-file-not-in-Edits rule must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesDependsOnCriterion verifies the rendered prompt
// states WHEN a card must declare a Depends-on: edge, not just the field's
// grammar: a live docs-only run (round fable-r3) produced a card whose
// Context: named a file an earlier card Creates: while declaring
// "Depends-on: none" — an under-declared DAG-of-intent edge no mechanical
// plan-format-v3 check can catch (check 12 tolerates the cross-card path
// reference; check 14 only validates edges that exist), so the criterion
// must reach the agent through the template.
func TestPlanSpec_PromptStatesDependsOnCriterion(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"what depends on what",
		"not compile-visible",
		"other card were dropped",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the Depends-on declaration criterion must reach the agent", want)
		}
	}
}

// renderedPlanPrompt returns the prompt PlanSpec renders for a hand-built
// Layout and the default in-memory Config, for template-content assertions.
func renderedPlanPrompt(t *testing.T) string {
	t.Helper()

	layout := &hubgeometry.Layout{WorktreeRoot: filepath.Join("home", "user", "repo")}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	spec, err := PlanSpec(layout, cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}
	return spec.Prompt
}

// TestPlanSpec_MalformedModelSpec verifies a Config with an ungrammatical
// plan model-spec yields a non-nil error rather than propagating the bad
// spec into a Spec.
func TestPlanSpec_MalformedModelSpec(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	cfg := Config{Plan: "opus[effort", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	if _, err := PlanSpec(layout, cfg, reg); err == nil {
		t.Fatal("PlanSpec(..., Plan=\"opus[effort\") = _, nil; want non-nil error")
	}
}
