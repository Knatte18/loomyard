//go:build integration

// siblinglayout_test.go proves that hubgeometry.Layout.SiblingLayout, the
// spawn-free sibling-layout deriver, is byte-for-byte equivalent to Resolve
// for hub-sibling worktree roots, and documents where the two diverge for a
// worktree that lives outside the hub (the guard case card 3's
// hostLayoutFor helper exists to cover).

package hubgeometry_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestSiblingLayout_EquivalentToResolve verifies that SiblingLayout(root)
// produces a Layout deep-equal to Resolve(root) for every worktree that is a
// direct child of the hub — the fast path warpengine's hostLayoutFor takes
// for the normal (hub-sibling) case.
func TestSiblingLayout_EquivalentToResolve(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)

	l, err := hubgeometry.Resolve(fix.Hub)
	if err != nil {
		t.Fatalf("Resolve(fix.Hub) error = %v; want nil", err)
	}

	// Add a second worktree that is a hub sibling: its parent directory is
	// filepath.Dir(fix.Hub), the same container hubgeometry considers Hub.
	siblingPath := filepath.Join(filepath.Dir(fix.Hub), "sibling-wt")
	lyxtest.MustRun(t, fix.Hub, "git", "worktree", "add", siblingPath, "-b", "sibling-branch")

	// Assert equivalence for both the original hub root and the newly added
	// sibling: both are direct children of the same hub container.
	roots := []string{fix.Hub, siblingPath}
	for _, root := range roots {
		t.Run(filepath.Base(root), func(t *testing.T) {
			t.Parallel()

			got := l.SiblingLayout(root)
			want, err := hubgeometry.Resolve(root)
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v; want nil", root, err)
			}

			if !reflect.DeepEqual(*got, *want) {
				t.Errorf("SiblingLayout(%q) = %+v; want %+v (Resolve result)", root, *got, *want)
			}
			// Assert each field individually so a mismatch is legible without
			// diffing the whole struct dump above.
			if got.Cwd != want.Cwd {
				t.Errorf("Cwd = %q; want %q", got.Cwd, want.Cwd)
			}
			if got.WorktreeRoot != want.WorktreeRoot {
				t.Errorf("WorktreeRoot = %q; want %q", got.WorktreeRoot, want.WorktreeRoot)
			}
			if got.Hub != want.Hub {
				t.Errorf("Hub = %q; want %q", got.Hub, want.Hub)
			}
			if got.RelPath != want.RelPath {
				t.Errorf("RelPath = %q; want %q", got.RelPath, want.RelPath)
			}
			if got.Prime != want.Prime {
				t.Errorf("Prime = %q; want %q", got.Prime, want.Prime)
			}
		})
	}
}

// TestSiblingLayout_NonSiblingDiverges documents why hostLayoutFor's guard
// exists: for a worktree root outside the hub, SiblingLayout (which reuses
// the receiver's Hub) and Resolve (which recomputes Hub from
// filepath.Dir(root)) disagree, so callers must not call SiblingLayout
// unconditionally on any worktree returned by List.
func TestSiblingLayout_NonSiblingDiverges(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)

	l, err := hubgeometry.Resolve(fix.Hub)
	if err != nil {
		t.Fatalf("Resolve(fix.Hub) error = %v; want nil", err)
	}

	// Add a worktree whose parent is NOT the hub container, so it is not a
	// hub sibling of fix.Hub.
	outRoot := filepath.Join(t.TempDir(), "outside-wt")
	lyxtest.MustRun(t, fix.Hub, "git", "worktree", "add", outRoot, "-b", "outside-branch")

	resolved, err := hubgeometry.Resolve(outRoot)
	if err != nil {
		t.Fatalf("Resolve(outRoot) error = %v; want nil", err)
	}

	// SiblingLayout reuses l.Hub unconditionally, so for a non-sibling root it
	// disagrees with Resolve's freshly recomputed Hub — this is exactly the
	// divergence hostLayoutFor's filepath.Dir(worktreeRoot) != l.Hub guard exists
	// to catch before it can silently corrupt a call site's geometry.
	got := l.SiblingLayout(outRoot)
	if got.Hub == resolved.Hub {
		t.Errorf("SiblingLayout(outRoot).Hub = %q; want it to diverge from Resolve(outRoot).Hub = %q (outRoot is not a hub sibling)", got.Hub, resolved.Hub)
	}
}
