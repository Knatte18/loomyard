package mergeresolve

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// fakeMergeSurface implements MergeSurface, recording call order and returning caller-configured
// results/errors.
type fakeMergeSurface struct {
	mergeInResult      fabricengine.MergeResult
	mergeInErr         error
	mergeInProgress    bool
	mergeInProgressErr error
	stageResolvedErr   error
	stageTrackedErr    error
	untrackedFiles     []string
	untrackedErr       error
	continueErr        error
	abortErr           error

	calls       []string
	stagedPaths []string
}

func (f *fakeMergeSurface) MergeStageTracked() (fabricengine.StageResult, error) {
	f.calls = append(f.calls, "MergeStageTracked")
	return fabricengine.StageResult{}, f.stageTrackedErr
}

func (f *fakeMergeSurface) MergeUntrackedFiles() ([]string, error) {
	f.calls = append(f.calls, "MergeUntrackedFiles")
	return f.untrackedFiles, f.untrackedErr
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

// conflictStencilFixture is a minimal, valid conflict stencil carrying exactly the markers
// buildConflictSpec fills.
const conflictStencilFixture = "# Conflict\n\n{{.parent_directive}}\n{{.edit_directive}}\n\nPaths:\n{{.conflicted_paths}}\n\nReport: {{.report_path}}\n"

// newTestDeps returns a Deps wired against fake, shuttle, and a fresh worktree/scratch/stencils
// directory tree under t.TempDir(), with the conflict stencil fixture seeded.
func newTestDeps(t *testing.T, fake *fakeMergeSurface, shuttle *shedfake.MergeShuttle) Deps {
	t.Helper()

	root := t.TempDir()
	worktreeRoot := filepath.Join(root, "worktree")
	scratchDir := filepath.Join(root, "scratch")
	stencilsDir := filepath.Join(root, "stencils", "landing")

	if err := os.MkdirAll(worktreeRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(worktreeRoot): %v", err)
	}
	stencilkit.SeedInto(t, filepath.Dir(stencilsDir))
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
	t.Parallel()

	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: nil}}
	shuttle := &shedfake.MergeShuttle{}
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
	if len(shuttle.Specs) != 0 {
		t.Errorf("shuttle was called %d time(s); want 0 for a clean merge", len(shuttle.Specs))
	}
}

func TestResolve_ConflictThenCleanScan_StagesThenConcludes(t *testing.T) {
	t.Parallel()

	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	shuttle := &shedfake.MergeShuttle{
		Results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDone}},
		DuringRun: func(callNumber int, spec shuttleengine.Spec) {
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
	t.Parallel()

	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	shuttle := &shedfake.MergeShuttle{
		Results: []shuttleengine.Result{
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
	if len(shuttle.Specs) != 2 {
		t.Errorf("shuttle was called %d time(s); want exactly one retry (2 total)", len(shuttle.Specs))
	}
	if !fake.calledAny("MergeAbort") {
		t.Error("MergeAbort was never called")
	}
	if fake.calledAny("MergeContinue") {
		t.Error("MergeContinue was called; want it never reached")
	}
}

func TestResolve_MergeInProgressAtEntry_AbortsBeforeNewAttempt(t *testing.T) {
	t.Parallel()

	fake := &fakeMergeSurface{
		mergeInProgress: true,
		mergeInResult:   fabricengine.MergeResult{Conflicts: nil},
	}
	shuttle := &shedfake.MergeShuttle{}
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

// unrecognizedMergeError is a typed error MergeIn's own disposition table does not name explicitly,
// proving the catch-all default is escalate rather than fall through unhandled.
type unrecognizedMergeError struct{}

func (unrecognizedMergeError) Error() string { return "unrecognized merge failure" }

// TestResolve_MergeInError_StuckWithReasonNoAbort asserts every error MergeIn returns -- a foreign merge state, an unmergeable state, and one its disposition table does not name -- ends stuck with the error surfaced, without aborting a merge the run does not own.
func TestResolve_MergeInError_StuckWithReasonNoAbort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{"foreign merge state", &fabricengine.ErrForeignMergeState{}},
		{"unmergeable state", &fabricengine.ErrUnmergeableState{}},
		{"unrecognized error hits the catch-all", unrecognizedMergeError{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeMergeSurface{mergeInErr: tt.err}
			r, err := New(newTestDeps(t, fake, &shedfake.MergeShuttle{}))
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
				t.Error("MergeAbort was called; want it never reached for a MergeIn error")
			}
		})
	}
}

// Not parallel: logcapture.Capture replaces the process-global logger.
func TestResolve_ShuttleOutcomes_MapToStuckNoConclude(t *testing.T) {
	tests := []struct {
		name    string
		outcome shuttleengine.Outcome
	}{
		{"Died", shuttleengine.OutcomeDied},
		{"Timeout", shuttleengine.OutcomeTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := []string{"a.txt"}
			fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
			deps := newTestDeps(t, fake, nil)
			writeConflictedFixture(t, deps, "a.txt", conflictedContent)

			shuttle := &shedfake.MergeShuttle{Results: []shuttleengine.Result{{Outcome: tt.outcome, SessionID: "session-1", RunDir: "/run/dir"}}}
			deps.Shuttle = shuttle
			r, err := New(deps)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			buf := logcapture.Capture(t)

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

// TestResolve_ShuttleOutcomes_WarnSurvivesAbortFailure pins the placement guarantee: the Warn line lands before abortAndStuck's own MergeAbort call, so a subsequent MergeAbort failure does not erase the diagnostic.
// Not parallel: logcapture.Capture replaces the process-global logger.
func TestResolve_ShuttleOutcomes_WarnSurvivesAbortFailure(t *testing.T) {
	paths := []string{"a.txt"}
	abortErr := errors.New("abort failed")
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}, abortErr: abortErr}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	shuttle := &shedfake.MergeShuttle{Results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDied, SessionID: "session-1", RunDir: "/run/dir"}}}
	deps.Shuttle = shuttle
	r, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	buf := logcapture.Capture(t)

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
	t.Parallel()

	fake := &fakeMergeSurface{}
	shuttle := &shedfake.MergeShuttle{}
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
	t.Parallel()

	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{AlreadyUpToDate: true}}
	shuttle := &shedfake.MergeShuttle{}
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
	if len(shuttle.Specs) != 0 {
		t.Errorf("shuttle was called %d time(s); want 0", len(shuttle.Specs))
	}
	if fake.calledAny("MergeContinue") {
		t.Error("MergeContinue was called; want it never reached")
	}
}

func TestResolve_RetryUsesDistinctReportPath(t *testing.T) {
	t.Parallel()

	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	shuttle := &shedfake.MergeShuttle{
		Results: []shuttleengine.Result{
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

	if len(shuttle.Specs) != 2 {
		t.Fatalf("shuttle was called %d time(s); want 2", len(shuttle.Specs))
	}
	first := shuttle.Specs[0].OutputFiles[0]
	second := shuttle.Specs[1].OutputFiles[0]
	if first == second {
		t.Errorf("both attempts named the same report path %q; want distinct per-attempt paths", first)
	}
}

func TestResolve_ScratchDirAbsent_FirstSpecBuildCreatesIt(t *testing.T) {
	t.Parallel()

	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	if _, err := os.Stat(deps.ScratchDir); !os.IsNotExist(err) {
		t.Fatalf("ScratchDir already exists before Resolve; test setup is invalid")
	}

	shuttle := &shedfake.MergeShuttle{
		Results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDone}},
		DuringRun: func(callNumber int, spec shuttleengine.Spec) {
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

func TestResolve_SessionEditsTrackedFile_StagesTrackedBeforeConclude(t *testing.T) {
	t.Parallel()

	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)
	writeConflictedFixture(t, deps, "other.txt", "before\n")

	shuttle := &shedfake.MergeShuttle{
		Results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDone}},
		DuringRun: func(callNumber int, spec shuttleengine.Spec) {
			writeConflictedFixture(t, deps, "a.txt", resolvedContent)
			writeConflictedFixture(t, deps, "other.txt", "after\n")
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

	resolvedIdx := fake.indexOf("MergeStageResolved")
	trackedIdx := fake.indexOf("MergeStageTracked")
	continueIdx := fake.indexOf("MergeContinue")
	if resolvedIdx == -1 || trackedIdx < resolvedIdx || continueIdx < trackedIdx {
		t.Errorf("call order = %v; want MergeStageResolved, MergeStageTracked, MergeContinue in that order", fake.calls)
	}
}

func TestResolve_SessionLeavesUntrackedFile_AbortsStuckNamingItWithoutStaging(t *testing.T) {
	t.Parallel()

	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{
		mergeInResult:  fabricengine.MergeResult{Conflicts: paths},
		untrackedFiles: []string{"new/one.go", "two.go"},
	}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	shuttle := &shedfake.MergeShuttle{
		Results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDone}},
		DuringRun: func(callNumber int, spec shuttleengine.Spec) {
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
	if res.Outcome != OutcomeStuck {
		t.Errorf("Resolve() outcome = %q; want %q", res.Outcome, OutcomeStuck)
	}
	for _, name := range fake.untrackedFiles {
		if !strings.Contains(res.Reason, name) {
			t.Errorf("Reason = %q; want it to name %q", res.Reason, name)
		}
	}
	if !fake.calledAny("MergeAbort") {
		t.Error("MergeAbort was never called")
	}
	for _, verb := range []string{"MergeStageResolved", "MergeStageTracked", "MergeContinue"} {
		if fake.calledAny(verb) {
			t.Errorf("%s was called; want it never reached once an untracked file halts the merge", verb)
		}
	}
}

func TestResolve_StaleReportFromEarlierCall_IsClearedAtEntry(t *testing.T) {
	t.Parallel()

	paths := []string{"a.txt"}
	fake := &fakeMergeSurface{mergeInResult: fabricengine.MergeResult{Conflicts: paths}}
	deps := newTestDeps(t, fake, nil)
	writeConflictedFixture(t, deps, "a.txt", conflictedContent)

	stale := filepath.Join(deps.ScratchDir, reportNamePrefix+"1.md")
	if err := os.MkdirAll(deps.ScratchDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(ScratchDir): %v", err)
	}
	if err := os.WriteFile(stale, []byte("an earlier call's report\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale report): %v", err)
	}

	var staleAtRun bool
	shuttle := &shedfake.MergeShuttle{
		Results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDone}},
		DuringRun: func(callNumber int, spec shuttleengine.Spec) {
			_, err := os.Stat(stale)
			staleAtRun = err == nil
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
	if staleAtRun {
		t.Error("the earlier call's report still existed when the session ran; want it cleared at entry")
	}
}

// TestBuildConflictSpec_SkillsAndParentDirective covers the conflict session's skill list and its parent directive, with and without a parent.
func TestBuildConflictSpec_SkillsAndParentDirective(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		parentName string
		want       string
	}{
		{"with parent", "ab:cd:webster", "ab:cd:webster"},
		{"no parent", "", "No parent is recorded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := newTestDeps(t, &fakeMergeSurface{}, &shedfake.MergeShuttle{})
			deps.ParentName = tt.parentName

			spec, err := buildConflictSpec(deps, []string{"a.txt"}, 1)
			if err != nil {
				t.Fatalf("buildConflictSpec error = %v; want nil", err)
			}
			if !strings.Contains(spec.Prompt, tt.want) {
				t.Errorf("Prompt = %q; want it to contain %q", spec.Prompt, tt.want)
			}
			if !strings.Contains(spec.Prompt, "Edit or Write") {
				t.Errorf("Prompt = %q; want it to contain the edit directive's \"Edit or Write\" sentence", spec.Prompt)
			}
			if tt.parentName == "" && strings.Contains(spec.Prompt, "Your parent is") {
				t.Errorf("Prompt = %q; want the no-parent variant", spec.Prompt)
			}
			if got := strings.Join(spec.Skills, ","); got != "scribe:prose,scribe:code-quality" {
				t.Errorf("Skills = %q; want scribe:prose,scribe:code-quality", got)
			}
			if spec.Segment != segmentcolor.Landing {
				t.Errorf("Segment = %q; want %q", spec.Segment, segmentcolor.Landing)
			}
		})
	}
}
