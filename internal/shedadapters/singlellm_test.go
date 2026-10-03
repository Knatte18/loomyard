package shedadapters

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
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
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Done)
	if ptr.Path != outputs[0] {
		t.Errorf("Call() pointer = %q; want %q (first entry)", ptr.Path, outputs[0])
	}
	if !shuttle.Called {
		t.Error("Call() did not invoke the shuttle seam")
	}
}

func TestSingleLLMProducer_OutcomeAsking(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "ask", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	tests := []struct {
		name       string
		result     shuttleengine.Result
		wantReason string
	}{
		{"WithSessionID", shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking, LastAssistantMessage: "what next?", SessionID: "sess-9", RunDir: "/tmp/run"}, "agent is asking a question; session sess-9"},
		{"EmptySessionIDNamesRunDir", shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking, LastAssistantMessage: "what next?", RunDir: "/tmp/run"}, "agent is asking a question; run dir /tmp/run"},
		{"NeitherIsBareText", shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking, LastAssistantMessage: "what next?"}, "agent is asking a question"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := &shedfake.Shuttle{Result: tt.result}
			p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

			ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
			if ptr.Path != "" || ptr.GateAttempts != nil {
				t.Errorf("Call() pointer = %+v; want empty Path and no GateAttempts", ptr)
			}
			if ptr.Reason != tt.wantReason {
				t.Errorf("Call() Reason = %q; want %q", ptr.Reason, tt.wantReason)
			}
			if strings.Contains(ptr.Reason, "what next?") {
				t.Errorf("Call() Reason %q contains the agent's message", ptr.Reason)
			}
		})
	}
}

func TestSingleLLMProducer_OutcomeDiedAndTimeout(t *testing.T) {
	tests := []struct {
		name    string
		outcome shuttleengine.Outcome
	}{
		{"Died", shuttleengine.OutcomeDied},
		{"Timeout", shuttleengine.OutcomeTimeout},
		{"Unrecognized", shuttleengine.Outcome("bogus-outcome")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
			shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{
				Outcome:    tt.outcome,
				SessionID:  "session-1",
				StrandGUID: "strand-1",
				RunDir:     dir,
			}}
			p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)
			buf := captureLogOutput(t)

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
			logged := buf.String()
			if !strings.Contains(logged, "WARN") {
				t.Errorf("captured log %q does not contain level token %q", logged, "WARN")
			}
			for _, key := range []string{"sessionID", "strandGUID", "runDir", "outcome"} {
				if !strings.Contains(logged, key) {
					t.Errorf("captured log %q does not contain field key %q", logged, key)
				}
			}
		})
	}
}

// TestSingleLLMProducer_NotStartedWrapsErrNotStarted pins that a died outcome the shuttle reports as never-ready wraps ErrNotStarted,
// and an ordinary died outcome does not.
func TestSingleLLMProducer_NotStartedWrapsErrNotStarted(t *testing.T) {
	for _, notStarted := range []bool{true, false} {
		dir := t.TempDir()
		spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
		shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied, RunDir: dir, NotStarted: notStarted}}
		p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)
		captureLogOutput(t)

		_, _, err := p.Call(context.Background())
		if err == nil {
			t.Fatalf("NotStarted=%v: Call() error = nil; want non-nil", notStarted)
		}
		if got := errors.Is(err, shuttleengine.ErrNotStarted); got != notStarted {
			t.Errorf("NotStarted=%v: errors.Is(err, ErrNotStarted) = %v; want %v (err %q)", notStarted, got, notStarted, err)
		}
	}
}

// TestSingleLLMProducer_CancelledDuringRun_DiedOutcomeEmitsNoWarn pins the ordering the
// died/timeout and default branches share with the cancellation guard: a cancelled context still
// returns the cancellation error, and the log line before the fmt.Errorf return must never fire on
// that path.
func TestSingleLLMProducer_CancelledDuringRun_DiedOutcomeEmitsNoWarn(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}

	ctx, cancel := context.WithCancel(context.Background())
	shuttle := &shedfake.Shuttle{
		Result:    shuttleengine.Result{Outcome: shuttleengine.OutcomeDied},
		DuringRun: cancel,
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)
	buf := captureLogOutput(t)

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if buf.Len() != 0 {
		t.Errorf("captured log buffer = %q; want empty on the cancellation path", buf.String())
	}
}

func TestSingleLLMProducer_SeamErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	seamErr := errors.New("seam exploded")
	shuttle := &shedfake.Shuttle{Err: seamErr}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

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
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil (guard against indexing empty OutputFiles)")
	}
}

func TestSingleLLMProducer_SpecSourceError(t *testing.T) {
	specErr := errors.New("spec build failed")
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(shuttleengine.Spec{}, specErr), shuttle, fixedClock(time.Now()), nil)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, specErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, specErr)
	}
	if shuttle.Called {
		t.Error("Call() invoked the shuttle seam after a SpecSource error")
	}
}

func TestSingleLLMProducer_RelativeOutputFileRejected(t *testing.T) {
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{"relative/out.md"}}
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "relative/out.md") {
		t.Errorf("Call() error %q does not name the offending entry", err.Error())
	}
	if shuttle.Called {
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
	shuttle := &shedfake.Shuttle{
		Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		DuringRun: func() {
			_, err := os.Stat(outPath)
			archivedFree = os.IsNotExist(err)
		},
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(instant), nil)

	shedfake.CallOK(t, p)
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
	shuttle1 := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p1 := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle1, fixedClock(instant), nil)
	shedfake.CallOK(t, p1)

	if err := os.WriteFile(outPath, []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	shuttle2 := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p2 := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle2, fixedClock(instant), nil)
	shedfake.CallOK(t, p2)

	want := filepath.Join(dir, "out-20260816T151326Z-1.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected collision-suffixed archive file %s to exist: %v", want, err)
	}
}

func TestSingleLLMProducer_MissingOutputFileIsNoOp(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	shedfake.CallOK(t, p)
}

func TestSingleLLMProducer_NilNowStillArchives(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	if err := os.WriteFile(outPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, nil, nil)

	shedfake.CallOK(t, p)
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
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if shuttle.Called {
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
	shuttle := &shedfake.Shuttle{
		Result:    shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		DuringRun: cancel,
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

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
	shuttle := &shedfake.Shuttle{
		Result:    shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking},
		DuringRun: cancel,
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

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
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	shedfake.CallOK(t, p)
	// The seam's Run(shuttleengine.Spec) (shuttleengine.Result, error) shape carries no callback
	// field and no cancellation channel; the recorded Spec being exactly the SpecSource's output
	// pins that the seam receives nothing else.
	if shuttle.GotSpec.Prompt != spec.Prompt {
		t.Errorf("recorded spec.Prompt = %q; want %q", shuttle.GotSpec.Prompt, spec.Prompt)
	}
	if len(shuttle.GotSpec.OutputFiles) != 1 || shuttle.GotSpec.OutputFiles[0] != spec.OutputFiles[0] {
		t.Errorf("recorded spec.OutputFiles = %v; want %v", shuttle.GotSpec.OutputFiles, spec.OutputFiles)
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
	shuttle := &shedfake.Shuttle{
		AttachFound: false,
		Result:      shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(instant), nil)

	shedfake.CallOK(t, p)
	if !shuttle.AttachCalled {
		t.Error("Call() did not probe Attach")
	}
	if !shuttle.Called {
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
	shuttle := &shedfake.Shuttle{
		AttachFound:  true,
		AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	shedfake.CallOK(t, p)
	if !shuttle.AttachCalled {
		t.Error("Call() did not probe Attach")
	}
	if shuttle.Called {
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
	shuttle := &shedfake.Shuttle{
		AttachFound:  true,
		AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Done)
	if ptr.Path != outputs[0] {
		t.Errorf("Call() pointer = %q; want %q (first entry)", ptr.Path, outputs[0])
	}
	if shuttle.Called {
		t.Error("Call() called Run after an attached OutcomeDone")
	}
}

func TestSingleLLMProducer_AttachedOutcomeAsking(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	shuttle := &shedfake.Shuttle{
		AttachFound:  true,
		AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking, LastAssistantMessage: "what next?"},
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if ptr.Path != "" || ptr.Reason != "agent is asking a question" {
		t.Errorf("Call() pointer = %+v; want empty Path and the asking Reason", ptr)
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
			shuttle := &shedfake.Shuttle{
				AttachFound:  true,
				AttachResult: shuttleengine.Result{Outcome: tt.outcome},
			}
			p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

			_, _, err := p.Call(context.Background())
			if err == nil {
				t.Fatal("Call() error = nil; want non-nil")
			}
			if !strings.Contains(err.Error(), string(tt.outcome)) {
				t.Errorf("Call() error %q does not contain outcome %q", err.Error(), tt.outcome)
			}
			if shuttle.Called {
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
	shuttle := &shedfake.Shuttle{AttachErr: attachErr}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, attachErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, attachErr)
	}
	if shuttle.Called {
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
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if shuttle.AttachCalled {
		t.Error("Call() probed Attach with an already-cancelled context")
	}
}

func TestSingleLLMProducer_CancelledDuringProbe_YieldsContextError(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	attachErr := errors.New("probe exploded")

	ctx, cancel := context.WithCancel(context.Background())
	shuttle := &shedfake.Shuttle{
		AttachErr:    attachErr,
		DuringAttach: cancel,
	}
	p := NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if shuttle.Called {
		t.Error("Call() called Run after a cancelled probe")
	}
}

// TestSingleLLMProducer_PrepareFreshSpawnRunsOnlyOnTheRespawnPath is the guard for the ordering the
// whole probe-before-archive design rests on: a caller's destructive preparation must never touch the
// output files while a live agent may still be writing them.
//
// The AttachFound row is the direct regression guard. Plan-Write used to rotate _lyx/plan from a
// decorator wrapping this producer, so the rotation ran before Call and therefore before the probe:
// on a resume it moved 00-overview.md aside and then attached to the live plan agent, whose
// completion shuttle's Wait could no longer observe -- a finished plan became a hard timeout failure.
func TestSingleLLMProducer_PrepareFreshSpawnRunsOnlyOnTheRespawnPath(t *testing.T) {
	tests := []struct {
		name        string
		attachFound bool
		wantPrepare int
		wantRun     bool
	}{
		{name: "AttachFound_PrepareIsSkipped", attachFound: true, wantPrepare: 0, wantRun: false},
		{name: "NothingToAttachTo_PrepareRuns", attachFound: false, wantPrepare: 1, wantRun: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "00-overview.md")
			if err := os.WriteFile(output, []byte("the finished artifact"), 0o644); err != nil {
				t.Fatalf("WriteFile(%s): %v", output, err)
			}
			spec := shuttleengine.Spec{Prompt: "plan", OutputFiles: []string{output}}
			shuttle := &shedfake.Shuttle{
				AttachFound:  tt.attachFound,
				AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
				Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			}

			prepared := 0
			prepare := func() (string, error) {
				prepared++
				// Stand in for the real rotation: move the output file out from under whatever is
				// writing it. On the attach path this must never happen.
				return "", os.Rename(output, filepath.Join(dir, "rotated-away.md"))
			}
			p := NewSingleLLMProducer("Plan-Write", specSource(spec, nil), shuttle, fixedClock(time.Now()), prepare)

			shedfake.CallOK(t, p)

			if !shuttle.AttachCalled {
				t.Error("Call() never probed Attach")
			}
			if prepared != tt.wantPrepare {
				t.Errorf("prepareFreshSpawn ran %d time(s); want %d", prepared, tt.wantPrepare)
			}
			if shuttle.Called != tt.wantRun {
				t.Errorf("shuttle.Run called = %v; want %v", shuttle.Called, tt.wantRun)
			}
			_, statErr := os.Stat(output)
			if tt.attachFound && statErr != nil {
				t.Errorf("Stat(%s) = %v; want the attached run's output file left untouched", output, statErr)
			}
			if !tt.attachFound && statErr == nil {
				t.Errorf("Stat(%s) = nil; want the respawn path's preparation to have moved it", output)
			}
		})
	}
}

// --- Gate ---

func TestSingleLLMProducer_Gate_PassingGateReachesDone(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	gate := shuttleengine.GateSpec{{Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		return shuttleengine.GateResult{Passed: true}, nil
	}}}
	p := NewSingleLLMProducerGated("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil, gate)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Done)
	if ptr.Path != outPath {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, outPath)
	}
}

// TestSingleLLMProducer_Gate_FailedGateReachesStuckWithArtifactPointer is the load-bearing assertion
// for the "the two producers' output pointers mean different things" decision's writer-row half: the
// pointer must be explicitly equal to spec.OutputFiles[0], never the zero OutputPointer -- a test
// asserting an empty pointer here would silently disable commit-on-gate-failure.
func TestSingleLLMProducer_Gate_FailedGateReachesStuckWithArtifactPointer(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	gate := shuttleengine.GateSpec{{Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		return shuttleengine.GateResult{Passed: false, Findings: "the widget is wrong"}, nil
	}}}
	p := NewSingleLLMProducerGated("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil, gate)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if ptr.Path != outPath {
		t.Errorf("Call() pointer.Path = %q; want %q (never empty)", ptr.Path, outPath)
	}
	if ptr.GateAttempts == nil || *ptr.GateAttempts != 0 {
		t.Errorf("Call() pointer.GateAttempts = %v; want pointer to 0 (shedfake.Shuttle's default attempt count)", ptr.GateAttempts)
	}
	if want := "gate did not pass after 0 attempts; findings: "; ptr.Reason != want {
		t.Errorf("Call() Reason = %q; want %q (attempt count and findings path)", ptr.Reason, want)
	}
	if strings.HasPrefix(ptr.Reason, "agent is asking") {
		t.Errorf("Call() Reason %q is the asking reason; want the gate-failed one", ptr.Reason)
	}
}

// TestSingleLLMProducer_Gate_TerminalReasonReachesStuck proves a failing gate outcome carrying Reason maps to Stuck with that reason in place of the generic text.
func TestSingleLLMProducer_Gate_TerminalReasonReachesStuck(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, GateReason: "the parent rejected the cap round"}
	gate := shuttleengine.GateSpec{{Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		return shuttleengine.GateResult{Passed: false, Findings: "the parent rejected the cap round", Terminal: true}, nil
	}}}
	p := NewSingleLLMProducerGated("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil, gate)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if want := "the parent rejected the cap round"; ptr.Reason != want {
		t.Errorf("Call() Reason = %q; want %q", ptr.Reason, want)
	}
}

// TestSingleLLMProducer_Gate_AskingKeepsEmptyPointer proves a gated producer's OutcomeAsking still
// maps to Stuck with an empty pointer, exactly as the ungated case does -- the gate is never
// consulted for a non-done outcome.
func TestSingleLLMProducer_Gate_AskingKeepsEmptyPointer(t *testing.T) {
	dir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "ask", OutputFiles: []string{filepath.Join(dir, "out.md")}}
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking, LastAssistantMessage: "what next?"}}
	gate := shuttleengine.GateSpec{{Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		t.Fatal("gate closure invoked for a non-done outcome")
		return shuttleengine.GateResult{}, nil
	}}}
	p := NewSingleLLMProducerGated("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil, gate)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if ptr.Path != "" || ptr.GateAttempts != nil || ptr.Reason != "agent is asking a question" {
		t.Errorf("Call() pointer = %+v; want empty Path, no GateAttempts, and the asking Reason", ptr)
	}
}

// TestSingleLLMProducer_Gate_AttachPathIsGatedToo proves the probe reaches AttachGated with the
// producer's own GateSpec, so a resumed run is gated exactly as a fresh one is.
func TestSingleLLMProducer_Gate_AttachPathIsGatedToo(t *testing.T) {
	dir := t.TempDir()
	outputs := []string{filepath.Join(dir, "primary.md")}
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: outputs}
	shuttle := &shedfake.Shuttle{
		AttachFound:  true,
		AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	gate := shuttleengine.GateSpec{{Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		return shuttleengine.GateResult{Passed: true}, nil
	}}}
	p := NewSingleLLMProducerGated("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil, gate)

	shedfake.CallOK(t, p)
	if len(shuttle.GotAttachGateSpec) == 0 {
		t.Error("AttachGated was not called with the producer's own GateSpec")
	}
}

// TestSingleLLMProducer_Gate_AttemptsPropagatesOntoOutputPointer proves a non-zero
// GateOutcome.Attempts (a gate that needed re-prompts before passing) reaches the returned
// OutputPointer.GateAttempts unchanged, per the producer-gate contract's "recorded in the
// row's envelope/history so the status file shows it" requirement -- shedengine's own
// TestStep_GateAttempts_* tests (internal/shedengine/gateattempts_test.go) cover the persisted
// side of that chain; this test covers the producer's own half, that it reads result.Gate.Attempts
// and carries it, rather than shedengine silently receiving a value nobody actually populated.
func TestSingleLLMProducer_Gate_AttemptsPropagatesOntoOutputPointer(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.md")
	spec := shuttleengine.Spec{Prompt: "run", OutputFiles: []string{outPath}}
	shuttle := &shedfake.Shuttle{
		Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		GateAttempts: 2,
	}
	gate := shuttleengine.GateSpec{{Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		return shuttleengine.GateResult{Passed: true}, nil
	}}}
	p := NewSingleLLMProducerGated("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil, gate)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Done)
	if ptr.GateAttempts == nil || *ptr.GateAttempts != 2 {
		t.Errorf("Call() pointer.GateAttempts = %v; want pointer to 2 (passed after two re-prompts)", ptr.GateAttempts)
	}
}

// TestSingleLLMProducer_PrepareFreshSpawnErrorNeitherArchivesNorSpawns pins the failure posture: a preparation that cannot complete is a returned error,
// and nothing downstream of it runs.
func TestSingleLLMProducer_PrepareFreshSpawnErrorNeitherArchivesNorSpawns(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "00-overview.md")
	if err := os.WriteFile(output, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", output, err)
	}
	spec := shuttleengine.Spec{Prompt: "plan", OutputFiles: []string{output}}
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	prepareErr := errors.New("rotation failed")
	p := NewSingleLLMProducer("Plan-Write", specSource(spec, nil), shuttle, fixedClock(time.Now()), func() (string, error) { return "", prepareErr })

	outcome, ptr, err := p.Call(context.Background())
	if !errors.Is(err, prepareErr) {
		t.Fatalf("Call() error = %v; want it to wrap %v", err, prepareErr)
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want the empty value", outcome)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want the zero value", ptr)
	}
	if shuttle.Called {
		t.Error("Call() spawned despite a failed preparation")
	}
	if data, readErr := os.ReadFile(output); readErr != nil || string(data) != "stale" {
		t.Errorf("output file = %q (err %v); want it left unarchived at its original path", string(data), readErr)
	}
}

// TestSingleLLMProducer_PrepareFreshSpawnAmendment pins the amendment seam: the text a preparation returns is appended to the prompt of the respawned run's spec, and to no other spec.
func TestSingleLLMProducer_PrepareFreshSpawnAmendment(t *testing.T) {
	const composed = "plan"
	newSpec := func(t *testing.T) shuttleengine.Spec {
		return shuttleengine.Spec{Prompt: composed, OutputFiles: []string{filepath.Join(t.TempDir(), "00-overview.md")}}
	}
	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}

	t.Run("RespawnAppendsAmendmentAndPreparesOnce", func(t *testing.T) {
		shuttle := &shedfake.Shuttle{Result: done}
		prepared := 0
		prepare := func() (string, error) {
			prepared++
			return "\n\nprior plan here", nil
		}
		p := NewSingleLLMProducer("Plan-Write", specSource(newSpec(t), nil), shuttle, fixedClock(time.Now()), prepare)

		shedfake.CallOK(t, p)
		if prepared != 1 {
			t.Errorf("preparation ran %d time(s); want 1", prepared)
		}
		if want := composed + "\n\nprior plan here"; shuttle.GotSpec.Prompt != want {
			t.Errorf("RunGated spec.Prompt = %q; want %q", shuttle.GotSpec.Prompt, want)
		}
		if shuttle.GotAttachSpec.Prompt != composed {
			t.Errorf("AttachGated spec.Prompt = %q; want the composed prompt %q", shuttle.GotAttachSpec.Prompt, composed)
		}
	})

	t.Run("AttachFoundNeverPreparesAndKeepsComposedPrompt", func(t *testing.T) {
		shuttle := &shedfake.Shuttle{AttachFound: true, AttachResult: done}
		prepared := 0
		prepare := func() (string, error) {
			prepared++
			return "amendment", nil
		}
		p := NewSingleLLMProducer("Plan-Write", specSource(newSpec(t), nil), shuttle, fixedClock(time.Now()), prepare)

		shedfake.CallOK(t, p)
		if prepared != 0 {
			t.Errorf("preparation ran %d time(s) on the attach branch; want 0", prepared)
		}
		if shuttle.GotAttachSpec.Prompt != composed {
			t.Errorf("AttachGated spec.Prompt = %q; want %q", shuttle.GotAttachSpec.Prompt, composed)
		}
	})

	t.Run("NilAndEmptyAmendmentLeavePromptByteIdentical", func(t *testing.T) {
		empty := func() (string, error) { return "", nil }
		for name, prepare := range map[string]func() (string, error){"Nil": nil, "Empty": empty} {
			t.Run(name, func(t *testing.T) {
				shuttle := &shedfake.Shuttle{Result: done}
				p := NewSingleLLMProducer("Plan-Write", specSource(newSpec(t), nil), shuttle, fixedClock(time.Now()), prepare)

				shedfake.CallOK(t, p)
				if shuttle.GotSpec.Prompt != composed {
					t.Errorf("RunGated spec.Prompt = %q; want %q", shuttle.GotSpec.Prompt, composed)
				}
			})
		}
	})
}
