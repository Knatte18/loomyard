// attach_test.go covers Runner.Attach's scan, disposition, combine, and reconstruction, driven over
// a temp run-dir root with hand-written run.json fixtures and the existing fakeReed/fakeEngine/
// fakeClock doubles. Reed's own state file is seeded through the exported reedengine.SaveState so
// the absent/present/unreadable three-way answer is exercised through the real decoder, never by
// hand-writing JSON for that file. Every test here is hermetic: no tmux, no claude, no real
// sleeping.

package shuttleengine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// seedPresentReedState writes a minimal, valid ReedState via the real SaveState/decoder path,
// answering LoadState's "does reed have a state table at all" gate with "present". The strand
// table's actual content is irrelevant to that gate — tracked/live dispositioning is answered
// separately, by fakeReed.StatusQueue.
func seedPresentReedState(t *testing.T, dotLyxDir string) {
	t.Helper()
	if err := reedengine.SaveState(dotLyxDir, &reedengine.ReedState{}); err != nil {
		t.Fatalf("reedengine.SaveState: %v", err)
	}
}

// seedUnreadableReedState writes a corrupt reed.json directly, bypassing SaveState, so LoadState's
// decode fails rather than reporting absent.
func seedUnreadableReedState(t *testing.T, dotLyxDir string) {
	t.Helper()
	if err := os.MkdirAll(dotLyxDir, 0o755); err != nil {
		t.Fatalf("mkdir dotLyxDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dotLyxDir, "reed.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write corrupt reed.json: %v", err)
	}
}

// seedAttachRunOpts describes one hand-written run.json fixture. includeOutcome false writes the
// JSON object with the "outcome" key omitted entirely — a legacy pre-Outcome-field record — rather
// than one that sets it to the empty string, so the decode path itself is exercised, not just the
// comparison.
type seedAttachRunOpts struct {
	strandGUID     string
	sessionID      string
	outputFiles    []string
	outcome        string
	includeOutcome bool
	// started seeds RunState.Started. Omitted from every existing call site but the ones that
	// specifically pin started-seed behavior, since Go's zero value (false) is what a run.json a
	// pre-Started-field binary wrote also decodes to — the fixture default matches the production
	// default.
	started bool
}

// seedAttachRun writes a run.json fixture at <root>/<id>/run.json built from a plain map rather than
// RunState, so includeOutcome can omit the "outcome" key entirely. Returns the run directory.
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

// setDirAge sets runDir's mtime to at, so Attach's age rule sees a directory of a known age against
// a fakeClock's Now().
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

// deadStatus returns a StatusResult tracking exactly one strand, not live, with paneID (empty for a
// cleared binding).
func deadStatus(guid, paneID string) reedengine.StatusResult {
	return reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: guid, PaneID: paneID, Live: false}}}
}

// TestAttach_NoCandidates covers the ordinary first-call case: nothing to attach to, and the reed
// gate is never consulted — proving the zero-candidates precedence.
func TestAttach_NoCandidates(t *testing.T) {
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
			// Reed's state file is deliberately left absent: the scan must short-circuit before ever
			// reading it, so the ordinary first call on a fresh worktree is never blocked by the gate.
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

// TestAttach_OutcomeDisposition is attach-only-a-run-that-never-terminated's coverage: exactly the
// "running" sentinel attaches, and every other Outcome value — terminal, empty/omitted, or
// unrecognized — is respawn-eligible, for a strand that is otherwise tracked and live.
//
//testtiming:keep pins which persisted Outcome values attach and which are respawn-eligible, with a row each for every terminal, omitted and unrecognized value
func TestAttach_OutcomeDisposition(t *testing.T) {
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
		{"running_attaches", runOutcomeRunning, true, true},
		{"omitted_outcome_is_legacy_upgrade_path", "", false, false},
		{"unrecognized_outcome", "some-future-value", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
			seedPresentReedState(t, dotLyxDir)

			outputFile := filepath.Join(runRoot, "out.md")
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", sessionID: "session-1",
				outputFiles: []string{outputFile}, outcome: tt.outcome, includeOutcome: tt.includeOutcome,
			})
			// Seeded for the one case (running_attaches) that reaches Wait, so it classifies OutcomeDone
			// on its very first tick instead of looping to the config deadline; harmless for every
			// other case, which never gets past dispositioning.
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
		})
	}
}

// TestAttach_RemovesSupersededStrands covers the removal on Attach's not-found answer:
// only the live strand of a respawn-eligible candidate of the same output-file set is removed,
// an attach or a failed removal never answers not found,
// and AttachIfLive removes nothing.
func TestAttach_RemovesSupersededStrands(t *testing.T) {
	t.Parallel()

	live := func(guid string) reedengine.StrandStatus {
		return reedengine.StrandStatus{GUID: guid, PaneID: "%" + guid, Live: true}
	}
	tests := []struct {
		name          string
		outcome       string
		strands       []reedengine.StrandStatus
		otherOutputs  bool
		removeErr     error
		ifLiveOnly    bool
		wantRemoved   []string
		wantFound     bool
		wantErrSubstr string
	}{
		{name: "terminal_candidate_on_a_live_strand_is_removed", outcome: "done", strands: []reedengine.StrandStatus{live("strand-1")}, wantRemoved: []string{"strand-1"}},
		{name: "dead_pane_candidate_removes_nothing", outcome: "died", strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: false}}},
		{name: "untracked_candidate_removes_nothing", outcome: "timeout"},
		{name: "other_output_set_strand_survives", outcome: "done", strands: []reedengine.StrandStatus{live("strand-1"), live("strand-other")}, otherOutputs: true, wantRemoved: []string{"strand-1"}},
		{name: "attachable_candidate_is_attached_not_removed", outcome: runOutcomeRunning, strands: []reedengine.StrandStatus{live("strand-1")}, wantFound: true},
		{name: "removal_failure_is_an_error_not_a_not_found", outcome: "done", strands: []reedengine.StrandStatus{live("strand-1")}, removeErr: errors.New("reed down"), wantRemoved: []string{"strand-1"}, wantErrSubstr: `could not remove the superseded strand strand-1`},
		{name: "attach_if_live_removes_nothing", outcome: "done", strands: []reedengine.StrandStatus{live("strand-1")}, ifLiveOnly: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: tt.strands}}, RemoveStrandErr: tt.removeErr}
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
			seedPresentReedState(t, dotLyxDir)

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
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			// KeepPane so the attachable row's own harvest does not remove its strand, which would read as a superseded-strand removal.
			spec := Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, KeepPane: true}
			attach := runner.Attach
			if tt.ifLiveOnly {
				attach = runner.AttachIfLive
			}
			_, found, err := attach(spec)

			if tt.wantErrSubstr == "" && err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if tt.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) || !strings.Contains(err.Error(), `lyx reed remove strand-1`) {
					t.Fatalf("Attach() error = %v; want it to contain %q and the way forward `lyx reed remove strand-1`", err, tt.wantErrSubstr)
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
		})
	}
}

// TestAttach_Multiplicity covers candidate-evaluation-order and one-live-match-or-none: the
// multiplicity rule applies only to the surviving ATTACHABLE set, an error verdict dominates
// whatever the other candidates say, and two ordinary leftovers (both non-terminal-classified,
// tracked-live-idle records) must respawn rather than error.
func TestAttach_Multiplicity(t *testing.T) {
	t.Run("TwoTimeoutLeftovers_RespawnsNotError", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{
			Strands: []reedengine.StrandStatus{
				{GUID: "strand-1", PaneID: "%1", Live: true},
				{GUID: "strand-2", PaneID: "%2", Live: true},
			},
		}}}
		fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
		runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
		seedPresentReedState(t, dotLyxDir)

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
		runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
		seedPresentReedState(t, dotLyxDir)

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
		runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
		seedPresentReedState(t, dotLyxDir)

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

// TestAttach_OutputFilesMismatch pins that a run dir whose OutputFiles do not match the spec's is
// never a candidate at all.
func TestAttach_OutputFilesMismatch(t *testing.T) {
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

// TestAttach_UntrackedTerminalRecord_RespawnEligibleRegardlessOfAge pins the new rule: a candidate
// whose persisted Outcome already reads a terminal value (done/asking/died/timeout) is respawn-eligible
// at any directory age, whatever reed says of its pane — parity with dispositionCandidate's own
// tracked-and-live branch, which already treats a terminal Outcome as respawn-eligible regardless of
// age. Covers both routes into leftoverThenAgeVerdict: an untracked strand, and a tracked strand with a
// cleared pane binding.
//
//testtiming:keep pins that a terminal persisted Outcome is respawn-eligible at any directory age for both an untracked strand and a cleared pane binding, which the age-rule rows never reach
func TestAttach_UntrackedTerminalRecord_RespawnEligibleRegardlessOfAge(t *testing.T) {
	t.Run("Untracked", func(t *testing.T) {
		for _, outcome := range []string{"done", "asking", "died", "timeout"} {
			t.Run(outcome, func(t *testing.T) {
				reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: nil}}}
				fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
				runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
				seedPresentReedState(t, dotLyxDir)
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
		for _, outcome := range []string{"done", "asking", "died", "timeout"} {
			t.Run(outcome, func(t *testing.T) {
				reed := &fakeReed{StatusQueue: []reedengine.StatusResult{deadStatus("strand-1", "")}}
				fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
				runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
				seedPresentReedState(t, dotLyxDir)
				fc := newFakeClock(time.Now())
				runner.clock = fc

				outputFile := filepath.Join(runRoot, "out.md")
				runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: outcome, includeOutcome: true})
				setDirAge(t, runDir, fc.Now().Add(-2*time.Duration(30)*time.Second))

				_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, Display: render.Display{Anchor: render.AnchorBelowParent}})
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

// TestAttach_ReedStateGate_AbsentOrUnreadable pins that both an absent and an unreadable reed.json
// are errors, never found == false, at any directory age, and that Attach does NOT consult Status()
// for the absent question — the asymmetry LoadState vs ReedOps.Status() exists to preserve.
func TestAttach_ReedStateGate_AbsentOrUnreadable(t *testing.T) {
	tests := []struct {
		name          string
		seed          func(t *testing.T, dotLyxDir string)
		checkNoStatus bool
	}{
		{"absent", func(t *testing.T, dotLyxDir string) {}, true},
		{"unreadable", seedUnreadableReedState, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
			fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
			tt.seed(t, dotLyxDir)

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

// TestAttach_StatusError pins that ReedOps.Status() itself failing is an error, never found ==
// false, at any directory age, covering both a torn-down session and an unrelated tmux fault.
func TestAttach_StatusError(t *testing.T) {
	tests := []struct {
		name    string
		wantErr error
	}{
		{"torn_down_session", errors.New(`no reed session; run "lyx reed up"`)},
		{"tmux_fault", errors.New("tmux: server not found")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusErr: tt.wantErr}
			fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
			seedPresentReedState(t, dotLyxDir)

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

// TestAttach_NegativeTimeout pins that a negative Timeout is rejected before scanning and before any
// reed read.
func TestAttach_NegativeTimeout(t *testing.T) {
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

// TestAttach_RunningRecordSatisfiedFileContract_HarvestsNotRespawn is the F1 regression guard
// (crucible round fable5-xhigh-r6): a run.json whose persisted Outcome still reads runOutcomeRunning
// AND whose every declared output file is already on disk is a run that FINISHED but whose driver
// crashed before any Wait could classify it — so it is harvested as OutcomeDone rather than
// archived-and-respawned, for every negative liveness answer (untracked, a dead pane, and a cleared
// pane binding) and at any directory age.
//
// Before the fix, dispositionCandidate routed all three of these to verdictRespawnEligible (the dead
// pane directly, the other two via leftoverThenAgeVerdict's own files-exist branch), so Attach
// reported not-found and SingleLLMProducer archived the finished files and re-ran the whole LLM step —
// reproduced live in round fable5-xhigh-r6 against a staged Discussion-Write. Reverting the attach.go
// change makes every subtest here fail with found=false, so the guard is load-bearing rather than
// vacuous.
//
// The strand shape governs only how the reconstructed run's own Wait harvests it: checkLivenessTick's
// not-tracked and not-live branches both consult allOutputFilesExist first and classify OutcomeDone,
// so all three shapes reach the same Done outcome and the same run-dir cleanup.
//
// The same harvest applies when reed's own bookkeeping is unavailable (crucible round
// opus5-high-r7's F1): each of Attach's three reed-state gates — an unreadable reed.json, an absent
// one, and a Status() that errors — must consult the file contract before refusing. All three gates
// sit AHEAD of dispositionCandidate, so the guard there could never reach them: a run whose agent
// had written every declared output file before its driver died hard-failed the whole Shed step
// instead of being harvested, and the run directory was left in place for every later resume to
// re-refuse identically. Each row drives the reconstructed run all the way to its own Outcome,
// because harvesting is only correct if Wait can classify OutcomeDone with reed still broken.
func TestAttach_RunningRecordSatisfiedFileContract_HarvestsNotRespawn(t *testing.T) {
	tests := []struct {
		name   string
		status reedengine.StatusResult
		age    time.Duration
		// seedState seeds reed.json; nil seeds a present, valid one.
		seedState func(t *testing.T, dotLyxDir string)
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
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
			if tt.seedState != nil {
				tt.seedState(t, dotLyxDir)
			} else {
				seedPresentReedState(t, dotLyxDir)
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

// TestAttach_RunningRecordUnsatisfiedFileContract_RespawnsOrErrors pins the other side of the F1 fix:
// a running record whose output files are NOT all present is never harvested — a partially-written or
// never-started run has no finished work to attribute, so the ordinary liveness dispositioning still
// applies (respawn for a confirmed-dead pane; an age-gated error for the ambiguous untracked/
// binding-cleared answers). This is what keeps the harvest gated on a genuinely satisfied contract
// rather than on the run.json's mere presence.
//
// The rows are also the leftover-then-age rule: a tracked strand with a dead pane is unambiguous
// evidence the agent is gone, so it respawns at any directory age; an untracked strand
// (errStrandNotTracked) and a binding-cleared one (errStrandPaneBindingCleared: tracked, not live,
// PaneID empty, spec Anchor not hidden) error while the directory is young enough to be a
// concurrently-starting run and respawn once it clears the age guard (StartupTimeoutS 30 -> minAge
// 60s). An empty (legacy) Outcome keeps the age rule exactly like the runOutcomeRunning case: it is
// neither a terminal Outcome nor a satisfied file contract, so a young directory still cannot be
// ruled dead.
func TestAttach_RunningRecordUnsatisfiedFileContract_RespawnsOrErrors(t *testing.T) {
	tests := []struct {
		name   string
		status reedengine.StatusResult
		age    time.Duration
		// omitOutcome writes the run.json without an "outcome" key, a legacy pre-Outcome-field record.
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
			display: render.Display{Anchor: render.AnchorBelowParent}, wantError: true,
		},
		{
			name: "binding_cleared_old_dir_respawns", status: deadStatus("strand-1", ""), age: 2 * time.Minute,
			display: render.Display{Anchor: render.AnchorBelowParent},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{tt.status}}
			fx := newFixture(t, reed, &fakeEngine{}, withSeparateRunDir())
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
			seedPresentReedState(t, dotLyxDir)
			fc := newFakeClock(time.Now())
			runner.clock = fc

			// Output file deliberately NOT created: the file contract is unsatisfied.
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

// TestAttach_KeepPane pins that KeepPane on the caller's spec is honoured by the attached run: set,
// it suppresses the Done cleanup; absent, cleanup runs exactly as it does for a started run.
//
// Both rows seed a tracked-and-live candidate whose output files are already present and whose
// events file holds a Done, so they also pin that liveness is answered first (the candidate is
// attached, never treated as a leftover) and that the reconstructed run's Wait reads the persisted
// events path and classifies OutcomeDone.
//
//testtiming:keep pins that Spec.KeepPane suppresses an attached run's done cleanup (run dir kept, no RemoveStrand) and that its absence performs both
func TestAttach_KeepPane(t *testing.T) {
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
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
			seedPresentReedState(t, dotLyxDir)

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

// TestAttach_UnreadableRunJSONMidScan_DoesNotAbortScan pins the scan's skip discipline: a corrupt
// run.json alongside a valid, matching one must not abort the whole scan.
func TestAttach_UnreadableRunJSONMidScan_DoesNotAbortScan(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
	runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
	seedPresentReedState(t, dotLyxDir)

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

// TestAttach_DeadlineAtAttachTime pins the deadline of an attached run. It restarts at attach time
// (now + spec.Timeout, never CreatedAt + Timeout), proven by attaching to a run directory aged far
// past spec.Timeout and asserting it does not immediately time out; and a spec with a zero Timeout
// attaches with cfg.RunTimeoutMin applied, normalized the way the spec it matches on is.
//
// If Attach got either wrong, tick 1 would classify OutcomeTimeout immediately, before ever calling
// Sleep — so the terminating event is planted only from inside the first Sleep call, which only a
// correct (fresh, non-zero) deadline ever reaches.
func TestAttach_DeadlineAtAttachTime(t *testing.T) {
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
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
			seedPresentReedState(t, dotLyxDir)

			outputFile := filepath.Join(runRoot, "out.md")
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
			eventsPath := filepath.Join(runDir, eventsFileName)

			fc := newFakeClock(time.Now())
			// CreatedAt in the fixture is "now" (seedAttachRun's own timestamp), well within
			// spec.Timeout — so to prove the deadline is NOT CreatedAt-derived, the run DIRECTORY
			// itself is aged far past it.
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

// TestAttach_OutputFileMatching_ResolvedAbsoluteSet pins that matching is on the resolved absolute
// set: a spec with relative entries matches a run.json written with absolute ones, and a spec naming
// the same files in a different order still matches.
func TestAttach_OutputFileMatching_ResolvedAbsoluteSet(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
	runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
	seedPresentReedState(t, dotLyxDir)

	// NewRunner's worktreeRoot is the parent of anchorPath; resolve the same absolute files a caller
	// with a relative OutputFiles entry, joined against worktreeRoot, would produce.
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

// TestAttach_OffsetStartsAtZero covers attach-reconstructs-the-run-explicitly's replay decision:
// a pre-existing events.jsonl whose last event is a completion classifies OutcomeDone on the first tick after attach (the missed-terminal-Stop case),
// and the same backlog with output files absent is a held turn end that keeps the run polling to its deadline.
//
//testtiming:keep pins that the whole pre-existing events backlog is replayed on attach and its last event wins, for a completion and for a held Stop
func TestAttach_OffsetStartsAtZero(t *testing.T) {
	t.Run("BacklogEndsInCompletion_ClassifiesDone", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
		fx := newFixture(t, reed, engine, withConfig(fastConfig), withSeparateRunDir())
		runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
		seedPresentReedState(t, dotLyxDir)

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
		runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
		seedPresentReedState(t, dotLyxDir)

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

// TestAttach_StartedSeededTrue pins the started seed: attaching to a strand whose run.json already
// carries Started: true (a prior Wait already observed StartupReady for it) and whose CapturePane
// now returns a mid-turn capture (no ready markers) must not classify OutcomeDied after
// startup_timeout_s, and must not play the trust-dismiss key sequence even when the capture happens
// to contain a trust-dialog phrase — because the startup probe must never run at all once the
// persisted Started is true, whatever else is true of the run. Without the seeded started:true, this
// run.json would decode with Started still at its zero value (false), and the mid-turn capture here
// would instead correctly re-trigger the startup probe — proving the seed, not just the field's
// existence, is what this test exercises.
//
//testtiming:keep pins that a persisted Started: true skips the startup probe on attach, with no engine Startup call and no trust-dismiss Enter played into a live pane
func TestAttach_StartedSeededTrue(t *testing.T) {
	reed := &fakeReed{
		StatusQueue:  []reedengine.StatusResult{liveStatus("strand-1", "%1")},
		CaptureQueue: []string{"please trust this folder — mid turn capture"},
	}
	// StartupScript deliberately left empty: fakeEngine.Startup would return StartupPending for
	// every call, and any call at all is the regression this test exists to catch.
	engine := &fakeEngine{}
	fx := newFixture(t, reed, engine, withConfig(Config{StartupTimeoutS: 1, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1}), withSeparateRunDir())
	runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true, started: true})
	eventsPath := filepath.Join(runDir, eventsFileName)

	fc := newFakeClock(time.Now())
	// Nothing ends this run on tick 1 (no events yet, and started=true skips the startup probe
	// entirely, including its own output-files check) — the terminating event is planted from
	// inside the first Sleep call purely to keep the test finite; the regression this test guards
	// against (re-probing startup) would have shown up already, on tick 1, via StartupCalls/SendKeyCalls.
	mc := &multiStepClock{fakeClock: fc, steps: []func(){
		func() {
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("append done event: %v", err)
			}
		},
	}}
	runner.clock = mc

	result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: 10 * time.Minute})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if !found {
		t.Fatal("found = false; want true")
	}
	if result.Outcome == OutcomeDied {
		t.Errorf("Outcome = %q; want anything but died — the started seed must skip the startup probe entirely", result.Outcome)
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

// TestAttach_StrandLaterLosesPaneBinding pins that an attached run keeps full liveness coverage:
// Attach's own dispositioning read sees a live pane (so the candidate is attachable) and Wait's
// first liveness tick sees the binding cleared. With output files present a hidden-anchor spec
// still classifies OutcomeDone, proving the inherited checkLivenessTick branches are reached. With
// the output file never created and an empty Anchor, Attach defaults it to AnchorBelowParent on the
// reconstructed Run, so Wait surfaces errStrandPaneBindingCleared rather than the empty Anchor
// taking the hidden-strand carve-out and classifying OutcomeDied.
//
//testtiming:keep pins that an attached run keeps liveness coverage after a cleared binding: done with files present, and errStrandPaneBindingCleared once an empty Anchor is defaulted
func TestAttach_StrandLaterLosesPaneBinding(t *testing.T) {
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
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
			seedPresentReedState(t, dotLyxDir)

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

// TestAttach_ReedStateUnavailable_StillRefusesWithoutAFinishedRun pins the other side of F1's fix:
// the harvest is gated on a genuinely satisfied file contract plus a record still declaring itself
// running, and every reed gate keeps its original refusal when either is missing.
//
// The unsatisfied-contract row is what keeps the crash-versus-bounce trap shut, and the terminal-record
// row is what keeps a record that already ended from being re-harvested. The two-running-records row
// covers soleFinishedCandidate's exactly-one rule: reed is precisely the thing that cannot be
// consulted to pick between them here, so falling through to the gate's own refusal is the honest
// answer rather than choosing one silently.
func TestAttach_ReedStateUnavailable_StillRefusesWithoutAFinishedRun(t *testing.T) {
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
			// reed.json deliberately not seeded: the absent-state-file gate.

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
