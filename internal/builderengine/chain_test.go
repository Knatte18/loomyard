//go:build integration

// chain_test.go covers ChainMembers/ChainEndFor against the plan-valid
// fixture's batch 03/04 deferred-verify chain, and RestartChain end-to-end
// against a scratch git repo (Tier 2 — see docs/benchmarks/running-
// tests.md): a recorded anchor rolls the repo back and clears the chain's
// reports, while a chainless or unrecorded-anchor request errors instead
// of guessing a reset target.

package builderengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
)

func TestChainMembers(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "plan-valid")
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	got := builderengine.ChainMembers(plan, 4)
	want := []int{3, 4}
	if len(got) != len(want) {
		t.Fatalf("ChainMembers(plan, 4) = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ChainMembers(plan, 4)[%d] = %d; want %d", i, got[i], want[i])
		}
	}

	if got := builderengine.ChainMembers(plan, 1); got != nil {
		t.Errorf("ChainMembers(plan, 1) = %v; want nil (batch 1 names no chain)", got)
	}
}

func TestChainEndFor(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "plan-valid")
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	tests := []struct {
		batch int
		want  int
	}{
		{3, 4}, // intermediate: declares its own chain-end
		{4, 4}, // the chain-end batch itself
		{1, 0}, // chainless
	}
	for _, tt := range tests {
		if got := builderengine.ChainEndFor(plan, tt.batch); got != tt.want {
			t.Errorf("ChainEndFor(plan, %d) = %d; want %d", tt.batch, got, tt.want)
		}
	}
}

func TestRestartChain(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "plan-valid")
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

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

	st := &builderengine.State{
		CurrentBatch: 3,
		Batches: map[int]*builderengine.BatchState{
			3: {Slug: "refactor-a", StartSHA: anchor},
		},
		ChainStartSHAs: map[int]string{4: anchor},
	}

	if err := builderengine.RestartChain(worktree, st, plan, 4, reportsDir); err != nil {
		t.Fatalf("RestartChain() error = %v; want nil", err)
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
		t.Errorf("st.Batches[3] still present after RestartChain; want reset")
	}
	if st.CurrentBatch != 0 {
		t.Errorf("st.CurrentBatch = %d after RestartChain; want 0", st.CurrentBatch)
	}
}

func TestRestartChain_ChainlessErrors(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "plan-valid")
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	worktree := newScratchRepo(t)
	anchor := commitFile(t, worktree, "base.txt", "base", "base commit")

	// The anchor IS recorded for chainEnd 1, isolating the chainless check
	// from the unrecorded-anchor check exercised below.
	st := &builderengine.State{ChainStartSHAs: map[int]string{1: anchor}}
	if err := builderengine.RestartChain(worktree, st, plan, 1, t.TempDir()); err == nil {
		t.Errorf("RestartChain(chainEnd=1) error = nil; want error (batch 1 names no chain)")
	}
}

func TestRestartChain_UnrecordedAnchorErrors(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "plan-valid")
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	worktree := newScratchRepo(t)
	commitFile(t, worktree, "base.txt", "base", "base commit")

	st := &builderengine.State{ChainStartSHAs: map[int]string{}}
	if err := builderengine.RestartChain(worktree, st, plan, 4, t.TempDir()); err == nil {
		t.Errorf("RestartChain with no recorded anchor error = nil; want error")
	}
}
