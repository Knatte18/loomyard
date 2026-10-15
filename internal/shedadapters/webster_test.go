package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeWebsterRunner records the RunDeps/RunOptions it was handed and returns a caller-configured
// websterengine.RunResult/error. An optional duringRun hook lets a test cancel the context (or
// otherwise act) as if it happened mid-run, mirroring shedfake.Shuttle's own hook.
type fakeWebsterRunner struct {
	result websterengine.RunResult
	err    error

	calls      int
	gotDeps    websterengine.RunDeps
	gotOptions websterengine.RunOptions
	duringRun  func()
}

func (f *fakeWebsterRunner) run(deps websterengine.RunDeps, opts websterengine.RunOptions) (websterengine.RunResult, error) {
	f.calls++
	f.gotDeps = deps
	f.gotOptions = opts
	if f.duringRun != nil {
		f.duringRun()
	}
	return f.result, f.err
}

// --- Outcome mapping table ---

// TestWebsterProducer_OutcomeDone covers a completed run, which survives a cancellation that
// arrives during the run. The run is never asked for a fresh start: RunOptions.Fresh is false as a
// safety property, not a default.
//
//testtiming:keep pins that a done run returns the summary as pointer, is never asked for a fresh start, and survives a cancellation during the run
func TestWebsterProducer_OutcomeDone(t *testing.T) {
	tests := []struct {
		name            string
		cancelDuringRun bool
	}{
		{"Plain", false},
		{"SurvivesCancellationDuringRun", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fake := &fakeWebsterRunner{result: websterengine.RunResult{Outcome: "done"}}
			if tt.cancelDuringRun {
				fake.duringRun = cancel
			}
			p := NewWebsterProducer("loom", fake.run, deps)

			outcome, ptr, err := p.Call(ctx)
			if err != nil {
				t.Fatalf("Call() error = %v; want nil (genuine success survives cancellation)", err)
			}
			if outcome != shedengine.Done {
				t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
			}
			wantPath := summaryparser.Path(dir)
			if ptr.Path != wantPath {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPath)
			}
			if fake.calls != 1 {
				t.Errorf("runner calls = %d; want 1", fake.calls)
			}
			if fake.gotOptions.Fresh {
				t.Error("Call() invoked the run seam with RunOptions.Fresh = true; want false (a safety property, not a default)")
			}
			if !fake.gotOptions.AutoRebaseline {
				t.Error("Call() invoked the run seam with RunOptions.AutoRebaseline = false; want true")
			}
		})
	}
}

// websterDepsWithScratch returns RunDeps whose scratch directory is a fresh temporary one, so a stuck outcome's conflict claim never resolves against the test's working directory.
func websterDepsWithScratch(t *testing.T) websterengine.RunDeps {
	t.Helper()
	return websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: t.TempDir(), ScratchDir: t.TempDir()}}
}

// seedWebsterConflictRecord writes a conflict record naming findings into deps' scratch directory.
func seedWebsterConflictRecord(t *testing.T, deps websterengine.RunDeps, findings ...string) {
	t.Helper()
	conflict := &websterengine.PlanConflictError{Batch: 1, Cause: websterengine.ConflictCausePlanDefect, Findings: findings}
	if err := websterengine.WriteConflictRecord(deps.Geom.ScratchDir, conflict); err != nil {
		t.Fatalf("WriteConflictRecord() error = %v", err)
	}
}

// writeWebsterFile writes content to filePath, failing the test on error.
func writeWebsterFile(t *testing.T, filePath, content string) {
	t.Helper()
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", filePath, err)
	}
}

// requireWebsterFile fails the test unless filePath holds exactly want.
func requireWebsterFile(t *testing.T, filePath, want string) {
	t.Helper()
	got, err := os.ReadFile(filePath)
	if err != nil || string(got) != want {
		t.Errorf("%s = %q, %v; want %q", filePath, got, err, want)
	}
}

// TestWebsterProducer_OutcomeStuck proves a stuck outcome maps by the conflict record it follows:
// no record is a plain Stuck with the stuck reason, a new conflict is a Reroute, and a repeated one, a record naming no finding or an unreadable or unwritable file is a plain Stuck.
func TestWebsterProducer_OutcomeStuck(t *testing.T) {
	const stuckReason = "cards ran out"
	tests := []struct {
		name string
		// seed shapes the scratch directory before the run.
		seed func(t *testing.T, scratch string, deps websterengine.RunDeps)
		// wantReroute is whether the pointer asks for a reroute.
		wantReroute bool
		// wantReason is the reason in full, or "" to check wantReasonHas and wantReasonLacks instead.
		wantReason     string
		wantReasonHas  []string
		wantReasonLack []string
		// wantRouted is the routed file's content afterwards, "" meaning it must be absent.
		wantRouted string
	}{
		{name: "no record is a plain stuck", wantReason: stuckReason},
		{
			name: "a new record reroutes and leaves the routed file holding its findings",
			seed: func(t *testing.T, _ string, deps websterengine.RunDeps) {
				seedWebsterConflictRecord(t, deps, "finding-a", "finding-b")
			},
			wantReroute:    true,
			wantReasonHas:  []string{stuckReason, "lyx loom resume", "pending cards"},
			wantReasonLack: []string{"plan review took place", "plan review found"},
			wantRouted:     "- finding-a\n- finding-b\n",
		},
		{
			name: "a record equal to the routed findings is a repeat",
			seed: func(t *testing.T, scratch string, deps websterengine.RunDeps) {
				seedWebsterConflictRecord(t, deps, "finding-a", "finding-b")
				writeWebsterFile(t, websterengine.RoutedConflictPath(scratch), "- finding-a\n- finding-b\n")
			},
			wantReasonHas:  []string{"the same webster plan conflict recurred", stuckReason, "lyx loom resume"},
			wantReasonLack: []string{"review"},
			wantRouted:     "- finding-a\n- finding-b\n",
		},
		{
			name: "a record with the routed findings reordered is a repeat",
			seed: func(t *testing.T, scratch string, deps websterengine.RunDeps) {
				seedWebsterConflictRecord(t, deps, "finding-b", "finding-a")
				writeWebsterFile(t, websterengine.RoutedConflictPath(scratch), "- finding-a\n- finding-b\n")
			},
			wantReasonHas: []string{"the same webster plan conflict recurred"},
			wantRouted:    "- finding-a\n- finding-b\n",
		},
		{
			name: "a record differing from the routed findings reroutes and rewrites the routed file",
			seed: func(t *testing.T, scratch string, deps websterengine.RunDeps) {
				seedWebsterConflictRecord(t, deps, "finding-c")
				writeWebsterFile(t, websterengine.RoutedConflictPath(scratch), "- finding-a\n")
			},
			wantReroute:   true,
			wantReasonHas: []string{stuckReason},
			wantRouted:    "- finding-c\n",
		},
		{
			name: "a record with a heading and no finding line is a plain stuck",
			seed: func(t *testing.T, scratch string, _ websterengine.RunDeps) {
				writeWebsterFile(t, websterengine.ConflictRecordPath(scratch), "# Plan conflict at begin-batch 01\ncause: plan_defect\n\n")
			},
			wantReason: stuckReason,
		},
		{
			name: "a record path that is a directory is a plain stuck",
			seed: func(t *testing.T, scratch string, _ websterengine.RunDeps) {
				if err := os.MkdirAll(filepath.Join(websterengine.ConflictRecordPath(scratch), "occupant"), 0o755); err != nil {
					t.Fatalf("occupy the record path: %v", err)
				}
			},
			wantReason: stuckReason,
		},
		{
			name: "a new record whose routed path is a non-empty directory is a plain stuck",
			seed: func(t *testing.T, scratch string, deps websterengine.RunDeps) {
				seedWebsterConflictRecord(t, deps, "finding-a")
				if err := os.MkdirAll(filepath.Join(websterengine.RoutedConflictPath(scratch), "occupant"), 0o755); err != nil {
					t.Fatalf("occupy the routed path: %v", err)
				}
			},
			wantReason: stuckReason,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := websterDepsWithScratch(t)
			scratch := deps.Geom.ScratchDir
			if tt.seed != nil {
				tt.seed(t, scratch, deps)
			}
			fake := &fakeWebsterRunner{result: websterengine.RunResult{Outcome: "stuck", StuckReason: stuckReason, BatchesDone: 3}}
			p := NewWebsterProducer("loom", fake.run, deps)

			ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)

			if ptr.Path != "" || ptr.Reroute != tt.wantReroute {
				t.Errorf("Call() pointer = %+v; want empty Path and Reroute %v", ptr, tt.wantReroute)
			}
			if tt.wantReason != "" && ptr.Reason != tt.wantReason {
				t.Errorf("Call() Reason = %q; want %q", ptr.Reason, tt.wantReason)
			}
			for _, want := range tt.wantReasonHas {
				if !strings.Contains(ptr.Reason, want) {
					t.Errorf("Call() Reason = %q; want it to contain %q", ptr.Reason, want)
				}
			}
			for _, lack := range tt.wantReasonLack {
				if strings.Contains(ptr.Reason, lack) {
					t.Errorf("Call() Reason = %q; want it not to contain %q", ptr.Reason, lack)
				}
			}
			if strings.Contains(strings.ToLower(ptr.Reason), "question") {
				t.Errorf("Call() pointer reason %q names a question; a stuck outcome is never reported as one", ptr.Reason)
			}
			routedPath := websterengine.RoutedConflictPath(scratch)
			if tt.wantRouted != "" {
				requireWebsterFile(t, routedPath, tt.wantRouted)
			} else if info, err := os.Stat(routedPath); err == nil && !info.IsDir() {
				t.Errorf("routed file %s exists; want it absent", routedPath)
			}
		})
	}
}

func TestWebsterProducer_UnrecognizedOutcome(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	fake := &fakeWebsterRunner{result: websterengine.RunResult{Outcome: "WEIRD"}}
	p := NewWebsterProducer("loom", fake.run, deps)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "WEIRD") {
		t.Errorf("Call() error %q does not quote the unrecognised value", err.Error())
	}
}

// --- Error mapping table ---

func TestWebsterProducer_OtherEngineErrors(t *testing.T) {
	autoRebaselineErr := fmt.Errorf("%w: %w; way forward: transient, re-step the loom row", websterengine.ErrAutoRebaseline, errors.New("save the restamped state"))
	tests := []struct {
		name string
		err  error
		// wantStuck marks a sentinel that blocks the row resumable, with the error text as the reason.
		wantStuck bool
	}{
		{name: "MasterDied", err: &websterengine.MasterDiedError{SessionID: "sess-1", RunDir: "/tmp/run"}},
		{name: "MasterTimeout", err: &websterengine.MasterTimeoutError{SessionID: "sess-1", RunDir: "/tmp/run"}},
		{name: "RunBusy", err: websterengine.ErrRunBusy},
		{name: "FingerprintMismatch", err: websterengine.ErrFingerprintMismatch},
		{name: "NilBatcher", err: websterengine.ErrNilBatcher},
		{name: "PlainUnmatchedError", err: errors.New("webster: plan validation refused this run")},
		{name: "AutoRebaselineWrap", err: autoRebaselineErr, wantStuck: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := websterDepsWithScratch(t)
			seedWebsterConflictRecord(t, deps, "finding-a")
			fake := &fakeWebsterRunner{err: tt.err}
			p := NewWebsterProducer("loom", fake.run, deps)

			outcome, ptr, err := p.Call(context.Background())
			if _, statErr := os.Stat(websterengine.RoutedConflictPath(deps.Geom.ScratchDir)); !os.IsNotExist(statErr) {
				t.Errorf("stat routed file = %v; want it absent, since an error never consults the record", statErr)
			}
			if tt.wantStuck {
				if err != nil || outcome != shedengine.Stuck || ptr.Reason != tt.err.Error() || ptr.Reroute {
					t.Errorf("Call() = %q, %+v, %v; want %q with the error text %q as the reason, no reroute and no error", outcome, ptr, err, shedengine.Stuck, tt.err.Error())
				}
				return
			}
			if err == nil {
				t.Fatal("Call() error = nil; want non-nil")
			}
			if outcome == shedengine.Stuck {
				t.Errorf("Call() outcome = %q; want the error, not %q (only the pending-audit-findings and auto-rebaseline sentinels map to Stuck)", outcome, shedengine.Stuck)
			}
		})
	}
}

func pendingAuditErr() error {
	return fmt.Errorf("%w: 1 correctness finding(s): 1) parent-write: wrote internal/x.go; way forward: 1) lyx webster accept-audit; 2) re-step the loom row", websterengine.ErrPendingAuditFindings)
}

// TestWebsterProducer_PendingAuditFindingsIsStuck covers a pending-audit refusal: it is a Stuck
// with the refusal's own text as reason and no path, and the adapter hands the run its row's
// re-entry step while keeping one a caller set.
//
//testtiming:keep pins the pending-audit refusal's reason, which is the refusal's own text, and the re-entry step the run is handed
func TestWebsterProducer_PendingAuditFindingsIsStuck(t *testing.T) {
	tests := []struct {
		name            string
		reentryStep     string
		wantReentryStep string
	}{
		{"RowReentryStepIsHandedToTheRun", "", "re-step the loom row"},
		{"CallersStepIsKept", "lyx webster run", "lyx webster run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}, ReentryStep: tt.reentryStep}
			fake := &fakeWebsterRunner{err: pendingAuditErr()}
			p := NewWebsterProducer("loom", fake.run, deps)

			ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
			if ptr.Path != "" {
				t.Errorf("Call() path = %q; want empty", ptr.Path)
			}
			for _, want := range []string{"internal/x.go", "lyx webster accept-audit", "re-step the loom row"} {
				if !strings.Contains(ptr.Reason, want) {
					t.Errorf("Reason = %q; want it to contain %q", ptr.Reason, want)
				}
			}
			if ptr.Reason != pendingAuditErr().Error() {
				t.Errorf("Reason = %q; want the refusal's own text, with no step appended", ptr.Reason)
			}
			if got := fake.gotDeps.ReentryStep; got != tt.wantReentryStep {
				t.Errorf("ReentryStep = %q; want %q", got, tt.wantReentryStep)
			}
		})
	}
}

func TestWebsterProducer_PendingAuditFindingsBlocksRun(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.lock")
	err := state.UpdateJSON(statusPath, statusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
		cur.CurrentProducer = "Webster"
		cur.State = shedengine.StateRunning
		return cur, nil
	})
	if err != nil {
		t.Fatalf("seed status: %v", err)
	}
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	fake := &fakeWebsterRunner{err: pendingAuditErr()}
	shed := &shedengine.Shed{
		Producers:      []shedengine.ProducerDef{{Name: "Webster", Producer: NewWebsterProducer("Webster", fake.run, deps)}},
		StatusPath:     statusPath,
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: statusLockPath,
	}

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step() error = %v; want nil", err)
	}
	if res.State != shedengine.StateBlocked {
		t.Errorf("Step() state = %q; want %q", res.State, shedengine.StateBlocked)
	}
	if !strings.Contains(res.Reason, "lyx webster accept-audit") {
		t.Errorf("Step() reason = %q; want it to name the accept-audit verb", res.Reason)
	}

	got, _, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if got.State != shedengine.StateBlocked || got.CurrentProducer != "Webster" {
		t.Errorf("status = %q at %q; want blocked at Webster", got.State, got.CurrentProducer)
	}
	if got.Error != res.Reason {
		t.Errorf("status error = %q; want %q", got.Error, res.Reason)
	}
}

// --- Context rows ---

func TestWebsterProducer_AlreadyCancelledContext(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	fake := &fakeWebsterRunner{result: websterengine.RunResult{Outcome: "done"}}
	p := NewWebsterProducer("loom", fake.run, deps)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if fake.calls != 0 {
		t.Errorf("runner calls = %d; want 0 (seam never invoked)", fake.calls)
	}
}

func TestWebsterProducer_CancelledDuringRun_StuckPausedAndErrorBecomeContextError(t *testing.T) {
	tests := []struct {
		name   string
		result websterengine.RunResult
		err    error
	}{
		{"Stuck", websterengine.RunResult{Outcome: "stuck", StuckReason: "x"}, nil},
		{"EngineError", websterengine.RunResult{}, &websterengine.MasterDiedError{SessionID: "s", RunDir: "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := websterDepsWithScratch(t)
			seedWebsterConflictRecord(t, deps, "finding-a")
			ctx, cancel := context.WithCancel(context.Background())
			fake := &fakeWebsterRunner{result: tt.result, err: tt.err, duringRun: cancel}
			p := NewWebsterProducer("loom", fake.run, deps)

			outcome, ptr, err := p.Call(ctx)
			if _, statErr := os.Stat(websterengine.RoutedConflictPath(deps.Geom.ScratchDir)); !os.IsNotExist(statErr) {
				t.Errorf("stat routed file = %v; want it absent, since the cancellation check precedes the claim", statErr)
			}
			if err == nil {
				t.Fatal("Call() error = nil; want the context error")
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
			}
			if outcome == shedengine.Stuck {
				t.Errorf("Call() outcome = %q; want the cancellation error, not %q", outcome, shedengine.Stuck)
			}
			if ptr != (shedengine.OutputPointer{}) {
				t.Errorf("Call() pointer = %+v; want empty", ptr)
			}
		})
	}
}

// --- No mid-run bridge ---

func TestWebsterProducer_NoBridgeInstalled(t *testing.T) {
	deps := websterDepsWithScratch(t)
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeWebsterRunner{
		result:    websterengine.RunResult{Outcome: "stuck", StuckReason: "x"},
		duringRun: cancel,
	}
	p := NewWebsterProducer("loom", fake.run, deps)

	if _, _, err := p.Call(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
}

// doneProducer is a shedengine.ShedProducer that always returns Done.
type doneProducer struct{}

func (doneProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// TestWebsterProducer_ConflictRerouteCapThenRepeatHalts proves a new plan conflict that meets shed's reroute cap halts with the told clause and counts as routed.
// Resuming the halt without the hand edit halts again as a repeat and adds no reroute.
func TestWebsterProducer_ConflictRerouteCapThenRepeatHalts(t *testing.T) {
	const capWayForward = "run \"lyx loom resume\" in the task worktree once the named pending cards are edited by hand"
	deps := websterDepsWithScratch(t)
	seedWebsterConflictRecord(t, deps, "finding-a")
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.lock")
	err := state.UpdateJSON(statusPath, statusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
		cur.CurrentProducer = "Webster"
		cur.State = shedengine.StateRunning
		for range 3 {
			cur.History = append(cur.History, shedengine.HistoryEntry{Producer: "Webster", Outcome: shedengine.OutcomeReroute, RoutedTo: "Review"})
		}
		return cur, nil
	})
	if err != nil {
		t.Fatalf("seed status: %v", err)
	}
	fake := &fakeWebsterRunner{result: websterengine.RunResult{Outcome: "stuck", StuckReason: "batch 2 refused"}}
	shed := &shedengine.Shed{
		Producers: []shedengine.ProducerDef{
			{Name: "Webster", Producer: NewWebsterProducer("Webster", fake.run, deps), OnReroute: "Review"},
			{Name: "Review", Producer: doneProducer{}},
		},
		StatusPath:           statusPath,
		LockPath:             filepath.Join(dir, "run.lock"),
		StatusLockPath:       statusLockPath,
		RerouteCapWayForward: capWayForward,
	}

	first, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run() error = %v; want nil", err)
	}
	if first.Outcome != shedengine.RunBlocked || !strings.Contains(first.Reason, "reroute cap reached") || !strings.Contains(first.Reason, capWayForward) {
		t.Fatalf("first Run() = %q, %q; want blocked with the reroute cap reason and the told clause", first.Outcome, first.Reason)
	}
	requireWebsterFile(t, websterengine.RoutedConflictPath(deps.Geom.ScratchDir), "- finding-a\n")

	second, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run() error = %v; want nil", err)
	}
	if second.Outcome != shedengine.RunBlocked || !strings.Contains(second.Reason, "the same webster plan conflict recurred") {
		t.Fatalf("second Run() = %q, %q; want blocked with the repeated-conflict reason", second.Outcome, second.Reason)
	}
	reroutes := 0
	for _, entry := range second.History {
		if entry.Outcome == shedengine.OutcomeReroute {
			reroutes++
		}
	}
	if reroutes != 3 {
		t.Errorf("reroute entries = %d; want the 3 seeded ones, with none added by the capped or repeated runs", reroutes)
	}
}
