// merriambase_test.go covers the start base the batchifier prices Merriam's session from: the line sum of the texts it loads, the fixed system-prompt context, and how the base follows the worktree's CLAUDE.md and the plan's overview.
// Tier 1: the base is read from a temp worktree with the shipped stencils seeded; no git, no process.

package websterengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestMerriamBase pins MerriamBaseOf's line sum and fixed context, and that MerriamBase grows with the plan's 00-overview.md and with the worktree's CLAUDE.md, adds nothing for an absent CLAUDE.md or CLAUDE.local.md and refuses an absent overview.
// The steps share one temp worktree and each rewrites its own files, so none relies on another's result.
// The scenario calls t.Parallel as a whole and no step does, since the steps share the one worktree.
func TestMerriamBase(t *testing.T) {
	t.Parallel()

	// wantFixed is 20600, the measured Merriam session start, minus the computed size of the texts the base covers at the commit the constant was taken at.
	const wantFixed = 15896

	t.Run("MerriamBaseOf sums the lines of its texts and carries the fixed context", func(t *testing.T) {
		got := websterengine.MerriamBaseOf("one\ntwo\n", "three", "", "four\nfive\nsix\n")

		want := batcher.StartBase{Lines: 6, Fixed: wantFixed}
		if got != want {
			t.Errorf("MerriamBaseOf() = %+v; want %+v", got, want)
		}
		if none := websterengine.MerriamBaseOf(); none != (batcher.StartBase{Fixed: wantFixed}) {
			t.Errorf("MerriamBaseOf() with no text = %+v; want only the fixed context", none)
		}
	})

	worktree := t.TempDir()
	planDir := filepath.Join(worktree, "plan")
	stencilsDir := stencilkit.Seed(t)
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	geom := websterengine.Geometry{WorktreeRoot: worktree, PlanDir: planDir, StencilsDir: stencilsDir}
	write := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	overview := filepath.Join(planDir, "00-overview.md")
	claude := filepath.Join(worktree, "CLAUDE.md")
	claudeLocal := filepath.Join(worktree, "CLAUDE.local.md")

	baseLines := 0
	t.Run("an absent CLAUDE.md and CLAUDE.local.md add nothing", func(t *testing.T) {
		write(t, overview, "# plan\n")

		got, err := websterengine.MerriamBase(geom)
		if err != nil {
			t.Fatalf("MerriamBase() error = %v; want nil", err)
		}
		if got.Fixed != wantFixed || got.Lines == 0 {
			t.Errorf("MerriamBase() = %+v; want the fixed context and the stencil and overview lines", got)
		}
		baseLines = got.Lines
	})

	t.Run("the base grows with the overview", func(t *testing.T) {
		write(t, overview, "# plan\nsecond line\nthird line\n")

		got, err := websterengine.MerriamBase(geom)
		if err != nil {
			t.Fatalf("MerriamBase() error = %v; want nil", err)
		}
		if want := baseLines + 2; got.Lines != want {
			t.Errorf("MerriamBase().Lines = %d; want %d, two overview lines more", got.Lines, want)
		}
		baseLines = got.Lines
	})

	t.Run("the base grows with CLAUDE.md and CLAUDE.local.md", func(t *testing.T) {
		write(t, claude, "a\nb\nc\n")
		write(t, claudeLocal, "d\n")

		got, err := websterengine.MerriamBase(geom)
		if err != nil {
			t.Fatalf("MerriamBase() error = %v; want nil", err)
		}
		if want := baseLines + 4; got.Lines != want {
			t.Errorf("MerriamBase().Lines = %d; want %d, four CLAUDE lines more", got.Lines, want)
		}
	})

	t.Run("an absent overview is refused as transient naming the file", func(t *testing.T) {
		if err := os.Remove(overview); err != nil {
			t.Fatalf("remove overview: %v", err)
		}

		_, err := websterengine.MerriamBase(geom)
		if err == nil {
			t.Fatal("MerriamBase() error = nil; want a refusal for the absent overview")
		}
		for _, want := range []string{"00-overview.md", "way forward: transient, re-run the verb"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("MerriamBase() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})
}
