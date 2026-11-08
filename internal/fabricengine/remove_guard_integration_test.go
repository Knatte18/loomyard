// remove_guard_integration_test.go pins a guard of `lym fabric remove`:
// it keeps the command from deleting a directory that git itself declined to remove.
// Remove must not fall back to an unconditional os.RemoveAll when git refuses a path.
// Such a path includes the main working tree and any path that is not a linked worktree of the repo.
//
// It shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRemove_RefusesForeignWorktreeWithoutDeletingIt asserts Remove refuses a foreign worktree.
// The directory is a clean git worktree of the WEFT repo, not of warp.
// The warp repo refuses it with "not a working tree", the exact shape of the hub's Bolt checkout.
// Remove must not delete it, though it holds .lym-anchor, .lym-warp and the repo-wide fabric.yaml.
// The slug is not reserved, so this test exercises removeWarpWorktreeDir's registered-linked-worktree rule.
// It does not reach the slug guard in front of that rule.
func TestRemove_RefusesForeignWorktreeWithoutDeletingIt(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	const slug = "sidecar"
	foreign := filepath.Join(l.HubPath, slug)
	gitkit.MustRun(t, h.PrimeRecords(), "git", "worktree", "add", "--detach", foreign)

	marker := filepath.Join(foreign, "content-marker")
	gitkit.CommitFile(t, foreign, "content-marker", "keep me\n", "seed marker")

	topology := h.Topology
	var removeCalls, sleepCalls int
	fabricengine.SetLeftoverSeamForTest(topology,
		func(container, target string) (bool, bool, error) {
			removeCalls++
			return false, false, nil
		},
		func(time.Duration) { sleepCalls++ })
	_, err := topology.Remove(l, slug, false, false)
	if err == nil {
		t.Fatalf("Remove(%q) = nil error; want a refusal naming git's own reason", slug)
	}
	if removeCalls != 0 || sleepCalls != 0 {
		t.Errorf("leftover seam ran remove %d and sleep %d times; want neither, since the gate refused before git ran", removeCalls, sleepCalls)
	}
	if !strings.Contains(err.Error(), "not a linked worktree of this repo") {
		t.Errorf("Remove(%q) error = %v; want it to say fabric will not delete a directory git declined to remove", slug, err)
	}

	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("foreign worktree content at %s was destroyed by a refused Remove: %v", marker, statErr)
	}
}
