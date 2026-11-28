// run_test.go pins Run's behavior over the fakes of run_fixture_test.go:
// the batches it runs, the ladder it climbs, how it stops and how it ends the run.

package weaverengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// mustRun runs the fixture's deps and fails the test on an error.
func mustRun(t *testing.T, fx *runFixture) weaverengine.RunResult {
	t.Helper()
	result, err := weaverengine.Run(fx.deps)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	return result
}

// requireDone fails the test unless result is done after batches batches.
func requireDone(t *testing.T, result weaverengine.RunResult, batches int) {
	t.Helper()
	if result.Outcome != "done" || result.BatchesDone != batches {
		t.Fatalf("Run() = %+v; want done after %d batch(es)", result, batches)
	}
}

// requireStuck fails the test unless result is stuck with a reason holding every want.
func requireStuck(t *testing.T, result weaverengine.RunResult, want ...string) {
	t.Helper()
	if result.Outcome != "stuck" {
		t.Fatalf("Run() = %+v; want stuck", result)
	}
	for _, w := range want {
		if !strings.Contains(result.StuckReason, w) {
			t.Errorf("stuck reason lacks %q:\n%s", w, result.StuckReason)
		}
	}
}

// readFile returns the content of path, or fails the test.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("ThreeCardPlanRunsTwoBatchesInOrderAndIsDone", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 2, "one", "two", "three")
		fx.deps.Gate = shuttleengine.GateSpec{{Name: "drift", Attempts: 1, Gate: func() (shuttleengine.GateResult, error) {
			return shuttleengine.GateResult{Passed: true}, nil
		}}}
		fx.runner.scripts = []attemptScript{{commit: true}, {commit: true}}

		result := mustRun(t, fx)

		requireDone(t, result, 2)
		if got := len(fx.runner.specs); got != 2 {
			t.Fatalf("strands started = %d; want 2", got)
		}
		for i, spec := range fx.runner.specs {
			if spec.Model != "model-a" || spec.Role != weaverengine.BatchStrandRole || spec.PromptEngine != "claude" {
				t.Errorf("strand %d = model %q role %q prompt engine %q; want the first rung under role %q rendered for claude", i, spec.Model, spec.Role, spec.PromptEngine, weaverengine.BatchStrandRole)
			}
		}
		if got, want := fx.runner.gateNames(0), []string{"card 1-one", "card 2-two", "batch", "drift"}; !slices.Equal(got, want) {
			t.Errorf("first batch's gates = %v; want %v", got, want)
		}
		if got, want := fx.runner.gateNames(1), []string{"card 3-three", "batch", "drift"}; !slices.Equal(got, want) {
			t.Errorf("second batch's gates = %v; want %v", got, want)
		}
		if strings.Contains(fx.runner.specs[0].Prompt, "--card 2-two --main-packages") {
			t.Errorf("the first batch's prompt names the main packages; only the last batch's gate runs them")
		}
		if !strings.Contains(fx.runner.specs[1].Prompt, "--card 3-three --main-packages") {
			t.Errorf("the last batch's prompt lacks its gate call with the main packages")
		}

		wantCommits := []string{"weaver: batch 01-one", "weaver: batch 03-three", "weaver: run record"}
		if got := fx.commitMessages(); !slices.Equal(got, wantCommits) {
			t.Errorf("commits = %v; want %v", got, wantCommits)
		}
		st := fx.loadState()
		if st.PlanRecordsCommit != "records-head" {
			t.Errorf("PlanRecordsCommit = %q; want the first begin's records head", st.PlanRecordsCommit)
		}
		if st.Stop != nil {
			t.Errorf("Stop = %+v; want none after Done", st.Stop)
		}
		outcome := readFile(t, weaverengine.OutcomePath(fx.deps.Geom.RunDir))
		for _, want := range []string{"outcome: done", "batches_done: 2"} {
			if !strings.Contains(outcome, want) {
				t.Errorf("outcome.yaml lacks %q:\n%s", want, outcome)
			}
		}
		summary := readFile(t, filepath.Join(fx.deps.Geom.RunDir, "summary.md"))
		if !strings.HasPrefix(summary, "# ") || strings.Contains(summary, "## Integration suite triage") || strings.Contains(summary, "## Verify gate fixes") {
			t.Errorf("summary.md = %q; want a title and no triage or fix section", summary)
		}
	})

	t.Run("BeginRecordsAreFixedAndLeaseIsReleased", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		start, _ := fx.git.HeadSHA("")
		fx.runner.scripts = []attemptScript{{report: runplan.Report{Status: runplan.ReportStatusFailed}}, {}}

		requireDone(t, mustRun(t, fx), 1)

		st := fx.loadState()
		if got := st.Batches[1].StartSHA; got != start {
			t.Errorf("batch start = %q; want the HEAD before the first attempt %q", got, start)
		}
		if got := st.Batches[1].Cards; !slices.Equal(got, []string{"01-one"}) {
			t.Errorf("batch cards = %v; want [01-one]", got)
		}
		lease, acquired, err := lock.AcquireWriteLockWithin(filepath.Join(fx.deps.Geom.ScratchDir, "mutate.lock"), time.Second)
		if err != nil || !acquired {
			t.Fatalf("state-mutation lease after Run = acquired %v, %v; want it free", acquired, err)
		}
		_ = lease.Release()
	})

	t.Run("StaleRecordsAreRemovedAndDoneWritesThemAgain", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		geom := fx.deps.Geom
		if err := os.MkdirAll(geom.RunDir, 0o755); err != nil {
			t.Fatal(err)
		}
		outcomePath := weaverengine.OutcomePath(geom.RunDir)
		summaryPath := filepath.Join(geom.RunDir, "summary.md")
		for _, path := range []string{outcomePath, summaryPath} {
			if err := os.WriteFile(path, []byte("outcome: done\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		conflict := &runplan.PlanConflictError{Batch: 1, Cause: runplan.ConflictCausePlanDefect, Findings: []string{"stale"}}
		if err := runplan.WriteConflictRecord(runplan.ConflictDir(geom.AnchorRoot), conflict); err != nil {
			t.Fatal(err)
		}
		fx.runner.scripts = []attemptScript{{report: runplan.Report{Status: runplan.ReportStatusNeedsParent, Need: "grant a permission"}}}

		requireStuck(t, mustRun(t, fx), "grant a permission")

		for _, path := range []string{outcomePath, summaryPath, runplan.ConflictRecordPath(runplan.ConflictDir(geom.AnchorRoot))} {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("stat %s = %v; want it removed by the stuck call", path, err)
			}
		}

		fx.runner.scripts = append(fx.runner.scripts, attemptScript{})
		requireDone(t, mustRun(t, fx), 1)
		for _, path := range []string{outcomePath, summaryPath} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("stat %s = %v; want the Done return to write it", path, err)
			}
		}
	})

	t.Run("GateConfigWithoutAKeyRefusesNamingTheWayForward", func(t *testing.T) {
		t.Parallel()
		for _, key := range []string{"card", "batch"} {
			fx := newRunFixture(t, 1, "one")
			fx.deps.Geom.GateConfig = func() (gateslot.Config, error) {
				cfg := testGateConfig()
				delete(cfg.Gates, key)
				return cfg, nil
			}
			requireStuck(t, mustRun(t, fx), `gates holds no key "`+key+`"`, "lym config gate")
			if len(fx.runner.specs) != 0 {
				t.Errorf("without the %q key, %d strand(s) started; want none", key, len(fx.runner.specs))
			}
		}
	})

	failures := []struct {
		name string
		// first scripts the failing first attempt.
		first attemptScript
		// setup prepares the fixture's index or runner.
		setup func(fx *runFixture)
		// cause is what the failed attempt's recorded cause holds.
		cause string
		// journaled reports whether the first attempt's report parsed, so its journal entry is appended.
		journaled bool
	}{
		{name: "FailedReport", first: attemptScript{report: runplan.Report{Status: runplan.ReportStatusFailed, Journal: "tried"}}, cause: "reported status FAILED", journaled: true},
		{name: "FailingGate", first: attemptScript{failGate: "batch", report: runplan.Report{Journal: "tried"}}, cause: "batch gate still failed", journaled: true},
		{name: "UncleanTree", first: attemptScript{dirty: []string{"x.go"}, report: runplan.Report{Journal: "tried"}}, cause: "uncommitted paths", journaled: true},
		{name: "TimeoutWithNoReport", first: attemptScript{noReport: true}, cause: "timed out"},
		{name: "UnparseableReport", first: attemptScript{rawReport: "status: [\n"}, cause: "does not parse"},
		{name: "HeadThatIsNotHEAD", first: attemptScript{commit: true, report: runplan.Report{HeadSHA: strings.Repeat("ab", 20), Journal: "tried"}}, cause: "is not the worktree's HEAD", journaled: true},
		{
			name:      "RejectedMerge",
			first:     attemptScript{commit: true, mergeOnTop: true, report: runplan.Report{Journal: "tried"}},
			setup:     func(fx *runFixture) { fx.git.rejection = "it adds content of its own" },
			cause:     "it adds content of its own",
			journaled: true,
		},
		{
			name:  "DoneCheckFinding",
			first: attemptScript{report: runplan.Report{Journal: "tried"}},
			setup: func(fx *runFixture) {
				finding := planindex.Finding{Check: "create-not-done", Card: "1-one", Severity: planindex.SeverityBlocking, Detail: "internal/x/one.go does not resolve"}
				fx.runner.onStart = func(index int) {
					fx.index.notDone = nil
					if index == 0 {
						fx.index.notDone = []planindex.Finding{finding}
					}
				}
			},
			cause:     "internal/x/one.go does not resolve",
			journaled: true,
		},
		{
			name:  "RemovedLaterTarget",
			first: attemptScript{report: runplan.Report{Journal: "tried"}},
			setup: func(fx *runFixture) {
				fx.index.removed = []planindex.Finding{{Check: "removed-target", Card: "2-two", Severity: planindex.SeverityBlocking, Detail: "pkg#Gone is removed"}}
			},
			cause:     "pkg#Gone is removed",
			journaled: true,
		},
	}
	for _, tt := range failures {
		t.Run("FailedAttemptClimbs_"+tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, 1, "one")
			fx.deps.Ladder[1].Engine = shuttlefake.EngineName
			stencilkit.SeedEditDirectiveCopies(t, fx.deps.Geom.StencilsDir, shuttlefake.EngineName)
			fx.runner.scripts = []attemptScript{tt.first, {}}
			if tt.setup != nil {
				tt.setup(fx)
			}

			requireDone(t, mustRun(t, fx), 1)

			if got := len(fx.runner.specs); got != 2 {
				t.Fatalf("strands started = %d; want 2", got)
			}
			if fx.runner.specs[0].Model != "model-a" || fx.runner.specs[1].Model != "model-b" {
				t.Errorf("models = %q then %q; want the next rung's model for the second attempt", fx.runner.specs[0].Model, fx.runner.specs[1].Model)
			}
			if fx.runner.specs[0].PromptEngine != "claude" || fx.runner.specs[1].PromptEngine != shuttlefake.EngineName {
				t.Errorf("prompt engines = %q then %q; want %q then %q, each rung's own engine", fx.runner.specs[0].PromptEngine, fx.runner.specs[1].PromptEngine, "claude", shuttlefake.EngineName)
			}
			st := fx.loadState()
			first, second := st.Attempts[0], st.Attempts[1]
			if first.Outcome != "failed" || !strings.Contains(first.Cause, tt.cause) {
				t.Errorf("first attempt = %q %q; want failed with a cause holding %q", first.Outcome, first.Cause, tt.cause)
			}
			if second.Outcome != "done" || second.Number != 2 {
				t.Errorf("second attempt = %q number %d; want done, number 2", second.Outcome, second.Number)
			}
			if st.BatchEnds[1] == "" || st.LastEnd != st.BatchEnds[1] {
				t.Errorf("batch end = %q, last end = %q; want the end commit recorded", st.BatchEnds[1], st.LastEnd)
			}
			if strings.Contains(fx.runner.specs[1].Prompt, "## Why this attempt exists\n\nnone") {
				t.Errorf("the second attempt's prompt tells it nothing of the failure")
			}
			if tt.name == "UncleanTree" {
				if !slices.Equal(first.Uncommitted, []string{"x.go"}) || !strings.Contains(fx.runner.specs[1].Prompt, "x.go") {
					t.Errorf("uncommitted paths = %v, second prompt lacks them: %v; want them recorded and told", first.Uncommitted, !strings.Contains(fx.runner.specs[1].Prompt, "x.go"))
				}
			}
			journal, _ := os.ReadFile(weaverengine.JournalPath(fx.deps.Geom.RunDir))
			if got := strings.Contains(string(journal), "## Batch 01-one, attempt 1"); got != tt.journaled {
				t.Errorf("journal holds attempt 1's entry = %v; want %v", got, tt.journaled)
			}
			wantCommits := []string{"weaver: batch 01-one", "weaver: batch 01-one", "weaver: run record"}
			if got := fx.commitMessages(); !slices.Equal(got, wantCommits) {
				t.Errorf("commits = %v; want each attempt's end committed: %v", got, wantCommits)
			}
			if !slices.Equal(fx.runner.stops, []string{first.StrandGUID}) {
				t.Errorf("stopped strands = %v; want the failed attempt's strand %q alone", fx.runner.stops, first.StrandGUID)
			}
		})
	}

	t.Run("AbbreviatedHeadAndCleanParentMergeAreRecordedDone", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one", "two")
		fx.runner.scripts = []attemptScript{{commit: true, abbreviate: true}, {commit: true, mergeOnTop: true}}

		requireDone(t, mustRun(t, fx), 2)

		if got := len(fx.runner.specs); got != 2 {
			t.Errorf("strands started = %d; want each batch recorded by its first attempt", got)
		}
	})

	t.Run("ExhaustedLadderStopsNamingTheBatchAndRecordsTheStop", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		failed := attemptScript{report: runplan.Report{Status: runplan.ReportStatusFailed, Journal: "tried"}}
		fx.runner.scripts = []attemptScript{failed, failed, failed}

		result := mustRun(t, fx)

		geom := fx.deps.Geom
		requireStuck(t, result, "batch 01-one", filepath.Join(geom.RunDir, "state.json"), "01-one-a3.yaml", weaverengine.JournalPath(geom.RunDir), "re-step the row")
		if strings.Contains(result.StuckReason, "lym webster") {
			t.Errorf("stuck reason names a webster verb: %s", result.StuckReason)
		}
		if got := len(fx.runner.specs); got != 3 {
			t.Errorf("strands started = %d; want one per rung", got)
		}
		st := fx.loadState()
		if st.Stop == nil || st.Stop.Kind != "batch" || st.Stop.Batch != 1 || st.Stop.Rung != 2 {
			t.Errorf("Stop = %+v; want a batch stop on batch 1 at rung 2", st.Stop)
		}
		if _, err := os.Stat(weaverengine.OutcomePath(geom.RunDir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat outcome.yaml = %v; want none for a stuck run", err)
		}
	})

	t.Run("MachineryLossRetriesOnceOnTheSameRungThenClimbs", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.runner.scripts = []attemptScript{{dieAfterStart: true}, {dieAfterStart: true}, {}}

		requireDone(t, mustRun(t, fx), 1)

		models := []string{}
		for _, spec := range fx.runner.specs {
			models = append(models, spec.Model)
		}
		if want := []string{"model-a", "model-a", "model-b"}; !slices.Equal(models, want) {
			t.Errorf("models = %v; want the loss retried on its rung, then the second loss climbed: %v", models, want)
		}
		st := fx.loadState()
		if st.Attempts[0].Outcome != "lost" || st.Attempts[1].Outcome != "lost" || st.Attempts[1].Number != 2 {
			t.Errorf("attempts = %q, %q (number %d); want two lost attempts with new numbers", st.Attempts[0].Outcome, st.Attempts[1].Outcome, st.Attempts[1].Number)
		}
	})

	t.Run("StartFailureIsALoss", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.runner.scripts = []attemptScript{{startErr: errors.New("no pane")}, {}}

		requireDone(t, mustRun(t, fx), 1)

		st := fx.loadState()
		if st.Attempts[0].Outcome != "lost" || !strings.Contains(st.Attempts[0].Cause, "no pane") || fx.runner.specs[1].Model != "model-a" {
			t.Errorf("first attempt = %q %q, second model %q; want a lost start retried on the same rung", st.Attempts[0].Outcome, st.Attempts[0].Cause, fx.runner.specs[1].Model)
		}
	})

	t.Run("DeliveredHeadingWrittenDuringAnAttemptSurvivesItsEnd", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.runner.scripts = []attemptScript{{}}
		geom := fx.deps.Geom
		fx.runner.onStart = func(int) {
			st, err := weaverengine.LoadState(geom.RunDir, geom.ScratchDir)
			if err != nil || st == nil {
				t.Errorf("LoadState() = %v, %v; want the attempt in flight", st, err)
				return
			}
			st.Attempts[0].Delivered = []string{"## a decision"}
			if err := weaverengine.SaveState(geom.RunDir, geom.ScratchDir, st); err != nil {
				t.Errorf("SaveState() error = %v", err)
			}
		}

		requireDone(t, mustRun(t, fx), 1)

		if got := fx.loadState().Attempts[0].Delivered; !slices.Equal(got, []string{"## a decision"}) {
			t.Errorf("delivered = %v; want the heading another process recorded kept", got)
		}
	})

	t.Run("NeedsParentStopsAtOnceWithItsNeed", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.runner.scripts = []attemptScript{{report: runplan.Report{Status: runplan.ReportStatusNeedsParent, Need: "run lym fabric merge"}}}

		requireStuck(t, mustRun(t, fx), "run lym fabric merge")

		if len(fx.runner.specs) != 1 || len(fx.runner.stops) != 1 {
			t.Errorf("strands started = %d, stopped = %d; want one each and no climb", len(fx.runner.specs), len(fx.runner.stops))
		}
		if st := fx.loadState(); st.Stop == nil || st.Stop.Kind != "needs_parent" {
			t.Errorf("Stop = %+v; want a needs_parent stop", st.Stop)
		}
	})

	t.Run("FailedStopStartsNothingAndNamesTheGUID", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.runner.stopErr = errors.New(strings.Repeat("reed is gone ", 100))
		fx.runner.scripts = []attemptScript{{report: runplan.Report{Status: runplan.ReportStatusFailed}}}

		result := mustRun(t, fx)

		st := fx.loadState()
		requireStuck(t, result, st.Attempts[0].StrandGUID, "lym reed remove "+st.Attempts[0].StrandGUID, "...")
		if len(fx.runner.specs) != 1 {
			t.Errorf("strands started = %d; want no next attempt after a failed stop", len(fx.runner.specs))
		}
	})

	t.Run("FailedStopOfANeedsParentAttemptStillRecordsANeedsParentStop", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.runner.stopErr = errors.New("reed is gone")
		fx.runner.scripts = []attemptScript{{report: runplan.Report{Status: runplan.ReportStatusNeedsParent, Need: "run lym fabric merge"}}, {commit: true}}

		requireStuck(t, mustRun(t, fx), "lym reed remove", "run lym fabric merge")

		if st := fx.loadState(); st.Stop == nil || st.Stop.Kind != "needs_parent" || st.Stop.Rung != 0 {
			t.Fatalf("Stop = %+v; want a needs_parent stop on rung 0", st.Stop)
		}
		fx.runner.stopErr = nil

		requireDone(t, mustRun(t, fx), 1)

		if len(fx.runner.specs) != 2 || fx.runner.specs[1].Model != "model-a" {
			t.Fatalf("started = %d strand(s); want a second attempt on the asking rung", len(fx.runner.specs))
		}
		if len(fx.verify.calls) != 1 {
			t.Errorf("verify calls = %d; want the plan-level verify alone, with no re-entry gate run", len(fx.verify.calls))
		}
	})

	stateWriteFailures := []struct {
		name   string
		report runplan.Report
	}{
		{"FailedAttempt", runplan.Report{Status: runplan.ReportStatusFailed}},
		{"DoneAttempt", runplan.Report{}},
	}
	for _, tt := range stateWriteFailures {
		t.Run("FailedStateWriteAfterAnOutcomeStopsTheStrandAndNamesTheFault_"+tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, 1, "one")
			statePath := filepath.Join(fx.deps.Geom.RunDir, "state.json")
			fx.runner.scripts = []attemptScript{{
				report: tt.report,
				onGate: func() {
					if err := os.Remove(statePath); err != nil {
						t.Errorf("remove the state file: %v", err)
					}
					if err := os.MkdirAll(filepath.Join(statePath, "held"), 0o755); err != nil {
						t.Errorf("replace the state file with a directory: %v", err)
					}
				},
			}}

			requireStuck(t, mustRun(t, fx), "weaver: load state")

			if len(fx.runner.stops) != 1 {
				t.Errorf("stopped strands = %v; want the attempt's strand stopped all the same", fx.runner.stops)
			}
			if len(fx.runner.specs) != 1 {
				t.Errorf("strands started = %d; want no next attempt", len(fx.runner.specs))
			}
		})
	}

	t.Run("ReportedRenameRewritesAPendingCardAndTheNextBatchIsBegunFromIt", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one", "two")
		fx.index.rewriteRenames = true
		card := filepath.Join(fx.deps.Geom.PlanDir, "02-two.md")
		pending, err := os.OpenFile(card, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pending.WriteString("\nThis card calls internal/x#Old.\n"); err != nil {
			t.Fatal(err)
		}
		_ = pending.Close()
		rename := runplan.ReportRename{Old: "internal/x#Old", New: "internal/x#New", Reason: "the old name collides"}
		fx.runner.scripts = []attemptScript{{report: runplan.Report{Renames: []runplan.ReportRename{rename}}}, {}}

		requireDone(t, mustRun(t, fx), 2)

		text := readFile(t, card)
		if !strings.Contains(text, "internal/x#New") || strings.Contains(text, "internal/x#Old") {
			t.Errorf("pending card = %q; want the rename rewritten into it", text)
		}
		sum := sha256.Sum256([]byte(text))
		if got := fx.loadState().Batches[2].CardHashes["02-two"]; got != hex.EncodeToString(sum[:]) {
			t.Errorf("batch 2 card hash = %q; want the hash of the rewritten card", got)
		}
	})

	t.Run("OnlyTheRunsLastBatchIsBuiltAsLastAndTheFirstBatchRefusesARemovedLaterTarget", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one", "two")
		fx.index.removedOnlyWithLaterCards = true
		fx.index.removed = []planindex.Finding{{Check: "removed-target", Card: "2-two", Severity: planindex.SeverityBlocking, Detail: "pkg#Gone is removed"}}
		fx.runner.scripts = []attemptScript{{}, {}, {}}

		requireDone(t, mustRun(t, fx), 2)

		st := fx.loadState()
		if first := st.Attempts[0]; first.Batch != 1 || first.Outcome != "failed" || !strings.Contains(first.Cause, "pkg#Gone is removed") {
			t.Errorf("first attempt = batch %d %q %q; want batch 1 refused for the removed later target", first.Batch, first.Outcome, first.Cause)
		}
		if got := len(fx.runner.specs); got != 3 {
			t.Fatalf("strands started = %d; want the first batch's two attempts and the last batch's one", got)
		}
		for i := range 2 {
			if strings.Contains(fx.runner.specs[i].Prompt, "--card 1-one --main-packages") {
				t.Errorf("attempt %d of the first batch is built as the last batch", i+1)
			}
		}
		if !strings.Contains(fx.runner.specs[2].Prompt, "--card 2-two --main-packages") {
			t.Errorf("the last batch's prompt lacks its gate call with the main packages")
		}
	})

	t.Run("FailedBatchCommitStopsTheStrandAndNamesTheReStep", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.commitErr = func(string) error { return errors.New("the records repository is locked") }
		fx.runner.scripts = []attemptScript{{report: runplan.Report{Status: runplan.ReportStatusFailed}}}

		requireStuck(t, mustRun(t, fx), "the records repository is locked", "transient", "re-step the row")

		if len(fx.runner.stops) != 1 {
			t.Errorf("stopped strands = %v; want the failed attempt's strand stopped despite the commit failure", fx.runner.stops)
		}
	})

	t.Run("PlanConflictAtBeginWritesTheRecordAndRecordsNoStop", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.index.dispatch = []planindex.Finding{{Check: "glyph-not-found", Card: "1-one", Severity: planindex.SeverityBlocking, Detail: "pkg#Missing does not resolve"}}

		result := mustRun(t, fx)

		requireStuck(t, result, "pkg#Missing does not resolve", "plan review", "re-step the row")
		if strings.Contains(result.StuckReason, "lym webster") {
			t.Errorf("stuck reason names a webster verb: %s", result.StuckReason)
		}
		record, err := runplan.ReadConflictRecord(runplan.ConflictDir(fx.deps.Geom.AnchorRoot))
		if err != nil || len(record.Findings) == 0 {
			t.Errorf("conflict record = %+v, %v; want the findings written", record, err)
		}
		if len(fx.runner.specs) != 0 {
			t.Errorf("strands started = %d; want none", len(fx.runner.specs))
		}
		if st, _ := weaverengine.LoadState(fx.deps.Geom.RunDir, fx.deps.Geom.ScratchDir); st != nil && st.Stop != nil {
			t.Errorf("Stop = %+v; want none for a plan conflict", st.Stop)
		}
	})

	t.Run("VerifyFailureSpawnsOneFixerAndRunsTheVerifyAgain", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.verify.scripts = []verifyScript{{failing: "TestBroken"}, {failing: "TestBroken"}}
		fx.fixer.scripts = []fixerScript{{commit: "fix: repair the broken test"}}

		requireDone(t, mustRun(t, fx), 1)

		if len(fx.fixer.specs) != 1 {
			t.Fatalf("fixers spawned = %d; want 1", len(fx.fixer.specs))
		}
		spec := fx.fixer.specs[0]
		if spec.Timeout != 7*time.Minute || spec.ModelChoice.Model != "model-a" || spec.PromptEngine != "claude" || len(spec.Prompt) == 0 || !strings.Contains(string(spec.Prompt), "TestBroken") {
			t.Errorf("fixer spec = timeout %v, model %q, prompt engine %q, prompt names the failure %v; want fixer_timeout_min, the first rung, claude and the findings",
				spec.Timeout, spec.ModelChoice.Model, spec.PromptEngine, strings.Contains(string(spec.Prompt), "TestBroken"))
		}
		if len(fx.verify.calls) != 3 {
			t.Errorf("verify calls = %d; want the run, its rerun and the run after the fixer", len(fx.verify.calls))
		}
		if _, err := os.Stat(runplan.VerifyGateReportPath(fx.deps.Geom.ReportsDir)); err != nil {
			t.Errorf("verify-failure record: %v", err)
		}
		summary := readFile(t, filepath.Join(fx.deps.Geom.RunDir, "summary.md"))
		for _, want := range []string{"## Verify gate fixes", "TestBroken", "fix: repair the broken test"} {
			if !strings.Contains(summary, want) {
				t.Errorf("summary.md lacks %q:\n%s", want, summary)
			}
		}
	})

	t.Run("VerifyThatPassesOnRerunIsDoneWithFlakyIdentitiesAndNoFixer", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.verify.scripts = []verifyScript{{failing: "TestFlaky"}}

		requireDone(t, mustRun(t, fx), 1)

		if len(fx.fixer.specs) != 0 {
			t.Errorf("fixers spawned = %d; want none for a flaky pass", len(fx.fixer.specs))
		}
		summary := readFile(t, filepath.Join(fx.deps.Geom.RunDir, "summary.md"))
		if !strings.Contains(summary, "## Integration suite triage") || !strings.Contains(summary, "example.test/pkg.TestFlaky") {
			t.Errorf("summary.md lacks the flaky identity under its triage section:\n%s", summary)
		}
	})

	t.Run("VerifyTimeoutAndDirtyTreeAreNotRerunAndGoToTheFixer", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.verify.scripts = []verifyScript{{timedOut: true}, {status: verifytree.StatusDirty, dirty: []string{"left-over.go"}}}

		requireDone(t, mustRun(t, fx), 1)

		if len(fx.verify.calls) != 3 {
			t.Errorf("verify calls = %d; want one per evaluation and no rerun of a timeout or a dirty tree", len(fx.verify.calls))
		}
		if len(fx.fixer.specs) != 2 || !strings.Contains(string(fx.fixer.specs[1].Prompt), "left-over.go") {
			t.Errorf("fixers = %d, second prompt names the dirty path: want 2 and yes", len(fx.fixer.specs))
		}
	})

	t.Run("RunnerCrashStopsTheRowWithNoFixerAndNoRecord", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.verify.scripts = []verifyScript{{crash: true}}

		requireStuck(t, mustRun(t, fx), "test runner crashed")

		if len(fx.fixer.specs) != 0 {
			t.Errorf("fixers spawned = %d; want none for a runner crash", len(fx.fixer.specs))
		}
		if _, err := os.Stat(runplan.VerifyGateReportPath(fx.deps.Geom.ReportsDir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat verify-failure record = %v; want none for a runner crash", err)
		}
		if st := fx.loadState(); st.Stop == nil || st.Stop.Kind != "verify" {
			t.Errorf("Stop = %+v; want a verify stop", st.Stop)
		}
	})

	t.Run("FixerLadderClimbsAndAFailureAfterTheLastRungStops", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.deps.Ladder[1].Engine = shuttlefake.EngineName
		stencilkit.SeedEditDirectiveCopies(t, fx.deps.Geom.StencilsDir, shuttlefake.EngineName)
		for range 8 {
			fx.verify.scripts = append(fx.verify.scripts, verifyScript{failing: "TestBroken"})
		}

		result := mustRun(t, fx)

		requireStuck(t, result, "plan-level verify", "TestBroken")
		var models, promptEngines []string
		for _, spec := range fx.fixer.specs {
			models = append(models, spec.ModelChoice.Model)
			promptEngines = append(promptEngines, spec.PromptEngine)
		}
		if want := []string{"model-a", "model-b", "model-c"}; !slices.Equal(models, want) {
			t.Errorf("fixer models = %v; want one fixer per rung %v", models, want)
		}
		if want := []string{"claude", shuttlefake.EngineName, "claude"}; !slices.Equal(promptEngines, want) {
			t.Errorf("fixer prompt engines = %v; want each rung's own engine %v", promptEngines, want)
		}
		if st := fx.loadState(); st.Stop == nil || st.Stop.Kind != "verify" {
			t.Errorf("Stop = %+v; want a verify stop", st.Stop)
		}
	})

	t.Run("LostFixerIsRetriedOnItsRungWithItsRecordSavedTerminal", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.verify.scripts = []verifyScript{{failing: "TestBroken"}, {failing: "TestBroken"}, {failing: "TestBroken"}, {failing: "TestBroken"}}
		fx.fixer.scripts = []fixerScript{{dead: true}, {}}

		requireDone(t, mustRun(t, fx), 1)

		st := fx.loadState()
		if len(st.Fixers) != 2 || st.Fixers[0].Outcome != "lost" || st.Fixers[0].Record == nil || st.Fixers[0].Record.Status != "dead" {
			t.Fatalf("fixers = %+v; want a first fixer lost with its record saved dead", st.Fixers)
		}
		if st.Fixers[1].Rung != 0 || st.Fixers[1].Outcome != "done" {
			t.Errorf("second fixer = rung %d, %q; want the same rung and done", st.Fixers[1].Rung, st.Fixers[1].Outcome)
		}
	})

	t.Run("FixerWhoseStopFailedStartsNoNextFixer", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.verify.scripts = []verifyScript{{failing: "TestBroken"}, {failing: "TestBroken"}}
		fx.fixer.scripts = []fixerScript{{warnings: []string{"stop strand fixer-1: reed is gone"}}}

		requireStuck(t, mustRun(t, fx), "fixer-1", "lym reed remove fixer-1")

		if len(fx.fixer.specs) != 1 {
			t.Errorf("fixers spawned = %d; want no next fixer after a failed stop", len(fx.fixer.specs))
		}
	})

	t.Run("FailedCommitOfTheRunRecordStopsNamingTheReStep", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.commitErr = func(msg string) error {
			if msg == "weaver: run record" {
				return errors.New("the records repository is locked")
			}
			return nil
		}

		requireStuck(t, mustRun(t, fx), "the records repository is locked", "re-step the row")
	})

	t.Run("HeldRunLockReturnsErrRunBusyNamingTheReStep", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		if err := os.MkdirAll(fx.deps.Geom.ScratchDir, 0o755); err != nil {
			t.Fatal(err)
		}
		held, acquired, err := lock.TryAcquireWriteLock(filepath.Join(fx.deps.Geom.ScratchDir, "run.lock"))
		if err != nil || !acquired {
			t.Fatalf("acquire the run lock = %v, %v", acquired, err)
		}
		defer func() { _ = held.Release() }()

		_, err = weaverengine.Run(fx.deps)

		if !errors.Is(err, weaverengine.ErrRunBusy) || !strings.Contains(err.Error(), "re-step the row") {
			t.Errorf("Run() error = %v; want ErrRunBusy naming the re-step", err)
		}
	})

	t.Run("FirstBatchBeginRemovesAStaleResetPendingMarker", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		marker := filepath.Join(fx.deps.Geom.ScratchDir, "reset-pending")
		if err := os.MkdirAll(fx.deps.Geom.ScratchDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(marker, []byte("sha\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		requireDone(t, mustRun(t, fx), 1)

		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat reset-pending = %v; want the stale marker removed", err)
		}
	})

	t.Run("UntoldSeamIsAWiringErrorNamingTheField", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.deps.Runner = nil

		if _, err := weaverengine.Run(fx.deps); err == nil || !strings.Contains(err.Error(), "RunDeps.Runner") {
			t.Errorf("Run() error = %v; want one naming RunDeps.Runner", err)
		}
	})
}
