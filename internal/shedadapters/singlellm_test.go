package shedadapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeShuttle records the Spec it was handed and returns a caller-configured Result/error.
// An optional duringRun hook lets a test cancel the context (or otherwise act) as if it happened
// mid-run, before Run returns its configured result.
// attachResult, attachFound, and attachErr script Attach's return; attachCalled and gotAttachSpec
// record whether Attach ran and with what Spec, so a test can assert the probe ran and with what.
type fakeShuttle struct {
	result    shuttleengine.Result
	err       error
	called    bool
	gotSpec   shuttleengine.Spec
	duringRun func()

	attachResult  shuttleengine.Result
	attachFound   bool
	attachErr     error
	attachCalled  bool
	gotAttachSpec shuttleengine.Spec
}

func (f *fakeShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.called = true
	f.gotSpec = spec
	if f.duringRun != nil {
		f.duringRun()
	}
	return f.result, f.err
}

// Attach implements shedadapters.Shuttle's probe method, recording spec and returning f's
// caller-configured attachResult/attachFound/attachErr.
func (f *fakeShuttle) Attach(spec shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	f.attachCalled = true
	f.gotAttachSpec = spec
	return f.attachResult, f.attachFound, f.attachErr
}

func specSource(spec shuttleengine.Spec, err error) SpecSource {
	return func() (shuttleengine.Spec, error) {
		return spec, err
	}
}

func TestSingleLLMProducer_OutcomeDone(t *testing.T) {
	dir := t.TempDir()
	outputs := []string{
		filepath.Join(dir, "primary.md"),
		filepath.Join(dir, "secondary.md"),
	}
	spec := shuttleengine.Spec{Prompt: "do the thing", OutputFiles: outputs}
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if ptr.Path != outputs[0] {
		t.Errorf("Call() pointer = %q; want %q (first entry)", ptr.Path, outputs[0])
	}
	if !shuttle.called {
		t.Error("Call() did not invoke the shuttle seam")
	}
}

func TestSingleLLMProducer_OutcomeAsking(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "ask", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking, LastAssistantMessage: "what next?"}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

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

func TestSingleLLMProducer_OutcomeDiedAndTimeout(t *testing.T) {
	tests := []struct {
		name    string
		outcome shuttleengine.Outcome
	}{
		{"Died", shuttleengine.OutcomeDied},
		{"Timeout", shuttleengine.OutcomeTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
			shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: tt.outcome}}
			p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

			_, _, err := p.Call(context.Background())
			if err == nil {
				t.Fatal("Call() error = nil; want non-nil")
			}
			if !strings.Contains(err.Error(), string(tt.outcome)) {
				t.Errorf("Call() error %q does not contain outcome %q", err.Error(), tt.outcome)
			}
			if !strings.Contains(err.Error(), "loom") {
				t.Errorf("Call() error %q does not contain producer name %q", err.Error(), "loom")
			}
		})
	}
}

func TestSingleLLMProducer_SeamErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	seamErr := errors.New("seam exploded")
	shuttle := &fakeShuttle{err: seamErr}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, seamErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, seamErr)
	}
	if strings.Contains(err.Error(), string(shuttleengine.OutcomeDied)) || strings.Contains(err.Error(), string(shuttleengine.OutcomeTimeout)) {
		t.Errorf("Call() error %q reads like the died/timeout mapping, want distinguishable seam-error text", err.Error())
	}
}

func TestSingleLLMProducer_OutcomeDoneWithEmptyOutputFiles(t *testing.T) {
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: nil}
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil (guard against indexing empty OutputFiles)")
	}
}

func TestSingleLLMProducer_SpecSourceError(t *testing.T) {
	specErr := errors.New("spec build failed")
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(shuttleengine.Spec{}, specErr), shuttle, fixedClock(time.Now()))

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, specErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, specErr)
	}
	if shuttle.called {
		t.Error("Call() invoked the shuttle seam after a SpecSource error")
	}
}

func TestSingleLLMProducer_RelativeOutputFileRejected(t *testing.T) {
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{"relative/out.md"}}
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "relative/out.md") {
		t.Errorf("Call() error %q does not name the offending entry", err.Error())
	}
	if shuttle.called {
		t.Error("Call() invoked the shuttle seam with a relative OutputFiles entry")
	}
}

func TestSingleLLMProducer_ArchivesPreexistingOutput(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	if err := os.WriteFile(outPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	instant := time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}

	var archivedFree bool
	shuttle := &fakeShuttle{
		result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		duringRun: func() {
			_, err := os.Stat(outPath)
			archivedFree = os.IsNotExist(err)
		},
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(instant))

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if !archivedFree {
		t.Error("original output path was not free by the time the shuttle seam ran")
	}
	want := filepath.Join(dir, "out-20260816T151326Z.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected archived file %s to exist: %v", want, err)
	}
}

func TestSingleLLMProducer_ArchiveCollisionSuffix(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	instant := time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}

	if err := os.WriteFile(outPath, []byte("first"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	shuttle1 := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p1 := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle1, fixedClock(instant))
	if _, _, err := p1.Call(context.Background()); err != nil {
		t.Fatalf("Call() (first) error = %v; want nil", err)
	}

	if err := os.WriteFile(outPath, []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	shuttle2 := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p2 := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle2, fixedClock(instant))
	if _, _, err := p2.Call(context.Background()); err != nil {
		t.Fatalf("Call() (second) error = %v; want nil", err)
	}

	want := filepath.Join(dir, "out-20260816T151326Z-1.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected collision-suffixed archive file %s to exist: %v", want, err)
	}
}

func TestSingleLLMProducer_MissingOutputFileIsNoOp(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
}

func TestSingleLLMProducer_NilNowStillArchives(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	if err := os.WriteFile(outPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, nil)

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Error("original output path still exists after archive under a nil now")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one archived entry in %s, got %d", dir, len(entries))
	}
	if !strings.HasPrefix(entries[0].Name(), "out-") {
		t.Errorf("archived entry name = %q; want prefix %q", entries[0].Name(), "out-")
	}
}

func TestSingleLLMProducer_AlreadyCancelledContext(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if shuttle.called {
		t.Error("Call() invoked the shuttle seam with an already-cancelled context")
	}
}

func TestSingleLLMProducer_CancelledDuringRun_OutcomeDoneStillSucceeds(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	if err := os.WriteFile(outPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}

	ctx, cancel := context.WithCancel(context.Background())
	shuttle := &fakeShuttle{
		result:    shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		duringRun: cancel,
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	outcome, ptr, err := p.Call(ctx)
	if err != nil {
		t.Fatalf("Call() error = %v; want nil (genuine success survives cancellation)", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if ptr.Path != outPath {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, outPath)
	}
}

func TestSingleLLMProducer_CancelledDuringRun_OutcomeAskingYieldsContextError(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}

	ctx, cancel := context.WithCancel(context.Background())
	shuttle := &fakeShuttle{
		result:    shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking},
		duringRun: cancel,
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	outcome, ptr, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil context error")
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
}

func TestSingleLLMProducer_NoBridgeInstalled(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	// The seam's Run(shuttleengine.Spec) (shuttleengine.Result, error) shape carries no callback
	// field and no cancellation channel; the recorded Spec being exactly the SpecSource's output
	// pins that the seam receives nothing else.
	if shuttle.gotSpec.Prompt != spec.Prompt {
		t.Errorf("recorded spec.Prompt = %q; want %q", shuttle.gotSpec.Prompt, spec.Prompt)
	}
	if len(shuttle.gotSpec.OutputFiles) != 1 || shuttle.gotSpec.OutputFiles[0] != spec.OutputFiles[0] {
		t.Errorf("recorded spec.OutputFiles = %v; want %v", shuttle.gotSpec.OutputFiles, spec.OutputFiles)
	}
}

func TestSingleLLMProducer_ProbeNotFound_ArchivesAndRuns(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	if err := os.WriteFile(outPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	instant := time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	shuttle := &fakeShuttle{
		attachFound: false,
		result:      shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(instant))

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if !shuttle.attachCalled {
		t.Error("Call() did not probe Attach")
	}
	if !shuttle.called {
		t.Error("Call() did not call Run after a not-found probe")
	}
	want := filepath.Join(dir, "out-20260816T151326Z.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected archived file %s to exist: %v", want, err)
	}
}

func TestSingleLLMProducer_ProbeFound_NoArchiveNoRun(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	content := []byte("live agent has not finished yet")
	if err := os.WriteFile(outPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	shuttle := &fakeShuttle{
		attachFound:  true,
		attachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if !shuttle.attachCalled {
		t.Error("Call() did not probe Attach")
	}
	if shuttle.called {
		t.Error("Call() called Run after a found probe; want no respawn")
	}
	// Prove no archive happened by asserting the original path still holds its original content --
	// not merely that no archive sibling appeared, which would also be true if the file were simply
	// deleted.
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("original output file %s no longer exists: %v", outPath, err)
	}
	if string(got) != string(content) {
		t.Errorf("original output file content = %q; want %q (unarchived)", got, content)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly one entry in %s (no archive sibling), got %d", dir, len(entries))
	}
}

func TestSingleLLMProducer_AttachedOutcomeDone(t *testing.T) {
	dir := t.TempDir()
	outputs := []string{filepath.Join(dir, "primary.md"), filepath.Join(dir, "secondary.md")}
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: outputs}
	shuttle := &fakeShuttle{
		attachFound:  true,
		attachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if ptr.Path != outputs[0] {
		t.Errorf("Call() pointer = %q; want %q (first entry)", ptr.Path, outputs[0])
	}
	if shuttle.called {
		t.Error("Call() called Run after an attached OutcomeDone")
	}
}

func TestSingleLLMProducer_AttachedOutcomeAsking(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	shuttle := &fakeShuttle{
		attachFound:  true,
		attachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking, LastAssistantMessage: "what next?"},
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

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

func TestSingleLLMProducer_AttachedOutcomeDiedAndTimeout(t *testing.T) {
	tests := []struct {
		name    string
		outcome shuttleengine.Outcome
	}{
		{"Died", shuttleengine.OutcomeDied},
		{"Timeout", shuttleengine.OutcomeTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
			shuttle := &fakeShuttle{
				attachFound:  true,
				attachResult: shuttleengine.Result{Outcome: tt.outcome},
			}
			p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

			_, _, err := p.Call(context.Background())
			if err == nil {
				t.Fatal("Call() error = nil; want non-nil")
			}
			if !strings.Contains(err.Error(), string(tt.outcome)) {
				t.Errorf("Call() error %q does not contain outcome %q", err.Error(), tt.outcome)
			}
			if shuttle.called {
				t.Error("Call() called Run after an attached outcome")
			}
		})
	}
}

func TestSingleLLMProducer_ProbeErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	if err := os.WriteFile(outPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	attachErr := errors.New("probe exploded")
	shuttle := &fakeShuttle{attachErr: attachErr}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, attachErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, attachErr)
	}
	if shuttle.called {
		t.Error("Call() called Run after a probe error")
	}
	got, err2 := os.ReadFile(outPath)
	if err2 != nil {
		t.Fatalf("original output file %s no longer exists: %v", outPath, err2)
	}
	if string(got) != "stale" {
		t.Errorf("original output file was archived despite a probe error: content = %q", got)
	}
}

func TestSingleLLMProducer_AlreadyCancelledContext_NoProbeAttempted(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if shuttle.attachCalled {
		t.Error("Call() probed Attach with an already-cancelled context")
	}
}

func TestSingleLLMProducer_CancelledDuringProbe_YieldsContextError(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	attachErr := errors.New("probe exploded")

	ctx, cancel := context.WithCancel(context.Background())
	shuttle := &fakeShuttleWithAttachHook{
		fakeShuttle:  fakeShuttle{attachErr: attachErr},
		duringAttach: cancel,
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()))

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if shuttle.called {
		t.Error("Call() called Run after a cancelled probe")
	}
}

// fakeShuttleWithAttachHook embeds fakeShuttle and runs an optional duringAttach hook mid-Attach, so
// a test can cancel the context (or otherwise act) as if it happened during the probe, before Attach
// returns its configured result.
type fakeShuttleWithAttachHook struct {
	fakeShuttle
	duringAttach func()
}

// Attach overrides fakeShuttle's Attach to run duringAttach (if set) before returning the embedded
// fakeShuttle's configured attach result.
func (f *fakeShuttleWithAttachHook) Attach(spec shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	result, found, err := f.fakeShuttle.Attach(spec)
	if f.duringAttach != nil {
		f.duringAttach()
	}
	return result, found, err
}
