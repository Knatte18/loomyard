//go:build integration

// drift_test.go covers the stateless PairInSync drift check for warp topology.
// Tests verify in-sync, branch-divergence, and broken-junction cases over real
// host+weft worktrees.

package warp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestPairInSync_InSync verifies that a paired host+weft worktree in sync reports ok=true.
func TestPairInSync_InSync(t *testing.T) {
	t.Parallel()

	const slug = "pair-in-sync-test"

	f := lyxtest.CopyPairedLocal(t)

	// Create a paired worktree via Add.
	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Wire junctions for the pair.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions(%q): %v", slug, err)
	}

	// Resolve layout for the paired host worktree and check pair sync.
	hostWorktreePath := f.Layout.WorktreePath(slug)
	hostLayout, err := f.Layout.Resolve(hostWorktreePath)
	if err != nil {
		t.Fatalf("resolve layout for host: %v", err)
	}

	ok, reason, err := PairInSync(hostLayout)
	if err != nil {
		t.Fatalf("PairInSync: %v", err)
	}
	if !ok {
		t.Errorf("PairInSync() = (false, %q); want (true, '', nil)", reason)
	}
}

// TestPairInSync_BranchDivergence verifies that mismatched host and weft branches
// are detected and reported as out-of-sync.
func TestPairInSync_BranchDivergence(t *testing.T) {
	t.Parallel()

	const slug = "divergence-test"

	f := lyxtest.CopyPairedLocal(t)

	// Create a paired worktree via Add.
	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Wire junctions.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions(%q): %v", slug, err)
	}

	// Manually switch the host worktree to a different branch (checkout main).
	hostWorktreePath := f.Layout.WorktreePath(slug)
	lyxtest.MustRun(t, hostWorktreePath, "git", "checkout", "main")

	// Resolve layout for the host and check pair sync.
	hostLayout, err := f.Layout.Resolve(hostWorktreePath)
	if err != nil {
		t.Fatalf("resolve layout for host: %v", err)
	}

	ok, reason, err := PairInSync(hostLayout)
	if err != nil {
		t.Fatalf("PairInSync: %v", err)
	}
	if ok {
		t.Errorf("PairInSync() = (true, '', nil); want out-of-sync due to branch divergence")
	}
	if !strings.Contains(reason, "host on") || !strings.Contains(reason, "weft on") {
		t.Errorf("PairInSync reason = %q; want branch divergence message", reason)
	}
}

// TestPairInSync_BrokenJunction verifies that a missing or broken host junction
// is detected and reported as out-of-sync.
func TestPairInSync_BrokenJunction(t *testing.T) {
	t.Parallel()

	const slug = "broken-junction-test"

	f := lyxtest.CopyPairedLocal(t)

	// Create a paired worktree via Add.
	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Wire junctions.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions(%q): %v", slug, err)
	}

	// Remove the host junction to simulate a broken pair.
	hostLayout, err := f.Layout.Resolve(f.Layout.WorktreePath(slug))
	if err != nil {
		t.Fatalf("resolve layout: %v", err)
	}

	hostLink := hostLayout.HostLyxLinkHere()
	if err := fslink.Remove(hostLink); err != nil {
		t.Fatalf("remove junction: %v", err)
	}

	// Check pair sync; should report missing junction.
	ok, reason, err := PairInSync(hostLayout)
	if err != nil {
		t.Fatalf("PairInSync: %v", err)
	}
	if ok {
		t.Errorf("PairInSync() = (true, '', nil); want out-of-sync due to missing junction")
	}
	if !strings.Contains(reason, "junction") {
		t.Errorf("PairInSync reason = %q; want junction message", reason)
	}
}

// TestPairInSync_JunctionPointsElsewhere verifies that a junction pointing to
// the wrong target is detected and reported as out-of-sync.
func TestPairInSync_JunctionPointsElsewhere(t *testing.T) {
	t.Parallel()

	const slug = "wrong-target-test"

	f := lyxtest.CopyPairedLocal(t)

	// Create a paired worktree via Add.
	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Wire junctions.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions(%q): %v", slug, err)
	}

	// Resolve layout and create a fake weft target directory.
	hostLayout, err := f.Layout.Resolve(f.Layout.WorktreePath(slug))
	if err != nil {
		t.Fatalf("resolve layout: %v", err)
	}

	// Create a decoy directory.
	decoyTarget := filepath.Join(f.Layout.Hub, "decoy-weft-lyx")
	if err := os.MkdirAll(decoyTarget, 0755); err != nil {
		t.Fatalf("mkdir decoy: %v", err)
	}

	// Remove the junction and re-create it pointing to the decoy.
	hostLink := hostLayout.HostLyxLinkHere()
	if err := fslink.Remove(hostLink); err != nil {
		t.Fatalf("remove junction: %v", err)
	}
	if err := fslink.CreateDirLink(hostLink, decoyTarget); err != nil {
		t.Fatalf("create wrong junction: %v", err)
	}

	// Check pair sync; should report junction pointing elsewhere.
	ok, reason, err := PairInSync(hostLayout)
	if err != nil {
		t.Fatalf("PairInSync: %v", err)
	}
	if ok {
		t.Errorf("PairInSync() = (true, '', nil); want out-of-sync due to wrong junction target")
	}
	if !strings.Contains(reason, "junction") && !strings.Contains(reason, "elsewhere") {
		t.Errorf("PairInSync reason = %q; want junction target message", reason)
	}
}
