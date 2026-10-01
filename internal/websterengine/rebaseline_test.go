//go:build integration

// rebaseline_test.go exercises Rebaseline (Tier 2), reusing the begin fixture for the foreign-edit scenario and building bare RebaselineDeps for the card-set refusals.
package websterengine_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

	res, err := websterengine.Rebaseline(websterengine.RebaselineDeps{Plan: fx.Deps.Plan, Batches: fx.Deps.Batches, State: fx.Deps.State, Geom: fx.Deps.Geom})
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
	worktree := newScratchRepo(t)
	base := commitFile(t, worktree, "base.txt", "base", "base commit")
	for _, rec := range recs {
		if rec.StartSHA == "startsha1" {
			rec.StartSHA = base
		}
	}
	return websterengine.RebaselineDeps{
		Plan:    &planparser.Plan{Dir: planDir, Format: 5},
		Batches: batches,
		State:   &websterengine.State{PlanFingerprint: "old-fingerprint", Batches: recs},
		Geom:    websterengine.Geometry{WorktreeRoot: worktree, WebsterDir: t.TempDir()},
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
			for _, want := range []string{"batch 1", "01-json-flag", "--fresh", deps.State.Batches[1].StartSHA} {
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

// TestRebaseline_RefusalNamesStartCommitByAncestry pins that the start commit a refusal names is the one every other start descends from, not the lowest-numbered batch's:
// batch 2 ran first, so its start C0 is the run's start and batch 1's start C1 is not.
func TestRebaseline_RefusalNamesStartCommitByAncestry(t *testing.T) {
	deps := rebaselineDeps(t, []batcher.Batch{beginCard(3, "other")}, map[int]*websterengine.BatchState{})
	root := deps.Geom.WorktreeRoot
	c0 := strings.TrimSpace(mustGit(t, root, "rev-parse", "HEAD"))
	c1 := commitFile(t, root, "next.txt", "next", "next commit")
	first := doneBatchOne()
	first.StartSHA = c1
	second := doneBatchOne()
	second.Slug, second.Cards, second.StartSHA = "list-tests", []string{"02-list-tests"}, c0
	deps.State.Batches = map[int]*websterengine.BatchState{1: first, 2: second}

	_, err := websterengine.Rebaseline(deps)
	if !errors.Is(err, websterengine.ErrRebaselineCardSetChanged) {
		t.Fatalf("Rebaseline() error = %v; want errors.Is(err, ErrRebaselineCardSetChanged)", err)
	}
	if !strings.Contains(err.Error(), "start commit "+c0) {
		t.Errorf("error %q; want the start commit %s named", err.Error(), c0)
	}
	if strings.Contains(err.Error(), "start commit "+c1) {
		t.Errorf("error %q names the later commit %s as the start commit", err.Error(), c1)
	}
}

// TestRebaseline_RefusalWithoutStartSHANamesNoBlankCommit pins the refusal text when no record carries a StartSHA:
// it still names the run's start commit, with no empty SHA slot.
func TestRebaseline_RefusalWithoutStartSHANamesNoBlankCommit(t *testing.T) {
	rec := doneBatchOne()
	rec.StartSHA = ""
	deps := rebaselineDeps(t, []batcher.Batch{beginCard(2, "list-tests")}, map[int]*websterengine.BatchState{1: rec})
	_, err := websterengine.Rebaseline(deps)
	if !errors.Is(err, websterengine.ErrRebaselineCardSetChanged) {
		t.Fatalf("Rebaseline() error = %v; want errors.Is(err, ErrRebaselineCardSetChanged)", err)
	}
	if !strings.Contains(err.Error(), "reset the branch to the run's start commit with git") {
		t.Errorf("error %q; want the start commit named without a blank SHA", err.Error())
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

// beginAndFinishBatchOne begins batch 1 through the fixture, so its record carries CardHashes, and marks it done.
func beginAndFinishBatchOne(t *testing.T, fx *beginFixture) {
	t.Helper()
	if _, err := websterengine.BeginBatch(fx.Deps, 1); err != nil {
		t.Fatalf("BeginBatch(1) error = %v; want nil", err)
	}
	rec := fx.Deps.State.Batches[1]
	rec.Terminal = true
	rec.Status = "done"
}

func rebaselineFixtureDeps(fx *beginFixture) websterengine.RebaselineDeps {
	return websterengine.RebaselineDeps{Plan: fx.Deps.Plan, Batches: fx.Deps.Batches, State: fx.Deps.State, Geom: fx.Deps.Geom}
}

func TestRebaseline_RefusesChangedBegunCardBody(t *testing.T) {
	fx := newBeginFixture(t)
	beginAndFinishBatchOne(t, fx)
	if len(fx.Deps.State.Batches[1].CardHashes) != 1 {
		t.Fatalf("CardHashes = %v; want one hash recorded at begin", fx.Deps.State.Batches[1].CardHashes)
	}
	fingerprint := fx.Deps.State.PlanFingerprint

	card := filepath.Join(fx.PlanDir, "01-json-flag.md")
	if err := os.WriteFile(card, []byte("# Card 1 — json-flag\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** placeholder card.\n\n**Verify:** true\n"), 0o644); err != nil {
		t.Fatalf("edit card: %v", err)
	}

	deps := rebaselineFixtureDeps(fx)
	deps.Cards = []int{1}
	_, err := websterengine.Rebaseline(deps)
	if !errors.Is(err, websterengine.ErrRebaselineCardSetChanged) {
		t.Fatalf("Rebaseline() error = %v; want errors.Is(err, ErrRebaselineCardSetChanged)", err)
	}
	if !strings.Contains(err.Error(), "batch 1 card 01-json-flag changed since it was begun") {
		t.Errorf("error %q lacks the changed-card naming", err.Error())
	}
	if fx.Deps.State.PlanFingerprint != fingerprint {
		t.Errorf("PlanFingerprint = %q; want it unchanged on refusal", fx.Deps.State.PlanFingerprint)
	}
}

func TestRebaseline_AcceptsEditedUnbegunCard(t *testing.T) {
	fx := newBeginFixture(t)
	beginAndFinishBatchOne(t, fx)
	before := *fx.Deps.State.Batches[1]

	if err := os.WriteFile(filepath.Join(fx.PlanDir, "02-list-tests.md"), []byte("# Card 2 — list-tests\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** a reworded intent.\n"), 0o644); err != nil {
		t.Fatalf("edit card: %v", err)
	}

	deps := rebaselineFixtureDeps(fx)
	deps.Cards = []int{2}
	res, err := websterengine.Rebaseline(deps)
	if err != nil {
		t.Fatalf("Rebaseline() error = %v; want nil", err)
	}
	if res.BatchesKept != 1 {
		t.Errorf("BatchesKept = %d; want 1", res.BatchesKept)
	}
	if !slices.Equal(res.CardsAccepted, []string{"02-list-tests.md"}) {
		t.Errorf("CardsAccepted = %v; want [02-list-tests.md]", res.CardsAccepted)
	}
	if fx.Deps.State.PlanFileHashes["02-list-tests.md"] == "" {
		t.Errorf("PlanFileHashes = %v; want card 2's new hash recorded", fx.Deps.State.PlanFileHashes)
	}
	got := fx.Deps.State.Batches[1]
	if got.Status != before.Status || got.StartSHA != before.StartSHA || !maps.Equal(got.CardHashes, before.CardHashes) {
		t.Errorf("batch 1 record changed: got %+v, want %+v", *got, before)
	}
}

func TestRebaseline_RecordWithoutCardHashesComparesIDsOnly(t *testing.T) {
	fx := newBeginFixture(t)
	fx.Deps.State.Batches = map[int]*websterengine.BatchState{1: doneBatchOne()}

	if err := os.WriteFile(filepath.Join(fx.PlanDir, "01-json-flag.md"), []byte("# Card 1 — json-flag\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** edited after begin.\n"), 0o644); err != nil {
		t.Fatalf("edit card: %v", err)
	}
	deps := rebaselineFixtureDeps(fx)
	deps.Cards = []int{1}
	if _, err := websterengine.Rebaseline(deps); err != nil {
		t.Fatalf("Rebaseline() error = %v; want nil for a record without CardHashes", err)
	}
}

// editCard2 rewrites unbegun card 2 of the begin fixture with a reworded intent.
func editCard2(t *testing.T, fx *beginFixture, intent string) {
	t.Helper()
	body := "# Card 2 — list-tests\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** " + intent + "\n"
	if err := os.WriteFile(filepath.Join(fx.PlanDir, "02-list-tests.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("edit card 2: %v", err)
	}
}

func TestRebaseline_RefusesUnnamedEditedCard(t *testing.T) {
	fx := newBeginFixture(t)
	beginAndFinishBatchOne(t, fx)
	fingerprint := fx.Deps.State.PlanFingerprint
	editCard2(t, fx, "a reworded intent.")

	_, err := websterengine.Rebaseline(rebaselineFixtureDeps(fx))
	if !errors.Is(err, websterengine.ErrRebaselineCardSetChanged) {
		t.Fatalf("Rebaseline() error = %v; want errors.Is(err, ErrRebaselineCardSetChanged)", err)
	}
	if !strings.Contains(err.Error(), "02-") || !strings.Contains(err.Error(), "--card") {
		t.Errorf("error %q; want it to name 02- and --card", err.Error())
	}
	if fx.Deps.State.PlanFingerprint != fingerprint {
		t.Errorf("PlanFingerprint = %q; want it unchanged on refusal", fx.Deps.State.PlanFingerprint)
	}
}

func TestRebaseline_RefusesEditedOverviewEvenWhenEveryCardIsNamed(t *testing.T) {
	fx := newBeginFixture(t)
	beginAndFinishBatchOne(t, fx)
	if err := os.WriteFile(filepath.Join(fx.PlanDir, "00-overview.md"), []byte("# plan, edited\n"), 0o644); err != nil {
		t.Fatalf("edit overview: %v", err)
	}

	deps := rebaselineFixtureDeps(fx)
	deps.Cards = []int{1, 2}
	_, err := websterengine.Rebaseline(deps)
	if !errors.Is(err, websterengine.ErrRebaselineCardSetChanged) {
		t.Fatalf("Rebaseline() error = %v; want errors.Is(err, ErrRebaselineCardSetChanged)", err)
	}
	if !strings.Contains(err.Error(), "00-overview.md") || !strings.Contains(err.Error(), "--fresh") {
		t.Errorf("error %q; want it to name 00-overview.md and --fresh", err.Error())
	}
}

func TestRebaseline_RefusesEditedCardTheOperatorDidNotName(t *testing.T) {
	fx := newBeginFixture(t)
	beginAndFinishBatchOne(t, fx)
	editCard2(t, fx, "a reworded intent.")
	if err := os.WriteFile(filepath.Join(fx.PlanDir, "03-third.md"), []byte("# Card 3 — third\n\n**Intent:** new.\n"), 0o644); err != nil {
		t.Fatalf("add card 3: %v", err)
	}

	deps := rebaselineFixtureDeps(fx)
	deps.Cards = []int{2}
	_, err := websterengine.Rebaseline(deps)
	if !errors.Is(err, websterengine.ErrRebaselineCardSetChanged) {
		t.Fatalf("Rebaseline() error = %v; want errors.Is(err, ErrRebaselineCardSetChanged)", err)
	}
	if !strings.Contains(err.Error(), "03-third.md") || strings.Contains(err.Error(), "02-list-tests.md") {
		t.Errorf("error %q; want it to name only 03-third.md", err.Error())
	}
}

func TestRebaseline_StateWithoutPlanFileHashesChecksBegunCardsOnly(t *testing.T) {
	fx := newBeginFixture(t)
	beginAndFinishBatchOne(t, fx)
	fx.Deps.State.PlanFileHashes = nil
	editCard2(t, fx, "a reworded intent.")

	if _, err := websterengine.Rebaseline(rebaselineFixtureDeps(fx)); err != nil {
		t.Fatalf("Rebaseline() error = %v; want nil for a state without PlanFileHashes", err)
	}
}
