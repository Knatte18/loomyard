//go:build integration

// chain_test.go exercises RestartChain end to end against a real scratch git
// repo (Tier 2 — see docs/benchmarks/running-tests.md, mirroring
// builderengine/chain_test.go's own RestartChain coverage): a recorded
// anchor rolls the repo back and clears every chain member's stale report
// and BatchState (digests included), returning the chain's lowest member
// regardless of which member was named; an unrecorded anchor or a chainless
// batch errors instead of guessing a reset target, leaving everything
// untouched.

package websterengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// chainTestPlan returns a literal two-batch plan with a deferred-verify
// chain (batch 3 declares chain-end 4; batch 4 IS the chain-end), mirroring
// builderengine's own testdata/plan-valid chain shape.
func chainTestPlan() *builderengine.Plan {
	return &builderengine.Plan{
		Batches: []builderengine.PlanBatch{
			{Number: 3, Slug: "refactor-a", File: "03-refactor-a.md", ChainEnd: 4},
			{Number: 4, Slug: "refactor-b", File: "04-refactor-b.md"},
		},
	}
}

// TestRestartChain_ResetAndClearMembers proves the full rollback act: naming
// the chain's END member (4) still resets the repo to the recorded anchor,
// deletes every member's stale report, clears every member's BatchState
// (its digest included), zeros CurrentBatch, and returns the chain's LOWEST
// member (3) — never the member the caller actually named.
func TestRestartChain_ResetAndClearMembers(t *testing.T) {
	worktree := newScratchRepo(t)
	anchor := commitFile(t, worktree, "base.txt", "base", "base commit")
	commitFile(t, worktree, "03.md", "batch 03 intermediate work", "03.1: intermediate work")

	reportsDir := t.TempDir()
	reportNames := []string{"03-refactor-a.yaml", "04-refactor-b.yaml"}
	for _, name := range reportNames {
		if err := os.WriteFile(filepath.Join(reportsDir, name), []byte("batch: x\n"), 0o644); err != nil {
			t.Fatalf("seed report %s: %v", name, err)
		}
	}

	st := &websterengine.State{
		CurrentBatch: 3,
		Batches: map[int]*websterengine.BatchState{
			3: {Slug: "refactor-a", StartSHA: anchor, Digest: &builderengine.Digest{Batch: "03-refactor-a", Status: builderengine.DigestStatusDone}},
			4: {Slug: "refactor-b"},
		},
		ChainStartSHAs: map[int]string{4: anchor},
	}

	lowest, err := websterengine.RestartChain(worktree, st, chainTestPlan(), 4, reportsDir)
	if err != nil {
		t.Fatalf("RestartChain() error = %v; want nil", err)
	}
	if lowest != 3 {
		t.Errorf("RestartChain() lowest = %d; want 3 (the chain's lowest member, even though 4 was named)", lowest)
	}

	head, err := builderengine.HeadSHA(worktree)
	if err != nil {
		t.Fatalf("HeadSHA() error = %v; want nil", err)
	}
	if head != anchor {
		t.Errorf("HeadSHA() after RestartChain = %q; want anchor %q", head, anchor)
	}

	for _, name := range reportNames {
		if _, err := os.Stat(filepath.Join(reportsDir, name)); !os.IsNotExist(err) {
			t.Errorf("report %s still present after RestartChain; want removed", name)
		}
	}

	if _, ok := st.Batches[3]; ok {
		t.Errorf("st.Batches[3] still present after RestartChain; want cleared (digest included)")
	}
	if _, ok := st.Batches[4]; ok {
		t.Errorf("st.Batches[4] still present after RestartChain; want cleared")
	}
	if st.CurrentBatch != 0 {
		t.Errorf("st.CurrentBatch = %d after RestartChain; want 0", st.CurrentBatch)
	}
}

// TestRestartChain_UnrecordedAnchorErrors proves an unrecorded chain-start
// SHA is refused rather than guessed, and that the refusal touches nothing:
// the repo is not reset and the seeded report survives untouched.
func TestRestartChain_UnrecordedAnchorErrors(t *testing.T) {
	worktree := newScratchRepo(t)
	base := commitFile(t, worktree, "base.txt", "base", "base commit")

	reportsDir := t.TempDir()
	reportPath := filepath.Join(reportsDir, "03-refactor-a.yaml")
	if err := os.WriteFile(reportPath, []byte("batch: x\n"), 0o644); err != nil {
		t.Fatalf("seed report: %v", err)
	}

	st := &websterengine.State{
		CurrentBatch:   3,
		Batches:        map[int]*websterengine.BatchState{3: {Slug: "refactor-a"}},
		ChainStartSHAs: map[int]string{},
	}

	_, err := websterengine.RestartChain(worktree, st, chainTestPlan(), 4, reportsDir)
	if err == nil {
		t.Fatal("RestartChain() error = nil; want an error for an unrecorded chain-start SHA")
	}

	head, headErr := builderengine.HeadSHA(worktree)
	if headErr != nil {
		t.Fatalf("HeadSHA() error = %v; want nil", headErr)
	}
	if head != base {
		t.Errorf("HeadSHA() = %q after a refused reset; want unchanged %q", head, base)
	}
	if _, statErr := os.Stat(reportPath); statErr != nil {
		t.Errorf("stat(%s) = %v after a refused reset; want the report untouched", reportPath, statErr)
	}
	if _, ok := st.Batches[3]; !ok {
		t.Errorf("st.Batches[3] cleared after a refused reset; want untouched")
	}
	if st.CurrentBatch != 3 {
		t.Errorf("st.CurrentBatch = %d after a refused reset; want unchanged 3", st.CurrentBatch)
	}
}

// TestRestartChain_ChainlessBatchErrors proves a batch naming no
// deferred-verify chain at all is refused before any reset is attempted.
func TestRestartChain_ChainlessBatchErrors(t *testing.T) {
	worktree := newScratchRepo(t)
	commitFile(t, worktree, "base.txt", "base", "base commit")

	plan := &builderengine.Plan{
		Batches: []builderengine.PlanBatch{
			{Number: 1, Slug: "json-flag", File: "01-json-flag.md"},
		},
	}
	st := &websterengine.State{ChainStartSHAs: map[int]string{}}

	_, err := websterengine.RestartChain(worktree, st, plan, 1, t.TempDir())
	if err == nil {
		t.Fatal("RestartChain() error = nil; want an error (batch 1 names no chain)")
	}
}
