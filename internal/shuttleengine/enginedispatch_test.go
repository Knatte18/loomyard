// enginedispatch_test.go covers how a Runner picks each run's engine from its EngineSet:
// Start and Open resolve the spec's engine and record it, and every later operation resolves the engine the run's record names.
// dispatchEngine records every seam method a call consults, so a call that reaches the wrong engine shows in that engine's own record.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// dispatchEngine is a fakeEngine that records every seam method consulted, per instance.
// It answers with values that differ by instance name: its own interrupt key, settle and session commands.
// It carries every optional capability a dispatch row consults.
type dispatchEngine struct {
	*fakeEngine

	name   string
	callMu sync.Mutex
	calls  []string
}

func newDispatchEngine(name string, settle time.Duration) *dispatchEngine {
	return &dispatchEngine{fakeEngine: &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd-" + name}, Settle: settle}, name: name}
}

func (e *dispatchEngine) record(method string) {
	e.callMu.Lock()
	defer e.callMu.Unlock()
	e.calls = append(e.calls, method)
}

func (e *dispatchEngine) consulted() []string {
	e.callMu.Lock()
	defer e.callMu.Unlock()
	return slices.Clone(e.calls)
}

func (e *dispatchEngine) forget() {
	e.callMu.Lock()
	defer e.callMu.Unlock()
	e.calls = nil
}

func (e *dispatchEngine) Prepare(runDir string, spec Spec, cfg Config) (Launch, error) {
	e.record("Prepare")
	return e.fakeEngine.Prepare(runDir, spec, cfg)
}

func (e *dispatchEngine) ParseEvents(data []byte) ([]Event, error) {
	e.record("ParseEvents")
	return e.fakeEngine.ParseEvents(data)
}

func (e *dispatchEngine) Startup(capture string) StartupState {
	e.record("Startup")
	return e.fakeEngine.Startup(capture)
}

func (e *dispatchEngine) InterruptSequence() []PaneInput {
	e.record("InterruptSequence")
	return []PaneInput{{Key: "Interrupt-" + e.name}}
}

func (e *dispatchEngine) RunningTurn(capture string) bool {
	e.record("RunningTurn")
	return e.fakeEngine.RunningTurn(capture)
}

func (e *dispatchEngine) InterruptSettle() time.Duration {
	e.record("InterruptSettle")
	return e.fakeEngine.InterruptSettle()
}

func (e *dispatchEngine) ColorSequence(color segmentcolor.Color) []PaneInput {
	e.record("ColorSequence")
	return []PaneInput{{Text: "/color-" + e.name + " " + string(color), Submit: true}}
}

func (e *dispatchEngine) ContextTokens(Event) ContextReading {
	e.record("ContextTokens")
	return ContextReading{Tokens: len(e.name), Known: true}
}

func (e *dispatchEngine) CompactedSince(Event, time.Time) (CompactionBoundary, bool) {
	e.record("CompactedSince")
	return CompactionBoundary{}, false
}

func (e *dispatchEngine) IdleSession(string) bool {
	e.record("IdleSession")
	return true
}

func (e *dispatchEngine) PaneTooShort(string) bool {
	e.record("PaneTooShort")
	return false
}

func (e *dispatchEngine) ClearSessionSequence() []PaneInput {
	e.record("ClearSessionSequence")
	return []PaneInput{{Text: "/clear-" + e.name, Submit: true}}
}

func (e *dispatchEngine) CompactSessionSequence(focus string) []PaneInput {
	e.record("CompactSessionSequence")
	return []PaneInput{{Text: "/compact-" + e.name + " " + focus, Submit: true}}
}

func (e *dispatchEngine) ReloadPluginsSequence() []PaneInput {
	e.record("ReloadPluginsSequence")
	return []PaneInput{{Text: "/reload-" + e.name, Submit: true}}
}

func (e *dispatchEngine) SkillLoadMessage(skills []string) string {
	e.record("SkillLoadMessage")
	return "LOAD-" + e.name + ":" + strings.Join(skills, ",")
}

func (e *dispatchEngine) ClassifySkillLoad(Event, []string) SkillLoadReport {
	e.record("ClassifySkillLoad")
	return SkillLoadReport{Verified: true}
}

func (e *dispatchEngine) DefaultSkillLoadTimeout() time.Duration {
	e.record("DefaultSkillLoadTimeout")
	return time.Second
}

func (e *dispatchEngine) ParseSessionSignals(data []byte) ([]SessionSignal, int) {
	e.record("ParseSessionSignals")
	return nil, len(data)
}

func (e *dispatchEngine) OpenLaunch(OpenSpec, Config) (Launch, error) {
	e.record("OpenLaunch")
	return Launch{Cmd: "open-" + e.name, ResumeCmd: "resume-" + e.name, SessionID: "session-" + e.name}, nil
}

var (
	_ SessionCycler       = (*dispatchEngine)(nil)
	_ SkillLoader         = (*dispatchEngine)(nil)
	_ SessionSignalParser = (*dispatchEngine)(nil)
	_ SessionOpener       = (*dispatchEngine)(nil)
	_ InterruptGuard      = (*dispatchEngine)(nil)
)

// dispatchRig is a Runner over a set holding dispatchEngines "a" (the default) and "b", and a plain engine "p" without optional capabilities.
type dispatchRig struct {
	t       *testing.T
	fx      *fixture
	reed    *fakeReed
	a, b    *dispatchEngine
	engines EngineSet
}

func newDispatchRig(t *testing.T) *dispatchRig {
	t.Helper()
	a, b := newDispatchEngine("a", 7*time.Millisecond), newDispatchEngine("b", 9*time.Millisecond)
	engines := NewEngineSet("a", map[string]Engine{"a": a, "b": b, "p": &fakeEngine{}})
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}, CaptureQueue: []string{"IDLE"}}
	readyStart(reed, a.fakeEngine)
	readyStart(reed, b.fakeEngine)
	fx := newFixture(t, reed, a, withEngines(engines), withConfig(fastConfig), withSeparateRunDir())
	return &dispatchRig{t: t, fx: fx, reed: reed, a: a, b: b, engines: engines}
}

// start starts a run on the named engine and forgets what the start consulted.
func (r *dispatchRig) start(engine string) *Run {
	r.t.Helper()
	run, err := r.fx.Runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}, Engine: engine})
	if err != nil {
		r.t.Fatalf("Start(engine %q) error: %v", engine, err)
	}
	r.a.forget()
	r.b.forget()
	return run
}

// record returns the run record the Runner holds for strand-1.
func (r *dispatchRig) record() (RunState, string) {
	r.t.Helper()
	state, runDir, err := FindRun(r.fx.Runner.cfg, r.fx.Anchor, "strand-1")
	if err != nil {
		r.t.Fatalf("FindRun(strand-1): %v", err)
	}
	return state, runDir
}

// writeEvents appends one STOP line to the run's events file, so a read of the events has data to parse.
func (r *dispatchRig) writeEvents(state RunState) {
	r.t.Helper()
	if err := os.WriteFile(state.EventsPath, []byte("STOP:done\n"), 0o644); err != nil {
		r.t.Fatalf("write events: %v", err)
	}
}

// dispatchOp is one engine-consulting operation on a run of strand-1.
// wantMethod is the seam method the run's engine must record;
// wantOutcome is "ok" for a call that must succeed, "refused" for one that must fail, and empty when only the consult is pinned.
type dispatchOp struct {
	name        string
	call        func(r *dispatchRig) error
	wantMethod  string
	wantOutcome string
}

func dispatchOps() []dispatchOp {
	return []dispatchOp{
		{"Runner.Send", func(r *dispatchRig) error { return r.fx.Runner.Send("strand-1", "hello") }, "IdleSession", ""},
		{"Runner.SendConditioned", func(r *dispatchRig) error {
			state, _ := r.record()
			r.writeEvents(state)
			_, err := r.fx.Runner.SendConditioned("strand-1", "hello", SendCondition{State: SessionIdleStalled})
			return err
		}, "ParseSessionSignals", ""},
		{"Runner.Interrupt", func(r *dispatchRig) error {
			r.reed.CaptureQueue = []string{"RUNNING"}
			return r.fx.Runner.Interrupt("strand-1")
		}, "InterruptSequence", ""},
		{"Runner.Inject_with_the_other_engines_interrupt_key", func(r *dispatchRig) error {
			return r.fx.Runner.Inject("strand-1", []PaneInput{{Key: "Interrupt-a"}})
		}, "InterruptSequence", "ok"},
		{"Runner.Inject_with_its_own_interrupt_key", func(r *dispatchRig) error {
			return r.fx.Runner.Inject("strand-1", []PaneInput{{Key: "Interrupt-b"}})
		}, "InterruptSequence", "refused"},
		{"Runner.PaneRunningTurn", func(r *dispatchRig) error {
			_, err := r.fx.Runner.PaneRunningTurn("strand-1")
			return err
		}, "RunningTurn", "ok"},
		{"Runner.ReadEvents", func(r *dispatchRig) error {
			state, _ := r.record()
			r.writeEvents(state)
			_, _, err := r.fx.Runner.ReadEvents("strand-1", 0)
			return err
		}, "ParseEvents", "ok"},
		{"Runner.SessionState", func(r *dispatchRig) error {
			state, _ := r.record()
			r.writeEvents(state)
			_, err := r.fx.Runner.SessionState("strand-1")
			return err
		}, "ParseSessionSignals", "ok"},
		{"Runner.SessionIdle", func(r *dispatchRig) error {
			_, err := r.fx.Runner.SessionIdle("strand-1")
			return err
		}, "IdleSession", "ok"},
		{"Runner.LoadSkills", func(r *dispatchRig) error { return r.fx.Runner.LoadSkills("strand-1", []string{"s"}) }, "SkillLoadMessage", ""},
		{"Runner.TypeColor", func(r *dispatchRig) error { return r.fx.Runner.TypeColor("strand-1", segmentcolor.Green) }, "ColorSequence", ""},
		{"Runner.ClearSession", func(r *dispatchRig) error { return r.fx.Runner.ClearSession("strand-1") }, "ClearSessionSequence", ""},
	}
}

// requireConsulted fails unless used recorded method and other recorded nothing.
func requireConsulted(t *testing.T, used, other *dispatchEngine, method string) {
	t.Helper()
	if !slices.Contains(used.consulted(), method) {
		t.Errorf("engine %q consulted %v; want %q among them", used.name, used.consulted(), method)
	}
	if got := other.consulted(); len(got) != 0 {
		t.Errorf("engine %q consulted %v; want nothing", other.name, got)
	}
}

// TestRunner_EngineSetDispatch pins which engine of the set each operation consults.
func TestRunner_EngineSetDispatch(t *testing.T) {
	t.Parallel()

	t.Run("Start_prepares_through_the_named_engine_and_records_its_name", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			spec     string
			wantName string
		}{
			{"a", "a"},
			{"b", "b"},
			{"", "a"},
		}
		for _, tt := range tests {
			rig := newDispatchRig(t)
			used, other := rig.a, rig.b
			if tt.wantName == "b" {
				used, other = rig.b, rig.a
			}
			run, err := rig.fx.Runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}, Engine: tt.spec})
			if err != nil {
				t.Fatalf("Start(engine %q) error: %v", tt.spec, err)
			}
			requireConsulted(t, used, other, "Prepare")
			if state, _ := rig.record(); state.Engine != tt.wantName {
				t.Errorf("Start(engine %q) recorded engine %q; want %q", tt.spec, state.Engine, tt.wantName)
			}
			if run.engineName != tt.wantName {
				t.Errorf("Start(engine %q) run engine name = %q; want %q", tt.spec, run.engineName, tt.wantName)
			}
		}
	})

	t.Run("Open_resolves_the_named_engine_and_refuses_one_without_a_session_opener", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			spec    string
			want    string
			wantErr bool
		}{
			{"named", "b", "b", false},
			{"default", "", "a", false},
			{"without_the_capability", "p", "", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				rig := newDispatchRig(t)
				result, err := rig.fx.Runner.Open(OpenSpec{Role: "explore", Engine: tt.spec})
				if tt.wantErr {
					if err == nil {
						t.Fatalf("Open(engine %q) = nil error; want a refusal", tt.spec)
					}
					if len(rig.reed.AddStrandCalls) != 0 {
						t.Errorf("AddStrand calls = %d; want none for a refused open", len(rig.reed.AddStrandCalls))
					}
					return
				}
				if err != nil {
					t.Fatalf("Open(engine %q) error: %v", tt.spec, err)
				}
				if result.Engine != tt.want {
					t.Errorf("Open(engine %q).Engine = %q; want %q", tt.spec, result.Engine, tt.want)
				}
				used, other := rig.a, rig.b
				if tt.want == "b" {
					used, other = rig.b, rig.a
				}
				requireConsulted(t, used, other, "OpenLaunch")
			})
		}
	})

	t.Run("an_unregistered_engine_is_refused_before_anything_is_created", func(t *testing.T) {
		t.Parallel()
		rig := newDispatchRig(t)
		_, startErr := rig.fx.Runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}, Engine: "c"})
		_, openErr := rig.fx.Runner.Open(OpenSpec{Role: "explore", Engine: "c"})
		for name, err := range map[string]error{"Start": startErr, "Open": openErr} {
			if !errors.Is(err, ErrEngineNotRegistered) || !strings.Contains(err.Error(), "a, b, p") {
				t.Errorf("%s(engine c) error = %v; want ErrEngineNotRegistered naming the registered engines", name, err)
			}
		}
		if len(rig.reed.AddStrandCalls) != 0 {
			t.Errorf("AddStrand calls = %d; want none", len(rig.reed.AddStrandCalls))
		}
		if entries, err := os.ReadDir(rig.fx.RunRoot); err != nil || len(entries) != 0 {
			t.Errorf("run root entries = %v, err = %v; want it empty", entries, err)
		}
	})

	t.Run("a_run_started_on_b_is_operated_through_b", func(t *testing.T) {
		t.Parallel()
		for _, op := range dispatchOps() {
			t.Run(op.name, func(t *testing.T) {
				t.Parallel()
				rig := newDispatchRig(t)
				rig.start("b")
				err := op.call(rig)
				switch op.wantOutcome {
				case "ok":
					if err != nil {
						t.Errorf("%s error = %v; want nil", op.name, err)
					}
				case "refused":
					if err == nil {
						t.Errorf("%s = nil error; want a refusal", op.name)
					}
				}
				requireConsulted(t, rig.b, rig.a, op.wantMethod)
			})
		}

		t.Run("Run.DeliverPendingPrompt", func(t *testing.T) {
			t.Parallel()
			rig := newDispatchRig(t)
			run := rig.start("b")
			state, runDir := rig.record()
			state.PromptLine = "pending"
			if err := saveRunState(runDir, state); err != nil {
				t.Fatalf("saveRunState: %v", err)
			}
			_, _ = run.DeliverPendingPrompt()
			if got := rig.b.consulted(); len(got) == 0 {
				t.Errorf("engine b consulted nothing; want the pending-prompt delivery to read through it")
			}
			if got := rig.a.consulted(); len(got) != 0 {
				t.Errorf("engine a consulted %v; want nothing", got)
			}
		})

		t.Run("Runner.Attach", func(t *testing.T) {
			t.Parallel()
			rig := newDispatchRig(t)
			rig.start("b")
			state, _ := rig.record()
			rig.writeEvents(state)
			seedPresentReedState(t, rig.fx.DotLyx)
			out := filepath.Join(rig.fx.Worktree, "out.md")
			touchOutputFile(t, out)

			result, found, err := rig.fx.Runner.Attach(Spec{OutputFiles: []string{out}, Timeout: time.Minute})
			if err != nil || !found {
				t.Fatalf("Attach() = %+v, %v, %v; want the live run found", result, found, err)
			}
			requireConsulted(t, rig.b, rig.a, "ParseEvents")
		})
	})

	t.Run("an_attach_by_a_spec_naming_a_different_engine_waits_through_the_record_and_warns", func(t *testing.T) {
		t.Parallel()
		logs := logcapture.Capture(t)
		rig := newDispatchRig(t)
		rig.start("b")
		state, _ := rig.record()
		rig.writeEvents(state)
		seedPresentReedState(t, rig.fx.DotLyx)
		out := filepath.Join(rig.fx.Worktree, "out.md")
		touchOutputFile(t, out)
		fresh := NewRunner(rig.reed, rig.engines, rig.fx.Anchor, rig.fx.Worktree, rig.fx.Runner.cfg)
		fresh.SetClock(newFakeClock(time.Now()))

		if _, found, err := fresh.Attach(Spec{OutputFiles: []string{out}, Timeout: time.Minute, Engine: "a"}); err != nil || !found {
			t.Fatalf("Attach() found = %v, error = %v; want the live run found", found, err)
		}
		requireConsulted(t, rig.b, rig.a, "ParseEvents")
		if got := countLevelLines(logs.String(), "WARN", "the spec names a different engine"); got != 1 {
			t.Errorf("WARN lines naming the engine mismatch = %d; want 1; log: %s", got, logs.String())
		}
		for _, name := range []string{"specEngine=a", "recordEngine=b"} {
			if !strings.Contains(logs.String(), name) {
				t.Errorf("log lacks %q; want the Warn to name both engines; log: %s", name, logs.String())
			}
		}
	})

	t.Run("SessionIdle_on_a_live_strand_with_no_record_consults_the_default", func(t *testing.T) {
		t.Parallel()
		rig := newDispatchRig(t)
		rig.reed.StatusQueue = liveStrandStatus(true)
		if _, err := rig.fx.Runner.SessionIdle("strand-1"); err != nil {
			t.Fatalf("SessionIdle() error: %v", err)
		}
		requireConsulted(t, rig.a, rig.b, "IdleSession")
	})

	t.Run("a_record_with_no_engine_key_is_driven_through_the_default", func(t *testing.T) {
		t.Parallel()
		rig := newDispatchRig(t)
		recordless := newFixture(t, rig.reed, rig.a, withEngines(rig.engines), withStrand("strand-1"))
		if err := os.WriteFile(recordless.EventsPath, []byte("STOP:done\n"), 0o644); err != nil {
			t.Fatalf("write events: %v", err)
		}
		if _, _, err := recordless.Runner.ReadEvents("strand-1", 0); err != nil {
			t.Fatalf("ReadEvents() error: %v", err)
		}
		requireConsulted(t, rig.a, rig.b, "ParseEvents")
	})

	t.Run("a_record_naming_an_unregistered_engine_refuses_every_operation_and_never_blocks_a_stop", func(t *testing.T) {
		t.Parallel()
		tie := StepTie{StepsDir: "/steps", TraceID: "trace-c"}
		seed := func(t *testing.T) *dispatchRig {
			t.Helper()
			rig := newDispatchRig(t)
			rig.reed.StatusQueue = liveStrandStatus(true)
			runDir := filepath.Join(rig.fx.RunRoot, "run-1")
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				t.Fatalf("mkdir run dir: %v", err)
			}
			state := RunState{
				RunID:       "run-1",
				StrandGUID:  "strand-1",
				StrandName:  "name-c",
				Engine:      "c",
				Outcome:     runOutcomeRunning,
				Step:        &tie,
				OutputFiles: []string{filepath.Join(rig.fx.Worktree, "out.md")},
				EventsPath:  filepath.Join(runDir, eventsFileName),
			}
			if err := saveRunState(runDir, state); err != nil {
				t.Fatalf("saveRunState: %v", err)
			}
			if err := os.WriteFile(state.EventsPath, []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("write events: %v", err)
			}
			return rig
		}

		for _, op := range dispatchOps() {
			t.Run(op.name, func(t *testing.T) {
				t.Parallel()
				rig := seed(t)
				if err := op.call(rig); !errors.Is(err, ErrEngineNotRegistered) {
					t.Errorf("%s error = %v; want ErrEngineNotRegistered", op.name, err)
				}
				if len(rig.reed.SendKeyCalls) != 0 || len(rig.reed.SendTextCalls) != 0 || len(rig.reed.PasteSubmitCalls) != 0 {
					t.Errorf("%s reached the pane: keys %+v, texts %+v, pastes %+v", op.name, rig.reed.SendKeyCalls, rig.reed.SendTextCalls, rig.reed.PasteSubmitCalls)
				}
			})
		}

		t.Run("Runner.Attach", func(t *testing.T) {
			t.Parallel()
			rig := seed(t)
			seedPresentReedState(t, rig.fx.DotLyx)
			if _, _, err := rig.fx.Runner.Attach(Spec{OutputFiles: []string{filepath.Join(rig.fx.Worktree, "out.md")}, Timeout: time.Minute}); !errors.Is(err, ErrEngineNotRegistered) {
				t.Errorf("Attach() error = %v; want ErrEngineNotRegistered", err)
			}
		})

		stops := []struct {
			name string
			stop func(r *Runner) error
		}{
			{"StopStrand", func(r *Runner) error { return r.StopStrand("strand-1") }},
			{"StopStepRuns", func(r *Runner) error {
				report, err := r.StopStepRuns(tie)
				if err == nil && len(report.Stopped) != 1 {
					return errors.New("the step stop did not stop the run")
				}
				return err
			}},
		}
		for _, tt := range stops {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				rig := seed(t)
				if err := tt.stop(rig.fx.Runner); err != nil {
					t.Fatalf("%s error = %v; want nil, since a stop resolves no engine", tt.name, err)
				}
				if len(rig.reed.RemoveStrandCalls) != 1 || rig.reed.RemoveStrandCalls[0].GUID != "strand-1" {
					t.Errorf("RemoveStrand calls = %v; want the removal of strand-1", rig.reed.RemoveStrandCalls)
				}
			})
		}
	})
}
