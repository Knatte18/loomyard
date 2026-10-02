// run_resume_test.go covers Runner.start's resume preflight: a spec with a ResumeSessionID runs the
// engine's SessionResumer check against the runner's pane cwd before Prepare, a refusal leaves no run
// directory or strand, a warning reaches Run.ResumeWarning, and an engine without the capability
// refuses.

package shuttleengine

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

func newResumeTestEngine() *resumeFakeEngine {
	return &resumeFakeEngine{fakeEngine: &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"}}}
}

func resumeSpec() Spec {
	return Spec{Prompt: "x", OutputFiles: []string{"out.md"}, ResumeSessionID: "11111111-2222-3333-4444-555555555555"}
}

func TestRunnerStart_Resume_ChecksSessionAgainstPaneCwdBeforePrepare(t *testing.T) {
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := newResumeTestEngine()
	runner, anchorPath, _ := newTestRunner(t, reed, engine)
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
	if len(engine.Order) < 2 || engine.Order[0] != "CheckResume" || engine.Order[1] != "Prepare" {
		t.Errorf("call order = %v, want CheckResume before Prepare", engine.Order)
	}
}

func TestRunnerStart_Resume_RefusalLeavesNoRunDirOrStrand(t *testing.T) {
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := newResumeTestEngine()
	engine.Err = errors.New("session is gone")
	runner, anchorPath, _ := newTestRunner(t, reed, engine)

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
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := newResumeTestEngine()
	engine.Warning = "registry unreadable"
	runner, _, _ := newTestRunner(t, reed, engine)
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
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd"}}
	runner, _, _ := newTestRunner(t, reed, engine)

	_, err := runner.Start(resumeSpec())
	if err == nil || !strings.Contains(err.Error(), "SessionResumer") {
		t.Fatalf("Start() error = %v, want it to name the SessionResumer capability", err)
	}
	if len(reed.AddStrandCalls) != 0 || len(engine.PrepareCalls) != 0 {
		t.Errorf("run proceeded despite refusal: AddStrand=%d Prepare=%d", len(reed.AddStrandCalls), len(engine.PrepareCalls))
	}
}

func TestRunnerStart_NoResumeSessionID_NeverCallsCheck(t *testing.T) {
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := newResumeTestEngine()
	runner, _, _ := newTestRunner(t, reed, engine)
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
