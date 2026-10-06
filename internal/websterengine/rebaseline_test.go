// rebaseline_test.go exercises Rebaseline (Tier 1), reusing the begin fixture for the foreign-edit scenario and building bare RebaselineDeps for the card-set refusals.
package websterengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"os"
	"path/filepath"
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
	worktree := t.TempDir()
	git := newFakeGit()
	base := git.head
	for _, rec := range recs {
		if rec.StartSHA == "startsha1" {
			rec.StartSHA = base
		}
	}
	return websterengine.RebaselineDeps{
		Plan:    &planparser.Plan{Dir: planDir, Format: 5},
		Batches: batches,
		State:   &websterengine.State{PlanFingerprint: "old-fingerprint", Batches: recs},
		Geom:    websterengine.Geometry{WorktreeRoot: worktree, WebsterDir: t.TempDir(), Git: git},
	}
}

// TestRebaseline_CardSet proves Rebaseline compares a begun batch's recorded card set with the edited
// plan's: a removed, renumbered or regrouped card refuses, leaving the fingerprint and naming the
// batch, the card and the fresh-restart steps whether or not the record carries a StartSHA, while a
// legacy record with no card set is accepted and restamped.
func TestRebaseline_CardSet(t *testing.T) {
	t.Parallel()

	two := batcher.Batch{Cards: []planparser.Card{
		{Number: 1, Slug: "json-flag"},
		{Number: 2, Slug: "list-tests"},
	}}
	cases := []struct {
		name    string
		batches []batcher.Batch
		// edit adjusts the batch-1 record before the call.
		edit       func(rec *websterengine.BatchState)
		wantRefuse bool
	}{
		{name: "removed card", batches: []batcher.Batch{beginCard(2, "list-tests")}, wantRefuse: true},
		{name: "renumbered card", batches: []batcher.Batch{beginCard(1, "other-slug"), beginCard(2, "list-tests")}, wantRefuse: true},
		{name: "regrouped batch", batches: []batcher.Batch{two}, wantRefuse: true},
		{
			name:       "removed card from a record without a StartSHA",
			batches:    []batcher.Batch{beginCard(2, "list-tests")},
			edit:       func(rec *websterengine.BatchState) { rec.StartSHA = "" },
			wantRefuse: true,
		},
		{
			name:    "a legacy record without cards is accepted",
			batches: []batcher.Batch{beginCard(1, "json-flag")},
			edit:    func(rec *websterengine.BatchState) { rec.Cards = nil },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doneBatchOne()
			if tc.edit != nil {
				tc.edit(rec)
			}
			deps := rebaselineDeps(t, tc.batches, map[int]*websterengine.BatchState{1: rec})
			_, err := websterengine.Rebaseline(deps)
			if !tc.wantRefuse {
				if err != nil {
					t.Fatalf("Rebaseline() error = %v; want nil", err)
				}
				if deps.State.PlanFingerprint == "old-fingerprint" {
					t.Error("PlanFingerprint not restamped")
				}
				return
			}
			if !errors.Is(err, websterengine.ErrRebaselineCardSetChanged) {
				t.Fatalf("Rebaseline() error = %v; want errors.Is(err, ErrRebaselineCardSetChanged)", err)
			}
			for _, want := range []string{"batch 1", "01-json-flag", "or 1) lyx webster reset --to start; 2) lyx webster run --fresh"} {
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
			wantText: []string{"batch 1 card 01-json-flag changed since it was begun"},
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
		})
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

// handlePlan is a three-card plan whose card 2 declares a draft handle that card 1 Uses, so canonicalizing card 2's handle rewrites card 1 as well.
func handlePlan(draft bool) plankit.Plan {
	spelling := "Baz"
	if draft {
		spelling = "Bazz"
	}
	base := []plankit.Group{{Label: "Prosa", Targets: []string{"base.txt"}}}
	return plankit.Plan{
		Approved: true,
		Language: "go",
		Cards: []plankit.Card{
			{Number: 1, Slug: "json-flag", Summary: "uses a handle card 2 declares", Groups: base, Uses: []string{"plan:internal/foo#" + spelling}, Intent: "placeholder card."},
			{
				Number:  2,
				Slug:    "list-tests",
				Summary: "declares the handle",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"plan:internal/foo#" + spelling + "` -> `func Baz()"}}},
				Intent:  "declare the handle.",
			},
			{Number: 3, Slug: "third", Summary: "an unbegun card", Groups: base, Intent: "placeholder card."},
		},
	}
}

// beginThenLeaveHandleDraft returns a fixture whose batch 1 is begun and done, and whose plan on disk carries card 2's handle in draft spelling
// with state.json describing exactly those bytes, so the next BeginBatch's canonicalization rewrites begun card 1 (#330).
func beginThenLeaveHandleDraft(t *testing.T) *beginFixture {
	t.Helper()
	fx := newBeginFixture(t)
	writePlan := func(p plankit.Plan) { plankit.Write(t, fx.PlanDir, p) }
	writePlan(handlePlan(false))
	plan, err := planparser.ParsePlan(fx.PlanDir)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	fx.Deps.Plan = plan
	fx.Deps.Batches = append(fx.Deps.Batches, batcher.Batch{Cards: []planparser.Card{{Number: 3, Slug: "third", Title: "third", Intent: "placeholder card third"}}})
	fx.Deps.State.PlanFingerprint = mustFingerprint(t, fx.PlanDir)
	beginAndFinishBatchOne(t, fx)

	writePlan(handlePlan(true))
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

func TestRebaseline_AfterBeginBatchRewroteBegunCard_Regression330(t *testing.T) {
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

	if err := os.WriteFile(filepath.Join(fx.PlanDir, "03-third.md"), []byte("# Card 3 — third\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** reworded.\n"), 0o644); err != nil {
		t.Fatalf("edit card 3: %v", err)
	}
	deps := rebaselineFixtureDeps(fx)
	deps.Cards = []int{3}
	if _, err := websterengine.Rebaseline(deps); err != nil {
		t.Fatalf("Rebaseline() naming card 3 error = %v; want nil", err)
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
