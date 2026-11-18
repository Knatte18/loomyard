// hubstop_integration_test.go drives hubStopWorktrees over a real hub of a prime, a beam and a task pair.

package beamcli

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHubStopWorktrees_SplitsBeamsFromTaskPairs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "t", fabricengine.AddOptions{SkipPush: true})
	prime, err := lymcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("resolve the prime: %v", err)
	}

	beams, tasks, err := hubStopWorktrees(prime)

	if err != nil {
		t.Fatalf("hubStopWorktrees() error = %v; want none", err)
	}
	names := func(locations []*lymcwd.Location) []string {
		var got []string
		for _, location := range locations {
			got = append(got, location.WorktreeName)
		}
		return got
	}
	wantBeams := []string{filepath.Base(h.PairCodeWorktree("b"))}
	wantTasks := []string{filepath.Base(h.PairCodeWorktree("t"))}
	if got := names(beams); !slices.Equal(got, wantBeams) {
		t.Errorf("hubStopWorktrees() beams = %v; want %v", got, wantBeams)
	}
	if got := names(tasks); !slices.Equal(got, wantTasks) {
		t.Errorf("hubStopWorktrees() tasks = %v; want %v", got, wantTasks)
	}
}
