//go:build tier1

// add_adoptlocal_integration_test.go covers Add with AddOptions.AdoptLocal over a hubforge hub: the adoption of an unmarked local branch, the refusals that precede any mutation and the root pair of the default branch.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestAdd_AdoptLocal(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	prime := h.PrimeWorktree()
	defaultBranch := gitOut(t, prime, "rev-parse", "--abbrev-ref", "HEAD")
	addAdopting := func(acting *lyxcwd.Location, slug string) (fabricengine.AddResult, error) {
		return h.Topology.Add(acting, slug, fabricengine.AddOptions{SkipPush: true, AdoptLocal: true})
	}
	// branchPrefix is what a pair's slug is prefixed with to name its warp branch.
	probe := hubforge.AddPairWith(t, h, "probe", fabricengine.AddOptions{SkipPush: true})
	branchPrefix := strings.TrimSuffix(probe.Branch, "probe")

	// commitOnNewBranch makes branch with one commit on top of the default branch, leaving the prime on the default branch.
	commitOnNewBranch := func(t *testing.T, branch string) string {
		t.Helper()
		gitOut(t, prime, "switch", "-c", branch)
		gitkit.CommitFile(t, prime, branch+".txt", branch+"\n", "commit on "+branch)
		sha := gitOut(t, prime, "rev-parse", "HEAD")
		gitOut(t, prime, "switch", defaultBranch)
		return sha
	}

	t.Run("adopts an unmarked local branch only when asked", func(t *testing.T) {
		const slug = "x"
		commit := commitOnNewBranch(t, branchPrefix+slug)

		_, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
		var exists *fabricengine.ErrBranchExists
		if !errors.As(err, &exists) {
			t.Fatalf("Add(%q) without AdoptLocal error = %v; want *ErrBranchExists", slug, err)
		}

		if _, err := addAdopting(l, slug); err != nil {
			t.Fatalf("Add(%q, AdoptLocal): %v", slug, err)
		}
		if got := gitOut(t, fabricengine.WorktreePath(l, slug), "rev-parse", "HEAD"); got != commit {
			t.Errorf("adopted pair HEAD = %s; want the branch's commit %s", got, commit)
		}
		origin, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found {
			t.Fatalf("ReadOriginFor(%q) = (%+v, %v, %v); want a record", slug, origin, found, err)
		}
		want := fabricengine.Origin{ParentBranch: defaultBranch, ParentWorktree: l.WorktreeName}
		if origin != want {
			t.Errorf("adopted pair origin record = %+v; want %+v", origin, want)
		}
	})

	t.Run("refuses a branch another worktree holds", func(t *testing.T) {
		const slug = "held"
		commitOnNewBranch(t, branchPrefix+slug)
		elsewhere := filepath.Join(t.TempDir(), "elsewhere")
		gitOut(t, prime, "worktree", "add", elsewhere, branchPrefix+slug)

		_, err := addAdopting(l, slug)
		if err == nil {
			t.Fatalf("Add(%q, AdoptLocal) over a checked-out branch = nil; want a refusal", slug)
		}
		for _, want := range []string{filepath.Base(elsewhere), "lyx fabric checkout"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Add(%q, AdoptLocal) error = %v; want it to contain %q", slug, err, want)
			}
		}
	})

	t.Run("refuses a records branch another records worktree holds", func(t *testing.T) {
		const slug = "records-held"
		commitOnNewBranch(t, branchPrefix+slug)
		recordsRoot, err := fabricengine.RecordsRepoRoot(l)
		if err != nil {
			t.Fatalf("RecordsRepoRoot: %v", err)
		}
		recordsBranch := fabricengine.RecordsBranchName(branchPrefix + slug)
		elsewhere := filepath.Join(t.TempDir(), "records-elsewhere")
		gitOut(t, recordsRoot, "worktree", "add", "-b", recordsBranch, elsewhere)

		_, err = addAdopting(l, slug)
		if err == nil {
			t.Fatalf("Add(%q, AdoptLocal) over a held records branch = nil; want a refusal", slug)
		}
		for _, want := range []string{filepath.Base(elsewhere), "git switch --detach"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Add(%q, AdoptLocal) error = %v; want it to contain %q", slug, err, want)
			}
		}
	})

	t.Run("the default branch makes a root pair", func(t *testing.T) {
		slug := strings.TrimPrefix(defaultBranch, branchPrefix)
		acting, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(l, "probe"))
		if err != nil {
			t.Fatalf("ResolveWorktree of the probe pair: %v", err)
		}
		// The default branch and its records branch are checked out nowhere once the prime and its records worktree let go of them.
		gitOut(t, prime, "switch", "--detach")
		gitOut(t, h.PrimeRecords(), "switch", "--detach")

		if _, err := addAdopting(acting, slug); err != nil {
			t.Fatalf("Add(%q, AdoptLocal) over the default branch: %v", slug, err)
		}
		origin, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found {
			t.Fatalf("ReadOriginFor(%q) = (%+v, %v, %v); want a record", slug, origin, found, err)
		}
		if origin.ParentBranch != "" || origin.ParentWorktree != "" {
			t.Errorf("default-branch pair origin record = %+v; want empty parent branch and worktree", origin)
		}
	})
}
