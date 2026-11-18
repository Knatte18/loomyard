package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordSeenReed wraps a fakeReed and notes the outcome run.json holds at the moment a strand is removed.
type recordSeenReed struct {
	*fakeReed
	runDir          string
	outcomeAtRemove string
}

func (r *recordSeenReed) RemoveStrand(guid string, recursive bool) (reedengine.Removed, error) {
	if record, found, err := loadRunState(r.runDir); err == nil && found {
		r.outcomeAtRemove = record.Outcome
	}
	return r.fakeReed.RemoveStrand(guid, recursive)
}

// TestStop_RecordsTheStopBeforeRemovingTheStrand tables the handle form and the guid form.
func TestStop_RecordsTheStopBeforeRemovingTheStrand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	liveStrand := []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}
	tests := []struct {
		name         string
		outcome      string
		outputExists bool
		strands      []reedengine.StrandStatus
		statusErr    error
		removeErr    error
		wantOutcome  string
		wantRemoved  bool
		wantErr      bool
	}{
		{name: "running_run_is_recorded_stopped_then_removed", outcome: runOutcomeRunning, strands: liveStrand, wantOutcome: runOutcomeStopped, wantRemoved: true},
		{name: "running_run_with_every_output_file_keeps_running", outcome: runOutcomeRunning, outputExists: true, strands: liveStrand, wantOutcome: runOutcomeRunning, wantRemoved: true},
		{name: "terminal_died_record_keeps_its_outcome", outcome: "died", strands: liveStrand, wantOutcome: "died", wantRemoved: true},
		{name: "dead_strand_is_recorded_and_not_removed", outcome: runOutcomeRunning, strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1"}}, wantOutcome: runOutcomeStopped},
		{name: "dead_own_window_strand_is_recorded_and_removed", outcome: runOutcomeRunning, strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", OwnWindow: true, Present: true}}, wantOutcome: runOutcomeStopped, wantRemoved: true},
		{name: "own_window_strand_with_no_present_pane_is_recorded_and_not_removed", outcome: runOutcomeRunning, strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", OwnWindow: true}}, wantOutcome: runOutcomeStopped},
		{name: "removal_failure_leaves_the_record_terminal_and_returns_the_error", outcome: runOutcomeRunning, strands: liveStrand, removeErr: errors.New("reed down"), wantOutcome: runOutcomeStopped, wantRemoved: true, wantErr: true},
		{name: "probe_failure_leaves_the_record_terminal_and_removes_nothing", outcome: runOutcomeRunning, statusErr: errors.New("reed unreadable"), wantOutcome: runOutcomeStopped, wantErr: true},
	}
	for _, form := range []string{"handle", "guid"} {
		for _, tt := range tests {
			t.Run(form+"/"+tt.name, func(t *testing.T) {
				t.Parallel()

				fake := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: tt.strands}}, StatusErr: tt.statusErr, RemoveStrandErr: tt.removeErr}
				fx := newFixture(t, fake, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
				outputFile := filepath.Join(fx.RunRoot, "out.md")
				if tt.outputExists {
					touchOutputFile(t, outputFile)
				}
				runDir := seedAttachRun(t, fx.RunRoot, "run-1", seedAttachRunOpts{
					strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: tt.outcome, includeOutcome: true,
				})
				seen := &recordSeenReed{fakeReed: fake, runDir: runDir}
				fx.Runner.reed = seen

				var err error
				if form == "handle" {
					record, _, loadErr := loadRunState(runDir)
					if loadErr != nil {
						t.Fatalf("loadRunState: %v", loadErr)
					}
					err = fx.newRun(Spec{OutputFiles: []string{outputFile}}, withRunDir(runDir), withRunState(record)).Stop()
				} else {
					err = fx.Runner.StopStrand("strand-1")
				}

				if (err != nil) != tt.wantErr {
					t.Fatalf("stop error = %v; want error: %v", err, tt.wantErr)
				}
				record, _, loadErr := loadRunState(runDir)
				if loadErr != nil {
					t.Fatalf("loadRunState: %v", loadErr)
				}
				if record.Outcome != tt.wantOutcome {
					t.Errorf("recorded outcome = %q; want %q", record.Outcome, tt.wantOutcome)
				}
				if removed := len(fake.RemoveStrandCalls) > 0; removed != tt.wantRemoved {
					t.Errorf("strand removed = %v; want %v", removed, tt.wantRemoved)
				}
				if tt.wantRemoved && seen.outcomeAtRemove != tt.wantOutcome {
					t.Errorf("record read %q when the strand was removed; want %q written first", seen.outcomeAtRemove, tt.wantOutcome)
				}
			})
		}
	}
}

// TestStop_FailedRecordWriteRemovesNothingAndRestoresTheHandle covers a handle whose record cannot be written.
func TestStop_FailedRecordWriteRemovesNothingAndRestoresTheHandle(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	fake := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}}}
	fx := newFixture(t, fake, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	run := fx.newRun(Spec{OutputFiles: []string{filepath.Join(fx.RunRoot, "out.md")}},
		withRunDir(filepath.Join(blocker, "run")), withRunState(RunState{StrandGUID: "strand-1", Outcome: runOutcomeRunning}))

	if err := run.Stop(); err == nil {
		t.Fatal("Stop() error = nil; want the record write failure")
	}
	if len(fake.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %v; want none after a failed record write", fake.RemoveStrandCalls)
	}
	if run.state.Outcome != runOutcomeRunning || run.stopMarked {
		t.Errorf("handle outcome = %q, stopMarked = %v; want %q unmarked", run.state.Outcome, run.stopMarked, runOutcomeRunning)
	}
}

// TestStopStrand_UnrecordableStopRemovesNothing covers the guid form's record that cannot be read or rewritten.
// The read row calls stopRecorded because FindRun skips an unreadable record;
// only a record that turns unreadable after the scan reaches that refusal.
func TestStopStrand_UnrecordableStopRemovesNothing(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		corrupt func(t *testing.T, runDir string)
	}{
		{name: "unreadable_record", corrupt: func(t *testing.T, runDir string) {
			if err := os.WriteFile(filepath.Join(runDir, runStateFileName), []byte("{not json"), 0o644); err != nil {
				t.Fatalf("corrupt run.json: %v", err)
			}
		}},
		{name: "unwritable_record", corrupt: func(t *testing.T, runDir string) {
			if _, _, err := loadRunState(runDir); err != nil {
				t.Fatalf("loadRunState: %v", err)
			}
			permkit.Chmod(t, runDir, 0o555)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}}}
			fx := newFixture(t, fake, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
			runDir := seedAttachRun(t, fx.RunRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", outputFiles: []string{filepath.Join(fx.RunRoot, "out.md")}, outcome: runOutcomeRunning, includeOutcome: true,
			})
			tt.corrupt(t, runDir)
			before, err := os.ReadFile(filepath.Join(runDir, runStateFileName))
			if err != nil {
				t.Fatalf("read run.json: %v", err)
			}

			if err := fx.Runner.stopRecorded(runDir, "strand-1"); err == nil {
				t.Fatal("stopRecorded() error = nil; want the record failure")
			}

			if len(fake.RemoveStrandCalls) != 0 {
				t.Errorf("RemoveStrand calls = %v; want none when the stop cannot be recorded", fake.RemoveStrandCalls)
			}
			after, err := os.ReadFile(filepath.Join(runDir, runStateFileName))
			if err != nil {
				t.Fatalf("read run.json: %v", err)
			}
			if string(after) != string(before) {
				t.Errorf("run.json = %s; want it untouched: %s", after, before)
			}
		})
	}
}

// TestStopStrand_GuidWithNoRunRecordIsRemovedByStrandAlone covers the guid form for a strand shuttle never recorded.
func TestStopStrand_GuidWithNoRunRecordIsRemovedByStrandAlone(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	fake := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-9", PaneID: "%9", Live: true}}}}}
	fx := newFixture(t, fake, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())

	if err := fx.Runner.StopStrand("strand-9"); err != nil {
		t.Fatalf("StopStrand() error = %v; want nil", err)
	}
	if len(fake.RemoveStrandCalls) != 1 || fake.RemoveStrandCalls[0].GUID != "strand-9" {
		t.Errorf("RemoveStrand calls = %v; want the one removal of strand-9", fake.RemoveStrandCalls)
	}
}

// TestRunner_StopStepRuns_StopsOnlyTheStepsRuns covers the step form over one run-directory root:
// every running run tied to the step is stopped, and only a live or dead own-window strand is removed;
// a run whose strand is a dead window 0 one has its record stopped with no removal;
// a run whose strand is untracked or has its pane binding cleared is stopped only when no live process holds its session;
// otherwise it is listed as held and left running;
// a run whose output files all exist is listed as finished and keeps reading running;
// an untied run with a removable strand is listed and left alone;
// a run of another step and a terminal run are left alone;
// and a failed stop is reported and skipped.
func TestRunner_StopStepRuns_StopsOnlyTheStepsRuns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tie := StepTie{StepsDir: "/steps", TraceID: "trace-a"}
	otherTie := StepTie{StepsDir: "/steps", TraceID: "trace-b"}
	strands := []reedengine.StrandStatus{
		{GUID: "guid-tied", PaneID: "%1", Live: true},
		{GUID: "guid-other", PaneID: "%2", Live: true},
		{GUID: "guid-untied", PaneID: "%3", Live: true},
		{GUID: "guid-terminal", PaneID: "%4", Live: true},
		{GUID: "guid-dead-own", PaneID: "%6", OwnWindow: true, Present: true},
		{GUID: "guid-dead-window0", PaneID: "%7", Present: true},
		{GUID: "guid-failing", PaneID: "%5", Live: true},
		{GUID: "guid-cleared"},
	}
	fake := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: strands}}}
	engine := &resumableFakeEngine{fakeEngine: &fakeEngine{}, heldSessions: map[string]int{"session-held": 4242}}
	fx := newFixture(t, fake, engine, withConfig(fastConfig), withSeparateRunDir())
	finishedFile := filepath.Join(t.TempDir(), "finished.md")
	touchOutputFile(t, finishedFile)
	seed := func(id string, record RunState) string {
		t.Helper()
		runDir := filepath.Join(fx.RunRoot, id)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatalf("mkdir run dir: %v", err)
		}
		record.RunID = id
		// A run with no output file reads finished, which a stop never turns into a stopped record.
		if record.OutputFiles == nil {
			record.OutputFiles = []string{filepath.Join(fx.RunRoot, "never-written.md")}
		}
		if err := saveRunState(runDir, record); err != nil {
			t.Fatalf("saveRunState: %v", err)
		}
		return runDir
	}
	dirs := map[string]string{
		"tied":         seed("run-1", RunState{StrandGUID: "guid-tied", StrandName: "name-tied", Outcome: runOutcomeRunning, Step: &tie}),
		"other":        seed("run-2", RunState{StrandGUID: "guid-other", StrandName: "name-other", Outcome: runOutcomeRunning, Step: &otherTie}),
		"untied":       seed("run-3", RunState{StrandGUID: "guid-untied", StrandName: "name-untied", Outcome: runOutcomeRunning}),
		"terminal":     seed("run-4", RunState{StrandGUID: "guid-terminal", StrandName: "name-terminal", Outcome: "died", Step: &tie}),
		"dead-own":     seed("run-5", RunState{StrandGUID: "guid-dead-own", StrandName: "name-dead-own", Outcome: runOutcomeRunning, Step: &tie}),
		"dead-window0": seed("run-6", RunState{StrandGUID: "guid-dead-window0", StrandName: "name-dead-window0", Outcome: runOutcomeRunning, Step: &tie}),
		"untracked":    seed("run-8", RunState{StrandGUID: "guid-untracked", StrandName: "name-untracked", SessionID: "session-free", Outcome: runOutcomeRunning, Step: &tie}),
		"cleared":      seed("run-9", RunState{StrandGUID: "guid-cleared", StrandName: "name-cleared", SessionID: "session-free", Outcome: runOutcomeRunning, Step: &tie}),
		"held":         seed("run-91", RunState{StrandGUID: "guid-held", StrandName: "name-held", SessionID: "session-held", Outcome: runOutcomeRunning, Step: &tie}),
		"finished":     seed("run-92", RunState{StrandGUID: "guid-finished", StrandName: "name-finished", SessionID: "session-free", OutputFiles: []string{finishedFile}, Outcome: runOutcomeRunning, Step: &tie}),
	}

	report, err := fx.Runner.StopStepRuns(tie)
	if err != nil {
		t.Fatalf("StopStepRuns() error = %v", err)
	}
	if !slices.Equal(report.Stopped, []string{"name-tied", "name-dead-own", "name-dead-window0", "name-untracked", "name-cleared"}) ||
		!slices.Equal(report.Held, []string{"name-held"}) || !slices.Equal(report.Finished, []string{"name-finished"}) ||
		!slices.Equal(report.Untied, []string{"name-untied"}) || len(report.Failed) != 0 {
		t.Errorf("report = %+v; want the unheld tied runs stopped, the held one and the finished one listed, and the untied one listed", report)
	}
	if len(fake.RemoveStrandCalls) != 2 || fake.RemoveStrandCalls[0].GUID != "guid-tied" || fake.RemoveStrandCalls[1].GUID != "guid-dead-own" {
		t.Errorf("RemoveStrand calls = %v; want the removals of guid-tied and guid-dead-own", fake.RemoveStrandCalls)
	}
	for name, wantOutcome := range map[string]string{"tied": runOutcomeStopped, "other": runOutcomeRunning, "untied": runOutcomeRunning, "terminal": "died", "dead-own": runOutcomeStopped, "dead-window0": runOutcomeStopped, "untracked": runOutcomeStopped, "cleared": runOutcomeStopped, "held": runOutcomeRunning, "finished": runOutcomeRunning} {
		record, _, err := loadRunState(dirs[name])
		if err != nil {
			t.Fatalf("loadRunState %s: %v", name, err)
		}
		if record.Outcome != wantOutcome {
			t.Errorf("%s run outcome = %q; want %q", name, record.Outcome, wantOutcome)
		}
	}

	seed("run-7", RunState{StrandGUID: "guid-failing", Outcome: runOutcomeRunning, Step: &tie})
	fake.RemoveStrandErr = errors.New("reed down")
	report, err = fx.Runner.StopStepRuns(tie)
	if err != nil {
		t.Fatalf("StopStepRuns() with a failing removal error = %v; want the failure in the report", err)
	}
	if len(report.Stopped) != 0 || len(report.Failed) != 1 {
		t.Errorf("report = %+v; want the one tied running run listed as failed, none stopped", report)
	}
	for _, failure := range report.Failed {
		if failure.Strand == "" || !strings.Contains(failure.Error, "reed down") {
			t.Errorf("failure = %+v; want a strand name and the removal error text", failure)
		}
	}

	// A failing holder lookup leaves a live holder unruled out, so an untracked run stays running and is listed as held.
	engine.holderErr = errors.New("registry entry unreadable")
	lookupFailedDir := seed("run-93", RunState{StrandGUID: "guid-lookup-failed", StrandName: "name-lookup-failed", SessionID: "session-free", Outcome: runOutcomeRunning, Step: &tie})
	report, err = fx.Runner.StopStepRuns(tie)
	if err != nil {
		t.Fatalf("StopStepRuns() with a failing holder lookup error = %v", err)
	}
	if !slices.Contains(report.Held, "name-lookup-failed") || slices.Contains(report.Stopped, "name-lookup-failed") {
		t.Errorf("report = %+v; want the run with a failing holder lookup listed as held and not stopped", report)
	}
	if record, _, err := loadRunState(lookupFailedDir); err != nil || record.Outcome != runOutcomeRunning {
		t.Errorf("lookup-failed run outcome = %q (err %v); want %q", record.Outcome, err, runOutcomeRunning)
	}
}

// TestStop_WaitThatSeesTheStoppedStrandGoEndsDied covers a Wait on a handle whose own Stop removed the strand:
// with the run's files missing, the strand's absence is the stop;
// the run ends died and records stopped, never the not-tracked mechanism failure;
// with the files present the finished work stays done.
func TestStop_WaitThatSeesTheStoppedStrandGoEndsDied(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		writeOutput bool
		want        Outcome
	}{
		{"OutputMissing", false, OutcomeDied},
		{"OutputPresent", true, OutcomeDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeReed{StatusQueue: []reedengine.StatusResult{
				{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}},
				{},
			}}
			fx := newFixture(t, fake, &fakeEngine{}, withConfig(fastConfig))
			runDir := t.TempDir()
			outputFile := filepath.Join(runDir, "out.md")
			clock := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, KeepPane: true, Timeout: time.Minute},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: filepath.Join(runDir, "events.jsonl"), Outcome: runOutcomeRunning}),
				withRunClock(clock, clock.Now().Add(time.Minute)))

			if err := run.Stop(); err != nil {
				t.Fatalf("Stop() error = %v; want nil", err)
			}
			if tt.writeOutput {
				if err := os.WriteFile(outputFile, []byte("done"), 0o600); err != nil {
					t.Fatalf("write output file: %v", err)
				}
			}
			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error = %v; want nil", err)
			}
			if result.Outcome != tt.want {
				t.Errorf("Wait() outcome = %q; want %q", result.Outcome, tt.want)
			}
			if len(fake.RemoveStrandCalls) != 1 {
				t.Errorf("RemoveStrand calls = %v; want only the stop's one removal", fake.RemoveStrandCalls)
			}
			if tt.want != OutcomeDied {
				return
			}
			record, _, err := loadRunState(runDir)
			if err != nil {
				t.Fatalf("loadRunState: %v", err)
			}
			if record.Outcome != runOutcomeStopped {
				t.Errorf("recorded outcome = %q; want %q", record.Outcome, runOutcomeStopped)
			}
		})
	}
}

// TestStop_MarkedHandleFinalizesStoppedUnlessDone covers a Wait that sees the strand go after Stop.
func TestStop_MarkedHandleFinalizesStoppedUnlessDone(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		classified  Outcome
		wantOutcome string
	}{
		{"died_is_recorded_stopped", OutcomeDied, runOutcomeStopped},
		{"timeout_is_recorded_stopped", OutcomeTimeout, runOutcomeStopped},
		{"done_through_the_file_contract_wins", OutcomeDone, string(OutcomeDone)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}}}
			fx := newFixture(t, fake, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
			outputFile := filepath.Join(fx.RunRoot, "out.md")
			runDir := seedAttachRun(t, fx.RunRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true,
			})
			record, _, err := loadRunState(runDir)
			if err != nil {
				t.Fatalf("loadRunState: %v", err)
			}
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, KeepPane: true}, withRunDir(runDir), withRunState(record))

			if err := run.Stop(); err != nil {
				t.Fatalf("Stop() error = %v; want nil", err)
			}
			result, err := run.finalize(tt.classified)
			if err != nil {
				t.Fatalf("finalize() error = %v; want nil", err)
			}

			if result.Outcome != tt.classified {
				t.Errorf("Result.Outcome = %q; want the classified %q", result.Outcome, tt.classified)
			}
			final, _, err := loadRunState(runDir)
			if err != nil {
				t.Fatalf("loadRunState: %v", err)
			}
			if final.Outcome != tt.wantOutcome {
				t.Errorf("recorded outcome = %q; want %q", final.Outcome, tt.wantOutcome)
			}
		})
	}
}
