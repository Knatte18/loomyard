// runlevel_test.go exercises Run's whole sequence against a fake
// BlockingRunner and the plan-valid/plan-unapproved testdata fixtures: the
// run-lock busy refusal, the automatic validation gate, the plan-
// fingerprint mismatch guard and its --fresh archive/re-init escape, a
// from-scratch fresh init, the shuttle-outcome-to-RunResult mapping for all
// four shuttle outcomes, the pause-clearing rule's done/stuck/paused split,
// and progress rendering for a partially-reported state. No real agent
// spawns and no real git subprocess — Run's own logic never shells out to
// git itself (that is spawn-batch's and poll's job), so this file carries no
// //go:build integration tag and runs in Tier 1.

package builderengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeBlockingRunner is a hermetic builderengine.BlockingRunner double: Run
// records every Spec it was handed and returns the canned Result/error a
// test configured, optionally writing WriteOutcome's content to the spec's
// sole OutputFiles entry first — mirroring the real shuttle file contract's
// guarantee that an OutcomeDone result means the agent already wrote every
// output file.
type fakeBlockingRunner struct {
	Result       shuttleengine.Result
	Err          error
	WriteOutcome string

	// RequestPauseIn, when non-empty, has RequestPause called against it
	// just before Run returns — simulating a `lyx builder pause` call that
	// landed WHILE the orchestrator's blocking spawn was in flight, i.e.
	// strictly after Run's own entry-time ClearPause already ran.
	RequestPauseIn string

	Calls []shuttleengine.Spec
}

func (f *fakeBlockingRunner) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.Calls = append(f.Calls, spec)
	if f.WriteOutcome != "" {
		if err := os.WriteFile(spec.OutputFiles[0], []byte(f.WriteOutcome), 0o644); err != nil {
			return shuttleengine.Result{}, err
		}
	}
	if f.RequestPauseIn != "" {
		if err := builderengine.RequestPause(f.RequestPauseIn); err != nil {
			return shuttleengine.Result{}, err
		}
	}
	if f.Err != nil {
		return shuttleengine.Result{}, f.Err
	}
	return f.Result, nil
}

var _ builderengine.BlockingRunner = (*fakeBlockingRunner)(nil)

// runFixture is a fully-wired, fresh-per-call set of Run dependencies: a
// copy of the plan-valid fixture (so a test may mutate its content without
// touching the checked-in testdata), fresh temp builder/reports dirs, every
// one of builderengine's four roles pre-resolved, and a fakeBlockingRunner
// a test configures per case.
type runFixture struct {
	Deps   builderengine.RunDeps
	Runner *fakeBlockingRunner
}

// copyPlanFixture copies every top-level file under srcDir into a fresh temp
// directory and returns that directory's path, so a test that mutates plan
// content (the fingerprint-mismatch case) never touches the checked-in
// testdata fixture shared by every other test in this package.
func copyPlanFixture(t *testing.T, srcDir string) string {
	t.Helper()

	dstDir := t.TempDir()
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", srcDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", filepath.Join(srcDir, e.Name()), err)
		}
		if err := os.WriteFile(filepath.Join(dstDir, e.Name()), data, 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", filepath.Join(dstDir, e.Name()), err)
		}
	}
	return dstDir
}

// newRunFixture builds a fresh runFixture rooted at a copy of testdata's
// plan-valid fixture, with fresh empty builder/reports temp dirs and every
// builder role resolved to a distinct, inspectable Model value.
func newRunFixture(t *testing.T) *runFixture {
	t.Helper()

	planDir := copyPlanFixture(t, filepath.Join("testdata", "plan-valid"))
	builderDir := filepath.Join(t.TempDir(), "builder")
	reportsDir := filepath.Join(builderDir, "reports")

	roles := map[builderengine.Role]modelspec.Resolved{
		builderengine.RoleOrchestrator: {
			Engine: "claude", Model: "orchestrator-model",
			Params: map[string]string{"effort": "medium", "version": "v1"},
		},
		builderengine.RoleImplementer:          {Engine: "claude", Model: "implementer-model", Params: map[string]string{}},
		builderengine.RoleImplementerOversized: {Engine: "claude", Model: "implementer-oversized-model", Params: map[string]string{}},
		builderengine.RoleRecovery:             {Engine: "claude", Model: "recovery-model", Params: map[string]string{}},
	}

	cfg := builderengine.Config{
		SelfFixCap:             2,
		PollWaitS:              480,
		BatchTimeoutMin:        60,
		OrchestratorTimeoutMin: 480,
		BatchContextCapTokens:  100000,
		BatchCardCap:           10,
	}

	runner := &fakeBlockingRunner{}

	return &runFixture{
		Deps: builderengine.RunDeps{
			Runner:     runner,
			PlanDir:    planDir,
			BuilderDir: builderDir,
			ReportsDir: reportsDir,
			// WorktreeRoot must be the same copied planDir the fixture's
			// self-referencing card paths resolve against (per the
			// fixture-self-reference decision) — an unrelated temp dir
			// would make plan-valid's Moves: source (03-refactor-a.md)
			// look missing to Validate's move-source-missing check.
			WorktreeRoot: planDir,
			Config:       cfg,
			Roles:        roles,
		},
		Runner: runner,
	}
}

// doneOutcomeYAML is a well-formed outcome.yaml body a fakeBlockingRunner
// writes to simulate the orchestrator's own final action on a clean finish.
const doneOutcomeYAML = "outcome: done\nstuck_reason: null\nbatches_done: 5\n"

// TestRun_LockBusy proves Run refuses fast with ErrRunBusy when another
// caller already holds the builder dir's run.lock, and that the fake
// runner is never reached — the losing call must touch nothing.
func TestRun_LockBusy(t *testing.T) {
	fx := newRunFixture(t)

	if err := os.MkdirAll(fx.Deps.BuilderDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", fx.Deps.BuilderDir, err)
	}
	held, locked, err := lock.TryAcquireWriteLock(filepath.Join(fx.Deps.BuilderDir, "run.lock"))
	if err != nil || !locked {
		t.Fatalf("pre-acquire run.lock: locked=%v err=%v; want locked=true, err=nil", locked, err)
	}
	defer held.Release()

	_, err = builderengine.Run(fx.Deps, builderengine.RunOptions{})
	if !errors.Is(err, builderengine.ErrRunBusy) {
		t.Fatalf("Run() error = %v; want errors.Is(err, ErrRunBusy)", err)
	}
	if len(fx.Runner.Calls) != 0 {
		t.Errorf("fake runner was reached (%d calls) while run.lock was held; want zero", len(fx.Runner.Calls))
	}
}

// TestRun_ValidationRefusal proves Run refuses an unapproved plan before
// ever reaching the fake runner — the automatic gate half of plan-format's
// validate-both decision.
func TestRun_ValidationRefusal(t *testing.T) {
	fx := newRunFixture(t)
	fx.Deps.PlanDir = copyPlanFixture(t, filepath.Join("testdata", "plan-unapproved"))

	_, err := builderengine.Run(fx.Deps, builderengine.RunOptions{})
	if err == nil {
		t.Fatalf("Run() error = nil; want a validation refusal for an unapproved plan")
	}
	if !strings.Contains(err.Error(), "plan-unapproved") {
		t.Errorf("Run() error = %q; want it to name the plan-unapproved check", err.Error())
	}
	if len(fx.Runner.Calls) != 0 {
		t.Errorf("fake runner was reached (%d calls) for a refused plan; want zero", len(fx.Runner.Calls))
	}
}

// TestRun_FreshInitPersistsState proves a from-scratch Run (no prior
// state.json) initializes a fresh State — a minted RunGUID and the plan's
// own Fingerprint recorded — and persists it to disk before ever spawning
// the orchestrator, so a later cross-process spawn-batch call can read it.
func TestRun_FreshInitPersistsState(t *testing.T) {
	fx := newRunFixture(t)
	fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "sess-fresh", RunDir: "/kept/fresh"}
	fx.Runner.WriteOutcome = doneOutcomeYAML

	result, err := builderengine.Run(fx.Deps, builderengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != builderengine.OutcomeDone || result.BatchesDone != 5 {
		t.Errorf("Run() result = %+v; want outcome done, batches_done 5", result)
	}

	st, err := builderengine.LoadState(fx.Deps.BuilderDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v; want nil", err)
	}
	if st == nil {
		t.Fatal("LoadState() = nil; want the state Run just initialized and saved")
	}
	if st.RunGUID == "" {
		t.Error("st.RunGUID is empty; want a minted run guid")
	}

	wantFingerprint, err := builderengine.Fingerprint(fx.Deps.PlanDir)
	if err != nil {
		t.Fatalf("Fingerprint() error = %v", err)
	}
	if st.PlanFingerprint != wantFingerprint {
		t.Errorf("st.PlanFingerprint = %q; want %q", st.PlanFingerprint, wantFingerprint)
	}
}

// TestRun_FingerprintMismatchThenFreshArchivesAndReinits proves the
// discussion's crash/resume guard end to end: a second Run against a
// plan whose content changed since state.json was created refuses with
// ErrFingerprintMismatch (and never reaches the fake runner); a third Run
// with --fresh instead archives the stale state.json and reports dir and
// re-inits with a fresh RunGUID and the new fingerprint.
func TestRun_FingerprintMismatchThenFreshArchivesAndReinits(t *testing.T) {
	fx := newRunFixture(t)
	fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	fx.Runner.WriteOutcome = doneOutcomeYAML

	// First run: fresh init, plus a batch report on disk standing in for
	// completed progress that a --fresh re-init must sweep away.
	if _, err := builderengine.Run(fx.Deps, builderengine.RunOptions{}); err != nil {
		t.Fatalf("first Run() error = %v; want nil", err)
	}
	if err := os.MkdirAll(fx.Deps.ReportsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(reports dir): %v", err)
	}
	staleReportPath := filepath.Join(fx.Deps.ReportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(staleReportPath, []byte("batch: 01-json-flag\nstatus: done\ntests: green\nstuck_reason: null\n"), 0o644); err != nil {
		t.Fatalf("seed stale report: %v", err)
	}
	firstState, err := builderengine.LoadState(fx.Deps.BuilderDir)
	if err != nil || firstState == nil {
		t.Fatalf("LoadState() after first run = %v, %v; want a state, nil", firstState, err)
	}

	// Mutate the plan's content so its Fingerprint changes, without
	// disturbing anything ParsePlan actually reads: appending a trailing
	// comment line after batch 01's own "## verify:" section is inert —
	// parseVerifySection only ever reads that section's first non-empty
	// line as the command.
	batchPath := filepath.Join(fx.Deps.PlanDir, "01-json-flag.md")
	original, err := os.ReadFile(batchPath)
	if err != nil {
		t.Fatalf("ReadFile(batch 01): %v", err)
	}
	if err := os.WriteFile(batchPath, append(original, []byte("\n<!-- mutated for fingerprint mismatch -->\n")...), 0o644); err != nil {
		t.Fatalf("mutate batch 01: %v", err)
	}

	callsBeforeMismatch := len(fx.Runner.Calls)
	_, err = builderengine.Run(fx.Deps, builderengine.RunOptions{})
	if !errors.Is(err, builderengine.ErrFingerprintMismatch) {
		t.Fatalf("second Run() error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
	}
	if len(fx.Runner.Calls) != callsBeforeMismatch {
		t.Errorf("fake runner was reached on a fingerprint mismatch; calls before=%d after=%d, want equal", callsBeforeMismatch, len(fx.Runner.Calls))
	}

	result, err := builderengine.Run(fx.Deps, builderengine.RunOptions{Fresh: true})
	if err != nil {
		t.Fatalf("third Run(--fresh) error = %v; want nil", err)
	}
	if result.Outcome != builderengine.OutcomeDone {
		t.Errorf("third Run(--fresh) result = %+v; want outcome done", result)
	}

	// The stale report must be swept away — archived alongside the whole
	// reports dir, never left sitting in the freshly recreated one.
	if _, err := os.Stat(staleReportPath); !os.IsNotExist(err) {
		t.Errorf("stale report %q still present after --fresh; want it archived away", staleReportPath)
	}
	entries, err := os.ReadDir(fx.Deps.BuilderDir)
	if err != nil {
		t.Fatalf("ReadDir(builder dir): %v", err)
	}
	var sawArchivedState, sawArchivedReportsDir bool
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "state-") && strings.HasSuffix(e.Name(), ".json") {
			sawArchivedState = true
		}
		if e.IsDir() && strings.HasPrefix(e.Name(), "reports-") {
			sawArchivedReportsDir = true
		}
	}
	if !sawArchivedState {
		t.Errorf("builder dir %v does not contain an archived state-*.json file", entries)
	}
	if !sawArchivedReportsDir {
		t.Errorf("builder dir %v does not contain an archived reports-* directory", entries)
	}

	newState, err := builderengine.LoadState(fx.Deps.BuilderDir)
	if err != nil || newState == nil {
		t.Fatalf("LoadState() after --fresh = %v, %v; want a state, nil", newState, err)
	}
	if newState.RunGUID == firstState.RunGUID {
		t.Errorf("newState.RunGUID = %q; want a freshly minted guid distinct from the first run's %q", newState.RunGUID, firstState.RunGUID)
	}
	wantFingerprint, err := builderengine.Fingerprint(fx.Deps.PlanDir)
	if err != nil {
		t.Fatalf("Fingerprint() error = %v", err)
	}
	if newState.PlanFingerprint != wantFingerprint {
		t.Errorf("newState.PlanFingerprint = %q; want the mutated plan's own fingerprint %q", newState.PlanFingerprint, wantFingerprint)
	}
}

// TestRun_OutcomeMapping tables Run's mapping for all four shuttle outcomes:
// OutcomeDone parses outcome.yaml into a successful RunResult; Asking, Died,
// and Timeout each map to their own distinct *Orchestrator*Error, carrying
// SessionID and the kept RunDir (and, for asking, the LastAssistantMessage)
// — and never attempt to parse a (possibly absent) outcome.yaml.
func TestRun_OutcomeMapping(t *testing.T) {
	t.Run("done parses outcome.yaml into a RunResult", func(t *testing.T) {
		fx := newRunFixture(t)
		fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "sess-done", RunDir: "/kept/done"}
		fx.Runner.WriteOutcome = "outcome: stuck\nstuck_reason: \"batch 03 red\"\nbatches_done: 2\n"

		result, err := builderengine.Run(fx.Deps, builderengine.RunOptions{})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		want := builderengine.RunResult{
			Outcome:     builderengine.OutcomeStuck,
			StuckReason: "batch 03 red",
			BatchesDone: 2,
			SessionID:   "sess-done",
			RunDir:      "/kept/done",
		}
		if result != want {
			t.Errorf("Run() result = %+v; want %+v", result, want)
		}
	})

	t.Run("asking maps to OrchestratorAskingError", func(t *testing.T) {
		fx := newRunFixture(t)
		fx.Runner.Result = shuttleengine.Result{
			Outcome: shuttleengine.OutcomeAsking, SessionID: "sess-ask", RunDir: "/kept/ask",
			LastAssistantMessage: "which batch should I retry?",
		}

		_, err := builderengine.Run(fx.Deps, builderengine.RunOptions{})
		if !errors.Is(err, builderengine.ErrOrchestratorAsking) {
			t.Fatalf("Run() error = %v; want errors.Is(err, ErrOrchestratorAsking)", err)
		}
		var askErr *builderengine.OrchestratorAskingError
		if !errors.As(err, &askErr) {
			t.Fatalf("Run() error = %v; want errors.As match on *OrchestratorAskingError", err)
		}
		if askErr.SessionID != "sess-ask" || askErr.RunDir != "/kept/ask" || askErr.Message != "which batch should I retry?" {
			t.Errorf("askErr = %+v; want SessionID=sess-ask RunDir=/kept/ask Message=%q", askErr, "which batch should I retry?")
		}
	})

	t.Run("died maps to OrchestratorDiedError", func(t *testing.T) {
		fx := newRunFixture(t)
		fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDied, SessionID: "sess-died", RunDir: "/kept/died"}

		_, err := builderengine.Run(fx.Deps, builderengine.RunOptions{})
		if !errors.Is(err, builderengine.ErrOrchestratorDied) {
			t.Fatalf("Run() error = %v; want errors.Is(err, ErrOrchestratorDied)", err)
		}
		var diedErr *builderengine.OrchestratorDiedError
		if !errors.As(err, &diedErr) {
			t.Fatalf("Run() error = %v; want errors.As match on *OrchestratorDiedError", err)
		}
		if diedErr.SessionID != "sess-died" || diedErr.RunDir != "/kept/died" {
			t.Errorf("diedErr = %+v; want SessionID=sess-died RunDir=/kept/died", diedErr)
		}
	})

	t.Run("timeout maps to OrchestratorTimeoutError", func(t *testing.T) {
		fx := newRunFixture(t)
		fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout, SessionID: "sess-timeout", RunDir: "/kept/timeout"}

		_, err := builderengine.Run(fx.Deps, builderengine.RunOptions{})
		if !errors.Is(err, builderengine.ErrOrchestratorTimeout) {
			t.Fatalf("Run() error = %v; want errors.Is(err, ErrOrchestratorTimeout)", err)
		}
		var timeoutErr *builderengine.OrchestratorTimeoutError
		if !errors.As(err, &timeoutErr) {
			t.Fatalf("Run() error = %v; want errors.As match on *OrchestratorTimeoutError", err)
		}
		if timeoutErr.SessionID != "sess-timeout" || timeoutErr.RunDir != "/kept/timeout" {
			t.Errorf("timeoutErr = %+v; want SessionID=sess-timeout RunDir=/kept/timeout", timeoutErr)
		}
	})
}

// TestRun_ClearsPauseOnDoneAndStuckButNotOnPaused proves step 9's rule: a
// terminal outcome clears the builder dir's pause flag when the parsed
// outcome is done or stuck, but leaves it in place when the parsed outcome
// is paused — the operator's own pause request must not be silently
// erased.
func TestRun_ClearsPauseOnDoneAndStuckButNotOnPaused(t *testing.T) {
	tests := []struct {
		name          string
		outcomeYAML   string
		wantPauseLeft bool
	}{
		{name: "done clears pause", outcomeYAML: "outcome: done\nstuck_reason: null\nbatches_done: 1\n", wantPauseLeft: false},
		{name: "stuck clears pause", outcomeYAML: "outcome: stuck\nstuck_reason: \"x\"\nbatches_done: 0\n", wantPauseLeft: false},
		{name: "paused leaves pause flag in place", outcomeYAML: "outcome: paused\nstuck_reason: null\nbatches_done: 0\n", wantPauseLeft: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newRunFixture(t)
			fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
			fx.Runner.WriteOutcome = tt.outcomeYAML
			// Simulates a `lyx builder pause` call landing while the
			// orchestrator's blocking spawn is in flight — strictly AFTER
			// Run's own entry-time ClearPause already ran, per the
			// discussion's race description.
			fx.Runner.RequestPauseIn = fx.Deps.BuilderDir

			if _, err := builderengine.Run(fx.Deps, builderengine.RunOptions{}); err != nil {
				t.Fatalf("Run() error = %v; want nil", err)
			}

			gotPaused := builderengine.PauseRequested(fx.Deps.BuilderDir)
			if gotPaused != tt.wantPauseLeft {
				t.Errorf("PauseRequested() after Run = %v; want %v", gotPaused, tt.wantPauseLeft)
			}
		})
	}
}

// TestRun_ProgressRenderingPartiallyReported proves {{.progress}} renders a
// "done" line for exactly the batches that already have a report on disk
// (a partially-reported resume state), and the literal word "none" would
// have rendered instead had none existed — checked here by asserting the
// filled prompt the fake runner received names the reported batch but not
// an unreported one.
func TestRun_ProgressRenderingPartiallyReported(t *testing.T) {
	fx := newRunFixture(t)
	fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	fx.Runner.WriteOutcome = doneOutcomeYAML

	if err := os.MkdirAll(fx.Deps.ReportsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(reports dir): %v", err)
	}
	reportPath := filepath.Join(fx.Deps.ReportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(reportPath, []byte("batch: 01-json-flag\nstatus: done\ntests: green\nstuck_reason: null\n"), 0o644); err != nil {
		t.Fatalf("seed report: %v", err)
	}

	if _, err := builderengine.Run(fx.Deps, builderengine.RunOptions{}); err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}

	if len(fx.Runner.Calls) != 1 {
		t.Fatalf("fake runner Calls = %d; want exactly 1", len(fx.Runner.Calls))
	}
	prompt := fx.Runner.Calls[0].Prompt

	if !strings.Contains(prompt, "01-json-flag: done") {
		t.Errorf("filled prompt does not mention the reported batch as done:\n%s", prompt)
	}
	if strings.Contains(prompt, "02-list-tests: done") {
		t.Errorf("filled prompt wrongly claims the unreported batch 02-list-tests is done:\n%s", prompt)
	}
}

// TestRun_ProgressRenderingStuckBatchIsNotDone proves {{.progress}} renders a
// batch's OWN reported status, so a batch that reported stuck on a prior run
// is summarized "stuck" (needing recovery) — never "done" — to the resumed
// orchestrator. Labeling a stuck report "done" would make the resumed
// orchestrator skip the recovery the batch still needs and finish the run
// "done" for an incomplete plan, a silent false-success across crash/resume.
func TestRun_ProgressRenderingStuckBatchIsNotDone(t *testing.T) {
	fx := newRunFixture(t)
	fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	fx.Runner.WriteOutcome = doneOutcomeYAML

	if err := os.MkdirAll(fx.Deps.ReportsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(reports dir): %v", err)
	}
	reportPath := filepath.Join(fx.Deps.ReportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(reportPath, []byte("batch: 01-json-flag\nstatus: stuck\ntests: red\nstuck_reason: \"batch 01 blocked\"\n"), 0o644); err != nil {
		t.Fatalf("seed stuck report: %v", err)
	}

	if _, err := builderengine.Run(fx.Deps, builderengine.RunOptions{}); err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}

	if len(fx.Runner.Calls) != 1 {
		t.Fatalf("fake runner Calls = %d; want exactly 1", len(fx.Runner.Calls))
	}
	prompt := fx.Runner.Calls[0].Prompt

	if !strings.Contains(prompt, "01-json-flag: stuck") {
		t.Errorf("filled prompt does not summarize the stuck batch as stuck:\n%s", prompt)
	}
	if strings.Contains(prompt, "01-json-flag: done") {
		t.Errorf("filled prompt wrongly summarizes the stuck batch as done:\n%s", prompt)
	}
}

// TestRun_SpecFieldsMapped proves the shuttleengine.Spec built for the
// orchestrator's own spawn matches modelspec's documented consumer mapping
// and this batch's remaining Spec-field requirements (single output file,
// Interactive false, Timeout from OrchestratorTimeoutMin).
func TestRun_SpecFieldsMapped(t *testing.T) {
	fx := newRunFixture(t)
	fx.Runner.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	fx.Runner.WriteOutcome = doneOutcomeYAML

	if _, err := builderengine.Run(fx.Deps, builderengine.RunOptions{}); err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if len(fx.Runner.Calls) != 1 {
		t.Fatalf("fake runner Calls = %d; want exactly 1", len(fx.Runner.Calls))
	}
	spec := fx.Runner.Calls[0]

	wantResolved := fx.Deps.Roles[builderengine.RoleOrchestrator]
	if spec.Model != wantResolved.Model {
		t.Errorf("spec.Model = %q; want %q", spec.Model, wantResolved.Model)
	}
	if spec.Effort != wantResolved.Params["effort"] {
		t.Errorf("spec.Effort = %q; want %q", spec.Effort, wantResolved.Params["effort"])
	}
	if spec.Version != wantResolved.Params["version"] {
		t.Errorf("spec.Version = %q; want %q", spec.Version, wantResolved.Params["version"])
	}
	if spec.Role != string(builderengine.RoleOrchestrator) {
		t.Errorf("spec.Role = %q; want %q", spec.Role, builderengine.RoleOrchestrator)
	}
	if spec.Interactive {
		t.Errorf("spec.Interactive = true; want false")
	}
	if len(spec.OutputFiles) != 1 {
		t.Fatalf("spec.OutputFiles = %v; want exactly one entry", spec.OutputFiles)
	}
	if filepath.Base(spec.OutputFiles[0]) != "outcome.yaml" {
		t.Errorf("spec.OutputFiles[0] = %q; want it to name outcome.yaml", spec.OutputFiles[0])
	}
	wantTimeout := fx.Deps.Config.OrchestratorTimeoutMin
	if spec.Timeout.Minutes() != float64(wantTimeout) {
		t.Errorf("spec.Timeout = %v; want %d minutes", spec.Timeout, wantTimeout)
	}
	if strings.TrimSpace(spec.Prompt) == "" {
		t.Errorf("spec.Prompt is empty; want the filled orchestrator template")
	}
}
