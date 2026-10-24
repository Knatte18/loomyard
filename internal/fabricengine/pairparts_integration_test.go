//go:build tier1

// pairparts_integration_test.go covers Topology.PairParts and Topology.TaskBranchOnOrigin over hubforge hubs.
// Every hub has an empty branch_prefix, so a pair's task branch is the bare slug.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestPairParts pins what PairParts reads of a pair across its life and what TaskBranchOnOrigin reads of the task repository's origin.
func TestPairParts(t *testing.T) {
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
