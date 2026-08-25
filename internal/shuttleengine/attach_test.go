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

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// newAttachTestRunner returns a Runner scoped to a fresh temp worktree, with its run-dir root
// pointed at a separate temp directory (runRoot) via cfg.RunDir, so a test can seed run.json
// fixtures without reasoning about the anchor-relative default layout. dotLyxDir is where reed's own
// state file lives, derived the same way Attach derives it.
func newAttachTestRunner(t *testing.T, reed ReedOps, engine Engine, cfg Config) (runner *Runner, anchorPath, dotLyxDir, runRoot string) {
	t.Helper()
	worktreeRoot := t.TempDir()
	anchorPath = filepath.Join(worktreeRoot, "sub", "dir")
	if err := os.MkdirAll(anchorPath, 0o755); err != nil {
		t.Fatalf("mkdir anchor path: %v", err)
	}
	runRoot = t.TempDir()
	cfg.RunDir = runRoot
	runner = NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)
	dotLyxDir = filepath.Join(anchorPath, lyxdirs.DotLyxDirName)
	return runner, anchorPath, dotLyxDir, runRoot
}

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
			runner, _, _, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
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
			if result != (Result{}) {
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
func TestAttach_OutcomeDisposition(t *testing.T) {
	tests := []struct {
		name           string
		outcome        string
		includeOutcome bool
		wantFound      bool
	}{
		{"terminal_done", "done", true, false},
		{"terminal_asking", "asking", true, false},
		{"terminal_died", "died", true, false},
		{"terminal_timeout", "timeout", true, false},
		{"running_attaches", runOutcomeRunning, true, true},
		{"omitted_outcome_is_legacy_upgrade_path", "", false, false},
		{"unrecognized_outcome", "some-future-value", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
			runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
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
			if !tt.wantFound && result != (Result{}) {
				t.Errorf("result = %+v; want zero Result when not found", result)
			}
		})
	}
}

// TestAttach_Multiplicity covers candidate-evaluation-order and one-live-match-or-none: the
// multiplicity rule applies only to the surviving ATTACHABLE set, an error verdict dominates
// whatever the other candidates say, and two ordinary leftovers (both non-terminal-classified,
// tracked-live-idle records) must respawn rather than error.
func TestAttach_Multiplicity(t *testing.T) {
	t.Run("TwoAskingLeftovers_RespawnsNotError", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{
			Strands: []reedengine.StrandStatus{
				{GUID: "strand-1", PaneID: "%1", Live: true},
				{GUID: "strand-2", PaneID: "%2", Live: true},
			},
		}}}
		runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
		seedPresentReedState(t, dotLyxDir)

		outputFile := filepath.Join(runRoot, "out.md")
		seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: "asking", includeOutcome: true})
		seedAttachRun(t, runRoot, "run-2", seedAttachRunOpts{strandGUID: "strand-2", outputFiles: []string{outputFile}, outcome: "asking", includeOutcome: true})

		result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
		if err != nil {
			t.Fatalf("Attach() error = %v; want nil (two respawn-eligible leftovers, not an error)", err)
		}
		if found {
			t.Errorf("found = true; want false")
		}
		if result != (Result{}) {
			t.Errorf("result = %+v; want zero Result", result)
		}
	})

	t.Run("ErrorCandidateDominatesAttachableCandidate", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-2", "%2")}}
		runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
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
		runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
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

// TestAttach_DeadPane covers a strand tracked with a dead pane (PaneID != ""): unambiguous evidence
// the agent is gone, so it needs neither the age rule nor the output-files tie-breaker — respawn
// at both a young and an old directory age.
func TestAttach_DeadPane(t *testing.T) {
	for _, age := range []struct {
		name string
		at   time.Time
	}{
		{"young_dir", time.Now()},
		{"old_dir", time.Now().Add(-time.Hour)},
	} {
		t.Run(age.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{deadStatus("strand-1", "%1")}}
			runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
			seedPresentReedState(t, dotLyxDir)

			outputFile := filepath.Join(runRoot, "out.md")
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
			setDirAge(t, runDir, age.at)

			result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
			if err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if found {
				t.Errorf("found = true; want false — a dead pane is unambiguous evidence the agent is gone")
			}
			if result != (Result{}) {
				t.Errorf("result = %+v; want zero Result", result)
			}
		})
	}
}

// TestAttach_OutputFilesMismatch pins that a run dir whose OutputFiles do not match the spec's is
// never a candidate at all.
func TestAttach_OutputFilesMismatch(t *testing.T) {
	reed := &fakeReed{}
	runner, _, _, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
	seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{filepath.Join(runRoot, "other.md")}, outcome: runOutcomeRunning, includeOutcome: true})

	result, found, err := runner.Attach(Spec{OutputFiles: []string{filepath.Join(runRoot, "out.md")}, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if found {
		t.Errorf("found = true; want false — output files do not match, so no candidate matched")
	}
	if result != (Result{}) {
		t.Errorf("result = %+v; want zero Result", result)
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed.CallLog = %v; want empty — no candidate matched, so the reed gate is never reached", reed.CallLog)
	}
}

// TestAttach_UntrackedStrand_AgeRule covers the leftover-then-age rule for an untracked strand
// (errStrandNotTracked): an error while the directory is young enough to be a concurrently-starting
// run, respawn-eligible once it clears the age guard.
func TestAttach_UntrackedStrand_AgeRule(t *testing.T) {
	tests := []struct {
		name      string
		age       time.Duration
		wantErr   bool
		wantFound bool
	}{
		{"younger_than_minAge_errors", 10 * time.Second, true, false},
		{"older_than_minAge_respawns", 2 * time.Minute, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// StartupTimeoutS 30 -> minAge = 60s.
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: nil}}}
			runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
			seedPresentReedState(t, dotLyxDir)
			fc := newFakeClock(time.Now())
			runner.clock = fc

			outputFile := filepath.Join(runRoot, "out.md") // never written: not the leftover case
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
			setDirAge(t, runDir, fc.Now().Add(-tt.age))

			_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
			if tt.wantErr && err == nil {
				t.Fatal("Attach() error = nil; want an error — a live agent cannot yet be ruled out")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if found != tt.wantFound {
				t.Errorf("found = %v; want %v", found, tt.wantFound)
			}
		})
	}
}

// TestAttach_BindingClearedStrand_AgeRule mirrors TestAttach_UntrackedStrand_AgeRule for the
// errStrandPaneBindingCleared case: tracked, not live, PaneID empty, and the normalized spec's
// Anchor is not hidden.
func TestAttach_BindingClearedStrand_AgeRule(t *testing.T) {
	tests := []struct {
		name      string
		age       time.Duration
		wantErr   bool
		wantFound bool
	}{
		{"younger_than_minAge_errors", 10 * time.Second, true, false},
		{"older_than_minAge_respawns", 2 * time.Minute, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{deadStatus("strand-1", "")}}
			runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
			seedPresentReedState(t, dotLyxDir)
			fc := newFakeClock(time.Now())
			runner.clock = fc

			outputFile := filepath.Join(runRoot, "out.md")
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
			setDirAge(t, runDir, fc.Now().Add(-tt.age))

			_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, Display: render.Display{Anchor: render.AnchorBelowParent}})
			if tt.wantErr && err == nil {
				t.Fatal("Attach() error = nil; want an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if found != tt.wantFound {
				t.Errorf("found = %v; want %v", found, tt.wantFound)
			}
		})
	}
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
			runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
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
			runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
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
	runner, _, _, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
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

// TestAttach_LeftoverOutputFilesExist covers leftover-run-dir-from-a-completed-run: in a
// non-attachable branch, a matched record whose output files all already exist is respawn-eligible
// at any directory age — including a directory younger than the age guard (the fast-bounce-after-
// failed-cleanup case) and a KeepPane leftover.
func TestAttach_LeftoverOutputFilesExist(t *testing.T) {
	tests := []struct {
		name string
		age  time.Duration
	}{
		{"fast_bounce_young_dir", 10 * time.Second},
		{"keep_pane_leftover_old_dir", 2 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: nil}}}
			runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5})
			seedPresentReedState(t, dotLyxDir)
			fc := newFakeClock(time.Now())
			runner.clock = fc

			outputFile := filepath.Join(runRoot, "out.md")
			touchOutputFile(t, outputFile)
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
			setDirAge(t, runDir, fc.Now().Add(-tt.age))

			result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
			if err != nil {
				t.Fatalf("Attach() error = %v; want nil", err)
			}
			if found {
				t.Errorf("found = true; want false — a leftover from a finished run, not an interrupted one")
			}
			if result != (Result{}) {
				t.Errorf("result = %+v; want zero Result", result)
			}
		})
	}
}

// TestAttach_OutputFilesExistButLive_AttachesNotLeftover pins that liveness is answered FIRST: a
// tracked-and-live candidate with all output files already present is attached, never treated as a
// leftover, and its own first tick classifies OutcomeDone with the output files still on disk.
func TestAttach_OutputFilesExistButLive_AttachesNotLeftover(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, engine, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)
	fc := newFakeClock(time.Now())
	runner.clock = fc

	outputFile := filepath.Join(runRoot, "out.md")
	touchOutputFile(t, outputFile)
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
	// A live candidate's own first tick classifies OutcomeDone from allOutputFilesExist on its own —
	// this event is seeded only so Wait terminates on its very first tick instead of looping to the
	// config deadline.
	if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if !found {
		t.Fatal("found = false; want true — tracked and live wins over the output-files leftover check")
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q; want %q", result.Outcome, OutcomeDone)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Errorf("run dir still exists after Done cleanup, stat err = %v", err)
	}
}

// TestAttach_AnchorDefaulting pins attach-normalizes-the-spec-it-matches-on's third normalization: a
// spec with an empty Anchor attaches to a binding-cleared strand and its Wait surfaces
// errStrandPaneBindingCleared, rather than the empty Anchor taking the hidden-strand carve-out and
// classifying OutcomeDied.
func TestAttach_AnchorDefaulting(t *testing.T) {
	// The candidate itself must be attachable (tracked, live, running) so it reaches Wait; the
	// binding-cleared condition is then reproduced INSIDE Wait's own liveness tick via a second,
	// later Status() answer.
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{
		liveStatus("strand-1", "%1"), // Attach's own dispositioning read: attachable.
		{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "", Live: false}}}, // Wait's first tick.
	}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md") // never created
	seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})

	// Spec.Display.Anchor left empty deliberately: Attach must default it to AnchorBelowParent on the
	// reconstructed Run, not leave it empty (which checkLivenessTick would read as the hidden-strand
	// carve-out).
	_, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
	if !found {
		t.Fatal("found = false; want true — Attach's own dispositioning read saw a live pane")
	}
	if err == nil {
		t.Fatal("Attach() error = nil; want Wait to surface errStrandPaneBindingCleared")
	}
	if !errors.Is(err, errStrandPaneBindingCleared) {
		t.Errorf("Attach() error = %v; want one wrapping errStrandPaneBindingCleared, not the hidden-strand carve-out's OutcomeDied", err)
	}
}

// TestAttach_KeepPane pins that KeepPane on the caller's spec is honoured by the attached run: set,
// it suppresses the Done cleanup; absent, cleanup runs exactly as it does for a started run.
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
			runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, engine, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
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

// TestAttach_WaitRunsAgainstPersistedEventsPath pins that a found attach reconstructs a Run reading
// the persisted EventsPath, not a freshly derived one.
func TestAttach_WaitRunsAgainstPersistedEventsPath(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, engine, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
	eventsPath := filepath.Join(runDir, eventsFileName)
	if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	touchOutputFile(t, outputFile)

	result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if !found {
		t.Fatal("found = false; want true")
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q; want %q — Wait must have read the persisted events file", result.Outcome, OutcomeDone)
	}
}

// TestAttach_UnreadableRunJSONMidScan_DoesNotAbortScan pins the scan's skip discipline: a corrupt
// run.json alongside a valid, matching one must not abort the whole scan.
func TestAttach_UnreadableRunJSONMidScan_DoesNotAbortScan(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
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

// TestAttach_DeadlineRestartedAtAttachTime pins attach-restarts-the-deadline: the attached run's
// deadline is now + spec.Timeout, never CreatedAt + Timeout, proven by attaching to a record whose
// CreatedAt is already older than spec.Timeout and asserting it does not immediately time out.
func TestAttach_DeadlineRestartedAtAttachTime(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, engine, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
	eventsPath := filepath.Join(runDir, eventsFileName)

	fc := newFakeClock(time.Now())
	// CreatedAt in the fixture is "now" (seedAttachRun's own timestamp), well within spec.Timeout —
	// but to prove the deadline is NOT CreatedAt-derived, age the run DIRECTORY itself far past
	// spec.Timeout. If Attach wrongly inherited CreatedAt for the deadline, tick 1 would classify
	// OutcomeTimeout immediately, before ever calling Sleep — so the terminating event is planted
	// only from inside the FIRST Sleep call, which only a correctly-restarted (fresh, not-yet-past)
	// deadline ever reaches.
	setDirAge(t, runDir, fc.Now().Add(-10*time.Minute))
	mc := &multiStepClock{fakeClock: fc, steps: []func(){
		func() {
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("append done event: %v", err)
			}
		},
	}}
	runner.clock = mc

	result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if !found {
		t.Fatal("found = false; want true")
	}
	if result.Outcome == OutcomeTimeout {
		t.Errorf("Outcome = %q; want anything but an immediate timeout — the deadline must restart at attach time", result.Outcome)
	}
}

// TestAttach_ZeroTimeoutUsesConfigDefault pins attach-normalizes-the-spec-it-matches-on: a spec with
// a zero Timeout attaches with cfg.RunTimeoutMin applied, and does not classify OutcomeTimeout on
// its first tick.
func TestAttach_ZeroTimeoutUsesConfigDefault(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, engine, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
	eventsPath := filepath.Join(runDir, eventsFileName)

	fc := newFakeClock(time.Now())
	// If Attach failed to default the zero Timeout, the deadline would be now+0 and tick 1 would
	// classify OutcomeTimeout immediately, before ever calling Sleep — so the terminating event is
	// planted only from inside the first Sleep call, which only a correctly-defaulted (non-zero)
	// deadline ever reaches.
	mc := &multiStepClock{fakeClock: fc, steps: []func(){
		func() {
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("append done event: %v", err)
			}
		},
	}}
	runner.clock = mc

	result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: 0})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if !found {
		t.Fatal("found = false; want true")
	}
	if result.Outcome == OutcomeTimeout {
		t.Errorf("Outcome = %q; want anything but an immediate timeout — a zero Timeout must default to cfg.RunTimeoutMin", result.Outcome)
	}
}

// TestAttach_OutputFileMatching_ResolvedAbsoluteSet pins that matching is on the resolved absolute
// set: a spec with relative entries matches a run.json written with absolute ones, and a spec naming
// the same files in a different order still matches.
func TestAttach_OutputFileMatching_ResolvedAbsoluteSet(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
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

// TestAttach_OffsetStartsAtZero covers attach-reconstructs-the-run-explicitly's replay decision: a
// pre-existing events.jsonl whose last event is a completion classifies OutcomeDone on the first
// tick after attach (the missed-terminal-Stop case), and the same backlog with output files absent
// classifies OutcomeAsking.
func TestAttach_OffsetStartsAtZero(t *testing.T) {
	t.Run("BacklogEndsInCompletion_ClassifiesDone", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
		runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, engine, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
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

	t.Run("BacklogEndsInAsk_ClassifiesAsking", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
		runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, engine, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
		seedPresentReedState(t, dotLyxDir)

		outputFile := filepath.Join(runRoot, "out.md") // never created
		runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
		if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:need operator input\n"), 0o644); err != nil {
			t.Fatalf("seed events: %v", err)
		}

		result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
		if err != nil {
			t.Fatalf("Attach() error = %v; want nil", err)
		}
		if !found || result.Outcome != OutcomeAsking {
			t.Fatalf("found=%v Outcome=%q; want found=true, Outcome=%q — terminal without AwaitOperator, dropped with polling continuing under it", found, result.Outcome, OutcomeAsking)
		}
	})
}

// TestAttach_StartedSeededTrue pins the started seed: attaching to a strand whose CapturePane
// returns a mid-turn capture (no ready markers) must not classify OutcomeDied after
// startup_timeout_s, and must not play the trust-dismiss key sequence even when the capture happens
// to contain a trust-dialog phrase — because the startup probe must never run at all on an attached
// run.
func TestAttach_StartedSeededTrue(t *testing.T) {
	reed := &fakeReed{
		StatusQueue:  []reedengine.StatusResult{liveStatus("strand-1", "%1")},
		CaptureQueue: []string{"please trust this folder — mid turn capture"},
	}
	// StartupScript deliberately left empty: fakeEngine.Startup would return StartupPending for
	// every call, and any call at all is the regression this test exists to catch.
	engine := &fakeEngine{}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, engine, Config{StartupTimeoutS: 1, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})
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

// TestAttach_LaterGoesNotLive_StillClassifiesDone pins that an attached run keeps full liveness
// coverage after the started seed: a strand that later goes not-live with output files present still
// classifies OutcomeDone, proving the inherited checkLivenessTick branches are reached.
func TestAttach_LaterGoesNotLive_StillClassifiesDone(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{
		liveStatus("strand-1", "%1"), // Attach's own dispositioning read.
		deadStatus("strand-1", ""),   // Wait's first liveness tick: pane gone.
	}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md")
	touchOutputFile(t, outputFile)
	seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true})

	result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, Display: render.Display{Anchor: render.AnchorHidden}})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if !found {
		t.Fatal("found = false; want true")
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q; want %q — output files satisfied the file contract despite the pane going not-live", result.Outcome, OutcomeDone)
	}
}
