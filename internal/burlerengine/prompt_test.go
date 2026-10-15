// prompt_test.go covers the half prompts' marker composition.
// The happy path fills every marker across the review half's three rendered assets and the fix half's two.
// Each switched block (fix-scope, tool-use, prior-rounds, cluster-rules) renders the branch its Profile field selects and not the other branch's exclusive phrasing.
// Each block helper's content lands in its intended instruction file rather than leaking into a seat stencil or a sibling instruction file.
// It also declares newTestStencilsDir, the package-local test helper every burlerengine test uses to seed a hermetic stencils directory through `stencilkit`.

package burlerengine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

// Placeholder absolute instruction paths every compose call in this file passes.
// The compose functions do no filesystem access on these beyond baking them into the seat stencils' marker values, so they need not exist on disk (the halves, not the compose functions, write the files there).
var (
	testReviewFiles = reviewFilesIn("/tmp/review-half")
	testFixFiles    = fixFilesIn("/tmp/fix-half")
)

// The positions of the rendered instruction files in a halfPrompt's Files.
const (
	reviewExploreFile = iota
	reviewStepFile
	reviewFormatFile
)

const (
	fixFormatFile = iota
	fixStepFile
)

// newComposableProfile builds a minimal Profile whose paths already exist
// on disk (the compose functions' directory annotation stats them) and whose
// fields are all resolved absolute paths, as validateReview and validateFix would
// leave them — the compose functions are documented to run only after validation.
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
	}
}

// composeReview composes p's review half prompt over the placeholder paths, failing the test on error.
func composeReview(t *testing.T, stencilsDir string, p *Profile, patternDirective string) halfPrompt {
	t.Helper()
	prompt, err := composeReviewPrompt(stencilsDir, p, patternDirective, testReviewFiles)
	if err != nil {
		t.Fatalf("composeReviewPrompt() = %v; want nil error", err)
	}
	return prompt
}

// composeFix composes p's fix half prompt over the placeholder paths and a review's findings as the work, failing the test on error.
func composeFix(t *testing.T, stencilsDir string, p *Profile, frictionDirective string) halfPrompt {
	t.Helper()
	prompt, err := composeFixPrompt(stencilsDir, p, frictionDirective, fixWork(p.ReviewPath, ""), testFixFiles)
	if err != nil {
		t.Fatalf("composeFixPrompt() = %v; want nil error", err)
	}
	return prompt
}

// chairPrompt renders the one chair seat's prompt of a half's table the way the seats engine fills it, told parentName.
func chairPrompt(t *testing.T, stencilsDir, parentName string, p *Profile, table seatengine.Table) string {
	t.Helper()
	root := filepath.Dir(p.ReviewPath)
	rendered, err := seatengine.Prompts(
		seatengine.Geometry{WorktreeRoot: root, AnchorPath: root, StencilsDir: stencilsDir, ParentName: parentName, Shortname: "tst", Slug: "task"},
		table,
	)
	if err != nil {
		t.Fatalf("seatengine.Prompts() = %v; want nil error", err)
	}
	return rendered[""]
}

// combinedPrompt joins a half's chair prompt with every instruction file's Content, newline-separated.
// It gives tests a single haystack to search when a presence/absence assertion does not care which of the rendered assets carries the text.
func combinedPrompt(t *testing.T, stencilsDir string, p *Profile, table seatengine.Table, prompt halfPrompt) string {
	t.Helper()
	parts := make([]string, 0, len(prompt.Files)+1)
	parts = append(parts, chairPrompt(t, stencilsDir, "", p, table))
	for _, f := range prompt.Files {
		parts = append(parts, f.Content)
	}
	return strings.Join(parts, "\n")
}

// reviewHalf composes p's review half and returns its prompt with its combined haystack.
func reviewHalf(t *testing.T, stencilsDir string, p *Profile, patternDirective string) (halfPrompt, string) {
	t.Helper()
	prompt := composeReview(t, stencilsDir, p, patternDirective)
	return prompt, combinedPrompt(t, stencilsDir, p, p.reviewTable(RunOpts{}, prompt.Values, burlerSkills), prompt)
}

// fixHalf composes p's fix half and returns its prompt with its combined haystack.
func fixHalf(t *testing.T, stencilsDir string, p *Profile, frictionDirective string) (halfPrompt, string) {
	t.Helper()
	prompt := composeFix(t, stencilsDir, p, frictionDirective)
	return prompt, combinedPrompt(t, stencilsDir, p, p.fixTable(RunOpts{}, prompt.Values, burlerSkills), prompt)
}

// TestComposePrompt_FocusDirective proves a round's focus file reaches the explore step through its own channel, with the precedence rule, and never reaches the review step's prior-rounds block;
// and that a round without a directive renders no focus text and no marker residue.
func TestComposePrompt_FocusDirective(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)

	t.Run("directive present", func(t *testing.T) {
		p := newComposableProfile(t)
		p.FocusDirective = filepath.Join(t.TempDir(), "focus.md")
		if err := os.WriteFile(p.FocusDirective, []byte("look at the seam"), 0o644); err != nil {
			t.Fatalf("WriteFile(focus) = %v; want nil", err)
		}

		prompt := composeReview(t, stencilsDir, &p, "")

		requireContains(t, prompt.Files[reviewExploreFile].Content, p.FocusDirective)
		requireContains(t, prompt.Files[reviewExploreFile].Content, "The rubric binds over the focus directive")
		requireNotContains(t, prompt.Files[reviewStepFile].Content, p.FocusDirective)
	})

	t.Run("directive absent", func(t *testing.T) {
		p := newComposableProfile(t)

		got := composeReview(t, stencilsDir, &p, "").Files[reviewExploreFile].Content

		requireNotContains(t, got, "Focus directive for this round")
		if strings.Contains(got, "{{") || strings.Contains(got, "<no value>") {
			t.Errorf("explore step contains marker residue: %q", got)
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
		t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })
		composeReview(t, dir, &p, "")
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

// TestComposePrompt_MinimalProfile proves a minimal valid profile composes both halves cleanly through stencil (no unfilled-marker error) and one render of each satisfies the compose contract:
// the review half's combined prompt carries the profile's content (the review path and the verbatim rubric text), a Target.Paths directory entry is annotated as one while a file entry is not, its files come back with Path values equal to the path parameters in order (the contract each half relies on to write every rendered file to the path the seat stencil names), and its seat stencil names the explore and review files;
// the fix half's combined prompt carries both output paths, its seat stencil names the review, the format file and the fix step, and no seat stencil carries a downstream instruction body or a ready marker.
// The tokens a seat stencil must not carry each appear only inside a downstream instruction file's body, so a regression that inlines one back trips the guard on the first offending token without colliding with a seat stencil's legitimate bare-word "verdict"/"findings" usage.
//
//testtiming:keep pins the profile content, the directory annotation, the instruction file paths, the fixer stencil's pointers and the seat stencils' exclusion of downstream bodies, which the cluster-rules test covering its blocks does not assert
func TestComposePrompt_MinimalProfile(t *testing.T) {
	t.Parallel()

	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	review, reviewAll := reviewHalf(t, stencilsDir, &p, "")
	fix, fixAll := fixHalf(t, stencilsDir, &p, "")

	requireContains(t, reviewAll, p.ReviewPath)
	requireContains(t, reviewAll, p.Rubric)
	requireContains(t, fixAll, p.ReviewPath)
	requireContains(t, fixAll, p.FixerReportPath)

	dirLine := findLineContaining(reviewAll, "targetdir")
	if dirLine == "" {
		t.Fatalf("composeReviewPrompt() output missing a line for the target directory entry")
	}
	requireContains(t, dirLine, "a directory")
	fileLine := findLineContaining(reviewAll, "target.txt")
	if fileLine == "" {
		t.Fatalf("composeReviewPrompt() output missing a line for the target file entry")
	}
	requireNotContains(t, fileLine, "a directory")

	for _, tt := range []struct {
		name      string
		prompt    halfPrompt
		wantPaths []string
	}{
		{"review", review, []string{testReviewFiles.Explore, testReviewFiles.Review, testReviewFiles.Format}},
		{"fix", fix, []string{testFixFiles.Format, testFixFiles.Fix}},
	} {
		if len(tt.prompt.Files) != len(tt.wantPaths) {
			t.Fatalf("%s half files = %d entries; want %d", tt.name, len(tt.prompt.Files), len(tt.wantPaths))
		}
		for i, want := range tt.wantPaths {
			if tt.prompt.Files[i].Path != want {
				t.Errorf("%s half files[%d].Path = %q; want %q", tt.name, i, tt.prompt.Files[i].Path, want)
			}
		}
	}

	reviewer := chairPrompt(t, stencilsDir, "", &p, p.reviewTable(RunOpts{}, review.Values, burlerSkills))
	fixer := chairPrompt(t, stencilsDir, "", &p, p.fixTable(RunOpts{}, fix.Values, burlerSkills))
	requireContains(t, reviewer, testReviewFiles.Explore)
	requireContains(t, reviewer, testReviewFiles.Review)
	requireContains(t, fixer, testFixFiles.Format)
	requireContains(t, fixer, testFixFiles.Fix)
	requireContains(t, fixer, p.ReviewPath)
	for _, seat := range []string{reviewer, fixer} {
		requireNotContains(t, seat, "not whether it gets fixed")
		requireNotContains(t, seat, "verdict:")
		requireNotContains(t, seat, "findings:")
		requireNotContains(t, seat, "SINGLE message")
		requireNotContains(t, seat, "subagent_type")
		requireNotContains(t, seat, "lyx shuttle await")
	}
}

// TestComposePrompt_ParentDirective proves both seat stencils carry the parent variant naming the parent when one is told, the no-parent variant when none is, and no instruction file carries either.
func TestComposePrompt_ParentDirective(t *testing.T) {
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	review := composeReview(t, stencilsDir, &p, "")
	fix := composeFix(t, stencilsDir, &p, "")
	reviewTable, fixTable := p.reviewTable(RunOpts{}, review.Values, burlerSkills), p.fixTable(RunOpts{}, fix.Values, burlerSkills)

	for _, seat := range []string{
		chairPrompt(t, stencilsDir, "tst:task:orch", &p, reviewTable),
		chairPrompt(t, stencilsDir, "tst:task:orch", &p, fixTable),
	} {
		requireContains(t, seat, "tst:task:orch")
		requireContains(t, seat, "A question to the operator in your pane is never the way forward.")
		requireContains(t, seat, "Edit or Write")
	}
	for _, seat := range []string{
		chairPrompt(t, stencilsDir, "", &p, reviewTable),
		chairPrompt(t, stencilsDir, "", &p, fixTable),
	} {
		requireContains(t, seat, "No parent is recorded for this run.")
	}

	for _, f := range append(append([]instructionFile{}, review.Files...), fix.Files...) {
		if strings.Contains(f.Content, "## Your parent") {
			t.Errorf("instruction file %s carries the parent directive; want it only in the seat stencil", f.Path)
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

		fix, got := fixHalf(t, stencilsDir, &p, "")

		requireContains(t, got, "commit")
		requireNotContains(t, got, "no git")
		requireNotContains(t, findLineContaining(fix.Files[fixStepFile].Content, "Write surface"), p.ReviewPath)
	})

	t.Run("overlay", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.FixScope = FixScopeOverlay

		fix, got := fixHalf(t, stencilsDir, &p, "")

		requireContains(t, got, "no git")
		requireNotContains(t, got, "commit each fix")
		writeSurface := findLineContaining(fix.Files[fixStepFile].Content, "Write surface")
		requireContains(t, writeSurface, "plus the fixer-report (`"+p.FixerReportPath+"`)")
		requireNotContains(t, writeSurface, p.ReviewPath)
	})
}

// TestComposePrompt_ToolUse proves the tool-use block switches on p.ToolUse, each value's phrase
// present and the other's absent.
func TestComposePrompt_ToolUse(t *testing.T) {
	t.Run("true drives the substrate", func(t *testing.T) {
		p := newComposableProfile(t)
		p.ToolUse = true

		_, got := reviewHalf(t, newTestStencilsDir(t), &p, "")

		requireContains(t, got, "Drive the real substrate")
		requireNotContains(t, got, "Read-only analysis")
	})

	t.Run("false is read-only", func(t *testing.T) {
		p := newComposableProfile(t)
		p.ToolUse = false

		_, got := reviewHalf(t, newTestStencilsDir(t), &p, "")

		requireContains(t, got, "Read-only analysis")
		requireNotContains(t, got, "Drive the real substrate")
	})
}

// TestComposePrompt_PriorRounds proves the prior-rounds block distinguishes a first round (no prior
// files) from a round hydrated with prior review / fixer-report paths.
func TestComposePrompt_PriorRounds(t *testing.T) {
	t.Run("first round", func(t *testing.T) {
		p := newComposableProfile(t)

		_, got := reviewHalf(t, newTestStencilsDir(t), &p, "")

		requireContains(t, got, "This is the first round")
	})

	t.Run("prior round", func(t *testing.T) {
		p := newComposableProfile(t)
		p.PriorReviews = []string{filepath.Join(t.TempDir(), "prior-review.md")}
		p.PriorFixerReports = []string{filepath.Join(t.TempDir(), "prior-fixer-report.md")}

		_, got := reviewHalf(t, newTestStencilsDir(t), &p, "")

		requireNotContains(t, got, "This is the first round")
		requireContains(t, got, p.PriorReviews[0])
		requireContains(t, got, p.PriorFixerReports[0])
		requireContains(t, got, "OWN findings first")
		requireContains(t, got, "deferred and disputed items")
		requireContains(t, got, "why the dispute's evidence does not hold")
	})
}

// TestComposePrompt_ClusterRules proves the cluster-rules block switches on p.ClusterFan: empty
// renders the explicit single-reviewer prose with none of the fork machinery language, while a
// resolved fan renders every lens name plus both load-bearing fork-discipline ban phrases (no Agent
// tool, no git).
// composeReviewPrompt reads p.clusterLenses directly (as (*Profile).validateReview would have left it) rather
// than calling ResolveFan itself.
func TestComposePrompt_ClusterRules(t *testing.T) {
	t.Run("non-cluster", func(t *testing.T) {
		p := newComposableProfile(t)

		_, got := reviewHalf(t, newTestStencilsDir(t), &p, "")

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

		review, got := reviewHalf(t, stencilsDir, &p, "pattern directive placeholder")
		fix := composeFix(t, stencilsDir, &p, "")

		requireContains(t, got, "style")
		requireContains(t, got, "security")
		requireContains(t, got, "never call the Agent tool")
		requireContains(t, got, "never run any git command")

		// The cluster round's load-bearing fork-discipline statements: this content is composed dynamically by clusterRulesBlock into the review step, not baked into burler-step-review.md itself, so an edit that silently waters any of them down fails here rather than only in human review.
		explore, reviewStep := review.Files[reviewExploreFile].Content, review.Files[reviewStepFile].Content
		fixStep := fix.Files[fixStepFile].Content
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
			requireContains(t, reviewStep, statement)
		}

		// Each block helper's rendered content lands in its intended instruction file and nowhere else: fix_scope_rules is the fix step's alone, cluster_rules (the lens names) is the review step's alone, and pattern_directive/target is the explore step's alone.
		requireContains(t, fixStep, "Write surface")
		requireNotContains(t, explore, "Write surface")
		requireNotContains(t, reviewStep, "Write surface")

		requireContains(t, explore, "pattern directive placeholder")
		requireContains(t, explore, p.Target.Paths[0])
		requireNotContains(t, reviewStep, "pattern directive placeholder")
		requireNotContains(t, fixStep, "pattern directive placeholder")
		for _, seat := range []string{
			chairPrompt(t, stencilsDir, "", &p, p.reviewTable(RunOpts{}, review.Values, burlerSkills)),
			chairPrompt(t, stencilsDir, "", &p, p.fixTable(RunOpts{}, fix.Values, burlerSkills)),
		} {
			requireNotContains(t, seat, "Write surface")
			requireNotContains(t, seat, "style")
			requireNotContains(t, seat, "pattern directive placeholder")
		}
		requireNotContains(t, explore, "style")
		requireNotContains(t, fixStep, "style")
	})
}

// TestComposePrompt_FrictionDirective proves the friction_directive marker composes the same way
// pattern_directive does: enabled carries the directive text verbatim into the fix step alone, disabled
// composes cleanly with no directive text, and a marker-free template composes cleanly regardless.
func TestComposePrompt_FrictionDirective(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)

		fix := composeFix(t, stencilsDir, &p, "friction directive placeholder")
		review := composeReview(t, stencilsDir, &p, "")

		requireContains(t, fix.Files[fixStepFile].Content, "friction directive placeholder")
		for _, f := range review.Files {
			requireNotContains(t, f.Content, "friction directive placeholder")
		}
	})

	t.Run("disabled", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)

		_, fixAll := fixHalf(t, stencilsDir, &p, "")
		_, reviewAll := reviewHalf(t, stencilsDir, &p, "")

		requireNotContains(t, fixAll, "friction directive")
		requireNotContains(t, reviewAll, "friction directive")
	})

	t.Run("marker-free template", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		markerFree := bytes.ReplaceAll(stencils.BurlerStepFix, []byte("{{.friction_directive}}"), nil)
		if err := os.WriteFile(filepath.Join(stencilsDir, "burler", "burler-step-fix.md"), markerFree, 0o644); err != nil {
			t.Fatalf("WriteFile(marker-free burler-step-fix.md) = %v; want nil", err)
		}

		composeFix(t, stencilsDir, &p, "friction directive placeholder")
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
			t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })

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
