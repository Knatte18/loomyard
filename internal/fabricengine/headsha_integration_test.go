//go:build integration

// headsha_integration_test.go proves Fabric.HeadSHA tracks the task worktree's HEAD over a real pair.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestFabricHeadSHA_TracksWorktreeHead asserts HeadSHA equals git rev-parse HEAD of the task worktree
// before and after a new commit.
func TestFabricHeadSHA_TracksWorktreeHead(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	f, err := fabricengine.Open(fixture.Layout)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}
	dir := fixture.Layout.WorktreePath()

	before, err := f.HeadSHA()
	if err != nil {
		t.Fatalf("HeadSHA (before): %v", err)
	}
	if want := currentSHAOf(t, dir); before != want {
		t.Errorf("HeadSHA before commit = %q; want %q", before, want)
	}

	newSHA := commitFile(t, dir, "headsha.txt", "x", "headsha commit")

	after, err := f.HeadSHA()
	if err != nil {
		t.Fatalf("HeadSHA (after): %v", err)
	}
	if after != newSHA {
		t.Errorf("HeadSHA after commit = %q; want %q", after, newSHA)
	}
	if after == before {
		t.Errorf("HeadSHA did not change after a new commit: %q", after)
	}
}
