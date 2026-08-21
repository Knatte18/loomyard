// engine_test.go drives Engine.Run against a scripted fakeRunner (RoundRunner) and a scripted
// queuedShuttle (judge/triage/targeting), adapted to the attempt-level RoundRunner seam.
// Deliberately scoped to the seam contract itself: that the generalized loop works against a
// non-burler runner, AttemptInput population and hydration, retry/triage semantics at the seam,
// name-parameterized diagnostics for an arbitrary caller name, ladder + gate parity, profile
// validation, and the profile-gated pre-round targeting capability.
// Untagged file: no spawning — the fake CommandRunner is an in-process func (Test Tier Purity
// Invariant).

package treadleengine

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// queuedAttemptResult is one scripted (AttemptResult, error) pair
// fakeRunner.RunAttempt dequeues.
type queuedAttemptResult struct {
	result AttemptResult
	err    error
}

// fakeRunner is a same-package RoundRunner double: RunAttempt records every
// AttemptInput it receives, in call order, and dequeues the next scripted
// AttemptResult (or error). For a scripted done result it echoes
// in.ReviewPath/in.FixerReportPath onto the result exactly as a real
// adapter would (the runner never invents its own paths).
type fakeRunner struct {
	calls []AttemptInput
	queue []queuedAttemptResult
	// onCall, when non-nil, runs at the top of every RunAttempt with the
	// incoming AttemptInput — a test's hook for mutating on-disk state
	// mid-block at a precise round boundary (e.g. corrupting an
	// already-recorded handoff file before a later round runs).
	onCall func(in AttemptInput)
}

func (f *fakeRunner) RunAttempt(in AttemptInput) (AttemptResult, error) {
	f.calls = append(f.calls, in)
	if f.onCall != nil {
		f.onCall(in)
	}
	if len(f.queue) == 0 {
		return AttemptResult{}, fmt.Errorf("fakeRunner: no scripted result for call %d", len(f.calls))
	}
	next := f.queue[0]
	f.queue = f.queue[1:]
	if next.err != nil {
		return AttemptResult{}, next.err
	}
	result := next.result
	if result.Outcome == shuttleengine.OutcomeDone {
		result.ReviewPath = in.ReviewPath
		result.FixerReportPath = in.FixerReportPath
	}
	return result, nil
}

// queuedShuttleEntry is one scripted judge/triage verdict-file content (or
// error) queuedShuttle.Run dequeues. handoffContent, when non-empty, is
// written to a judge call's second OutputFiles entry (the handoff path) —
// a triage call's Spec has only one OutputFiles entry, so handoffContent is
// simply unused for those scripted entries.
type queuedShuttleEntry struct {
	verdictContent string
	handoffContent string
	err            error
}

// queuedShuttle is a same-package Shuttle double for the judge/triage
// calls: Run records every Spec it receives, dequeues the next scripted
// verdict file content (or error), writes it to the Spec's sole
// OutputFiles entry when non-empty, and returns a scripted done Result.
type queuedShuttle struct {
	specs []shuttleengine.Spec
	queue []queuedShuttleEntry
}

func (q *queuedShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	q.specs = append(q.specs, spec)
	if len(q.queue) == 0 {
		return shuttleengine.Result{}, fmt.Errorf("queuedShuttle: no scripted result for call %d", len(q.specs))
	}
	next := q.queue[0]
	q.queue = q.queue[1:]
	if next.err != nil {
		return shuttleengine.Result{}, next.err
	}
	if err := os.WriteFile(spec.OutputFiles[0], []byte(next.verdictContent), 0o644); err != nil {
		return shuttleengine.Result{}, err
	}
	if next.handoffContent != "" {
		if err := os.WriteFile(spec.OutputFiles[1], []byte(next.handoffContent), 0o644); err != nil {
			return shuttleengine.Result{}, err
		}
	}
	return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
}

// recordedCommand is one invocation fakeCommandRunner.run recorded.
type recordedCommand struct {
	argv    []string
	dir     string
	timeout time.Duration
}

// queuedCommandResult is one scripted (output, exitZero, error) triple
// fakeCommandRunner.run dequeues.
type queuedCommandResult struct {
	output   []byte
	exitZero bool
	err      error
}

// fakeCommandRunner is a scripted CommandRunner double: run records argv,
// dir, and timeout, and dequeues the next scripted result — an in-process
// func, never a real spawn (Test Tier Purity Invariant).
type fakeCommandRunner struct {
	calls []recordedCommand
	queue []queuedCommandResult
}

func (f *fakeCommandRunner) run(argv []string, dir string, timeout time.Duration) ([]byte, bool, error) {
	f.calls = append(f.calls, recordedCommand{argv: argv, dir: dir, timeout: timeout})
	if len(f.queue) == 0 {
		return nil, false, fmt.Errorf("fakeCommandRunner: no scripted result for call %d", len(f.calls))
	}
	next := f.queue[0]
	f.queue = f.queue[1:]
	return next.output, next.exitZero, next.err
}

// verdictFileContent renders a judge/triage verdict file's frontmatter.
func verdictFileContent(verdict, rationale string) string {
	return fmt.Sprintf("---\nverdict: %s\nrationale: %s\n---\n", verdict, rationale)
}

// stringSlicesEqual2 reports whether a and b contain the same strings in
// the same order. Named with a numeric suffix to avoid colliding with a
// same-named helper elsewhere in the tree — there is no cross-package
// sharing; each copy is a few lines, per the differential-test-bar's
// mechanical-package-split helper-fallout clause.
func stringSlicesEqual2(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestEngine_AttemptInputPopulation proves AttemptInput is populated correctly per attempt:
// round/attempt numbers and the RoundToken letter suffix on a retry, tuning passthrough
// (Model/Effort/Timeout), and hydration accumulating across rounds — including a failed gate file
// fed forward — with both attempts of the same round seeing IDENTICAL hydration (a retry produces
// no new completed round).
func TestEngine_AttemptInputPopulation(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	gateDir := t.TempDir()

	fr := &fakeRunner{}
	fr.queue = []queuedAttemptResult{
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDied, SessionID: "died-1", RunDir: "/kept/died-1"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s2"}},
	}
	fcr := &fakeCommandRunner{}
	fcr.queue = []queuedCommandResult{
		{output: []byte("fail"), exitZero: false}, // round 1's gate fails
		{output: []byte("ok"), exitZero: true},    // round 2's gate passes
	}

	p := Profile{
		ProfileHash: "hash-1",
		Gate:        Gate{Mode: GateCommand, Command: []string{"make", "test"}},
		GateDir:     gateDir,
		RoundCaps:   []int{5},
		Model:       "opus",
		Effort:      "high",
		Timeout:     5 * time.Minute,
	}

	e := New("gate", fr, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t), RunCommand: fcr.run})
	got, err := e.Run(p, runDir)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if got.Outcome != OutcomeApproved {
		t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("fakeRunner called %d times; want 3", len(fr.calls))
	}

	round1 := fr.calls[0]
	if round1.Round != 1 || round1.Attempt != 1 || round1.RoundToken != "1" {
		t.Errorf("round1 = %+v; want Round=1 Attempt=1 RoundToken=\"1\"", round1)
	}
	round2Attempt1 := fr.calls[1]
	round2Attempt2 := fr.calls[2]
	if round2Attempt1.Round != 2 || round2Attempt1.Attempt != 1 || round2Attempt1.RoundToken != "2" {
		t.Errorf("round2 attempt1 = %+v; want Round=2 Attempt=1 RoundToken=\"2\"", round2Attempt1)
	}
	if round2Attempt2.Round != 2 || round2Attempt2.Attempt != 2 || round2Attempt2.RoundToken != "2b" {
		t.Errorf("round2 attempt2 = %+v; want Round=2 Attempt=2 RoundToken=\"2b\" (the retry letter suffix)", round2Attempt2)
	}

	// Hydration: round 2 must see round 1's review path AND round 1's
	// failed gate file (GatePassed false feeds the gate path forward as
	// hydration); a passing gate is never fed forward (an original pin,
	// unchanged by the extraction).
	if len(round2Attempt1.PriorReviews) != 2 {
		t.Fatalf("round2 attempt1 PriorReviews = %v; want 2 entries (review + failed gate)", round2Attempt1.PriorReviews)
	}
	if round2Attempt1.PriorReviews[0] != round1.ReviewPath {
		t.Errorf("round2 PriorReviews[0] = %q; want round1's review path %q", round2Attempt1.PriorReviews[0], round1.ReviewPath)
	}
	if !strings.Contains(round2Attempt1.PriorReviews[1], "gate") {
		t.Errorf("round2 PriorReviews[1] = %q; want it to name the round-1 gate file", round2Attempt1.PriorReviews[1])
	}
	if len(round2Attempt1.PriorFixerReports) != 1 || round2Attempt1.PriorFixerReports[0] != round1.FixerReportPath {
		t.Errorf("round2 PriorFixerReports = %v; want [%q]", round2Attempt1.PriorFixerReports, round1.FixerReportPath)
	}
	if !stringSlicesEqual2(round2Attempt1.PriorReviews, round2Attempt2.PriorReviews) {
		t.Errorf("round2 attempt2 PriorReviews = %v; want identical to attempt1's %v (a retry reuses the same hydration)", round2Attempt2.PriorReviews, round2Attempt1.PriorReviews)
	}
	if !stringSlicesEqual2(round2Attempt1.PriorFixerReports, round2Attempt2.PriorFixerReports) {
		t.Errorf("round2 attempt2 PriorFixerReports = %v; want identical to attempt1's %v", round2Attempt2.PriorFixerReports, round2Attempt1.PriorFixerReports)
	}

	// Tuning passthrough: every attempt carries Profile's Model/Effort/
	// Timeout unchanged.
	for i, call := range fr.calls {
		if call.Model != "opus" || call.Effort != "high" || call.Timeout != 5*time.Minute {
			t.Errorf("call %d tuning = (Model=%q Effort=%q Timeout=%s); want (\"opus\", \"high\", 5m)", i, call.Model, call.Effort, call.Timeout)
		}
	}

	// The gate command's cwd is always Profile.GateDir — the caller-supplied
	// absolute path that keeps treadle itself geometry-blind.
	for i, call := range fcr.calls {
		if call.dir != gateDir {
			t.Errorf("gate command %d dir = %q; want Profile.GateDir %q", i, call.dir, gateDir)
		}
	}
}

// TestEngine_RetrySemantics proves the seam's retry policy: a second consecutive non-done attempt
// is a name-prefixed hard error (never STUCK),
// and an asking outcome's triage call determines whether the round retries (RETRY) or the block
// errors (GIVE_UP).
func TestEngine_RetrySemantics(t *testing.T) {
	t.Run("second consecutive died is a name-prefixed hard error", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDied, SessionID: "died-1", RunDir: "/kept/died-1"}},
			{result: AttemptResult{Outcome: shuttleengine.OutcomeTimeout, SessionID: "died-2", RunDir: "/kept/died-2"}},
		}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
		e := New("gate", fr, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t)})

		_, err := e.Run(p, runDir)
		if err == nil {
			t.Fatal("Run() error = nil; want an error for two consecutive non-done attempts")
		}
		if !strings.HasPrefix(err.Error(), "gate: ") {
			t.Errorf("Run() error = %q; want a \"gate: \"-prefixed message", err.Error())
		}
		if !strings.Contains(err.Error(), "failed twice") || !strings.Contains(err.Error(), "died-2") || !strings.Contains(err.Error(), "/kept/died-2") {
			t.Errorf("Run() error = %q; want it to carry \"failed twice\" and the second attempt's session id and kept run dir", err.Error())
		}
	})

	t.Run("asking with triage RETRY re-attempts the round", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeAsking, SessionID: "ask-1", RunDir: "/kept/ask-1", LastAssistantMessage: "should I proceed?"}},
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s2"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []queuedShuttleEntry{
			{verdictContent: verdictFileContent(string(TriageRetry), "plausibly proceeds")},
		}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Rounds[0].Attempts != 2 {
			t.Errorf("Rounds[0].Attempts = %d; want 2", got.Rounds[0].Attempts)
		}
		if len(qs.specs) != 1 || qs.specs[0].Role != "triage" {
			t.Errorf("queuedShuttle specs = %+v; want exactly one triage spec", qs.specs)
		}
		if got.Rounds[0].TriagePath == "" {
			t.Error("Rounds[0].TriagePath is empty; want the triage verdict path mirrored onto the Result")
		}
	})

	t.Run("asking with triage GIVE_UP errors carrying the rationale", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeAsking, SessionID: "ask-1", RunDir: "/kept/ask-1", LastAssistantMessage: "the fasit file does not exist"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []queuedShuttleEntry{
			{verdictContent: verdictFileContent(string(TriageGiveUp), "the fasit file referenced does not exist")},
		}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		_, err := e.Run(p, runDir)
		if err == nil {
			t.Fatalf("Run() error = nil; want an error carrying the triage rationale")
		}
		if !strings.Contains(err.Error(), "the fasit file referenced does not exist") {
			t.Errorf("Run() error = %q; want it to carry the triage rationale", err.Error())
		}
		if !strings.Contains(err.Error(), "ask-1") || !strings.Contains(err.Error(), "/kept/ask-1") {
			t.Errorf("Run() error = %q; want it to carry the session id and kept run dir", err.Error())
		}
	})
}

// TestEngine_NameParameterization proves an Engine constructed with an arbitrary caller name produces
// diagnostics carrying that name's prefix — even for the ErrBlockBusy sentinel wrap — and that
// errors.Is still matches the shared sentinel regardless of which name produced the wrap.
func TestEngine_NameParameterization(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")

	// blockingRunner's single RunAttempt call blocks until release is
	// closed, standing in for a real round-runner still in flight when a
	// second Run call arrives against the same run dir.
	release := make(chan struct{})
	fr1 := &blockingRunner{entered: make(chan struct{}), release: release}

	p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
	e1 := New("tenter", fr1, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t)})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = e1.Run(p, runDir)
	}()

	select {
	case <-fr1.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("first Run() never entered its blocking attempt")
	}

	fr2 := &fakeRunner{}
	e2 := New("tenter", fr2, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t)})
	_, err := e2.Run(p, runDir)
	if err == nil {
		t.Fatal("second Run() error = nil; want an already-running error while the first Run holds the run dir")
	}
	if !strings.HasPrefix(err.Error(), "tenter: ") {
		t.Errorf("second Run() error = %q; want a \"tenter: \"-prefixed message", err.Error())
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("second Run() error = %q; want it to name the block as already running", err.Error())
	}
	if !errors.Is(err, ErrBlockBusy) {
		t.Errorf("second Run() error = %v; want errors.Is(err, ErrBlockBusy) regardless of the calling engine's name", err)
	}
	if len(fr2.calls) != 0 {
		t.Errorf("fr2 called %d times; want 0 (the second Run must never touch the runner)", len(fr2.calls))
	}

	close(release)
	<-done
}

// blockingRunner is a same-package RoundRunner double whose single
// RunAttempt call signals entered, then blocks until release is closed —
// standing in for a real round-runner attempt still in flight, so a
// concurrency test can deterministically observe "the first Run call holds
// the lock" without a timing-based sleep.
type blockingRunner struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingRunner) RunAttempt(AttemptInput) (AttemptResult, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return AttemptResult{}, errors.New("blockingRunner: released without a scripted result")
}

// TestEngine_LadderAndGateParity smoke-tests the milestone ladder and pluggable gate carried
// forward unchanged by the extraction: an unconditional hard-cap STUCK, a milestone rung's STOP
// verdict, and GateCommand convergence via a fake CommandRunner that observes Profile.GateDir as
// its cwd argument.
func TestEngine_LadderAndGateParity(t *testing.T) {
	t.Run("hard cap stops unconditionally with no judge call", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
		}
		qs := &queuedShuttle{}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{1}}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeStuck || got.StuckReason != StuckHardCap {
			t.Fatalf("Run() = (%q, %q); want (%q, %q)", got.Outcome, got.StuckReason, OutcomeStuck, StuckHardCap)
		}
		if len(qs.specs) != 0 {
			t.Errorf("queuedShuttle called %d times; want 0 (no judge call at the hard-cap round)", len(qs.specs))
		}
	})

	t.Run("milestone STOP halts immediately at a non-final rung", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []queuedShuttleEntry{
			{verdictContent: verdictFileContent(string(JudgeStop), "the trajectory does not justify continuing")},
		}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{1, 3}}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeStuck || got.StuckReason != StuckMilestoneStop {
			t.Fatalf("Run() = (%q, %q); want (%q, %q)", got.Outcome, got.StuckReason, OutcomeStuck, StuckMilestoneStop)
		}
		if got.RoundsRun != 1 {
			t.Fatalf("Run() RoundsRun = %d; want 1", got.RoundsRun)
		}
		if len(qs.specs) != 1 {
			t.Errorf("queuedShuttle called %d times; want exactly 1 (the milestone rung's judge call)", len(qs.specs))
		}
	})

	t.Run("GateCommand convergence observes Profile.GateDir as the command's cwd", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		gateDir := t.TempDir()
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
		}
		fcr := &fakeCommandRunner{}
		fcr.queue = []queuedCommandResult{{output: []byte("ok"), exitZero: true}}
		p := Profile{
			ProfileHash: "hash-1",
			Gate:        Gate{Mode: GateCommand, Command: []string{"make", "test"}},
			GateDir:     gateDir,
			RoundCaps:   []int{10},
		}
		e := New("gate", fr, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t), RunCommand: fcr.run})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q (a passing command converges even on a BLOCKING verdict)", got.Outcome, OutcomeApproved)
		}
		if len(fcr.calls) != 1 || fcr.calls[0].dir != gateDir {
			t.Fatalf("fakeCommandRunner calls = %+v; want exactly one call with dir %q", fcr.calls, gateDir)
		}
	})
}

// TestEngine_ProfileValidation proves Run's fail-loud profile checks reject a structurally invalid
// Profile before ever touching the runner: an empty ProfileHash, a non-increasing RoundCaps ladder,
// and an illegal Gate.Mode.
func TestEngine_ProfileValidation(t *testing.T) {
	tests := []struct {
		name      string
		profile   Profile
		errSubstr string
	}{
		{
			name:      "empty ProfileHash",
			profile:   Profile{Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{5}},
			errSubstr: "ProfileHash must not be empty",
		},
		{
			name:      "non-increasing RoundCaps",
			profile:   Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{5, 5, 10}},
			errSubstr: "must be strictly increasing",
		},
		{
			name:      "illegal Gate.Mode",
			profile:   Profile{ProfileHash: "hash-1", Gate: Gate{Mode: "bogus"}, RoundCaps: []int{5}},
			errSubstr: "Gate.Mode must be",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := filepath.Join(t.TempDir(), "run")
			fr := &fakeRunner{}
			e := New("gate", fr, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t)})

			_, err := e.Run(tt.profile, runDir)
			if err == nil {
				t.Fatalf("Run() error = nil; want an error containing %q", tt.errSubstr)
			}
			if !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("Run() error = %q; want substring %q", err.Error(), tt.errSubstr)
			}
			if !strings.HasPrefix(err.Error(), "gate: ") {
				t.Errorf("Run() error = %q; want a \"gate: \"-prefixed message", err.Error())
			}
			if len(fr.calls) != 0 {
				t.Errorf("fakeRunner called %d times; want 0 (an invalid profile must never reach the runner)", len(fr.calls))
			}
		})
	}
}

// readRunState reads runDir's persisted state.json, failing the test loudly
// if it is missing or unreadable. Same-package access to the unexported
// runState/roundRecord types means this is a direct read of the real
// persisted shape — no test-local mirror struct needed, unlike a reader
// that would have to mirror the shape across a package boundary.
func readRunState(t *testing.T, runDir string) runState {
	t.Helper()
	path := filepath.Join(runDir, stateFileName)
	lockPath := path + ".lock"
	got, found, err := state.ReadJSON[runState](path, lockPath)
	if err != nil {
		t.Fatalf("state.ReadJSON(%q) = %v; want nil", path, err)
	}
	if !found {
		t.Fatalf("state.ReadJSON(%q) found = false; want true", path)
	}
	return got
}

// TestEngine_HandoffLifecycle_RecordedOnlyWhenProduced proves (a): a judge round's HandoffPath is
// set only when the shuttle fake actually wrote a valid handoff file alongside its verdict.
// A judge call whose verdict succeeds but never produces a handoff file still records the round's
// JudgeVerdict — the two are independent — and leaves HandoffPath empty, never an error and never
// STUCK from the missing handoff alone.
func TestEngine_HandoffLifecycle_RecordedOnlyWhenProduced(t *testing.T) {
	newProfile := func() Profile {
		return Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
	}

	t.Run("handoff file produced alongside the verdict", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s2"}},
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s3"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []queuedShuttleEntry{
			// Round 2's circling check: round 1 never runs a judge (no prior
			// round to compare it against yet).
			{
				verdictContent: verdictFileContent(string(JudgeProgressing), "still moving"),
				handoffContent: "---\ncovers_rounds: [1, 2]\nledger: []\n---\n\nstill moving.\n",
			},
		}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		got, err := e.Run(newProfile(), runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
		}

		st := readRunState(t, runDir)
		if len(st.Rounds) < 2 {
			t.Fatalf("st.Rounds = %+v; want at least 2 rounds", st.Rounds)
		}
		if st.Rounds[1].JudgeVerdict != string(JudgeProgressing) {
			t.Errorf("Rounds[1].JudgeVerdict = %q; want %q", st.Rounds[1].JudgeVerdict, JudgeProgressing)
		}
		wantHandoffPath := qs.specs[0].OutputFiles[1]
		if st.Rounds[1].HandoffPath != wantHandoffPath {
			t.Errorf("Rounds[1].HandoffPath = %q; want %q (the produced handoff file)", st.Rounds[1].HandoffPath, wantHandoffPath)
		}
	})

	t.Run("no handoff file produced leaves HandoffPath empty", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s2"}},
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s3"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []queuedShuttleEntry{
			// A real verdict, but the fake never writes the handoff file —
			// exactly what a genuine agent turn-limit or write failure looks
			// like from the loop's perspective.
			{verdictContent: verdictFileContent(string(JudgeProgressing), "still moving")},
		}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		got, err := e.Run(newProfile(), runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q (a missing handoff must never affect the verdict outcome)", got.Outcome, OutcomeApproved)
		}

		st := readRunState(t, runDir)
		if len(st.Rounds) < 2 {
			t.Fatalf("st.Rounds = %+v; want at least 2 rounds", st.Rounds)
		}
		if st.Rounds[1].JudgeVerdict != string(JudgeProgressing) {
			t.Errorf("Rounds[1].JudgeVerdict = %q; want %q (the verdict itself still records)", st.Rounds[1].JudgeVerdict, JudgeProgressing)
		}
		if st.Rounds[1].HandoffPath != "" {
			t.Errorf("Rounds[1].HandoffPath = %q; want empty (no handoff file was ever written)", st.Rounds[1].HandoffPath)
		}
	})
}

// TestEngine_HandoffLifecycle_ReadSetCoverage proves (b), (c), and the
// first-call half of (e) together, in one flowing GateCommand scenario
// chosen so a round can fail to converge on a passing VERDICT alone (an
// APPROVED-but-gate-failing round never runs a judge at all, and neither
// does the BLOCKING round immediately after it — both are judge-skipped
// rounds that must still be fed forward):
//
//   - round 1: APPROVED, gate fails — no judge (Verdict != Blocking).
//   - round 2: BLOCKING, gate fails — no judge (prevRoundApproved exemption).
//   - round 3: BLOCKING, gate fails — FIRST judge call. No previous handoff
//     exists yet, so (e): the read-set degrades to exactly all completed
//     rounds' reviews — proving rounds 1 and 2's judge-skipped reviews are
//     fed forward to this first call. Its scripted handoff covers rounds
//     1-3.
//   - round 4: BLOCKING, gate fails — SECOND judge call. (b): its
//     previous_handoff marker carries round 3's recorded handoff path, and
//     its read-set excludes rounds 1-3 (all covered) and carries only round
//     4's own fresh review. (c): covers_rounds absorbing the judge-skipped
//     rounds 1 and 2 (transitively, via round 3's handoff) is what keeps
//     them out of round 4's read-set too, never resurfacing.
//   - round 5: BLOCKING, gate passes — converges; no third judge call.
func TestEngine_HandoffLifecycle_ReadSetCoverage(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	gateDir := t.TempDir()

	fr := &fakeRunner{}
	fr.queue = []queuedAttemptResult{
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s1"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s2"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s3"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s4"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s5"}},
	}
	fcr := &fakeCommandRunner{}
	fcr.queue = []queuedCommandResult{
		{output: []byte("fail"), exitZero: false},
		{output: []byte("fail"), exitZero: false},
		{output: []byte("fail"), exitZero: false},
		{output: []byte("fail"), exitZero: false},
		{output: []byte("ok"), exitZero: true},
	}
	qs := &queuedShuttle{}
	qs.queue = []queuedShuttleEntry{
		// Round 3's circling check — the block's first-ever judge call.
		{
			verdictContent: verdictFileContent(string(JudgeProgressing), "still moving"),
			handoffContent: "---\ncovers_rounds: [1, 2, 3]\nledger: []\n---\n\nstill moving.\n",
		},
		// Round 4's circling check — the "subsequent call" half.
		{verdictContent: verdictFileContent(string(JudgeProgressing), "still moving")},
	}

	p := Profile{
		ProfileHash: "hash-1",
		Gate:        Gate{Mode: GateCommand, Command: []string{"make", "test"}},
		GateDir:     gateDir,
		RoundCaps:   []int{10},
	}
	e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t), RunCommand: fcr.run})

	got, err := e.Run(p, runDir)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if got.Outcome != OutcomeApproved {
		t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
	}
	if len(qs.specs) != 2 {
		t.Fatalf("queuedShuttle called %d times; want exactly 2 (rounds 3 and 4's circling checks)", len(qs.specs))
	}

	// (e): the first judge call (round 3) had no previous handoff, so its
	// read-set is exactly today's all-reviews behavior — which is what
	// feeds rounds 1 and 2's judge-skipped reviews forward at all.
	firstCall := qs.specs[0]
	if !strings.Contains(firstCall.Prompt, "(none)") {
		t.Errorf("round 3 judge prompt = %q; want the previous_handoff marker to read \"(none)\"", firstCall.Prompt)
	}
	for _, want := range []string{"round-1-review.md", "round-2-review.md", "round-3-review.md"} {
		if !strings.Contains(firstCall.Prompt, want) {
			t.Errorf("round 3 judge prompt = %q; want it to list %q (no valid handoff yet degrades to all reviews)", firstCall.Prompt, want)
		}
	}
	recordedHandoffPath := firstCall.OutputFiles[1]

	// (b) + (c): the second judge call (round 4) reads {round 3's handoff +
	// uncovered reviews} — rounds 1-3 are all covered (1 and 2
	// transitively, via round 3's own all-reviews read-set), so only round
	// 4's fresh review remains, and previous_handoff carries the handoff's
	// own path.
	secondCall := qs.specs[1]
	if !strings.Contains(secondCall.Prompt, recordedHandoffPath) {
		t.Errorf("round 4 judge prompt = %q; want the previous_handoff marker to carry %q", secondCall.Prompt, recordedHandoffPath)
	}
	for _, unwanted := range []string{"round-1-review.md", "round-2-review.md", "round-3-review.md"} {
		if strings.Contains(secondCall.Prompt, unwanted) {
			t.Errorf("round 4 judge prompt = %q; want %q excluded (covered by the round-3 handoff)", secondCall.Prompt, unwanted)
		}
	}
	if !strings.Contains(secondCall.Prompt, "round-4-review.md") {
		t.Errorf("round 4 judge prompt = %q; want it to list round-4-review.md (its own fresh review, never covered)", secondCall.Prompt)
	}

	st := readRunState(t, runDir)
	if len(st.Rounds) != 5 {
		t.Fatalf("st.Rounds = %+v; want 5 rounds", st.Rounds)
	}
	if st.Rounds[0].JudgeVerdict != "" || st.Rounds[1].JudgeVerdict != "" {
		t.Errorf("Rounds[0].JudgeVerdict/Rounds[1].JudgeVerdict = %q/%q; want both empty (no judge ran either round)", st.Rounds[0].JudgeVerdict, st.Rounds[1].JudgeVerdict)
	}
	if st.Rounds[2].HandoffPath != recordedHandoffPath {
		t.Errorf("Rounds[2].HandoffPath = %q; want %q", st.Rounds[2].HandoffPath, recordedHandoffPath)
	}
}

// TestEngine_HandoffLifecycle_InvalidHandoffFallback proves (d): a recorded handoff whose file
// content is corrupted (fails ParseHandoff) is a fail-safe skip, never a hard stop.
// The loop simply behaves as if that round's handoff had never been produced — the next judge
// call's read-set falls back to the older-or-all-reviews list,
// and the block continues (never STUCK, never an error) purely from the handoff machinery.
func TestEngine_HandoffLifecycle_InvalidHandoffFallback(t *testing.T) {
	// These fail-safe Warns reach an operator's stderr on an ordinary run
	// (logger's default threshold IS Warn), so the engine is named "tenter"
	// here rather than this file's usual "gate": that proves the
	// corrupt-handoff Warns carry the CALLING engine's name like every other
	// Warn this package emits, instead of a hardcoded package label the
	// operator would never recognize.
	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	runDir := filepath.Join(t.TempDir(), "run")

	fr := &fakeRunner{}
	fr.queue = []queuedAttemptResult{
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s2"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s3"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s4"}},
	}
	qs := &queuedShuttle{}
	qs.queue = []queuedShuttleEntry{
		// Round 2's circling check: a real verdict, but a corrupted (not
		// even frontmatter-shaped) handoff file.
		{
			verdictContent: verdictFileContent(string(JudgeProgressing), "still moving"),
			handoffContent: "this is not YAML frontmatter at all",
		},
		// Round 3's circling check: must fall back to the all-reviews
		// read-set, exactly as if round 2 had recorded no handoff.
		{verdictContent: verdictFileContent(string(JudgeProgressing), "still moving")},
	}
	p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
	e := New("tenter", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

	got, err := e.Run(p, runDir)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (a corrupted handoff must never surface as an engine error)", err)
	}
	if got.Outcome != OutcomeApproved {
		t.Fatalf("Run() Outcome = %q; want %q (a corrupted handoff must never cause STUCK)", got.Outcome, OutcomeApproved)
	}

	st := readRunState(t, runDir)
	if len(st.Rounds) < 2 || st.Rounds[1].HandoffPath != "" {
		t.Fatalf("Rounds[1].HandoffPath = %q; want empty (the corrupted handoff must never be recorded as valid)", st.Rounds[1].HandoffPath)
	}

	secondCall := qs.specs[1]
	if !strings.Contains(secondCall.Prompt, "(none)") {
		t.Errorf("round 3 judge prompt = %q; want the previous_handoff marker to read \"(none)\" (the round-2 handoff was corrupted)", secondCall.Prompt)
	}
	if !strings.Contains(secondCall.Prompt, "round-1-review.md") || !strings.Contains(secondCall.Prompt, "round-2-review.md") {
		t.Errorf("round 3 judge prompt = %q; want it to list both round-1-review.md and round-2-review.md (fallback to all reviews)", secondCall.Prompt)
	}

	// The refusal to record the corrupted handoff must announce itself under
	// the calling engine's name.
	logged := logBuf.String()
	if !strings.Contains(logged, "tenter: circling judge handoff file unparseable") {
		t.Errorf("captured log = %q; want a \"tenter: \"-prefixed unparseable-handoff Warn", logged)
	}
}

// TestEngine_HandoffLifecycle_NoValidHandoffDegradesToAllReviews proves (e) as its own minimal
// case: with no handoff ever produced at all, a judge call's read-set is exactly today's
// all-reviews list — the degrade path a block with handoff-maintenance disabled (or simply never
// yet exercised) must still behave identically to the pre-handoff loop.
func TestEngine_HandoffLifecycle_NoValidHandoffDegradesToAllReviews(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")

	fr := &fakeRunner{}
	fr.queue = []queuedAttemptResult{
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s2"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s3"}},
	}
	qs := &queuedShuttle{}
	qs.queue = []queuedShuttleEntry{
		{verdictContent: verdictFileContent(string(JudgeProgressing), "still moving")},
	}
	p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
	e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

	got, err := e.Run(p, runDir)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if got.Outcome != OutcomeApproved {
		t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
	}
	if len(qs.specs) != 1 {
		t.Fatalf("queuedShuttle called %d times; want exactly 1", len(qs.specs))
	}

	got1 := qs.specs[0]
	if !strings.Contains(got1.Prompt, "(none)") {
		t.Errorf("judge prompt = %q; want the previous_handoff marker to read \"(none)\"", got1.Prompt)
	}
	if !strings.Contains(got1.Prompt, "round-1-review.md") {
		t.Errorf("judge prompt = %q; want it to list round-1-review.md — exactly the all-reviews list", got1.Prompt)
	}
}

// TestEngine_JudgeSkippedRoundReadsNoHandoffFiles proves the judge read-set is assembled lazily,
// only inside the two judge branches: a judge-skipped BLOCKING round — here, the round immediately
// after an APPROVED round under a command gate — must neither walk recorded handoff files on disk
// nor emit their corrupt-handoff Warns, since no judge call happens.
// The recorded handoff is corrupted mid-block, at the exact judge-skipped round's attempt
// (fakeRunner.onCall), so any handoff read on that round would surface as a "recorded handoff file
// unparseable" Warn in the captured log — the pre-fix behavior.
func TestEngine_JudgeSkippedRoundReadsNoHandoffFiles(t *testing.T) {
	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	runDir := filepath.Join(t.TempDir(), "run")
	gateDir := t.TempDir()

	fr := &fakeRunner{}
	fr.queue = []queuedAttemptResult{
		// r1 BLOCKING, gate fails — round 1 never judges.
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
		// r2 BLOCKING, gate fails — circling judge runs and records a VALID handoff.
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s2"}},
		// r3 APPROVED, gate fails — no judge (verdict approved), block loops.
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s3"}},
		// r4 BLOCKING, gate fails — judge-SKIPPED (prevRoundApproved); the
		// onCall hook below corrupts the recorded handoff just before this
		// round runs, so any handoff read here would Warn.
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s4"}},
		// r5 APPROVED, gate passes — converges.
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s5"}},
	}
	// Corrupt round 2's recorded handoff at the moment round 4's attempt
	// starts: recorded (and valid) at judge time, unreadable garbage by the
	// time the judge-skipped round could wrongly walk it.
	fr.onCall = func(in AttemptInput) {
		if in.Round == 4 {
			handoffPath := artifactPaths(runDir, 2, 1).Handoff
			if err := os.WriteFile(handoffPath, []byte("no frontmatter garbage\n"), 0o644); err != nil {
				t.Errorf("corrupt recorded handoff: %v", err)
			}
		}
	}

	qs := &queuedShuttle{}
	qs.queue = []queuedShuttleEntry{
		// r2's circling judge: real verdict + valid handoff covering [1, 2].
		{
			verdictContent: verdictFileContent(string(JudgeProgressing), "still moving"),
			handoffContent: "---\ncovers_rounds: [1, 2]\nledger: []\n---\n\nstill moving.\n",
		},
	}
	fcr := &fakeCommandRunner{}
	fcr.queue = []queuedCommandResult{
		{output: []byte("fail"), exitZero: false},
		{output: []byte("fail"), exitZero: false},
		{output: []byte("fail"), exitZero: false},
		{output: []byte("fail"), exitZero: false},
		{output: []byte("ok"), exitZero: true},
	}

	p := Profile{
		ProfileHash: "hash-1",
		Gate:        Gate{Mode: GateCommand, Command: []string{"fake"}, Timeout: time.Minute},
		GateDir:     gateDir,
		RoundCaps:   []int{10},
	}
	e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t), RunCommand: fcr.run})

	got, err := e.Run(p, runDir)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if got.Outcome != OutcomeApproved {
		t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
	}
	// Exactly one judge call happened (round 2's) — rounds 1, 3, 4, and 5
	// all skip the judge, round 4 by the prevRoundApproved exemption.
	if len(qs.specs) != 1 {
		t.Fatalf("queuedShuttle called %d times; want exactly 1 (only round 2 judges)", len(qs.specs))
	}
	// The load-bearing assertion: the judge-skipped round 4 must not have
	// walked the (by then corrupt) recorded handoff — no handoff Warn may
	// appear in the log. Pre-fix, judgeReadSet ran before the judge-branch
	// switch and this corrupt read logged "recorded handoff file
	// unparseable" for a judge call that never happened.
	if logged := logBuf.String(); strings.Contains(logged, "recorded handoff") {
		t.Errorf("judge-skipped round walked recorded handoff files; log:\n%s", logged)
	}
}

// TestEngine_RecordedHandoffCorruptedMidBlockWarnsUnderCallerName covers the one fail-safe path
// that only fires when a handoff was VALID at record time and became unreadable afterwards:
// latestValidHandoff's newest-to-oldest walk.
// Round 2 records a well-formed handoff;
// the fakeRunner hook corrupts that file just before round 3's attempt, so round 3's circling judge
// finds a recorded-but-broken handoff, skips it, and degrades its read-set.
// These Warns reach an operator's stderr at logger's default threshold during an ordinary run, so
// they must carry the CALLING engine's name — the engine is named "tenter" here precisely so a
// hardcoded package label would fail.
func TestEngine_RecordedHandoffCorruptedMidBlockWarnsUnderCallerName(t *testing.T) {
	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	runDir := filepath.Join(t.TempDir(), "run")

	fr := &fakeRunner{}
	fr.queue = []queuedAttemptResult{
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s2"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s3"}},
		{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s4"}},
	}
	fr.onCall = func(in AttemptInput) {
		if in.Round == 3 {
			handoffPath := artifactPaths(runDir, 2, 1).Handoff
			if err := os.WriteFile(handoffPath, []byte("no frontmatter garbage\n"), 0o644); err != nil {
				t.Errorf("corrupt recorded handoff: %v", err)
			}
		}
	}

	qs := &queuedShuttle{}
	qs.queue = []queuedShuttleEntry{
		// Round 2's circling judge: a real verdict and a VALID handoff, so
		// round 2's record carries a HandoffPath for round 3 to walk.
		{
			verdictContent: verdictFileContent(string(JudgeProgressing), "still moving"),
			handoffContent: "---\ncovers_rounds: [1, 2]\nledger: []\n---\n\nstill moving.\n",
		},
		// Round 3's circling judge: reached only after the walk above skipped
		// the now-corrupt round-2 handoff.
		{verdictContent: verdictFileContent(string(JudgeProgressing), "still moving")},
	}

	p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
	e := New("tenter", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

	got, err := e.Run(p, runDir)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (a corrupted recorded handoff must never surface as an engine error)", err)
	}
	if got.Outcome != OutcomeApproved {
		t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
	}

	logged := logBuf.String()
	if !strings.Contains(logged, "tenter: recorded handoff file unparseable, falling back to an older handoff") {
		t.Errorf("captured log = %q; want the fallback Warn prefixed with the calling engine's name", logged)
	}
	if strings.Contains(logged, "treadle: recorded handoff") {
		t.Errorf("captured log = %q; want no hardcoded \"treadle: \" label on this Warn — it must carry the caller's name", logged)
	}

	// With round 2's handoff skipped and no older one, round 3's read-set
	// degrades to the full all-reviews list — the documented fallback.
	secondCall := qs.specs[1]
	if !strings.Contains(secondCall.Prompt, "(none)") {
		t.Errorf("round 3 judge prompt = %q; want previous_handoff to read \"(none)\"", secondCall.Prompt)
	}
}

// TestEngine_PreRoundTargeting drives Profile.PreRoundTargeting through Engine.Run, covering
// (a)-(c) and (e) of the pre-round-targeting card's requirements;
// (d)'s "targeting shuttle error" case is covered here too, with the "non-done outcome"/"missing
// seed file" fail-safe paths covered directly against runTargeting in TestRunTargeting_FailSafe
// below (the scripted queuedShuttle fake used here has no way to produce a non-done Outcome or a
// claimed-done-but-unwritten output file).
func TestEngine_PreRoundTargeting(t *testing.T) {
	// roundsThroughHandoff scripts a fakeRunner/queuedShuttle pair that
	// carries a block through round 1 (BLOCKING, no judge — nothing to
	// compare it against yet) and round 2 (BLOCKING, triggers the circling
	// judge, which writes a valid handoff), leaving round 3 as the next
	// round every subtest below actually exercises pre-round targeting
	// against.
	roundsThroughHandoff := func() (*fakeRunner, *queuedShuttle) {
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s2"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []queuedShuttleEntry{
			{
				verdictContent: verdictFileContent(string(JudgeProgressing), "still moving"),
				handoffContent: "---\ncovers_rounds: [1, 2]\nledger: []\n---\n\nstill moving.\n",
			},
		}
		return fr, qs
	}

	t.Run("(a) flag off never issues a targeting spec and every SeedPath stays empty", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr, qs := roundsThroughHandoff()
		// Round 3 converges once a valid handoff exists from round 2, so
		// this subtest can observe whether targeting fires at the very
		// round where — with the flag on — it would have something to
		// target from.
		fr.queue = append(fr.queue, queuedAttemptResult{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s3"}})

		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
		}
		for i, call := range fr.calls {
			if call.SeedPath != "" {
				t.Errorf("fr.calls[%d].SeedPath = %q; want empty (PreRoundTargeting is off)", i, call.SeedPath)
			}
		}
		for _, spec := range qs.specs {
			if spec.Role == "targeting" {
				t.Errorf("queuedShuttle specs = %+v; want no \"targeting\" role spec (PreRoundTargeting is off)", qs.specs)
			}
		}
	})

	t.Run("(b) flag on with a valid handoff threads the seed through both attempts and records it", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr, qs := roundsThroughHandoff()
		// Round 3's first attempt dies, forcing a same-round retry — both
		// attempts must see the identical SeedPath (round-scoped, not
		// recomputed per attempt).
		fr.queue = append(fr.queue,
			queuedAttemptResult{result: AttemptResult{Outcome: shuttleengine.OutcomeDied, SessionID: "died-3a", RunDir: "/kept/died-3a"}},
			queuedAttemptResult{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s3b"}},
		)
		qs.queue = append(qs.queue, queuedShuttleEntry{verdictContent: "prioritize the auth findings; leave the docs alone"})

		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}, PreRoundTargeting: true}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
		}
		if len(fr.calls) != 4 {
			t.Fatalf("fakeRunner called %d times; want 4 (round1, round2, round3 attempt1+2)", len(fr.calls))
		}

		wantSeedPath := artifactPaths(runDir, 3, 1).Seed
		round3Attempt1, round3Attempt2 := fr.calls[2], fr.calls[3]
		if round3Attempt1.SeedPath != wantSeedPath {
			t.Errorf("round3 attempt1 SeedPath = %q; want %q (the round-scoped, attempt-1 token path)", round3Attempt1.SeedPath, wantSeedPath)
		}
		if round3Attempt2.SeedPath != wantSeedPath {
			t.Errorf("round3 attempt2 SeedPath = %q; want %q (a same-round retry reuses the identical seed path)", round3Attempt2.SeedPath, wantSeedPath)
		}
		if fr.calls[0].SeedPath != "" || fr.calls[1].SeedPath != "" {
			t.Errorf("round1/round2 SeedPath = %q/%q; want both empty (no valid handoff existed at either round)", fr.calls[0].SeedPath, fr.calls[1].SeedPath)
		}

		if len(qs.specs) != 2 {
			t.Fatalf("queuedShuttle called %d times; want 2 (round2's circling judge, round3's targeting)", len(qs.specs))
		}
		targetingSpec := qs.specs[1]
		if targetingSpec.Role != "targeting" {
			t.Errorf("qs.specs[1].Role = %q; want %q", targetingSpec.Role, "targeting")
		}
		if len(targetingSpec.OutputFiles) != 1 || targetingSpec.OutputFiles[0] != wantSeedPath {
			t.Errorf("targeting spec OutputFiles = %v; want exactly [%q]", targetingSpec.OutputFiles, wantSeedPath)
		}
		recordedHandoffPath := qs.specs[0].OutputFiles[1]
		if !strings.Contains(targetingSpec.Prompt, recordedHandoffPath) {
			t.Errorf("targeting prompt = %q; want it to carry round 2's handoff path %q", targetingSpec.Prompt, recordedHandoffPath)
		}

		st := readRunState(t, runDir)
		if len(st.Rounds) != 3 {
			t.Fatalf("st.Rounds = %+v; want 3 rounds", st.Rounds)
		}
		if st.Rounds[2].SeedPath != wantSeedPath {
			t.Errorf("Rounds[2].SeedPath = %q; want %q", st.Rounds[2].SeedPath, wantSeedPath)
		}

		content, err := os.ReadFile(wantSeedPath)
		if err != nil {
			t.Fatalf("ReadFile(%q) = %v; want nil (the fake shuttle must have written the seed)", wantSeedPath, err)
		}
		if string(content) != "prioritize the auth findings; leave the docs alone" {
			t.Errorf("seed file content = %q; want the scripted targeting brief", string(content))
		}
	})

	t.Run("(c) flag on with no handoff yet at round 1 runs no targeting call", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s1"}},
		}
		qs := &queuedShuttle{}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}, PreRoundTargeting: true}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
		}
		if len(fr.calls) != 1 || fr.calls[0].SeedPath != "" {
			t.Errorf("fr.calls = %+v; want one call with an empty SeedPath (round 1 has nothing to target from yet)", fr.calls)
		}
		if len(qs.specs) != 0 {
			t.Errorf("queuedShuttle called %d times; want 0 (no handoff exists yet, so no targeting call is issued)", len(qs.specs))
		}
	})

	t.Run("(d) a failing targeting shuttle call leaves the round running with an empty SeedPath", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr, qs := roundsThroughHandoff()
		fr.queue = append(fr.queue, queuedAttemptResult{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s3"}})
		qs.queue = append(qs.queue, queuedShuttleEntry{err: errors.New("targeting shuttle unavailable")})

		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}, PreRoundTargeting: true}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil (a fail-safe targeting failure must never surface as an engine error)", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q (targeting failure must never block round convergence)", got.Outcome, OutcomeApproved)
		}
		if len(fr.calls) != 3 || fr.calls[2].SeedPath != "" {
			t.Fatalf("fr.calls = %+v; want round3's call to carry an empty SeedPath", fr.calls)
		}

		st := readRunState(t, runDir)
		if len(st.Rounds) != 3 || st.Rounds[2].SeedPath != "" {
			t.Errorf("Rounds[2].SeedPath = %q; want empty (the failed targeting call must never be recorded)", st.Rounds[2].SeedPath)
		}
	})

	t.Run("(e) stale-artifact move-aside covers a leftover seed file on re-run", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}

		// Seed a resumable state: rounds 1-2 already recorded (round 2
		// carrying a valid handoff), round 3 is next.
		handoffPath := filepath.Join(runDir, "round-2-handoff.md")
		writeFile(t, handoffPath, "---\ncovers_rounds: [1, 2]\nledger: []\n---\n\nstill moving.\n")
		seed := runState{
			ProfileHash: "hash-1",
			RoundCaps:   []int{10},
			Rounds: []roundRecord{
				{Round: 1, Attempts: 1, Verdict: "BLOCKING"},
				{Round: 2, Attempts: 1, Verdict: "BLOCKING", JudgeVerdict: "PROGRESSING", HandoffPath: handoffPath},
			},
		}
		if err := saveState(runDir, runDir, seed); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}

		// A leftover seed file from an interrupted prior attempt at round
		// 3 — exactly what a crash between targeting's write and round 3's
		// own attempt completing would leave behind.
		leftoverSeedPath := artifactPaths(runDir, 3, 1).Seed
		writeFile(t, leftoverSeedPath, "stale seed from an interrupted run")

		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s3"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []queuedShuttleEntry{
			{verdictContent: "fresh targeting brief for round 3"},
		}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}, PreRoundTargeting: true}
		e := New("gate", fr, qs, Options{StencilsDir: newTestStencilsDir(t)})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
		}
		if len(qs.specs) != 1 || qs.specs[0].Role != "targeting" {
			t.Fatalf("queuedShuttle specs = %+v; want exactly one targeting spec", qs.specs)
		}
		if len(fr.calls) != 1 || fr.calls[0].SeedPath != leftoverSeedPath {
			t.Fatalf("fr.calls = %+v; want one call carrying SeedPath %q", fr.calls, leftoverSeedPath)
		}

		if !fileExists(leftoverSeedPath + staleSuffix) {
			t.Errorf("stale seed path %q was not created; want moveStaleArtifacts to have moved the leftover aside before targeting ran", leftoverSeedPath+staleSuffix)
		}
		content, err := os.ReadFile(leftoverSeedPath)
		if err != nil {
			t.Fatalf("ReadFile(%q) = %v; want nil", leftoverSeedPath, err)
		}
		if string(content) != "fresh targeting brief for round 3" {
			t.Errorf("seed file content = %q; want the freshly written targeting brief, not the stale leftover", string(content))
		}
	})
}

// fixedShuttle is a same-package Shuttle double that always returns a fixed
// (Result, error) pair without touching the filesystem, regardless of the
// Spec it receives — used to drive runTargeting's fail-safe paths directly,
// including outcomes queuedShuttle cannot script (a claimed-non-done
// Outcome, or a claimed-done Outcome that never actually wrote its output
// file).
type fixedShuttle struct {
	result shuttleengine.Result
	err    error
}

// Run implements Shuttle by returning the scripted result unconditionally.
func (f *fixedShuttle) Run(shuttleengine.Spec) (shuttleengine.Result, error) {
	return f.result, f.err
}

// TestRunTargeting_FailSafe proves runTargeting's fail-safe posture directly against every failure
// path (d) names: a shuttle Run error, a non-done Outcome, and a claimed-done Outcome whose seed
// file was never actually written (or was written empty) — every path returns ("", false) rather
// than an error, mirroring runCircling/runMilestone/runTriage.
func TestRunTargeting_FailSafe(t *testing.T) {
	tests := []struct {
		name  string
		shSet func(seedPath string) Shuttle
	}{
		{
			name: "shuttle run error",
			shSet: func(string) Shuttle {
				return &fixedShuttle{err: errors.New("targeting shuttle unavailable")}
			},
		},
		{
			name: "non-done outcome",
			shSet: func(string) Shuttle {
				return &fixedShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking}}
			},
		},
		{
			name: "claimed done but the seed file was never written",
			shSet: func(string) Shuttle {
				return &fixedShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
			},
		},
		{
			name: "claimed done but the seed file is empty",
			shSet: func(seedPath string) Shuttle {
				writeFile(t, seedPath, "   \n")
				return &fixedShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			seedPath := filepath.Join(runDir, "round-3-seed.md")
			sh := tt.shSet(seedPath)

			content, ok := runTargeting(newTestStencilsDir(t), sh, "gate", 3, "/run/round-2-handoff.md", seedPath, "opus", "high")
			if ok {
				t.Errorf("runTargeting() ok = true; want false")
			}
			if content != "" {
				t.Errorf("runTargeting() content = %q; want empty", content)
			}
		})
	}
}

// TestEngine_ScratchDirSeam covers the scratch-directory seam (Options.ScratchDir) and its
// back-compat default: (a) unset ScratchDir keeps every transient (run.lock, state.json.lock, the
// pause flag) in runDir exactly as before this seam existed; (b) a set ScratchDir routes all three
// away from runDir while state.json and round artifacts stay put, and runDir is asserted to hold
// NO .lock file and NO pause entry at all — the load-bearing check, since "the lock is in scratch"
// alone would also pass an implementation that writes it to both; (c) ErrBlockBusy still fires for
// a second concurrent Run against the same run/scratch pair; (d) PauseFlagPath(scratchDir) and a
// subsequent Run agree on where the flag lives, and a resuming Run clears the leftover flag at
// entry. Every subtest uses its own fakeRunner.onCall hook to write the pause flag mid-round (an
// external caller's pause verb would race the same way against a real block), since the entry-time
// clearPauseFlag would otherwise erase a flag pre-seeded before Run is ever called.
func TestEngine_ScratchDirSeam(t *testing.T) {
	t.Run("(a) unset ScratchDir keeps run.lock, state.json.lock, and the pause flag in runDir", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		flagPath := PauseFlagPath(runDir)
		pauseFn := func() bool {
			_, err := os.Stat(flagPath)
			return err == nil
		}
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}}}
		fr.onCall = func(in AttemptInput) {
			if in.Round == 1 {
				writeFile(t, flagPath, "")
			}
		}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
		e := New("gate", fr, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t), PauseRequested: pauseFn})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomePaused {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomePaused)
		}
		if !fileExists(filepath.Join(runDir, runLockName)) {
			t.Errorf("run.lock not found in runDir %q; want the back-compat default to place it there", runDir)
		}
		if !fileExists(filepath.Join(runDir, stateFileName+".lock")) {
			t.Errorf("state.json.lock not found in runDir %q; want the back-compat default to place it there", runDir)
		}
		if !fileExists(flagPath) {
			t.Errorf("pause flag %q not found in runDir; want the back-compat default to place it there too", flagPath)
		}
	})

	t.Run("(b) set ScratchDir routes run.lock, state.json.lock, and the pause flag out of runDir", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		scratchDir := filepath.Join(t.TempDir(), "scratch")
		flagPath := PauseFlagPath(scratchDir)
		pauseFn := func() bool {
			_, err := os.Stat(flagPath)
			return err == nil
		}
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}}}
		fr.onCall = func(in AttemptInput) {
			if in.Round == 1 {
				writeFile(t, flagPath, "")
			}
		}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
		e := New("gate", fr, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t), ScratchDir: scratchDir, PauseRequested: pauseFn})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomePaused {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomePaused)
		}
		if !fileExists(filepath.Join(scratchDir, runLockName)) {
			t.Errorf("run.lock not found in scratchDir %q", scratchDir)
		}
		if !fileExists(filepath.Join(scratchDir, stateFileName+".lock")) {
			t.Errorf("state.json.lock not found in scratchDir %q", scratchDir)
		}
		if !fileExists(flagPath) {
			t.Errorf("pause flag %q not found in scratchDir", flagPath)
		}
		if !fileExists(filepath.Join(runDir, stateFileName)) {
			t.Errorf("state.json not found in runDir %q; it must stay in the durable dir", runDir)
		}
		if len(fr.calls) != 1 || filepath.Dir(fr.calls[0].ReviewPath) != runDir {
			t.Fatalf("fakeRunner calls = %+v; want round 1's review artifact path rooted at runDir %q", fr.calls, runDir)
		}

		// The load-bearing assertion: runDir must hold NO .lock file and NO
		// pause entry at all — "the lock is in scratch" alone would also pass
		// an implementation that writes it to both.
		entries, err := os.ReadDir(runDir)
		if err != nil {
			t.Fatalf("ReadDir(%q) = %v; want nil", runDir, err)
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".lock") || entry.Name() == PauseFlagName {
				t.Errorf("runDir contains %q; want no lock or pause file when ScratchDir is set", entry.Name())
			}
		}
	})

	t.Run("(c) ErrBlockBusy still fires for a second concurrent Run against the same run/scratch pair", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		scratchDir := filepath.Join(t.TempDir(), "scratch")

		release := make(chan struct{})
		fr1 := &blockingRunner{entered: make(chan struct{}), release: release}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
		e1 := New("gate", fr1, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t), ScratchDir: scratchDir})

		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = e1.Run(p, runDir)
		}()

		select {
		case <-fr1.entered:
		case <-time.After(10 * time.Second):
			t.Fatal("first Run() never entered its blocking attempt")
		}

		fr2 := &fakeRunner{}
		e2 := New("gate", fr2, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t), ScratchDir: scratchDir})
		_, err := e2.Run(p, runDir)
		if err == nil {
			t.Fatal("second Run() error = nil; want an already-running error while the first Run holds the run/scratch lock")
		}
		if !errors.Is(err, ErrBlockBusy) {
			t.Errorf("second Run() error = %v; want errors.Is(err, ErrBlockBusy)", err)
		}
		if len(fr2.calls) != 0 {
			t.Errorf("fr2 called %d times; want 0 (the second Run must never touch the runner)", len(fr2.calls))
		}

		close(release)
		<-done
	})

	t.Run("(d) PauseFlagPath(scratchDir) and a subsequent Run agree when scratch and run dirs differ", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		scratchDir := filepath.Join(t.TempDir(), "scratch")
		flagPath := PauseFlagPath(scratchDir)
		pauseFn := func() bool {
			_, err := os.Stat(flagPath)
			return err == nil
		}

		fr1 := &fakeRunner{}
		fr1.queue = []queuedAttemptResult{{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}}}
		fr1.onCall = func(in AttemptInput) {
			if in.Round == 1 {
				writeFile(t, flagPath, "")
			}
		}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
		e1 := New("gate", fr1, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t), ScratchDir: scratchDir, PauseRequested: pauseFn})

		got, err := e1.Run(p, runDir)
		if err != nil {
			t.Fatalf("first Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomePaused {
			t.Fatalf("first Run() Outcome = %q; want %q", got.Outcome, OutcomePaused)
		}
		if len(fr1.calls) != 1 {
			t.Fatalf("fakeRunner called %d times; want 1 (only round 1 ran before pause fired at round 2's boundary)", len(fr1.calls))
		}
		if !fileExists(flagPath) {
			t.Fatalf("pause flag %q does not exist right after a paused Run; want it left in place for the caller to observe", flagPath)
		}

		// A subsequent Run resuming the same block clears the leftover flag
		// at entry (the resumed-block-must-never-instantly-re-pause rule),
		// then proceeds to completion untouched by it.
		fr2 := &fakeRunner{}
		fr2.queue = []queuedAttemptResult{{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictApproved, SessionID: "s2"}}}
		e2 := New("gate", fr2, &queuedShuttle{}, Options{StencilsDir: newTestStencilsDir(t), ScratchDir: scratchDir})

		got2, err := e2.Run(p, runDir)
		if err != nil {
			t.Fatalf("second Run() error = %v; want nil", err)
		}
		if got2.Outcome != OutcomeApproved {
			t.Fatalf("second Run() Outcome = %q; want %q", got2.Outcome, OutcomeApproved)
		}
		if fileExists(flagPath) {
			t.Errorf("pause flag %q still exists after the resuming Run; want it cleared at entry", flagPath)
		}
	})
}
