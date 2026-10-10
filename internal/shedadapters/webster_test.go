package shedadapters

import (
	"context"
	"errors"
	"fmt"
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

func TestWebsterProducer_OutcomeStuck(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	fake := &fakeWebsterRunner{result: websterengine.RunResult{Outcome: "stuck", StuckReason: "cards ran out", BatchesDone: 3}}
	p := NewWebsterProducer("loom", fake.run, deps)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if ptr.Path != "" || ptr.Reason != "cards ran out" {
		t.Errorf("Call() pointer = %+v; want empty Path and Reason %q", ptr, "cards ran out")
	}
	if strings.Contains(strings.ToLower(ptr.Reason), "question") {
		t.Errorf("Call() pointer reason %q names a question; a stuck outcome is never reported as one", ptr.Reason)
	}
}

func TestWebsterProducer_OutcomePaused(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	fake := &fakeWebsterRunner{result: websterengine.RunResult{Outcome: "paused"}}
	p := NewWebsterProducer("loom", fake.run, deps)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "way forward: re-step the loom row") || !strings.Contains(err.Error(), "lyx webster run") {
		t.Errorf("Call() error %q does not end in the re-step way forward", err.Error())
	}

	// Taking the way forward: the pause is cleared, so the re-step proceeds.
	fake.result = websterengine.RunResult{Outcome: "done"}
	outcome, _, err := p.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Errorf("re-step Call() = (%q, %v); want Done once the pause is cleared", outcome, err)
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
			dir := t.TempDir()
			deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
			fake := &fakeWebsterRunner{err: tt.err}
			p := NewWebsterProducer("loom", fake.run, deps)

			outcome, ptr, err := p.Call(context.Background())
			if tt.wantStuck {
				if err != nil || outcome != shedengine.Stuck || ptr.Reason != tt.err.Error() {
					t.Errorf("Call() = %q, %+v, %v; want %q with the error text %q as the reason and no error", outcome, ptr, err, shedengine.Stuck, tt.err.Error())
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
		{"Paused", websterengine.RunResult{Outcome: "paused"}, nil},
		{"EngineError", websterengine.RunResult{}, &websterengine.MasterDiedError{SessionID: "s", RunDir: "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
			ctx, cancel := context.WithCancel(context.Background())
			fake := &fakeWebsterRunner{result: tt.result, err: tt.err, duringRun: cancel}
			p := NewWebsterProducer("loom", fake.run, deps)

			outcome, ptr, err := p.Call(ctx)
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
	scratchDir := t.TempDir()
	websterDir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: websterDir, ScratchDir: scratchDir}}
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeWebsterRunner{
		result:    websterengine.RunResult{Outcome: "stuck", StuckReason: "x"},
		duringRun: cancel,
	}
	p := NewWebsterProducer("loom", fake.run, deps)

	if _, _, err := p.Call(ctx); err == nil {
		t.Fatal("Call() error = nil; want the context error")
	}
	if websterengine.PauseRequested(scratchDir) {
		t.Error("PauseRequested(scratchDir) = true after a cancelled call; want false (operator's own pause channel untouched)")
	}
}
