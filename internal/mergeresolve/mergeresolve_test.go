package mergeresolve

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// captureLogOutput redirects logger output into a buffer for the duration of one test, restoring
// os.Stderr via t.Cleanup -- the same pattern internal/loomshed/gatefindings_test.go uses.
func captureLogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
	return &buf
}

// fakeMergeSurface implements MergeSurface, recording call order and returning caller-configured
// results/errors.
type fakeMergeSurface struct {
	mergeInResult      fabricengine.MergeResult
	mergeInErr         error
	mergeInProgress    bool
	mergeInProgressErr error
	stageResolvedErr   error
	continueErr        error
	abortErr           error

	calls       []string
	stagedPaths []string
}

func (f *fakeMergeSurface) MergeIn(source string) (fabricengine.MergeResult, error) {
	f.calls = append(f.calls, "MergeIn")
	return f.mergeInResult, f.mergeInErr
}

func (f *fakeMergeSurface) MergeStageResolved(paths []string) (fabricengine.StageResult, error) {
	f.calls = append(f.calls, "MergeStageResolved")
	f.stagedPaths = paths
	return fabricengine.StageResult{}, f.stageResolvedErr
}

func (f *fakeMergeSurface) MergeContinue(msg string) (fabricengine.MergeResult, error) {
	f.calls = append(f.calls, "MergeContinue")
	return fabricengine.MergeResult{}, f.continueErr
}

func (f *fakeMergeSurface) MergeAbort() (fabricengine.MergeResult, error) {
	f.calls = append(f.calls, "MergeAbort")
	return fabricengine.MergeResult{}, f.abortErr
}

func (f *fakeMergeSurface) MergeInProgress() (bool, error) {
	f.calls = append(f.calls, "MergeInProgress")
	return f.mergeInProgress, f.mergeInProgressErr
}

func (f *fakeMergeSurface) calledAny(name string) bool {
	for _, c := range f.calls {
		if c == name {
			return true
		}
	}
	return false
}

// indexOf returns the first index of name in f.calls, or -1.
func (f *fakeMergeSurface) indexOf(name string) int {
	for i, c := range f.calls {
		if c == name {
			return i
		}
	}
	return -1
}

// fakeShuttle records every Spec it was handed and returns one scripted Result/error per call, in
// order. An optional runHook is invoked with the attempt's 1-based call number and the spec just
// before the scripted result is returned, letting a test mutate worktree fixture files as if a real
// session had just edited them.
type fakeShuttle struct {
	results []shuttleengine.Result
	errs    []error
	runHook func(callNumber int, spec shuttleengine.Spec)

	specs []shuttleengine.Spec
}

func (f *fakeShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.specs = append(f.specs, spec)
	callNumber := len(f.specs)

	var res shuttleengine.Result
	if callNumber-1 < len(f.results) {
		res = f.results[callNumber-1]
	}
	var err error
	if callNumber-1 < len(f.errs) {
		err = f.errs[callNumber-1]
	}

	if f.runHook != nil {
		f.runHook(callNumber, spec)
	}

	return res, err
}

// conflictStencilFixture is a minimal, valid conflict stencil carrying exactly the two markers
// buildConflictSpec fills.
const conflictStencilFixture = "# Conflict\n\nPaths:\n{{.conflicted_paths}}\n\nReport: {{.report_path}}\n"

// newTestDeps returns a Deps wired against fake, shuttle, and a fresh worktree/scratch/stencils
// directory tree under t.TempDir(), with the conflict stencil fixture seeded.
func newTestDeps(t *testing.T, fake *fakeMergeSurface, shuttle *fakeShuttle) Deps {
	t.Helper()

	root := t.TempDir()
	worktreeRoot := filepath.Join(root, "worktree")
	scratchDir := filepath.Join(root, "scratch")
	stencilsDir := filepath.Join(root, "stencils", "landing")

	if err := os.MkdirAll(worktreeRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(worktreeRoot): %v", err)
	}
	if err := os.MkdirAll(stencilsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(stencilsDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(stencilsDir, "landing-template-conflict.md"), []byte(conflictStencilFixture), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict stencil): %v", err)
	}

	return Deps{
		Fabric:       fake,
		Shuttle:      shuttle,
		WorktreeRoot: worktreeRoot,
		ScratchDir:   scratchDir,
		StencilsDir:  filepath.Dir(stencilsDir),
		ConflictSpec: "claude:sonnet",
		Registry:     modelspec.Registry{},
		Timeout:      time.Minute,
	}
}

// writeConflictedFixture writes relPath under deps.WorktreeRoot with content, creating parent
// directories as needed.
func writeConflictedFixture(t *testing.T, deps Deps, relPath, content string) {
	t.Helper()
	full := filepath.Join(deps.WorktreeRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", full, err)
	}
}

const conflictedContent = "<<<<<<< HEAD\nmine\n=======\ntheirs\n>>>>>>> feature\n"
const resolvedContent = "resolved content, no markers\n"

func TestResolve_CleanMergeNoSession(t *testing.T) {
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: nil}}
	shuttle := &fakeShuttle{}
	r, err := New(newTestDeps(t, fake, shuttle))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := r.Resolve(context.Background(), "source")
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}
	if res.Outcome != OutcomeResolved {
		t.Errorf("Resolve() outcome = %q; want %q", res.Outcome, OutcomeResolved)
	}
	if len(shuttle.specs) != 0 {
		t.Errorf("shuttle was called %d time(s); want 0 for a clean merge", len(shuttle.specs))
	}
}

func TestResolve_ConflictThenCleanScan_StagesThenConcludes(t *testing.T) {
	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	shuttle := &fakeShuttle{
		results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDone}},
		runHook: func(callNumber int, spec shuttleengine.Spec) {
			writeConflictedFixture(t, deps, "a.txt", resolvedContent)
		},
	}
	deps.Shuttle = shuttle
	r, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := r.Resolve(context.Background(), "source")
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}
	if res.Outcome != OutcomeResolved {
		t.Errorf("Resolve() outcome = %q; want %q", res.Outcome, OutcomeResolved)
	}
	if len(fake.stagedPaths) != 1 || fake.stagedPaths[0] != "a.txt" {
		t.Errorf("staged paths = %v; want [\"a.txt\"]", fake.stagedPaths)
	}
	if !fake.calledAny("MergeContinue") {
		t.Error("MergeContinue was never called")
	}

	// Ordering assertion: conclude never precedes staging.
	stageIdx := fake.indexOf("MergeStageResolved")
	continueIdx := fake.indexOf("MergeContinue")
	if stageIdx == -1 || continueIdx == -1 || continueIdx < stageIdx {
		t.Errorf("call order = %v; want MergeStageResolved before MergeContinue", fake.calls)
	}
}

func TestResolve_ConflictStillUnresolvedAfterSession_RetriesThenAborts(t *testing.T) {
	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	shuttle := &fakeShuttle{
		results: []shuttleengine.Result{
			{Outcome: shuttleengine.OutcomeDone},
			{Outcome: shuttleengine.OutcomeDone},
		},
		// Never removes the markers, so both attempts scan unresolved.
	}
	deps.Shuttle = shuttle
	r, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := r.Resolve(context.Background(), "source")
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}
	if res.Outcome != OutcomeStuck {
		t.Errorf("Resolve() outcome = %q; want %q", res.Outcome, OutcomeStuck)
	}
	if len(shuttle.specs) != 2 {
		t.Errorf("shuttle was called %d time(s); want exactly one retry (2 total)", len(shuttle.specs))
	}
	if !fake.calledAny("MergeAbort") {
		t.Error("MergeAbort was never called")
	}
	if fake.calledAny("MergeContinue") {
		t.Error("MergeContinue was called; want it never reached")
	}
}

func TestResolve_MergeInProgressAtEntry_AbortsBeforeNewAttempt(t *testing.T) {
	fake := &fakeMergeSurface{
		mergeInProgress: true,
		mergeInResult:   fabricengine.MergeResult{Conflicts: nil},
	}
	shuttle := &fakeShuttle{}
	r, err := New(newTestDeps(t, fake, shuttle))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := r.Resolve(context.Background(), "source"); err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	abortIdx := fake.indexOf("MergeAbort")
	mergeInIdx := fake.indexOf("MergeIn")
	if abortIdx == -1 || mergeInIdx == -1 || abortIdx > mergeInIdx {
		t.Errorf("call order = %v; want MergeAbort before MergeIn", fake.calls)
	}
}

func TestResolve_ForeignMergeStateError_StuckNoAbort(t *testing.T) {
	fake := &fakeMergeSurface{mergeInErr: &fabricengine.ErrForeignMergeState{}}
	shuttle := &fakeShuttle{}
	r, err := New(newTestDeps(t, fake, shuttle))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := r.Resolve(context.Background(), "source")
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}
	if res.Outcome != OutcomeStuck {
		t.Errorf("Resolve() outcome = %q; want %q", res.Outcome, OutcomeStuck)
	}
	if fake.calledAny("MergeAbort") {
		t.Error("MergeAbort was called; want it never reached for a foreign merge state")
	}
}

func TestResolve_UnmergeableStateError_StuckWithErrorSurfacedNoAbort(t *testing.T) {
	fake := &fakeMergeSurface{mergeInErr: &fabricengine.ErrUnmergeableState{}}
	shuttle := &fakeShuttle{}
	r, err := New(newTestDeps(t, fake, shuttle))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := r.Resolve(context.Background(), "source")
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}
	if res.Outcome != OutcomeStuck {
		t.Errorf("Resolve() outcome = %q; want %q", res.Outcome, OutcomeStuck)
	}
	if res.Reason == "" {
		t.Error("Reason is empty; want the error surfaced")
	}
	if fake.calledAny("MergeAbort") {
		t.Error("MergeAbort was called; want it never reached for an unmergeable-state error")
	}
}

// unrecognizedMergeError is a typed error MergeIn's own disposition table does not name explicitly,
// proving the catch-all default is escalate rather than fall through unhandled.
type unrecognizedMergeError struct{}

func (unrecognizedMergeError) Error() string { return "unrecognized merge failure" }

func TestResolve_UnrecognizedMergeInError_CatchAllStuck(t *testing.T) {
	fake := &fakeMergeSurface{mergeInErr: unrecognizedMergeError{}}
	shuttle := &fakeShuttle{}
	r, err := New(newTestDeps(t, fake, shuttle))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := r.Resolve(context.Background(), "source")
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}
	if res.Outcome != OutcomeStuck {
		t.Errorf("Resolve() outcome = %q; want %q", res.Outcome, OutcomeStuck)
	}
}

func TestResolve_ShuttleOutcomes_MapToStuckNoConclude(t *testing.T) {
	tests := []struct {
		name    string
		outcome shuttleengine.Outcome
	}{
		{"Asking", shuttleengine.OutcomeAsking},
		{"Died", shuttleengine.OutcomeDied},
		{"Timeout", shuttleengine.OutcomeTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := []string{"a.txt"}
			fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
			deps := newTestDeps(t, fake, nil)
			writeConflictedFixture(t, deps, "a.txt", conflictedContent)

			shuttle := &fakeShuttle{results: []shuttleengine.Result{{Outcome: tt.outcome, SessionID: "session-1", RunDir: "/run/dir"}}}
			deps.Shuttle = shuttle
			r, err := New(deps)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			buf := captureLogOutput(t)

			res, err := r.Resolve(context.Background(), "source")
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}
			if res.Outcome != OutcomeStuck {
				t.Errorf("Resolve() outcome = %q; want %q", res.Outcome, OutcomeStuck)
			}
			if fake.calledAny("MergeContinue") {
				t.Error("MergeContinue was called; want it never reached")
			}
			logged := buf.String()
			if !strings.Contains(logged, "WARN") {
				t.Errorf("captured log %q does not contain level token %q", logged, "WARN")
			}
			for _, key := range []string{"outcome", "attempt", "sessionID", "runDir"} {
				if !strings.Contains(logged, key) {
					t.Errorf("captured log %q does not contain field key %q", logged, key)
				}
			}
		})
	}
}

// TestResolve_ShuttleOutcomes_WarnSurvivesAbortFailure pins the placement guarantee: the Warn line
// lands before abortAndStuck's own MergeAbort call, so a subsequent MergeAbort failure does not
// erase the diagnostic.
func TestResolve_ShuttleOutcomes_WarnSurvivesAbortFailure(t *testing.T) {
	paths := []string{"a.txt"}
	abortErr := errors.New("abort failed")
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}, abortErr: abortErr}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	shuttle := &fakeShuttle{results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDied, SessionID: "session-1", RunDir: "/run/dir"}}}
	deps.Shuttle = shuttle
	r, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	buf := captureLogOutput(t)

	_, err = r.Resolve(context.Background(), "source")
	if err == nil {
		t.Fatal("Resolve() error = nil; want non-nil (MergeAbort failed)")
	}
	if !errors.Is(err, abortErr) {
		t.Errorf("Resolve() error = %v; want it to wrap %v", err, abortErr)
	}
	logged := buf.String()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("captured log %q does not contain level token %q; want the warn line to survive the abort failure", logged, "WARN")
	}
}

func TestResolve_ContextCancellation_SurfacedAsError(t *testing.T) {
	fake := &fakeMergeSurface{}
	shuttle := &fakeShuttle{}
	r, err := New(newTestDeps(t, fake, shuttle))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := r.Resolve(ctx, "source")
	if err == nil {
		t.Fatal("Resolve() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Resolve() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if res.Outcome == OutcomeStuck {
		t.Errorf("Resolve() outcome = %q; want no stuck verdict under a cancelled context", res.Outcome)
	}
}

func TestResolve_AlreadyUpToDate_ResolvedNoSessionNoConclude(t *testing.T) {
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{AlreadyUpToDate: true}}
	shuttle := &fakeShuttle{}
	r, err := New(newTestDeps(t, fake, shuttle))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := r.Resolve(context.Background(), "source")
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}
	if res.Outcome != OutcomeResolved || !res.AlreadyUpToDate {
		t.Errorf("Resolve() = %+v; want resolved with AlreadyUpToDate", res)
	}
	if len(shuttle.specs) != 0 {
		t.Errorf("shuttle was called %d time(s); want 0", len(shuttle.specs))
	}
	if fake.calledAny("MergeContinue") {
		t.Error("MergeContinue was called; want it never reached")
	}
}

func TestResolve_RetryUsesDistinctReportPath(t *testing.T) {
	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	shuttle := &fakeShuttle{
		results: []shuttleengine.Result{
			{Outcome: shuttleengine.OutcomeDone},
			{Outcome: shuttleengine.OutcomeDone},
		},
	}
	deps.Shuttle = shuttle
	r, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := r.Resolve(context.Background(), "source"); err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	if len(shuttle.specs) != 2 {
		t.Fatalf("shuttle was called %d time(s); want 2", len(shuttle.specs))
	}
	first := shuttle.specs[0].OutputFiles[0]
	second := shuttle.specs[1].OutputFiles[0]
	if first == second {
		t.Errorf("both attempts named the same report path %q; want distinct per-attempt paths", first)
	}
}

func TestResolve_ScratchDirAbsent_FirstSpecBuildCreatesIt(t *testing.T) {
	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	if _, err := os.Stat(deps.ScratchDir); !os.IsNotExist(err) {
		t.Fatalf("ScratchDir already exists before Resolve; test setup is invalid")
	}

	shuttle := &fakeShuttle{
		results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDone}},
		runHook: func(callNumber int, spec shuttleengine.Spec) {
			writeConflictedFixture(t, deps, "a.txt", resolvedContent)
		},
	}
	deps.Shuttle = shuttle
	r, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := r.Resolve(context.Background(), "source"); err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}
	if info, err := os.Stat(deps.ScratchDir); err != nil || !info.IsDir() {
		t.Errorf("ScratchDir was not created by the first spec build: %v", err)
	}
}
