package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
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
// The read row calls stopRecorded because FindRun skips an unreadable record, so only a record that turns unreadable after the scan reaches that refusal.
func TestStopStrand_UnrecordableStopRemovesNothing(t *testing.T) {
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
			if err := os.Chmod(runDir, 0o555); err != nil {
				t.Fatalf("make run dir read-only: %v", err)
			}
			t.Cleanup(func() { os.Chmod(runDir, 0o755) })
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

// TestStop_MarkedHandleFinalizesStoppedUnlessDone covers a Wait that sees the strand go after Stop.
func TestStop_MarkedHandleFinalizesStoppedUnlessDone(t *testing.T) {
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
