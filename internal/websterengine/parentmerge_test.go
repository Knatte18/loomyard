//go:build integration

// parentmerge_test.go pins the observed wedge end to end (Tier 2 — see docs/benchmarks/running-tests.md):
// a parent merge-in lands between a fork's commit and record-batch, and the run must still record the batch and open the next one.

package websterengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestParentMergeBetweenForkCommitAndRecordBatch is the 2026-09-30 parent-merge-ring regression.
// It walks begin → fork commit → parent merge → record → next begin → next record and asserts each recorded value exactly.
func TestParentMergeBetweenForkCommitAndRecordBatch(t *testing.T) {
	fx := newBeginFixture(t)
	deps := fx.Deps

	// 1. begin-batch 1 records its StartSHA.
	startSHA := gitkit.RevParse(t, fx.Worktree, "HEAD")
	begun, err := websterengine.BeginBatch(deps, 1)
	if err != nil {
		t.Fatalf("BeginBatch(1) error = %v; want nil", err)
	}
	if begun.StartSHA != startSHA || deps.State.Batches[1].StartSHA != startSHA {
		t.Fatalf("batch 1 StartSHA = %q (result %q); want %q", deps.State.Batches[1].StartSHA, begun.StartSHA, startSHA)
	}

	// 2. The fork's commit lands and its OK report names it.
	forkSHA := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n", "1: json-flag")
	reportPath := filepath.Join(deps.Geom.ReportsDir, websterengine.ReportFileName(1, "json-flag"))
	if err := os.WriteFile(reportPath, []byte(validReport(forkSHA)), 0o644); err != nil {
		t.Fatalf("write batch report: %v", err)
	}

	// 3. A parent-side commit off StartSHA is merged --no-ff into the branch.
	base := strings.TrimSpace(gitkit.Git(t, fx.Worktree, "rev-parse", "--abbrev-ref", "HEAD"))
	gitkit.Git(t, fx.Worktree, "checkout", "-b", "parent-side", startSHA)
	gitkit.CommitFile(t, fx.Worktree, "parent.txt", "from the parent", "parent commit")
	gitkit.Git(t, fx.Worktree, "checkout", base)
	gitkit.Git(t, fx.Worktree, "merge", "--no-ff", "-m", "merge parent-side", "parent-side")
	mergeSHA := gitkit.RevParse(t, fx.Worktree, "HEAD")

	// 4. record-batch succeeds at the fork's commit and warns about the moved HEAD.
	contractDir := t.TempDir()
	// A copy of the plan carrying the cards: begin-batch's own Plan stays as the fixture built it.
	recordPlan := *deps.Plan
	for _, b := range deps.Batches {
		recordPlan.Cards = append(recordPlan.Cards, b.Cards...)
	}
	recordDeps := websterengine.RecordDeps{
		Batches: deps.Batches,
		State:   deps.State,
		Config:  websterengine.Config{},
		Engine: &recordFakeEngine{scripted: []shuttleengine.ForkAudit{
			{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
			{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}, {TranscriptPath: "subagents/f2.jsonl", ReportReturned: true}}},
		}},
		Geom:         deps.Geom,
		RefMatcher:   websterengine.NeverMatches{},
		OutcomePath:  filepath.Join(contractDir, "outcome.yaml"),
		SummaryPath:  filepath.Join(contractDir, "summary.md"),
		Sleeper:      &recordFakeSleeper{},
		Plan:         &recordPlan,
		ParentBranch: func() (string, error) { return "parent-side", nil },
	}

	result, err := websterengine.RecordBatch(recordDeps, 1)
	if err != nil {
		t.Fatalf("RecordBatch(1) error = %v; want nil", err)
	}
	if result.Digest == nil || !deps.State.Batches[1].Terminal {
		t.Fatalf("RecordBatch(1) digest = %+v; want batch 1 terminal", result.Digest)
	}
	if got := deps.State.Batches[1].CardSHAs; len(got) != 1 || got[0] != forkSHA {
		t.Errorf("batch 1 CardSHAs = %v; want [%s]", got, forkSHA)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], mergeSHA) {
		t.Errorf("Warnings = %v; want exactly one moved-HEAD warning naming merge %s", result.Warnings, mergeSHA)
	}

	// 5. begin-batch 2 opens at the merge commit.
	begun2, err := websterengine.BeginBatch(deps, 2)
	if err != nil {
		t.Fatalf("BeginBatch(2) error = %v; want nil", err)
	}
	if begun2.StartSHA != mergeSHA || deps.State.Batches[2].StartSHA != mergeSHA {
		t.Errorf("batch 2 StartSHA = %q (result %q); want merge commit %q", deps.State.Batches[2].StartSHA, begun2.StartSHA, mergeSHA)
	}

	// 6. The batch after the merge-in is recorded too: no verb in the ring refuses.
	fork2SHA := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl2.go", "package foo\n", "2: list-tests")
	report2Path := filepath.Join(deps.Geom.ReportsDir, websterengine.ReportFileName(2, "list-tests"))
	if err := os.WriteFile(report2Path, []byte(validReport(fork2SHA)), 0o644); err != nil {
		t.Fatalf("write batch 2 report: %v", err)
	}
	result2, err := websterengine.RecordBatch(recordDeps, 2)
	if err != nil {
		t.Fatalf("RecordBatch(2) error = %v; want nil after the parent merge-in", err)
	}
	if result2.Digest == nil || !deps.State.Batches[2].Terminal {
		t.Fatalf("RecordBatch(2) digest = %+v; want batch 2 terminal", result2.Digest)
	}
	if got := deps.State.Batches[2].CardSHAs; len(got) != 1 || got[0] != fork2SHA {
		t.Errorf("batch 2 CardSHAs = %v; want [%s]", got, fork2SHA)
	}
}
