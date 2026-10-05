//go:build integration

// resetpairwarp_integration_test.go covers Fabric.ResetPairWarp, the gated reset of a task pair's warp checkout:
// a reset past later commits and own-path dirt that keeps untracked files and records one worktree_reset entry,
// a refusal on dirt outside the own paths, and the four ownership refusals.
//
// Every hub is built through hubforge.NewHub with an empty branch_prefix, so the pair's warp branch is the bare slug.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// pairFabric adds a task pair for slug to h and opens the fabric handle on the pair's warp worktree.
func pairFabric(t *testing.T, h *hubforge.Hub, slug string) (*fabricengine.Fabric, string) {
	t.Helper()

	hubforge.AddPair(t, h, slug)
	warp := h.PairWarpWorktree(slug)
	loc, err := lyxcwd.ResolveWorktree(warp)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", warp, err)
	}
	f, err := fabricengine.Open(loc)
	if err != nil {
		t.Fatalf("Open(%s): %v", warp, err)
	}
	return f, warp
}

// assertResetRefused fails the test unless err is a gate refusal on want that records nothing in rec.
func assertResetRefused(t *testing.T, err error, rec *fabricengine.Mutations, want fabricengine.Check) fabricengine.Refusal {
	t.Helper()

	refusal, ok := fabricengine.RefusalOf(err)
	if !ok {
		t.Fatalf("ResetPairWarp error = %v; want a gate refusal", err)
	}
	if refusal.Check != want {
		t.Errorf("refusal.Check = %s; want %s (reason %q)", refusal.Check, want, refusal.Reason)
	}
	if got := len(rec.Entries()); got != 0 {
		t.Errorf("refusal recorded %d entries; want 0", got)
	}
	return refusal
}

func TestResetPairWarp_DiscardsCommitsAndOwnPathDirtKeepsUntracked(t *testing.T) {
	t.Parallel()

	const slug = "rpw-reset"
	h := hubforge.NewHub(t, ".")
	f, warp := pairFabric(t, h, slug)

	gitkit.CommitFile(t, warp, "own.txt", "base", "own base")
	older := gitkit.RevParse(t, warp, "HEAD")
	gitkit.CommitFile(t, warp, "later.txt", "later", "later commit")

	if err := os.WriteFile(filepath.Join(warp, "own.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	untracked := filepath.Join(warp, "untracked.txt")
	if err := os.WriteFile(untracked, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := fabricengine.NewMutations("")
	if err := f.ResetPairWarp(rec, older, "main", []string{"own.txt"}); err != nil {
		t.Fatalf("ResetPairWarp: %v", err)
	}

	if got := gitkit.RevParse(t, warp, "HEAD"); got != older {
		t.Errorf("HEAD after reset = %q; want %q", got, older)
	}
	if _, err := os.Stat(filepath.Join(warp, "later.txt")); !os.IsNotExist(err) {
		t.Errorf("later.txt still present after reset (err=%v)", err)
	}
	if got, err := os.ReadFile(filepath.Join(warp, "own.txt")); err != nil || string(got) != "base" {
		t.Errorf("own.txt = %q, %v; want %q", got, err, "base")
	}
	if _, err := os.Stat(untracked); err != nil {
		t.Errorf("untracked file removed by reset: %v", err)
	}

	entries := rec.Entries()
	if len(entries) != 1 || entries[0].Kind != fabricengine.KindWorktreeReset {
		t.Fatalf("entries = %+v; want exactly one %s", entries, fabricengine.KindWorktreeReset)
	}
}

func TestResetPairWarp_DirtyPathOutsideOwnPathsRefuses(t *testing.T) {
	t.Parallel()

	const slug = "rpw-foreign"
	h := hubforge.NewHub(t, ".")
	f, warp := pairFabric(t, h, slug)

	gitkit.CommitFile(t, warp, "own.txt", "base", "own base")
	gitkit.CommitFile(t, warp, "foreign.txt", "base", "foreign base")
	older := gitkit.RevParse(t, warp, "HEAD~1")
	head := gitkit.RevParse(t, warp, "HEAD")

	foreign := filepath.Join(warp, "foreign.txt")
	if err := os.WriteFile(foreign, []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(warp, "own.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := fabricengine.NewMutations("")
	err := f.ResetPairWarp(rec, older, "main", []string{"own.txt"})
	refusal := assertResetRefused(t, err, rec, fabricengine.CheckDirtiness)
	if !strings.Contains(refusal.Reason, "foreign.txt") {
		t.Errorf("refusal reason %q does not name foreign.txt", refusal.Reason)
	}
	if strings.Contains(refusal.Reason, "own.txt") {
		t.Errorf("refusal reason %q names the exempt own.txt", refusal.Reason)
	}

	if got := gitkit.RevParse(t, warp, "HEAD"); got != head {
		t.Errorf("HEAD moved to %q; want %q", got, head)
	}
	if got, err := os.ReadFile(foreign); err != nil || string(got) != "dirty" {
		t.Errorf("foreign.txt = %q, %v; want the dirt untouched", got, err)
	}
}

func TestResetPairWarp_OwnershipRefusals(t *testing.T) {
	t.Parallel()

	t.Run("PrimeCheckout", func(t *testing.T) {
		t.Parallel()
		h := hubforge.NewHub(t, ".")
		f, err := fabricengine.Open(h.Location)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		head := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD")
		rec := fabricengine.NewMutations("")
		assertResetRefused(t, f.ResetPairWarp(rec, head, "other", nil), rec, fabricengine.CheckOwnership)
	})

	t.Run("PairOnParentBranch", func(t *testing.T) {
		t.Parallel()
		const slug = "rpw-parent"
		h := hubforge.NewHub(t, ".")
		f, warp := pairFabric(t, h, slug)
		head := gitkit.RevParse(t, warp, "HEAD")
		rec := fabricengine.NewMutations("")
		assertResetRefused(t, f.ResetPairWarp(rec, head, slug, nil), rec, fabricengine.CheckOwnership)
	})

	t.Run("WeftOnAnotherBranch", func(t *testing.T) {
		t.Parallel()
		const slug = "rpw-otherweft"
		h := hubforge.NewHub(t, ".")
		f, warp := pairFabric(t, h, slug)
		gitkit.MustRun(t, h.PairWeftSibling(slug), "git", "checkout", "-b", "some-other-branch")
		head := gitkit.RevParse(t, warp, "HEAD")
		rec := fabricengine.NewMutations("")
		assertResetRefused(t, f.ResetPairWarp(rec, head, "main", nil), rec, fabricengine.CheckOwnership)
	})

	t.Run("DetachedHead", func(t *testing.T) {
		t.Parallel()
		const slug = "rpw-detached"
		h := hubforge.NewHub(t, ".")
		f, warp := pairFabric(t, h, slug)
		gitkit.MustRun(t, warp, "git", "checkout", "--detach")
		head := gitkit.RevParse(t, warp, "HEAD")
		rec := fabricengine.NewMutations("")
		assertResetRefused(t, f.ResetPairWarp(rec, head, "main", nil), rec, fabricengine.CheckOwnership)
	})
}
