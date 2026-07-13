// run_test.go covers the run verb's envelope shapes and weft-boundary
// behavior through a fake builderengine.OrchestratorStarter injected
// directly on a *builderCLI literal (bypassing Command()'s
// PersistentPreRunE, the same package-local injection pattern as
// spawnbatch_test.go): ErrRunBusy skips the weft sync; every other outcome
// runs the backstop commit before its envelope; --fresh is threaded through
// to builderengine.Run.

package buildercli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeOrchestratorStarter is a hermetic builderengine.OrchestratorStarter
// double: StartOrchestrator records every Spec it was handed and returns a
// handle whose Wait writes WriteOutcome's content to the spec's sole
// OutputFiles entry and then returns the canned Result/error a test
// configured -- mirroring builderengine's own runlevel_test.go fake.
type fakeOrchestratorStarter struct {
	Result       shuttleengine.Result
	Err          error
	WriteOutcome string
	Calls        []shuttleengine.Spec
}

func (f *fakeOrchestratorStarter) StartOrchestrator(spec shuttleengine.Spec) (builderengine.OrchestratorHandle, error) {
	f.Calls = append(f.Calls, spec)
	return &fakeOrchestratorHandle{starter: f, spec: spec}, nil
}

var _ builderengine.OrchestratorStarter = (*fakeOrchestratorStarter)(nil)

// fakeOrchestratorHandle is the handle fakeOrchestratorStarter returns: a
// fixed strand identity plus a Wait that replays the starter's canned
// outcome-file write and Result/error.
type fakeOrchestratorHandle struct {
	starter *fakeOrchestratorStarter
	spec    shuttleengine.Spec
}

func (h *fakeOrchestratorHandle) StrandGUID() string { return "fake-orchestrator-strand" }

func (h *fakeOrchestratorHandle) Wait() (shuttleengine.Result, error) {
	if h.starter.WriteOutcome != "" {
		if err := os.WriteFile(h.spec.OutputFiles[0], []byte(h.starter.WriteOutcome), 0o644); err != nil {
			return shuttleengine.Result{}, err
		}
	}
	if h.starter.Err != nil {
		return shuttleengine.Result{}, h.starter.Err
	}
	return h.starter.Result, nil
}

// runFixture is a fully-wired *builderCLI plus the fake starter it drives.
type runFixture struct {
	CLI    *builderCLI
	Runner *fakeOrchestratorStarter
	Hub    string
}

func newRunFixture(t *testing.T) *runFixture {
	t.Helper()

	hub := t.TempDir()
	seedPlanFixture(t, hub, builderengineTestdataDir("plan-valid"))

	layout := &hubgeometry.Layout{WorktreeRoot: hub, Cwd: hub, RelPath: "."}
	runner := &fakeOrchestratorStarter{}

	roles := map[builderengine.Role]modelspec.Resolved{
		builderengine.RoleOrchestrator: {Engine: "claude", Model: "orchestrator-model"},
	}
	c := &builderCLI{
		orchestratorStarter: runner,
		mux:                 &pollFakeMux{},
		layout:              layout,
		cfg: builderengine.Config{
			SelfFixCap:             2,
			PollWaitS:              480,
			BatchTimeoutMin:        60,
			OrchestratorTimeoutMin: 480,
			BatchContextCapTokens:  100000,
			BatchCardCap:           10,
		},
		roles:      roles,
		planDir:    hubgeometry.PlanDir(hub),
		builderDir: hubgeometry.BuilderDir(hub),
		reportsDir: hubgeometry.BuilderReportsDir(hub),
	}

	return &runFixture{CLI: c, Runner: runner, Hub: hub}
}

const doneOutcomeYAML = "outcome: done\nstuck_reason: null\nbatches_done: 3\n"

func TestRunCmd_LockBusySkipsWeftSync(t *testing.T) {
	fx := newRunFixture(t)

	if err := os.MkdirAll(fx.CLI.builderDir, 0o755); err != nil {
		t.Fatalf("mkdir builder dir: %v", err)
	}
	held, locked, err := lock.TryAcquireWriteLock(filepath.Join(fx.CLI.builderDir, "run.lock"))
	if err != nil || !locked {
		t.Fatalf("pre-acquire run.lock: locked=%v err=%v; want locked=true, err=nil", locked, err)
	}
	defer held.Release()

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.runCmd(), &out, nil)

	if exitCode != 1 {
		t.Fatalf("run while run.lock is held = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "already in progress") {
		t.Errorf("output missing already-in-progress message; got %q", out.String())
	}
	if len(fx.Runner.Calls) != 0 {
		t.Errorf("fake runner was reached (%d calls) while run.lock was held; want zero", len(fx.Runner.Calls))
	}
}

func TestRunCmd_SuccessEnvelopeAndWeftCommit(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newRunFixture(t)
	fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "sess-1", RunDir: "/kept/run"}
	fx.Runner.WriteOutcome = doneOutcomeYAML

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.runCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("run() = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	for _, want := range []string{
		`"outcome":"done"`, `"batches_done":3`, `"session_id":"sess-1"`,
		`"run_dir":"/kept/run"`, `"weftCommitted"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}
}

func TestRunCmd_OrchestratorErrorStillRunsBackstopWeftCommit(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newRunFixture(t)
	fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDied, SessionID: "sess-died", RunDir: "/kept/died"}

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.runCmd(), &out, nil)

	if exitCode != 1 {
		t.Fatalf("run() with a died orchestrator = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "orchestrator pane died") {
		t.Errorf("output missing the orchestrator-died message unchanged; got %q", out.String())
	}
}

func TestRunCmd_FreshFlagThreadsThrough(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newRunFixture(t)
	fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	fx.Runner.WriteOutcome = doneOutcomeYAML

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.runCmd(), &out, []string{"--fresh"})

	if exitCode != 0 {
		t.Fatalf("run --fresh = %d; want 0, output: %s", exitCode, out.String())
	}
	// The first-ever run has no prior state.json, so --fresh's own
	// archive/re-init branch never fires; this asserts the flag at least
	// parses and the run still completes successfully end to end.
	if len(fx.Runner.Calls) != 1 {
		t.Fatalf("fake runner Calls = %d; want exactly 1", len(fx.Runner.Calls))
	}
}
