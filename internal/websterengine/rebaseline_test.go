// rebaseline_test.go exercises Rebaseline (Tier 1), reusing the begin fixture for the foreign-edit scenario and building bare RebaselineDeps for the card-set refusals.
package websterengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
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

// TestRebaseline_ForeignEditAcceptedMidRun walks the mid-run loop: begin-batch refuses a foreign plan
// edit naming rebaseline, Rebaseline accepts it, and the next begin-batch succeeds with the done
// record intact.
//
//testtiming:keep pins the whole refuse, rebaseline, re-begin loop across three calls on one state; the covering tests each make one of those calls
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

	res, err := websterengine.Rebaseline(websterengine.RebaselineDeps{Plan: fx.Deps.Plan, Active: batcher.Identity(), State: fx.Deps.State, Geom: fx.Deps.Geom})
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

// fixedBatcher is a Batcher stand-in that returns the batches it holds whatever the plan and size source,
// so a test pins the grouping Rebaseline compares the run's begun batches against.
type fixedBatcher struct{ batches []batcher.Batch }

func (f fixedBatcher) Batch(*planparser.Plan, []planparser.Card, batcher.SizeSource, int, batcher.StartBase) ([]batcher.Batch, error) {
	return f.batches, nil
}

func (fixedBatcher) Name() string { return "fixed" }

// rebaselineDeps builds bare RebaselineDeps over a plan holding planCards, a state recording partition (nil for a state from before partitions) and recs, and active as the tail's batchifier.
func rebaselineDeps(t *testing.T, planCards []planparser.Card, partition []websterengine.PartitionBatch, active batcher.Batcher, recs map[int]*websterengine.BatchState) websterengine.RebaselineDeps {
	t.Helper()
	planDir := seedPlanDir(t)
	worktree := t.TempDir()
	git := newFakeGit()
	base := git.head
	for _, rec := range recs {
		if rec.StartSHA == "startsha1" {
			rec.StartSHA = base
		}
	}
	return websterengine.RebaselineDeps{
		Plan:   &planparser.Plan{Dir: planDir, Format: 5, Cards: planCards},
		Active: active,
		State:  &websterengine.State{PlanFingerprint: "old-fingerprint", Batches: recs, Partition: slices.Clone(partition)},
		Geom:   websterengine.Geometry{WorktreeRoot: worktree, WebsterDir: t.TempDir(), Git: git},
	}
}

// TestRebaseline_CardSet proves Rebaseline compares the cards of every batch up to the last begun one with the edited plan's:
// a removed, renumbered, reordered or inserted card at or before the last begun card refuses, naming the batch, the card and the fresh-restart steps, whether or not the record carries a StartSHA;
// cards after it are re-batched by the active batchifier, with the begun batch keeping its recorded profile and estimate and the new partition replacing the old only on accept;
// a tail whose order breaks a dependency refuses with ErrBatchOrder;
// a state without a partition regroups by identity and records none;
// and a legacy record with no card set is accepted and restamped.
func TestRebaseline_CardSet(t *testing.T) {
	t.Parallel()

	card := func(number int, slug string) planparser.Card { return planparser.Card{Number: number, Slug: slug} }
	recorded := []websterengine.PartitionBatch{
		{Cards: []string{"01-json-flag"}, Profile: "cautious", Estimate: 3.5},
		{Cards: []string{"02-list-tests"}, Profile: "cautious", Estimate: 1.5},
	}
	grouped := fixedBatcher{[]batcher.Batch{{
		Cards:    []planparser.Card{card(2, "list-tests"), card(3, "added")},
		Profile:  "fixed",
		Estimate: 7,
	}}}
	unordered := fixedBatcher{[]batcher.Batch{
		{Cards: []planparser.Card{{Number: 2, Slug: "list-tests", Uses: []string{"x.go"}}}},
		{Cards: []planparser.Card{{Number: 3, Slug: "added", Targets: []string{"x.go"}}}},
	}}
	positioned := batcher.NewCost("positioned", batcher.CostParams{Budget: 1e9, MaxCards: 2, Weights: batcher.Weights{Orientation: 100, BatchGrowth: 10}})
	cases := []struct {
		name      string
		cards     []planparser.Card
		partition []websterengine.PartitionBatch
		active    batcher.Batcher
		// edit adjusts the batch-1 record before the call.
		edit func(rec *websterengine.BatchState)
		// wantErr is the sentinel a refusal wraps;
		// nil expects an accept.
		wantErr error
		// wantPartition is State.Partition after the call;
		// a refusal expects the partition it began with.
		wantPartition []websterengine.PartitionBatch
		// wantTailPosition, when set, is the position the regrouped tail's first batch records in its Breakdown, and wantPartition is not compared.
		wantTailPosition int
	}{
		{
			name:             "a tail regrouped by a cost profile is priced after the kept batches",
			cards:            []planparser.Card{card(1, "json-flag"), card(2, "list-tests"), card(3, "added")},
			partition:        recorded,
			active:           positioned,
			wantTailPosition: 2,
		},
		{
			name:          "a plan that still holds the kept cards keeps the recorded grouping",
			cards:         []planparser.Card{card(1, "json-flag"), card(2, "list-tests")},
			partition:     recorded,
			active:        fixedBatcher{[]batcher.Batch{{Cards: []planparser.Card{card(2, "list-tests")}, Profile: "fixed", Estimate: 2}}},
			wantPartition: []websterengine.PartitionBatch{recorded[0], {Cards: []string{"02-list-tests"}, Profile: "fixed", Estimate: 2}},
		},
		{
			name:          "a card added after the last begun card is grouped into the tail",
			cards:         []planparser.Card{card(1, "json-flag"), card(2, "list-tests"), card(3, "added")},
			partition:     recorded,
			active:        grouped,
			wantPartition: []websterengine.PartitionBatch{recorded[0], {Cards: []string{"02-list-tests", "03-added"}, Profile: "fixed", Estimate: 7}},
		},
		{
			name:          "a removed card at the last begun card refuses",
			cards:         []planparser.Card{card(2, "list-tests")},
			partition:     recorded,
			active:        grouped,
			wantErr:       websterengine.ErrRebaselineCardSetChanged,
			wantPartition: recorded,
		},
		{
			name:          "a card inserted before the last begun card refuses",
			cards:         []planparser.Card{card(3, "added"), card(1, "json-flag"), card(2, "list-tests")},
			partition:     recorded,
			active:        grouped,
			wantErr:       websterengine.ErrRebaselineCardSetChanged,
			wantPartition: recorded,
		},
		{
			name:          "a reordered card at the last begun card refuses",
			cards:         []planparser.Card{card(2, "list-tests"), card(1, "json-flag")},
			partition:     recorded,
			active:        grouped,
			wantErr:       websterengine.ErrRebaselineCardSetChanged,
			wantPartition: recorded,
		},
		{
			name:          "a renumbered card refuses",
			cards:         []planparser.Card{card(1, "other-slug"), card(2, "list-tests")},
			partition:     recorded,
			active:        grouped,
			wantErr:       websterengine.ErrRebaselineCardSetChanged,
			wantPartition: recorded,
		},
		{
			name:          "a removed card from a record without a StartSHA refuses",
			cards:         []planparser.Card{card(2, "list-tests")},
			partition:     recorded,
			active:        grouped,
			edit:          func(rec *websterengine.BatchState) { rec.StartSHA = "" },
			wantErr:       websterengine.ErrRebaselineCardSetChanged,
			wantPartition: recorded,
		},
		{
			name:          "a tail that uses a later batch's target refuses with ErrBatchOrder",
			cards:         []planparser.Card{card(1, "json-flag"), card(2, "list-tests"), card(3, "added")},
			partition:     recorded,
			active:        unordered,
			wantErr:       websterengine.ErrBatchOrder,
			wantPartition: recorded,
		},
		{
			name:   "a state without a partition regroups by identity and records none",
			cards:  []planparser.Card{card(1, "json-flag"), card(2, "list-tests")},
			active: grouped,
		},
		{
			name:    "a state without a partition refuses a removed card",
			cards:   []planparser.Card{card(2, "list-tests")},
			active:  grouped,
			wantErr: websterengine.ErrRebaselineCardSetChanged,
		},
		{
			name:   "a legacy record without cards is accepted",
			cards:  []planparser.Card{card(1, "json-flag")},
			active: grouped,
			edit:   func(rec *websterengine.BatchState) { rec.Cards = nil },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doneBatchOne()
			if tc.edit != nil {
				tc.edit(rec)
			}
			deps := rebaselineDeps(t, tc.cards, tc.partition, tc.active, map[int]*websterengine.BatchState{1: rec})
			_, err := websterengine.Rebaseline(deps)
			if tc.wantTailPosition > 0 {
				if len(deps.State.Partition) < 2 || deps.State.Partition[1].Breakdown == nil || deps.State.Partition[1].Breakdown.Position != tc.wantTailPosition {
					t.Errorf("Partition = %+v; want the tail's first batch recorded at position %d", deps.State.Partition, tc.wantTailPosition)
				}
			} else if !reflect.DeepEqual(deps.State.Partition, tc.wantPartition) && len(deps.State.Partition)+len(tc.wantPartition) > 0 {
				t.Errorf("Partition = %+v; want %+v", deps.State.Partition, tc.wantPartition)
			}
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Rebaseline() error = %v; want nil", err)
				}
				if deps.State.PlanFingerprint == "old-fingerprint" {
					t.Error("PlanFingerprint not restamped")
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Rebaseline() error = %v; want errors.Is(err, %v)", err, tc.wantErr)
			}
			if tc.wantErr == websterengine.ErrRebaselineCardSetChanged {
				for _, want := range []string{"batch 1", "01-json-flag", "or 1) lyx webster reset --to start; 2) lyx webster run --fresh"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q lacks %q", err.Error(), want)
					}
				}
			}
			if deps.State.PlanFingerprint != "old-fingerprint" {
				t.Errorf("PlanFingerprint = %q; want it unchanged on refusal", deps.State.PlanFingerprint)
			}
		})
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
	return websterengine.RebaselineDeps{Plan: fx.Deps.Plan, Active: batcher.Identity(), State: fx.Deps.State, Geom: fx.Deps.Geom}
}

// editCard2 rewrites unbegun card 2 of the begin fixture with a reworded intent.
func editCard2(t *testing.T, fx *beginFixture, intent string) {
	t.Helper()
	body := "# Card 2 — list-tests\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** " + intent + "\n"
	if err := os.WriteFile(filepath.Join(fx.PlanDir, "02-list-tests.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("edit card 2: %v", err)
	}
}

// writePlanFile writes body to name in the fixture's plan directory.
func writePlanFile(t *testing.T, fx *beginFixture, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(fx.PlanDir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestRebaseline_EditedPlan proves which plan edits Rebaseline accepts after batch 1 was begun and
// finished: an edited unbegun card it is told about is accepted, recording its hash and leaving the
// begun record alone, as is a begun card's edit under a record without card hashes and any edit under
// a state without plan-file hashes; a begun card's changed body, an edited card it was not told about
// and an edited overview refuse, naming what changed and leaving the fingerprint.
func TestRebaseline_EditedPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// prepare records batch 1 and edits the plan.
		prepare func(t *testing.T, fx *beginFixture)
		cards   []int
		// wantText lists what a refusal names, wantNotText what it must not; both empty expect an accept.
		wantText    []string
		wantNotText []string
		check       func(t *testing.T, fx *beginFixture, before websterengine.BatchState, res *websterengine.RebaselineResult)
	}{
		{
			name: "an edited unbegun card that is named is accepted",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				editCard2(t, fx, "a reworded intent.")
			},
			cards: []int{2},
			check: func(t *testing.T, fx *beginFixture, before websterengine.BatchState, res *websterengine.RebaselineResult) {
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
			},
		},
		{
			name: "a begun card edited under a record without card hashes compares ids only",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{1: doneBatchOne()}
				writePlanFile(t, fx, "01-json-flag.md", "# Card 1 — json-flag\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** edited after begin.\n")
			},
			cards: []int{1},
		},
		{
			name: "a state without plan-file hashes checks begun cards only",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				fx.Deps.State.PlanFileHashes = nil
				editCard2(t, fx, "a reworded intent.")
			},
		},
		{
			name: "a begun card's changed body refuses",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				if len(fx.Deps.State.Batches[1].CardHashes) != 1 {
					t.Fatalf("CardHashes = %v; want one hash recorded at begin", fx.Deps.State.Batches[1].CardHashes)
				}
				writePlanFile(t, fx, "01-json-flag.md", "# Card 1 — json-flag\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** placeholder card.\n\n**Verify:** true\n")
			},
			cards:    []int{1},
			wantText: []string{"batch 1 card 01-json-flag changed since it was begun", "batch is done", "--fresh"},
		},
		{
			name: "a named card of a failed batch is accepted and restamped",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				setBatchOneState(fx, true, "failed", nil)
				editBegunCardOne(t, fx)
			},
			cards: []int{1},
			check: checkBegunCardRestamped,
		},
		{
			name: "a named card of a dead batch is accepted and restamped",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				setBatchOneState(fx, true, websterengine.DigestStatusDead, nil)
				editBegunCardOne(t, fx)
			},
			cards: []int{1},
			check: checkBegunCardRestamped,
		},
		{
			name: "a named card of a stuck batch is accepted and restamped",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				setBatchOneState(fx, true, websterengine.DigestStatusStuck, nil)
				editBegunCardOne(t, fx)
			},
			cards: []int{1},
			check: checkBegunCardRestamped,
		},
		{
			name: "a named card of an unfinished batch refuses naming record-batch and recover-batch",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				setBatchOneState(fx, false, "", nil)
				editBegunCardOne(t, fx)
			},
			cards:    []int{1},
			wantText: []string{"batch 1 card 01-json-flag changed since it was begun", "unfinished", "lyx webster record-batch 1", "lyx webster recover-batch 1"},
		},
		{
			name: "a named card of a failed batch with uncheckable findings refuses toward a fresh restart",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				setBatchOneState(fx, true, "failed", []string{"no-path: unattributed write"})
				editBegunCardOne(t, fx)
			},
			cards:    []int{1},
			wantText: []string{"batch 1 card 01-json-flag changed since it was begun", "cannot check", "--fresh"},
		},
		{
			name: "an unnamed edited card of a failed batch still refuses as unnamed",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				setBatchOneState(fx, true, "failed", nil)
				editBegunCardOne(t, fx)
			},
			wantText: []string{"01-json-flag.md", "changed but not named", "--card"},
		},
		{
			name: "an edited card that is not named refuses naming --card",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				editCard2(t, fx, "a reworded intent.")
			},
			wantText: []string{"02-", "--card"},
		},
		{
			name: "an added card that is not named refuses naming only that card",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				editCard2(t, fx, "a reworded intent.")
				writePlanFile(t, fx, "03-third.md", "# Card 3 — third\n\n**Intent:** new.\n")
			},
			cards:       []int{2},
			wantText:    []string{"03-third.md"},
			wantNotText: []string{"02-list-tests.md"},
		},
		{
			name: "an edited overview refuses even when every card is named",
			prepare: func(t *testing.T, fx *beginFixture) {
				beginAndFinishBatchOne(t, fx)
				writePlanFile(t, fx, "00-overview.md", "# plan, edited\n")
			},
			cards:    []int{1, 2},
			wantText: []string{"00-overview.md", "--fresh"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newBeginFixture(t)
			tt.prepare(t, fx)
			before := *fx.Deps.State.Batches[1]
			before.CardHashes = maps.Clone(before.CardHashes)
			fingerprint := fx.Deps.State.PlanFingerprint

			deps := rebaselineFixtureDeps(fx)
			deps.Cards = tt.cards
			res, err := websterengine.Rebaseline(deps)
			if len(tt.wantText) == 0 {
				if err != nil {
					t.Fatalf("Rebaseline() error = %v; want nil", err)
				}
				if tt.check != nil {
					tt.check(t, fx, before, res)
				}
				return
			}
			if !errors.Is(err, websterengine.ErrRebaselineCardSetChanged) {
				t.Fatalf("Rebaseline() error = %v; want errors.Is(err, ErrRebaselineCardSetChanged)", err)
			}
			for _, want := range tt.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err.Error(), want)
				}
			}
			for _, notWant := range tt.wantNotText {
				if strings.Contains(err.Error(), notWant) {
					t.Errorf("error %q names %q; want it left out", err.Error(), notWant)
				}
			}
			if fx.Deps.State.PlanFingerprint != fingerprint {
				t.Errorf("PlanFingerprint = %q; want it unchanged on refusal", fx.Deps.State.PlanFingerprint)
			}
			if got := fx.Deps.State.Batches[1].CardHashes; !maps.Equal(got, before.CardHashes) {
				t.Errorf("CardHashes = %v; want %v unchanged on refusal", got, before.CardHashes)
			}
		})
	}
}

// setBatchOneState rewrites batch 1's record after begin to the given terminal state, with one audit warning to prove a restamp keeps it.
func setBatchOneState(fx *beginFixture, terminal bool, status string, uncheckable []string) {
	rec := fx.Deps.State.Batches[1]
	rec.Terminal = terminal
	rec.Status = status
	rec.Uncheckable = uncheckable
	rec.AuditWarnings = []websterengine.AuditWarning{{}}
}

// editBegunCardOne rewrites begun card 1 of the begin fixture with a reworded intent.
func editBegunCardOne(t *testing.T, fx *beginFixture) {
	t.Helper()
	writePlanFile(t, fx, "01-json-flag.md", "# Card 1 — json-flag\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** reworded after the batch stopped.\n")
}

// checkBegunCardRestamped asserts the accepted edit moved card 1's recorded hash to the file's new hash and kept the rest of batch 1's record.
func checkBegunCardRestamped(t *testing.T, fx *beginFixture, before websterengine.BatchState, res *websterengine.RebaselineResult) {
	t.Helper()
	got := fx.Deps.State.Batches[1]
	want := fileSHA(t, filepath.Join(fx.PlanDir, "01-json-flag.md"))
	if got.CardHashes["01-json-flag"] != want || want == before.CardHashes["01-json-flag"] {
		t.Errorf("CardHashes = %v; want 01-json-flag restamped to %s, was %v", got.CardHashes, want, before.CardHashes)
	}
	if got.StartSHA != before.StartSHA || got.Status != before.Status || got.Terminal != before.Terminal || len(got.AuditWarnings) != len(before.AuditWarnings) {
		t.Errorf("batch 1 record = %+v; want StartSHA, status and warnings of %+v kept", *got, before)
	}
	if !slices.Equal(res.CardsAccepted, []string{"01-json-flag.md"}) {
		t.Errorf("CardsAccepted = %v; want [01-json-flag.md]", res.CardsAccepted)
	}
}

// fileSHA is the hex SHA-256 of the file at path, the hash State.PlanFileHashes and BatchState.CardHashes record.
func fileSHA(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// handlePlan is a three-card plan whose card 2 declares a handle; with cardOneUsesHandle, card 1 Uses it in the given spelling,
// so canonicalizing card 2's draft handle rewrites card 1 as well.
// Card 1 comes first in a plan that passes the format checks, so the Uses edge is added only after batch 1 was begun, as a mid-run plan edit.
func handlePlan(draft, cardOneUsesHandle bool) plankit.Plan {
	spelling := "Baz"
	if draft {
		spelling = "Bazz"
	}
	var cardOneUses []string
	if cardOneUsesHandle {
		cardOneUses = []string{"plan:internal/foo#" + spelling}
	}
	base := []plankit.Group{{Label: "Prosa", Targets: []string{"base.txt"}}}
	return plankit.Plan{
		Approved: true,
		Language: "go",
		Cards: []plankit.Card{
			{Number: 1, Slug: "json-flag", Summary: "uses a handle card 2 declares", Groups: base, Uses: cardOneUses, Intent: "placeholder card."},
			{
				Number:  2,
				Slug:    "list-tests",
				Summary: "declares the handle",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"plan:internal/foo#" + spelling + "` -> `func Baz()"}}},
				Intent:  "declare the handle.",
			},
			{Number: 3, Slug: "third", Summary: "an unbegun card that keeps the handle referenced", Groups: base, Uses: []string{"plan:internal/foo#" + spelling}, Intent: "placeholder card."},
		},
	}
}

// beginThenLeaveHandleDraft returns a fixture whose batch 1 is begun and done, and whose plan on disk carries card 2's handle in draft spelling
// with state.json describing exactly those bytes, so the next BeginBatch's canonicalization rewrites begun card 1 (#330).
func beginThenLeaveHandleDraft(t *testing.T) *beginFixture {
	t.Helper()
	fx := newBeginFixture(t)
	writePlan := func(p plankit.Plan) { plankit.Write(t, fx.PlanDir, p) }
	writePlan(handlePlan(false, false))
	plan, err := planparser.ParsePlan(fx.PlanDir)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	fx.Deps.Plan = plan
	fx.Deps.Batches = append(fx.Deps.Batches, batcher.Batch{Cards: []planparser.Card{{Number: 3, Slug: "third", Title: "third", Intent: "placeholder card third"}}})
	fx.Deps.State.PlanFingerprint = mustFingerprint(t, fx.PlanDir)
	beginAndFinishBatchOne(t, fx)

	writePlan(handlePlan(true, true))
	if fx.Deps.Plan, err = planparser.ParsePlan(fx.PlanDir); err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	st := fx.Deps.State
	st.PlanFingerprint = mustFingerprint(t, fx.PlanDir)
	for name := range st.PlanFileHashes {
		st.PlanFileHashes[name] = fileSHA(t, filepath.Join(fx.PlanDir, name))
	}
	st.Batches[1].CardHashes["01-json-flag"] = fileSHA(t, filepath.Join(fx.PlanDir, "01-json-flag.md"))
	return fx
}

// TestBeginBatch_RewrittenBegunCardMovesRecordedHash_Regression330 pins the begin-batch half of #330:
// when begin-batch's handle canonicalization rewrites a begun card, the card's recorded hash moves to the rewritten bytes, so the card never reads as a foreign edit.
// The rewrite happens only when a begun card Uses a handle a later card declares, a backward dependency that Rebaseline refuses on the batch order, so the accept after a moved hash is pinned by the record-batch variant in rebaseline_git_test.go.
func TestBeginBatch_RewrittenBegunCardMovesRecordedHash_Regression330(t *testing.T) {
	fx := beginThenLeaveHandleDraft(t)
	st := fx.Deps.State
	card1 := filepath.Join(fx.PlanDir, "01-json-flag.md")
	draftHash := st.Batches[1].CardHashes["01-json-flag"]

	if _, err := websterengine.BeginBatch(fx.Deps, 2); err != nil {
		t.Fatalf("BeginBatch(2) error = %v; want nil", err)
	}
	rewritten, err := os.ReadFile(card1)
	if err != nil {
		t.Fatalf("read card 1: %v", err)
	}
	if !strings.Contains(string(rewritten), "plan:internal/foo#Baz`") {
		t.Fatalf("card 1 = %q; want the handle canonicalized, or the fixture exercises no rewrite", rewritten)
	}
	if got := st.Batches[1].CardHashes["01-json-flag"]; got == draftHash || got != fileSHA(t, card1) {
		t.Fatalf("batch 1 CardHashes = %q; want it moved to the rewritten card's hash %q", got, fileSHA(t, card1))
	}
}

// TestRebaseline_ForeignEditToBegunCardStaysRefused proves a foreign edit to a begun card is refused
// by begin-batch without moving the card's recorded hash, so Rebaseline naming that card refuses too.
//
//testtiming:keep pins a refused begin-batch leaving the begun card's recorded hash untouched, so a later rebaseline naming that card still refuses; the covering tests make only one of the two calls
func TestRebaseline_ForeignEditToBegunCardStaysRefused(t *testing.T) {
	fx := newBeginFixture(t)
	beginAndFinishBatchOne(t, fx)
	recorded := maps.Clone(fx.Deps.State.Batches[1].CardHashes)

	card := filepath.Join(fx.PlanDir, "01-json-flag.md")
	if err := os.WriteFile(card, []byte("# Card 1 — json-flag\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** edited by someone else.\n"), 0o644); err != nil {
		t.Fatalf("edit card: %v", err)
	}
	if _, err := websterengine.BeginBatch(fx.Deps, 2); !errors.Is(err, websterengine.ErrFingerprintMismatch) {
		t.Fatalf("BeginBatch(2) error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
	}
	if got := fx.Deps.State.Batches[1].CardHashes; !maps.Equal(got, recorded) {
		t.Errorf("CardHashes = %v; want %v unchanged by the refused call", got, recorded)
	}

	deps := rebaselineFixtureDeps(fx)
	deps.Cards = []int{1}
	if _, err := websterengine.Rebaseline(deps); !errors.Is(err, websterengine.ErrRebaselineCardSetChanged) {
		t.Fatalf("Rebaseline() naming card 1 error = %v; want errors.Is(err, ErrRebaselineCardSetChanged)", err)
	}
}
