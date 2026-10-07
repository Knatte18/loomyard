// prompt_test.go covers composePrompt's marker composition: the happy path fills every marker
// across all four rendered assets, each switched block (fix-scope, tool-use, prior-rounds,
// cluster-rules) renders the branch its Profile field selects and not the other branch's exclusive
// phrasing, and each block helper's content lands in its intended instruction file rather than
// leaking into an orchestrator or a sibling instruction file.
// It also declares newTestStencilsDir, the package-local test helper every burlerengine test uses to
// seed a hermetic stencils directory through `stencilkit`.

package burlerengine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

// Placeholder absolute instruction paths every composePrompt call in this
// file passes — composePrompt does no filesystem access on these beyond
// baking them into the orchestrators' marker values, so they need not
// exist on disk (Engine.Run, not composePrompt, writes the files there).
var testRoundFilePaths = roundFilePaths{
	ReviewerExplore: "/tmp/instruction-1-explore-reviewer.md",
	FixerExplore:    "/tmp/instruction-1-explore-fixer.md",
	Review:          "/tmp/instruction-2-review.md",
	Fix:             "/tmp/instruction-3-fix.md",
}

// newComposableProfile builds a minimal Profile whose paths already exist
// on disk (composePrompt's directory annotation stats them) and whose
// fields are all resolved absolute paths, as (*Profile).validate would
// leave them — composePrompt is documented to run only after validate.
func newComposableProfile(t *testing.T) Profile {
	t.Helper()
	root := t.TempDir()

	targetFile := filepath.Join(root, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target content"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) = %v; want nil", err)
	}
	targetDir := filepath.Join(root, "targetdir")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatalf("Mkdir(targetdir) = %v; want nil", err)
	}
	fasitFile := filepath.Join(root, "fasit.txt")
	if err := os.WriteFile(fasitFile, []byte("fasit content"), 0o644); err != nil {
		t.Fatalf("WriteFile(fasit) = %v; want nil", err)
	}

	return Profile{
		Target:          FileSet{Paths: []string{targetFile, targetDir}},
		Fasit:           FileSet{Paths: []string{fasitFile}},
		Rubric:          "the widget's color must match the housing's color",
		FixScope:        FixScopeSource,
		ToolUse:         false,
		ReviewPath:      filepath.Join(root, "review.md"),
		FixerReportPath: filepath.Join(root, "fixer-report.md"),
		ReadyMarkerPath: filepath.Join(root, "review.md.ready"),
	}
}

// combinedPrompt joins both orchestrator strings with every instruction file's Content, newline-separated.
// It gives tests a single haystack to search when a presence/absence assertion does not care which of the rendered assets carries the text.
func combinedPrompt(prompts roundPrompts) string {
	parts := make([]string, 0, len(prompts.Files)+2)
	parts = append(parts, prompts.Reviewer, prompts.Fixer)
	for _, f := range prompts.Files {
		parts = append(parts, f.Content)
	}
	return strings.Join(parts, "\n")
}

// The positions of the rendered instruction files in roundPrompts.Files.
const (
	reviewerExploreFile = iota
	fixerExploreFile
	reviewFile
	fixFile
)

// TestComposePrompt_FocusDirective proves a round's focus file reaches instruction 1 through its own channel, with the precedence rule, and never reaches instruction 2's prior-rounds block;
// and that a round without a directive renders no focus text and no marker residue.
func TestComposePrompt_FocusDirective(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)

	t.Run("directive present", func(t *testing.T) {
		p := newComposableProfile(t)
		p.FocusDirective = filepath.Join(t.TempDir(), "focus.md")
		if err := os.WriteFile(p.FocusDirective, []byte("look at the seam"), 0o644); err != nil {
			t.Fatalf("WriteFile(focus) = %v; want nil", err)
		}

		prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		for _, explore := range []int{reviewerExploreFile, fixerExploreFile} {
			requireContains(t, prompts.Files[explore].Content, p.FocusDirective)
			requireContains(t, prompts.Files[explore].Content, "The rubric binds over the focus directive")
		}
		if strings.Contains(prompts.Files[reviewFile].Content, p.FocusDirective) {
			t.Errorf("instruction 2 contains the focus path %q; want it absent", p.FocusDirective)
		}
	})

	t.Run("directive absent", func(t *testing.T) {
		p := newComposableProfile(t)

		prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := prompts.Files[reviewerExploreFile].Content
		if strings.Contains(got, "Focus directive for this round") {
			t.Errorf("instruction 1 contains the focus heading with no directive: %q", got)
		}
		if strings.Contains(got, "{{") || strings.Contains(got, "<no value>") {
			t.Errorf("instruction 1 contains marker residue: %q", got)
		}
	})

	// The warning subtests swap the process-global logger output, so none runs in parallel.
	focusWarningLines := func(out string) []string {
		var lines []string
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "{{.focus_directive}}") {
				lines = append(lines, line)
			}
		}
		return lines
	}
	composeCapturing := func(t *testing.T, markerFree, withDirective bool) string {
		t.Helper()
		dir := newTestStencilsDir(t)
		if markerFree {
			stripped := bytes.ReplaceAll(stencils.BurlerStep1Explore, []byte("{{.focus_directive}}"), nil)
			if err := os.WriteFile(filepath.Join(dir, "burler", "burler-step-1-explore.md"), stripped, 0o644); err != nil {
				t.Fatalf("WriteFile(marker-free burler-step-1-explore.md) = %v; want nil", err)
			}
		}
		p := newComposableProfile(t)
		if withDirective {
			p.FocusDirective = filepath.Join(t.TempDir(), "focus.md")
			if err := os.WriteFile(p.FocusDirective, []byte("look at the seam"), 0o644); err != nil {
				t.Fatalf("WriteFile(focus) = %v; want nil", err)
			}
		}
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		t.Cleanup(func() { logger.SetOutput(os.Stderr) })
		if _, err := composePrompt(dir, "", &p, "", "", testRoundFilePaths); err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		return buf.String()
	}

	t.Run("warns when marker absent", func(t *testing.T) {
		lines := focusWarningLines(composeCapturing(t, true, true))
		if len(lines) != 1 {
			t.Fatalf("focus-marker warning lines = %d; want 1: %q", len(lines), lines)
		}
		requireContains(t, lines[0], "burler-step-1-explore")
	})

	t.Run("no warning with shipped stencil", func(t *testing.T) {
		if lines := focusWarningLines(composeCapturing(t, false, true)); len(lines) != 0 {
			t.Errorf("focus-marker warning lines = %q; want none", lines)
		}
	})

	t.Run("no warning without directive", func(t *testing.T) {
		if lines := focusWarningLines(composeCapturing(t, true, false)); len(lines) != 0 {
			t.Errorf("focus-marker warning lines = %q; want none", lines)
		}
	})
}

// TestComposePrompt_MinimalProfile proves a minimal valid profile composes cleanly through stencil (no unfilled-marker error) and one render satisfies every part of composePrompt's contract:
// the combined prompt carries the profile's content (both output paths, the ready marker path and the verbatim rubric text), a Target.Paths directory entry is annotated as one while a file entry is not, both non-empty orchestrators come back with exactly four instructionFile entries whose Path values equal the four path parameters in order (the contract Engine.Run relies on to write each rendered file to the path the orchestrator names), the fixer orchestrator names the rendered review file as the review format and the marker as the file to wait on, and neither orchestrator carries a downstream instruction body.
// The tokens an orchestrator must not carry each appear only inside a downstream instruction file's body, so a regression that inlines one back trips the guard on the first offending token without colliding with an orchestrator's legitimate bare-word "verdict"/"findings" usage.
//
//testtiming:keep pins the profile content, the directory annotation, the four instruction file paths, the fixer orchestrator's pointers and the orchestrators' exclusion of downstream bodies, which the cluster-rules test covering its blocks does not assert
func TestComposePrompt_MinimalProfile(t *testing.T) {
	t.Parallel()

	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}
	got := combinedPrompt(prompts)

	requireContains(t, got, p.ReviewPath)
	requireContains(t, got, p.FixerReportPath)
	requireContains(t, got, p.ReadyMarkerPath)
	requireContains(t, got, p.Rubric)

	dirLine := findLineContaining(got, "targetdir")
	if dirLine == "" {
		t.Fatalf("composePrompt() output missing a line for the target directory entry")
	}
	requireContains(t, dirLine, "a directory")
	fileLine := findLineContaining(got, "target.txt")
	if fileLine == "" {
		t.Fatalf("composePrompt() output missing a line for the target file entry")
	}
	requireNotContains(t, fileLine, "a directory")

	if prompts.Reviewer == "" || prompts.Fixer == "" {
		t.Errorf("composePrompt() orchestrators = (%q, %q); want both non-empty", prompts.Reviewer, prompts.Fixer)
	}
	wantPaths := []string{testRoundFilePaths.ReviewerExplore, testRoundFilePaths.FixerExplore, testRoundFilePaths.Review, testRoundFilePaths.Fix}
	if len(prompts.Files) != len(wantPaths) {
		t.Fatalf("composePrompt() files = %d entries; want %d", len(prompts.Files), len(wantPaths))
	}
	for i, want := range wantPaths {
		if prompts.Files[i].Path != want {
			t.Errorf("files[%d].Path = %q; want %q", i, prompts.Files[i].Path, want)
		}
	}

	requireContains(t, prompts.Reviewer, testRoundFilePaths.ReviewerExplore)
	requireContains(t, prompts.Reviewer, testRoundFilePaths.Review)
	requireContains(t, prompts.Fixer, testRoundFilePaths.FixerExplore)
	requireContains(t, prompts.Fixer, testRoundFilePaths.Fix)
	requireContains(t, prompts.Fixer, "lyx burler await-review "+p.ReadyMarkerPath)
	requireContains(t, prompts.Fixer, testRoundFilePaths.Review)
	requireNotContains(t, prompts.Reviewer, p.ReadyMarkerPath)

	for _, orchestrator := range []string{prompts.Reviewer, prompts.Fixer} {
		requireNotContains(t, orchestrator, "not whether it gets fixed")
		requireNotContains(t, orchestrator, "verdict:")
		requireNotContains(t, orchestrator, "findings:")
		requireNotContains(t, orchestrator, "SINGLE message")
		requireNotContains(t, orchestrator, "subagent_type")
	}
}

// TestComposePrompt_ParentDirective proves both orchestrators carry the parent variant naming the parent when one is told, the no-parent variant when none is, and no instruction file carries either.
func TestComposePrompt_ParentDirective(t *testing.T) {
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	withParent, err := composePrompt(stencilsDir, "tst:task:orch", &p, "", "", testRoundFilePaths)
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}
	for _, orchestrator := range []string{withParent.Reviewer, withParent.Fixer} {
		requireContains(t, orchestrator, "tst:task:orch")
		requireContains(t, orchestrator, "A question to the operator in your pane is never the way forward.")
	}

	without, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}
	for _, orchestrator := range []string{without.Reviewer, without.Fixer} {
		requireContains(t, orchestrator, "No parent is recorded for this run.")
	}

	for _, f := range withParent.Files {
		if strings.Contains(f.Content, "## Your parent") {
			t.Errorf("instruction file %s carries the parent directive; want it only in the orchestrator", f.Path)
		}
	}
}

// TestComposePrompt_FixScope proves the fix-scope block switches on p.FixScope: FixScopeSource's
// output carries the commit-per-fix phrasing and not the overlay-exclusive "no git" phrasing,
// and vice versa for FixScopeOverlay.
// In neither scope does the fixer's rendered write surface list the review file, which only the reviewer writes.
func TestComposePrompt_FixScope(t *testing.T) {
	t.Run("source", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.FixScope = FixScopeSource

		prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(prompts)
		requireContains(t, got, "commit")
		requireNotContains(t, got, "no git")
		requireNotContains(t, findLineContaining(prompts.Files[fixFile].Content, "Write surface"), p.ReviewPath)
	})

	t.Run("overlay", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.FixScope = FixScopeOverlay

		prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(prompts)
		requireContains(t, got, "no git")
		requireNotContains(t, got, "commit each fix")
		writeSurface := findLineContaining(prompts.Files[fixFile].Content, "Write surface")
		requireContains(t, writeSurface, "plus the fixer-report (`"+p.FixerReportPath+"`)")
		requireNotContains(t, writeSurface, p.ReviewPath)
	})
}

// TestComposePrompt_ToolUse proves the tool-use block switches on p.ToolUse, each value's phrase
// present and the other's absent.
func TestComposePrompt_ToolUse(t *testing.T) {
	t.Run("true drives the substrate", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.ToolUse = true

		prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(prompts)
		requireContains(t, got, "Drive the real substrate")
		requireNotContains(t, got, "Read-only analysis")
	})

	t.Run("false is read-only", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.ToolUse = false

		prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(prompts)
		requireContains(t, got, "Read-only analysis")
		requireNotContains(t, got, "Drive the real substrate")
	})
}

// TestComposePrompt_PriorRounds proves the prior-rounds block distinguishes a first round (no prior
// files) from a round hydrated with prior review / fixer-report paths.
func TestComposePrompt_PriorRounds(t *testing.T) {
	t.Run("first round", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)

		prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(prompts)
		requireContains(t, got, "This is the first round")
	})

	t.Run("prior round", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.PriorReviews = []string{filepath.Join(t.TempDir(), "prior-review.md")}
		p.PriorFixerReports = []string{filepath.Join(t.TempDir(), "prior-fixer-report.md")}

		prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(prompts)
		requireNotContains(t, got, "This is the first round")
		requireContains(t, got, p.PriorReviews[0])
		requireContains(t, got, p.PriorFixerReports[0])
		requireContains(t, got, "OWN findings first")
		requireContains(t, got, "deferred and disputed items")
	})
}

// TestComposePrompt_ClusterRules proves the cluster-rules block switches on p.ClusterFan: empty
// renders the explicit single-reviewer prose with none of the fork machinery language, while a
// resolved fan renders every lens name plus both load-bearing fork-discipline ban phrases (no Agent
// tool, no git).
// composePrompt reads p.clusterLenses directly (as (*Profile).validate would have left it) rather
// than calling ResolveFan itself.
func TestComposePrompt_ClusterRules(t *testing.T) {
	t.Run("non-cluster", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)

		prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(prompts)
		requireContains(t, got, "single-reviewer round")
		requireNotContains(t, got, "subagent_type")
	})

	t.Run("cluster", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.ClusterFan = "standard"
		p.clusterLenses = []Lens{
			{Name: "style", Text: "pay extra attention to style"},
			{Name: "security", Text: "pay extra attention to security"},
		}

		prompts, err := composePrompt(stencilsDir, "", &p, "pattern directive placeholder", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(prompts)
		requireContains(t, got, "style")
		requireContains(t, got, "security")
		requireContains(t, got, "never call the Agent tool")
		requireContains(t, got, "never run any git command")

		// The cluster round's load-bearing fork-discipline statements: this content is composed dynamically by clusterRulesBlock into instruction 2, not baked into burler-step-2-review.md itself, so an edit that silently waters any of them down fails here rather than only in human review.
		instruction1, instruction2, instruction3 := prompts.Files[reviewerExploreFile].Content, prompts.Files[reviewFile].Content, prompts.Files[fixFile].Content
		for _, statement := range []string{
			"SINGLE message",
			"subagent_type",
			"never pass a `name`",
			"READ-ONLY",
			"never run any git command",
			"never call the Agent tool",
			"HOLISTIC",
			"complete once it is fully written to disk",
			"origin:",
			"Rejected",
		} {
			requireContains(t, instruction2, statement)
		}

		// Each block helper's rendered content lands in its intended instruction file and nowhere else: fix_scope_rules is instruction 3's alone, cluster_rules (the lens names) is instruction 2's alone, and pattern_directive/target is instruction 1's alone.
		requireContains(t, instruction3, "Write surface")
		requireNotContains(t, instruction1, "Write surface")
		requireNotContains(t, instruction2, "Write surface")

		requireContains(t, instruction1, "pattern directive placeholder")
		requireContains(t, instruction1, p.Target.Paths[0])
		requireNotContains(t, instruction2, "pattern directive placeholder")
		requireNotContains(t, instruction3, "pattern directive placeholder")
		for _, orchestrator := range []string{prompts.Reviewer, prompts.Fixer} {
			requireNotContains(t, orchestrator, "Write surface")
			requireNotContains(t, orchestrator, "style")
			requireNotContains(t, orchestrator, "pattern directive placeholder")
		}
		requireNotContains(t, instruction1, "style")
		requireNotContains(t, instruction3, "style")
	})
}

// TestComposePrompt_FrictionDirective proves the friction_directive marker composes the same way
// pattern_directive does: enabled carries the directive text verbatim into the fixer's instruction 1 alone, disabled
// composes cleanly with no directive text, and a marker-free template composes cleanly regardless.
func TestComposePrompt_FrictionDirective(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)

		prompts, err := composePrompt(stencilsDir, "", &p, "", "friction directive placeholder", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		requireContains(t, prompts.Files[fixerExploreFile].Content, "friction directive placeholder")
		requireNotContains(t, prompts.Files[reviewerExploreFile].Content, "friction directive placeholder")
	})

	t.Run("disabled", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)

		prompts, err := composePrompt(stencilsDir, "", &p, "", "", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(prompts)
		requireNotContains(t, got, "friction directive")
	})

	t.Run("marker-free template", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		markerFree := bytes.ReplaceAll(stencils.BurlerStep1Explore, []byte("{{.friction_directive}}"), nil)
		if err := os.WriteFile(filepath.Join(stencilsDir, "burler", "burler-step-1-explore.md"), markerFree, 0o644); err != nil {
			t.Fatalf("WriteFile(marker-free burler-step-1-explore.md) = %v; want nil", err)
		}

		_, err := composePrompt(stencilsDir, "", &p, "", "friction directive placeholder", testRoundFilePaths)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
	})
}

// findLineContaining returns the first line of text containing needle, or
// "" if no line matches.
func findLineContaining(text, needle string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

// requireNotContains fails the test if text contains needle.
func requireNotContains(t *testing.T, text, needle string) {
	t.Helper()
	if strings.Contains(text, needle) {
		t.Errorf("output unexpectedly contains %q", needle)
	}
}

// TestWarnIfFocusMarkerAbsent covers the helper directly:
// it logs nothing for an empty focus block or a template carrying the marker, and exactly one line naming the stencil and the marker literal otherwise.
// The logger output is process-global, so the test never runs in parallel.
func TestWarnIfFocusMarkerAbsent(t *testing.T) {
	const stencilName = "burler-step-1-explore"
	tests := []struct {
		name      string
		template  string
		block     string
		wantLines int
	}{
		{name: "empty block, marker absent", template: "no marker here", block: "", wantLines: 0},
		{name: "block, marker present", template: "has {{.focus_directive}} here", block: "focus text", wantLines: 0},
		{name: "block, marker absent", template: "no marker here", block: "focus text", wantLines: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger.SetOutput(&buf)
			t.Cleanup(func() { logger.SetOutput(os.Stderr) })

			warnIfFocusMarkerAbsent([]byte(tt.template), stencilName, tt.block)

			out := strings.TrimSpace(buf.String())
			got := 0
			if out != "" {
				got = len(strings.Split(out, "\n"))
			}
			if got != tt.wantLines {
				t.Fatalf("logged %d lines; want %d; output: %q", got, tt.wantLines, out)
			}
			if tt.wantLines == 1 {
				if !strings.Contains(out, stencilName) {
					t.Errorf("line %q does not name stencil %q", out, stencilName)
				}
				if !strings.Contains(out, "{{.focus_directive}}") {
					t.Errorf("line %q does not name the marker literal", out)
				}
			}
		})
	}
}
