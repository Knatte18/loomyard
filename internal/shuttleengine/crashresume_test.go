// crashresume_test.go covers the resume of an interrupted run's session by a later start of its role.
// It also covers the set-aside of interrupted runs that a deliberate restart makes.

package shuttleengine

import (
	"fmt"
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

// interruptedSession is the session id of a seeded earlier run.
const interruptedSession = "11111111-2222-3333-4444-555555555555"

// TestStart_ResumesAnInterruptedRunsSession pins when a start resumes the session of its role's newest earlier run.
// A resumed start prepares that session with the continue preamble and no skill-load turn;
// any other start prepares a fresh session with the prompt as given.
// The earlier run is set aside once a start of its output files is up, resumed or fresh.
func TestStart_ResumesAnInterruptedRunsSession(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	died := RunState{Outcome: string(OutcomeDied), SessionID: interruptedSession}
	tests := []struct {
		name        string
		predecessor RunState
		// otherOutputs makes the earlier run declare another output file than the start.
		otherOutputs bool
		// plainEngine starts on an engine without the SessionResumer capability.
		plainEngine bool
		checkErr    error
		wantResumed bool
		// carried seeds the earlier run's carried operator instructions.
		carried []string
	}{
		{name: "a run that died is resumed", predecessor: died, wantResumed: true},
		{name: "a resumed run keeps its carried instructions out of the prompt and tells no one", predecessor: died, wantResumed: true, carried: []string{"keep the branch"}},
		{name: "a run whose stop was recorded as interrupted is resumed", predecessor: RunState{Outcome: runOutcomeStopped, Interrupted: true, SessionID: interruptedSession}, wantResumed: true},
		{name: "a run reading running with its waiter gone is resumed", predecessor: RunState{Outcome: runOutcomeRunning, PID: deadPID, SessionID: interruptedSession}, wantResumed: true},
		{name: "a run stopped on purpose starts fresh", predecessor: RunState{Outcome: runOutcomeStopped, SessionID: interruptedSession}},
		{name: "a run that timed out starts fresh", predecessor: RunState{Outcome: string(OutcomeTimeout), SessionID: interruptedSession}},
		{name: "a run with a live waiter starts fresh", predecessor: RunState{Outcome: runOutcomeRunning, PID: os.Getpid(), SessionID: interruptedSession}},
		{name: "a run set aside starts fresh", predecessor: RunState{Outcome: string(OutcomeDied), SessionID: interruptedSession, NoResume: true}},
		{name: "a run with no session starts fresh", predecessor: RunState{Outcome: string(OutcomeDied)}},
		{name: "a run of other output files starts fresh and is not set aside", predecessor: died, otherOutputs: true},
		{name: "a session the resume check refuses starts fresh", predecessor: died, checkErr: fmt.Errorf("%w: none under the project", ErrNoTranscript)},
		{name: "an engine that cannot resume a session starts fresh", predecessor: died, plainEngine: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1", Name: "hub:task:darn"}}
			fake := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-new"}}
			readyStart(reed, fake)
			var engine Engine = &resumeFakeEngine{fakeEngine: fake, Err: tt.checkErr}
			if tt.plainEngine {
				engine = fake
			}
			fx := newFixture(t, reed, engine, withConfig(fastConfig))
			var notices []string
			fx.Runner.SetNotifier(func(line string) error {
				notices = append(notices, line)
				return nil
			})
			outputFile := filepath.Join(t.TempDir(), "out.md")
			predecessor := tt.predecessor
			predecessor.CarriedInstructions = tt.carried
			predecessor.RunID, predecessor.RoleKey = "run-old", "darn"
			predecessor.CreatedAt = time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC).Format(time.RFC3339)
			predecessor.OutputFiles = []string{outputFile}
			if tt.otherOutputs {
				predecessor.OutputFiles = []string{filepath.Join(t.TempDir(), "other.md")}
			}
			seedPredecessor(t, fx, predecessor, "PROMPT:skill load\n")

			run, err := fx.Runner.Start(Spec{Prompt: "do the task", OutputFiles: []string{outputFile}, Role: "darn"})
			if err != nil {
				t.Fatalf("Start() error = %v; want nil", err)
			}

			wantSession, wantPrompt, wantSkip := "", "do the task", false
			if tt.wantResumed {
				wantSession, wantPrompt, wantSkip = interruptedSession, interruptedResumePreamble+"do the task", true
			}
			if len(fake.PrepareCalls) != 1 {
				t.Fatalf("Prepare calls = %d; want 1", len(fake.PrepareCalls))
			}
			if got := fake.PrepareCalls[0].Spec; got.ResumeSessionID != wantSession || got.Prompt != wantPrompt || got.SkipSkillLoadTurn != wantSkip {
				t.Errorf("prepared ResumeSessionID, Prompt, SkipSkillLoadTurn = %q, %q, %v; want %q, %q, %v",
					got.ResumeSessionID, got.Prompt, got.SkipSkillLoadTurn, wantSession, wantPrompt, wantSkip)
			}
			record, _, err := loadRunState(filepath.Join(fx.RunRoot, "run-old"))
			if err != nil {
				t.Fatal(err)
			}
			if wantSetAside := !tt.otherOutputs; record.NoResume != wantSetAside {
				t.Errorf("earlier run NoResume = %v; want %v", record.NoResume, wantSetAside)
			}
			if !tt.wantResumed {
				return
			}
			started, _, err := loadRunState(run.RunDir())
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(started.CarriedInstructions, tt.carried) {
				t.Errorf("new record CarriedInstructions = %q; want %q", started.CarriedInstructions, tt.carried)
			}
			if len(notices) != 0 {
				t.Errorf("notices = %q; want none for a resumed start", notices)
			}
		})
	}
}

// TestStart_FailedResumeRetriesAsAFreshSession pins how a start retries a resumed attempt.
// An attempt that dies before its first input is not resumed again.
// The failed attempt and the earlier run are both set aside.
// The retry prepares a fresh session with the prompt as given.
func TestStart_FailedResumeRetriesAsAFreshSession(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	live := reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}
	dead := reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: false}}}
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{live, dead, live}, CaptureQueue: []string{"$ claude ...", "❯ "}}
	fake := &fakeEngine{StartupScript: []StartupState{StartupPending, StartupReady}}
	seedStartupFakes(reed, fake)
	cfg := fastConfig
	cfg.StartRetries = 1
	clock := &awaitClock{fakeClock: newFakeClock(time.Now())}
	fx := newFixture(t, reed, &resumeFakeEngine{fakeEngine: fake}, withConfig(cfg), withClock(clock))
	outputFile := filepath.Join(t.TempDir(), "out.md")
	seedPredecessor(t, fx, RunState{
		RunID: "run-old", RoleKey: "darn", Outcome: string(OutcomeDied), SessionID: interruptedSession,
		CreatedAt:   time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
		OutputFiles: []string{outputFile},
	}, "PROMPT:skill load\n")

	run, err := fx.Runner.Start(Spec{Prompt: "do the task", OutputFiles: []string{outputFile}, Role: "darn"})

	if err != nil || run == nil {
		t.Fatalf("Start() = %v, %v; want the ready run", run, err)
	}
	if len(fake.PrepareCalls) != 2 {
		t.Fatalf("Prepare calls = %d; want 2, the resumed attempt that died and the retry", len(fake.PrepareCalls))
	}
	if got := fake.PrepareCalls[0].Spec; got.ResumeSessionID != interruptedSession || got.Prompt != interruptedResumePreamble+"do the task" {
		t.Errorf("first attempt ResumeSessionID, Prompt = %q, %q; want the resume of %q", got.ResumeSessionID, got.Prompt, interruptedSession)
	}
	if got := fake.PrepareCalls[1].Spec; got.ResumeSessionID != "" || got.Prompt != "do the task" || got.SkipSkillLoadTurn {
		t.Errorf("retry ResumeSessionID, Prompt, SkipSkillLoadTurn = %q, %q, %v; want a fresh session with the prompt as given",
			got.ResumeSessionID, got.Prompt, got.SkipSkillLoadTurn)
	}
	for _, runDir := range runDirsUnder(t, fx.RunRoot) {
		record, _, err := loadRunState(runDir)
		if err != nil {
			t.Fatal(err)
		}
		wantSetAside := runDir != run.RunDir()
		if record.NoResume != wantSetAside {
			t.Errorf("run %s NoResume = %v; want %v", record.RunID, record.NoResume, wantSetAside)
		}
	}
}

// TestRunner_RetireInterruptedRuns_SetsAsideOnlyResumableRuns pins which runs a deliberate restart sets aside.
// Only a run whose session a later start would resume is marked, and a missing run root marks nothing.
// A run whose record cannot be rewritten fails the call without keeping it from marking the others.
func TestRunner_RetireInterruptedRuns_SetsAsideOnlyResumableRuns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	fx := newFixture(t, &fakeReed{}, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())

	if retired, err := fx.Runner.RetireInterruptedRuns(); retired != 0 || err != nil {
		t.Fatalf("RetireInterruptedRuns() over a missing root = %d, %v; want 0, nil", retired, err)
	}

	runs := map[string]struct {
		record      RunState
		wantRetired bool
	}{
		"died":        {RunState{Outcome: string(OutcomeDied), SessionID: interruptedSession}, true},
		"interrupted": {RunState{Outcome: runOutcomeStopped, Interrupted: true, SessionID: interruptedSession}, true},
		"waiterless":  {RunState{Outcome: runOutcomeRunning, PID: deadPID, SessionID: interruptedSession}, true},
		"live":        {RunState{Outcome: runOutcomeRunning, PID: os.Getpid(), SessionID: interruptedSession}, false},
		"stopped":     {RunState{Outcome: runOutcomeStopped, SessionID: interruptedSession}, false},
		"timed-out":   {RunState{Outcome: string(OutcomeTimeout), SessionID: interruptedSession}, false},
		"unwritable":  {RunState{Outcome: string(OutcomeDied), SessionID: interruptedSession}, false},
	}
	for id, run := range runs {
		runDir := filepath.Join(fx.RunRoot, id)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		run.record.RunID = id
		if err := saveRunState(runDir, run.record); err != nil {
			t.Fatal(err)
		}
	}
	unwritableDir := filepath.Join(fx.RunRoot, "unwritable")
	permkit.Chmod(t, unwritableDir, 0o555)

	retired, err := fx.Runner.RetireInterruptedRuns()
	if retired != 3 {
		t.Errorf("RetireInterruptedRuns() retired = %d; want 3", retired)
	}
	if err == nil || !strings.Contains(err.Error(), unwritableDir) {
		t.Errorf("RetireInterruptedRuns() error = %v; want one naming %s", err, unwritableDir)
	}
	for id, run := range runs {
		record, _, err := loadRunState(filepath.Join(fx.RunRoot, id))
		if err != nil {
			t.Fatal(err)
		}
		if record.NoResume != run.wantRetired {
			t.Errorf("%s run NoResume = %v; want %v", id, record.NoResume, run.wantRetired)
		}
	}
}
