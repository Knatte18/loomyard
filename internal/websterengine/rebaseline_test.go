//go:build integration

// rebaseline_test.go exercises Rebaseline (Tier 2), reusing the begin fixture for the foreign-edit
// scenario and building bare RebaselineDeps for the card-set refusals.
package websterengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// doneBatchOne is a terminal done record for the fixture's first batch.
func doneBatchOne() *websterengine.BatchState {
	return &websterengine.BatchState{
		Slug:     "json-flag",
		Cards:    []string{"01-json-flag"},
		StartSHA: "startsha1",
		Terminal: true,
		Status:   "done",
		Digest:   &websterengine.Digest{Batch: "01-json-flag", Status: websterengine.DigestStatusDone, HeadSHA: "deadbeef"},
	}
}

func TestRebaseline_ForeignEditAcceptedMidRun(t *testing.T) {
	fx := newBeginFixture(t)
	fx.Deps.State.Batches = map[int]*websterengine.BatchState{1: doneBatchOne()}
	before := *fx.Deps.State.Batches[1]

	if err := os.WriteFile(filepath.Join(fx.PlanDir, "00-overview.md"), []byte("# plan, edited\n"), 0o644); err != nil {
		t.Fatalf("edit plan: %v", err)
	}

	_, err := websterengine.BeginBatch(fx.Deps, 2)
	if !errors.Is(err, websterengine.ErrFingerprintMismatch) {
		t.Fatalf("BeginBatch(2) error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
	}
	requireWayForward(t, err, "lyx webster rebaseline", "lyx webster run --fresh")

	res, err := websterengine.Rebaseline(websterengine.RebaselineDeps{Plan: fx.Deps.Plan, Batches: fx.Deps.Batches, State: fx.Deps.State})
	if err != nil {
		t.Fatalf("Rebaseline() error = %v; want nil", err)
	}
	if res.BatchesKept != 1 || res.PreviousFingerprint == res.Fingerprint {
		t.Errorf("Rebaseline() = %+v; want 1 kept batch and a changed fingerprint", res)
	}

	if _, err := websterengine.BeginBatch(fx.Deps, 2); err != nil {
		t.Fatalf("BeginBatch(2) after Rebaseline error = %v; want nil", err)
	}
	got := fx.Deps.State.Batches[1]
	if got.Status != before.Status || got.StartSHA != before.StartSHA || got.Digest != before.Digest {
		t.Errorf("batch 1 record changed: got %+v, want %+v", *got, before)
	}
}

func rebaselineDeps(t *testing.T, batches []batcher.Batch, recs map[int]*websterengine.BatchState) websterengine.RebaselineDeps {
	t.Helper()
	planDir := seedPlanDir(t)
	return websterengine.RebaselineDeps{
		Plan:    &planparser.Plan{Dir: planDir, Format: 5},
		Batches: batches,
		State:   &websterengine.State{PlanFingerprint: "old-fingerprint", Batches: recs},
	}
}

func TestRebaseline_RefusesChangedCardSet(t *testing.T) {
	two := batcher.Batch{Cards: []planparser.Card{
		{Number: 1, Slug: "json-flag"},
		{Number: 2, Slug: "list-tests"},
	}}
	cases := []struct {
		name    string
		batches []batcher.Batch
	}{
		{"removed card", []batcher.Batch{beginCard(2, "list-tests")}},
		{"renumbered card", []batcher.Batch{beginCard(1, "other-slug"), beginCard(2, "list-tests")}},
		{"regrouped batch", []batcher.Batch{two}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := rebaselineDeps(t, tc.batches, map[int]*websterengine.BatchState{1: doneBatchOne()})
			_, err := websterengine.Rebaseline(deps)
			if !errors.Is(err, websterengine.ErrRebaselineCardSetChanged) {
				t.Fatalf("Rebaseline() error = %v; want errors.Is(err, ErrRebaselineCardSetChanged)", err)
			}
			for _, want := range []string{"batch 1", "01-json-flag", "--fresh", "startsha1"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err.Error(), want)
				}
			}
			if deps.State.PlanFingerprint != "old-fingerprint" {
				t.Errorf("PlanFingerprint = %q; want it unchanged on refusal", deps.State.PlanFingerprint)
			}
		})
	}
}

func TestRebaseline_LegacyRecordWithoutCardsAccepted(t *testing.T) {
	legacy := doneBatchOne()
	legacy.Cards = nil
	deps := rebaselineDeps(t, []batcher.Batch{beginCard(1, "json-flag")}, map[int]*websterengine.BatchState{1: legacy})
	if _, err := websterengine.Rebaseline(deps); err != nil {
		t.Fatalf("Rebaseline() error = %v; want nil", err)
	}
	if deps.State.PlanFingerprint == "old-fingerprint" {
		t.Error("PlanFingerprint not restamped")
	}
}
