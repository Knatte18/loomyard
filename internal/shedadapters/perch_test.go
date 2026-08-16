package shedadapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// fakePerchRunner records the Profile and directory arguments it was handed and returns a
// caller-configured perchengine.Result/error. It counts its own invocations so a test can assert a
// second Call builds a distinct runner.
type fakePerchRunner struct {
	result perchengine.Result
	err    error

	gotProfile     perchengine.Profile
	gotRunDir      string
	gotScratchDir  string
	gotStencilsDir string
	calls          int

	// duringRun, when set, is invoked from inside Run before it returns -- letting a test cancel the
	// context (or otherwise act) as if it happened mid-run, mirroring fakeShuttle's own hook in
	// singlellm_test.go.
	duringRun func()
}

func (f *fakePerchRunner) Run(p perchengine.Profile, runDir, scratchDir, stencilsDir string) (perchengine.Result, error) {
	f.calls++
	f.gotProfile = p
	f.gotRunDir = runDir
	f.gotScratchDir = scratchDir
	f.gotStencilsDir = stencilsDir
	if f.duringRun != nil {
		f.duringRun()
	}
	return f.result, f.err
}

// fakeFactory builds a PerchFactory that records every pauseRequested callback it was handed and
// returns runner (or, when nilRunner is set, a nil PerchRunner) on every invocation.
type fakeFactory struct {
	runner            *fakePerchRunner
	nilRunner         bool
	invocations       int
	recordedCallbacks []func() bool
}

func (f *fakeFactory) factory() PerchFactory {
	return func(pauseRequested func() bool) PerchRunner {
		f.invocations++
		f.recordedCallbacks = append(f.recordedCallbacks, pauseRequested)
		if f.nilRunner {
			return nil
		}
		return f.runner
	}
}

// simpleProfile returns a minimal valid Profile suitable for hashing and passing through Call --
// its content fields are irrelevant since this package never invokes burlerengine validation.
func simpleProfile() perchengine.Profile {
	return perchengine.Profile{Gate: perchengine.Gate{Mode: perchengine.GateLLMVerdict}}
}

// newTestProducer builds a PerchProducer over dir's run/scratch bases with a fake factory
// returning runner, failing the test on constructor error.
func newTestProducer(t *testing.T, dir string, profile perchengine.Profile, runner *fakePerchRunner) (*PerchProducer, *fakeFactory) {
	t.Helper()
	ff := &fakeFactory{runner: runner}
	runDirBase := filepath.Join(dir, "runs")
	scratchDirBase := filepath.Join(dir, "scratch")
	p, err := NewPerchProducer("gate", ff.factory(), profile, runDirBase, scratchDirBase, filepath.Join(dir, "stencils"), "myprefix")
	if err != nil {
		t.Fatalf("NewPerchProducer() error = %v; want nil", err)
	}
	return p, ff
}

// writeState writes a hand-built state.json under runDir/state.json against treadle's own json
// tags -- its runState struct is unexported, so a test in this package cannot construct one.
func writeState(t *testing.T, runDir, outcome string) {
	t.Helper()
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", runDir, err)
	}
	body := `{"profileHash":"deadbeef","roundCaps":[5]`
	if outcome != "" {
		body += `,"outcome":"` + outcome + `"`
	}
	body += "}"
	if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile state.json: %v", err)
	}
}

func hash8(t *testing.T, p perchengine.Profile) string {
	t.Helper()
	h, err := perchengine.ProfileHash(p)
	if err != nil {
		t.Fatalf("ProfileHash: %v", err)
	}
	return h[:8]
}

// --- Outcome mapping table ---

func TestPerchProducer_OutcomeApproved(t *testing.T) {
	dir := t.TempDir()
	runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
	p, ff := newTestProducer(t, dir, simpleProfile(), runner)

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty (pinned decision)", ptr)
	}
	if ff.invocations != 1 {
		t.Errorf("factory invocations = %d; want 1", ff.invocations)
	}
}

func TestPerchProducer_OutcomeStuck(t *testing.T) {
	tests := []struct {
		name   string
		reason perchengine.StuckReason
	}{
		{"HardCap", perchengine.StuckHardCap},
		{"MilestoneStop", perchengine.StuckMilestoneStop},
		{"Circling", perchengine.StuckCircling},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeStuck, StuckReason: tt.reason, RoundsRun: 3}}
			p, _ := newTestProducer(t, dir, simpleProfile(), runner)

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
		})
	}
}

func TestPerchProducer_OutcomePaused(t *testing.T) {
	dir := t.TempDir()
	runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomePaused}}
	p, _ := newTestProducer(t, dir, simpleProfile(), runner)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "out of band") && !strings.Contains(err.Error(), "paused") {
		t.Errorf("Call() error %q does not name the out-of-band pause", err.Error())
	}
}

func TestPerchProducer_UnrecognizedOutcome(t *testing.T) {
	dir := t.TempDir()
	runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.Outcome("WEIRD")}}
	p, _ := newTestProducer(t, dir, simpleProfile(), runner)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "WEIRD") {
		t.Errorf("Call() error %q does not quote the unrecognised value", err.Error())
	}
}

// --- Context rows ---

func TestPerchProducer_AlreadyCancelledContext(t *testing.T) {
	dir := t.TempDir()
	runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
	p, ff := newTestProducer(t, dir, simpleProfile(), runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if ff.invocations != 0 {
		t.Errorf("factory invocations = %d; want 0 (factory never invoked)", ff.invocations)
	}
}

func TestPerchProducer_BridgeReflectsContextState(t *testing.T) {
	dir := t.TempDir()
	runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
	p, ff := newTestProducer(t, dir, simpleProfile(), runner)

	ctx, cancel := context.WithCancel(context.Background())
	if _, _, err := p.Call(ctx); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if len(ff.recordedCallbacks) != 1 {
		t.Fatalf("recorded callbacks = %d; want 1", len(ff.recordedCallbacks))
	}
	cb := ff.recordedCallbacks[0]
	if cb() {
		t.Error("callback reported true before cancellation; want false")
	}
	cancel()
	if !cb() {
		t.Error("callback reported false after cancellation; want true")
	}
}

func TestPerchProducer_CancelledDuringRun_ApprovedStillDone(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	runner := &fakePerchRunner{
		result:    perchengine.Result{Outcome: perchengine.OutcomeApproved},
		duringRun: cancel,
	}
	p, _ := newTestProducer(t, dir, simpleProfile(), runner)

	outcome, ptr, err := p.Call(ctx)
	if err != nil {
		t.Fatalf("Call() error = %v; want nil (genuine success survives cancellation)", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
}

func TestPerchProducer_CancelledDuringRun_StuckAndPausedBecomeContextError(t *testing.T) {
	tests := []struct {
		name    string
		outcome perchengine.Outcome
	}{
		{"Stuck", perchengine.OutcomeStuck},
		{"Paused", perchengine.OutcomePaused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			runner := &fakePerchRunner{result: perchengine.Result{Outcome: tt.outcome}, duringRun: cancel}
			p, _ := newTestProducer(t, dir, simpleProfile(), runner)

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

func TestPerchProducer_TwoCallsRecordTwoFactoryInvocations(t *testing.T) {
	dir := t.TempDir()
	runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
	p, ff := newTestProducer(t, dir, simpleProfile(), runner)

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() (first) error = %v; want nil", err)
	}
	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() (second) error = %v; want nil", err)
	}
	if ff.invocations != 2 {
		t.Errorf("factory invocations = %d; want 2", ff.invocations)
	}
}

// --- Run-id advancement ---

func TestPerchProducer_RunIDAdvancement(t *testing.T) {
	profile := simpleProfile()
	h8 := hash8(t, profile)

	t.Run("EmptyBaseStartsAtOne", func(t *testing.T) {
		subdir := t.TempDir()
		runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
		p, _ := newTestProducer(t, subdir, profile, runner)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantRunDir := filepath.Join(subdir, "runs", "myprefix-"+h8+"-1")
		if runner.gotRunDir != wantRunDir {
			t.Errorf("gotRunDir = %q; want %q", runner.gotRunDir, wantRunDir)
		}
	})

	t.Run("TerminalAdvancesToNext", func(t *testing.T) {
		subdir := t.TempDir()
		runsBase := filepath.Join(subdir, "runs")
		writeState(t, filepath.Join(runsBase, "myprefix-"+h8+"-1"), "APPROVED")

		runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
		p, _ := newTestProducer(t, subdir, profile, runner)
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantRunDir := filepath.Join(runsBase, "myprefix-"+h8+"-2")
		if runner.gotRunDir != wantRunDir {
			t.Errorf("gotRunDir = %q; want %q", runner.gotRunDir, wantRunDir)
		}
	})

	t.Run("NonTerminalIsReused", func(t *testing.T) {
		subdir := t.TempDir()
		runsBase := filepath.Join(subdir, "runs")
		writeState(t, filepath.Join(runsBase, "myprefix-"+h8+"-1"), "")

		runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
		p, _ := newTestProducer(t, subdir, profile, runner)
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantRunDir := filepath.Join(runsBase, "myprefix-"+h8+"-1")
		if runner.gotRunDir != wantRunDir {
			t.Errorf("gotRunDir = %q; want %q", runner.gotRunDir, wantRunDir)
		}
	})

	t.Run("HighestNNotFirstGap", func(t *testing.T) {
		subdir := t.TempDir()
		runsBase := filepath.Join(subdir, "runs")
		writeState(t, filepath.Join(runsBase, "myprefix-"+h8+"-1"), "APPROVED")
		writeState(t, filepath.Join(runsBase, "myprefix-"+h8+"-2"), "APPROVED")

		runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
		p, _ := newTestProducer(t, subdir, profile, runner)
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantRunDir := filepath.Join(runsBase, "myprefix-"+h8+"-3")
		if runner.gotRunDir != wantRunDir {
			t.Errorf("gotRunDir = %q; want %q", runner.gotRunDir, wantRunDir)
		}
	})
}

func TestPerchProducer_ProfileHashNamespacing(t *testing.T) {
	subdir := t.TempDir()
	runsBase := filepath.Join(subdir, "runs")

	profile1 := perchengine.Profile{Gate: perchengine.Gate{Mode: perchengine.GateLLMVerdict}}
	h1 := hash8(t, profile1)
	writeState(t, filepath.Join(runsBase, "myprefix-"+h1+"-1"), "")

	profile2 := perchengine.Profile{Gate: perchengine.Gate{Mode: perchengine.GateLLMVerdict}, Rubric: "different"}
	h2 := hash8(t, profile2)
	if h1 == h2 {
		t.Fatalf("test setup: profile1 and profile2 hash the same (%s)", h1)
	}

	runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
	p, _ := newTestProducer(t, subdir, profile2, runner)
	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}

	wantRunDir := filepath.Join(runsBase, "myprefix-"+h2+"-1")
	if runner.gotRunDir != wantRunDir {
		t.Errorf("gotRunDir = %q; want %q", runner.gotRunDir, wantRunDir)
	}

	// H1's directory is left untouched: no reuse, no deletion.
	if _, err := os.Stat(filepath.Join(runsBase, "myprefix-"+h1+"-1")); err != nil {
		t.Errorf("H1's run dir was disturbed: %v", err)
	}
	entries, err := os.ReadDir(runsBase)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", runsBase, err)
	}
	if len(entries) != 2 {
		t.Errorf("runsBase entries = %d; want 2 (H1's untouched dir plus H2's fresh dir)", len(entries))
	}
}

// --- Filesystem rows ---

func TestPerchProducer_AbsentScratchSiblingResolvesNormally(t *testing.T) {
	dir := t.TempDir()
	runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
	p, _ := newTestProducer(t, dir, simpleProfile(), runner)

	_, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if _, err := os.Stat(runner.gotScratchDir); err != nil {
		t.Errorf("adapter did not create scratch dir %q: %v", runner.gotScratchDir, err)
	}
}

func TestPerchProducer_CorruptStateJSONFailsCall(t *testing.T) {
	dir := t.TempDir()
	profile := simpleProfile()
	h8 := hash8(t, profile)
	runsBase := filepath.Join(dir, "runs")
	runDir := filepath.Join(runsBase, "myprefix-"+h8+"-1")
	scratchDir := filepath.Join(dir, "scratch", "myprefix-"+h8+"-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(runDir): %v", err)
	}
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(scratchDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
	p, _ := newTestProducer(t, dir, profile, runner)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil (corrupt state.json)")
	}
	if runner.calls != 0 {
		t.Error("Call() invoked the seam despite a corrupt state.json")
	}
}

func TestPerchProducer_RunDirAndScratchDirPairShareRunID(t *testing.T) {
	dir := t.TempDir()
	runner := &fakePerchRunner{result: perchengine.Result{Outcome: perchengine.OutcomeApproved}}
	p, _ := newTestProducer(t, dir, simpleProfile(), runner)

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	runID := filepath.Base(runner.gotRunDir)
	wantScratch := filepath.Join(dir, "scratch", runID)
	if runner.gotScratchDir != wantScratch {
		t.Errorf("gotScratchDir = %q; want %q (same run-id as runDir %q)", runner.gotScratchDir, wantScratch, runner.gotRunDir)
	}
}

func TestPerchProducer_InvalidRunIDPrefixRejectedBeforeAnyDirectory(t *testing.T) {
	dir := t.TempDir()
	ff := &fakeFactory{runner: &fakePerchRunner{}}
	_, err := NewPerchProducer("gate", ff.factory(), simpleProfile(), filepath.Join(dir, "runs"), filepath.Join(dir, "scratch"), filepath.Join(dir, "stencils"), "Not Valid!")
	if err == nil {
		t.Fatal("NewPerchProducer() error = nil; want non-nil")
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("t.TempDir() base vanished: %v", statErr)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("directory entries created = %d; want 0 (rejected before any directory touched)", len(entries))
	}
}

func TestPerchProducer_SeamErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	seamErr := errors.New("seam exploded")
	runner := &fakePerchRunner{err: seamErr}
	p, _ := newTestProducer(t, dir, simpleProfile(), runner)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, seamErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, seamErr)
	}
}

func TestPerchProducer_ConstructorValidation(t *testing.T) {
	dir := t.TempDir()
	profile := simpleProfile()
	ff := &fakeFactory{runner: &fakePerchRunner{}}

	tests := []struct {
		name           string
		factory        PerchFactory
		runDirBase     string
		scratchDirBase string
		stencilsDir    string
		runIDPrefix    string
	}{
		{"NilFactory", nil, filepath.Join(dir, "r"), filepath.Join(dir, "s"), filepath.Join(dir, "st"), "prefix"},
		{"EmptyRunDirBase", ff.factory(), "", filepath.Join(dir, "s"), filepath.Join(dir, "st"), "prefix"},
		{"EmptyScratchDirBase", ff.factory(), filepath.Join(dir, "r"), "", filepath.Join(dir, "st"), "prefix"},
		{"EmptyStencilsDir", ff.factory(), filepath.Join(dir, "r"), filepath.Join(dir, "s"), "", "prefix"},
		{"InvalidPrefix", ff.factory(), filepath.Join(dir, "r"), filepath.Join(dir, "s"), filepath.Join(dir, "st"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPerchProducer("gate", tt.factory, profile, tt.runDirBase, tt.scratchDirBase, tt.stencilsDir, tt.runIDPrefix)
			if err == nil {
				t.Fatal("NewPerchProducer() error = nil; want non-nil")
			}
		})
	}
}

func TestPerchProducer_NilRunnerFromFactoryIsError(t *testing.T) {
	dir := t.TempDir()
	ff := &fakeFactory{nilRunner: true}
	runDirBase := filepath.Join(dir, "runs")
	scratchDirBase := filepath.Join(dir, "scratch")
	p, err := NewPerchProducer("gate", ff.factory(), simpleProfile(), runDirBase, scratchDirBase, filepath.Join(dir, "stencils"), "myprefix")
	if err != nil {
		t.Fatalf("NewPerchProducer() error = %v; want nil", err)
	}

	_, _, callErr := p.Call(context.Background())
	if callErr == nil {
		t.Fatal("Call() error = nil; want non-nil (nil PerchRunner from factory)")
	}
}
