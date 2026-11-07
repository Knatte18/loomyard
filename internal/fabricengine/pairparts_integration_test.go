// pairparts_integration_test.go covers Topology.PairParts, TaskBranchOnOrigin and TaskBranchSlugs over hubforge hubs.
// Every hub has an empty branch_prefix unless a test seeds one,
// so a pair's task branch is the bare slug.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPairParts pins what PairParts reads of a pair across its life:
// what TaskBranchOnOrigin reads of the task repository's origin.
func TestPairParts(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	all := fabricengine.PairParts{TaskWorktree: true, RecordsWorktree: true, RecordsBranch: true, TaskBranch: true, Launchers: true}

	t.Run("AfterAddEveryPartIsPresent", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, "pp-added", fabricengine.AddOptions{})

		got, err := h.Topology.PairParts(h.Location, "pp-added")
		if err != nil {
			t.Fatalf("PairParts: %v", err)
		}
		if got != all || !got.Any() {
			t.Errorf("PairParts() = %+v; want %+v", got, all)
		}
	})

	t.Run("AfterRemoveOfUnlandedPairOnlyTaskBranchRemains", func(t *testing.T) {
		t.Parallel()
		const slug = "pp-unlanded"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
		warpBranchCommit(t, h.Location, slug, "work.txt")
		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove(%q): %v", slug, err)
		}
		gitkit.MustRun(t, h.PrimeWorktree(), "git", "push", "origin", slug)

		got, err := h.Topology.PairParts(h.Location, slug)
		if err != nil {
			t.Fatalf("PairParts: %v", err)
		}
		if want := (fabricengine.PairParts{TaskBranch: true}); got != want {
			t.Errorf("PairParts() = %+v; want %+v", got, want)
		}
		onOrigin, err := h.Topology.TaskBranchOnOrigin(h.Location, slug)
		if err != nil || !onOrigin {
			t.Errorf("TaskBranchOnOrigin() = %v, %v; want true, nil", onOrigin, err)
		}
	})

	t.Run("PlainDirectoryAtTheTaskWorktreePathReadsAsStrayPathOnly", func(t *testing.T) {
		t.Parallel()
		const slug = "pp-stray"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		if err := os.MkdirAll(filepath.Join(fabricengine.WorktreePath(h.Location, slug), "obj"), 0o755); err != nil {
			t.Fatalf("plant a plain directory at the task worktree path: %v", err)
		}

		got, err := h.Topology.PairParts(h.Location, slug)
		if err != nil {
			t.Fatalf("PairParts: %v", err)
		}
		if want := (fabricengine.PairParts{StrayPath: true}); got != want || !got.Any() {
			t.Errorf("PairParts() = %+v, Any() = %v; want %+v, true", got, got.Any(), want)
		}
	})

	t.Run("NeverAddedSlugReadsNothing", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

		got, err := h.Topology.PairParts(h.Location, "pp-never")
		if err != nil {
			t.Fatalf("PairParts: %v", err)
		}
		if got.Any() {
			t.Errorf("PairParts() = %+v; want no part", got)
		}
		onOrigin, err := h.Topology.TaskBranchOnOrigin(h.Location, "pp-never")
		if err != nil || onOrigin {
			t.Errorf("TaskBranchOnOrigin() = %v, %v; want false, nil", onOrigin, err)
		}
	})

	t.Run("NoOriginRemoteReadsFalse", func(t *testing.T) {
		t.Parallel()
		const slug = "pp-noorigin"
		h := pushedPair(t, slug)
		gitkit.MustRun(t, h.PrimeWorktree(), "git", "remote", "remove", "origin")

		onOrigin, err := h.Topology.TaskBranchOnOrigin(h.Location, slug)
		if err != nil || onOrigin {
			t.Errorf("TaskBranchOnOrigin() = %v, %v; want false, nil", onOrigin, err)
		}
	})

	t.Run("UnreachableOriginIsAnError", func(t *testing.T) {
		t.Parallel()
		const slug = "pp-unreachable"
		h := pushedPair(t, slug)
		gitkit.MustRun(t, h.PrimeWorktree(), "git", "remote", "set-url", "origin", t.TempDir()+"/no-such-origin")

		if onOrigin, err := h.Topology.TaskBranchOnOrigin(h.Location, slug); err == nil {
			t.Errorf("TaskBranchOnOrigin() = %v, nil; want an error", onOrigin)
		}
	})
}

// TestTaskBranchSlugs pins which slugs of a prefix TaskBranchSlugs reads:
// from the local branches and the origin remote-tracking branches, under the empty branch_prefix and under a set one.
func TestTaskBranchSlugs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// branchPrefix is the hub's branch_prefix; an empty value seeds none.
		branchPrefix string
		prefix       string
		want         []string
	}{
		{"LocalAndOriginTrackingBranches", "", "adopt-fix-x-t", []string{"adopt-fix-x-t1", "adopt-fix-x-t3"}},
		{"NoMatch", "", "nothing-", nil},
		{"BareAndPrefixedBranchesUnderABranchPrefix", "hanf/", "adopt-fix-x-t", []string{"adopt-fix-x-t1", "adopt-fix-x-t3", "adopt-fix-x-t4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			if tt.branchPrefix != "" {
				hubforge.SeedFabricConfig(t, h, "branch_prefix: "+tt.branchPrefix+"\npathspec: \"\"\n")
			}
			prime := h.PrimeWorktree()
			gitkit.MustRun(t, prime, "git", "branch", "adopt-fix-x-t1")
			gitkit.MustRun(t, prime, "git", "branch", "other")
			gitkit.MustRun(t, prime, "git", "branch", "adopt-fix-x-t3")
			gitkit.MustRun(t, prime, "git", "push", "origin", "adopt-fix-x-t3")
			gitkit.MustRun(t, prime, "git", "branch", "-D", "adopt-fix-x-t3")
			gitkit.MustRun(t, prime, "git", "branch", "hanf/adopt-fix-x-t4")

			got, err := fabricengine.TaskBranchSlugs(h.Location, tt.prefix)
			if err != nil {
				t.Fatalf("TaskBranchSlugs(%q): %v", tt.prefix, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("TaskBranchSlugs(%q) = %v; want %v", tt.prefix, got, tt.want)
			}
		})
	}
}
