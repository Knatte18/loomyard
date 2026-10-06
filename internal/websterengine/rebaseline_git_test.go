//go:build integration

// rebaseline_git_test.go pins Rebaseline after a record-batch handle binding, which needs the real delta of a real commit.
// Every other Rebaseline behavior is tested over a fakeGit in rebaseline_test.go.

package websterengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestRebaseline_AfterRecordBatchBoundBegunCard_Regression330 is the #330 scene with a record-batch handle binding as the rewrite:
// binding card 1's handle moves batch 1's recorded hash, so a later Rebaseline naming only an edited pending card succeeds.
func TestRebaseline_AfterRecordBatchBoundBegunCard_Regression330(t *testing.T) {
	fx := newRealRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})

	planDir := t.TempDir()
	plankit.Write(t, planDir, plankit.Plan{
		Approved: true,
		Language: "go",
		Framing:  "framing",
		Cards: []plankit.Card{
			{
				Number:  1,
				Slug:    "json-flag",
				Summary: "declares a handle",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"plan:internal/foo#Bar` -> `func Bar() {}"}}},
				Intent:  "add Bar",
			},
			{Number: 2, Slug: "pending", Summary: "an unbegun card", Groups: []plankit.Group{{Label: "Prosa", Targets: []string{"base.txt"}}}, Intent: "placeholder card."},
		},
	})
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
