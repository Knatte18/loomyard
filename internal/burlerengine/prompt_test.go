// prompt_test.go covers composePrompt's marker composition: the happy path
// fills every marker, and each switched block (fix-scope, tool-use,
// prior-rounds, cluster-rules) renders the branch its Profile field selects
// and not the other branch's exclusive phrasing.

package burlerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// TestComposePrompt_FillsAllMarkers proves a minimal valid profile composes
// cleanly through stencil (no unfilled-marker error) and that the rendered
// prompt actually carries the profile's content — both output paths and
// the verbatim rubric text.
func TestComposePrompt_FillsAllMarkers(t *testing.T) {
	p := newComposableProfile(t)

	got, err := composePrompt(&p)
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}

	requireContains(t, got, p.ReviewPath)
	requireContains(t, got, p.FixerReportPath)
	requireContains(t, got, p.Rubric)
}

// TestComposePrompt_FixScope proves the fix-scope block switches on
// p.FixScope: FixScopeSource's output carries the commit-per-fix phrasing
// and not the overlay-exclusive "no git" phrasing, and vice versa for
// FixScopeOverlay.
func TestComposePrompt_FixScope(t *testing.T) {
	t.Run("source", func(t *testing.T) {
		p := newComposableProfile(t)
		p.FixScope = FixScopeSource

		got, err := composePrompt(&p)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		requireContains(t, got, "commit")
		requireNotContains(t, got, "no git")
	})

	t.Run("overlay", func(t *testing.T) {
		p := newComposableProfile(t)
		p.FixScope = FixScopeOverlay

		got, err := composePrompt(&p)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		requireContains(t, got, "no git")
		requireNotContains(t, got, "commit each fix")
	})
}

// TestComposePrompt_ToolUse proves the tool-use block switches on
// p.ToolUse, each value's phrase present and the other's absent.
func TestComposePrompt_ToolUse(t *testing.T) {
	t.Run("true drives the substrate", func(t *testing.T) {
		p := newComposableProfile(t)
		p.ToolUse = true

		got, err := composePrompt(&p)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		requireContains(t, got, "Drive the real substrate")
		requireNotContains(t, got, "Read-only analysis")
	})

	t.Run("false is read-only", func(t *testing.T) {
		p := newComposableProfile(t)
		p.ToolUse = false

		got, err := composePrompt(&p)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		requireContains(t, got, "Read-only analysis")
		requireNotContains(t, got, "Drive the real substrate")
	})
}

// TestComposePrompt_PriorRounds proves the prior-rounds block distinguishes
// a first round (no prior files) from a round hydrated with prior review /
// fixer-report paths.
func TestComposePrompt_PriorRounds(t *testing.T) {
	t.Run("first round", func(t *testing.T) {
		p := newComposableProfile(t)

		got, err := composePrompt(&p)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		requireContains(t, got, "This is the first round")
	})

	t.Run("prior round", func(t *testing.T) {
		p := newComposableProfile(t)
		p.PriorReviews = []string{filepath.Join(t.TempDir(), "prior-review.md")}
		p.PriorFixerReports = []string{filepath.Join(t.TempDir(), "prior-fixer-report.md")}

		got, err := composePrompt(&p)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		requireNotContains(t, got, "This is the first round")
		requireContains(t, got, p.PriorReviews[0])
		requireContains(t, got, p.PriorFixerReports[0])
		requireContains(t, got, "OWN findings first")
	})
}

// TestComposePrompt_DirectoryAnnotation proves a Target.Paths entry that is
// a directory is annotated as one, while a file entry is not.
func TestComposePrompt_DirectoryAnnotation(t *testing.T) {
	p := newComposableProfile(t)

	got, err := composePrompt(&p)
	if err != nil {
		t.Fatalf("composePrompt() = %v; want nil error", err)
	}

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

// TestComposePrompt_ClusterRules proves the cluster-rules block switches on
// p.ClusterFan: empty renders the explicit single-reviewer prose with none
// of the fork machinery language, while a resolved fan renders every lens
// name plus both load-bearing fork-discipline ban phrases (no Agent tool,
// no git). composePrompt reads p.clusterLenses directly (as
// (*Profile).validate would have left it) rather than calling ResolveFan
// itself.
func TestComposePrompt_ClusterRules(t *testing.T) {
	t.Run("non-cluster", func(t *testing.T) {
		p := newComposableProfile(t)

		got, err := composePrompt(&p)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		requireContains(t, got, "single-reviewer round")
		requireNotContains(t, got, "subagent_type")
	})

	t.Run("cluster", func(t *testing.T) {
		p := newComposableProfile(t)
		p.ClusterFan = "standard"
		p.clusterLenses = []Lens{
			{Name: "style", Text: "pay extra attention to style"},
			{Name: "security", Text: "pay extra attention to security"},
		}

		got, err := composePrompt(&p)
		if err != nil {
			t.Fatalf("composePrompt() = %v; want nil error", err)
		}
		requireContains(t, got, "style")
		requireContains(t, got, "security")
		requireContains(t, got, "never call the Agent tool")
		requireContains(t, got, "never run any git command")
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
