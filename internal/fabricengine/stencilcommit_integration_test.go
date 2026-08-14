//go:build integration

// stencilcommit_integration_test.go covers CommitSeededStencils against a real hubforge hub: the
// no-op empty-input case, the scoped-pathspec commit (including the seeded .gitattributes), the
// regression guard that an unrelated dirty board file is never swept into the commit, the mutation
// record the commit produces, and that the verb never pushes.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestCommitSeededStencils_EmptyInputIsNoOp asserts an empty writtenRelPaths list takes no lock,
// commits nothing, and leaves the record empty.
func TestCommitSeededStencils_EmptyInputIsNoOp(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	rec := fabricengine.NewMutations(filepath.Dir(hub.Path))

	res, err := fabricengine.CommitSeededStencils(hub.Path, nil, "lyx: seed stencils", rec)
	if err != nil {
		t.Fatalf("CommitSeededStencils(nil) error = %v", err)
	}
	if res.Committed {
		t.Errorf("CommitSeededStencils(nil) Committed = true; want false")
	}
	if rec.Len() != 0 {
		t.Errorf("CommitSeededStencils(nil) recorded %d mutations; want 0", rec.Len())
	}
}

// TestCommitSeededStencils_ScopedCommitExcludesUnrelatedDirt asserts a non-empty writtenRelPaths
// list lands exactly one commit whose changed-file set is confined to the stencils subtree
// (including a seeded .gitattributes), leaves an unrelated dirty board file untouched, records
// file_written and commit_created entries, and never pushes.
func TestCommitSeededStencils_ScopedCommitExcludesUnrelatedDirt(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	board := gitrepo.New(hub.BoardDir())

	// Push whatever hubforge already committed so HasUnpushed starts false, proving below that
	// CommitSeededStencils' own commit -- not some pre-existing unpushed state -- is what makes it
	// true again.
	if err := board.PushCoalesced(); err != nil {
		t.Fatalf("PushCoalesced() (pre-seed) error = %v", err)
	}
	if unpushed, err := board.HasUnpushed(); err != nil {
		t.Fatalf("HasUnpushed() (pre-seed) error = %v", err)
	} else if unpushed {
		t.Fatalf("HasUnpushed() (pre-seed) = true; want false after PushCoalesced")
	}

	// An unrelated dirty file elsewhere in the board repo, never staged by CommitSeededStencils.
	unrelatedPath := filepath.Join(hub.BoardDir(), "unrelated.txt")
	if err := os.WriteFile(unrelatedPath, []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write unrelated dirty file: %v", err)
	}

	stencilsDir := fabricengine.StencilsDir(hub.Path)
	if err := os.MkdirAll(filepath.Join(stencilsDir, "loom"), 0o755); err != nil {
		t.Fatalf("mkdir stencils dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stencilsDir, "loom", "loom-template-discussion.md"), []byte("stencil content\n"), 0o644); err != nil {
		t.Fatalf("write stencil: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stencilsDir, ".gitattributes"), []byte("*.md text eol=lf\n"), 0o644); err != nil {
		t.Fatalf("write .gitattributes: %v", err)
	}
	writtenRelPaths := []string{"loom/loom-template-discussion.md", ".gitattributes"}

	beforeSHA, err := board.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (before) error = %v", err)
	}

	rec := fabricengine.NewMutations(filepath.Dir(hub.Path))
	res, err := fabricengine.CommitSeededStencils(hub.Path, writtenRelPaths, "lyx: seed stencils", rec)
	if err != nil {
		t.Fatalf("CommitSeededStencils() error = %v", err)
	}
	if !res.Committed {
		t.Fatalf("CommitSeededStencils() Committed = false; want true")
	}
	if res.SHA == "" {
		t.Fatalf("CommitSeededStencils() SHA is empty; want a landed commit SHA")
	}

	changed, err := board.ChangedFilesSince(beforeSHA)
	if err != nil {
		t.Fatalf("ChangedFilesSince() error = %v", err)
	}
	if len(changed) == 0 {
		t.Fatalf("ChangedFilesSince() = empty; want the seeded stencil paths")
	}
	for _, path := range changed {
		if !strings.HasPrefix(path, "_lyx/stencils/") {
			t.Errorf("ChangedFilesSince() includes %q; want every changed path confined to _lyx/stencils/ -- the regression guard against a stage-all commit sweeping in the unrelated dirty file", path)
		}
	}

	// The unrelated dirty file must still be untracked on disk after the commit -- the regression
	// guard against reverting to a stage-all commit.
	status := gitkit.GitStatusPorcelain(t, hub.BoardDir())
	found := false
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) == "?? unrelated.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("git status --porcelain = %q; want unrelated.txt still reported as untracked (?? unrelated.txt)", status)
	}

	entries := rec.Snapshot().Entries()
	var sawFileWritten, sawCommitCreated int
	for _, e := range entries {
		switch e.Kind {
		case fabricengine.KindFileWritten:
			sawFileWritten++
		case fabricengine.KindCommitCreated:
			sawCommitCreated++
		}
	}
	if sawFileWritten != len(writtenRelPaths) {
		t.Errorf("recorded %d file_written entries; want %d", sawFileWritten, len(writtenRelPaths))
	}
	if sawCommitCreated != 1 {
		t.Errorf("recorded %d commit_created entries; want 1", sawCommitCreated)
	}

	unpushed, err := board.HasUnpushed()
	if err != nil {
		t.Fatalf("HasUnpushed() (post-seed) error = %v", err)
	}
	if !unpushed {
		t.Errorf("HasUnpushed() (post-seed) = false; want true -- CommitSeededStencils must never push")
	}
}
