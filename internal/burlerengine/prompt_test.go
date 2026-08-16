// prompt_test.go covers composePrompt's marker composition: the happy path fills every marker
// across all four rendered assets, each switched block (fix-scope, tool-use, prior-rounds,
// cluster-rules) renders the branch its Profile field selects and not the other branch's exclusive
// phrasing, and each block helper's content lands in its intended instruction file rather than
// leaking into the orchestrator or a sibling instruction file.
// It also declares newTestStencilsDir, the package-local test helper every burlerengine test uses to
// seed a hermetic stencils directory from the shipped stencils package defaults.

package burlerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
)

// newTestStencilsDir builds a t.TempDir() seeded with burler's four stencils plus the three
// pattern-directive stencils, copied byte-for-byte from the stencils package's embedded defaults, and
// returns the directory to pass as stencilsDir.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	burlerDir := filepath.Join(dir, "burler")
	if err := os.MkdirAll(burlerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", burlerDir, err)
	}
	files := map[string][]byte{
		"burler-template-round-orchestrator.md": stencils.BurlerTemplateRoundOrchestrator,
		"burler-step-1-explore.md":              stencils.BurlerStep1Explore,
		"burler-step-2-review.md":               stencils.BurlerStep2Review,
		"burler-step-3-fix.md":                  stencils.BurlerStep3Fix,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(burlerDir, name), content, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
		}
	}

	patternDir := filepath.Join(dir, "pattern")
	if err := os.MkdirAll(patternDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", patternDir, err)
	}
	patternFiles := map[string][]byte{
		"pattern-directive-implementer.md":  stencils.PatternDirectiveImplementer,
		"pattern-directive-review-fix.md":   stencils.PatternDirectiveReviewFix,
		"pattern-directive-orchestrator.md": stencils.PatternDirectiveOrchestrator,
	}
	for name, content := range patternFiles {
		if err := os.WriteFile(filepath.Join(patternDir, name), content, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
		}
	}
	return dir
}

// Placeholder absolute instruction paths every composePrompt call in this
// file passes — composePrompt does no filesystem access on these beyond
// baking them into the orchestrator's marker values, so they need not
// exist on disk (Engine.Run, not composePrompt, writes the files there).
const (
	testInst1Path = "/tmp/instruction-1-explore.md"
	testInst2Path = "/tmp/instruction-2-review.md"
	testInst3Path = "/tmp/instruction-3-fix.md"
)

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
	}
}

// combinedPrompt joins the orchestrator string with every instruction
// file's Content, newline-separated, giving tests a single haystack to
// search when a presence/absence assertion does not care which of the four
// rendered assets carries the text — this preserves the pre-split tests'
// present/absent semantics now that composePrompt returns four assets
// instead of one.
func combinedPrompt(orchestrator string, files []instructionFile) string {
	parts := make([]string, 0, len(files)+1)
	parts = append(parts, orchestrator)
	for _, f := range files {
		parts = append(parts, f.Content)
	}
	return strings.Join(parts, "\n")
}

// TestComposePrompt_FillsAllMarkers proves a minimal valid profile composes cleanly through stencil
// (no unfilled-marker error) and that the combined rendered prompt actually carries the profile's
// content — both output paths and the verbatim rubric text.
func TestComposePrompt_FillsAllMarkers(t *testing.T) {
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}
	got := combinedPrompt(orchestrator, files)

	requireContains(t, got, p.ReviewPath)
	requireContains(t, got, p.FixerReportPath)
	requireContains(t, got, p.Rubric)
}

// TestComposePrompt_FixScope proves the fix-scope block switches on p.FixScope: FixScopeSource's
// output carries the commit-per-fix phrasing and not the overlay-exclusive "no git" phrasing,
// and vice versa for FixScopeOverlay.
func TestComposePrompt_FixScope(t *testing.T) {
	t.Run("source", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.FixScope = FixScopeSource

		orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(orchestrator, files)
		requireContains(t, got, "commit")
		requireNotContains(t, got, "no git")
	})

	t.Run("overlay", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.FixScope = FixScopeOverlay

		orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(orchestrator, files)
		requireContains(t, got, "no git")
		requireNotContains(t, got, "commit each fix")
	})
}

// TestComposePrompt_ToolUse proves the tool-use block switches on p.ToolUse, each value's phrase
// present and the other's absent.
func TestComposePrompt_ToolUse(t *testing.T) {
	t.Run("true drives the substrate", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.ToolUse = true

		orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(orchestrator, files)
		requireContains(t, got, "Drive the real substrate")
		requireNotContains(t, got, "Read-only analysis")
	})

	t.Run("false is read-only", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.ToolUse = false

		orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(orchestrator, files)
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

		orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(orchestrator, files)
		requireContains(t, got, "This is the first round")
	})

	t.Run("prior round", func(t *testing.T) {
		p := newComposableProfile(t)
		stencilsDir := newTestStencilsDir(t)
		p.PriorReviews = []string{filepath.Join(t.TempDir(), "prior-review.md")}
		p.PriorFixerReports = []string{filepath.Join(t.TempDir(), "prior-fixer-report.md")}

		orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(orchestrator, files)
		requireNotContains(t, got, "This is the first round")
		requireContains(t, got, p.PriorReviews[0])
		requireContains(t, got, p.PriorFixerReports[0])
		requireContains(t, got, "OWN findings first")
	})
}

// TestComposePrompt_DirectoryAnnotation proves a Target.Paths entry that is a directory is
// annotated as one, while a file entry is not.
func TestComposePrompt_DirectoryAnnotation(t *testing.T) {
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}
	got := combinedPrompt(orchestrator, files)

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

		orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(orchestrator, files)
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

		orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		got := combinedPrompt(orchestrator, files)
		requireContains(t, got, "style")
		requireContains(t, got, "security")
		requireContains(t, got, "never call the Agent tool")
		requireContains(t, got, "never run any git command")
	})
}

// TestComposePrompt_ReturnsThreeInstructionFiles proves the happy path returns a non-empty
// orchestrator and exactly three instructionFile entries whose Path values equal the three path
// parameters, in order — the contract Engine.Run relies on to write each rendered file to the path
// it names in the orchestrator.
func TestComposePrompt_ReturnsThreeInstructionFiles(t *testing.T) {
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)

	orchestrator, files, err := composePrompt(stencilsDir, &p, "", testInst1Path, testInst2Path, testInst3Path)
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}

	if orchestrator == "" {
		t.Errorf("composePrompt() orchestrator = \"\"; want non-empty")
	}
	if len(files) != 3 {
		t.Fatalf("composePrompt() files = %d entries; want 3", len(files))
	}
	wantPaths := []string{testInst1Path, testInst2Path, testInst3Path}
	for i, want := range wantPaths {
		if files[i].Path != want {
			t.Errorf("files[%d].Path = %q; want %q", i, files[i].Path, want)
		}
	}
}

// TestComposePrompt_BlockHelpersLandInIntendedAsset proves each block helper's rendered content
// lands in its intended instruction file and nowhere else: fix_scope_rules content is instruction
// 3's alone, cluster_rules content is instruction 2's alone, and pattern_directive/target content
// is instruction 1's alone.
// This is the per-asset counterpart to the marker-value composition tests above, which only prove
// presence in the combined text.
func TestComposePrompt_BlockHelpersLandInIntendedAsset(t *testing.T) {
	p := newComposableProfile(t)
	stencilsDir := newTestStencilsDir(t)
	p.ClusterFan = "standard"
	p.clusterLenses = []Lens{{Name: "style", Text: "pay extra attention to style"}}

	orchestrator, files, err := composePrompt(stencilsDir, &p, "pattern directive placeholder", testInst1Path, testInst2Path, testInst3Path)
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}
	instruction1, instruction2, instruction3 := files[0].Content, files[1].Content, files[2].Content

	requireContains(t, instruction3, "Write surface")
	requireNotContains(t, orchestrator, "Write surface")
	requireNotContains(t, instruction1, "Write surface")
	requireNotContains(t, instruction2, "Write surface")

	requireContains(t, instruction2, "style")
	requireNotContains(t, orchestrator, "style")
	requireNotContains(t, instruction1, "style")
	requireNotContains(t, instruction3, "style")

	requireContains(t, instruction1, "pattern directive placeholder")
	requireContains(t, instruction1, p.Target.Paths[0])
	requireNotContains(t, orchestrator, "pattern directive placeholder")
	requireNotContains(t, instruction2, "pattern directive placeholder")
	requireNotContains(t, instruction3, "pattern directive placeholder")
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
