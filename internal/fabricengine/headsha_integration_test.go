// headsha_integration_test.go checks that Fabric.HeadSHA and Fabric.RecordsHeadSHA track their checkout's HEAD.
// The check runs against a real paired hub.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestFabricHeadSHA_TracksWorktreeHead asserts that HeadSHA equals git rev-parse HEAD of the task worktree.
// It checks this before and after a new commit, and RecordsHeadSHA does the same for the weft checkout.
func TestFabricHeadSHA_TracksWorktreeHead(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f, err := fabricengine.Open(h.Location)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}
	dir := h.PrimeWorktree()

	before, err := f.HeadSHA()
	if err != nil {
		t.Fatalf("HeadSHA (before): %v", err)
	}
	if want := gitkit.RevParse(t, dir, "HEAD"); before != want {
		t.Errorf("HeadSHA before commit = %q; want %q", before, want)
	}

	newSHA := gitkit.CommitFile(t, dir, "headsha.txt", "x", "headsha commit")

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

	weftDir := h.PrimeRecords()
	weftBefore, err := f.RecordsHeadSHA()
	if err != nil {
		t.Fatalf("RecordsHeadSHA (before): %v", err)
	}
	if want := gitkit.RevParse(t, weftDir, "HEAD"); weftBefore != want {
		t.Errorf("RecordsHeadSHA before commit = %q; want %q", weftBefore, want)
	}

	newWeftSHA := gitkit.CommitFile(t, weftDir, "weftheadsha.txt", "x", "weft headsha commit")

	weftAfter, err := f.RecordsHeadSHA()
	if err != nil {
		t.Fatalf("RecordsHeadSHA (after): %v", err)
	}
	if weftAfter != newWeftSHA {
		t.Errorf("RecordsHeadSHA after commit = %q; want %q", weftAfter, newWeftSHA)
	}
}
