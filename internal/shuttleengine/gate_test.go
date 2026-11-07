// gate_test.go covers the gate attempt loop end to end, driven over the package's existing fakes and seams:
// fakeReed, fakeEngine, newFixture, and the fakeClock and multiStepClock seams.
// It is hermetic and untagged, per the Test Tier Purity Invariant.
// It spawns no external process, builds no real fixture hub, and never sleeps for real.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// appendEventsLine appends line, with a trailing newline, to path.
// It stands in for the agent's next turn appending a fresh events.jsonl record.
func appendEventsLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open events file for append: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("append events line: %v", err)
	}
}

// spanKinds returns the kinds of spans, in order.
func spanKinds(spans []GateSpan) []GateSpanKind {
	kinds := make([]GateSpanKind, len(spans))
	for i, span := range spans {
		kinds[i] = span.Kind
	}
	return kinds
}

// repromptCaptureSequence returns the fakeReed.CaptureQueue entries for n Send() calls, three per call.
// Each Send() succeeds on its first delivery poll.
// Per call, a probe capture answers requireReadyAgentPane.
// A clean baseline capture answers sendVerified's pre-send snapshot, holding no copy of the re-prompt needle.
// A delivery capture holds the re-prompt text itself,
// so the needle's count rises above the baseline's on the very first poll.
func repromptCaptureSequence(findingsPath string, n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, "idle pane", "idle pane", gateRepromptText(findingsPath))
	}
	return out
}

// TestGate_FirstDoneSettlesWithoutReprompt pins the happy path, which preserves ungated behaviour:
// a gate that passes when finalize first evaluates it sends no re-prompt and reports zero attempts.
// Its cleanup is identical to an ungated run's.
// A zero-value GateSpec, which every ungated run supplies, leaves Result.Gate nil and cleans up the same way.
//
//testtiming:keep pins that a gate passing on the first Done sends nothing and charges no attempt, that it reports one gate span the closure's length without writing it through the memoised verdict, that a zero GateSpec leaves Result.Gate nil, and that both clean the run dir
func TestGate_FirstDoneSettlesWithoutReprompt(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name  string
		gated bool
	}{
		{name: "gate passes the first time", gated: true},
		{name: "zero gate spec leaves Result.Gate nil", gated: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, eventsFileName)
			outputFile := filepath.Join(runDir, "out.md")
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			fc := newFakeClock(time.Now())
			gateCalls := 0
			var closureAt time.Time
			gate := func() (GateResult, error) {
				gateCalls++
				closureAt = fc.Now()
				fc.asleep(3 * time.Second)
				return GateResult{Passed: true}, nil
			}

			reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(gateConfig))
			opts := []runOpt{
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
				withRunClock(fc, fc.Now().Add(time.Hour)),
			}
			if tt.gated {
				opts = append(opts, withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))
			}
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour}, opts...)

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
			}
			if tt.gated {
				if result.Gate == nil || !result.Gate.Passed {
					t.Errorf("Gate = %+v, want a passed verdict", result.Gate)
				}
				if result.Gate != nil && result.Gate.Attempts != 0 {
					t.Errorf("Attempts = %d, want 0", result.Gate.Attempts)
				}
				if gateCalls != 1 {
					t.Errorf("gate closure invoked %d times, want 1", gateCalls)
				}
				wantSpans := []GateSpan{{Kind: GateSpanGate, Start: closureAt, End: closureAt.Add(3 * time.Second)}}
				if result.Gate != nil && !reflect.DeepEqual(result.Gate.Entries[0].Spans, wantSpans) {
					t.Errorf("Spans = %+v, want %+v", result.Gate.Entries[0].Spans, wantSpans)
				}
				if run.gateVerdict == nil || run.gateVerdict.Entries[0].Spans != nil {
					t.Errorf("memoised verdict = %+v, want one holding no spans", run.gateVerdict)
				}
			} else if result.Gate != nil {
				t.Errorf("Gate = %+v, want nil for an ungated run", result.Gate)
			}
			if len(reed.SendTextCalls) != 0 {
				t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
			}
			if _, err := os.Stat(runDir); !os.IsNotExist(err) {
				t.Errorf("run dir still exists after done cleanup, stat err = %v", err)
			}
		})
	}
}

// TestGate_FailsOnceThenPassesOnNextTurn covers the core re-prompt loop:
// a failed first attempt sends exactly one re-prompt naming the findings file,
// and the agent's next turn passing settles the run.
// The re-prompt is the default line, or the line the failing entry's Reprompt renders.
//
//testtiming:keep pins that the re-prompt is a single line naming the findings file, that an entry's own renderer replaces the default line, and that the findings file holds the failed attempt's findings
func TestGate_FailsOnceThenPassesOnNextTurn(t *testing.T) {
	tiergate.Require(t, 0)
	customReprompt := func(findingsPath string) string { return "custom line: open " + findingsPath }
	tests := []struct {
		name string
		// reprompt is the entry's renderer, nil for the default line.
		reprompt   func(findingsPath string) string
		wantPrompt func(findingsPath string) string
	}{
		{name: "the default line", wantPrompt: gateRepromptText},
		{name: "an entry's own line", reprompt: customReprompt, wantPrompt: customReprompt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, eventsFileName)
			outputFile := filepath.Join(runDir, "out.md")
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			findingsPath := filepath.Join(runDir, gateFindingsFileName)
			wantFindings := "fix the thing"
			wantSent := tt.wantPrompt(findingsPath)
			gateCalls := 0
			gate := func() (GateResult, error) {
				gateCalls++
				if gateCalls == 1 {
					return GateResult{Passed: false, Findings: wantFindings}, nil
				}
				return GateResult{Passed: true}, nil
			}

			reed := &fakeReed{
				StatusQueue:  liveStrandStatus(true),
				CaptureQueue: []string{"idle pane", "idle pane", wantSent},
			}
			engine := readyAgentEngine()
			fx := newFixture(t, reed, engine, withConfig(gateConfig))
			stubInputSleep(t)

			fc := newFakeClock(time.Now())
			mc := &multiStepClock{fakeClock: fc, steps: []func(){
				func() { appendEventsLine(t, eventsPath, "STOP:turn2") },
			}}
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour, KeepPane: true},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
				withRunClock(mc, mc.Now().Add(time.Hour)),
				withRunGate(GateSpec{{Gate: gate, Attempts: 3, Reprompt: tt.reprompt}}))

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
			}
			if result.Gate == nil || !result.Gate.Passed {
				t.Errorf("Gate = %+v, want a passed verdict", result.Gate)
			}
			if result.Gate != nil && result.Gate.Attempts != 1 {
				t.Errorf("Attempts = %d, want 1", result.Gate.Attempts)
			}
			if len(reed.SendTextCalls) != 1 {
				t.Fatalf("SendText calls = %+v, want exactly one", reed.SendTextCalls)
			}
			sent := reed.SendTextCalls[0].Text
			if sent != wantSent {
				t.Errorf("re-prompt text = %q, want %q", sent, wantSent)
			}
			if strings.ContainsAny(sent, "\n\r") {
				t.Errorf("re-prompt text = %q, want a single line", sent)
			}
			if !strings.Contains(sent, findingsPath) {
				t.Errorf("re-prompt text = %q, want it to name the findings file %q", sent, findingsPath)
			}

			gotFindings, err := os.ReadFile(findingsPath)
			if err != nil {
				t.Fatalf("read findings file: %v", err)
			}
			if string(gotFindings) != wantFindings {
				t.Errorf("findings file = %q, want the first attempt's findings %q", gotFindings, wantFindings)
			}
		})
	}
}

// TestGate_ClosureError covers a gate closure that returns an infrastructure fault.
// The fault fails the run as an ordinary error.
// It does so whether the closure is the only entry or a later one after an earlier entry passed.
// The run sends no re-prompt and charges no attempt or failure.
func TestGate_ClosureError(t *testing.T) {
	tiergate.Require(t, 0)
	wantErr := errors.New("gate infrastructure fault")
	fault := func() (GateResult, error) { return GateResult{}, wantErr }
	pass := func() (GateResult, error) { return GateResult{Passed: true}, nil }

	tests := []struct {
		name   string
		events string
		spec   GateSpec
	}{
		{name: "only entry", events: "STOP:turn1\n", spec: GateSpec{{Gate: fault, Attempts: 3}}},
		{
			name:   "second entry after the first passes",
			events: "STOP:done\n",
			spec: GateSpec{
				{Name: "first", Gate: pass, Attempts: 3},
				{Name: "second", Gate: fault, Attempts: 3},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, eventsFileName)
			outputFile := filepath.Join(runDir, "out.md")
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(eventsPath, []byte(tt.events), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(gateConfig))
			fc := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
				withRunClock(fc, fc.Now().Add(time.Hour)),
				withRunGate(tt.spec))

			_, err := run.Wait()
			if err == nil || !errors.Is(err, wantErr) {
				t.Fatalf("Wait() error = %v, want it to wrap %v", err, wantErr)
			}
			if len(reed.SendTextCalls) != 0 {
				t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
			}
			for i, sent := range run.gateSent {
				if sent != 0 {
					t.Errorf("gateSent[%d] = %d, want 0 (an infrastructure error charges no attempt)", i, sent)
				}
			}
			for i, fails := range run.gateFails {
				if fails != 0 {
					t.Errorf("gateFails[%d] = %d, want 0 (an infrastructure error counts no failure)", i, fails)
				}
			}
		})
	}
}

// TestGate_NoLiveSessionDonePaths covers the Done exits a gated run reaches with no live session:
// the strand is not tracked, its pane is dead, its startup window expires, the run deadline expires,
// or the events file is unreadable.
// Each exit finalizes without the events-tick Send loop.
// So each asserts a failed verdict, zero sends, zero attempts charged, and the gate running exactly once.
//
//testtiming:keep pins that every exit with no live session finalizes through its own branch with the gate run once, a failed verdict, and no re-prompt or attempt charged
func TestGate_NoLiveSessionDonePaths(t *testing.T) {
	tiergate.Require(t, 0)
	liveness := Config{PollIntervalMS: 1000, LivenessEveryNPolls: 1, StartupTimeoutS: 0}
	tests := []struct {
		name          string
		cfg           Config
		statusQueue   []reedengine.StatusResult
		startupScript []StartupState
		// eventsSeed is written to the events file; empty leaves it unwritten, so no Stop event arrives.
		eventsSeed     string
		parseEventsErr error
		// deadlineOffset is where the run deadline sits relative to the clock's start.
		deadlineOffset time.Duration
	}{
		{name: "not_tracked", cfg: liveness, statusQueue: []reedengine.StatusResult{{Strands: nil}}, deadlineOffset: time.Hour},
		{name: "not_live_dead_pane", cfg: liveness, statusQueue: []reedengine.StatusResult{deadStatus("strand-1", "%1")}, deadlineOffset: time.Hour},
		{
			name: "startup_window_expires", cfg: liveness, deadlineOffset: time.Hour,
			statusQueue:   []reedengine.StatusResult{liveStatus("strand-1", "%1")},
			startupScript: []StartupState{StartupPending},
		},
		// The deadline is already expired, so the very first deadline check trips it.
		// The gate still runs once, through classifyDeadlineExpiry and finalize, with no session addressed.
		{name: "run_deadline_expires", cfg: gateConfig, deadlineOffset: -time.Minute},
		// The events-unreadable mechanism-failure exit still runs the gate once, through
		// finishedDespiteMechanismFailure and finalize.
		{
			name: "events_unreadable_mechanism_failure", cfg: gateConfig, deadlineOffset: time.Hour,
			eventsSeed: "STOP:x\n", parseEventsErr: errors.New("events file unparseable"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, eventsFileName)
			if tt.eventsSeed != "" {
				if err := os.WriteFile(eventsPath, []byte(tt.eventsSeed), 0o644); err != nil {
					t.Fatalf("seed events: %v", err)
				}
			}
			outputFile := filepath.Join(runDir, "out.md")
			touchOutputFile(t, outputFile)

			gateCalls := 0
			gate := func() (GateResult, error) {
				gateCalls++
				return GateResult{Passed: false, Findings: "n/a"}, nil
			}

			reed := &fakeReed{StatusQueue: tt.statusQueue}
			engine := &fakeEngine{StartupScript: tt.startupScript, ParseEventsErr: tt.parseEventsErr}
			fx := newFixture(t, reed, engine, withConfig(tt.cfg))
			fc := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
				withRunClock(fc, fc.Now().Add(tt.deadlineOffset)),
				withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
			}
			if result.Gate == nil || result.Gate.Passed {
				t.Errorf("Gate = %+v, want a failed verdict", result.Gate)
			}
			if result.Gate != nil && result.Gate.Attempts != 0 {
				t.Errorf("Attempts = %d, want 0 (no live session ever received a re-prompt)", result.Gate.Attempts)
			}
			if gateCalls != 1 {
				t.Errorf("gate closure invoked %d times, want 1", gateCalls)
			}
			if len(reed.SendTextCalls) != 0 {
				t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
			}
		})
	}
}

// TestGate_EvaluateOncePerAttempt_Memoized pins that the gate runs exactly once per settling: a
// second evaluateGate() call within the same attempt reads the stored memo rather than re-invoking
// the closure.
//
//testtiming:keep pins that evaluateGate memoizes within an attempt, returning the same pointer without re-running the closure
func TestGate_EvaluateOncePerAttempt_Memoized(t *testing.T) {
	tiergate.Require(t, 0)
	callCount := 0
	gate := func() (GateResult, error) {
		callCount++
		return GateResult{Passed: true}, nil
	}
	run := newFixture(t, &fakeReed{}, &fakeEngine{}).newRun(Spec{}, withRunDir(t.TempDir()), withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

	first, err := run.evaluateGate(false)
	if err != nil {
		t.Fatalf("evaluateGate() error: %v", err)
	}
	second, err := run.evaluateGate(false)
	if err != nil {
		t.Fatalf("evaluateGate() error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("gate closure invoked %d times, want 1 (memoised per attempt)", callCount)
	}
	if first != second {
		t.Errorf("evaluateGate() = %+v then %+v, want the same memoised pointer both times", first, second)
	}
}

// TestGate_SendFailsMidLoop_EndsLoopWithAttemptsSoFar covers a re-prompt Send that itself fails.
// The loop ends there rather than retrying, and the failed send charges no attempt.
// A pane that swallows the input fails the send after it was typed.
// A renderer returning two lines fails it before anything is typed.
//
//testtiming:keep pins that a failed re-prompt send ends the loop without a retry and charges no attempt or rework span, and that an invalid rendered line sends nothing
func TestGate_SendFailsMidLoop_EndsLoopWithAttemptsSoFar(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name          string
		sendTextErr   error
		reprompt      func(findingsPath string) string
		wantSendCalls int
	}{
		{name: "the pane swallows the input", sendTextErr: errors.New("pane swallowed input"), wantSendCalls: 1},
		{name: "an entry's renderer returns two lines", reprompt: func(findingsPath string) string { return "line one\nline two " + findingsPath }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, eventsFileName)
			outputFile := filepath.Join(runDir, "out.md")
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			gate := func() (GateResult, error) { return GateResult{Passed: false, Findings: "bad"}, nil }

			reed := &fakeReed{
				StatusQueue:  liveStrandStatus(true),
				CaptureQueue: []string{"idle pane", "idle pane"},
				SendTextErr:  tt.sendTextErr,
			}
			engine := readyAgentEngine()
			fx := newFixture(t, reed, engine, withConfig(gateConfig))
			stubInputSleep(t)

			fc := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
				withRunClock(fc, fc.Now().Add(time.Hour)),
				withRunGate(GateSpec{{Gate: gate, Attempts: 3, Reprompt: tt.reprompt}}))

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
			}
			if result.Gate == nil || result.Gate.Passed {
				t.Errorf("Gate = %+v, want a failed verdict", result.Gate)
			}
			if result.Gate != nil && result.Gate.Attempts != 0 {
				t.Errorf("Attempts = %d, want 0 (the failed send charges no attempt)", result.Gate.Attempts)
			}
			if len(reed.SendTextCalls) != tt.wantSendCalls {
				t.Errorf("SendText calls = %d, want %d (no retry after a failed send)", len(reed.SendTextCalls), tt.wantSendCalls)
			}
			if result.Gate != nil {
				if got := spanKinds(result.Gate.Entries[0].Spans); !reflect.DeepEqual(got, []GateSpanKind{GateSpanGate}) {
					t.Errorf("span kinds = %v, want one gate span and no rework span (a failed send opens none)", got)
				}
			}
		})
	}
}

// TestGate_DeadlineExpiresBetweenAttempts covers a run that ends mid-loop after one successful re-prompt.
// The run ends by its deadline or by a lost session.
// The loop stops, and the Done classification runs the gate once more.
// The reported Attempts stays honest about the one send that preceded it,
// and the rework span is closed at the run's end.
//
//testtiming:keep pins that a deadline expiring or a session lost after one re-prompt runs the gate once more through its own Done, with Attempts still honest at one and the rework span closed at the run's end
func TestGate_DeadlineExpiresBetweenAttempts(t *testing.T) {
	tiergate.Require(t, 0)
	live := liveStrandStatus(true)[0]
	notTracked := reedengine.StatusResult{Strands: nil}
	tests := []struct {
		name string
		cfg  Config
		// statusQueue is consumed FIFO by reed's Status, the last entry repeating.
		statusQueue []reedengine.StatusResult
		// deadlineOffset is where the run deadline sits relative to the clock's start.
		deadlineOffset time.Duration
		// endsBeforeDeadline reports that the run must end before the deadline, so it did not end through the deadline branch.
		endsBeforeDeadline bool
	}{
		{
			// PollIntervalMS's 1s Sleep after the one successful re-prompt crosses the 2ms-out deadline,
			// so the SECOND tick's deadline check trips with no further event ever appended.
			name:           "the deadline expires after the re-prompt",
			cfg:            Config{PollIntervalMS: 1000, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30},
			statusQueue:    liveStrandStatus(true),
			deadlineOffset: 2 * time.Millisecond,
		},
		{
			name:               "the session is lost after the re-prompt",
			cfg:                Config{PollIntervalMS: 1000, LivenessEveryNPolls: 1, StartupTimeoutS: 0},
			statusQueue:        []reedengine.StatusResult{live, live, live, live, notTracked},
			deadlineOffset:     time.Hour,
			endsBeforeDeadline: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, eventsFileName)
			outputFile := filepath.Join(runDir, "out.md")
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			findingsPath := filepath.Join(runDir, gateFindingsFileName)
			gateCalls := 0
			gate := func() (GateResult, error) {
				gateCalls++
				return GateResult{Passed: false, Findings: "still bad"}, nil
			}

			reed := &fakeReed{
				StatusQueue:  tt.statusQueue,
				CaptureQueue: repromptCaptureSequence(findingsPath, 1),
			}
			fx := newFixture(t, reed, readyAgentEngine(), withConfig(tt.cfg))
			stubInputSleep(t)

			fc := newFakeClock(time.Now())
			deadline := fc.Now().Add(tt.deadlineOffset)
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
				withRunClock(fc, deadline),
				withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
			}
			if result.Gate == nil || result.Gate.Passed {
				t.Fatalf("Gate = %+v, want a failed verdict", result.Gate)
			}
			if result.Gate.Attempts != 1 {
				t.Errorf("Attempts = %d, want 1 (the one re-prompt sent before the run ended)", result.Gate.Attempts)
			}
			if gateCalls != 2 {
				t.Errorf("gate closure invoked %d times, want 2 (the failed attempt, then once more on the run's own Done)", gateCalls)
			}
			if len(reed.SendTextCalls) != 1 {
				t.Errorf("SendText calls = %d, want 1 (the run's end never sends)", len(reed.SendTextCalls))
			}
			spans := result.Gate.Entries[0].Spans
			if got := spanKinds(spans); !reflect.DeepEqual(got, []GateSpanKind{GateSpanGate, GateSpanRework, GateSpanGate}) {
				t.Fatalf("span kinds = %v, want gate, rework, gate", got)
			}
			if !spans[1].End.Equal(result.EndedAt) {
				t.Errorf("rework span ends at %v, want result.EndedAt %v (closed at finalize)", spans[1].End, result.EndedAt)
			}
			if tt.endsBeforeDeadline && !result.EndedAt.Before(deadline) {
				t.Errorf("EndedAt = %v, want before the deadline %v", result.EndedAt, deadline)
			}
		})
	}
}

// TestAttach_GateThreading covers a resumed run's gate.
// AttachGated and AttachIfLiveGated gate it exactly as a fresh RunGated run is gated,
// and the plain Attach entry point never carries a gate.
//
//testtiming:keep pins that AttachGated and AttachIfLiveGated gate a resumed run exactly as a fresh one is gated and starts its gate spans afresh, that plain Attach leaves Result.Gate nil, and that the recorded fork authorization, not the attaching spec, chooses the gate closure and the fork audit
func TestAttach_GateThreading(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name  string
		gated bool
		// ifLive attaches through AttachIfLiveGated instead of AttachGated.
		ifLive bool
		// recordedFork seeds the run record's fork authorization; specFork sets the attaching Spec's.
		recordedFork, specFork bool
		// wantForked is whether the Forked closure, not Gate, is the one called.
		wantForked bool
	}{
		{name: "AttachGated threads the gate through reconstruct and wait", gated: true},
		{name: "AttachIfLiveGated threads the gate through reconstruct and wait", gated: true, ifLive: true},
		{name: "Attach leaves Result.Gate nil", gated: false},
		{name: "AttachGated of a fork-authorized record runs Forked and the fork audit, though the attaching spec does not authorize forks", gated: true, recordedFork: true, specFork: false, wantForked: true},
		{name: "AttachGated of a record without fork authorization runs Gate and no fork audit, though the attaching spec authorizes forks", gated: true, recordedFork: false, specFork: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
			engine := &fakeEngine{}
			fx := newFixture(t, reed, engine, withConfig(fastConfig), withSeparateRunDir())
			runner, dotLymDir, runRoot := fx.Runner, fx.DotLym, fx.RunRoot
			seedPresentReedState(t, dotLymDir)

			outputFile := filepath.Join(runRoot, "out.md")
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", sessionID: "session-1",
				outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true,
				forkSubagents: tt.recordedFork,
			})
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			spec := Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, ForkSubagents: tt.specFork}
			gateCalls, forkedCalls := 0, 0
			gate := func() (GateResult, error) {
				gateCalls++
				return GateResult{Passed: true}, nil
			}
			forked := func() (GateResult, error) {
				forkedCalls++
				return GateResult{Passed: true}, nil
			}

			var (
				result Result
				found  bool
				err    error
			)
			if tt.ifLive {
				result, found, err = runner.AttachIfLiveGated(spec, GateSpec{{Gate: gate, Forked: forked, Attempts: 3}})
			} else if tt.gated {
				result, found, err = runner.AttachGated(spec, GateSpec{{Gate: gate, Forked: forked, Attempts: 3}})
			} else {
				result, found, err = runner.Attach(spec)
			}
			if err != nil {
				t.Fatalf("attach error = %v; want nil", err)
			}
			if !found {
				t.Fatal("attach found = false; want true")
			}
			if !tt.gated {
				if result.Gate != nil {
					t.Errorf("Gate = %+v; want nil for the ungated Attach delegation", result.Gate)
				}
				return
			}
			if result.Gate == nil || !result.Gate.Passed {
				t.Errorf("Gate = %+v; want a passed verdict, proving a resumed run is gated exactly as a fresh one is", result.Gate)
			}
			wantGate, wantForkedCalls := 1, 0
			if tt.wantForked {
				wantGate, wantForkedCalls = 0, 1
			}
			if gateCalls != wantGate || forkedCalls != wantForkedCalls {
				t.Errorf("Gate invoked %d times and Forked %d times; want %d and %d", gateCalls, forkedCalls, wantGate, wantForkedCalls)
			}
			// Spans live in the wait loop's memory, so the attached run reports only the gate span of its own evaluation.
			if got := spanKinds(result.Gate.Entries[0].Spans); !reflect.DeepEqual(got, []GateSpanKind{GateSpanGate}) {
				t.Errorf("span kinds = %v; want only the attached run's own gate span", got)
			}
			wantAudits := 0
			if tt.recordedFork {
				wantAudits = 1
			}
			if len(engine.AuditForksCalls) != wantAudits || (result.ForkAudit != nil) != tt.recordedFork {
				t.Errorf("AuditForksCalls = %v and ForkAudit = %+v; want %d audit calls and an audit only for a fork-authorized record", engine.AuditForksCalls, result.ForkAudit, wantAudits)
			}
		})
	}
}

// TestGate_WaitMarkAroundEntry pins that a gate entry is marked on screen and on disk while its closure runs.
// The mark is cleared on every path out.
// A failing SetWaitMark changes neither the verdict nor the error the closure produced.
func TestGate_WaitMarkAroundEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		result     GateResult
		closureErr error
		markErr    error
		wantPassed bool
		wantErr    bool
	}{
		{name: "passing entry", result: GateResult{Passed: true}, wantPassed: true},
		{name: "failing entry", result: GateResult{Findings: "no"}},
		{name: "pending entry", result: GateResult{Pending: true}},
		{name: "erroring entry", closureErr: errors.New("boom"), wantErr: true},
		{name: "a failing SetWaitMark leaves a passing verdict", result: GateResult{Passed: true}, markErr: errors.New("no tmux"), wantPassed: true},
		{name: "a failing SetWaitMark leaves a failing verdict", result: GateResult{Findings: "no"}, markErr: errors.New("no tmux")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			markerPath := filepath.Join(runDir, waitMarkerFileName)
			reed := &fakeReed{WaitMarkErr: tt.markErr}
			var markerSeen WaitMarker
			gate := func() (GateResult, error) {
				data, err := os.ReadFile(markerPath)
				if err != nil {
					t.Errorf("wait marker unreadable while the closure runs: %v", err)
				}
				if err := yaml.Unmarshal(data, &markerSeen); err != nil {
					t.Errorf("wait marker undecodable: %v", err)
				}
				if got := reed.WaitMarkCalls; len(got) != 1 || got[0].Label != "gate review" {
					t.Errorf("mark calls while the closure runs = %+v; want one set of %q", got, "gate review")
				}
				return tt.result, tt.closureErr
			}
			run := newFixture(t, reed, &fakeEngine{}).newRun(Spec{}, withRunDir(runDir),
				withRunGate(GateSpec{{Name: "review", Gate: gate, Attempts: 3, MayHold: true}}))

			outcome, err := run.evaluateGate(false)

			if (err != nil) != tt.wantErr {
				t.Fatalf("evaluateGate error = %v; want error %v", err, tt.wantErr)
			}
			if err == nil && outcome.Passed != tt.wantPassed {
				t.Errorf("Passed = %v; want %v", outcome.Passed, tt.wantPassed)
			}
			if markerSeen.Kind != "gate review" || markerSeen.PID != os.Getpid() {
				t.Errorf("marker seen = %+v; want kind %q and this process's pid", markerSeen, "gate review")
			}
			if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
				t.Errorf("wait marker survives the closure: %v", statErr)
			}
			if got := reed.WaitMarkCalls; len(got) != 2 || got[1].Label != "" {
				t.Errorf("mark calls = %+v; want a set then a clear", got)
			}
		})
	}
}

// hookClock is a fakeClock that calls onSleep with the new time after every Sleep.
// A test uses it to move the writer between two ticks.
type hookClock struct {
	*fakeClock
	onSleep func(now time.Time)
}

func (c *hookClock) Sleep(d time.Duration) {
	c.fakeClock.Sleep(d)
	if c.onSleep != nil {
		c.onSleep(c.fakeClock.Now())
	}
}

var _ Clock = (*hookClock)(nil)

// gateRig is a gated run over a tuiReed and a tuiEngine whose events file starts as seeded.
type gateRig struct {
	run        *Run
	reed       *tuiReed
	engine     *tuiEngine
	clock      *hookClock
	start      time.Time
	eventsPath string
	findings   string
	// gateCalls counts the gate closure's evaluations.
	gateCalls int
}

// gateRigSettings is what a gateRigOption adjusts before newGateRig builds the rig.
type gateRigSettings struct {
	// entry replaces the rig's always-failing gate entry when non-nil.
	entry *GateEntry
	// strandName is the run record's StrandName.
	strandName string
	// engine replaces the rig's tuiEngine as the run's engine when non-nil.
	engine Engine
}

// withGateEngine runs the rig's run on engine in place of its tuiEngine.
func withGateEngine(engine Engine) gateRigOption {
	return func(s *gateRigSettings) { s.engine = engine }
}

// gateRigOption adjusts one setting of newGateRig.
type gateRigOption func(*gateRigSettings)

// withGateEntry replaces the rig's gate entry with entry.
func withGateEntry(entry GateEntry) gateRigOption {
	return func(s *gateRigSettings) { s.entry = &entry }
}

// withStrandName sets the StrandName the rig's run record holds.
func withStrandName(name string) gateRigOption {
	return func(s *gateRigSettings) { s.strandName = name }
}

// newGateRig builds the rig with a 30 s run deadline and one-second ticks.
// Its gate always fails and allows three attempts.
// cfg supplies the send windows; the poll cadence and liveness are set here.
func newGateRig(t *testing.T, cfg Config, events string, opts ...gateRigOption) *gateRig {
	t.Helper()
	var settings gateRigSettings
	for _, opt := range opts {
		opt(&settings)
	}
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte(events), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	cfg.StartupTimeoutS, cfg.RunTimeoutMin, cfg.PollIntervalMS, cfg.LivenessEveryNPolls = 30, 5, 1000, 1_000_000
	cfg.SubmitSettleMS = 100

	rig := &gateRig{eventsPath: eventsPath, findings: filepath.Join(runDir, gateFindingsFileName)}
	rig.clock = &hookClock{fakeClock: newFakeClock(time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC))}
	rig.start = rig.clock.Now()
	rig.reed = &tuiReed{fakeReed: &fakeReed{StatusQueue: liveStrandStatus(true)}, clock: rig.clock}
	rig.engine = newTUIEngine()
	gate := func() (GateResult, error) {
		rig.gateCalls++
		return GateResult{Passed: false, Findings: "bad"}, nil
	}
	entry := GateEntry{Gate: gate, Attempts: 3}
	if settings.entry != nil {
		entry = *settings.entry
	}
	var engine Engine = rig.engine
	if settings.engine != nil {
		engine = settings.engine
	}
	rig.run = newFixture(t, rig.reed, engine, withConfig(cfg)).newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", StrandName: settings.strandName, SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(rig.clock, rig.start.Add(30*time.Second)),
		withRunGate(GateSpec{entry}))
	return rig
}

// resumeOldestLive resumes the oldest live entry of the pause record at path at the given time.
func resumeOldestLive(t *testing.T, path string, at time.Time) {
	t.Helper()
	_, err := pauserecord.Update(path, at, func(record *pauserecord.Record) error {
		entry := record.Live[0]
		entry.ResumedAt = at
		record.Live = record.Live[1:]
		record.Resumed = append(record.Resumed, entry)
		return nil
	})
	if err != nil {
		t.Fatalf("resume the pause: %v", err)
	}
}

// TestGate_HoldsItsBoundaryWhileCovered pins the boundary of a gated Wait under a pause.
// A pause that covers the run's own session, by that session's slug, stops the gate closure and the typing.
// Both happen again after the resume.
// A failing gate's re-prompt spends one attempt.
// A closure's pending send goes out on each arrival.
// An expiry probe still finalizes a covered run whose writer is off the boundary.
func TestGate_HoldsItsBoundaryWhileCovered(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const pendingLine = "carry on"
	failing := func(evals *int) GateEntry {
		return GateEntry{Attempts: 3, Gate: func() (GateResult, error) {
			*evals++
			return GateResult{Passed: false, Findings: "bad"}, nil
		}}
	}
	prime := reedengine.StrandStatus{GUID: "strand-1", PaneID: "%0", Live: true, Name: "ly:p1:impl"}
	tests := []struct {
		name string
		cfg  Config
		// events seeds the events file.
		events string
		// strandName is the run record's StrandName; statusName is the name reed's status reports for the strand.
		strandName, statusName, runnerSlug string
		// initialScope is the scope of the entry live from the start, empty for none.
		initialScope string
		// addAt adds a hub entry once the clock passes it, zero for never; appendAt appends a turn end to the events file;
		// resumeAt resumes the oldest live entry, zero for never.
		addAt, appendAt, resumeAt time.Duration
		// activeFor keeps an operator active in the pane that long from the start.
		activeFor time.Duration
		// entry builds the rig's gate entry; evals counts its closure runs, and add adds a hub entry at the rig's clock.
		entry func(evals *int, add func()) GateEntry
		// wantEvalsAtResume and wantSendsAtResume are the closure runs and typed texts at the resume; -1 is unchecked.
		wantEvalsAtResume, wantSendsAtResume int
		// wantEvalsSoonAfterResume is the closure run count two seconds after the resume, within gateRecheckEvery.
		// Zero is unchecked.
		wantEvalsSoonAfterResume int
		wantSends                []string
		// wantAttempts is the attempts the result reports spent, -1 for unchecked.
		wantAttempts int
		// wantCachedSlug is the slug the run caches from reed's status, empty for none.
		wantCachedSlug string
	}{
		{
			name: "a failing gate's arrival under a hub entry waits for the resume and re-prompts once", events: "STOP:turn1\n",
			runnerSlug: "slug", initialScope: pauserecord.ScopeHub, resumeAt: 5 * time.Second,
			entry:             func(evals *int, _ func()) GateEntry { return failing(evals) },
			wantEvalsAtResume: 0, wantSendsAtResume: 0, wantAttempts: 1,
		},
		{
			name: "a slug entry holds a run named for it under a runner of another slug", events: "STOP:turn1\n",
			strandName: "ly:p1:impl", runnerSlug: "other", initialScope: "p1", resumeAt: 5 * time.Second,
			entry:             func(evals *int, _ func()) GateEntry { return failing(evals) },
			wantEvalsAtResume: 0, wantSendsAtResume: 0, wantAttempts: 1,
		},
		{
			name: "a record with no name takes the slug from reed's status, caches it and holds until the resume", events: "STOP:turn1\n",
			statusName: "ly:p1:impl", runnerSlug: "other", initialScope: "p1", resumeAt: 5 * time.Second,
			entry:             func(evals *int, _ func()) GateEntry { return failing(evals) },
			wantEvalsAtResume: 0, wantSendsAtResume: 0, wantAttempts: 1, wantCachedSlug: "p1",
		},
		{
			name: "a closure that adds a hub entry while it fails has its re-prompt refused and sent after the resume", events: "STOP:turn1\n",
			runnerSlug: "slug", resumeAt: 5 * time.Second,
			entry: func(evals *int, add func()) GateEntry {
				return GateEntry{Attempts: 3, Gate: func() (GateResult, error) {
					*evals++
					if *evals == 1 {
						add()
					}
					return GateResult{Passed: false, Findings: "bad"}, nil
				}}
			},
			wantEvalsAtResume: 1, wantSendsAtResume: 0, wantAttempts: 1,
		},
		{
			name: "a pending send is not repeated while covered and resumes with the next arrival", events: "STOP:turn1\n",
			runnerSlug: "slug", addAt: time.Second, appendAt: 2 * time.Second, resumeAt: 6 * time.Second,
			entry: func(evals *int, _ func()) GateEntry {
				pending := func() (GateResult, error) {
					*evals++
					return GateResult{Pending: true, Send: pendingLine}, nil
				}
				return GateEntry{Attempts: 3, PassOnCap: true, Gate: pending, Final: func() (GateResult, error) { return GateResult{Passed: true}, nil }}
			},
			wantEvalsAtResume: 1, wantSendsAtResume: 1, wantSends: []string{pendingLine, pendingLine}, wantAttempts: -1,
		},
		{
			name: "a re-prompt left unsent by an active operator is not retried while covered", cfg: Config{SendReadyTimeoutS: 1}, events: "STOP:turn1\n",
			runnerSlug: "slug", addAt: 2 * time.Second, activeFor: 3 * time.Second, resumeAt: 7 * time.Second,
			entry:             func(evals *int, _ func()) GateEntry { return failing(evals) },
			wantEvalsAtResume: -1, wantSendsAtResume: 0, wantAttempts: 1,
		},
		{
			name: "an expiry probe finalizes a covered run whose writer is off the boundary", events: "START\n",
			runnerSlug: "slug", initialScope: pauserecord.ScopeHub,
			entry: func(evals *int, _ func()) GateEntry {
				entry := failing(evals)
				entry.Expired = func() (bool, error) { return true, nil }
				return entry
			},
			wantEvalsAtResume: -1, wantSendsAtResume: -1, wantAttempts: 0,
		},
		{
			name: "a gated arrival on the first uncovered tick evaluates the boundary once, not again for the arrival held while covered", events: "STOP:turn1\n",
			runnerSlug: "slug", initialScope: pauserecord.ScopeHub, appendAt: 5 * time.Second, resumeAt: 5 * time.Second,
			entry: func(evals *int, _ func()) GateEntry {
				pending := func() (GateResult, error) {
					*evals++
					return GateResult{Pending: true}, nil
				}
				return GateEntry{Attempts: 3, PassOnCap: true, Gate: pending, Final: func() (GateResult, error) { return GateResult{Passed: true}, nil }}
			},
			wantEvalsAtResume: -1, wantSendsAtResume: -1, wantEvalsSoonAfterResume: 1, wantAttempts: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
			var rig *gateRig
			evals := 0
			entry := tt.entry(&evals, func() { addLiveEntry(t, path, pauserecord.ScopeHub, rig.clock.Now()) })
			rig = newGateRig(t, tt.cfg, tt.events, withGateEntry(entry), withStrandName(tt.strandName))
			if tt.statusName != "" {
				named := prime
				named.Name = tt.statusName
				rig.reed.StatusQueue = []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{named}}}
			}
			rig.reed.activeUntil = rig.start.Add(tt.activeFor)
			rig.run.runner.SetPause(pauserecord.Geometry{RecordPath: path, Slug: tt.runnerSlug})
			if tt.initialScope != "" {
				addLiveEntry(t, path, tt.initialScope, rig.start)
			}

			added, appended, resumed := tt.addAt == 0, tt.appendAt == 0, tt.resumeAt == 0
			evalsAtResume, sendsAtResume, evalsSoonAfterResume := -1, -1, -1
			rig.clock.onSleep = func(now time.Time) {
				elapsed := now.Sub(rig.start)
				if resumed && tt.resumeAt > 0 && evalsSoonAfterResume < 0 && elapsed >= tt.resumeAt+2*time.Second {
					evalsSoonAfterResume = evals
				}
				if !added && elapsed >= tt.addAt {
					added = true
					addLiveEntry(t, path, pauserecord.ScopeHub, now)
				}
				if !appended && elapsed >= tt.appendAt {
					appended = true
					appendEventsLine(t, rig.eventsPath, "STOP:turn2")
				}
				if !resumed && elapsed >= tt.resumeAt {
					resumed = true
					evalsAtResume, sendsAtResume = evals, len(rig.reed.SendTextCalls)
					resumeOldestLive(t, path, now)
				}
			}

			result, err := rig.run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}

			if tt.wantEvalsAtResume >= 0 && evalsAtResume != tt.wantEvalsAtResume {
				t.Errorf("closure runs at the resume = %d, want %d", evalsAtResume, tt.wantEvalsAtResume)
			}
			if tt.wantSendsAtResume >= 0 && sendsAtResume != tt.wantSendsAtResume {
				t.Errorf("texts typed at the resume = %d, want %d", sendsAtResume, tt.wantSendsAtResume)
			}
			if tt.wantEvalsSoonAfterResume != 0 && evalsSoonAfterResume != tt.wantEvalsSoonAfterResume {
				t.Errorf("closure runs two seconds after the resume = %d, want %d", evalsSoonAfterResume, tt.wantEvalsSoonAfterResume)
			}
			var sent []string
			for _, call := range rig.reed.SendTextCalls {
				sent = append(sent, call.Text)
			}
			switch {
			case tt.wantSends != nil:
				if !reflect.DeepEqual(sent, tt.wantSends) {
					t.Errorf("texts typed = %q, want %q", sent, tt.wantSends)
				}
			case tt.wantEvalsSoonAfterResume != 0:
			case tt.resumeAt > 0:
				if len(sent) != 1 || rig.reed.typedAt.Sub(rig.start) < tt.resumeAt {
					t.Errorf("texts typed = %q, first at %v, want one re-prompt at or after the resume at %v", sent, rig.reed.typedAt.Sub(rig.start), tt.resumeAt)
				}
			default:
				if len(sent) != 0 || rig.clock.Now().Sub(rig.start) >= 30*time.Second {
					t.Errorf("texts typed = %q, ended %v after the start, want nothing typed and an end before the deadline", sent, rig.clock.Now().Sub(rig.start))
				}
			}
			if tt.wantAttempts >= 0 && (result.Gate == nil || result.Gate.Attempts != tt.wantAttempts) {
				t.Errorf("Gate = %+v, want %d attempts spent", result.Gate, tt.wantAttempts)
			}
			if rig.run.sessionSlugKnown != (tt.wantCachedSlug != "") || rig.run.sessionSlug != tt.wantCachedSlug {
				t.Errorf("cached slug = %q (known %v), want %q", rig.run.sessionSlug, rig.run.sessionSlugKnown, tt.wantCachedSlug)
			}
		})
	}
}

// TestGate_WaitsForIdleWriter drives a gated Wait over a writer whose newest turn start has no matching turn end.
// This is the state an AttachGated replay of a writer mid-turn finds.
// No gate send is typed until one of these holds:
// a turn end follows the turn start;
// the engine reports the turn interrupted;
// a session start follows it;
// or the pane has read idle for turnStartIdleOverride while the engine does not read the turn in flight.
// All but the turn end restore the boundary and evaluate the gate.
func TestGate_WaitsForIdleWriter(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name        string
		interrupted bool
		// progress is what the engine reads of any turn start.
		progress TurnProgress
		// events seeds the events file; empty is a turn end then an unmatched turn start.
		events string
		// turnEndAt, when positive, is when the writer's turn end lands.
		turnEndAt time.Duration
		// wantTypedMin and wantTypedMax bound the first typed re-prompt from the start.
		wantTypedMin, wantTypedMax time.Duration
	}{
		{name: "a turn end after the turn start releases the boundary", turnEndAt: 4 * time.Second, wantTypedMin: 4 * time.Second, wantTypedMax: 6 * time.Second},
		{name: "an interrupt report restores the boundary at once", interrupted: true, wantTypedMax: 3 * time.Second},
		{name: "ten seconds of idle pane restores the boundary", wantTypedMin: turnStartIdleOverride, wantTypedMax: 15 * time.Second},
		{name: "an in-flight turn start holds the boundary past the override until its turn end", progress: TurnInFlight, turnEndAt: 14 * time.Second, wantTypedMin: 14 * time.Second, wantTypedMax: 16 * time.Second},
		{name: "a session start after an in-flight turn start restores the boundary by the override", progress: TurnInFlight, events: "STOP:turn1\nSTART\nSESSIONSTART:resume\n", wantTypedMin: turnStartIdleOverride, wantTypedMax: 15 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			events := tt.events
			if events == "" {
				events = "STOP:turn1\nSTART\n"
			}
			rig := newGateRig(t, Config{}, events)
			rig.engine.interrupted = tt.interrupted
			rig.engine.progress = tt.progress
			if tt.turnEndAt > 0 {
				appended := false
				rig.clock.onSleep = func(now time.Time) {
					if !appended && now.Sub(rig.start) >= tt.turnEndAt {
						appended = true
						appendEventsLine(t, rig.eventsPath, "STOP:turn2")
					}
				}
			}

			result, err := rig.run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if len(rig.reed.SendTextCalls) != 1 {
				t.Fatalf("SendText calls = %+v, want exactly one re-prompt", rig.reed.SendTextCalls)
			}
			if typedAfter := rig.reed.typedAt.Sub(rig.start); typedAfter < tt.wantTypedMin || typedAfter > tt.wantTypedMax {
				t.Errorf("re-prompt typed %v after the start, want within [%v, %v]", typedAfter, tt.wantTypedMin, tt.wantTypedMax)
			}
			if result.Gate == nil || result.Gate.Attempts != 1 {
				t.Errorf("Gate = %+v, want one attempt spent", result.Gate)
			}
		})
	}
}

// TestGate_UnsentRepromptRetries covers a re-prompt that cannot be delivered.
// Busy or unlanded, it spends no attempt and is re-sent on a later tick.
// A writer turn that starts meanwhile cancels it.
// Its idle wait and submit window end by the run deadline,
// and a re-prompt that stays undelivered is retried until the run deadline.
func TestGate_UnsentRepromptRetries(t *testing.T) {
	tiergate.Require(t, 0)
	const never = 24 * time.Hour
	tests := []struct {
		name string
		cfg  Config
		// activeFor and swallowEnterFor keep an operator active in the pane, or swallow its Enters, for that long from the start.
		activeFor, swallowEnterFor time.Duration
		// startAt, when positive, is when the writer begins a new turn.
		startAt time.Duration
		// wantSends is the SendText count: -1 means more than one.
		wantSends    int
		wantAttempts int
		// wantTypedMin bounds the first typed text from the start.
		wantTypedMin time.Duration
		wantClears   bool
		// wantEndBy bounds the clock at the end, from the start, under a 3 s run deadline; zero leaves it unchecked.
		wantEndBy time.Duration
		// wantEndAtLeast bounds the clock at the end from below, from the start, under the rig's own run deadline;
		// zero leaves it unchecked.
		wantEndAtLeast time.Duration
		// wantNoEvalAfterStart pins that the gate is not evaluated after the writer's new turn.
		// The deadline's final evaluation is the one exception.
		wantNoEvalAfterStart bool
		// wantHeldInfo asserts the held re-prompt logged one INFO line and no WARN one.
		wantHeldInfo bool
	}{
		{
			name: "a re-prompt held by an active operator is re-sent once the operator goes quiet", cfg: Config{SendReadyTimeoutS: 1},
			activeFor: 5 * time.Second, wantSends: 1, wantAttempts: 1, wantTypedMin: 5 * time.Second, wantHeldInfo: true,
		},
		{
			name: "an unlanded re-prompt is re-sent into the box its failed send cleared", cfg: Config{SubmitConfirmTimeoutS: 1},
			swallowEnterFor: 8 * time.Second, wantSends: -1, wantAttempts: 1, wantClears: true,
		},
		{
			name: "a writer turn starting meanwhile cancels the pending re-send", cfg: Config{SendReadyTimeoutS: 1},
			activeFor: never, startAt: 4 * time.Second, wantSends: 0, wantAttempts: 0, wantNoEvalAfterStart: true,
		},
		{
			// Each busy send waits out its 1 s idle window and a 1 s tick follows,
			// so the retries run until the 30 s deadline ends the loop.
			name: "a re-prompt that stays undelivered is retried until the run deadline", cfg: Config{SendReadyTimeoutS: 1},
			activeFor: never, wantSends: 0, wantAttempts: 0, wantEndAtLeast: 30 * time.Second, wantHeldInfo: true,
		},
		{
			name: "a re-prompt with little time left ends its idle wait by the run deadline", cfg: Config{SendReadyTimeoutS: 60},
			activeFor: never, wantSends: 0, wantAttempts: 0, wantEndBy: 10 * time.Second,
		},
		{
			name: "a re-prompt with little time left ends its submit window by the run deadline", cfg: Config{SubmitConfirmTimeoutS: 60},
			swallowEnterFor: never, wantSends: 1, wantAttempts: 0, wantEndBy: 10 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)

			rig := newGateRig(t, tt.cfg, "STOP:turn1\n")
			rig.reed.activeUntil = rig.start.Add(tt.activeFor)
			rig.reed.swallowEnterUntil = rig.start.Add(tt.swallowEnterFor)
			if tt.wantEndBy > 0 {
				rig.run.deadline = rig.start.Add(3 * time.Second)
			}
			callsAtStart := -1
			if tt.startAt > 0 {
				rig.clock.onSleep = func(now time.Time) {
					if callsAtStart < 0 && now.Sub(rig.start) >= tt.startAt {
						callsAtStart = rig.gateCalls
						rig.reed.working = true
						appendEventsLine(t, rig.eventsPath, "START")
					}
				}
			}

			result, err := rig.run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			sends := len(rig.reed.SendTextCalls)
			if tt.wantSends == -1 && sends < 2 || tt.wantSends >= 0 && sends != tt.wantSends {
				t.Errorf("SendText calls = %d, want %d", sends, tt.wantSends)
			}
			if result.Gate == nil || result.Gate.Attempts != tt.wantAttempts {
				t.Errorf("Gate = %+v, want %d attempts spent", result.Gate, tt.wantAttempts)
			}
			// A failed or unlanded send opens no rework span; the delivery that landed opens exactly one.
			reworks := 0
			for _, span := range result.Gate.Entries[0].Spans {
				if span.Kind != GateSpanRework {
					continue
				}
				reworks++
				if span.Start.Before(rig.reed.typedAt) {
					t.Errorf("rework span starts at %v, before the first text was typed at %v", span.Start, rig.reed.typedAt)
				}
			}
			if reworks != tt.wantAttempts {
				t.Errorf("rework spans = %d, want %d", reworks, tt.wantAttempts)
			}
			if tt.wantTypedMin > 0 && rig.reed.typedAt.Sub(rig.start) < tt.wantTypedMin {
				t.Errorf("first text typed %v after the start, want at least %v", rig.reed.typedAt.Sub(rig.start), tt.wantTypedMin)
			}
			if tt.wantClears && rig.reed.clears == 0 {
				t.Error("no C-u was played, want the unlanded text cleared from the box")
			}
			if tt.wantClears {
				if len(rig.reed.history) != 1 || !strings.Contains(rig.reed.history[0], rig.findings) || rig.reed.box != "" {
					t.Errorf("history %q and box %q, want the re-prompt submitted once into an otherwise empty box", rig.reed.history, rig.reed.box)
				}
			}
			if tt.wantEndBy > 0 && rig.clock.Now().Sub(rig.start) > tt.wantEndBy {
				t.Errorf("ended %v after the start, want by %v", rig.clock.Now().Sub(rig.start), tt.wantEndBy)
			}
			if tt.wantEndAtLeast > 0 && rig.clock.Now().Sub(rig.start) < tt.wantEndAtLeast {
				t.Errorf("ended %v after the start, want at least %v", rig.clock.Now().Sub(rig.start), tt.wantEndAtLeast)
			}
			if tt.wantNoEvalAfterStart && rig.gateCalls != callsAtStart+1 {
				t.Errorf("gate evaluated %d times, want %d: the calls up to the new turn plus the deadline's final evaluation", rig.gateCalls, callsAtStart+1)
			}
			if tt.wantHeldInfo {
				const held = "re-prompt not delivered"
				if info, warn := countLevelLines(buf.String(), "INFO", held), countLevelLines(buf.String(), "WARN", held); info != 1 || warn != 0 {
					t.Errorf("held re-prompt lines: INFO %d, WARN %d; want 1 and 0; log: %s", info, warn, buf.String())
				}
			}
		})
	}
}

// TestGate_RepromptOverPicker pins a re-prompt whose pane shows a picker in place of the input box.
// The engine under test has the readiness reading.
// A known picker is dismissed with its key, and the re-prompt is delivered.
// A pane with no input box that the dismissal does not clear ends the loop after maxNoInputBoxReprompts held sends.
// Such a loop spends no attempt.
// For a known picker it sends one key per send, whether or not the key can be sent; for an unknown one it sends none.
// An operator active over a known picker gets no key, and the re-prompt is held until the run deadline.
// It is not parallel: it captures the process-wide logger.
func TestGate_RepromptOverPicker(t *testing.T) {
	tiergate.Require(t, 0)
	const boundWarn = "re-prompt held by a pane with no input box past the bound"
	tests := []struct {
		name         string
		picker       string
		sticky       bool
		active       bool
		sendKeyErr   bool
		wantSends    int
		wantAttempts int
		wantEscapes  int
		// wantEndBefore bounds the clock at the end from above, from the start; zero leaves it unchecked.
		wantEndBefore time.Duration
		// wantEndAtLeast bounds the clock at the end from below, from the start; zero leaves it unchecked.
		wantEndAtLeast time.Duration
		wantBoundWarn  bool
	}{
		{
			name: "a known picker is dismissed and the re-prompt delivered", picker: "COMPACT-PICKER",
			wantSends: 1, wantAttempts: 1, wantEscapes: 1,
		},
		{
			name: "a known picker its key does not close ends the loop after the bound", picker: "COMPACT-PICKER", sticky: true,
			wantEscapes: maxNoInputBoxReprompts, wantEndBefore: 30 * time.Second, wantBoundWarn: true,
		},
		{
			name: "a known picker whose key cannot be sent ends the loop after the bound", picker: "COMPACT-PICKER", sticky: true, sendKeyErr: true,
			wantEscapes: maxNoInputBoxReprompts, wantEndBefore: 30 * time.Second, wantBoundWarn: true,
		},
		{
			name: "an unknown picker ends the loop after the bound with no key", picker: "some menu", sticky: true,
			wantEndBefore: 30 * time.Second, wantBoundWarn: true,
		},
		{
			name: "an operator active over a known picker gets no key and holds until the deadline", picker: "COMPACT-PICKER", active: true,
			wantEndAtLeast: 30 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)

			engine := &dismissPickerEngine{readinessEngine: readinessEngine{idleEngine: idleEngine{sessionFakeEngine: sessionFakeEngine{liveness: LivenessAlive}}}}
			rig := newGateRig(t, Config{SendReadyTimeoutS: 1}, "STOP:turn1\n", withGateEngine(engine))
			rig.reed.picker, rig.reed.pickerSticky = tt.picker, tt.sticky
			rig.reed.ActiveQueue = []bool{tt.active}
			if tt.sendKeyErr {
				rig.reed.SendKeyErr = errors.New("tmux: no such pane")
			}

			result, err := rig.run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if sends := len(rig.reed.SendTextCalls); sends != tt.wantSends {
				t.Errorf("SendText calls = %d, want %d", sends, tt.wantSends)
			}
			if result.Gate == nil || result.Gate.Attempts != tt.wantAttempts {
				t.Errorf("Gate = %+v, want %d attempts spent", result.Gate, tt.wantAttempts)
			}
			escapes := 0
			for _, call := range rig.reed.SendKeyCalls {
				if call.Key == "Escape" {
					escapes++
				}
			}
			if escapes != tt.wantEscapes {
				t.Errorf("Escape keys = %d, want %d", escapes, tt.wantEscapes)
			}
			ended := rig.clock.Now().Sub(rig.start)
			if tt.wantEndBefore > 0 && ended >= tt.wantEndBefore {
				t.Errorf("ended %v after the start, want before %v", ended, tt.wantEndBefore)
			}
			if tt.wantEndAtLeast > 0 && ended < tt.wantEndAtLeast {
				t.Errorf("ended %v after the start, want at least %v", ended, tt.wantEndAtLeast)
			}
			wantWarns := 0
			if tt.wantBoundWarn {
				wantWarns = 1
			}
			if warns := countLevelLines(buf.String(), "WARN", boundWarn); warns != wantWarns {
				t.Errorf("bound WARN lines = %d, want %d; log: %s", warns, wantWarns, buf.String())
			}
		})
	}
}

// TestAwaitIdleSession_FailedDismissalKeyHoldsAsNoInputBox pins a send held at its first poll:
// its dismissal key could not be sent, and the send still reports a pane with no input box.
// The gated loop counts it toward its bound.
func TestAwaitIdleSession_FailedDismissalKeyHoldsAsNoInputBox(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	engine := &dismissPickerEngine{readinessEngine: readinessEngine{idleEngine: idleEngine{sessionFakeEngine: sessionFakeEngine{liveness: LivenessAlive}}}}
	rig := newGateRig(t, Config{SendReadyTimeoutS: 1}, "STOP:turn1\n", withGateEngine(engine))
	rig.reed.picker, rig.reed.pickerSticky = "COMPACT-PICKER", true
	rig.reed.SendKeyErr = errors.New("tmux: no such pane")
	sc := rig.run.newSendContext()
	sc.deadline = rig.clock.Now()

	err := awaitIdleSession(sc)

	var busy *SessionBusyError
	if !errors.As(err, &busy) || !busy.NoInputBox {
		t.Fatalf("awaitIdleSession() error = %v, want a *SessionBusyError with NoInputBox set", err)
	}
	if keys := rig.reed.SendKeyCalls; len(keys) != 1 || keys[0].Key != "Escape" {
		t.Errorf("keys = %+v, want one Escape", keys)
	}
}
