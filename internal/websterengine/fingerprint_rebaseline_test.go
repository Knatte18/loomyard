//go:build integration

// fingerprint_rebaseline_test.go exercises RebaselinePlanFingerprint (Tier 2): it builds on
// runlevel_test.go's run fixture, which is backed by a real scratch git repo.

package websterengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestRebaselinePlanFingerprint_RestampsStaleFingerprintKeepingBatches proves an existing
// state.json with a stale fingerprint is restamped to the plan's current digest and its batch
// records survive untouched.
func TestRebaselinePlanFingerprint_RestampsStaleFingerprintKeepingBatches(t *testing.T) {
	fx := newRunFixture(t, 2)
	geom := fx.Deps.Geom

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
			2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done"},
		},
	})

	overview := filepath.Join(fx.PlanDir, "00-overview.md")
	data, err := os.ReadFile(overview)
	if err != nil {
		t.Fatalf("read overview: %v", err)
	}
	if err := os.WriteFile(overview, append(data, []byte("Extra line.\n")...), 0o644); err != nil {
		t.Fatalf("rewrite overview: %v", err)
	}
	want, err := websterengine.Fingerprint(fx.PlanDir)
	if err != nil {
		t.Fatalf("Fingerprint() error = %v", err)
	}

	if err := websterengine.RebaselinePlanFingerprint(geom); err != nil {
		t.Fatalf("RebaselinePlanFingerprint() error = %v; want nil", err)
	}

	st, err := websterengine.LoadState(geom.WebsterDir, geom.ScratchDir)
	if err != nil || st == nil {
		t.Fatalf("LoadState() = %v, %v; want a state", st, err)
	}
	if st.PlanFingerprint != want {
		t.Errorf("PlanFingerprint = %q; want the current digest %q", st.PlanFingerprint, want)
	}
	for n := 1; n <= 2; n++ {
		bs := st.Batches[n]
		if bs == nil || !bs.Terminal || bs.Status != "done" {
			t.Errorf("batch %d record = %+v; want the untouched terminal done record", n, bs)
		}
	}
}

// TestRebaselinePlanFingerprint_AbsentStateStaysAbsent proves that with no state.json the call is
// a no-op: no error, and no state file is created.
func TestRebaselinePlanFingerprint_AbsentStateStaysAbsent(t *testing.T) {
	fx := newRunFixture(t, 1)
	geom := fx.Deps.Geom

	if err := websterengine.RebaselinePlanFingerprint(geom); err != nil {
		t.Fatalf("RebaselinePlanFingerprint() error = %v; want nil", err)
	}
	if _, err := os.Stat(filepath.Join(geom.WebsterDir, "state.json")); !os.IsNotExist(err) {
		t.Errorf("state.json stat err = %v; want it to stay absent", err)
	}
}
