// run_resume_test.go covers Runner.start's resume preflight.
// A spec with a ResumeSessionID runs the engine's SessionResumer check against the runner's pane cwd before Prepare.
// A refusal leaves no run directory or strand, and a warning reaches Run.ResumeWarning.
// An engine without the capability refuses.
// A transcript carry runs first, and its copy fact reaches the run record and a held refusal.

package shuttleengine

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func newResumeTestEngine() *resumeFakeEngine {
	return &resumeFakeEngine{fakeEngine: &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"}}}
}

func resumeSpec() Spec {
	return Spec{Prompt: "x", OutputFiles: []string{"out.md"}, ResumeSessionID: "11111111-2222-3333-4444-555555555555"}
}

//testtiming:keep pins that the resume check runs against the runner's pane cwd and before Prepare, which the warning test does not observe
func TestRunnerStart_Resume_ChecksSessionAgainstPaneCwdBeforePrepare(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := newResumeTestEngine()
	fx := newFixture(t, reed, engine, withConfig(fastConfig))
	runner, anchorPath := fx.Runner, fx.Anchor
	readyStart(reed, engine.fakeEngine)

	spec := resumeSpec()
	if _, err := runner.Start(spec); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if len(engine.CheckResumeCalls) != 1 {
		t.Fatalf("CheckResume calls = %d, want 1", len(engine.CheckResumeCalls))
	}
	got := engine.CheckResumeCalls[0]
	if got.SessionID != spec.ResumeSessionID || got.Workdir != anchorPath {
		t.Errorf("CheckResume called with (%q, %q), want (%q, %q)", got.SessionID, got.Workdir, spec.ResumeSessionID, anchorPath)
	}
	if len(engine.Order) < 3 || engine.Order[1] != "CheckResume" || engine.Order[2] != "Prepare" {
		t.Errorf("call order = %v, want CheckResume before Prepare", engine.Order)
	}
}

// TestRunnerStart_Resume_CarriesTheTranscriptFirst pins the carry order: before the check and Prepare.
// The copy fact reaches the run record and a held refusal, and a failed carry refuses before any run directory.
func TestRunnerStart_Resume_CarriesTheTranscriptFirst(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// unawaited starts through StartSession in place of Start.
		unawaited bool
		// copied is what the carry reports.
		copied bool
		// specCopied is Spec.TranscriptCopied, the report of an earlier check.
		specCopied bool
		carryErr   error
		// held makes the resume check refuse with a *SessionHeldError.
		held    bool
		wantErr string
		// wantOrder is the engine's call order.
		wantOrder []string
		// wantRecorded is the run record's TranscriptCopied; it is read only for a run that started.
		wantRecorded bool
		// wantHeldCopied is the copy fact the held refusal carries.
		wantHeldCopied bool
	}{
		{name: "StartRecordsACopy", copied: true, wantOrder: []string{"CarryTranscript", "CheckResume", "Prepare"}, wantRecorded: true},
		{name: "StartSessionRecordsACopy", unawaited: true, copied: true, wantOrder: []string{"CarryTranscript", "CheckResume", "Prepare"}, wantRecorded: true},
		{name: "StartRecordsNoCopyWhenNothingWasCopied", wantOrder: []string{"CarryTranscript", "CheckResume", "Prepare"}},
		{name: "ARetrySpecsCopyFactIsRecordedThoughItsOwnCarryCopiedNothing", specCopied: true, wantOrder: []string{"CarryTranscript", "CheckResume", "Prepare"}, wantRecorded: true},
		{name: "AHeldRefusalCarriesTheCopy", copied: true, held: true, wantErr: "held by live process 4242", wantOrder: []string{"CarryTranscript", "CheckResume"}, wantHeldCopied: true},
		{name: "AHeldRefusalCarriesTheRetrySpecsCopyFact", specCopied: true, held: true, wantErr: "held by live process 4242", wantOrder: []string{"CarryTranscript", "CheckResume"}, wantHeldCopied: true},
		{name: "AFailedCarryRefusesBeforeTheCheck", carryErr: errors.New("disk full"), wantErr: "shuttle: resume check: disk full", wantOrder: []string{"CarryTranscript"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
			engine := newResumeTestEngine()
			engine.Copied, engine.CarryErr = tt.copied, tt.carryErr
			if tt.held {
				engine.Err = &SessionHeldError{SessionID: resumeSpec().ResumeSessionID, PID: 4242, Err: errors.New("held by live process 4242")}
			}
			readyStart(reed, engine.fakeEngine)
			fx := newFixture(t, reed, engine, withConfig(fastConfig))

			spec := resumeSpec()
			spec.TranscriptCopied = tt.specCopied
			var err error
			if tt.unawaited {
				spec.OutputFiles, spec.Unawaited = nil, true
				_, err = fx.Runner.StartSession(spec)
			} else {
				_, err = fx.Runner.Start(spec)
			}

			if !slices.Equal(engine.Order, tt.wantOrder) {
				t.Errorf("call order = %v; want %v", engine.Order, tt.wantOrder)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("start error = %v; want one containing %q", err, tt.wantErr)
				}
				var heldErr *SessionHeldError
				if errors.As(err, &heldErr) && heldErr.TranscriptCopied != tt.wantHeldCopied {
					t.Errorf("SessionHeldError.TranscriptCopied = %v; want %v", heldErr.TranscriptCopied, tt.wantHeldCopied)
				}
				if _, statErr := os.Stat(runDirRoot(fx.Runner.cfg, fx.Anchor)); !os.IsNotExist(statErr) {
					t.Errorf("run directory root stat error = %v; want it absent for a refused resume", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("start error: %v", err)
			}
			rs, found, err := loadRunState(soleStartupRunDir(t, fastConfig, fx.Anchor))
			if err != nil || !found {
				t.Fatalf("loadRunState: found=%v err=%v", found, err)
			}
			if rs.TranscriptCopied != tt.wantRecorded {
				t.Errorf("run record TranscriptCopied = %v; want %v", rs.TranscriptCopied, tt.wantRecorded)
			}
		})
	}
}

func TestRunnerStart_Resume_RefusalLeavesNoRunDirOrStrand(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := newResumeTestEngine()
	engine.Err = errors.New("session is gone")
	fx := newFixture(t, reed, engine, withConfig(fastConfig))
	runner, anchorPath := fx.Runner, fx.Anchor

	_, err := runner.Start(resumeSpec())
	if err == nil || !strings.Contains(err.Error(), "shuttle: resume check: session is gone") {
		t.Fatalf("Start() error = %v, want it wrapped as \"shuttle: resume check: session is gone\"", err)
	}
	if len(reed.AddStrandCalls) != 0 {
		t.Errorf("AddStrand calls = %d, want 0", len(reed.AddStrandCalls))
	}
	if len(engine.PrepareCalls) != 0 {
		t.Errorf("Prepare calls = %d, want 0", len(engine.PrepareCalls))
	}
	if _, statErr := os.Stat(runDirRoot(runner.cfg, anchorPath)); !os.IsNotExist(statErr) {
		t.Errorf("run directory root stat error = %v, want it absent", statErr)
	}
}

func TestRunnerStart_Resume_WarningReachesRunAndRunStarts(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := newResumeTestEngine()
	engine.Warning = "registry unreadable"
	runner := newFixture(t, reed, engine, withConfig(fastConfig)).Runner
	readyStart(reed, engine.fakeEngine)

	run, err := runner.Start(resumeSpec())
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if got := run.ResumeWarning(); got != "registry unreadable" {
		t.Errorf("ResumeWarning() = %q, want %q", got, "registry unreadable")
	}
}

func TestRunnerStart_Resume_EngineWithoutCapabilityRefuses(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd"}}
	runner := newFixture(t, reed, engine, withConfig(fastConfig)).Runner

	_, err := runner.Start(resumeSpec())
	if err == nil || !strings.Contains(err.Error(), "SessionResumer") {
		t.Fatalf("Start() error = %v, want it to name the SessionResumer capability", err)
	}
	if len(reed.AddStrandCalls) != 0 || len(engine.PrepareCalls) != 0 {
		t.Errorf("run proceeded despite refusal: AddStrand=%d Prepare=%d", len(reed.AddStrandCalls), len(engine.PrepareCalls))
	}
}

//testtiming:keep pins that a spec with no ResumeSessionID never calls the resume check and carries no resume warning
func TestRunnerStart_NoResumeSessionID_NeverCallsCheck(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := newResumeTestEngine()
	runner := newFixture(t, reed, engine, withConfig(fastConfig)).Runner
	readyStart(reed, engine.fakeEngine)

	run, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if len(engine.CheckResumeCalls) != 0 {
		t.Errorf("CheckResume calls = %d, want 0", len(engine.CheckResumeCalls))
	}
	if run.ResumeWarning() != "" {
		t.Errorf("ResumeWarning() = %q, want empty", run.ResumeWarning())
	}
}
