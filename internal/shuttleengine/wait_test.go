// wait_test.go covers Run.Wait's poll loop against fakeMux/fakeEngine and a
// fake clock: all four outcome classifications, KeepPane skipping cleanup,
// the startup probe's trust-dismiss and fast-fail-on-timeout paths,
// multi-Stop offset tracking, events-offset resilience across a partial
// line, and finalize's fork-audit attach (only for a fork-mode spec's done
// classification).

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// fakeClock is a virtual clock: Sleep instantly advances Now() by d instead
// of blocking, so Wait's poll loop runs an arbitrarily long scripted
// sequence at zero real wall-clock cost.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

var _ clock = (*fakeClock)(nil)

// scriptedClock wraps a fakeClock and runs onSleep once, after the first
// Sleep call, letting a test mutate on-disk fixtures (e.g. completing a
// partial events.jsonl line) exactly between two poll ticks.
type scriptedClock struct {
	*fakeClock
	onSleep func()
	fired   bool
}

func (c *scriptedClock) Sleep(d time.Duration) {
	c.fakeClock.Sleep(d)
	if !c.fired && c.onSleep != nil {
		c.fired = true
		c.onSleep()
	}
}

var _ clock = (*scriptedClock)(nil)

// newWaitTestRunner returns a Runner over mux/engine scoped to a fresh temp
// worktree, matching newTestRunner in run_test.go but kept local to this
// file since wait tests construct their Run handles directly rather than
// through Start.
func newWaitTestRunner(t *testing.T, mux MuxOps, engine Engine, cfg Config) *Runner {
	t.Helper()
	root := t.TempDir()
	layout := &hubgeometry.Layout{Cwd: root, WorktreeRoot: root}
	return NewRunner(mux, engine, layout, cfg)
}

// TestPollInterval_FloorsNonPositive pins the busy-spin guard: a configured
// poll_interval_ms of 0 or below must fall back to the template default
// rather than making Wait tick with a zero sleep.
func TestPollInterval_FloorsNonPositive(t *testing.T) {
	tests := []struct {
		name       string
		intervalMS int
		want       time.Duration
	}{
		{"zero_floored", 0, defaultPollIntervalMS * time.Millisecond},
		{"negative_floored", -100, defaultPollIntervalMS * time.Millisecond},
		{"positive_passthrough", 250, 250 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pollInterval(Config{PollIntervalMS: tt.intervalMS})
			if got != tt.want {
				t.Errorf("pollInterval({PollIntervalMS: %d}) = %v; want %v", tt.intervalMS, got, tt.want)
			}
		})
	}
}

func TestRun_Wait_DoneHappyPath_CleansUp(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}
	if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	mux := &fakeMux{StatusQueue: []muxengine.StatusResult{{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}

	foundRemove := false
	for _, c := range mux.RemoveStrandCalls {
		if c.GUID == "strand-1" && !c.Recursive {
			foundRemove = true
		}
	}
	if !foundRemove {
		t.Errorf("RemoveStrand(strand-1, false) not recorded, calls = %+v", mux.RemoveStrandCalls)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Errorf("run dir still exists after done cleanup, stat err = %v", err)
	}
}

func TestRun_Wait_DoneWithKeepPane_SkipsCleanup(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}
	if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	mux := &fakeMux{StatusQueue: []muxengine.StatusResult{{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, KeepPane: true},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if len(mux.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v, want none (KeepPane)", mux.RemoveStrandCalls)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Errorf("run dir removed despite KeepPane: %v", err)
	}
}

func TestRun_Wait_Asking_CarriesMessageKeepsStrand(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md") // never created

	if err := os.WriteFile(eventsPath, []byte("STOP:need operator input\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	mux := &fakeMux{StatusQueue: []muxengine.StatusResult{{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if result.LastAssistantMessage != "need operator input" {
		t.Errorf("LastAssistantMessage = %q, want %q", result.LastAssistantMessage, "need operator input")
	}
	if len(mux.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v, want none (asking keeps the strand)", mux.RemoveStrandCalls)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Errorf("run dir removed for asking outcome: %v", err)
	}
}

// TestRun_Wait_LiveAsk_ClassifiesRealTimeAsking verifies a live ask (an
// EventAsk with no output files present) classifies OutcomeAsking carrying
// the question as the message, keeping the pane and run dir just like the
// existing turn-end asking case — proving the unchanged pollEventsTick
// branch also covers the live-ask signal ParseEvents now emits.
func TestRun_Wait_LiveAsk_ClassifiesRealTimeAsking(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md") // never created

	if err := os.WriteFile(eventsPath, []byte("ASK:which approach?\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	mux := &fakeMux{StatusQueue: []muxengine.StatusResult{{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if result.LastAssistantMessage != "which approach?" {
		t.Errorf("LastAssistantMessage = %q, want %q", result.LastAssistantMessage, "which approach?")
	}
	if len(mux.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v, want none (asking keeps the strand)", mux.RemoveStrandCalls)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Errorf("run dir removed for asking outcome: %v", err)
	}
}

// TestRun_Wait_LiveAsk_DoneFirstStillWins verifies that when a live ask
// arrives but the output files already exist, done-first classification
// still wins — an EventAsk never overrides an already-satisfied file
// contract.
func TestRun_Wait_LiveAsk_DoneFirstStillWins(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}
	if err := os.WriteFile(eventsPath, []byte("ASK:which approach?\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	mux := &fakeMux{StatusQueue: []muxengine.StatusResult{{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q (done-first over a live ask when output files exist)", result.Outcome, OutcomeDone)
	}
}

func TestRun_Wait_Died_ViaStatusNotLive(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl") // never created
	outputFile := filepath.Join(runDir, "out.md")       // never created

	mux := &fakeMux{StatusQueue: []muxengine.StatusResult{{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: false}}}}}
	engine := &fakeEngine{}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDied {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDied)
	}
	if len(mux.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v, want none (died keeps the strand)", mux.RemoveStrandCalls)
	}
}

func TestRun_Wait_Died_ButOutputFilesExist_ClassifiesDone(t *testing.T) {
	// The pane died (mux.Status reports not live) but every output file
	// already exists on disk — the agent must have written its result and
	// then been killed (or exited) before its Stop hook ever appended a
	// turn-end line, so pollEventsTick had nothing to classify from. The
	// file contract is still satisfied: this must report done, not died, so
	// a caller does not needlessly respawn already-completed work.
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl") // never created: no Stop event fired
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}

	mux := &fakeMux{StatusQueue: []muxengine.StatusResult{{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: false}}}}}
	engine := &fakeEngine{}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q (file contract satisfied despite a dead pane and no Stop event)", result.Outcome, OutcomeDone)
	}
	// A "done" outcome without KeepPane still runs the normal cleanup path.
	foundRemove := false
	for _, c := range mux.RemoveStrandCalls {
		if c.GUID == "strand-1" && !c.Recursive {
			foundRemove = true
		}
	}
	if !foundRemove {
		t.Errorf("RemoveStrand(strand-1, false) not recorded, calls = %+v", mux.RemoveStrandCalls)
	}
}

func TestRun_Wait_Died_ViaStartupTimeout_TrustDismissRecorded(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl") // never created
	outputFile := filepath.Join(runDir, "out.md")       // never created

	mux := &fakeMux{StatusQueue: []muxengine.StatusResult{{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	// First probe sees the trust prompt (dismissed with Enter); every probe
	// after that sees a still-booting pane, so the run never becomes ready
	// and eventually fast-fails once the startup deadline passes.
	engine := &fakeEngine{StartupScript: []StartupState{StartupTrustPrompt, StartupPending}}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 1})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: 10 * time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(10 * time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDied {
		t.Errorf("Outcome = %q, want %q (startup deadline expiry)", result.Outcome, OutcomeDied)
	}

	foundEnter := false
	for _, c := range mux.SendKeyCalls {
		if c.GUID == "strand-1" && c.Key == "Enter" {
			foundEnter = true
		}
	}
	if !foundEnter {
		t.Errorf("SendKey(strand-1, Enter) not recorded (trust dismiss), calls = %+v", mux.SendKeyCalls)
	}
}

func TestRun_Wait_Timeout_KeepsStrand(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl") // never created
	outputFile := filepath.Join(runDir, "out.md")       // never created

	mux := &fakeMux{StatusQueue: []muxengine.StatusResult{{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Second},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Second),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeTimeout {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeTimeout)
	}
	if len(mux.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v, want none (timeout keeps the strand)", mux.RemoveStrandCalls)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Errorf("run dir removed for timeout outcome: %v", err)
	}
}

// TestRun_Wait_ForkAudit_AttachedOnlyForForkModeDone proves finalize's
// AuditForks wiring: a fork-mode spec's done classification calls
// engine.AuditForks(sessionID, layout.Cwd) and attaches its result to
// Result.ForkAudit, while a non-fork spec's done classification never calls
// AuditForks at all and leaves Result.ForkAudit nil.
func TestRun_Wait_ForkAudit_AttachedOnlyForForkModeDone(t *testing.T) {
	tests := []struct {
		name          string
		forkSubagents bool
	}{
		{"fork_mode_on_attaches_audit", true},
		{"fork_mode_off_no_audit_call", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, "events.jsonl")
			outputFile := filepath.Join(runDir, "out.md")
			if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
				t.Fatalf("seed output file: %v", err)
			}
			if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			cannedAudit := ForkAudit{SpawnCalls: 1, NamedSpawns: 0}
			mux := &fakeMux{StatusQueue: []muxengine.StatusResult{{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
			engine := &fakeEngine{StartupScript: []StartupState{StartupReady}, AuditForksResult: cannedAudit}
			runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
			fc := newFakeClock(time.Now())
			run := &Run{
				runner:   runner,
				spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, ForkSubagents: tt.forkSubagents},
				runDir:   runDir,
				state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
				clock:    fc,
				deadline: fc.Now().Add(time.Minute),
			}

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Fatalf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
			}

			if tt.forkSubagents {
				if len(engine.AuditForksCalls) != 1 {
					t.Fatalf("AuditForksCalls = %v; want exactly one call", engine.AuditForksCalls)
				}
				call := engine.AuditForksCalls[0]
				if call.SessionID != "session-1" || call.Workdir != runner.layout.Cwd {
					t.Errorf("AuditForks called with (%q, %q); want (%q, %q)", call.SessionID, call.Workdir, "session-1", runner.layout.Cwd)
				}
				if result.ForkAudit == nil || !reflect.DeepEqual(*result.ForkAudit, cannedAudit) {
					t.Errorf("Result.ForkAudit = %+v; want it to carry the fake's canned audit %+v", result.ForkAudit, cannedAudit)
				}
			} else {
				if len(engine.AuditForksCalls) != 0 {
					t.Errorf("AuditForksCalls = %v; want none for a non-fork spec", engine.AuditForksCalls)
				}
				if result.ForkAudit != nil {
					t.Errorf("Result.ForkAudit = %+v; want nil for a non-fork spec", result.ForkAudit)
				}
			}
		})
	}
}

func TestRun_Wait_MultiStopOffsetTracking(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md") // never created -> asking

	fixture := "STOP:first\nSTOP:second\n"
	if err := os.WriteFile(eventsPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	mux := &fakeMux{}
	engine := &fakeEngine{}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 100, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if result.LastAssistantMessage != "second" {
		t.Errorf("LastAssistantMessage = %q, want %q (the LAST of the two Stop events)", result.LastAssistantMessage, "second")
	}
	if run.offset != int64(len(fixture)) {
		t.Errorf("offset = %d, want %d (both events consumed)", run.offset, len(fixture))
	}
}

func TestRun_Wait_ParseEventsFailure_BytesReReadOnRetry(t *testing.T) {
	// A ParseEvents error must NOT advance run.offset past the bytes it
	// failed to parse: if it did, the batch's Stop event would be discarded
	// unread once ParseEvents starts succeeding on the NEXT tick's (empty)
	// read, and the run would never classify. This proves the fix: the
	// same fixture is retried and DOES classify once the transient failure
	// clears.
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	fixture := "STOP:hello\n"
	if err := os.WriteFile(eventsPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	outputFile := filepath.Join(runDir, "out.md") // never created -> asking once classified

	mux := &fakeMux{}
	// Fail the first two ParseEvents calls; the third (retrying the SAME
	// unconsumed bytes) succeeds. maxEventsReadRetries is 3, so this must
	// stay under that budget to prove a retry recovers rather than erroring.
	engine := &fakeEngine{ParseEventsFailCount: 2}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 100, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v, want the retry to recover and classify", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if result.LastAssistantMessage != "hello" {
		t.Errorf("LastAssistantMessage = %q, want %q — the batch a failed parse left unconsumed must still be classified once parsing succeeds", result.LastAssistantMessage, "hello")
	}
	if run.offset != int64(len(fixture)) {
		t.Errorf("offset = %d, want %d (bytes consumed only after a successful parse)", run.offset, len(fixture))
	}
}

func TestRun_Wait_EventsOffsetResilience_PartialLine(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	if err := os.WriteFile(eventsPath, []byte("STOP:partial"), 0o644); err != nil { // no trailing newline yet
		t.Fatalf("seed partial events: %v", err)
	}
	outputFile := filepath.Join(runDir, "out.md") // never created -> asking once classified

	mux := &fakeMux{}
	engine := &fakeEngine{}
	runner := newWaitTestRunner(t, mux, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 100, StartupTimeoutS: 30})

	fc := newFakeClock(time.Now())
	sc := &scriptedClock{fakeClock: fc, onSleep: func() {
		// Complete the partial line between tick 1 and tick 2 so the next
		// read sees a full Stop event.
		f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("open events file to append: %v", err)
		}
		defer f.Close()
		if _, err := f.WriteString("\n"); err != nil {
			t.Fatalf("append newline: %v", err)
		}
	}}

	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    sc,
		deadline: sc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if result.LastAssistantMessage != "partial" {
		t.Errorf("LastAssistantMessage = %q, want %q", result.LastAssistantMessage, "partial")
	}
}
