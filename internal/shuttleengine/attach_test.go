// attach_test.go covers Runner.Attach's scan, disposition, combine, and reconstruction.
// Each test runs over a temp run-dir root with hand-written run.json fixtures.
// The fakeReed, fakeEngine and fakeClock doubles replace the real collaborators.
// Reed's state file is seeded through the exported `reedengine.SaveState`,
// so the absent/present/unreadable three-way answer is exercised through the real decoder.
// Only the unreadable answer is written by hand, as a corrupt file.
// Every test here is hermetic: no tmux, no claude, no real sleeping.

package shuttleengine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedPresentReedState writes a minimal, valid ReedState via the real SaveState/decoder path.
// Loading reed's state then reports the state table as present.
// The strand table's content does not matter here: tracked and live answers come from `fakeReed.StatusQueue`.
func seedPresentReedState(t *testing.T, dotLymDir string) {
	t.Helper()
	if err := reedengine.SaveState(dotLymDir, &reedengine.ReedState{}); err != nil {
		t.Fatalf("reedengine.SaveState: %v", err)
	}
}

// seedUnreadableReedState writes a corrupt reed.json directly, bypassing SaveState,
// so the decode fails rather than reporting the state absent.
func seedUnreadableReedState(t *testing.T, dotLymDir string) {
	t.Helper()
	if err := os.MkdirAll(dotLymDir, 0o755); err != nil {
		t.Fatalf("mkdir dotLymDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dotLymDir, "reed.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write corrupt reed.json: %v", err)
	}
}

// seedAttachRunOpts describes one hand-written run.json fixture.
// includeOutcome false omits the "outcome" key from the JSON object entirely, as an older binary's record does.
// Setting it to the empty string instead would not exercise the decode path, only the comparison.
type seedAttachRunOpts struct {
	strandGUID     string
	sessionID      string
	outputFiles    []string
	outcome        string
	includeOutcome bool
	// started seeds RunState.Started.
	// Call sites leave it unset unless they pin started-seed behavior:
	// the zero value (false) is also what a run.json from an older binary decodes to.
	// The fixture default matches the production default.
	started bool
	// forkSubagents seeds RunState.ForkSubagents.
	// The key is written only when true, so a seed without it has the shape of an older record.
	forkSubagents bool
	// promptLine and promptOffset seed RunState.PromptLine and PromptOffset, written only when set.
	promptLine   string
	promptOffset int64
}

// seedAttachRun writes a run.json fixture at <root>/<id>/run.json from a plain map.
// includeOutcome can then omit the "outcome" key entirely.
// It returns the run directory.
func seedAttachRun(t *testing.T, root, id string, opts seedAttachRunOpts) string {
	t.Helper()
	runDir := filepath.Join(root, id)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	fields := map[string]any{
		"runId":        id,
		"strandGuid":   opts.strandGUID,
		"sessionId":    opts.sessionID,
		"interactive":  false,
		"outputFiles":  opts.outputFiles,
		"promptPath":   filepath.Join(runDir, promptFileName),
		"settingsPath": filepath.Join(runDir, settingsFileName),
		"eventsPath":   filepath.Join(runDir, eventsFileName),
		"createdAt":    time.Now().UTC().Format(time.RFC3339),
		"started":      opts.started,
	}
	if opts.includeOutcome {
		fields["outcome"] = opts.outcome
	}
	if opts.forkSubagents {
		fields["forkSubagents"] = true
	}
	if opts.promptLine != "" {
		fields["promptLine"] = opts.promptLine
	}
	if opts.promptOffset != 0 {
		fields["promptOffset"] = opts.promptOffset
	}
	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatalf("marshal run.json fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, runStateFileName), data, 0o644); err != nil {
		t.Fatalf("write run.json fixture: %v", err)
	}
	return runDir
}

// touchOutputFile creates an empty file at path, standing in for an agent-written output file.
func touchOutputFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("result"), 0o644); err != nil {
		t.Fatalf("touch output file %s: %v", path, err)
	}
}

// setDirAge sets runDir's mtime to at,
// so the age rule sees a directory of a known age against the test clock's Now().
func setDirAge(t *testing.T, runDir string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(runDir, at, at); err != nil {
		t.Fatalf("chtimes %s: %v", runDir, err)
	}
}

// liveStatus returns a StatusResult tracking exactly one strand, live with paneID.
func liveStatus(guid, paneID string) reedengine.StatusResult {
	return reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: guid, PaneID: paneID, Live: true}}}
}

// deadStatus returns a StatusResult tracking exactly one strand, not live, with paneID.
// An empty paneID stands for a cleared binding.
func deadStatus(guid, paneID string) reedengine.StatusResult {
	return reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: guid, PaneID: paneID, Live: false}}}
}

// TestAttach_NoCandidates covers the ordinary first-call case: nothing to attach to.
// The reed gate is never consulted, which proves that zero candidates take precedence over the gate.
func TestAttach_NoCandidates(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name          string
		rootDoesExist bool
	}{
		{"no_run_dirs_at_all", true},
		{"root_does_not_exist", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{}
			fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
			runner, runRoot := fx.Runner, fx.RunRoot
			if !tt.rootDoesExist {
				if err := os.RemoveAll(runRoot); err != nil {
					t.Fatalf("remove run root: %v", err)
				}
			}
			// Reed's state file is left absent: the scan must short-circuit before ever reading it,
			// so the ordinary first call on a fresh worktree is never blocked by the gate.
			result, found, err := runner.Attach(Spec{OutputFiles: []string{filepath.Join(runRoot, "out.md")}, Timeout: time.Minute})
			if err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if found {
				t.Errorf("found = true; want false")
			}
			if !isZeroResult(result) {
				t.Errorf("result = %+v; want zero Result", result)
			}
			if len(reed.CallLog) != 0 {
				t.Errorf("reed.CallLog = %v; want empty — the scan must short-circuit before any reed read", reed.CallLog)
			}
		})
	}
}

// TestAttach_OutcomeDisposition pins that only the "running" Outcome sentinel attaches.
// Every other Outcome value is respawn-eligible for a strand that is otherwise tracked and live:
// a terminal value, an empty or omitted one, and an unrecognized one.
//
//testtiming:keep pins which persisted Outcome values attach and which are respawn-eligible, with a row each for every terminal, omitted and unrecognized value
func TestAttach_OutcomeDisposition(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name           string
		outcome        string
		includeOutcome bool
		wantFound      bool
	}{
		{"terminal_done", "done", true, false},
		{"legacy_asking_attaches", "asking", true, true},
		{"terminal_died", "died", true, false},
		{"terminal_timeout", "timeout", true, false},
		{"terminal_stopped_is_respawn_eligible_on_a_live_strand", runOutcomeStopped, true, false},
		{"running_attaches", runOutcomeRunning, true, true},
		{"omitted_outcome_is_legacy_upgrade_path", "", false, false},
		{"unrecognized_outcome", "some-future-value", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
			runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
			seedPresentReedState(t, dotLymDir)

			outputFile := filepath.Join(runRoot, "out.md")
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", sessionID: "session-1",
				outputFiles: []string{outputFile}, outcome: tt.outcome, includeOutcome: tt.includeOutcome,
			})
			// Seeded for the one case (running_attaches) that reaches Wait.
			// It classifies OutcomeDone on its first tick instead of looping to the config deadline.
			// The seed is harmless for every other case, which never gets past dispositioning.
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
			if err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if found != tt.wantFound {
				t.Errorf("found = %v; want %v", found, tt.wantFound)
			}
			if !tt.wantFound && !isZeroResult(result) {
				t.Errorf("result = %+v; want zero Result when not found", result)
			}
			// The empty anchor defaults to own-window, yet attaching to a window 0 strand never moves or refuses it.
			if len(reed.AddStrandCalls) != 0 {
				t.Errorf("AddStrand calls = %v; want none, since attach never relaunches a strand", reed.AddStrandCalls)
			}
		})
	}
}

// TestAttach_RemovesSupersededStrands covers the removal on Attach's not-found answer:
// only the live or dead own-window strand of a respawn-eligible candidate of the same output-file set is removed;
// an attach or a failed removal never answers not found;
// and AttachIfLive and AttachIfLiveGated remove nothing.
func TestAttach_RemovesSupersededStrands(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	live := func(guid string) reedengine.StrandStatus {
		return reedengine.StrandStatus{GUID: guid, PaneID: "%" + guid, Live: true}
	}
	tests := []struct {
		name         string
		outcome      string
		strands      []reedengine.StrandStatus
		otherOutputs bool
		removeErr    error
		ifLiveOnly   bool
		gatedIfLive  bool
		wantRemoved  []string
		// outputMissing leaves the candidate's output file unwritten, so a stop is recorded over it.
		outputMissing bool
		wantRecorded  string
		wantFound     bool
		wantErrSubstr string
	}{
		{name: "terminal_candidate_on_a_live_strand_is_removed", outcome: "done", strands: []reedengine.StrandStatus{live("strand-1")}, wantRemoved: []string{"strand-1"}, wantRecorded: "done"},
		{name: "unrecognized_outcome_candidate_is_recorded_stopped_then_removed", outcome: "some-future-value", outputMissing: true, strands: []reedengine.StrandStatus{live("strand-1")}, wantRemoved: []string{"strand-1"}, wantRecorded: runOutcomeStopped},
		{name: "dead_pane_candidate_removes_nothing", outcome: "died", strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: false}}},
		{name: "dead_own_window_candidate_is_removed", outcome: "died", strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", OwnWindow: true, Present: true}}, wantRemoved: []string{"strand-1"}, wantRecorded: "died"},
		{name: "untracked_candidate_removes_nothing", outcome: "timeout"},
		{name: "other_output_set_strand_survives", outcome: "done", strands: []reedengine.StrandStatus{live("strand-1"), live("strand-other")}, otherOutputs: true, wantRemoved: []string{"strand-1"}},
		{name: "attachable_candidate_is_attached_not_removed", outcome: runOutcomeRunning, strands: []reedengine.StrandStatus{live("strand-1")}, wantFound: true},
		{name: "removal_failure_is_an_error_not_a_not_found", outcome: "done", strands: []reedengine.StrandStatus{live("strand-1")}, removeErr: errors.New("reed down"), wantRemoved: []string{"strand-1"}, wantErrSubstr: `could not remove the superseded strand strand-1`},
		{name: "attach_if_live_removes_nothing", outcome: "done", strands: []reedengine.StrandStatus{live("strand-1")}, ifLiveOnly: true},
		{name: "attach_if_live_gated_removes_nothing", outcome: "done", strands: []reedengine.StrandStatus{live("strand-1")}, gatedIfLive: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: tt.strands}}, RemoveStrandErr: tt.removeErr}
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
			runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
			seedPresentReedState(t, dotLymDir)

			outputFile := filepath.Join(runRoot, "out.md")
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", sessionID: "session-1",
				outputFiles: []string{outputFile}, outcome: tt.outcome, includeOutcome: true,
			})
			if tt.otherOutputs {
				seedAttachRun(t, runRoot, "run-other", seedAttachRunOpts{
					strandGUID: "strand-other", outputFiles: []string{filepath.Join(runRoot, "other.md")},
					outcome: "done", includeOutcome: true,
				})
			}
			// Seeded for the attachable row, which reaches Wait and must classify OutcomeDone on its first tick.
			if !tt.outputMissing {
				touchOutputFile(t, outputFile)
			}
			if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			// KeepPane so the attachable row's own harvest does not remove its strand;
			// otherwise that removal would read as a superseded-strand removal.
			spec := Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, KeepPane: true}
			attach := runner.Attach
			if tt.ifLiveOnly {
				attach = runner.AttachIfLive
			}
			if tt.gatedIfLive {
				attach = func(spec Spec) (Result, bool, error) { return runner.AttachIfLiveGated(spec, GateSpec{}) }
			}
			_, found, err := attach(spec)

			if tt.wantErrSubstr == "" && err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if tt.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) || !strings.Contains(err.Error(), `lym reed remove strand-1`) {
					t.Fatalf("Attach() error = %v; want it to contain %q and the way forward `lym reed remove strand-1`", err, tt.wantErrSubstr)
				}
			}
			if found != tt.wantFound {
				t.Errorf("found = %v; want %v", found, tt.wantFound)
			}
			var removed []string
			for _, call := range reed.RemoveStrandCalls {
				removed = append(removed, call.GUID)
				if call.Recursive {
					t.Errorf("RemoveStrand(%s) was recursive; want a non-cascading removal", call.GUID)
				}
			}
			if strings.Join(removed, ",") != strings.Join(tt.wantRemoved, ",") {
				t.Errorf("removed strands = %v; want %v", removed, tt.wantRemoved)
			}
			if tt.wantRecorded != "" {
				record, _, err := loadRunState(runDir)
				if err != nil {
					t.Fatalf("loadRunState: %v", err)
				}
				if record.Outcome != tt.wantRecorded {
					t.Errorf("superseded record outcome = %q; want %q", record.Outcome, tt.wantRecorded)
				}
			}
		})
	}
}

// TestAttach_Multiplicity covers candidate evaluation order and one-live-match-or-none:
// the multiplicity rule applies only to the surviving ATTACHABLE set;
// an error verdict dominates whatever the other candidates say;
// and two ordinary leftovers (both non-terminal-classified, tracked-live-idle records) must respawn rather than error.
func TestAttach_Multiplicity(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("TwoTimeoutLeftovers_RespawnsNotError", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{
			Strands: []reedengine.StrandStatus{
				{GUID: "strand-1", PaneID: "%1", Live: true},
				{GUID: "strand-2", PaneID: "%2", Live: true},
			},
		}}}
		fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
		runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
		seedPresentReedState(t, dotLymDir)

		outputFile := filepath.Join(runRoot, "out.md")
		seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: "timeout", includeOutcome: true})
		seedAttachRun(t, runRoot, "run-2", seedAttachRunOpts{strandGUID: "strand-2", outputFiles: []string{outputFile}, outcome: "timeout", includeOutcome: true})

		result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
		if err != nil {
			t.Fatalf("Attach() error = %v; want nil (two respawn-eligible leftovers, not an error)", err)
		}
		if found {
			t.Errorf("found = true; want false")
		}
		if !isZeroResult(result) {
			t.Errorf("result = %+v; want zero Result", result)
		}
	})

	t.Run("ErrorCandidateDominatesAttachableCandidate", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-2", "%2")}}
		fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
		runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
		seedPresentReedState(t, dotLymDir)

		outputFile := filepath.Join(runRoot, "out.md")
		// run-1: untracked (not in Status()'s strand list at all) and young — an error verdict.
		untrackedDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-untracked", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
		setDirAge(t, untrackedDir, time.Now())
		// run-2: tracked, live, running — attachable.
		seedAttachRun(t, runRoot, "run-2", seedAttachRunOpts{strandGUID: "strand-2", sessionID: "session-2", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})

		_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
		if err == nil {
			t.Fatalf("Attach() = (_, %v, nil); want an error — the error verdict must dominate the attachable one", found)
		}
		if found {
			t.Errorf("found = true; want false on an error return")
		}
	})

	t.Run("TwoAttachableCandidates_Errors", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{
			Strands: []reedengine.StrandStatus{
				{GUID: "strand-1", PaneID: "%1", Live: true},
				{GUID: "strand-2", PaneID: "%2", Live: true},
			},
		}}}
		fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
		runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
		seedPresentReedState(t, dotLymDir)

		outputFile := filepath.Join(runRoot, "out.md")
		dir1 := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
		dir2 := seedAttachRun(t, runRoot, "run-2", seedAttachRunOpts{strandGUID: "strand-2", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})

		_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
		if err == nil {
			t.Fatal("Attach() error = nil; want the multiplicity error")
		}
		if found {
			t.Errorf("found = true; want false")
		}
		if !strings.Contains(err.Error(), dir1) || !strings.Contains(err.Error(), dir2) {
			t.Errorf("Attach() error = %v; want it to name both run directories %q and %q", err, dir1, dir2)
		}
	})
}

// TestAttach_OutputFilesMismatch pins that a run dir whose OutputFiles do not match the spec's is never a candidate.
func TestAttach_OutputFilesMismatch(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{}
	fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
	runner, runRoot := fx.Runner, fx.RunRoot
	seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{filepath.Join(runRoot, "other.md")}, outcome: runOutcomeRunning, includeOutcome: true})

	result, found, err := runner.Attach(Spec{OutputFiles: []string{filepath.Join(runRoot, "out.md")}, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if found {
		t.Errorf("found = true; want false — output files do not match, so no candidate matched")
	}
	if !isZeroResult(result) {
		t.Errorf("result = %+v; want zero Result", result)
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed.CallLog = %v; want empty — no candidate matched, so the reed gate is never reached", reed.CallLog)
	}
}

// TestAttach_UntrackedTerminalRecord_RespawnEligibleRegardlessOfAge pins the following rule:
// a candidate whose persisted Outcome is terminal (done/asking/died/timeout) is respawn-eligible at any age.
// That holds whatever reed says of the candidate's pane.
// A tracked, live strand with a terminal Outcome is respawn-eligible at any age too.
// The test covers both routes into leftoverThenAgeVerdict:
// an untracked strand, and a tracked strand with a cleared pane binding.
//
//testtiming:keep pins that a terminal persisted Outcome is respawn-eligible at any directory age for both an untracked strand and a cleared pane binding, which the age-rule rows never reach
func TestAttach_UntrackedTerminalRecord_RespawnEligibleRegardlessOfAge(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("Untracked", func(t *testing.T) {
		for _, outcome := range []string{"done", "asking", "died", "timeout", "api-error", runOutcomeStopped} {
			t.Run(outcome, func(t *testing.T) {
				reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: nil}}}
				fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
				runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
				seedPresentReedState(t, dotLymDir)
				fc := newFakeClock(time.Now())
				runner.clock = fc

				outputFile := filepath.Join(runRoot, "out.md") // never written: not the leftover-files case
				runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: outcome, includeOutcome: true})
				setDirAge(t, runDir, fc.Now().Add(-2*time.Duration(30)*time.Second)) // younger than 2*StartupTimeoutS

				_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
				if err != nil {
					t.Fatalf("Attach() error = %v; want nil — a terminal-Outcome record is respawn-eligible regardless of age", err)
				}
				if found {
					t.Errorf("found = true; want false")
				}
			})
		}
	})

	t.Run("BindingCleared", func(t *testing.T) {
		for _, outcome := range []string{"done", "asking", "died", "timeout", "api-error", runOutcomeStopped} {
			t.Run(outcome, func(t *testing.T) {
				reed := &fakeReed{StatusQueue: []reedengine.StatusResult{deadStatus("strand-1", "")}}
				fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
				runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
				seedPresentReedState(t, dotLymDir)
				fc := newFakeClock(time.Now())
				runner.clock = fc

				outputFile := filepath.Join(runRoot, "out.md")
				runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: outcome, includeOutcome: true})
				setDirAge(t, runDir, fc.Now().Add(-2*time.Duration(30)*time.Second))

				_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, Display: render.Display{Anchor: render.AnchorOwnWindow}})
				if err != nil {
					t.Fatalf("Attach() error = %v; want nil — a terminal-Outcome record is respawn-eligible regardless of age", err)
				}
				if found {
					t.Errorf("found = true; want false")
				}
			})
		}
	})
}

// TestAttach_ReedStateGate_AbsentOrUnreadable pins that both an absent and an unreadable reed.json are errors:
// never found == false, at any directory age.
// For the absent case, Attach does not consult Status().
// The asymmetry between LoadState and ReedOps.Status() is what the test preserves.
func TestAttach_ReedStateGate_AbsentOrUnreadable(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name          string
		seed          func(t *testing.T, dotLymDir string)
		checkNoStatus bool
	}{
		{"absent", func(t *testing.T, dotLymDir string) {}, true},
		{"unreadable", seedUnreadableReedState, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
			fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
			runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
			tt.seed(t, dotLymDir)

			outputFile := filepath.Join(runRoot, "out.md")
			seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})

			_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
			if err == nil {
				t.Fatal("Attach() error = nil; want an error — an absent or unreadable strand table is not evidence any run is dead")
			}
			if found {
				t.Errorf("found = true; want false on error")
			}
			if tt.checkNoStatus {
				for _, call := range reed.CallLog {
					if call == "Status" {
						t.Errorf("reed.CallLog = %v; want no Status() call — the absent-state question must be answered by LoadState alone", reed.CallLog)
					}
				}
			}
		})
	}
}

// TestAttach_StatusError pins that an error from `ReedOps.Status()` is never reported as found == false.
// It holds at any directory age, for a torn-down session and for an unrelated tmux fault.
func TestAttach_StatusError(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		wantErr error
	}{
		{"torn_down_session", errors.New(`no reed session; run "lym reed up"`)},
		{"tmux_fault", errors.New("tmux: server not found")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusErr: tt.wantErr}
			fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
			runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
			seedPresentReedState(t, dotLymDir)

			outputFile := filepath.Join(runRoot, "out.md")
			seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})

			_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
			if err == nil {
				t.Fatal("Attach() error = nil; want the wrapped Status() error")
			}
			if found {
				t.Errorf("found = true; want false on error")
			}
		})
	}
}

// TestAttach_NegativeTimeout pins that a negative `Timeout` is rejected before scanning and before any reed read.
func TestAttach_NegativeTimeout(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{}
	fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
	runner, runRoot := fx.Runner, fx.RunRoot
	seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{filepath.Join(runRoot, "out.md")}, outcome: runOutcomeRunning, includeOutcome: true})

	_, found, err := runner.Attach(Spec{OutputFiles: []string{filepath.Join(runRoot, "out.md")}, Timeout: -time.Second})
	if err == nil {
		t.Fatal("Attach() error = nil; want the negative-Timeout rejection")
	}
	if !strings.Contains(err.Error(), "must not be negative") {
		t.Errorf("Attach() error = %v; want it to name the negative Timeout", err)
	}
	if found {
		t.Errorf("found = true; want false")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed.CallLog = %v; want empty — no reed read attempted", reed.CallLog)
	}
}

// TestAttach_RunningRecordSatisfiedFileContract_HarvestsNotRespawn pins that such a run is harvested.
// Its run.json still reads runOutcomeRunning, but every declared output file is already on disk.
// The run finished, but its driver crashed before any Wait could classify it.
// It is harvested as OutcomeDone, not archived and respawned.
// The rows cover each negative liveness answer (untracked, a dead pane, a cleared pane binding).
// They hold at any directory age.
//
// The run's own Wait checks the output files first for an untracked or not-live strand,
// so every shape reaches OutcomeDone and the same run-directory cleanup.
//
// Attach's three reed-state gates consult the file contract before refusing:
// an unreadable reed.json, an absent one, and a failing Status().
// They run ahead of `dispositionCandidate`, so each gate needs the check itself.
// Without it, a finished run would fail the whole Shed step.
//
// Each row drives the reconstructed run to its own Outcome.
// Harvesting is only correct if Wait can classify OutcomeDone while reed stays broken.
func TestAttach_RunningRecordSatisfiedFileContract_HarvestsNotRespawn(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name   string
		status reedengine.StatusResult
		age    time.Duration
		// seedState seeds reed.json; nil seeds a present, valid one.
		seedState func(t *testing.T, dotLymDir string)
		statusErr error
	}{
		{name: "untracked_strand_young_dir", status: reedengine.StatusResult{Strands: nil}, age: 10 * time.Second},
		{name: "untracked_strand_old_dir", status: reedengine.StatusResult{Strands: nil}, age: 2 * time.Minute},
		{name: "dead_pane_young_dir", status: deadStatus("strand-1", "%1"), age: 10 * time.Second},
		{name: "binding_cleared_old_dir", status: deadStatus("strand-1", ""), age: 2 * time.Minute},
		{name: "unreadable_reed_json", seedState: seedUnreadableReedState},
		{name: "absent_reed_json", seedState: func(t *testing.T, _ string) { t.Helper() }},
		{name: "reed_status_errors", statusErr: errors.New("tmux: no server running")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusErr: tt.statusErr}
			if tt.statusErr == nil && tt.seedState == nil {
				reed.StatusQueue = []reedengine.StatusResult{tt.status}
			}
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
			runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
			if tt.seedState != nil {
				tt.seedState(t, dotLymDir)
			} else {
				seedPresentReedState(t, dotLymDir)
			}
			fc := newFakeClock(time.Now())
			runner.clock = fc

			outputFile := filepath.Join(runRoot, "out.md")
			touchOutputFile(t, outputFile)
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
			setDirAge(t, runDir, fc.Now().Add(-tt.age))

			result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
			if err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if !found {
				t.Fatal("found = false; want true — a running record whose file contract is satisfied is a finished run to harvest, not a leftover to respawn over")
			}
			if result.Outcome != OutcomeDone {
				t.Errorf("Outcome = %q; want %q — the reconstructed run's Wait harvests the satisfied file contract", result.Outcome, OutcomeDone)
			}
			if _, err := os.Stat(runDir); !os.IsNotExist(err) {
				t.Errorf("run dir still exists after Done cleanup, stat err = %v", err)
			}
		})
	}
}

// TestAttach_RunningRecordUnsatisfiedFileContract_RespawnsOrErrors pins a running record with missing outputs.
// Such a record is never harvested, and it respawns or errors instead.
// A partly written or never-started run has no finished work to attribute,
// so the ordinary liveness dispositioning applies.
// The harvest is gated on the file contract, not on the presence of run.json.
//
// The rows also pin the leftover-then-age rule (`leftoverThenAgeVerdict`).
// A tracked strand with a dead pane is unambiguous evidence that the agent is gone,
// so it respawns at any directory age.
// An untracked strand (`errStrandNotTracked`) returns an error while the directory is young.
// A young directory may hold a run that is still starting.
// A binding-cleared strand (`errStrandPaneBindingCleared`) does too.
// It is tracked and not live, with an empty PaneID and a spec Anchor that is not hidden.
// Both respawn once the directory is at least `minAge` old, which is twice `StartupTimeoutS`.
// An empty legacy Outcome gets the same age rule as `runOutcomeRunning`.
// It is neither a terminal Outcome nor a satisfied file contract, so a young directory cannot be ruled dead.
func TestAttach_RunningRecordUnsatisfiedFileContract_RespawnsOrErrors(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name   string
		status reedengine.StatusResult
		age    time.Duration
		// omitOutcome writes run.json without an "outcome" key, the shape of a legacy record.
		omitOutcome bool
		display     render.Display
		wantError   bool
	}{
		{name: "dead_pane_respawns", status: deadStatus("strand-1", "%1"), age: 10 * time.Second},
		{name: "dead_pane_old_dir_respawns", status: deadStatus("strand-1", "%1"), age: time.Hour},
		{name: "untracked_old_dir_respawns", status: reedengine.StatusResult{Strands: nil}, age: 2 * time.Minute},
		{name: "untracked_young_dir_errors", status: reedengine.StatusResult{Strands: nil}, age: 10 * time.Second, wantError: true},
		{name: "untracked_empty_outcome_young_dir_errors", status: reedengine.StatusResult{Strands: nil}, age: 10 * time.Second, omitOutcome: true, wantError: true},
		{
			name: "binding_cleared_young_dir_errors", status: deadStatus("strand-1", ""), age: 10 * time.Second,
			display: render.Display{Anchor: render.AnchorOwnWindow}, wantError: true,
		},
		{
			name: "binding_cleared_old_dir_respawns", status: deadStatus("strand-1", ""), age: 2 * time.Minute,
			display: render.Display{Anchor: render.AnchorOwnWindow},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{tt.status}}
			fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
			runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
			seedPresentReedState(t, dotLymDir)
			fc := newFakeClock(time.Now())
			runner.clock = fc

			// The output file is not created, so the file contract is unsatisfied.
			outputFile := filepath.Join(runRoot, "out.md")
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: !tt.omitOutcome})
			setDirAge(t, runDir, fc.Now().Add(-tt.age))

			result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, Display: tt.display})
			if tt.wantError {
				if err == nil {
					t.Fatal("Attach() error = nil; want an error — a young candidate with no finished output cannot be ruled dead")
				}
				if found {
					t.Errorf("found = true; want false on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if found {
				t.Errorf("found = true; want false — an unsatisfied file contract is not a finished run to harvest")
			}
			if !isZeroResult(result) {
				t.Errorf("result = %+v; want zero Result", result)
			}
		})
	}
}

// TestAttach_KeepPane pins that KeepPane on the caller's spec is honoured by the attached run.
// When set, it suppresses the Done cleanup; when absent, cleanup runs exactly as it does for a started run.
//
// Both rows seed a tracked-and-live candidate whose output files exist and whose events file holds a Done.
// They also pin that liveness is answered first: the candidate is attached, never treated as a leftover.
// They also pin that the reconstructed run's Wait reads the persisted events path and classifies OutcomeDone.
//
//testtiming:keep pins that Spec.KeepPane suppresses an attached run's done cleanup (run dir kept, no RemoveStrand) and that its absence performs both
func TestAttach_KeepPane(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name     string
		keepPane bool
	}{
		{"keep_pane_suppresses_cleanup", true},
		{"no_keep_pane_performs_cleanup", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
			engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
			fx := newFixture(t, reed, engine, withConfig(fastConfig), withSeparateRunDir())
			runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
			seedPresentReedState(t, dotLymDir)

			outputFile := filepath.Join(runRoot, "out.md")
			touchOutputFile(t, outputFile)
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
			if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, KeepPane: tt.keepPane})
			if err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if !found || result.Outcome != OutcomeDone {
				t.Fatalf("found=%v result=%+v; want found=true, Outcome=%q", found, result, OutcomeDone)
			}
			_, statErr := os.Stat(runDir)
			if tt.keepPane {
				if statErr != nil {
					t.Errorf("run dir removed despite KeepPane: %v", statErr)
				}
				if len(reed.RemoveStrandCalls) != 0 {
					t.Errorf("RemoveStrand calls = %+v; want none (KeepPane)", reed.RemoveStrandCalls)
				}
			} else {
				if !os.IsNotExist(statErr) {
					t.Errorf("run dir still exists after Done cleanup, stat err = %v", statErr)
				}
				if len(reed.RemoveStrandCalls) == 0 {
					t.Errorf("RemoveStrand calls = %+v; want the strand removed", reed.RemoveStrandCalls)
				}
			}
		})
	}
}

// TestAttach_UnreadableRunJSONMidScan_DoesNotAbortScan pins that a corrupt run.json must not abort the scan.
// The valid, matching run.json beside it must still be found.
func TestAttach_UnreadableRunJSONMidScan_DoesNotAbortScan(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
	runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
	seedPresentReedState(t, dotLymDir)

	outputFile := filepath.Join(runRoot, "out.md")
	touchOutputFile(t, outputFile)
	goodDir := seedAttachRun(t, runRoot, "run-good", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
	if err := os.WriteFile(filepath.Join(goodDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	corruptDir := filepath.Join(runRoot, "run-corrupt")
	if err := os.MkdirAll(corruptDir, 0o755); err != nil {
		t.Fatalf("mkdir corrupt dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(corruptDir, runStateFileName), []byte("{not valid"), 0o644); err != nil {
		t.Fatalf("write corrupt run.json: %v", err)
	}

	_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil — the corrupt sibling must be skipped, not fatal", err)
	}
	if !found {
		t.Errorf("found = false; want true — the valid candidate must still be found")
	}
}

// TestAttach_DeadlineAtAttachTime pins the deadline of an attached run.
// The deadline restarts at attach time (now + spec.Timeout, never CreatedAt + Timeout).
// The test attaches to a run directory aged far past spec.Timeout and asserts it does not immediately time out.
// A spec with a zero Timeout attaches with cfg.RunTimeoutMin applied, normalized the way the spec it matches on is.
//
// If Attach got either wrong, tick 1 would classify OutcomeTimeout immediately, before any call to Sleep.
// So the terminating event is planted only inside the first Sleep call.
// Only a correct (fresh, non-zero) deadline ever reaches that call.
func TestAttach_DeadlineAtAttachTime(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		dirAge  time.Duration
		timeout time.Duration
	}{
		{name: "restarted at attach time, not derived from CreatedAt", dirAge: 10 * time.Minute, timeout: time.Minute},
		{name: "zero timeout defaults from config", timeout: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
			engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
			fx := newFixture(t, reed, engine, withConfig(fastConfig), withSeparateRunDir())
			runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
			seedPresentReedState(t, dotLymDir)

			outputFile := filepath.Join(runRoot, "out.md")
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
			eventsPath := filepath.Join(runDir, eventsFileName)

			fc := newFakeClock(time.Now())
			// CreatedAt in the fixture is "now" (seedAttachRun's own timestamp), well within spec.Timeout.
			// To prove the deadline is not derived from CreatedAt, the run directory itself is aged far past it.
			if tt.dirAge > 0 {
				setDirAge(t, runDir, fc.Now().Add(-tt.dirAge))
			}
			mc := &multiStepClock{fakeClock: fc, steps: []func(){
				func() {
					touchOutputFile(t, outputFile)
					if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
						t.Fatalf("append done event: %v", err)
					}
				},
			}}
			runner.clock = mc

			result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: tt.timeout})
			if err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if !found {
				t.Fatal("found = false; want true")
			}
			if result.Outcome == OutcomeTimeout {
				t.Errorf("Outcome = %q; want anything but an immediate timeout — the deadline must restart at attach time and a zero Timeout must default to cfg.RunTimeoutMin", result.Outcome)
			}
		})
	}
}

// TestProbeGated_ReturnsHandleWithoutWaiting pins that ProbeGated answers a live candidate before any wait has run.
// It answers with the reconstructed handle, and the handle's own Wait then classifies the run.
// A probe over nothing answers not found.
func TestProbeGated_ReturnsHandleWithoutWaiting(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("live_candidate", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
		fx := newFixture(t, reed, &fakeEngine{StartupScript: []StartupState{StartupReady}}, withConfig(fastConfig), withSeparateRunDir())
		runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
		seedPresentReedState(t, dotLymDir)
		runner.clock = newFakeClock(time.Now())

		outputFile := filepath.Join(runRoot, "out.md")
		runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})

		run, found, err := runner.ProbeGated(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute}, GateSpec{})
		if err != nil {
			t.Fatalf("ProbeGated() error = %v; want nil", err)
		}
		if !found || run == nil {
			t.Fatalf("found = %v, run = %v; want a handle", found, run)
		}
		if run.StrandGUID() != "strand-1" || run.RunDir() != runDir {
			t.Errorf("handle names strand %q in %q; want strand-1 in %q", run.StrandGUID(), run.RunDir(), runDir)
		}
		if _, err := os.Stat(runDir); err != nil {
			t.Errorf("run dir gone after the probe, stat err = %v; the probe must not have waited", err)
		}

		touchOutputFile(t, outputFile)
		if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
			t.Fatalf("append done event: %v", err)
		}
		result, err := run.Wait()
		if err != nil {
			t.Fatalf("Wait() error = %v; want nil", err)
		}
		if result.Outcome != OutcomeDone {
			t.Errorf("Outcome = %q; want %q", result.Outcome, OutcomeDone)
		}
	})

	t.Run("nothing_to_attach_to", func(t *testing.T) {
		fx := newFixture(t, &fakeReed{}, &fakeEngine{}, withSeparateRunDir())
		run, found, err := fx.Runner.ProbeGated(Spec{OutputFiles: []string{filepath.Join(fx.RunRoot, "out.md")}, Timeout: time.Minute}, GateSpec{})
		if err != nil || found || run != nil {
			t.Errorf("ProbeGated() = (%v, %v, %v); want (nil, false, nil)", run, found, err)
		}
	})
}

// TestAttach_OutputFileMatching_ResolvedAbsoluteSet pins that matching is on the resolved absolute set.
// A spec with relative entries matches a run.json written with absolute ones.
// A spec naming the same files in a different order still matches.
func TestAttach_OutputFileMatching_ResolvedAbsoluteSet(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
	runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
	seedPresentReedState(t, dotLymDir)

	// NewRunner's worktreeRoot is the parent of anchorPath.
	// Resolve the absolute files a caller gets by joining a relative OutputFiles entry against worktreeRoot.
	worktreeRoot := runner.worktreeRoot
	absA := filepath.Join(worktreeRoot, "out-a.md")
	absB := filepath.Join(worktreeRoot, "out-b.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{absB, absA}, outcome: runOutcomeRunning, includeOutcome: true})
	if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	touchOutputFile(t, absA)
	touchOutputFile(t, absB)

	relSpec := Spec{OutputFiles: []string{"out-a.md", "out-b.md"}, Timeout: time.Minute}
	_, found, err := runner.Attach(relSpec)
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if !found {
		t.Errorf("found = false; want true — relative entries must resolve and match a differently-ordered absolute record")
	}
}

// TestAttach_OffsetStartsAtZero pins that attach replays a pre-existing events.jsonl from offset zero.
// A backlog whose last event is a completion classifies OutcomeDone on the first tick after attach.
// That is the missed-terminal-Stop case.
// The same backlog with output files absent is a held turn end that keeps the run polling to its deadline.
//
//testtiming:keep pins that the whole pre-existing events backlog is replayed on attach and its last event wins, for a completion and for a held Stop
func TestAttach_OffsetStartsAtZero(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("BacklogEndsInCompletion_ClassifiesDone", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
		fx := newFixture(t, reed, engine, withConfig(fastConfig), withSeparateRunDir())
		runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
		seedPresentReedState(t, dotLymDir)

		outputFile := filepath.Join(runRoot, "out.md")
		touchOutputFile(t, outputFile)
		runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
		if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("ASK:earlier question\nSTOP:done\n"), 0o644); err != nil {
			t.Fatalf("seed events: %v", err)
		}

		result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
		if err != nil {
			t.Fatalf("Attach() error = %v; want nil", err)
		}
		if !found || result.Outcome != OutcomeDone {
			t.Fatalf("found=%v Outcome=%q; want found=true, Outcome=%q — the whole backlog is replayed and its last event wins", found, result.Outcome, OutcomeDone)
		}
	})

	t.Run("BacklogEndsInStop_IsHeldToTheDeadline", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
		fx := newFixture(t, reed, engine, withConfig(fastConfig), withSeparateRunDir(), withClock(newFakeClock(time.Now())))
		runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
		seedPresentReedState(t, dotLymDir)

		outputFile := filepath.Join(runRoot, "out.md") // never created
		runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
		if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:need operator input\n"), 0o644); err != nil {
			t.Fatalf("seed events: %v", err)
		}

		buf := logcapture.CaptureVerbose(t)
		result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: heldTestTimeout})
		if err != nil {
			t.Fatalf("Attach() error = %v; want nil", err)
		}
		if !found || result.Outcome != OutcomeTimeout {
			t.Fatalf("found=%v Outcome=%q; want found=true, Outcome=%q — the replayed Stop is held, so the run ends only at its deadline", found, result.Outcome, OutcomeTimeout)
		}
		if !strings.Contains(buf.String(), "need operator input") {
			t.Errorf("hold log = %q, want the replayed Stop's message", buf.String())
		}
	})
}

// TestAttach_DeliversTheDeferredPrompt pins the deferred prompt line of a reconstructed run.
// When its output files are missing and no turn of the prompt started, the line is typed once at Wait entry.
// When every output file exists, nothing is typed.
func TestAttach_DeliversTheDeferredPrompt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const promptLine = "Read the task prompt and carry it out."
	tests := []struct {
		name         string
		events       string
		promptOffset int64
		outputExists bool
		// A covered row runs Attach under a hub entry that a later clock sleep resumes.
		// sendFails makes every send into the pane fail.
		covered   bool
		sendFails bool
		wantTyped []string
	}{
		{name: "a zero prompt offset with output files missing types the prompt", events: "SESSIONSTART:startup\n", wantTyped: []string{promptLine}},
		{name: "a set prompt offset with no prompt turn start types the prompt", events: skillLoadEvents, promptOffset: skillLoadOffset, wantTyped: []string{promptLine}},
		{name: "output files that all exist type nothing", events: skillLoadEvents, promptOffset: skillLoadOffset, outputExists: true},
		{name: "a covering pause holds the prompt until the pause lifts", events: skillLoadEvents, promptOffset: skillLoadOffset, covered: true, wantTyped: []string{promptLine}},
		{name: "a failed send is logged and Attach still returns the run", events: skillLoadEvents, promptOffset: skillLoadOffset, sendFails: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reed := &echoReed{fakeReed: &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}, pane: "❯ "}
			engine := &sessionFakeEngine{waitingEngine: waitingEngine{fakeEngine: fakeEngine{StartupScript: []StartupState{StartupReady}}}, liveness: LivenessAlive}
			if tt.sendFails {
				reed.SendTextErr = errors.New("pane gone")
			}
			fc := newFakeClock(time.Now())
			geometry, resume := coverHub(t, fc.Now())
			var clock Clock = fc
			if tt.covered {
				clock = &liftClock{Clock: fc, liftAfter: 3, lift: func() { resume(fc.Now()) }}
			}
			fx := newFixture(t, reed, engine, withConfig(fastConfig), withSeparateRunDir(), withClock(clock))
			if tt.covered {
				fx.Runner.SetPause(geometry)
			}
			seedPresentReedState(t, fx.DotLym)
			outputFile := filepath.Join(fx.RunRoot, "out.md")
			if tt.outputExists {
				touchOutputFile(t, outputFile)
			}
			runDir := seedAttachRun(t, fx.RunRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true,
				promptLine: promptLine, promptOffset: tt.promptOffset,
			})
			if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte(tt.events), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			_, found, err := fx.Runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: heldTestTimeout})
			if err != nil || !found {
				t.Fatalf("Attach() = (_, %v, %v); want found and no error", found, err)
			}

			if !slices.Equal(reed.typed, tt.wantTyped) {
				t.Errorf("Attach() typed %q; want %q", reed.typed, tt.wantTyped)
			}
			if tt.sendFails && len(reed.SendTextCalls) == 0 {
				t.Errorf("SendText calls = none; want the failed send attempted")
			}
		})
	}
}

// TestAttach_StartedSeededTrue pins the started seed.
// Once the persisted Started is true, the startup probe must never run, whatever else is true of the run.
// Attaching to a strand whose run.json carries Started: true must not classify OutcomeDied after startup_timeout_s.
// That holds when CapturePane returns a mid-turn capture with no ready markers.
// The attach must also not play the trust-dismiss key sequence, even when the capture holds a trust-dialog phrase.
// Without the seeded started: true, this run.json would decode with Started at its zero value (false),
// and the mid-turn capture would correctly re-trigger the startup probe.
// So the test proves the seed, not just the field's existence.
//
//testtiming:keep pins that a persisted Started: true skips the startup probe on attach, with no engine Startup call and no trust-dismiss Enter played into a live pane
func TestAttach_StartedSeededTrue(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{
		StatusQueue:  []reedengine.StatusResult{liveStatus("strand-1", "%1")},
		CaptureQueue: []string{"please trust this folder — mid turn capture"},
	}
	// StartupScript is left empty: fakeEngine.Startup would return StartupPending for every call,
	// and any call at all is the regression this test exists to catch.
	engine := &fakeEngine{}
	fx := newFixture(t, reed, engine, withConfig(Config{StartupTimeoutS: 1, RunTimeoutMin: 5, PollIntervalMS: 1000, LivenessEveryNPolls: 1}), withSeparateRunDir())
	runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
	seedPresentReedState(t, dotLymDir)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true, started: true})
	eventsPath := filepath.Join(runDir, eventsFileName)

	fc := newFakeClock(time.Now())
	// Nothing ends this run on tick 1: there are no events yet, and started=true skips the startup probe entirely.
	// That skip includes the probe's own output-files check.
	// The terminating event is planted from inside the first Sleep call only to keep the test finite.
	// A regression that re-probes startup would show on tick 1, through StartupCalls and SendKeyCalls.
	mc := &multiStepClock{fakeClock: fc, steps: []func(){
		func() {
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("append done event: %v", err)
			}
		},
	}}
	runner.clock = mc

	// KeepPane leaves the finished run's record on disk, so the pid the attach wrote can be read back.
	result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: 10 * time.Minute, KeepPane: true})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if !found {
		t.Fatal("found = false; want true")
	}
	if result.Outcome == OutcomeDied {
		t.Errorf("Outcome = %q; want anything but died — the started seed must skip the startup probe entirely", result.Outcome)
	}
	if rs, recordFound, err := loadRunState(runDir); err != nil || !recordFound || rs.PID != os.Getpid() {
		t.Errorf("loadRunState after Attach = (pid %d, found %v, %v); want the attaching process's pid %d", rs.PID, recordFound, err, os.Getpid())
	}
	if len(engine.StartupCalls) != 0 {
		t.Errorf("engine.StartupCalls = %v; want none — an attached run must never re-run the startup classifier", engine.StartupCalls)
	}
	for _, k := range reed.SendKeyCalls {
		if k.Key == "Enter" {
			t.Errorf("reed.SendKeyCalls = %+v; want no trust-dismiss Enter played into a live agent's pane", reed.SendKeyCalls)
		}
	}
}

// TestAttach_StrandLaterLosesPaneBinding pins that an attached run keeps full liveness coverage.
// Attach's own dispositioning read sees a live pane, so the candidate is attachable.
// Wait's first liveness tick then sees the binding cleared.
// With output files present, a hidden-anchor spec still classifies OutcomeDone.
// That shows the inherited checkLivenessTick branches are reached.
// The output file is never created and the Anchor is empty.
// Attach then defaults the Anchor to AnchorOwnWindow on the reconstructed Run.
// Wait then surfaces errStrandPaneBindingCleared.
// The empty Anchor must not take the hidden-strand carve-out and classify OutcomeDied.
//
//testtiming:keep pins that an attached run keeps liveness coverage after a cleared binding: done with files present, and errStrandPaneBindingCleared once an empty Anchor is defaulted to own-window
func TestAttach_StrandLaterLosesPaneBinding(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name         string
		createOutput bool
		display      render.Display
		wantErr      error
	}{
		{name: "files present classify done despite the pane going not-live", createOutput: true, display: render.Display{Anchor: render.AnchorHidden}},
		{name: "empty anchor defaults so the cleared binding is an error", wantErr: errStrandPaneBindingCleared},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{
				liveStatus("strand-1", "%1"), // Attach's own dispositioning read: attachable.
				deadStatus("strand-1", ""),   // Wait's first liveness tick: pane gone.
			}}
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
			runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
			seedPresentReedState(t, dotLymDir)

			outputFile := filepath.Join(runRoot, "out.md")
			if tt.createOutput {
				touchOutputFile(t, outputFile)
			}
			seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})

			result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, Display: tt.display})
			if !found {
				t.Fatal("found = false; want true — Attach's own dispositioning read saw a live pane")
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Attach() error = %v; want one wrapping %v, not the hidden-strand carve-out's OutcomeDied", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if result.Outcome != OutcomeDone {
				t.Errorf("Outcome = %q; want %q — output files satisfied the file contract despite the pane going not-live", result.Outcome, OutcomeDone)
			}
		})
	}
}

// TestAttach_ReedStateUnavailable_StillRefusesWithoutAFinishedRun pins the refusal when reed state is unavailable.
// The harvest is gated on a satisfied file contract and a record still declaring itself running.
// Each reed gate still refuses when either condition is missing.
//
// The unsatisfied-contract row guards against the crash-versus-bounce trap.
// The terminal-record row keeps a record that already ended from being re-harvested.
// The two-running-records row covers soleFinishedCandidate's exactly-one rule.
// Reed state is absent here, so it cannot pick between the two running records.
// The attach falls through to the gate's refusal instead of silently choosing one.
func TestAttach_ReedStateUnavailable_StillRefusesWithoutAFinishedRun(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name            string
		createOutput    bool
		outcome         string
		secondRunning   bool
		wantErrFragment string
	}{
		{"unsatisfied_file_contract", false, runOutcomeRunning, false, "no reed state file"},
		{"already_terminal_record", true, string(OutcomeDone), false, "no reed state file"},
		{"two_running_records", true, runOutcomeRunning, true, "no reed state file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{}
			fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
			runner, runRoot := fx.Runner, fx.RunRoot
			// reed.json is not seeded: the absent-state-file gate.

			outputFile := filepath.Join(runRoot, "out.md")
			if tt.createOutput {
				touchOutputFile(t, outputFile)
			}
			seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: tt.outcome, includeOutcome: true})
			if tt.secondRunning {
				seedAttachRun(t, runRoot, "run-2", seedAttachRunOpts{strandGUID: "strand-2", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
			}

			_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
			if err == nil {
				t.Fatal("Attach() error = nil; want the reed-gate refusal — nothing here is a finished run to harvest")
			}
			if !strings.Contains(err.Error(), tt.wantErrFragment) {
				t.Errorf("Attach() error = %q; want it to contain %q", err.Error(), tt.wantErrFragment)
			}
			if found {
				t.Error("found = true; want false — a refusal never reports an attached run")
			}
		})
	}
}
