//go:build integration

// seed_integration_test.go proves the batten entry's seed-location rule against a real hub: the
// prime-name comparison behind battencli.RefuseUnlessPrime reaches a git worktree listing, which the
// Test Tier Purity Invariant bars from untagged files.

package shedcli

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestWriteSeed_BattenSeedIsRefusedOutsidePrime asserts a batten seed is written in the hub's prime
// worktree and refused in a task worktree, where every batten verb refuses too, leaving no run
// directory behind there.
func TestWriteSeed_BattenSeedIsRefusedOutsidePrime(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "seed-here")

	if err := writeSeed(h.Location, "some-run", shedrun.RecipeBatten, "", nil); err != nil {
		t.Fatalf("writeSeed(prime, batten) = %v; want nil", err)
	}

	taskLocation, err := lyxcwdResolveWorktreeForTest(t, h.PairWarpWorktree("seed-here"))
	if err != nil {
		t.Fatalf("resolve task worktree: %v", err)
	}
	err = writeSeed(taskLocation, "some-run", shedrun.RecipeBatten, "", nil)
	if err == nil {
		t.Fatal("writeSeed(task worktree, batten) = nil; want the prime-only refusal")
	}
	if !strings.Contains(err.Error(), "prime worktree only") {
		t.Errorf("writeSeed(task worktree, batten) = %q; want the prime-only wording", err.Error())
	}
	if _, found, readErr := shedrun.ReadSeed(taskLocation, "some-run"); readErr != nil || found {
		t.Errorf("ReadSeed in the task worktree after the refusal = (found=%v, err=%v); want (false, nil)", found, readErr)
	}

	if err := writeSeed(taskLocation, "some-run", shedrun.RecipeLoom, "", nil); err != nil {
		t.Errorf("writeSeed(task worktree, loom) = %v; want nil: loom seeds wherever its verbs drive", err)
	}
}
