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

func TestWebsterProducer_OutcomeDone(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	fake := &fakeWebsterRunner{result: websterengine.RunResult{Outcome: "done"}}
	p := NewWebsterProducer("loom", fake.run, deps)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Done)
	wantPath := summaryparser.Path(dir)
	if ptr.Path != wantPath {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPath)
	}
	if fake.calls != 1 {
		t.Errorf("runner calls = %d; want 1", fake.calls)
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

func TestWebsterProducer_MasterAskingError(t *testing.T) {
	tests := []struct {
		name       string
		askingErr  *websterengine.MasterAskingError
		wantReason string
	}{
		{"SessionAndRunDir", &websterengine.MasterAskingError{SessionID: "sess-1", RunDir: "/tmp/run", Message: "which model?"}, "webster master is asking a question; session sess-1, run dir /tmp/run"},
		{"EmptySessionIDNamesRunDir", &websterengine.MasterAskingError{RunDir: "/tmp/run", Message: "which model?"}, "webster master is asking a question; run dir /tmp/run"},
		{"NeitherIsBareText", &websterengine.MasterAskingError{Message: "which model?"}, "webster master is asking a question"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
			fake := &fakeWebsterRunner{err: tt.askingErr}
			p := NewWebsterProducer("loom", fake.run, deps)

			ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
			if ptr.Path != "" {
				t.Errorf("Call() pointer.Path = %q; want empty", ptr.Path)
			}
			if ptr.Reason != tt.wantReason {
				t.Errorf("Call() Reason = %q; want %q", ptr.Reason, tt.wantReason)
			}
			if strings.Contains(ptr.Reason, "which model?") {
				t.Errorf("Call() Reason %q contains the master's message", ptr.Reason)
			}
		})
	}
}

func TestWebsterProducer_OtherEngineErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"MasterDied", &websterengine.MasterDiedError{SessionID: "sess-1", RunDir: "/tmp/run"}},
		{"MasterTimeout", &websterengine.MasterTimeoutError{SessionID: "sess-1", RunDir: "/tmp/run"}},
		{"RunBusy", websterengine.ErrRunBusy},
		{"FingerprintMismatch", websterengine.ErrFingerprintMismatch},
		{"NilBatcher", websterengine.ErrNilBatcher},
		{"PlainUnmatchedError", errors.New("webster: plan validation refused this run")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
			fake := &fakeWebsterRunner{err: tt.err}
			p := NewWebsterProducer("loom", fake.run, deps)

			outcome, _, err := p.Call(context.Background())
			if err == nil {
				t.Fatal("Call() error = nil; want non-nil")
			}
			if outcome == shedengine.Stuck {
				t.Errorf("Call() outcome = %q; want the error, not %q (only the asking sentinel maps to Stuck)", outcome, shedengine.Stuck)
			}
		})
	}
}

func pendingAuditErr() error {
	return fmt.Errorf("%w: 1 correctness finding(s): 1) parent-write: wrote internal/x.go; way forward: 1) lyx webster accept-audit; 2) re-step the loom row", websterengine.ErrPendingAuditFindings)
}

// TestNewWebsterProducer_ReentryStepNamesTheRow proves the adapter hands the run its row's re-entry step, and keeps one a caller set.
func TestNewWebsterProducer_ReentryStepNamesTheRow(t *testing.T) {
	fake := &fakeWebsterRunner{err: pendingAuditErr()}
	p := NewWebsterProducer("loom", fake.run, websterengine.RunDeps{})
	shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if got := fake.gotDeps.ReentryStep; got != "re-step the loom row" {
		t.Errorf("ReentryStep = %q; want the row's re-entry step", got)
	}

	fake = &fakeWebsterRunner{err: pendingAuditErr()}
	p = NewWebsterProducer("loom", fake.run, websterengine.RunDeps{ReentryStep: "lyx webster run"})
	shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if got := fake.gotDeps.ReentryStep; got != "lyx webster run" {
		t.Errorf("ReentryStep = %q; want the caller's step kept", got)
	}
}

func TestWebsterProducer_PendingAuditFindingsIsStuck(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
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

func TestWebsterProducer_MasterAskingMatchedViaErrorsIs(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	// Wrapped with %w so the adapter must use errors.Is against ErrMasterAsking, not a string match.
	wrapped := &websterengine.MasterAskingError{SessionID: "sess-1", RunDir: "/tmp/run", Message: "hmm"}
	fake := &fakeWebsterRunner{err: wrapped}
	p := NewWebsterProducer("loom", fake.run, deps)

	shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if !errors.Is(wrapped, websterengine.ErrMasterAsking) {
		t.Fatalf("test setup: MasterAskingError does not satisfy errors.Is(_, ErrMasterAsking)")
	}
}

// --- RunOptions.Fresh safety property ---

func TestWebsterProducer_FreshIsAlwaysFalse(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	fake := &fakeWebsterRunner{result: websterengine.RunResult{Outcome: "done"}}
	p := NewWebsterProducer("loom", fake.run, deps)

	shedfake.CallOK(t, p)
	if fake.gotOptions.Fresh {
		t.Error("Call() invoked the run seam with RunOptions.Fresh = true; want false (a safety property, not a default)")
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

func TestWebsterProducer_CancelledDuringRun_OutcomeDoneStillSucceeds(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeWebsterRunner{
		result:    websterengine.RunResult{Outcome: "done"},
		duringRun: cancel,
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
