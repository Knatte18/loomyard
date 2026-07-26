// engine_test.go drives Engine.Run against a scripted fakeRunner
// (RoundRunner) and a scripted queuedShuttle (judge/triage), mirroring
// internal/perchengine/run_test.go's fakeBurler/queuedShuttle style but
// adapted to the attempt-level RoundRunner seam. Deliberately scoped to what
// the perch differential suite CANNOT prove — that the generalized loop
// works against a non-burler runner and that the seam contract itself
// holds: AttemptInput population and hydration, retry/triage semantics at
// the seam, name-parameterized diagnostics for a non-perch caller, ladder +
// gate parity, and profile validation. Untagged file: no spawning — the
// fake CommandRunner is an in-process func (Test Tier Purity Invariant).

package treadleengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
// adapter would (the runner never invents its own paths), mirroring
// perchengine/run_test.go's fakeBurler.
type fakeRunner struct {
	calls []AttemptInput
	queue []queuedAttemptResult
}

func (f *fakeRunner) RunAttempt(in AttemptInput) (AttemptResult, error) {
	f.calls = append(f.calls, in)
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
// error) queuedShuttle.Run dequeues.
type queuedShuttleEntry struct {
	verdictContent string
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
// the same order. Named distinctly from perchengine's own duplicate to
// avoid any reader confusion about cross-package sharing — there is none;
// each package's copy is a few lines, per the differential-test-bar's
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

// TestEngine_AttemptInputPopulation proves AttemptInput is populated
// correctly per attempt: round/attempt numbers and the RoundToken letter
// suffix on a retry, tuning passthrough (Model/Effort/Timeout), and
// hydration accumulating across rounds — including a failed gate file fed
// forward — with both attempts of the same round seeing IDENTICAL
// hydration (a retry produces no new completed round).
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

	e := New("perch", fr, &queuedShuttle{}, Options{RunCommand: fcr.run})
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
	// hydration); a passing gate is never fed forward (perchengine's own
	// pin, unchanged by the extraction).
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

// TestEngine_RetrySemantics proves the seam's retry policy: a second
// consecutive non-done attempt is a name-prefixed hard error (never
// STUCK), and an asking outcome's triage call determines whether the round
// retries (RETRY) or the block errors (GIVE_UP).
func TestEngine_RetrySemantics(t *testing.T) {
	t.Run("second consecutive died is a name-prefixed hard error", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDied, SessionID: "died-1", RunDir: "/kept/died-1"}},
			{result: AttemptResult{Outcome: shuttleengine.OutcomeTimeout, SessionID: "died-2", RunDir: "/kept/died-2"}},
		}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
		e := New("perch", fr, &queuedShuttle{}, Options{})

		_, err := e.Run(p, runDir)
		if err == nil {
			t.Fatal("Run() error = nil; want an error for two consecutive non-done attempts")
		}
		if !strings.HasPrefix(err.Error(), "perch: ") {
			t.Errorf("Run() error = %q; want a \"perch: \"-prefixed message", err.Error())
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
		e := New("perch", fr, qs, Options{})

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
		e := New("perch", fr, qs, Options{})

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

// TestEngine_NameParameterization proves an Engine constructed with a
// non-perch name produces diagnostics carrying that name's prefix — even
// for the ErrBlockBusy sentinel wrap — and that errors.Is still matches the
// shared sentinel regardless of which name produced the wrap.
func TestEngine_NameParameterization(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")

	// blockingRunner's single RunAttempt call blocks until release is
	// closed, standing in for a real round-runner still in flight when a
	// second Run call arrives against the same run dir.
	release := make(chan struct{})
	fr1 := &blockingRunner{entered: make(chan struct{}), release: release}

	p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{10}}
	e1 := New("tenter", fr1, &queuedShuttle{}, Options{})

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
	e2 := New("tenter", fr2, &queuedShuttle{}, Options{})
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

// TestEngine_LadderAndGateParity smoke-tests the milestone ladder and
// pluggable gate carried forward unchanged by the extraction: an
// unconditional hard-cap STUCK, a milestone rung's STOP verdict, and
// GateCommand convergence via a fake CommandRunner that observes
// Profile.GateDir as its cwd argument.
func TestEngine_LadderAndGateParity(t *testing.T) {
	t.Run("hard cap stops unconditionally with no judge call", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "run")
		fr := &fakeRunner{}
		fr.queue = []queuedAttemptResult{
			{result: AttemptResult{Outcome: shuttleengine.OutcomeDone, Verdict: VerdictBlocking, BlockingCount: 1, SessionID: "s1"}},
		}
		qs := &queuedShuttle{}
		p := Profile{ProfileHash: "hash-1", Gate: Gate{Mode: GateLLMVerdict}, RoundCaps: []int{1}}
		e := New("perch", fr, qs, Options{})

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
		e := New("perch", fr, qs, Options{})

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
		e := New("perch", fr, &queuedShuttle{}, Options{RunCommand: fcr.run})

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

// TestEngine_ProfileValidation proves Run's fail-loud profile checks reject
// a structurally invalid Profile before ever touching the runner: an empty
// ProfileHash, a non-increasing RoundCaps ladder, and an illegal Gate.Mode.
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
			e := New("perch", fr, &queuedShuttle{}, Options{})

			_, err := e.Run(tt.profile, runDir)
			if err == nil {
				t.Fatalf("Run() error = nil; want an error containing %q", tt.errSubstr)
			}
			if !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("Run() error = %q; want substring %q", err.Error(), tt.errSubstr)
			}
			if !strings.HasPrefix(err.Error(), "perch: ") {
				t.Errorf("Run() error = %q; want a \"perch: \"-prefixed message", err.Error())
			}
			if len(fr.calls) != 0 {
				t.Errorf("fakeRunner called %d times; want 0 (an invalid profile must never reach the runner)", len(fr.calls))
			}
		})
	}
}
