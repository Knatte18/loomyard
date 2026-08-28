package shedadapters

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeWebsterRunner records the RunDeps/RunOptions it was handed and returns a caller-configured
// websterengine.RunResult/error. An optional duringRun hook lets a test cancel the context (or
// otherwise act) as if it happened mid-run, mirroring fakeShuttle's own hook in singlellm_test.go.
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

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
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
}

func TestWebsterProducer_OutcomeStuck(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	fake := &fakeWebsterRunner{result: websterengine.RunResult{Outcome: "stuck", StuckReason: "cards ran out", BatchesDone: 3}}
	p := NewWebsterProducer("loom", fake.run, deps)

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
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
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	askingErr := &websterengine.MasterAskingError{SessionID: "sess-1", RunDir: "/tmp/run", Message: "which model?"}
	fake := &fakeWebsterRunner{err: askingErr}
	p := NewWebsterProducer("loom", fake.run, deps)

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil (asking maps to Stuck)", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
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

func TestWebsterProducer_MasterAskingMatchedViaErrorsIs(t *testing.T) {
	dir := t.TempDir()
	deps := websterengine.RunDeps{Geom: websterengine.Geometry{WebsterDir: dir}}
	// Wrapped with %w so the adapter must use errors.Is against ErrMasterAsking, not a string match.
	wrapped := &websterengine.MasterAskingError{SessionID: "sess-1", RunDir: "/tmp/run", Message: "hmm"}
	fake := &fakeWebsterRunner{err: wrapped}
	p := NewWebsterProducer("loom", fake.run, deps)

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
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

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
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
