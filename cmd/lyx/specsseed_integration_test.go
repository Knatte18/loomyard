//go:build integration

// specsseed_integration_test.go pins seedStencilsAt's specs half: that one call against a fresh hub
// seeds both subtrees (not just stencils), that a second run against an already-seeded hub writes
// nothing, and that the seeded specs files land committed in the board repository rather than merely
// written to disk -- the write-but-never-stage defect the generalisation in batch 3 exists to fix.

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// TestSeedStencilsAt_SeedsBothSubtrees asserts that one call against a fresh hub populates both the
// stencils subtree, as today, and the deployed-specs subtree: the two registered spec files plus a
// .gitattributes, each parseable and classified as untouched immediately after seeding.
func TestSeedStencilsAt_SeedsBothSubtrees(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	stencilsDir := fabricengine.StencilsDir(hub.Path)
	specsDir := fabricengine.SpecsDir(hub.Path)

	seedStencilsAt(hub.Path, hub.PrimeWorktree())

	discussionPath := filepath.Join(stencilsDir, "loom", "loom-template-discussion.md")
	if _, err := os.Stat(discussionPath); err != nil {
		t.Fatalf("stat %s after seedStencilsAt: %v; want the seeded stencil to exist", discussionPath, err)
	}

	specNames := []string{"loom-plan-spec"}
	for _, name := range specNames {
		path := stencilstore.Path(specsDir, name)
		onDisk, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s after seedStencilsAt: %v; want the seeded spec %q to exist", path, err, name)
		}

		hash, ok := stencilstore.ParseStamp(onDisk)
		if !ok || hash == "" {
			t.Errorf("stencilstore.ParseStamp(%s) = %q, %v; want a parseable, non-empty hash", path, hash, ok)
		}

		if state := stencilstore.Classify(onDisk, true, onDisk); state != stencilstore.StateUntouched {
			t.Errorf("stencilstore.Classify(%s) = %v; want StateUntouched immediately after seeding", path, state)
		}
	}

	attrsPath := filepath.Join(specsDir, ".gitattributes")
	if _, err := os.Stat(attrsPath); err != nil {
		t.Errorf("stat %s after seedStencilsAt: %v; want a seeded .gitattributes in the specs subtree", attrsPath, err)
	}
}

// TestSeedStencilsAt_SecondRunWritesNothing calls seedStencilsAt twice and asserts the second call
// leaves both subtrees byte-identical to what the first produced, and the board repository clean.
func TestSeedStencilsAt_SecondRunWritesNothing(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	worktree := hub.PrimeWorktree()
	stencilsDir := fabricengine.StencilsDir(hub.Path)
	specsDir := fabricengine.SpecsDir(hub.Path)

	seedStencilsAt(hub.Path, worktree)

	before := map[string][]byte{}
	specNames := []string{"loom-plan-spec"}
	for _, name := range specNames {
		path := stencilstore.Path(specsDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s after first seedStencilsAt: %v", path, err)
		}
		before[path] = content
	}
	discussionPath := filepath.Join(stencilsDir, "loom", "loom-template-discussion.md")
	discussionBefore, err := os.ReadFile(discussionPath)
	if err != nil {
		t.Fatalf("read %s after first seedStencilsAt: %v", discussionPath, err)
	}

	seedStencilsAt(hub.Path, worktree)

	for path, want := range before {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s after second seedStencilsAt: %v", path, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s changed on a second seedStencilsAt run; want it byte-identical to the first run's output", path)
		}
	}
	discussionAfter, err := os.ReadFile(discussionPath)
	if err != nil {
		t.Fatalf("read %s after second seedStencilsAt: %v", discussionPath, err)
	}
	if string(discussionAfter) != string(discussionBefore) {
		t.Errorf("%s changed on a second seedStencilsAt run; want it byte-identical to the first run's output", discussionPath)
	}

	status := gitkit.GitStatusPorcelain(t, hub.BoardDir())
	if status != "" {
		t.Errorf("git status --porcelain in %s after a second seedStencilsAt run = %q; want a clean tree", hub.BoardDir(), status)
	}
}

// TestSeedStencilsAt_CommitsTheSpecsSubtree asserts the seeded specs files are tracked in the board
// repository after the first call -- not merely present on disk -- via git ls-files rather than
// os.Stat, which is the assertion that would have caught the write-but-never-stage defect the
// generalisation in batch 3 exists to fix.
func TestSeedStencilsAt_CommitsTheSpecsSubtree(t *testing.T) {
	hub := hubforge.NewHub(t, ".")

	seedStencilsAt(hub.Path, hub.PrimeWorktree())

	specsSubtreeRel := fabricengine.SpecsSubtreeRel()
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"ls-files", specsSubtreeRel}, hub.BoardDir())
	if err != nil || exitCode != 0 {
		t.Fatalf("git ls-files %s (in %s) failed: %v (exit code %d); stderr: %s", specsSubtreeRel, hub.BoardDir(), err, exitCode, stderr)
	}
	if stdout == "" {
		t.Fatalf("git ls-files %s (in %s) returned nothing; want the seeded specs files to be tracked", specsSubtreeRel, hub.BoardDir())
	}

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	specNames := []string{"loom-plan-spec"}
	for _, name := range specNames {
		relPath := stencilstore.RelPath(name)
		want := filepath.ToSlash(filepath.Join(specsSubtreeRel, relPath))
		if !slices.Contains(lines, want) {
			t.Errorf("git ls-files %s (in %s) = %q; want it to list %s", specsSubtreeRel, hub.BoardDir(), stdout, want)
		}
	}
}
