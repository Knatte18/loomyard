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
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

func specSource(spec shuttleengine.Spec, err error) SpecSource {
	return func() (shuttleengine.Spec, error) {
		return spec, err
	}
}

// TestSingleLLMProducer_OutcomeDone covers a completed run: the first output file is the pointer, the
// seam receives exactly the SpecSource's spec and nothing else (the seam's Run shape carries no
// callback and no cancellation channel), a passing gate reaches Done with the gate's attempt count
// carried onto the pointer unchanged, and a genuine success survives a cancellation that arrives
// during the run.
//
//testtiming:keep pins the completed run's pointer, the recorded spec, the gate attempt count on the pointer and that a success survives a cancellation during the run
func TestSingleLLMProducer_OutcomeDone(t *testing.T) {
	tests := []struct {
		name            string
		gated           bool
		gateAttempts    int
		cancelDuringRun bool
	}{
		{name: "Ungated"},
		{name: "PassingGate", gated: true},
		{name: "GateAttemptsReachThePointer", gated: true, gateAttempts: 2},
		{name: "SuccessSurvivesCancellationDuringRun", cancelDuringRun: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			outputs := []string{
				filepath.Join(dir, "primary.md"),
				filepath.Join(dir, "secondary.md"),
			}
			spec := shuttleengine.Spec{Prompt: "do the thing", OutputFiles: outputs}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			shuttle := &shedfake.Shuttle{
				Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
				GateAttempts: tt.gateAttempts,
			}
			if tt.cancelDuringRun {
				shuttle.DuringRun = cancel
			}
			var p *SingleLLMProducer
			if tt.gated {
				gate := shuttleengine.GateSpec{{Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
					return shuttleengine.GateResult{Passed: true}, nil
				}}}
				p = NewSingleLLMProducerGated("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil, gate)
			} else {
				p = NewSingleLLMProducer("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil)
			}

			outcome, ptr, err := p.Call(ctx)
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != shedengine.Done {
				t.Fatalf("Call() outcome = %q; want %q", outcome, shedengine.Done)
			}
			if ptr.Path != outputs[0] {
				t.Errorf("Call() pointer = %q; want %q (first entry)", ptr.Path, outputs[0])
			}
			if tt.gateAttempts != 0 && (ptr.GateAttempts == nil || *ptr.GateAttempts != tt.gateAttempts) {
				t.Errorf("Call() pointer.GateAttempts = %v; want pointer to %d", ptr.GateAttempts, tt.gateAttempts)
			}
			if !shuttle.Called {
				t.Error("Call() did not invoke the shuttle seam")
			}
			if shuttle.GotSpec.Prompt != spec.Prompt {
				t.Errorf("recorded spec.Prompt = %q; want %q", shuttle.GotSpec.Prompt, spec.Prompt)
			}
			if len(shuttle.GotSpec.OutputFiles) != len(outputs) || shuttle.GotSpec.OutputFiles[0] != outputs[0] || shuttle.GotSpec.OutputFiles[1] != outputs[1] {
				t.Errorf("recorded spec.OutputFiles = %v; want %v", shuttle.GotSpec.OutputFiles, outputs)
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
			buf := logcapture.Capture(t)

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
		logcapture.Capture(t)

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
	buf := logcapture.Capture(t)

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if buf.String() != "" {
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

// TestSingleLLMProducer_ArchivesPreexistingOutput also pins that a second archive in the same clock
// second takes a numeric suffix rather than overwriting the first.
//
//testtiming:keep pins that a pre-existing output is archived to a stamped sibling before the seam runs, and that a same-second collision takes a numeric suffix
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
	if !shuttle.AttachCalled {
		t.Error("Call() did not probe Attach")
	}
	if !shuttle.Called {
		t.Error("Call() did not call Run after a not-found probe")
	}
	if !archivedFree {
		t.Error("original output path was not free by the time the shuttle seam ran")
	}
	want := filepath.Join(dir, "out-20260816T151326Z.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected archived file %s to exist: %v", want, err)
	}

	// A second pre-existing output in the same clock second collides with the first archive.
	if err := os.WriteFile(outPath, []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	p2 := NewSingleLLMProducer("loom", specSource(spec, nil), &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}, fixedClock(instant), nil)
	shedfake.CallOK(t, p2)
	wantSuffixed := filepath.Join(dir, "out-20260816T151326Z-1.md")
	if _, err := os.Stat(wantSuffixed); err != nil {
		t.Errorf("expected collision-suffixed archive file %s to exist: %v", wantSuffixed, err)
	}
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

// TestSingleLLMProducer_AttachedRun covers a live agent the probe found: its outcome maps as the
// spawned run's does, the producer never respawns, and the output file the live agent is still
// writing is left untouched -- asserted by its content, not merely by the absence of an archive
// sibling, which would also be true if the file were simply deleted -- and never archived.
//
//testtiming:keep pins the outcome mapping of an attached live run, that it is never respawned and that its output file is left unarchived with its content intact
func TestSingleLLMProducer_AttachedRun(t *testing.T) {
	passing := shuttleengine.GateSpec{{Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		return shuttleengine.GateResult{Passed: true}, nil
	}}}
	tests := []struct {
		name        string
		result      shuttleengine.Result
		gate        shuttleengine.GateSpec
		wantOutcome shedengine.Outcome
		wantReason  string
		// wantErr is a substring of the error an attached died or timed-out run returns.
		wantErr string
	}{
		{name: "Done", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, wantOutcome: shedengine.Done},
		{name: "Died", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}, wantErr: string(shuttleengine.OutcomeDied)},
		{name: "Timeout", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}, wantErr: string(shuttleengine.OutcomeTimeout)},
		// The probe reaches AttachGated with the producer's own GateSpec, so a resumed run is gated
		// exactly as a fresh one is.
		{name: "GatedToo", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, gate: passing, wantOutcome: shedengine.Done},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			outputs := []string{filepath.Join(dir, "primary.md"), filepath.Join(dir, "secondary.md")}
			content := []byte("live agent has not finished yet")
			if err := os.WriteFile(outputs[0], content, 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			spec := shuttleengine.Spec{Prompt: "run", OutputFiles: outputs}
			shuttle := &shedfake.Shuttle{
				AttachFound:  true,
				AttachResult: tt.result,
				Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			}
			p := NewSingleLLMProducerGated("loom", specSource(spec, nil), shuttle, fixedClock(time.Now()), nil, tt.gate)

			outcome, ptr, err := p.Call(context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Call() error = %v; want one containing %q", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("Call() error = %v; want nil", err)
				}
				if outcome != tt.wantOutcome {
					t.Fatalf("Call() outcome = %q; want %q", outcome, tt.wantOutcome)
				}
				wantPath := ""
				if tt.wantOutcome == shedengine.Done {
					wantPath = outputs[0]
				}
				if ptr.Path != wantPath || ptr.Reason != tt.wantReason {
					t.Errorf("Call() pointer = %+v; want Path %q and Reason %q", ptr, wantPath, tt.wantReason)
				}
			}
			if !shuttle.AttachCalled {
				t.Error("Call() did not probe Attach")
			}
			if shuttle.Called {
				t.Error("Call() called Run after a found probe; want no respawn")
			}
			if tt.gate != nil && len(shuttle.GotAttachGateSpec) == 0 {
				t.Error("AttachGated was not called with the producer's own GateSpec")
			}
			got, err := os.ReadFile(outputs[0])
			if err != nil {
				t.Fatalf("original output file %s no longer exists: %v", outputs[0], err)
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
	if shuttle.Called {
		t.Error("Call() invoked the shuttle seam with an already-cancelled context")
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
//
// It also pins the amendment seam: the text a preparation returns is appended to the prompt of the
// respawned run's spec, and to no other spec; a nil or empty amendment leaves the prompt
// byte-identical.
func TestSingleLLMProducer_PrepareFreshSpawnRunsOnlyOnTheRespawnPath(t *testing.T) {
	const composed = "plan"
	tests := []struct {
		name        string
		attachFound bool
		// noPrepare passes a nil preparation.
		noPrepare   bool
		amendment   string
		wantPrepare int
		wantRun     bool
		wantPrompt  string
	}{
		{name: "AttachFound_PrepareIsSkipped", attachFound: true, amendment: "amendment", wantPrepare: 0, wantRun: false, wantPrompt: composed},
		{name: "NothingToAttachTo_PrepareRunsAndAppendsItsAmendment", amendment: "\n\nprior plan here", wantPrepare: 1, wantRun: true, wantPrompt: composed + "\n\nprior plan here"},
		{name: "EmptyAmendmentLeavesThePromptByteIdentical", wantPrepare: 1, wantRun: true, wantPrompt: composed},
		{name: "NilPreparationLeavesThePromptByteIdentical", noPrepare: true, wantRun: true, wantPrompt: composed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "00-overview.md")
			if err := os.WriteFile(output, []byte("the finished artifact"), 0o644); err != nil {
				t.Fatalf("WriteFile(%s): %v", output, err)
			}
			spec := shuttleengine.Spec{Prompt: composed, OutputFiles: []string{output}}
			shuttle := &shedfake.Shuttle{
				AttachFound:  tt.attachFound,
				AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
				Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			}

			prepared := 0
			var prepare func() (string, error)
			if !tt.noPrepare {
				prepare = func() (string, error) {
					prepared++
					// Stand in for the real rotation: move the output file out from under whatever is
					// writing it. On the attach path this must never happen.
					return tt.amendment, os.Rename(output, filepath.Join(dir, "rotated-away.md"))
				}
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
			if tt.wantRun && shuttle.GotSpec.Prompt != tt.wantPrompt {
				t.Errorf("RunGated spec.Prompt = %q; want %q", shuttle.GotSpec.Prompt, tt.wantPrompt)
			}
			if shuttle.AttachCalled && shuttle.GotAttachSpec.Prompt != composed {
				t.Errorf("AttachGated spec.Prompt = %q; want the composed prompt %q", shuttle.GotAttachSpec.Prompt, composed)
			}
			_, statErr := os.Stat(output)
			if tt.attachFound && statErr != nil {
				t.Errorf("Stat(%s) = %v; want the attached run's output file left untouched", output, statErr)
			}
			if tt.wantPrepare > 0 && statErr == nil {
				t.Errorf("Stat(%s) = nil; want the respawn path's preparation to have moved it", output)
			}
		})
	}
}

// --- Gate ---

// TestSingleLLMProducer_Gate_FailedGateReachesStuckWithArtifactPointer is the load-bearing assertion
// for the "the two producers' output pointers mean different things" decision's writer-row half: the
// pointer must be explicitly equal to spec.OutputFiles[0], never the zero OutputPointer -- a test
// asserting an empty pointer here would silently disable commit-on-gate-failure.
//
//testtiming:keep pins that a failed gate is a Stuck whose pointer is the artifact, never empty, with the attempt count and findings in the reason
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
