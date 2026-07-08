// run_test.go tables Engine.Run's deterministic round loop against a
// scripted fakeBurler (a queue of burlerengine.Result values, recording
// every Profile/RunOpts it received and writing the review/fixer files its
// scripted done-results imply) and a scripted queuedShuttle (a queue of
// judge/triage verdict-file contents or errors, recording every Spec it
// received) — the full fake-seam surface the discussion's Testing section
// pins for perch's strong deterministic test suite: no LLM, no psmux, no
// weft.

package perchengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// scriptedBurlerCall records one burlerengine.Profile/RunOpts pair fakeBurler
// received, in call order.
type scriptedBurlerCall struct {
	profile burlerengine.Profile
	opts    burlerengine.RunOpts
}

// fakeBurler is a same-package Burler double: Run records every Profile/
// RunOpts it receives, dequeues the next scripted burlerengine.Result (or
// error), and — for a scripted done result — writes placeholder content to
// the round's ReviewPath/FixerReportPath so later rounds' PriorReviews/
// PriorFixerReports existence and hydration checks hold, mirroring the real
// burler's file contract.
type fakeBurler struct {
	calls []scriptedBurlerCall
	queue []struct {
		result burlerengine.Result
		err    error
	}
}

// Run implements Burler by dequeuing the next scripted result in FIFO
// order; it fails the test loudly (via a returned error, since Burler.Run
// itself can only report failure that way) if more calls arrive than were
// scripted, which would signal the loop ran more rounds than a test
// intended.
func (f *fakeBurler) Run(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.Result, error) {
	f.calls = append(f.calls, scriptedBurlerCall{profile: p, opts: opts})
	if len(f.queue) == 0 {
		return burlerengine.Result{}, fmt.Errorf("fakeBurler: no scripted result for call %d", len(f.calls))
	}
	next := f.queue[0]
	f.queue = f.queue[1:]
	if next.err != nil {
		return burlerengine.Result{}, next.err
	}

	result := next.result
	if result.Outcome == shuttleengine.OutcomeDone {
		result.ReviewPath = p.ReviewPath
		result.FixerReportPath = p.FixerReportPath
		if err := os.WriteFile(p.ReviewPath, []byte(fmt.Sprintf("verdict: %s\n", result.Verdict)), 0o644); err != nil {
			return burlerengine.Result{}, err
		}
		if err := os.WriteFile(p.FixerReportPath, []byte("fixed something"), 0o644); err != nil {
			return burlerengine.Result{}, err
		}
	}
	return result, nil
}

// queuedShuttle is a same-package Shuttle double for the judge/triage calls:
// Run records every Spec it receives, dequeues the next scripted verdict
// file content (or error), writes it to the Spec's sole OutputFiles entry
// when non-empty, and returns a scripted done Result.
type queuedShuttle struct {
	specs []shuttleengine.Spec
	queue []struct {
		verdictContent string
		err            error
	}
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

// fakeCommandRunner is a scripted CommandRunner double: run records argv,
// dir, and timeout, and dequeues the next scripted (output, exitZero,
// err) triple.
type fakeCommandRunner struct {
	calls []recordedCommand
	queue []struct {
		output   []byte
		exitZero bool
		err      error
	}
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

// verdictFileContent renders a judge/triage verdict file's frontmatter,
// usable for any of the three verdict vocabularies (circling, milestone,
// triage) since all three share the same verdict/rationale frontmatter
// shape.
func verdictFileContent(verdict, rationale string) string {
	return fmt.Sprintf("---\nverdict: %s\nrationale: %s\n---\n", verdict, rationale)
}

// oneBlockingFinding returns a single well-formed BLOCKING finding, the
// minimum a scripted VerdictBlocking result needs to stay self-consistent
// with the real review-file contract's rules.
func oneBlockingFinding() []burlerengine.Finding {
	return []burlerengine.Finding{{ID: "F1", Severity: burlerengine.SeverityBlocking, Location: "target.txt:1", Summary: "a blocking finding"}}
}

// testProfile builds a minimal valid perch Profile: burler content fields
// are Instructions-only (perchengine.Profile.validate never checks them —
// that is burlerengine.Profile.validate's job inside a real burler round,
// which fakeBurler bypasses entirely), plus the gate/caps/judge fields a
// test scenario tunes.
func testProfile(mode GateMode, command []string, caps []int) Profile {
	return Profile{
		Target:      burlerengine.FileSet{Instructions: "review the target"},
		Fasit:       burlerengine.FileSet{Instructions: "judge against the fasit"},
		Rubric:      "the widget must be blue",
		FixScope:    burlerengine.FixScopeSource,
		Gate:        Gate{Mode: mode, Command: command},
		RoundCaps:   caps,
		JudgeModel:  "haiku",
		JudgeEffort: "low",
	}
}

// newTestLayout returns a *hubgeometry.Layout rooted at a fresh temp
// directory, standing in for the worktree root a command gate's cwd
// resolves against.
func newTestLayout(t *testing.T) *hubgeometry.Layout {
	t.Helper()
	return &hubgeometry.Layout{WorktreeRoot: t.TempDir()}
}

// TestRun_LoopUntilDry proves the base convergence path under
// GateLLMVerdict: BLOCKING, BLOCKING, APPROVED reaches OutcomeApproved
// after exactly 3 rounds, and hydration accumulates — round 3's burler
// profile lists rounds 1 and 2's review and fixer-report paths.
func TestRun_LoopUntilDry(t *testing.T) {
	layout := newTestLayout(t)
	runDir := filepath.Join(t.TempDir(), "run")

	fb := &fakeBurler{}
	fb.queue = []struct {
		result burlerengine.Result
		err    error
	}{
		{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
		{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s2"}},
		{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s3"}},
	}
	qs := &queuedShuttle{}
	qs.queue = []struct {
		verdictContent string
		err            error
	}{
		// Round 2's per-round circling check (round 1 never runs a judge —
		// there is no prior round to compare it against).
		{verdictContent: verdictFileContent(string(JudgeProgressing), "still moving forward")},
	}

	e := New(fb, qs, Config{}, layout, Options{})
	p := testProfile(GateLLMVerdict, nil, []int{10})

	got, err := e.Run(p, runDir)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if got.Outcome != OutcomeApproved {
		t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
	}
	if got.RoundsRun != 3 {
		t.Fatalf("Run() RoundsRun = %d; want 3", got.RoundsRun)
	}
	if len(fb.calls) != 3 {
		t.Fatalf("fakeBurler called %d times; want 3", len(fb.calls))
	}

	round3Profile := fb.calls[2].profile
	wantPriorReviews := []string{fb.calls[0].profile.ReviewPath, fb.calls[1].profile.ReviewPath}
	if !stringSlicesEqual(round3Profile.PriorReviews, wantPriorReviews) {
		t.Errorf("round 3 PriorReviews = %v; want %v", round3Profile.PriorReviews, wantPriorReviews)
	}
	wantPriorFixerReports := []string{fb.calls[0].profile.FixerReportPath, fb.calls[1].profile.FixerReportPath}
	if !stringSlicesEqual(round3Profile.PriorFixerReports, wantPriorFixerReports) {
		t.Errorf("round 3 PriorFixerReports = %v; want %v", round3Profile.PriorFixerReports, wantPriorFixerReports)
	}
}

// TestRun_HardCap proves a block still BLOCKING at the ladder's final rung
// stops with STUCK/hard-cap unconditionally, issuing NO judge call at all
// for that final round.
func TestRun_HardCap(t *testing.T) {
	layout := newTestLayout(t)
	runDir := filepath.Join(t.TempDir(), "run")

	fb := &fakeBurler{}
	fb.queue = []struct {
		result burlerengine.Result
		err    error
	}{
		{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
		{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s2"}},
	}
	qs := &queuedShuttle{}

	e := New(fb, qs, Config{}, layout, Options{})
	p := testProfile(GateLLMVerdict, nil, []int{2})

	got, err := e.Run(p, runDir)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if got.Outcome != OutcomeStuck || got.StuckReason != StuckHardCap {
		t.Fatalf("Run() = (%q, %q); want (%q, %q)", got.Outcome, got.StuckReason, OutcomeStuck, StuckHardCap)
	}
	if got.RoundsRun != 2 {
		t.Fatalf("Run() RoundsRun = %d; want 2", got.RoundsRun)
	}
	if len(qs.specs) != 0 {
		t.Errorf("queuedShuttle called %d times; want 0 (no judge call at the hard-cap round)", len(qs.specs))
	}
}

// TestRun_MilestoneGate proves the milestone continuation gate at a
// non-final rung: CONTINUE and UNCERTAIN both let the loop proceed past the
// rung, STOP stops it immediately with STUCK/milestone-stop, and the rung
// round issues exactly one judge call either way. RoundCaps = [1, 3] makes
// round 1 itself the milestone rung, so no circling-check scripting is
// needed to reach it.
func TestRun_MilestoneGate(t *testing.T) {
	tests := []struct {
		name            string
		judgeVerdict    JudgeVerdict
		wantOutcome     Outcome
		wantStuckReason StuckReason
		wantRoundsRun   int
	}{
		{"continue proceeds past the rung", JudgeContinue, OutcomeApproved, "", 2},
		{"uncertain proceeds past the rung", JudgeUncertain, OutcomeApproved, "", 2},
		{"stop halts immediately", JudgeStop, OutcomeStuck, StuckMilestoneStop, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := newTestLayout(t)
			runDir := filepath.Join(t.TempDir(), "run")

			fb := &fakeBurler{}
			fb.queue = []struct {
				result burlerengine.Result
				err    error
			}{
				{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
			}
			if tt.judgeVerdict != JudgeStop {
				// A halted block never reaches round 2 at all.
				fb.queue = append(fb.queue, struct {
					result burlerengine.Result
					err    error
				}{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s2"}})
			}

			qs := &queuedShuttle{}
			qs.queue = []struct {
				verdictContent string
				err            error
			}{
				{verdictContent: verdictFileContent(string(tt.judgeVerdict), "milestone rationale")},
			}

			e := New(fb, qs, Config{}, layout, Options{})
			p := testProfile(GateLLMVerdict, nil, []int{1, 3})

			got, err := e.Run(p, runDir)
			if err != nil {
				t.Fatalf("Run() error = %v; want nil", err)
			}
			if got.Outcome != tt.wantOutcome || got.StuckReason != tt.wantStuckReason {
				t.Fatalf("Run() = (%q, %q); want (%q, %q)", got.Outcome, got.StuckReason, tt.wantOutcome, tt.wantStuckReason)
			}
			if got.RoundsRun != tt.wantRoundsRun {
				t.Fatalf("Run() RoundsRun = %d; want %d", got.RoundsRun, tt.wantRoundsRun)
			}
			if len(qs.specs) != 1 {
				t.Errorf("queuedShuttle called %d times; want exactly 1 (milestone replaces circling for this round)", len(qs.specs))
			}
			if got.Rounds[0].JudgeVerdict != string(tt.judgeVerdict) {
				t.Errorf("Rounds[0].JudgeVerdict = %q; want %q", got.Rounds[0].JudgeVerdict, tt.judgeVerdict)
			}
		})
	}
}

// TestRun_PerRoundCircling proves the per-round circling check: a CIRCLING
// verdict at a mid-window round >= 2 stops the block immediately with
// STUCK/circling, no judge call is ever issued for round 1, and an
// APPROVED-verdict round never triggers a judge call at all.
func TestRun_PerRoundCircling(t *testing.T) {
	t.Run("circling stops the loop at round 2", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s2"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []struct {
			verdictContent string
			err            error
		}{
			{verdictContent: verdictFileContent(string(JudgeCircling), "the same finding recurs")},
		}

		e := New(fb, qs, Config{}, layout, Options{})
		p := testProfile(GateLLMVerdict, nil, []int{10})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeStuck || got.StuckReason != StuckCircling {
			t.Fatalf("Run() = (%q, %q); want (%q, %q)", got.Outcome, got.StuckReason, OutcomeStuck, StuckCircling)
		}
		if got.RoundsRun != 2 {
			t.Fatalf("Run() RoundsRun = %d; want 2", got.RoundsRun)
		}
		// Exactly one judge call total proves round 1 (the first BLOCKING
		// round) never triggered the circling check on its own.
		if len(qs.specs) != 1 {
			t.Errorf("queuedShuttle called %d times; want exactly 1 (no judge call on round 1)", len(qs.specs))
		}
	})

	t.Run("no judge call on an approved-verdict round", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s1"}},
		}
		qs := &queuedShuttle{}

		e := New(fb, qs, Config{}, layout, Options{})
		p := testProfile(GateLLMVerdict, nil, []int{10})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
		}
		if len(qs.specs) != 0 {
			t.Errorf("queuedShuttle called %d times; want 0", len(qs.specs))
		}
	})
}

// TestRun_JudgeFailSafe proves every judge infrastructure failure (a
// shuttle Run error, a non-done outcome, and an unparseable verdict file)
// degrades to the safe default inside the judge call itself, so the loop
// continues rather than erroring or reporting STUCK.
func TestRun_JudgeFailSafe(t *testing.T) {
	tests := []struct {
		name  string
		entry struct {
			verdictContent string
			err            error
		}
	}{
		{
			name: "shuttle run error",
			entry: struct {
				verdictContent string
				err            error
			}{err: errors.New("fake shuttle run error")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := newTestLayout(t)
			runDir := filepath.Join(t.TempDir(), "run")

			fb := &fakeBurler{}
			fb.queue = []struct {
				result burlerengine.Result
				err    error
			}{
				{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
				{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s2"}},
				{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s3"}},
			}
			qs := &queuedShuttle{}
			qs.queue = []struct {
				verdictContent string
				err            error
			}{tt.entry}

			e := New(fb, qs, Config{}, layout, Options{})
			p := testProfile(GateLLMVerdict, nil, []int{10})

			got, err := e.Run(p, runDir)
			if err != nil {
				t.Fatalf("Run() error = %v; want nil — a judge infrastructure failure must never surface as an engine error", err)
			}
			if got.Outcome != OutcomeApproved {
				t.Fatalf("Run() Outcome = %q; want %q — a judge failure must never report STUCK", got.Outcome, OutcomeApproved)
			}
			if got.RoundsRun != 3 {
				t.Fatalf("Run() RoundsRun = %d; want 3", got.RoundsRun)
			}
		})
	}
}

// TestRun_GateModes tables the pluggable convergence gate: GateLLMVerdict
// never even invokes the command runner; GateCommand ignores the burler
// verdict entirely (an APPROVED round with a failing command does not
// converge, and its gate file is fed forward; a BLOCKING round with a
// passing command does converge); GateBoth requires both signals to agree.
func TestRun_GateModes(t *testing.T) {
	t.Run("llm-verdict never invokes the command runner", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s1"}},
		}
		fcr := &fakeCommandRunner{}

		e := New(fb, &queuedShuttle{}, Config{}, layout, Options{RunCommand: fcr.run})
		p := testProfile(GateLLMVerdict, nil, []int{10})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
		}
		if len(fcr.calls) != 0 {
			t.Errorf("fakeCommandRunner called %d times; want 0 (llm-verdict must never invoke it)", len(fcr.calls))
		}
	})

	t.Run("command mode ignores an approved verdict when the command fails, and feeds the gate file forward", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s1"}},
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s2"}},
		}
		fcr := &fakeCommandRunner{}
		fcr.queue = []struct {
			output   []byte
			exitZero bool
			err      error
		}{
			{output: []byte("build failed"), exitZero: false},
			{output: []byte("build ok"), exitZero: true},
		}

		e := New(fb, &queuedShuttle{}, Config{}, layout, Options{RunCommand: fcr.run})
		p := testProfile(GateCommand, []string{"make", "test"}, []int{10})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
		}
		if got.RoundsRun != 2 {
			t.Fatalf("Run() RoundsRun = %d; want 2 (round 1's failing command must not converge despite an approved verdict)", got.RoundsRun)
		}

		round2Profile := fb.calls[1].profile
		wantGatePath := got.Rounds[0].GatePath
		if wantGatePath == "" {
			t.Fatalf("Rounds[0].GatePath is empty; want the round-1-gate.md path")
		}
		found := false
		for _, r := range round2Profile.PriorReviews {
			if r == wantGatePath {
				found = true
			}
		}
		if !found {
			t.Errorf("round 2 PriorReviews = %v; want it to include the failing gate path %q", round2Profile.PriorReviews, wantGatePath)
		}
		if fcr.calls[0].dir != layout.WorktreeRoot {
			t.Errorf("gate command dir = %q; want layout.WorktreeRoot %q", fcr.calls[0].dir, layout.WorktreeRoot)
		}
	})

	t.Run("command mode converges on a blocking verdict when the command passes", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
		}
		fcr := &fakeCommandRunner{}
		fcr.queue = []struct {
			output   []byte
			exitZero bool
			err      error
		}{{output: []byte("ok"), exitZero: true}}

		e := New(fb, &queuedShuttle{}, Config{}, layout, Options{RunCommand: fcr.run})
		p := testProfile(GateCommand, []string{"make", "test"}, []int{10})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q (a passing command converges even on a BLOCKING verdict)", got.Outcome, OutcomeApproved)
		}
	})

	t.Run("both fails when the command fails despite an approved verdict", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s1"}},
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s2"}},
		}
		fcr := &fakeCommandRunner{}
		fcr.queue = []struct {
			output   []byte
			exitZero bool
			err      error
		}{
			{output: []byte("build failed"), exitZero: false},
			{output: []byte("build ok"), exitZero: true},
		}

		e := New(fb, &queuedShuttle{}, Config{}, layout, Options{RunCommand: fcr.run})
		p := testProfile(GateBoth, []string{"make", "test"}, []int{10})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.RoundsRun != 2 {
			t.Fatalf("Run() RoundsRun = %d; want 2", got.RoundsRun)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q after round 2's passing command", got.Outcome, OutcomeApproved)
		}
	})

	t.Run("both fails when the verdict is blocking despite a passing command", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s2"}},
		}
		fcr := &fakeCommandRunner{}
		fcr.queue = []struct {
			output   []byte
			exitZero bool
			err      error
		}{
			{output: []byte("ok"), exitZero: true},
			{output: []byte("ok"), exitZero: true},
		}

		e := New(fb, &queuedShuttle{}, Config{}, layout, Options{RunCommand: fcr.run})
		p := testProfile(GateBoth, []string{"make", "test"}, []int{10})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.RoundsRun != 2 {
			t.Fatalf("Run() RoundsRun = %d; want 2 (round 1's blocking verdict must not converge despite a passing command)", got.RoundsRun)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q after round 2's approved verdict", got.Outcome, OutcomeApproved)
		}
	})
}

// TestRun_NonDoneOutcomes tables burler's non-done outcomes: a died attempt
// followed by a done attempt completes the round with Attempts 2 and a
// b-token review path; a second consecutive died attempt is a hard error
// naming the session id and kept run dir, never STUCK; an asking outcome
// triages RETRY (re-attempts) or GIVE_UP (errors, carrying the rationale);
// and a triage infrastructure failure fail-safes to RETRY.
func TestRun_NonDoneOutcomes(t *testing.T) {
	t.Run("died then done completes with Attempts 2 and a b-token review path", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDied, SessionID: "died-1", RunDir: "/kept/died-1"}},
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s2"}},
		}

		e := New(fb, &queuedShuttle{}, Config{}, layout, Options{})
		p := testProfile(GateLLMVerdict, nil, []int{10})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.RoundsRun != 1 {
			t.Fatalf("Run() RoundsRun = %d; want 1", got.RoundsRun)
		}
		if got.Rounds[0].Attempts != 2 {
			t.Errorf("Rounds[0].Attempts = %d; want 2", got.Rounds[0].Attempts)
		}
		if !strings.Contains(got.Rounds[0].ReviewPath, "round-1b-review.md") {
			t.Errorf("Rounds[0].ReviewPath = %q; want it to name the round-1b (attempt-2) token", got.Rounds[0].ReviewPath)
		}
	})

	t.Run("died twice is a hard error naming the session id and kept run dir, never STUCK", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDied, SessionID: "died-1", RunDir: "/kept/died-1"}},
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeTimeout, SessionID: "died-2", RunDir: "/kept/died-2"}},
		}

		e := New(fb, &queuedShuttle{}, Config{}, layout, Options{})
		p := testProfile(GateLLMVerdict, nil, []int{10})

		_, err := e.Run(p, runDir)
		if err == nil {
			t.Fatalf("Run() error = nil; want an error for two consecutive non-done attempts")
		}
		if !strings.Contains(err.Error(), "died-2") || !strings.Contains(err.Error(), "/kept/died-2") {
			t.Errorf("Run() error = %q; want it to carry the second attempt's session id and kept run dir", err.Error())
		}
	})

	t.Run("asking with triage RETRY re-attempts the round", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeAsking, SessionID: "ask-1", RunDir: "/kept/ask-1", LastAssistantMessage: "should I proceed?"}},
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s2"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []struct {
			verdictContent string
			err            error
		}{
			{verdictContent: verdictFileContent(string(TriageRetry), "plausibly proceeds")},
		}

		e := New(fb, qs, Config{}, layout, Options{})
		p := testProfile(GateLLMVerdict, nil, []int{10})

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
	})

	t.Run("asking with triage GIVE_UP errors carrying the rationale", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeAsking, SessionID: "ask-1", RunDir: "/kept/ask-1", LastAssistantMessage: "the fasit file does not exist"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []struct {
			verdictContent string
			err            error
		}{
			{verdictContent: verdictFileContent(string(TriageGiveUp), "the fasit file referenced does not exist")},
		}

		e := New(fb, qs, Config{}, layout, Options{})
		p := testProfile(GateLLMVerdict, nil, []int{10})

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

	t.Run("triage infrastructure failure fail-safes to RETRY", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeAsking, SessionID: "ask-1", RunDir: "/kept/ask-1", LastAssistantMessage: "should I proceed?"}},
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s2"}},
		}
		qs := &queuedShuttle{}
		qs.queue = []struct {
			verdictContent string
			err            error
		}{
			{err: errors.New("fake triage shuttle error")},
		}

		e := New(fb, qs, Config{}, layout, Options{})
		p := testProfile(GateLLMVerdict, nil, []int{10})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil — triage's own fail-safe must default to retry", err)
		}
		if got.Rounds[0].Attempts != 2 {
			t.Errorf("Rounds[0].Attempts = %d; want 2 (the fail-safe RETRY re-attempted the round)", got.Rounds[0].Attempts)
		}
	})
}

// TestRun_Resume tables the resume mechanics: a fresh Engine.Run on a run
// dir with unfinished state continues at the recorded next round; a
// terminal state refuses to resume at all; a profile-hash mismatch fails
// loud naming a fresh --run-id; and a stale half-written artifact from an
// interrupted round is moved aside before that round re-runs from scratch.
func TestRun_Resume(t *testing.T) {
	t.Run("continues at the recorded next round", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")
		p := testProfile(GateLLMVerdict, nil, []int{10})

		// First Run: pause before round 3, after rounds 1 and 2 complete.
		// Round 2's BLOCKING verdict needs one scripted circling response.
		calls := 0
		pauseAfterTwo := func() bool {
			calls++
			return calls > 2
		}
		fb1 := &fakeBurler{}
		fb1.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s2"}},
		}
		qs1 := &queuedShuttle{}
		qs1.queue = []struct {
			verdictContent string
			err            error
		}{
			{verdictContent: verdictFileContent(string(JudgeProgressing), "still moving")},
		}
		e1 := New(fb1, qs1, Config{}, layout, Options{PauseRequested: pauseAfterTwo})

		first, err := e1.Run(p, runDir)
		if err != nil {
			t.Fatalf("first Run() error = %v; want nil", err)
		}
		if first.Outcome != OutcomePaused || first.RoundsRun != 2 {
			t.Fatalf("first Run() = (%q, rounds=%d); want (%q, rounds=2)", first.Outcome, first.RoundsRun, OutcomePaused)
		}

		// Second Run: a fresh Engine/fakeBurler resumes on the same runDir.
		fb2 := &fakeBurler{}
		fb2.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s3"}},
		}
		e2 := New(fb2, &queuedShuttle{}, Config{}, layout, Options{})

		second, err := e2.Run(p, runDir)
		if err != nil {
			t.Fatalf("second Run() error = %v; want nil", err)
		}
		if second.Outcome != OutcomeApproved {
			t.Fatalf("second Run() Outcome = %q; want %q", second.Outcome, OutcomeApproved)
		}
		if second.RoundsRun != 3 {
			t.Fatalf("second Run() RoundsRun = %d; want 3 (2 resumed + 1 new)", second.RoundsRun)
		}
		if len(fb2.calls) != 1 || fb2.calls[0].opts.Round != "3" {
			t.Fatalf("fb2.calls = %+v; want exactly one call with Round \"3\"", fb2.calls)
		}
	})

	t.Run("a terminal state refuses to resume", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")
		p := testProfile(GateLLMVerdict, nil, []int{10})

		fb1 := &fakeBurler{}
		fb1.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s1"}},
		}
		e1 := New(fb1, &queuedShuttle{}, Config{}, layout, Options{})
		if _, err := e1.Run(p, runDir); err != nil {
			t.Fatalf("first Run() error = %v; want nil", err)
		}

		fb2 := &fakeBurler{}
		e2 := New(fb2, &queuedShuttle{}, Config{}, layout, Options{})
		_, err := e2.Run(p, runDir)
		if err == nil {
			t.Fatalf("second Run() error = nil; want an error refusing to resume a finished block")
		}
		if !strings.Contains(err.Error(), "already finished") {
			t.Errorf("second Run() error = %q; want it to name the block as already finished", err.Error())
		}
		if len(fb2.calls) != 0 {
			t.Errorf("fb2 called %d times; want 0 (the block must not run at all)", len(fb2.calls))
		}
	})

	t.Run("a profile-hash mismatch fails loud naming a fresh --run-id", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")

		calls := 0
		pauseAfterOne := func() bool {
			calls++
			return calls > 1
		}
		fb1 := &fakeBurler{}
		fb1.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
		}
		p1 := testProfile(GateLLMVerdict, nil, []int{10})
		e1 := New(fb1, &queuedShuttle{}, Config{}, layout, Options{PauseRequested: pauseAfterOne})
		first, err := e1.Run(p1, runDir)
		if err != nil {
			t.Fatalf("first Run() error = %v; want nil", err)
		}
		if first.Outcome != OutcomePaused {
			t.Fatalf("first Run() Outcome = %q; want %q", first.Outcome, OutcomePaused)
		}

		// A profile that differs in content (different Rubric) hashes
		// differently, so resuming with it must be refused.
		p2 := testProfile(GateLLMVerdict, nil, []int{10})
		p2.Rubric = "a completely different rubric"
		fb2 := &fakeBurler{}
		e2 := New(fb2, &queuedShuttle{}, Config{}, layout, Options{})
		_, err = e2.Run(p2, runDir)
		if err == nil {
			t.Fatalf("second Run() error = nil; want a profile-hash mismatch error")
		}
		if !strings.Contains(err.Error(), "--run-id") {
			t.Errorf("second Run() error = %q; want it to name a fresh --run-id", err.Error())
		}
		if len(fb2.calls) != 0 {
			t.Errorf("fb2 called %d times; want 0", len(fb2.calls))
		}
	})

	t.Run("a stale half-written review file is moved aside and the round re-runs", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")
		p := testProfile(GateLLMVerdict, nil, []int{10})

		calls := 0
		pauseAfterOne := func() bool {
			calls++
			return calls > 1
		}
		fb1 := &fakeBurler{}
		fb1.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
		}
		e1 := New(fb1, &queuedShuttle{}, Config{}, layout, Options{PauseRequested: pauseAfterOne})
		if _, err := e1.Run(p, runDir); err != nil {
			t.Fatalf("first Run() error = %v; want nil", err)
		}

		// Simulate a crash mid-round-2: a partial review file was written
		// but the round never reached done, so no roundRecord exists for it.
		stalePath := artifactPaths(runDir, 2, 1).Review
		writeFile(t, stalePath, "partial content from an interrupted round")

		fb2 := &fakeBurler{}
		fb2.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s2"}},
		}
		e2 := New(fb2, &queuedShuttle{}, Config{}, layout, Options{})
		got, err := e2.Run(p, runDir)
		if err != nil {
			t.Fatalf("second Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("second Run() Outcome = %q; want %q", got.Outcome, OutcomeApproved)
		}
		if len(fb2.calls) != 1 {
			t.Fatalf("fb2 called %d times; want 1 (round 2 re-ran from scratch)", len(fb2.calls))
		}

		// The original path is rewritten by round 2's fresh (successful)
		// attempt, so it exists again — but with NEW content, proving the
		// round actually re-ran rather than trusting the stale leftover.
		freshContent, err := os.ReadFile(stalePath)
		if err != nil {
			t.Fatalf("ReadFile(%q) = %v; want round 2's fresh review file", stalePath, err)
		}
		if string(freshContent) == "partial content from an interrupted round" {
			t.Errorf("review file at %q still holds the stale content; want it overwritten by the re-run", stalePath)
		}
		staleContent, err := os.ReadFile(stalePath + ".stale")
		if err != nil {
			t.Fatalf("ReadFile(%q) = %v; want the moved-aside stale file to exist", stalePath+".stale", err)
		}
		if string(staleContent) != "partial content from an interrupted round" {
			t.Errorf("stale file content = %q; want the original partial content preserved", staleContent)
		}
	})
}

// TestRun_Pause proves the pause boundary is checked only between rounds
// (never mid-round): a PauseRequested callback that turns true after round
// 1 stops the loop with OutcomePaused having called burler exactly once,
// and a resumed run clears any leftover pause flag file rather than
// instantly re-pausing on it.
func TestRun_Pause(t *testing.T) {
	t.Run("pauses after round 1 completes, burler called exactly once", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")
		p := testProfile(GateLLMVerdict, nil, []int{10})

		calls := 0
		pauseAfterOne := func() bool {
			calls++
			return calls > 1
		}
		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
		}
		e := New(fb, &queuedShuttle{}, Config{}, layout, Options{PauseRequested: pauseAfterOne})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomePaused {
			t.Fatalf("Run() Outcome = %q; want %q", got.Outcome, OutcomePaused)
		}
		if got.RoundsRun != 1 {
			t.Fatalf("Run() RoundsRun = %d; want 1", got.RoundsRun)
		}
		if len(fb.calls) != 1 {
			t.Errorf("fakeBurler called %d times; want exactly 1", len(fb.calls))
		}
	})

	t.Run("resume clears a leftover pause flag file rather than instantly re-pausing", func(t *testing.T) {
		layout := newTestLayout(t)
		runDir := filepath.Join(t.TempDir(), "run")
		p := testProfile(GateLLMVerdict, nil, []int{10})

		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", runDir, err)
		}
		// A stray pause flag file left over from a previous run, exactly
		// as perchcli's pause verb would write it.
		writeFile(t, PauseFlagPath(runDir), "")

		checkFlag := func() bool {
			_, err := os.Stat(PauseFlagPath(runDir))
			return err == nil
		}

		fb := &fakeBurler{}
		fb.queue = []struct {
			result burlerengine.Result
			err    error
		}{
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking, Findings: oneBlockingFinding(), SessionID: "s1"}},
			{result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictApproved, SessionID: "s2"}},
		}
		e := New(fb, &queuedShuttle{}, Config{}, layout, Options{PauseRequested: checkFlag})

		got, err := e.Run(p, runDir)
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if got.Outcome != OutcomeApproved {
			t.Fatalf("Run() Outcome = %q; want %q — the leftover flag must be cleared at entry, not honored", got.Outcome, OutcomeApproved)
		}
		if got.RoundsRun != 2 {
			t.Fatalf("Run() RoundsRun = %d; want 2", got.RoundsRun)
		}
	})
}
