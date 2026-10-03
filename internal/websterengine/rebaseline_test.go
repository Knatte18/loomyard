//go:build integration

// rebaseline_test.go exercises Rebaseline (Tier 2), reusing the begin fixture for the foreign-edit scenario and building bare RebaselineDeps for the card-set refusals.
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
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
	base := gitkit.CommitFile(t, worktree, "base.txt", "base", "base commit")
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
	c0 := strings.TrimSpace(gitkit.Git(t, root, "rev-parse", "HEAD"))
	c1 := gitkit.CommitFile(t, root, "next.txt", "next", "next commit")
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
func handlePlan(draft bool) map[string]string {
	spelling := "Baz"
	if draft {
		spelling = "Bazz"
	}
	return map[string]string{
		"00-overview.md": "---\nformat: 5\napproved: true\nlanguage: go\n---\n\n# Plan: handle fixture\n\n## Card Index\n\n" +
			"1 — json-flag — uses a handle card 2 declares\n2 — list-tests — declares the handle\n3 — third — an unbegun card\n",
		"01-json-flag.md":  "# Card 1 — json-flag\n\n**Prosa:**\n- `base.txt`\n\n**Uses:**\n- `plan:internal/foo#" + spelling + "`\n\n**Intent:** placeholder card.\n",
		"02-list-tests.md": "# Card 2 — list-tests\n\n**Create:**\n- `plan:internal/foo#" + spelling + "` -> `func Baz()`\n\n**Intent:** declare the handle.\n",
		"03-third.md":      "# Card 3 — third\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** placeholder card.\n",
	}
}

// beginThenLeaveHandleDraft returns a fixture whose batch 1 is begun and done, and whose plan on disk carries card 2's handle in draft spelling
// with state.json describing exactly those bytes, so the next BeginBatch's canonicalization rewrites begun card 1 (#330).
func beginThenLeaveHandleDraft(t *testing.T) *beginFixture {
	t.Helper()
	fx := newBeginFixture(t)
	writePlan := func(files map[string]string) {
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(fx.PlanDir, name), []byte(body), 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
	}
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

// TestRebaseline_AfterRecordBatchBoundBegunCard_Regression330 is the #330 scene with a record-batch handle binding as the rewrite:
// binding card 1's handle moves batch 1's recorded hash, so a later Rebaseline naming only an edited pending card succeeds.
func TestRebaseline_AfterRecordBatchBoundBegunCard_Regression330(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})

	planDir := t.TempDir()
	files := map[string]string{
		"00-overview.md":  "---\nformat: 5\napproved: true\nlanguage: go\n---\n\n# Plan: test\n\nframing\n\n## Card Index\n\n1 — json-flag — declares a handle\n2 — pending — an unbegun card\n",
		"01-json-flag.md": "# Card 1 — json-flag\n\n**Create:**\n- `plan:internal/foo#Bar` -> `func Bar() {}`\n\n**Intent:** add Bar\n",
		"02-pending.md":   "# Card 2 — pending\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** placeholder card.\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(planDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	fx.Deps.Geom.PlanDir = planDir
	fx.Deps.Plan = plan
	fx.Deps.Batches = []batcher.Batch{{Cards: plan.Cards[:1]}, {Cards: plan.Cards[1:]}}
	st := fx.Deps.State
	card1 := filepath.Join(planDir, "01-json-flag.md")
	st.Batches[1].CardHashes = map[string]string{"01-json-flag": fileSHA(t, card1)}
	if err := websterengine.RestampPlanBaseline(st, planDir, fx.Deps.Geom.WebsterDir); err != nil {
		t.Fatalf("RestampPlanBaseline() error = %v", err)
	}
	unbound := st.Batches[1].CardHashes["01-json-flag"]

	headSHA := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Bar() {}\n", "01.1: add Bar")
	writeReport(t, fx.ReportsDir, validReport(headSHA))
	if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if got := st.Batches[1].CardHashes["01-json-flag"]; got == unbound || got != fileSHA(t, card1) {
		t.Fatalf("batch 1 CardHashes = %q; want it moved to the bound card's hash %q", got, fileSHA(t, card1))
	}

	if err := os.WriteFile(filepath.Join(planDir, "02-pending.md"), []byte("# Card 2 — pending\n\n**Prosa:**\n- `base.txt`\n\n**Intent:** reworded.\n"), 0o644); err != nil {
		t.Fatalf("edit card 2: %v", err)
	}
	deps := websterengine.RebaselineDeps{Plan: plan, Batches: fx.Deps.Batches, State: st, Geom: fx.Deps.Geom, Cards: []int{2}}
	if _, err := websterengine.Rebaseline(deps); err != nil {
		t.Fatalf("Rebaseline() naming card 2 error = %v; want nil", err)
	}
}

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
