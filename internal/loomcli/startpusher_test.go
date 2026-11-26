// startpusher_test.go pins the target a wired loom run's resume asks a pusher for.

package loomcli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWiredStartPusher_AsksForThePairsPusher(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("task1"))
	if err != nil {
		t.Fatalf("ResolveWorktree(task1) = %v; want nil", err)
	}
	var started []string
	c := &loomCLI{runID: shedrun.SelfRunID, pusherStart: func(target runpusher.Target) (bool, error) {
		if !target.Pair {
			t.Errorf("target.Pair = false; want a pair's target")
		}
		started = append(started, target.Location.WorktreeName+"/"+target.Slug)
		return false, nil
	}}
	if err := c.wire(pair, pair.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	c.startPusher()

	if want := []string{"task1/task1"}; !slices.Equal(started, want) {
		t.Errorf("pushers asked for = %q; want %q", started, want)
	}
}
