// run_test.go covers Runner.Start: the happy path's exact AddSpec wiring (including
// SessionID/Display passthrough), validation short-circuiting before any reed call, run-dir cleanup
// on an AddStrand failure, and the opportunistic orphan sweep never blocking Start on its own
// failure.

package shuttleengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestNewRunner_RefusesUnusableToldPaths pins the told-pair guard.
// anchorPath and worktreeRoot are adjacent parameters of the same type with four semantically
// distinct consumers, so a swap compiles cleanly and, in a subpath-anchored worktree, silently
// relocates the run-dir root, reed's state lookup, and the fork audit's transcript directory into
// the worktree root instead of the anchor. An empty or relative value fails the same way — it
// succeeds against whatever working directory the process happens to have.
// Every public entry point must refuse, not just Start: Interrupt/Send/Inject all resolve their run
// through the same anchorPath.
//
// The rows with no wantIn pin the other side: every pair hubgeom.ReedGeometry can produce must
// pass, including the anchor-at-worktree-root case (AnchorRel "."), where the two values are
// legitimately equal and a swap is a no-op.
func TestNewRunner_RefusesUnusableToldPaths(t *testing.T) {
	worktreeRoot := t.TempDir()
	anchorPath := filepath.Join(worktreeRoot, "sub", "dir")
	if err := os.MkdirAll(anchorPath, 0o755); err != nil {
		t.Fatalf("mkdir anchor path: %v", err)
	}

	tests := []struct {
		name         string
		anchorPath   string
		worktreeRoot string
		wantIn       string
	}{
		{"swapped_pair", worktreeRoot, anchorPath, "most likely swapped"},
		{"empty_anchor", "", worktreeRoot, "empty path"},
		{"empty_worktree_root", anchorPath, "", "empty path"},
		{"relative_anchor", filepath.Join("sub", "dir"), worktreeRoot, "relative path"},
		{"anchor_in_a_sibling_tree", t.TempDir(), worktreeRoot, "outside its worktree root"},
		// NewRunner must still refuse the standalone shape (anchor and worktree root disjoint) even
		// though NewDetachedRunner accepts it, so the two constructors are provably not
		// interchangeable.
		{"standalone_pair_disjoint_dirs", t.TempDir(), t.TempDir(), "outside its worktree root"},
		{"anchored_at_worktree_root", worktreeRoot, worktreeRoot, ""},
		{"subpath_anchored", anchorPath, worktreeRoot, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewRunner(&fakeReed{}, singleEngineSet(&fakeEngine{}), tt.anchorPath, tt.worktreeRoot, Config{RunTimeoutMin: 5})

			if tt.wantIn == "" {
				if runner.toldErr != nil {
					t.Errorf("NewRunner(%q, %q).toldErr = %v; want nil", tt.anchorPath, tt.worktreeRoot, runner.toldErr)
				}
				return
			}
			if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Start() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Interrupt("strand-1"); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Interrupt() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Send("strand-1", "hi"); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Send() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Inject("strand-1", []PaneInput{{Key: "C-u"}}); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Inject() error = %v; want it to name %q", err, tt.wantIn)
			}
		})
	}
}

// TestNewDetachedRunner_RefusesUnusableToldPaths pins validateDetachedToldPaths' guard, mirroring
// TestNewRunner_RefusesUnusableToldPaths' shape but driven by strict disjointness rather than
// containment: anchorPath and worktreeRoot must be two distinct, non-overlapping directories, and
// paneCwd is checked only for non-empty and absolute. Every public entry point is driven, not just
// Start, since Interrupt/Send/Inject all resolve their run through the same anchorPath.
func TestNewDetachedRunner_RefusesUnusableToldPaths(t *testing.T) {
	anchor := t.TempDir()
	worktree := t.TempDir()
	subpathOfWorktree := filepath.Join(worktree, "sub", "dir")
	if err := os.MkdirAll(subpathOfWorktree, 0o755); err != nil {
		t.Fatalf("mkdir subpath of worktree: %v", err)
	}
	subpathOfAnchor := filepath.Join(anchor, "sub", "dir")
	if err := os.MkdirAll(subpathOfAnchor, 0o755); err != nil {
		t.Fatalf("mkdir subpath of anchor: %v", err)
	}

	tests := []struct {
		name         string
		anchorPath   string
		worktreeRoot string
		paneCwd      string
		wantIn       string
	}{
		{"empty_anchor", "", worktree, anchor, "empty path"},
		{"empty_worktree_root", anchor, "", anchor, "empty path"},
		{"empty_pane_cwd", anchor, worktree, "", "empty path"},
		{"relative_anchor", filepath.Base(anchor), worktree, anchor, "relative path"},
		{"relative_worktree_root", anchor, filepath.Base(worktree), anchor, "relative path"},
		{"relative_pane_cwd", anchor, worktree, filepath.Base(anchor), "relative path"},
		{"equal_pair", anchor, anchor, anchor, "equal to its worktree root"},
		{"anchor_strictly_inside_worktree", subpathOfWorktree, worktree, subpathOfWorktree, "inside its worktree root"},
		{"worktree_strictly_inside_anchor", anchor, subpathOfAnchor, anchor, "inside its anchor path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewDetachedRunner(&fakeReed{}, singleEngineSet(&fakeEngine{}), tt.anchorPath, tt.worktreeRoot, tt.paneCwd, Config{RunTimeoutMin: 5})

			if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Start() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Interrupt("strand-1"); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Interrupt() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Send("strand-1", "hi"); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Send() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Inject("strand-1", []PaneInput{{Key: "C-u"}}); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Inject() error = %v; want it to name %q", err, tt.wantIn)
			}
		})
	}
}

// TestNewDetachedRunner_AcceptsStandaloneShapeAndBothPaneCwdPositions pins the accepted side of
// validateDetachedToldPaths: a standalone pair of two disjoint absolute directories passes, and
// paneCwd is accepted whether it equals anchorPath or worktreeRoot — exactly the hub and standalone
// shapes, neither of which validateDetachedToldPaths refuses since it makes no relational assertion
// about paneCwd at all.
// The verdict is asserted through a public entry point (Start's validation short-circuit never
// firing on the told-path check) rather than by reading the toldErr field directly, since toldErr is
// unexported implementation, not the contract callers depend on.
func TestNewDetachedRunner_AcceptsStandaloneShapeAndBothPaneCwdPositions(t *testing.T) {
	anchor := t.TempDir()
	worktree := t.TempDir()

	tests := []struct {
		name    string
		paneCwd string
	}{
		{"paneCwd_equals_anchor", anchor},
		{"paneCwd_equals_worktree_root", worktree},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
			engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd"}}
			runner := NewDetachedRunner(reed, singleEngineSet(engine), anchor, worktree, tt.paneCwd, Config{RunTimeoutMin: 5})
			readyStart(reed, engine)

			if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err != nil {
				t.Errorf("Start() error = %v; want the told-path verdict clean for a disjoint standalone pair with paneCwd %q", err, tt.paneCwd)
			}
		})
	}
}

// TestRunner_StartGated_CarriesNoDoneWhen pins that a gated run's strand carries no done-when list, because its output files existing does not mean its gate passed.
func TestRunner_StartGated_CarriesNoDoneWhen(t *testing.T) {
	t.Parallel()

	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "launch-cmd"}}
	fx := newFixture(t, reed, engine, withConfig(fastConfig))
	readyStart(reed, engine)

	gate := GateSpec{{Name: "check", Gate: func() (GateResult, error) { return GateResult{Passed: true}, nil }, Attempts: 1}}
	if _, err := fx.Runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}, gate); err != nil {
		t.Fatalf("StartGated() error: %v", err)
	}
	if len(reed.AddStrandCalls) != 1 {
		t.Fatalf("AddStrand calls = %d, want 1", len(reed.AddStrandCalls))
	}
	if got := reed.AddStrandCalls[0].DoneWhen; len(got) != 0 {
		t.Errorf("gated AddStrand DoneWhen = %v, want none", got)
	}
}

// TestRunner_Start_HappyPath covers one successful Start whose readyStart scripting resolves
// StartupReady on the very first tick.
// The steps share that run and read what it left behind.
//
//testtiming:keep pins Start's exact AddSpec wiring, its single ready probe, the persisted Started, running Outcome and fork authorization, and RunDir naming the created directory
func TestRunner_Start_HappyPath(t *testing.T) {
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "launch-cmd", ResumeCmd: "resume-cmd", SessionID: "session-1"}}
	fx := newFixture(t, reed, engine, withConfig(fastConfig))
	runner, anchorPath := fx.Runner, fx.Anchor
	readyStart(reed, engine)

	spec := Spec{
		Prompt:      "do the thing",
		OutputFiles: []string{"out.md"},
		Role:        "reviewer",
		Round:       "1",
		Segment:     segmentcolor.Review,
		Parent:      "parent-guid",
		Display:     render.Display{Anchor: render.AnchorBelowParent},

		ForkSubagents: true,
	}

	run, err := runner.Start(spec)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if run == nil {
		t.Fatal("Start() returned nil run")
	}

	t.Run("wires AddSpec verbatim", func(t *testing.T) {
		if len(reed.AddStrandCalls) != 1 {
			t.Fatalf("AddStrand calls = %d, want 1", len(reed.AddStrandCalls))
		}
		got := reed.AddStrandCalls[0]
		want := reedengine.AddSpec{
			Role:      "reviewer",
			Segment:   "review",
			Parent:    "parent-guid",
			Cmd:       "launch-cmd",
			ResumeCmd: "resume-cmd",
			SessionID: "session-1",
			Display:   render.Display{Anchor: render.AnchorBelowParent},
			DoneWhen:  []string{filepath.Join(fx.Worktree, "out.md")},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("AddStrand spec = %+v, want %+v", got, want)
		}

		if run.state.SessionID != "session-1" {
			t.Errorf("state.SessionID = %q, want %q", run.state.SessionID, "session-1")
		}
		if run.state.StrandGUID != "strand-1" {
			t.Errorf("state.StrandGUID = %q, want %q", run.state.StrandGUID, "strand-1")
		}
	})

	// A ready start issues exactly one Status/CapturePane probe before returning, and persists
	// Started: true: the startup step does not linger past the first successful probe.
	t.Run("probes exactly once and persists Started", func(t *testing.T) {
		statusCalls, captureCalls := 0, 0
		for _, c := range reed.CallLog {
			switch c {
			case "Status":
				statusCalls++
			case "CapturePane":
				captureCalls++
			}
		}
		if statusCalls != 1 {
			t.Errorf("Status call count = %d; want 1", statusCalls)
		}
		if captureCalls != 1 {
			t.Errorf("CapturePane call count = %d; want 1", captureCalls)
		}

		rs, found, rerr := loadRunState(run.RunDir())
		if rerr != nil || !found {
			t.Fatalf("loadRunState: found=%v err=%v", found, rerr)
		}
		if !rs.Started {
			t.Errorf("loadRunState().Started = false; want true")
		}
	})

	// A freshly started run's persisted run.json carries the runOutcomeRunning sentinel, before any
	// classification has happened: the fact on disk that a later Attach relies on to tell a live
	// run from an ended one.
	t.Run("persists the running outcome", func(t *testing.T) {
		rs, found, err := loadRunState(run.runDir)
		if err != nil {
			t.Fatalf("loadRunState: %v", err)
		}
		if !found {
			t.Fatal("loadRunState: run.json not found")
		}
		if rs.Outcome != runOutcomeRunning {
			t.Errorf("RunState.Outcome = %q, want %q", rs.Outcome, runOutcomeRunning)
		}
		if run.state.Outcome != runOutcomeRunning {
			t.Errorf("run.state.Outcome = %q, want %q", run.state.Outcome, runOutcomeRunning)
		}
		if rs.PID != os.Getpid() {
			t.Errorf("RunState.PID = %d, want the starting process's pid %d", rs.PID, os.Getpid())
		}
		if !rs.ForkSubagents {
			t.Error("RunState.ForkSubagents = false, want the spawning Spec's authorization recorded")
		}
	})

	// A runner told no step tie writes no step key, and one told a tie records it.
	t.Run("persists the told step tie only", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(run.RunDir(), runStateFileName))
		if err != nil {
			t.Fatalf("read run.json: %v", err)
		}
		if strings.Contains(string(data), `"step"`) {
			t.Errorf("run.json of a runner told no step carries a step key: %s", data)
		}

		tiedReed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-2"}}
		tiedEngine := &fakeEngine{PrepareLaunch: Launch{Cmd: "launch-cmd", SessionID: "session-2"}}
		tiedFixture := newFixture(t, tiedReed, tiedEngine, withConfig(fastConfig))
		readyStart(tiedReed, tiedEngine)
		tie := StepTie{StepsDir: filepath.Join(tiedFixture.Worktree, "steps"), TraceID: "trace-1"}
		tiedFixture.Runner.SetStepTie(tie)
		tiedRun, err := tiedFixture.Runner.Start(Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}, Role: "reviewer"})
		if err != nil {
			t.Fatalf("Start() with a step tie error: %v", err)
		}
		record, found, err := loadRunState(tiedRun.RunDir())
		if err != nil || !found {
			t.Fatalf("loadRunState: found=%v err=%v", found, err)
		}
		if record.Step == nil || *record.Step != tie {
			t.Errorf("persisted step = %+v, want %+v", record.Step, tie)
		}
	})

	// RunDir() names the one directory holding prompt.md, settings.json and the events file, for a
	// caller that starts a run and never Waits on it.
	t.Run("RunDir names the directory Start created", func(t *testing.T) {
		root := runDirRoot(runner.cfg, anchorPath)
		dir := run.RunDir()
		if dir == "" {
			t.Fatal("RunDir() = \"\", want the directory Start created")
		}
		if rel, err := filepath.Rel(root, dir); err != nil || strings.HasPrefix(rel, "..") {
			t.Errorf("RunDir() = %q, want a directory under the run-directory root %q", dir, root)
		}
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("RunDir() = %q, want it to name a directory Start actually created: %v", dir, err)
		}
	})
}

// TestRunner_Start_WaitsWhileAPauseCoversItsPair pins that a start registers no strand while a pause covers the runner's pair,
// and that the run it then returns counts its deadline from the resume, not from the call.
func TestRunner_Start_WaitsWhileAPauseCoversItsPair(t *testing.T) {
	t.Parallel()

	const timeout = 20 * time.Minute
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "launch-cmd", SessionID: "session-1"}}
	fx := newFixture(t, reed, engine, withConfig(fastConfig))
	readyStart(reed, engine)
	fc := newFakeClock(time.Now())
	geometry, resume := coverHub(t, fc.Now())
	fx.Runner.SetPause(geometry)

	var addedWhileHeld int
	var resumedAt time.Time
	fx.Runner.SetClock(&liftClock{Clock: fc, liftAfter: 3, lift: func() {
		addedWhileHeld = len(reed.AddStrandCalls)
		resumedAt = fc.Now()
		resume(resumedAt)
	}})

	run, err := fx.Runner.Start(Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}, Role: "reviewer", Timeout: timeout})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if addedWhileHeld != 0 {
		t.Errorf("AddStrand calls before the resume = %d, want none", addedWhileHeld)
	}
	if len(reed.AddStrandCalls) != 1 {
		t.Errorf("AddStrand calls after the resume = %d, want 1", len(reed.AddStrandCalls))
	}
	if want := resumedAt.Add(timeout); !run.deadline.Equal(want) {
		t.Errorf("run deadline = %v, want %v, the timeout past the resume", run.deadline, want)
	}
}

// TestRun_DeliverPendingPrompt_SendsOnlyAPromptWhoseTurnNeverStarted pins that a run reconstructed from its record types the deferred prompt line only when no turn started at or past the prompt offset.
func TestRun_DeliverPendingPrompt_SendsOnlyAPromptWhoseTurnNeverStarted(t *testing.T) {
	t.Parallel()

	const promptLine = "Read the task prompt and carry it out."
	const loadTurns = "START\nSTOP:loaded\n"
	tests := []struct {
		name         string
		promptLine   string
		events       string
		promptOffset int64
		wantSent     bool
		// wantOffset is the prompt offset the record holds afterwards.
		wantOffset int64
	}{
		{name: "a record with no prompt line sends nothing", events: loadTurns, promptOffset: int64(len(loadTurns)), wantOffset: int64(len(loadTurns))},
		{
			name: "a turn start past the prompt offset means the prompt was taken", promptLine: promptLine, events: loadTurns + "START\n",
			promptOffset: int64(len(loadTurns)), wantOffset: int64(len(loadTurns)),
		},
		{
			name: "only the load turns before the prompt offset sends the prompt once", promptLine: promptLine, events: loadTurns,
			promptOffset: int64(len(loadTurns)), wantSent: true, wantOffset: int64(len(loadTurns)),
		},
		{
			name: "a zero prompt offset is persisted as the events size before sending", promptLine: promptLine, events: "SESSIONSTART:startup\n",
			wantSent: true, wantOffset: int64(len("SESSIONSTART:startup\n")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reed := &echoReed{fakeReed: &fakeReed{StatusQueue: liveStrandStatus(true)}, pane: "❯ "}
			engine := &sessionFakeEngine{waitingEngine: waitingEngine{fakeEngine: fakeEngine{StartupScript: []StartupState{StartupReady}}}}
			fx := newFixture(t, reed, engine, withConfig(gateConfig))
			record := RunState{StrandGUID: "strand-1", PromptLine: tt.promptLine, PromptOffset: tt.promptOffset}
			run := fx.newRun(Spec{}, withRunState(record), withRunEvents(tt.events))
			record.EventsPath = run.state.EventsPath
			if err := saveRunState(run.runDir, record); err != nil {
				t.Fatalf("saveRunState: %v", err)
			}

			sent, err := run.DeliverPendingPrompt()
			if err != nil {
				t.Fatalf("DeliverPendingPrompt() error = %v", err)
			}
			if sent != tt.wantSent {
				t.Errorf("DeliverPendingPrompt() = %v, want %v", sent, tt.wantSent)
			}
			wantTyped := []string(nil)
			if tt.wantSent {
				wantTyped = []string{promptLine}
			}
			if !reflect.DeepEqual(reed.typed, wantTyped) {
				t.Errorf("typed = %q, want %q", reed.typed, wantTyped)
			}
			persisted, _, err := loadRunState(run.runDir)
			if err != nil {
				t.Fatalf("loadRunState: %v", err)
			}
			if persisted.PromptOffset != tt.wantOffset {
				t.Errorf("persisted PromptOffset = %d, want %d", persisted.PromptOffset, tt.wantOffset)
			}
		})
	}
}

// TestRunner_Start_TypesTheSegmentColorAfterStartup pins the color step.
// A colored strand gets the engine's color inputs after startup and after the skill load's first turn, which the launch line carries, and before the retry turn and the prompt.
// No color, a caller that colors itself and an engine without a color command type nothing, and a failing color play still starts the run.
func TestRunner_Start_TypesTheSegmentColorAfterStartup(t *testing.T) {
	loaded := func(skills ...string) SkillLoadReport { return SkillLoadReport{Verified: true, Loaded: skills} }
	tests := []struct {
		name string
		// skills and reports default to the skill a and its loaded report.
		skills   []string
		reports  []SkillLoadReport
		color    segmentcolor.Color
		byCaller bool
		noInputs bool
		colorErr error
		// hookState starts the run over an engine reading hook state, whose events file holds only a SessionStart(startup) line beside a busy-looking pane.
		hookState bool
		// lateEnds maps a load turn to the number of Status calls before its turn end is appended.
		lateEnds  map[int]int
		wantTyped string
	}{
		{name: "ColoredStrandIsColoredAfterTheSkillLoad", color: segmentcolor.Orange, wantTyped: "COLOR:orange|do the task"},
		{name: "NoColorTypesNone", wantTyped: "do the task"},
		{name: "ColorByCallerTypesNone", color: segmentcolor.Orange, byCaller: true, wantTyped: "do the task"},
		{name: "EngineWithoutAColorCommandTypesNone", color: segmentcolor.Orange, noInputs: true, wantTyped: "do the task"},
		{name: "FailingColorPlayStillStartsTheRun", color: segmentcolor.Orange, colorErr: errors.New("boom"), wantTyped: "COLOR:orange|do the task"},
		{name: "TheHookStateIdleAfterTheLoadTurnColorsAPaneThePaneFallbackRefuses", color: segmentcolor.Orange, hookState: true, wantTyped: "COLOR:orange|do the task"},
		{name: "TheColorWaitsForALateFirstTurnEnd", color: segmentcolor.Orange, lateEnds: map[int]int{0: 4}, wantTyped: "COLOR:orange|do the task"},
		{
			name:      "TheColorPrecedesTheRetryTurn",
			skills:    []string{"a", "b"},
			reports:   []SkillLoadReport{{Verified: true, Loaded: []string{"a"}, Missing: []string{"b"}}, loaded("b")},
			color:     segmentcolor.Orange,
			wantTyped: "COLOR:orange|LOAD:b|do the task",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skills := tt.skills
			if skills == nil {
				skills = []string{"a"}
			}
			reports := tt.reports
			if reports == nil {
				reports = []SkillLoadReport{loaded("a")}
			}
			spec := Spec{Skills: skills, ColorByCaller: tt.byCaller}
			strand := reedengine.Strand{GUID: "strand-1", Color: tt.color}

			run, reed, _, err := skillStartFixtureWith(t, strand, spec, reports, nil, nil, func(reed *skillReed, engine *skillFakeEngine) {
				reed.ColorErr = tt.colorErr
				reed.LateEnds = tt.lateEnds
				engine.NoColorInputs = tt.noInputs
				engine.HookState = tt.hookState
				if tt.hookState {
					reed.Screen = "working (esc to interrupt)"
				}
			})

			if err != nil || run == nil {
				t.Fatalf("Start() = (%v, %v), want a started run", run, err)
			}
			if got := strings.Join(typedTexts(reed), "|"); got != tt.wantTyped {
				t.Errorf("typed = %q; want %q", got, tt.wantTyped)
			}
			if strings.HasPrefix(tt.wantTyped, "COLOR:") && reed.EndsAtColor != 1 {
				t.Errorf("load turn ends at the color = %d; want 1, the color following the first turn's end", reed.EndsAtColor)
			}
		})
	}
}

func TestRunner_Start_ValidationFailure_ShortCircuitsBeforeReedCall(t *testing.T) {
	reed := &fakeReed{}
	engine := &fakeEngine{}
	runner := newFixture(t, reed, engine, withConfig(fastConfig)).Runner

	if _, err := runner.Start(Spec{}); err == nil {
		t.Fatal("Start() = nil error, want validation error for empty spec")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed calls = %v, want none (validation must short-circuit)", reed.CallLog)
	}
	if len(engine.PrepareCalls) != 0 {
		t.Errorf("engine.Prepare calls = %d, want 0", len(engine.PrepareCalls))
	}
}

func TestRunner_Start_AddStrandFailure_CleansRunDir(t *testing.T) {
	reed := &fakeReed{AddStrandErr: fmt.Errorf("boom")}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd"}}
	fx := newFixture(t, reed, engine, withConfig(fastConfig))
	runner, anchorPath := fx.Runner, fx.Anchor

	if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil {
		t.Fatal("Start() = nil error, want AddStrand failure to propagate")
	}

	root := runDirRoot(runner.cfg, anchorPath)
	entries, rerr := os.ReadDir(root)
	if rerr != nil && !os.IsNotExist(rerr) {
		t.Fatalf("read run dir root: %v", rerr)
	}
	if len(entries) != 0 {
		t.Errorf("run dir root has %d leftover entr(y/ies), want 0 (AddStrand failure must clean up)", len(entries))
	}
}

// TestRunner_Start_SaveRunStateFailure covers AddStrand succeeding (the strand and its launching
// pane exist) and the subsequent saveRunState failing. Start must tear the strand back down and
// remove the run directory rather than leaking a live, untracked pane no run.json can ever bind.
//
// By then AddStrand has put a real pane and a real provider process on the substrate, so the
// RemoveStrand that follows is a live-substrate teardown, and a RemoveStrand that ERRORS is
// precisely "a teardown that did not confirm clean", which PATTERN-spawn-observability requires on
// internal/logger at Warn. It used to go to the bare log package, which the durable Info+ trace sink
// never captures, so the one record of a leaked live pane existed only on an ephemeral stderr with no
// trace correlation id on it.
func TestRunner_Start_SaveRunStateFailure(t *testing.T) {
	tests := []struct {
		name            string
		removeStrandErr error
		wantLogged      []string
	}{
		{name: "removes the strand and the run dir"},
		{
			name:            "a teardown that errors is logged through the logger",
			removeStrandErr: errors.New("reed: no session"),
			wantLogged:      []string{"remove strand after save-state failure", "strand-1", "reed: no session"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{
				AddStrandResult: reedengine.Strand{GUID: "strand-1"},
				RemoveStrandErr: tt.removeStrandErr,
			}
			engine := &fakeEngine{
				PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"},
				// Plant run.json as a DIRECTORY so the later saveRunState write can never succeed,
				// forcing the mid-op failure this test exercises.
				PrepareHook: func(runDir string) {
					if err := os.MkdirAll(filepath.Join(runDir, runStateFileName), 0o755); err != nil {
						t.Fatalf("plant run.json dir: %v", err)
					}
				},
			}
			fx := newFixture(t, reed, engine, withConfig(fastConfig))
			runner, anchorPath := fx.Runner, fx.Anchor

			buf := logcapture.CaptureVerbose(t)
			if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil {
				t.Fatal("Start() = nil error, want save-run-state failure to propagate")
			}

			foundRemove := false
			for _, c := range reed.RemoveStrandCalls {
				if c.GUID == "strand-1" && !c.Recursive {
					foundRemove = true
				}
			}
			if !foundRemove {
				t.Errorf("RemoveStrand(strand-1, false) not recorded after save-state failure; strand leaked. calls = %+v", reed.RemoveStrandCalls)
			}

			logged := buf.String()
			for _, want := range tt.wantLogged {
				if !strings.Contains(logged, want) {
					t.Errorf("logger output = %q; want it to contain %q", logged, want)
				}
			}

			if tt.removeStrandErr != nil {
				return
			}
			root := runDirRoot(runner.cfg, anchorPath)
			entries, rerr := os.ReadDir(root)
			if rerr != nil && !os.IsNotExist(rerr) {
				t.Fatalf("read run dir root: %v", rerr)
			}
			if len(entries) != 0 {
				t.Errorf("run dir root has %d leftover entr(y/ies), want 0 (save-state failure must clean up)", len(entries))
			}
		})
	}
}

// TestShuttleengine_LiveSubstrateLoggingGoesThroughLogger is the other half of R2-F8's guard, and
// the one that actually prevents reintroduction.
//
// This whole package is a live-substrate module: every operational message it emits is about a real
// pane or a real provider process, so every one of them belongs on internal/logger's durable sink
// rather than on the stdlib log package, whose output the trace file never sees and which carries no
// trace correlation id. R1-F10 migrated finalize's two sites and left five siblings behind in these
// same two files; a behavioural test can only ever pin the sites it happens to exercise, so the
// no-reintroduction half is a source scan, following the guard-test idiom cmd/lyx already uses.
//
// The scan reads this package's own production sources by name from the test's own working
// directory — no process is spawned and no scan root has to be resolved, so the Test Tier Purity
// Invariant is satisfied without an allowlist entry.
//
//testtiming:keep scans this package's production sources for bare log and fmt.Println calls, which no behavioral test can pin
func TestShuttleengine_LiveSubstrateLoggingGoesThroughLogger(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("glob matched no .go files; the scan would pass vacuously")
	}

	scanned := 0
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		data, readErr := os.ReadFile(source)
		if readErr != nil {
			t.Fatalf("read %s: %v", source, readErr)
		}
		scanned++
		for _, banned := range []string{"log.Printf(", "log.Println(", "log.Fatal", "fmt.Println("} {
			if strings.Contains(string(data), banned) {
				t.Errorf("%s uses %s — this package's operational messages are all about a real pane or provider process, so they belong on internal/logger (logger.Info for a normal spawn/teardown, logger.Warn for a retry or a teardown that did not confirm clean), whose durable Info+ trace sink the stdlib log package never reaches", source, banned)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no production source files scanned; the guard would pass vacuously")
	}
}

// TestRunner_Start_SweepSkipsEntirelyWithoutReedState covers Start's opportunistic orphan sweep when
// reed's state file is not a trustworthy live set. Start must log and continue rather than fail the
// whole run over a housekeeping error, and the sweep must be skipped for this Start entirely.
//
// A LoadState ERROR must not degrade to "sweep with an empty live set": that would delete every
// old-enough run dir, including a kept died/timeout dir reed still genuinely tracks, over an
// unrelated I/O problem. An ABSENT reed.json used to fall through to the same empty live-guid set
// (R3-F2), so every run dir past the age guard was deleted — including one whose agent is still
// working. That state is not exotic: it is exactly what reed's own corrupt-state error recommends
// ("delete <path> by hand to keep the session (its panes and their processes keep running,
// untracked)") and what a `git clean -xdf` of .lyx leaves behind. The sweep then removes the live
// run's events.jsonl and run.json, after which interrupt/send answer "is not a shuttle strand" for a
// running agent.
func TestRunner_Start_SweepSkipsEntirelyWithoutReedState(t *testing.T) {
	tests := []struct {
		name string
		// reedJSON is the corrupt reed.json to seed; empty leaves .lyx without one — the
		// hand-deleted / git-cleaned shape.
		reedJSON string
	}{
		{name: "unreadable reed.json", reedJSON: "not json"},
		{name: "absent reed.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
			engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"}}

			// anchorPath is a real subpath of worktree, never the same value twice: passing one
			// directory for both fields would let a swapped NewRunner argument pair pass this test,
			// which is exactly the masking case newFixture's own contract exists to prevent.
			worktree := t.TempDir()
			anchorPath := filepath.Join(worktree, "sub", "dir")
			cfg := defaultConfig

			if err := os.MkdirAll(filepath.Join(anchorPath, lyxdirs.DotLyxDirName), 0o755); err != nil {
				t.Fatalf("mkdir .lyx: %v", err)
			}
			if tt.reedJSON != "" {
				if err := os.WriteFile(filepath.Join(anchorPath, lyxdirs.DotLyxDirName, "reed.json"), []byte(tt.reedJSON), 0o644); err != nil {
					t.Fatalf("seed corrupt reed.json: %v", err)
				}
			}

			// An old, kept run dir (as a died/timeout outcome would leave behind) whose
			// strand is not in a live set that reed.json cannot supply.
			shuttleRoot := runDirRoot(cfg, anchorPath)
			keptDir := seedRun(t, shuttleRoot, "kept-run", "some-other-strand")
			setDirMTime(t, keptDir, time.Now(), 10*time.Minute)

			runner := NewRunner(reed, singleEngineSet(engine), anchorPath, worktree, cfg)
			readyStart(reed, engine)
			run, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}})
			if err != nil {
				t.Fatalf("Start() error: %v, want sweep failure to be non-blocking", err)
			}
			if run == nil {
				t.Fatal("Start() returned nil run")
			}

			if _, err := os.Stat(keptDir); err != nil {
				t.Errorf("run dir was swept with no usable reed.json to prove it orphaned, want it preserved: %v", err)
			}
		})
	}
}

// liveStrandStatus scripts a fakeReed Status answer reporting strand-1 with
// the given liveness — what the Interrupt/Send liveness guard consumes.
// The pane id is part of the fixture rather than incidental: reed reports one for every strand whose
// pane it still holds, so an EMPTY pane id means something different (a binding reed cleared), and a
// not-live fixture without one would be testing that case instead of a dead pane.
func liveStrandStatus(live bool) []reedengine.StatusResult {
	return []reedengine.StatusResult{
		{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%0", Live: live}}},
	}
}

// readyAgentEngine returns a fakeEngine whose Startup classification sticks
// at StartupReady — the pane state requireReadyAgentPane demands before any
// Interrupt/Send key is played, so tests of the key choreography itself pass
// the guard without scripting captures per call.
func readyAgentEngine() *fakeEngine {
	return &fakeEngine{StartupScript: []StartupState{StartupReady}}
}

// TestRun_Interrupt_PlaysEscape covers the guarded Interrupt on a live pane: the guard sends the key only into a pane showing a running turn.
// One capture can land mid-redraw and classify a healthy TUI as still booting, so the ready probe
// must retry within agentPaneProbeAttempts and pass once a later capture classifies Ready, rather
// than refusing on the first inconclusive frame.
// A runner row also asserts the Runner's settle and pane running-turn readings on the same fixture.
func TestRun_Interrupt_PlaysEscape(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		script      []StartupState
		viaRunner   bool
		captures    []string
		captureErrs []error
		wantKeys    int
		wantErr     error
		wantMsg     string
		wantRunning bool
	}{
		{name: "ready pane showing a running turn", script: []StartupState{StartupReady}, captures: []string{"RUNNING"}, wantKeys: 1, wantRunning: true},
		{name: "ready probe retries a transient boot", script: []StartupState{StartupPending, StartupReady}, captures: []string{"RUNNING"}, wantKeys: 1, wantRunning: true},
		{name: "runner interrupt into a running turn", script: []StartupState{StartupReady}, viaRunner: true, captures: []string{"RUNNING"}, wantKeys: 1, wantRunning: true},
		{name: "a pane showing no running turn gets no key", script: []StartupState{StartupReady}, captures: []string{"idle"}, wantErr: ErrNoRunningTurn, wantMsg: "lyx shuttle send"},
		{name: "a pane showing no running turn gets no key from the runner", script: []StartupState{StartupReady}, viaRunner: true, captures: []string{"idle"}, wantErr: ErrNoRunningTurn, wantMsg: "lyx shuttle send"},
		{name: "a failed guard capture sends no key", script: []StartupState{StartupReady}, captures: []string{"RUNNING"}, captureErrs: []error{nil, boom}, wantErr: boom},
		{name: "a failed guard capture sends no key from the runner", script: []StartupState{StartupReady}, viaRunner: true, captures: []string{"RUNNING"}, captureErrs: []error{nil, boom}, wantErr: boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubInputSleep(t)
			reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: tt.captures, CaptureErrQueue: tt.captureErrs}
			engine := &fakeEngine{StartupScript: tt.script, Settle: 2 * time.Second}
			var err error
			var runner *Runner
			if tt.viaRunner {
				runner = newFixture(t, reed, engine, withConfig(Config{}), withStrand("strand-1")).Runner
				err = runner.Interrupt("strand-1")
			} else {
				err = newFixture(t, reed, engine, withConfig(Config{})).newRun(Spec{}).Interrupt()
			}

			if tt.wantErr == nil && err != nil {
				t.Fatalf("Interrupt() error: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Interrupt() error = %v; want it to wrap %v", err, tt.wantErr)
			}
			if tt.wantErr == boom && errors.Is(err, ErrNoRunningTurn) {
				t.Errorf("Interrupt() error = %v; a failed capture must not read as no running turn", err)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("Interrupt() error = %v; want it to contain %q", err, tt.wantMsg)
			}
			if len(reed.SendKeyCalls) != tt.wantKeys {
				t.Errorf("SendKey calls = %+v, want %d", reed.SendKeyCalls, tt.wantKeys)
			}
			if tt.wantKeys == 1 && reed.SendKeyCalls[0].Key != "Escape" {
				t.Errorf("SendKey calls = %+v, want one Escape", reed.SendKeyCalls)
			}
			if len(reed.SendTextCalls) != 0 {
				t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
			}
			if !tt.viaRunner {
				return
			}
			if got := runner.InterruptSettle(); got != 2*time.Second {
				t.Errorf("InterruptSettle() = %v; want 2s", got)
			}
			running, runningErr := runner.PaneRunningTurn("strand-1")
			if tt.wantErr == boom {
				if !errors.Is(runningErr, boom) {
					t.Errorf("PaneRunningTurn() error = %v; want it to wrap boom", runningErr)
				}
				return
			}
			if runningErr != nil || running != tt.wantRunning {
				t.Errorf("PaneRunningTurn() = %v, %v; want %v, nil", running, runningErr, tt.wantRunning)
			}
		})
	}
}

// TestGuardedPickerClose covers the guarded rewind-picker close before an interrupt and before a verified send:
// one interrupt key closes an open picker and fails the operation, nothing more is sent, and no text is typed.
func TestGuardedPickerClose(t *testing.T) {
	boom := errors.New("boom")
	const (
		runInterrupt    = "run-interrupt"
		runnerInterrupt = "runner-interrupt"
		runnerSend      = "runner-send"
	)
	tests := []struct {
		name        string
		ops         []string
		pickerOnly  bool
		captures    []string
		captureErrs []error
		wantKeys    int
		wantErr     error
		wantNotErr  error
		wantMsg     string
		wantSettle  bool
	}{
		{
			name: "a picker on an interrupt is closed with one key and fails", ops: []string{runInterrupt, runnerInterrupt},
			captures: []string{"REWIND-PICKER", "REWIND-PICKER", "❯ "},
			wantKeys: 1, wantErr: ErrRewindPickerOpen, wantNotErr: ErrSessionBusy, wantMsg: "closed it", wantSettle: true,
		},
		{
			name: "a picker on a send is closed with one key, fails and types nothing", ops: []string{runnerSend},
			captures: []string{"REWIND-PICKER", "❯ "},
			wantKeys: 1, wantErr: ErrRewindPickerOpen, wantNotErr: ErrSessionBusy, wantMsg: "closed it", wantSettle: true,
		},
		{
			name: "a picker still on screen after the close sends nothing more", ops: []string{runnerInterrupt, runnerSend},
			captures: []string{"REWIND-PICKER"},
			wantKeys: 1, wantErr: ErrRewindPickerOpen, wantNotErr: ErrSessionBusy, wantMsg: "still on screen", wantSettle: true,
		},
		{
			name: "no picker on an interrupt: the close sends nothing and the guard decides", ops: []string{runInterrupt, runnerInterrupt},
			captures: []string{"RUNNING ❯ "},
			wantKeys: 1,
		},
		{
			name: "no picker on a send: the close sends nothing and the send goes on to its idle wait", ops: []string{runnerSend},
			captures: []string{"working"},
			wantErr:  ErrSessionBusy,
		},
		{
			name: "a failed picker capture on a send skips the close and the idle wait holds", ops: []string{runnerSend},
			captures: []string{"working"}, captureErrs: []error{boom, nil},
			wantErr: ErrSessionBusy,
		},
		{
			name: "a failed picker capture on a send without polls fails closed", ops: []string{runnerSend}, pickerOnly: true,
			captures: []string{"❯ "}, captureErrs: []error{nil, boom},
			wantErr: boom, wantNotErr: ErrSessionBusy,
		},
	}
	for _, tt := range tests {
		for _, op := range tt.ops {
			t.Run(tt.name+"/"+op, func(t *testing.T) {
				stubInputSleep(t)
				reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: tt.captures, CaptureErrQueue: tt.captureErrs}
				clock := newFakeClock(time.Now())
				start := clock.Now()
				cfg := Config{SendReadyTimeoutS: 5}
				var engine Engine
				if tt.pickerOnly {
					picker := &pickerOnlyEngine{}
					picker.StartupScript, picker.Settle = []StartupState{StartupReady}, 2*time.Second
					engine = picker
				} else {
					picker := &pickerEngine{}
					picker.StartupScript, picker.Settle = []StartupState{StartupReady}, 2*time.Second
					engine = picker
				}
				fx := newFixture(t, reed, engine, withConfig(cfg), withClock(clock), withStrand("strand-1"))

				var err error
				switch op {
				case runInterrupt:
					err = fx.newRun(Spec{}, withRunClock(clock, time.Time{})).Interrupt()
				case runnerInterrupt:
					err = fx.Runner.Interrupt("strand-1")
				case runnerSend:
					err = fx.Runner.Send("strand-1", "hello")
				}

				switch {
				case tt.wantErr == nil && err != nil:
					t.Fatalf("%s error: %v", op, err)
				case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
					t.Fatalf("%s error = %v; want it to wrap %v", op, err, tt.wantErr)
				}
				if tt.wantNotErr != nil && errors.Is(err, tt.wantNotErr) {
					t.Errorf("%s error = %v; must not wrap %v", op, err, tt.wantNotErr)
				}
				if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("%s error = %v; want it to contain %q", op, err, tt.wantMsg)
				}
				if len(reed.SendKeyCalls) != tt.wantKeys {
					t.Errorf("SendKey calls = %+v; want %d", reed.SendKeyCalls, tt.wantKeys)
				}
				for _, call := range reed.SendKeyCalls {
					if call.Key != "Escape" {
						t.Errorf("SendKey calls = %+v; want only Escape", reed.SendKeyCalls)
					}
				}
				if len(reed.SendTextCalls) != 0 || len(reed.PasteSubmitCalls) != 0 {
					t.Errorf("text reached the pane: SendText=%+v PasteSubmit=%+v", reed.SendTextCalls, reed.PasteSubmitCalls)
				}
				if tt.wantSettle {
					if waited := clock.Now().Sub(start); waited < 2*time.Second {
						t.Errorf("clock advanced %v; want at least the 2s settle", waited)
					}
					if last := reed.CallLog[len(reed.CallLog)-1]; last != "CapturePane" {
						t.Errorf("last reed call = %q; want the re-check CapturePane", last)
					}
					if len(reed.CallLog) < 2 || reed.CallLog[len(reed.CallLog)-2] != "SendKey:Escape" {
						t.Errorf("reed calls = %v; want the re-check capture right after the Escape", reed.CallLog)
					}
				}
			})
		}
	}
}

func TestRun_Send_RejectsNewlines(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	engine := &fakeEngine{}
	run := newFixture(t, reed, engine, withConfig(Config{})).newRun(Spec{})

	if err := run.Send("line one\nline two"); err == nil {
		t.Fatal("Send() = nil error, want rejection for multiline text")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed calls = %v, want none (rejected before any reed call)", reed.CallLog)
	}
}

func TestRun_Send_RejectsEmptyOrWhitespace(t *testing.T) {
	// An empty or whitespace-only send has nothing to deliver: it would still
	// play the Escape+submit choreography (a stray empty turn) while making
	// sendVerified's delivery check vacuous (an empty needle every capture
	// "contains"), so Send must refuse before touching the pane rather than
	// report a falsely-verified success.
	tests := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"spaces", "   "},
		{"tab", "\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
			run := newFixture(t, reed, &fakeEngine{}, withConfig(Config{})).newRun(Spec{})

			if err := run.Send(tt.text); err == nil {
				t.Fatalf("Send(%q) = nil error, want rejection for empty/whitespace text", tt.text)
			}
			if len(reed.CallLog) != 0 {
				t.Errorf("reed calls = %v, want none (rejected before any reed call)", reed.CallLog)
			}
		})
	}
}

// stubInputSleep replaces the package-level inputSleep seam with a no-op for
// the duration of the calling test, restoring the real implementation via
// t.Cleanup — so sendVerified's poll/replay loops (and playInputs' SettleMS
// pause) run at test speed instead of real wall-clock time.
func stubInputSleep(t *testing.T) {
	t.Helper()
	orig := inputSleep
	inputSleep = func(time.Duration) {}
	t.Cleanup(func() { inputSleep = orig })
}

// repeatCapture returns n copies of capture, the fixture shape fakeReed's
// CaptureQueue consumes one-per-call.
func repeatCapture(capture string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = capture
	}
	return out
}

func TestRun_InterruptAndSend_RefuseDeadOrUntrackedStrand(t *testing.T) {
	// tmux send-keys against a dead or missing pane exits 0 while
	// delivering nothing (proven live), so Interrupt/Send must refuse
	// before touching the pane rather than report a silent-no-op success.
	tests := []struct {
		name   string
		status []reedengine.StatusResult
		// wantIn is a phrase the refusal must name. The untracked case pins the
		// state-reset cause explicitly: reed's table is also emptied by a remove,
		// a down/up cycle, and a server rebirth, so a refusal claiming only that
		// "its run has completed and been cleaned up" misdirects an operator whose
		// agent is still running in its pane (proven live).
		wantIn string
	}{
		{"dead_pane", liveStrandStatus(false), "no live pane"},
		// A strand reed tracks with NO pane bound is not a dead pane: reed cleared the binding as
		// stale, and its agent is very often still working in a pane reed can no longer address
		// (proven live). Naming a terminal outcome or a dead pane there was wrong on both counts.
		{"cleared_pane_binding", []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "", Live: false}}}}, "holds no pane id for it"},
		{"untracked_strand", []reedengine.StatusResult{{}}, "reed's strand table was reset under it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: tt.status}
			run := newFixture(t, reed, &fakeEngine{}, withConfig(Config{})).newRun(Spec{})

			if err := run.Interrupt(); err == nil {
				t.Error("Interrupt() = nil error, want liveness refusal")
			} else if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Interrupt() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := run.Send("still there?"); err == nil {
				t.Error("Send() = nil error, want liveness refusal")
			} else if errors.Is(err, ErrPaneNotReady) {
				t.Errorf("Send() error = %v, must not wrap ErrPaneNotReady", err)
			}
			if len(reed.SendKeyCalls) != 0 || len(reed.SendTextCalls) != 0 {
				t.Errorf("keys reached the pane despite refusal: SendKey=%+v SendText=%+v", reed.SendKeyCalls, reed.SendTextCalls)
			}
		})
	}
}

func TestRun_InterruptAndSend_RefuseAgentlessShellPane(t *testing.T) {
	// A live pane is not enough: a provider that failed at launch (or was
	// killed while its shell survived) leaves a live pane whose keys land at
	// the shell prompt — proven live, a send against a kept "died" run
	// reported ok while its text was EXECUTED as a pwsh command. The engine
	// classifying every probe capture as still-booting (a bare shell prompt
	// shows no ready marker) must therefore refuse before any key is played.
	stubInputSleep(t)
	reed := &fakeReed{
		StatusQueue:  liveStrandStatus(true),
		CaptureQueue: []string{"PS C:\\Code\\hub> "},
	}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending}}
	run := newFixture(t, reed, engine, withConfig(Config{})).newRun(Spec{})

	if err := run.Interrupt(); err == nil || !strings.Contains(err.Error(), "no input-ready provider TUI") {
		t.Errorf("Interrupt() error = %v, want the no-ready-TUI refusal", err)
	}
	if err := run.Send("echo poked"); err == nil || !strings.Contains(err.Error(), "no input-ready provider TUI") {
		t.Errorf("Send() error = %v, want the no-ready-TUI refusal", err)
	} else if !errors.Is(err, ErrPaneNotReady) {
		t.Errorf("Send() error = %v, want it to wrap ErrPaneNotReady", err)
	}
	// The refusal must own BOTH readings of a non-Ready capture — a provider
	// still starting up and one that exited behind a surviving shell — since
	// the probe cannot distinguish them and misdiagnosing a booting pane as a
	// dead agent steers an operator toward tearing down a healthy run.
	if err := run.Interrupt(); err == nil || !strings.Contains(err.Error(), "still starting up") || !strings.Contains(err.Error(), "exited") {
		t.Errorf("Interrupt() error = %v, want it to name both the still-starting and exited readings", err)
	}
	if len(reed.SendKeyCalls) != 0 || len(reed.SendTextCalls) != 0 {
		t.Errorf("keys reached the agent-less shell despite refusal: SendKey=%+v SendText=%+v", reed.SendKeyCalls, reed.SendTextCalls)
	}
}

// liveRecordedPaneFrame reads one of the pane captures recorded during round 4's live reproduction of
// R2-F11 against a real Claude TUI (220x48 pane, `tmux capture-pane -p`).
// The two frames are kept verbatim rather than hand-written, so this regression test is pinned to what
// the provider TUI actually rendered rather than to a reviewer's model of it.
func liveRecordedPaneFrame(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read recorded pane frame %q: %v", name, err)
	}
	return string(data)
}

// recordedScrollSendText is the exact text sent in that live reproduction.
// Its first 48 normalized characters — the needle sendVerified derives — are shared with the copy
// already on screen in the baseline frame, which is what makes the baseline count 1 rather than 0.
const recordedScrollSendText = "reply with the numbers 1 to 38 one per line and nothing else, and take care to print each number on its very own separate line, with no extra commentary, no preamble, no summary and no trailing remarks whatsoever, just the plain numbers in order"

// TestRun_Send_DeliveryVerification drives Send's delivery check over scripted pane captures. The
// first capture of every queue answers requireReadyAgentPane's TUI probe and the second is
// sendVerified's pre-send baseline; the rest are the verification polls, the last entry sticking.
//
// The sent text can already be on screen before the send — an operator retrying the same
// instruction after an uncertain first attempt, or text quoting the agent's own visible output —
// so presence alone would "verify" even a swallowed send; delivery is judged by the occurrence
// count RISING above the pre-send baseline. CapturePane returns the pane's visible VIEWPORT, not
// its scrollback, so an occurrence counted at baseline time can scroll off while the agent works
// (R2-F6); a false negative there is not a harmless retry but a duplicate agent turn, from a send
// that actually landed. R4-F1 closes R2-F11 on the branch re-baselining cannot reach: the delivered
// copy's position, 9 lines from the bottom where every copy the baseline counted sat 46 lines from
// it, is the evidence a count cannot carry, because a pane only ever appends at its bottom.
func TestRun_Send_DeliveryVerification(t *testing.T) {
	baselineFrame := liveRecordedPaneFrame(t, "pane-scroll-baseline.txt")
	tests := []struct {
		name     string
		captures []string
		text     string
		// wantErrIn is a fragment the delivery failure must name; empty with wantErr unset means
		// the send must verify.
		wantErr   bool
		wantErrIn string
		// wantSends is the expected SendText call count; zero leaves it unchecked.
		wantSends int
		wantLog   []string
	}{
		{
			// The later captures report the sent text back for the delivery check to succeed without
			// a replay.
			name:      "plays Escape then the text with submit",
			captures:  []string{"❯ ", "❯ ", "❯ updated instructions"},
			text:      "updated instructions",
			wantSends: 1,
			// Status then CapturePane lead the log: the liveness guard and the ready-TUI probe must
			// both run before any key reaches the pane; the second CapturePane is the pre-send
			// baseline snapshot. The final CapturePane follows the text step: the delivery poll.
			wantLog: []string{"Status", "CapturePane", "CapturePane", "SendKey:Escape", "SendText:updated instructions", "CapturePane"},
		},
		{
			// The provider TUI can swallow the whole Escape+text chunk with no error anywhere
			// (observed live): every poll of the first attempt sees no trace of the text, the
			// replay's polls see it delivered.
			name: "swallowed first attempt succeeds on the replay",
			captures: append(
				repeatCapture("❯ ", 2+sendVerifyAttempts),
				repeatCapture("❯ updated instructions", sendVerifyAttempts)...,
			),
			text:      "updated instructions",
			wantSends: 2,
		},
		{
			// If the text never appears even after the replay, Send must report a delivery failure
			// rather than the "keys were emitted" false ok:true observed live.
			name:      "never delivered reports an honest failure",
			captures:  repeatCapture("❯ ", 2*sendVerifyAttempts+2),
			text:      "updated instructions",
			wantErr:   true,
			wantErrIn: "never appeared",
			wantSends: 1 + sendReplays,
		},
		{
			// Every capture shows the text exactly once: it was already there, and the send never
			// lands. A presence check would falsely verify this.
			name:     "pre-existing text with a swallowed send reports failure",
			captures: []string{"❯ do it again"},
			text:     "do it again",
			wantErr:  true,
		},
		{
			// Probe and baseline see one pre-existing occurrence; the verification polls see a
			// second one appear — a genuine delivery.
			name:     "pre-existing text with a rising count verifies delivery",
			captures: []string{"❯ do it again", "❯ do it again", "do it again …\n❯ do it again"},
			text:     "do it again",
		},
		{
			// Probe and baseline: the text is already on screen TWICE from earlier turns, so the
			// baseline starts at 2. The first poll sees both earlier copies pushed off the top of
			// the viewport and the delivered copy not rendered yet (count 0); the second sees it
			// (count 1, below the ORIGINAL baseline, which is why the pre-fix check never passed).
			name: "baseline occurrences scrolled away do not duplicate the delivery",
			captures: []string{
				"❯ do it again do it again",
				"❯ do it again do it again",
				"❯ working...",
				"❯ do it again",
			},
			text:      "do it again",
			wantSends: 1,
		},
		{
			// Both frames are real captures taken 800 ms apart during the live reproduction. In the
			// baseline frame one copy of the text sits at line 1 — the top of a full viewport. In
			// the delivered frame the newly delivered copy is rendered at line 38 and that earlier
			// copy has scrolled off in the SAME redraw, so the count is 1 in both.
			name: "baseline occurrence evicted as the delivered one arrives does not replay",
			captures: []string{
				baselineFrame,
				baselineFrame,
				liveRecordedPaneFrame(t, "pane-scroll-delivered.txt"),
			},
			text:      recordedScrollSendText,
			wantSends: 1,
		},
		{
			// The send is swallowed and the pane merely scrolls, which moves the pre-existing copy
			// UP — further from the bottom, never closer. Scrolling ALONE is not evidence of
			// delivery, so the swallowed-send failure path must still fire, replays included.
			name: "viewport scrolling without delivery still reports failure",
			captures: []string{
				"● working\n❯ do it again\nstill working",
				"● working\n❯ do it again\nstill working",
				"❯ do it again\nline\nline\nline\nline\nstill working",
			},
			text:      "do it again",
			wantErr:   true,
			wantErrIn: "never appeared",
			wantSends: 1 + sendReplays,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: tt.captures}
			clock := newFakeClock(time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC))
			run := newFixture(t, reed, readyAgentEngine(), withConfig(Config{})).newRun(Spec{}, withRunClock(clock, clock.Now().Add(time.Hour)))

			err := run.Send(tt.text)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Send() = nil error, want a delivery failure")
				}
				if !errors.Is(err, ErrSubmissionNotLanded) {
					t.Errorf("Send() error = %v, want it to wrap ErrSubmissionNotLanded", err)
				}
				if !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Errorf("Send() error = %q, want it to name %q", err, tt.wantErrIn)
				}
			} else if err != nil {
				t.Fatalf("Send() error: %v, want the delivery verified", err)
			}
			if tt.wantSends != 0 && len(reed.SendTextCalls) != tt.wantSends {
				t.Errorf("SendText calls = %d, want %d", len(reed.SendTextCalls), tt.wantSends)
			}
			if tt.wantLog != nil {
				if !reflect.DeepEqual(reed.CallLog, tt.wantLog) {
					t.Errorf("call order = %v, want %v", reed.CallLog, tt.wantLog)
				}
				if len(reed.SendTextCalls) != 1 || !reed.SendTextCalls[0].Submit {
					t.Errorf("SendText calls = %+v, want one call with Submit=true", reed.SendTextCalls)
				}
			}
		})
	}
}

// inputBoxAnswer is one scripted InputBoxText reading.
type inputBoxAnswer struct {
	text string
	ok   bool
}

// inputBoxEngine wraps a fakeEngine with the InputBoxReader capability over a scripted queue of box answers.
// The queue is consumed FIFO and its last answer sticks, like fakeReed's capture queue.
type inputBoxEngine struct {
	*fakeEngine
	boxes  []inputBoxAnswer
	settle time.Duration
}

func (e *inputBoxEngine) InputBoxText(string) (string, bool) {
	answer := e.boxes[0]
	if len(e.boxes) > 1 {
		e.boxes = e.boxes[1:]
	}
	return answer.text, answer.ok
}

func (e *inputBoxEngine) SubmitSettle() time.Duration { return e.settle }

func (e *inputBoxEngine) TypeSequence(text string) []PaneInput {
	return []PaneInput{{Key: "Escape"}, {Text: text}}
}

// boxTestClock is a fakeClock that records every Sleep, so a row can pin the settle and confirm intervals.
type boxTestClock struct {
	*fakeClock
	sleeps []time.Duration
}

func (c *boxTestClock) Sleep(d time.Duration) {
	c.sleeps = append(c.sleeps, d)
	c.fakeClock.Sleep(d)
}

// clearableBoxEngine is inputBoxEngine plus the idle reading, session signals, a session prober and a scripted InputBoxClearer:
// A capture is idle when it starts with "IDLE".
// "START" is a turn start, "STOP:<message>" a turn end and "SESSIONSTART" or "SESSIONSTART:<source>" a session start with that source.
// Every turn start reads interrupted when interrupted is set, and TurnStartProgress answers progress for any turn start.

type clearableBoxEngine struct {
	*inputBoxEngine

	interrupted bool
	progress    TurnProgress
}

func (e *clearableBoxEngine) TurnStartProgress(SessionSignal) TurnProgress { return e.progress }

func (e *clearableBoxEngine) ProcessLiveness(string) Liveness { return LivenessUnproven }
func (e *clearableBoxEngine) TurnStartInterrupt(SessionSignal) (time.Time, bool) {
	return time.Time{}, e.interrupted
}

func (e *clearableBoxEngine) ContextTokens(Event) ContextReading { return ContextReading{} }
func (e *clearableBoxEngine) CompactedSince(Event, time.Time) (CompactionBoundary, bool) {
	return CompactionBoundary{}, false
}
func (e *clearableBoxEngine) IdleSession(capture string) bool {
	return strings.HasPrefix(capture, "IDLE")
}
func (e *clearableBoxEngine) PaneTooShort(string) bool { return false }
func (e *clearableBoxEngine) ClearSessionSequence() []PaneInput {
	return nil
}
func (e *clearableBoxEngine) ReloadPluginsSequence() []PaneInput { return nil }
func (e *clearableBoxEngine) CompactSessionSequence(string) []PaneInput {
	return nil
}
func (e *clearableBoxEngine) ClearInputSequence() []PaneInput { return []PaneInput{{Key: "C-u"}} }
func (e *clearableBoxEngine) ParseSessionSignals(data []byte) ([]SessionSignal, int) {
	var signals []SessionSignal
	for _, line := range strings.Split(string(data), "\n") {
		switch trimmed := strings.TrimSpace(line); {
		case trimmed == "START":
			signals = append(signals, SessionSignal{Kind: SessionSignalTurnStart})
		case trimmed == "APIERR":
			signals = append(signals, SessionSignal{Kind: SessionSignalAPIErrorTurnEnd})
		case strings.HasPrefix(trimmed, "STOP:"):
			signals = append(signals, SessionSignal{Kind: SessionSignalTurnEnd})
		case strings.HasPrefix(trimmed, "SESSIONSTART"):
			source := SessionStartSource(strings.TrimPrefix(strings.TrimPrefix(trimmed, "SESSIONSTART"), ":"))
			signals = append(signals, SessionSignal{Kind: SessionSignalSessionStart, Source: source})
		}
	}
	return signals, len(data)
}

var (
	_ SessionProber       = (*clearableBoxEngine)(nil)
	_ SessionCycler       = (*clearableBoxEngine)(nil)
	_ InputBoxClearer     = (*clearableBoxEngine)(nil)
	_ SessionSignalParser = (*clearableBoxEngine)(nil)
	_ TurnProgressReader  = (*clearableBoxEngine)(nil)
)

// enterHookReed is a fakeReed that runs onEnter after every Enter key.
type enterHookReed struct {
	*fakeReed
	onEnter func()
}

func (r *enterHookReed) SendKey(guid, key string) error {
	err := r.fakeReed.SendKey(guid, key)
	if key == "Enter" && r.onEnter != nil {
		r.onEnter()
	}
	return err
}

// TestRun_Send_ConfirmsSubmission drives Send's settle, submit window and clear through an engine with the InputBoxReader capability on a fake clock.
// No Enter goes out until two box reads agree.
// Every Enter is followed by one box read at a doubling interval.
// An extra Enter goes only while the box holds the sent text and the window is open.
// A send that did not land clears its own text only for an engine with the idle reading and InputBoxClearer.
func TestRun_Send_ConfirmsSubmission(t *testing.T) {
	t.Parallel()
	const shortText = "run the suite"
	const longText = "please review the whole change set carefully and report every finding you can substantiate"
	const placeholder = "[Pasted text #1 +4 lines]"
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	box := func(text string) inputBoxAnswer { return inputBoxAnswer{text, true} }
	// settled is the two agreeing reads before the first Enter.
	settled := func(text string) []inputBoxAnswer { return []inputBoxAnswer{box(text), box(text)} }
	join := func(parts ...[]inputBoxAnswer) []inputBoxAnswer {
		var out []inputBoxAnswer
		for _, part := range parts {
			out = append(out, part...)
		}
		return out
	}
	keys := func(names ...string) []string { return names }
	// tallFrame is twenty numbered rows with a blank row after each, taller than the pane tail a failure carries.
	var tallFrame string
	for row := 1; row <= 20; row++ {
		tallFrame += fmt.Sprintf("row-%02d\n\n", row)
	}

	tests := []struct {
		name string
		text string
		// idle gives the engine the idle reading and InputBoxClearer.
		idle bool
		// settleMS and confirmS override the 100 ms settle interval and the 30 s window.
		settleMS, confirmS int
		// captures replaces the pane captures; the last sticks.
		captures []string
		// boxes are the input-box answers from the first settle read on; the last sticks.
		boxes []inputBoxAnswer
		// startOnEnter appends a turn start to the events file after every Enter.
		startOnEnter bool
		// frozen gives the run a clock that never advances, so only the window's attempt count can end the send.
		frozen bool
		// pastDeadline sends through the gate's sendWithin with the run deadline already passed.
		pastDeadline bool
		wantErr      string
		wantAbsent   string
		wantNotLand  bool
		wantKeys     []string
		// wantSleeps, when set, is every clock sleep of the send.
		wantSleeps []time.Duration
	}{
		{
			// Also the empty box under a running turn: nothing pending.
			name: "box clears at once", text: shortText,
			boxes:    join(settled(shortText), []inputBoxAnswer{box("")}),
			wantKeys: keys("Escape", "Enter"), wantSleeps: []time.Duration{ms(100), ms(300)},
		},
		{
			name: "box holds the text once then clears", text: shortText,
			boxes:    join(settled(shortText), []inputBoxAnswer{box(shortText), box("")}),
			wantKeys: keys("Escape", "Enter", "Enter"), wantSleeps: []time.Duration{ms(100), ms(300), ms(600)},
		},
		{
			name: "box holds a long text's wrapped draft once then clears", text: longText,
			boxes:    join(settled(longText), []inputBoxAnswer{box(longText), box("")}),
			wantKeys: keys("Escape", "Enter", "Enter"),
		},
		{
			name: "box changes between reads and gets no Enter until two reads agree", text: shortText,
			boxes:    []inputBoxAnswer{box("r"), box("run"), box(shortText), box(shortText), box("")},
			wantKeys: keys("Escape", "Enter"), wantSleeps: []time.Duration{ms(100), ms(100), ms(100), ms(300)},
		},
		{
			name: "box that never settles fails with no Enter", text: shortText,
			settleMS: 400, confirmS: 1,
			boxes:   []inputBoxAnswer{box("a"), box("b"), box("c"), box("d")},
			wantErr: "still changing", wantNotLand: true,
			wantKeys: keys("Escape"), wantSleeps: []time.Duration{ms(400), ms(400), ms(400)},
		},
		{
			name: "box that settles only after the window closes fails with no Enter", text: shortText,
			settleMS: 400, confirmS: 1,
			boxes:   []inputBoxAnswer{box("a"), box("b"), box("c"), box("c")},
			wantErr: "settled only after the 1s submit window closed", wantNotLand: true,
			wantKeys: keys("Escape"), wantSleeps: []time.Duration{ms(400), ms(400), ms(400)},
		},
		{
			name: "Enters at a growing interval until the window closes", text: shortText,
			boxes:   join(settled(shortText), []inputBoxAnswer{box(shortText)}),
			wantErr: "still pending in the input box", wantNotLand: true,
			wantKeys: keys("Escape", "Enter", "Enter", "Enter", "Enter", "Enter", "Enter", "Enter", "Enter", "Enter", "Enter"),
			wantSleeps: []time.Duration{
				ms(100), ms(300), ms(600), ms(1200), ms(2400), ms(4800),
				confirmBackoffCap, confirmBackoffCap, confirmBackoffCap, confirmBackoffCap, ms(600),
			},
		},
		{
			name: "an Enter at the window's close is followed by one read and no further Enter", text: shortText,
			confirmS: 1,
			boxes:    join(settled(shortText), []inputBoxAnswer{box(shortText)}),
			wantErr:  "still pending in the input box", wantNotLand: true,
			wantKeys: keys("Escape", "Enter", "Enter"), wantSleeps: []time.Duration{ms(100), ms(300), ms(600)},
		},
		{
			// A 400 ms settle in a 1 s window allows four reads.
			name: "a box that never settles under a clock that never advances stops at the read count", text: shortText,
			settleMS: 400, confirmS: 1, frozen: true,
			boxes:   []inputBoxAnswer{box("a"), box("b"), box("c"), box("d"), box("e")},
			wantErr: "after 4 read(s)", wantNotLand: true,
			wantKeys: keys("Escape"),
		},
		{
			// A 1 s window allows six Enters at the 250 ms floor.
			name: "Enters under a clock that never advances stop at the Enter count", text: shortText,
			confirmS: 1, frozen: true,
			boxes:   join(settled(shortText), []inputBoxAnswer{box(shortText)}),
			wantErr: "after 6 Enter(s)", wantNotLand: true,
			wantKeys: keys("Escape", "Enter", "Enter", "Enter", "Enter", "Enter", "Enter"),
		},
		{
			name: "a send whose window closed before typing began says nothing was typed", text: shortText,
			pastDeadline: true,
			boxes:        []inputBoxAnswer{box("")},
			wantErr:      "had closed before typing began", wantNotLand: true, wantAbsent: "never appeared",
		},
		{
			name: "a turn start past the pre-send offset confirms despite an ambiguous box", text: shortText,
			idle: true, startOnEnter: true,
			boxes:    join(settled(shortText), []inputBoxAnswer{box(shortText)}),
			wantKeys: keys("Escape", "Enter"), wantSleeps: []time.Duration{ms(100), ms(300)},
		},
		{
			name: "box holds a draft of other text", text: shortText,
			boxes:    join(settled(shortText), []inputBoxAnswer{box("something else entirely")}),
			wantKeys: keys("Escape", "Enter"),
		},
		{
			name: "box holds a longer draft containing the short sent text", text: shortText,
			boxes:    join(settled(shortText), []inputBoxAnswer{box("run the suite and then deploy")}),
			wantKeys: keys("Escape", "Enter"),
		},
		{
			name: "box holds a collapsed paste placeholder", text: longText,
			boxes:    join(settled(longText), []inputBoxAnswer{box(placeholder)}),
			wantKeys: keys("Escape", "Enter"),
		},
		{
			name: "no readable box", text: shortText,
			boxes:    []inputBoxAnswer{{"", false}},
			wantKeys: keys("Escape", "Enter"),
		},
		{
			name: "an empty box before the clear means the submission landed late", text: shortText,
			idle: true, confirmS: 1,
			boxes:    join(settled(shortText), []inputBoxAnswer{box(shortText), box(shortText), box("")}),
			wantKeys: keys("Escape", "Enter", "Enter"),
		},
		{
			// The box merely contains the sent text's first 48 characters, which no longer count as the sent text.
			name: "a longer draft containing a long sent text's start is not held", text: longText,
			boxes:    join(settled(longText), []inputBoxAnswer{box(longText + " " + longText)}),
			wantKeys: keys("Escape", "Enter"),
		},
		{
			name: "a paste placeholder is left in place by the clear", text: longText,
			idle: true, captures: []string{"IDLE"},
			boxes:   []inputBoxAnswer{box(placeholder), box("")},
			wantErr: "not this send's", wantNotLand: true,
			wantKeys: keys("Escape", "Escape"),
		},
		{
			name: "a foreign draft gets no clear key", text: shortText,
			idle: true, captures: []string{"IDLE"},
			boxes:   []inputBoxAnswer{box("somebody else's draft")},
			wantErr: "not this send's", wantNotLand: true,
			wantKeys: keys("Escape", "Escape"),
		},
		{
			name: "an empty box after a never-appeared text means the submission landed late", text: shortText,
			idle: true, captures: []string{"IDLE"},
			boxes:    []inputBoxAnswer{box("")},
			wantKeys: keys("Escape", "Escape"),
		},
		{
			name: "an engine without the idle reading gets no clear", text: shortText,
			captures: []string{"❯ "},
			boxes:    []inputBoxAnswer{box(shortText)},
			wantErr:  "never appeared", wantNotLand: true,
			wantKeys: keys("Escape", "Escape"),
		},
		{
			// A 1 s window closes inside the first typing's polls, so there is no replay.
			name: "a text that has not appeared when the window closes names the window and the one typing", text: shortText,
			confirmS: 1, captures: []string{"❯ "},
			boxes:   []inputBoxAnswer{box(shortText)},
			wantErr: "after 1 typing(s) — the 1s submit window, cut to the send's deadline, closed before it appeared", wantNotLand: true,
			wantKeys: keys("Escape"),
		},
		{
			name: "a box the clear did not empty is named in the error", text: shortText,
			idle: true, captures: []string{"IDLE"},
			boxes:   []inputBoxAnswer{box(shortText)},
			wantErr: "after the clear", wantNotLand: true,
			wantKeys: keys("Escape", "Escape", "C-u"),
		},
		{
			name: "the error ends with the pane's last 15 non-blank lines", text: shortText,
			captures: []string{tallFrame},
			boxes:    []inputBoxAnswer{box("unrelated")},
			wantErr:  "row-20", wantNotLand: true, wantAbsent: "row-05",
			wantKeys: keys("Escape", "Escape"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			captures := tt.captures
			if captures == nil {
				prefix := "❯ "
				if tt.idle {
					prefix = "IDLE "
				}
				captures = []string{strings.TrimSpace(prefix), strings.TrimSpace(prefix), prefix + tt.text}
			}
			var run *Run
			reed := &enterHookReed{fakeReed: &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: captures}}
			if tt.startOnEnter {
				reed.onEnter = func() {
					f, err := os.OpenFile(run.state.EventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
					if err != nil {
						t.Errorf("open events: %v", err)
						return
					}
					defer f.Close()
					if _, err := f.WriteString("START\n"); err != nil {
						t.Errorf("append turn start: %v", err)
					}
				}
			}
			base := &inputBoxEngine{fakeEngine: readyAgentEngine(), boxes: tt.boxes, settle: ms(300)}
			var engine Engine = base
			if tt.idle {
				engine = &clearableBoxEngine{inputBoxEngine: base}
			}
			cfg := Config{SubmitSettleMS: 100, SubmitConfirmTimeoutS: 30}
			if tt.settleMS != 0 {
				cfg.SubmitSettleMS = tt.settleMS
			}
			if tt.confirmS != 0 {
				cfg.SubmitConfirmTimeoutS = tt.confirmS
			}
			clock := &boxTestClock{fakeClock: newFakeClock(time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC))}
			var runClock Clock = clock
			if tt.frozen {
				runClock = &frozenClock{now: clock.Now()}
			}
			deadline := clock.Now().Add(time.Hour)
			if tt.pastDeadline {
				deadline = clock.Now().Add(-time.Second)
			}
			run = newFixture(t, reed, engine, withConfig(cfg)).newRun(Spec{}, withRunEvents(""), withRunClock(runClock, deadline))

			send := run.Send
			if tt.pastDeadline {
				send = run.sendWithin
			}
			err := send(tt.text)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Send() error: %v, want the send confirmed", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Send() error = %v, want it to name %q", err, tt.wantErr)
			}
			if got := errors.Is(err, ErrSubmissionNotLanded); got != tt.wantNotLand {
				t.Errorf("errors.Is(err, ErrSubmissionNotLanded) = %v, want %v (err %v)", got, tt.wantNotLand, err)
			}
			if tt.wantErr != "" && IsDraftHold(err) {
				t.Errorf("IsDraftHold(err) = true for a send that did not land (err %v)", err)
			}
			var gotKeys []string
			for _, call := range reed.SendKeyCalls {
				gotKeys = append(gotKeys, call.Key)
			}
			if !reflect.DeepEqual(gotKeys, tt.wantKeys) {
				t.Errorf("keys = %v, want %v", gotKeys, tt.wantKeys)
			}
			if tt.wantSleeps != nil && !reflect.DeepEqual(clock.sleeps, tt.wantSleeps) {
				t.Errorf("sleeps = %v, want %v", clock.sleeps, tt.wantSleeps)
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), "The pane's last lines:") {
				t.Errorf("error %q does not end with the pane's last lines", err)
			}
			if tt.wantAbsent != "" && strings.Contains(err.Error(), tt.wantAbsent) {
				t.Errorf("error %q names %q, want only the pane's last 15 non-blank lines", err, tt.wantAbsent)
			}
		})
	}

	// The paste path: an engine with TextPaster is written to through one reed paste-submit, never typed keys.
	const shortPasted = "run the suite"
	const longPasted = "please review the whole change set carefully"
	pasteTests := []struct {
		name string
		text string
		// pasteSubmits is the engine's PasteSubmits answer.
		pasteSubmits bool
		// swallowFor is how long after the start the pane swallows an Enter.
		swallowFor time.Duration
		// active scripts the operator-activity answers, the idle wait's own reading first.
		active []bool
		// scripted, when set, replaces the tui pane with a fixed idle capture and these input-box answers.
		scripted []inputBoxAnswer
		wantErr  string
		wantIs   []error
		// wantDraft is whether IsDraftHold reports the error.
		wantDraft bool
		// wantNotLand is whether the error wraps ErrSubmissionNotLanded.
		wantNotLand bool
		wantKeys    []string
		wantSubmit  bool
		// strayBox starts the tui pane's box on a lone stray character the clear leaves on screen, which the send then pastes beside.
		strayBox string
		// wantRecord asserts the failure recorded the pending send.
		wantRecord bool
	}{
		{
			name: "a residue character beside the pasted text fails without a key of its own and records the send", text: shortPasted, pasteSubmits: true,
			strayBox: "+", swallowFor: ms(500),
			wantErr: "+run the suite", wantIs: []error{ErrSentTextInDraft, ErrSessionBusy}, wantDraft: true, wantKeys: keys("C-u"), wantSubmit: true, wantRecord: true,
		},
		{name: "one paste-submit and no typed keys", text: shortPasted, pasteSubmits: true, wantSubmit: true},
		{name: "a swallowed Enter leaves the exact text and gets one guarded Enter", text: shortPasted, pasteSubmits: true, swallowFor: ms(50), wantKeys: keys("Enter"), wantSubmit: true},
		{name: "a long text's placeholder box gets the guarded Enter", text: longPasted, pasteSubmits: true, swallowFor: ms(50), wantKeys: keys("Enter"), wantSubmit: true},
		{
			name: "a placeholder box for a short text is a foreign draft", text: shortPasted, pasteSubmits: true,
			scripted: []inputBoxAnswer{box(pastePlaceholder)},
			wantErr:  "not this send's", wantIs: []error{ErrForeignBoxText, ErrSessionBusy}, wantDraft: true, wantSubmit: true,
		},
		{
			name: "an active operator before the guarded Enter gets no Enter that read", text: shortPasted, pasteSubmits: true, swallowFor: ms(50),
			active:   []bool{false, true, false},
			wantKeys: keys("Enter"), wantSubmit: true,
		},
		{
			name: "the sent text beside a draft fails without a key", text: shortPasted, pasteSubmits: true,
			scripted: []inputBoxAnswer{box("my own draft " + shortPasted)},
			wantErr:  "my own draft", wantIs: []error{ErrSentTextInDraft, ErrSessionBusy}, wantDraft: true, wantSubmit: true,
		},
		{
			name: "a foreign draft fails without a key and quotes the draft", text: shortPasted, pasteSubmits: true,
			scripted: []inputBoxAnswer{box("somebody else's draft")},
			wantErr:  "somebody else's draft", wantIs: []error{ErrForeignBoxText, ErrSessionBusy}, wantDraft: true, wantSubmit: true,
		},
		{
			name: "a box that stays unreadable through the window fails without a key", text: shortPasted, pasteSubmits: true,
			scripted: []inputBoxAnswer{{"", false}},
			wantErr:  "unreadable", wantIs: []error{ErrSubmissionNotLanded}, wantNotLand: true, wantSubmit: true,
		},
		{name: "a paste without the Enter settles, then sends the guarded Enter", text: shortPasted, wantKeys: keys("Enter")},
	}
	for _, tt := range pasteTests {
		t.Run("paste: "+tt.name, func(t *testing.T) {
			t.Parallel()
			clock := newFakeClock(time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC))
			base := &fakeReed{StatusQueue: liveStrandStatus(true), ActiveQueue: tt.active}
			var reed ReedOps
			var engine Engine
			if tt.scripted != nil {
				base.CaptureQueue = []string{"IDLE"}
				reed = base
				engine = &scriptedPasteEngine{
					clearableBoxEngine: &clearableBoxEngine{inputBoxEngine: &inputBoxEngine{fakeEngine: readyAgentEngine(), boxes: tt.scripted, settle: ms(100)}},
					pasteSubmits:       tt.pasteSubmits,
				}
			} else if tt.strayBox != "" {
				reed = &tuiReed{fakeReed: base, clock: clock, swallowEnterUntil: clock.Now().Add(tt.swallowFor), box: tt.strayBox, keepOnClear: true}
				engine = &strayPasteEngine{pasteEngine: &pasteEngine{tuiEngine: newTUIEngine(), pasteSubmits: tt.pasteSubmits}}
			} else {
				reed = &tuiReed{fakeReed: base, clock: clock, swallowEnterUntil: clock.Now().Add(tt.swallowFor)}
				engine = &pasteEngine{tuiEngine: newTUIEngine(), pasteSubmits: tt.pasteSubmits}
			}
			cfg := Config{SubmitSettleMS: 100, SubmitConfirmTimeoutS: 30, SendReadyTimeoutS: 60}
			run := newFixture(t, reed, engine, withConfig(cfg)).newRun(Spec{}, withRunEvents(""), withRunClock(clock, clock.Now().Add(time.Hour)))

			err := run.Send(tt.text)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Send() error: %v, want the paste confirmed", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Send() error = %v, want it to name %q", err, tt.wantErr)
			}
			for _, want := range tt.wantIs {
				if !errors.Is(err, want) {
					t.Errorf("Send() error = %v, want it to wrap %v", err, want)
				}
			}
			if got := errors.Is(err, ErrSubmissionNotLanded); got != tt.wantNotLand {
				t.Errorf("errors.Is(err, ErrSubmissionNotLanded) = %v, want %v (err %v)", got, tt.wantNotLand, err)
			}
			if got := IsDraftHold(err); got != tt.wantDraft {
				t.Errorf("IsDraftHold(err) = %v, want %v (err %v)", got, tt.wantDraft, err)
			}
			if len(base.SendTextCalls) != 0 {
				t.Errorf("SendText calls = %+v, want none on the paste path", base.SendTextCalls)
			}
			if len(base.PasteSubmitCalls) != 1 || base.PasteSubmitCalls[0].Text != tt.text || base.PasteSubmitCalls[0].Submit != tt.wantSubmit {
				t.Errorf("PasteSubmit calls = %+v, want one of %q with submit %v", base.PasteSubmitCalls, tt.text, tt.wantSubmit)
			}
			var gotKeys []string
			for _, call := range base.SendKeyCalls {
				gotKeys = append(gotKeys, call.Key)
			}
			if !reflect.DeepEqual(gotKeys, tt.wantKeys) {
				t.Errorf("keys = %v, want %v", gotKeys, tt.wantKeys)
			}
			if tt.wantRecord {
				if _, recorded, loadErr := loadPendingSend(run.runDir); loadErr != nil || !recorded {
					t.Errorf("pending send recorded = %v (error %v), want the failed send recorded", recorded, loadErr)
				}
			}
		})
	}

	// The retry of a paste that may have left its text in the box: the pending-send record decides whether the second send types.
	const retriedText = "run the suite"
	retryTests := []struct {
		name string
		// firstBox is the box the first send ends on; wantRecord is whether that failure records the text.
		firstBox   inputBoxAnswer
		wantRecord bool
		// turnStarted appends a turn start to the events file between the sends.
		turnStarted bool
		// retryText is the text of the second send.
		retryText string
		// wantPastes is how many pastes both sends made together.
		wantPastes int
	}{
		{name: "a landed send leaves no record", firstBox: box(""), retryText: retriedText, wantPastes: 2},
		{
			name:     "text left in the draft with a turn started since types nothing on the retry",
			firstBox: box("my own draft " + retriedText), wantRecord: true, turnStarted: true, retryText: retriedText, wantPastes: 1,
		},
		{
			name:     "text left in the draft and cleared by the operator is pasted again",
			firstBox: box("my own draft " + retriedText), wantRecord: true, retryText: retriedText, wantPastes: 2,
		},
		{
			name:     "a send not landed after an unreadable box with a turn started since types nothing on the retry",
			firstBox: inputBoxAnswer{"", false}, wantRecord: true, turnStarted: true, retryText: retriedText, wantPastes: 1,
		},
		{
			name:     "a record for another text is cleared and the retry pastes",
			firstBox: box("my own draft " + retriedText), wantRecord: true, turnStarted: true, retryText: "a different request", wantPastes: 2,
		},
	}
	for _, tt := range retryTests {
		t.Run("retry: "+tt.name, func(t *testing.T) {
			t.Parallel()
			clock := newFakeClock(time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC))
			base := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"IDLE"}}
			boxEngine := &inputBoxEngine{fakeEngine: readyAgentEngine(), boxes: []inputBoxAnswer{tt.firstBox}, settle: ms(100)}
			engine := &scriptedPasteEngine{clearableBoxEngine: &clearableBoxEngine{inputBoxEngine: boxEngine}, pasteSubmits: true}
			cfg := Config{SubmitSettleMS: 100, SubmitConfirmTimeoutS: 30, SendReadyTimeoutS: 60}
			run := newFixture(t, base, engine, withConfig(cfg)).newRun(Spec{}, withRunEvents(""), withRunClock(clock, clock.Now().Add(time.Hour)))

			_ = run.Send(retriedText)
			_, recorded, err := loadPendingSend(run.runDir)
			if err != nil {
				t.Fatalf("loadPendingSend() error: %v", err)
			}
			if recorded != tt.wantRecord {
				t.Fatalf("pending send recorded = %v after the first send, want %v", recorded, tt.wantRecord)
			}

			if tt.turnStarted {
				f, err := os.OpenFile(run.state.EventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					t.Fatalf("open events: %v", err)
				}
				defer f.Close()
				if _, err := f.WriteString("START\nSTOP:done\n"); err != nil {
					t.Fatalf("append turn: %v", err)
				}
			}
			boxEngine.boxes = []inputBoxAnswer{box("")}
			if err := run.Send(tt.retryText); err != nil {
				t.Fatalf("retry Send() error: %v, want the retry confirmed", err)
			}
			if len(base.PasteSubmitCalls) != tt.wantPastes {
				t.Errorf("PasteSubmit calls = %+v, want %d", base.PasteSubmitCalls, tt.wantPastes)
			}
			if _, stillRecorded, _ := loadPendingSend(run.runDir); stillRecorded {
				t.Error("pending send still recorded after the retry, want it cleared")
			}
		})
	}
}

//testtiming:keep pins the pane scan's count and lines-below at the unit level, including a needle straddling a wrap boundary, which the delivery table reaches only through Send
func TestScanPaneForNeedle(t *testing.T) {
	tests := []struct {
		name           string
		capture        string
		needle         string
		wantCount      int
		wantLinesBelow int
	}{
		{"absent", "❯ nothing here\n", "doit", 0, -1},
		{"single occurrence at the last content line", "one\ntwo\n❯ do it", "doit", 1, 0},
		{"two occurrences, position taken from the last", "❯ do it\nfiller\n❯ do it\nfiller", "doit", 2, 1},
		{"trailing blank lines are not content", "❯ do it\n\n\n", "doit", 1, 0},
		{
			// Matching runs over the whole normalized capture, so a needle split across a wrap
			// boundary still counts, and its position is the line the match ENDS on.
			name: "needle straddling a wrap boundary", capture: "❯ do i\nt now\nafter", needle: "doit", wantCount: 1, wantLinesBelow: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scanPaneForNeedle(tt.capture, tt.needle)
			if got.count != tt.wantCount || got.linesBelow != tt.wantLinesBelow {
				t.Errorf("scanPaneForNeedle(%q, %q) = {count:%d linesBelow:%d}; want {count:%d linesBelow:%d}",
					tt.capture, tt.needle, got.count, got.linesBelow, tt.wantCount, tt.wantLinesBelow)
			}
		})
	}
}

// TestSend_WaitsForIdleSession drives the idle wait of Run.Send, and of Runner.Send in one row, over an engine with the idle reading.
// The pane turns idle at a fake-clock time, or never; an unmatched turn start in the events file holds a send against an idle pane until the override, an interrupt report or a later turn end releases it.
// Nothing is typed before the wait ends, and a session that stays busy fails with ErrSessionBusy carrying the pane's last lines.
//
// It is not parallel: logcapture redirects the process-global logger.
func TestSend_WaitsForIdleSession(t *testing.T) {
	const never = 24 * time.Hour
	tests := []struct {
		name string
		// idleAfter is when the pane turns idle, from the start; never keeps it busy.
		idleAfter time.Duration
		busyFrame string
		// failCaptureAfter, when positive, fails every pane capture from that offset.
		failCaptureAfter time.Duration
		events           string
		interrupted      bool
		// progress is what the engine reads of any turn start.
		progress TurnProgress
		// reading gives the engine a live process, so the hook-derived readiness reading decides instead of the pane;
		// reader also gives it an input-box reader, for a row that fails before anything is typed.
		reading, reader bool
		viaRunner       bool
		// activeQueue and activityErr script the fake reed's operator-activity answers.
		activeQueue []bool
		activityErr error
		// activityErrQueue scripts the fake reed's operator-activity failures per query, FIFO with the last entry sticking; a nil entry answers from activeQueue.
		activityErrQueue []error
		// sends is how many sends the row makes through one run, one when zero, so the activity queries span several waits of one runner.
		sends int
		// frozen gives the run a clock that never advances, so only the poll count can end the wait.
		frozen bool
		// wantTypedBetween is the window the first typed key lands in; ignored when wantBusy is set.
		wantTypedMin, wantTypedMax time.Duration
		wantBusy                   []string
		// wantReason, when set, is the prefix the busy error's Reason must carry.
		wantReason string
		// wantDraft is whether the busy error names a draft in the input box, checked through SessionBusyError.Draft and IsDraftHold.
		wantDraft bool
		wantWarns int
		// wantActivityWarns counts the WARN lines about an unreadable operator-activity query across the row's polls, and wantActivityRecoveries the INFO lines saying it reads again.
		wantActivityWarns, wantActivityRecoveries int
		// stray, when set, drives the row through a pane holding a lone stray character instead.
		stray *strayRow
	}{
		{name: "stray character cleared and the send goes out, on the hook reading", stray: &strayRow{box: "+", hook: true, wantClears: 1}},
		{name: "stray character cleared and the send goes out, on the pane reading", stray: &strayRow{box: "+", wantClears: 1}},
		{name: "stray character left on screen by the clear is residue, on the hook reading", stray: &strayRow{box: "+", hook: true, keepOnClear: true, wantClears: 1}},
		{name: "stray character left on screen by the clear is residue, on the pane reading", stray: &strayRow{box: "+", keepOnClear: true, wantClears: 1}},
		{name: "operator activity in the settle holds at the post-clear reading", stray: &strayRow{box: "+", hook: true, active: []bool{false, true}, wantClears: 1, wantBusy: []string{"the operator typed in this pane"}}},
		{name: "a quiet operator at the next reading sends the residue with no second clear", stray: &strayRow{box: "+", hook: true, keepOnClear: true, active: []bool{false, true, false}, wantClears: 1}},
		{name: "a lone stray character beside an active operator holds with the box quoted", stray: &strayRow{box: "+", active: []bool{true}, wantBusy: []string{"the operator typed in this pane", `the input box holds "+"`}, wantDraft: true}},
		{name: "a two-character box holds as a draft on the hook reading", stray: &strayRow{box: "++", hook: true, wantBusy: []string{"the input box holds a draft", "++"}, wantDraft: true}},
		{name: "a two-character box holds with the box quoted on the pane reading", stray: &strayRow{box: "++", wantBusy: []string{"the pane is not idle", `the input box holds "++"`}, wantDraft: true}},
		{name: "idle pane types at once", wantTypedMax: 0},
		{name: "running turn turns idle then types", idleAfter: 3 * time.Second, busyFrame: "working (esc to interrupt)", wantTypedMin: 3 * time.Second, wantTypedMax: 5 * time.Second},
		{name: "runner send waits as run send does", idleAfter: 3 * time.Second, busyFrame: "working (esc to interrupt)", viaRunner: true, wantTypedMin: 3 * time.Second, wantTypedMax: 5 * time.Second},
		{name: "draft that never clears fails busy", reader: true, idleAfter: never, busyFrame: "earlier output\n❯ a half-typed draft", wantBusy: []string{"the pane is not idle", "a half-typed draft"}, wantDraft: true},
		{name: "failed final capture is said so", idleAfter: never, busyFrame: "working", failCaptureAfter: 59 * time.Second, wantBusy: []string{"the final pane capture failed"}},
		{name: "a busy pane under a clock that never advances fails busy at the poll count", idleAfter: never, busyFrame: "working", frozen: true, wantBusy: []string{"the pane is not idle"}},
		{name: "unmatched turn start is released by the idle override", events: "START\n", reading: true, wantTypedMin: turnStartIdleOverride, wantTypedMax: 15 * time.Second, wantWarns: 1},
		{name: "unmatched turn start is released by an interrupt report", events: "START\n", reading: true, interrupted: true},
		{name: "unmatched turn start against a busy pane fails busy with the reading's reason", events: "START\n", reading: true, idleAfter: never, busyFrame: "working", wantBusy: []string{"a turn is running"}, wantReason: "a turn is running"},
		{name: "draft in the box fails busy with the reading's reason", events: "START\nSTOP:done\n", reading: true, reader: true, idleAfter: never, busyFrame: "earlier output\n❯ a half-typed draft", wantBusy: []string{"the input box holds a draft", "a half-typed draft"}, wantReason: "the input box holds a draft", wantDraft: true},
		{name: "an active operator holds the write until it goes quiet", activeQueue: []bool{true, true, true, false}, wantTypedMin: time.Second, wantTypedMax: 3 * time.Second},
		{name: "an operator active through the whole window fails busy naming the activity", activeQueue: []bool{true}, wantBusy: []string{"the operator typed in this pane within the last 5 s"}, wantReason: "the operator typed in this pane"},
		{name: "an unreadable activity query still holds on a draft", activityErr: errors.New("list-clients failed"), events: "START\nSTOP:done\n", reading: true, reader: true, idleAfter: never, busyFrame: "earlier output\n❯ a half-typed draft", wantBusy: []string{"the input box holds a draft", "a half-typed draft"}, wantDraft: true},
		{name: "an unreadable activity query holds nothing over an empty box", activityErr: errors.New("list-clients failed"), events: "START\nSTOP:done\n", reading: true, reader: true, idleAfter: time.Millisecond, busyFrame: "IDLE\n❯ ", wantActivityWarns: 1},
		{
			name:             "an activity query that reads again after failing logs its recovery",
			activityErrQueue: []error{errors.New("list-clients failed"), errors.New("list-clients failed"), nil},
			sends:            3, wantActivityWarns: 1, wantActivityRecoveries: 1,
		},
		{name: "turn end after the turn start releases at once", events: "START\nSTOP:done\n"},
		{name: "api error turn end counts as a turn end", events: "START\nAPIERR\n"},
		{name: "a hook turn end beside a pane needle types at once", events: "START\nSTOP:done\n", reading: true, idleAfter: time.Second, busyFrame: "working (esc to interrupt)\n❯ "},
		{name: "unmatched turn start under an unknown reading is released by the idle override", events: "START\n", wantTypedMin: turnStartIdleOverride, wantTypedMax: 15 * time.Second, wantWarns: 1},
		{name: "an in-flight turn start holds an idle pane past the override", events: "START\n", reading: true, progress: TurnInFlight, wantBusy: []string{"a turn is running"}, wantReason: "a turn is running"},
		{name: "an in-flight turn start under an unknown reading holds past the override", events: "START\n", progress: TurnInFlight, wantBusy: []string{"has no turn end"}},
		{name: "a reported interrupt releases an in-flight turn start at once", events: "START\n", reading: true, interrupted: true, progress: TurnInFlight},
		{name: "an over reading still releases by the idle override", events: "START\n", reading: true, progress: TurnOver, wantTypedMin: turnStartIdleOverride, wantTypedMax: 15 * time.Second, wantWarns: 1},
		{name: "a session start after an in-flight turn start releases by the idle override", events: "START\nSESSIONSTART:resume\n", reading: true, progress: TurnInFlight, wantTypedMin: turnStartIdleOverride, wantTypedMax: 15 * time.Second, wantWarns: 1},
		{name: "a session start after an in-flight turn start under an unknown reading releases by the idle override", events: "START\nSESSIONSTART:resume\n", progress: TurnInFlight, wantTypedMin: turnStartIdleOverride, wantTypedMax: 15 * time.Second, wantWarns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.stray != nil {
				checkStraySend(t, tt.stray)
				return
			}
			buf := logcapture.CaptureVerbose(t)
			clock := newFakeClock(time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC))
			start := clock.Now()
			reed := &idleReed{fakeReed: &fakeReed{StatusQueue: liveStrandStatus(true), ActiveQueue: tt.activeQueue, ActivityErr: tt.activityErr, ActivityErrQueue: tt.activityErrQueue}, clock: clock, busyFrame: tt.busyFrame}
			if tt.idleAfter > 0 {
				reed.idleAt = start.Add(tt.idleAfter)
			}
			if tt.failCaptureAfter > 0 {
				reed.failAt = start.Add(tt.failCaptureAfter)
			}
			var engine Engine
			idle := &idleEngine{}
			engine = idle
			if tt.reader {
				box := &readinessEngine{}
				idle = &box.idleEngine
				engine = box
			}
			idle.StartupScript = []StartupState{StartupReady}
			idle.interrupted = tt.interrupted
			idle.progress = tt.progress
			if tt.reading {
				idle.liveness = LivenessAlive
			}
			cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5, SendReadyTimeoutS: 60}

			var err error
			if tt.viaRunner {
				fx := newFixture(t, reed, engine, withConfig(cfg), withClock(clock), withStrand("strand-1"))
				err = fx.Runner.Send("strand-1", "hello")
			} else {
				var runClock Clock = clock
				if tt.frozen {
					runClock = &frozenClock{now: start}
				}
				run := newFixture(t, reed, engine, withConfig(cfg)).newRun(Spec{}, withRunEvents(tt.events), withRunClock(runClock, start.Add(time.Hour)))
				for i := range max(tt.sends, 1) {
					// A distinct text per send, so each one's appearance in the pane is its own.
					text := "hello"
					if i > 0 {
						text = fmt.Sprintf("hello again %d", i)
					}
					if err = run.Send(text); err != nil {
						break
					}
				}
			}

			if got := countLevelLines(buf.String(), "WARN", "operator activity unreadable"); got != tt.wantActivityWarns {
				t.Errorf("operator activity WARN lines = %d, want %d; log: %s", got, tt.wantActivityWarns, buf.String())
			}
			if got := countLevelLines(buf.String(), "INFO", "operator activity readable again"); got != tt.wantActivityRecoveries {
				t.Errorf("operator activity recovery INFO lines = %d, want %d; log: %s", got, tt.wantActivityRecoveries, buf.String())
			}

			if tt.wantBusy != nil {
				if !errors.Is(err, ErrSessionBusy) {
					t.Fatalf("Send error = %v, want ErrSessionBusy", err)
				}
				var busy *SessionBusyError
				if !errors.As(err, &busy) {
					t.Fatalf("Send error = %T, want a *SessionBusyError", err)
				}
				if busy.Reason == "" || strings.Contains(busy.Reason, "The pane's last lines") {
					t.Errorf("busy Reason = %q, want a non-empty reason without the pane tail", busy.Reason)
				}
				if tt.wantReason != "" && !strings.HasPrefix(busy.Reason, tt.wantReason) {
					t.Errorf("busy Reason = %q, want prefix %q", busy.Reason, tt.wantReason)
				}
				if busy.Draft != tt.wantDraft || IsDraftHold(err) != tt.wantDraft {
					t.Errorf("busy Draft = %v and IsDraftHold = %v, want both %v", busy.Draft, IsDraftHold(err), tt.wantDraft)
				}
				for _, want := range tt.wantBusy {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Send error = %q, want it to contain %q", err, want)
					}
				}
				if len(reed.SendKeyCalls)+len(reed.SendTextCalls) != 0 {
					t.Errorf("typed keys %v and text %v into a busy session, want nothing", reed.SendKeyCalls, reed.SendTextCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			typedAfter := reed.typedAt.Sub(start)
			if typedAfter < tt.wantTypedMin || typedAfter > tt.wantTypedMax {
				t.Errorf("first text typed %v after the start, want within [%v, %v]", typedAfter, tt.wantTypedMin, tt.wantTypedMax)
			}
			if got := strings.Count(buf.String(), "released an unmatched turn start"); got != tt.wantWarns {
				t.Errorf("release warnings = %d, want %d; log: %s", got, tt.wantWarns, buf.String())
			}
		})
	}
}

// strayRow is one Send into a pane whose input box starts holding box, driven over a strayPasteEngine.
// hook gives the engine a live process and a finished turn, so the hook-derived reading decides; otherwise the pane does.
// keepOnClear makes the clear leave the box as it is, active scripts the operator-activity answers, and wantClears is how many C-u keys the send plays.
// A row with wantBusy fails with ErrSessionBusy naming each fragment and pastes nothing; any other row pastes "hello" once.
type strayRow struct {
	box         string
	hook        bool
	keepOnClear bool
	active      []bool
	wantClears  int
	wantBusy    []string
	// wantDraft is whether the busy error names a draft in the input box.
	wantDraft bool
}

// checkStraySend runs row's Send and checks its outcome.
func checkStraySend(t *testing.T, row *strayRow) {
	t.Helper()
	clock := newFakeClock(time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC))
	base := &fakeReed{StatusQueue: liveStrandStatus(true), ActiveQueue: row.active}
	reed := &tuiReed{fakeReed: base, clock: clock, box: row.box, keepOnClear: row.keepOnClear}
	engine := &strayPasteEngine{pasteEngine: &pasteEngine{tuiEngine: newTUIEngine(), pasteSubmits: true}, alive: row.hook}
	events := ""
	if row.hook {
		events = "START\nSTOP:done\n"
	}
	cfg := Config{SubmitSettleMS: 100, SubmitConfirmTimeoutS: 30, SendReadyTimeoutS: 60}
	run := newFixture(t, reed, engine, withConfig(cfg)).newRun(Spec{},
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1"}),
		withRunEvents(events), withRunClock(clock, clock.Now().Add(time.Hour)))

	err := run.Send("hello")
	if reed.clears != row.wantClears {
		t.Errorf("C-u keys = %d, want %d", reed.clears, row.wantClears)
	}
	if row.wantBusy != nil {
		if !errors.Is(err, ErrSessionBusy) {
			t.Fatalf("Send error = %v, want ErrSessionBusy", err)
		}
		var busy *SessionBusyError
		if !errors.As(err, &busy) || busy.Draft != row.wantDraft || IsDraftHold(err) != row.wantDraft {
			t.Errorf("Send error %v: Draft and IsDraftHold = %v, want both %v", err, IsDraftHold(err), row.wantDraft)
		}
		for _, want := range row.wantBusy {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Send error = %q, want it to contain %q", err, want)
			}
		}
		if len(base.PasteSubmitCalls) != 0 {
			t.Errorf("PasteSubmit calls = %+v, want none for a held send", base.PasteSubmitCalls)
		}
		return
	}
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(base.PasteSubmitCalls) != 1 || base.PasteSubmitCalls[0].Text != "hello" {
		t.Errorf("PasteSubmit calls = %+v, want one of the exact text", base.PasteSubmitCalls)
	}
}

// paneReed is a fakeReed whose pane shows every text typed so far, so each Send's delivery check passes whatever order sends arrive in.
// Every call yields the processor first, so an unordered pair of sends would interleave.
type paneReed struct {
	*fakeReed
}

func (p *paneReed) SendKey(guid, key string) error {
	runtime.Gosched()
	return p.fakeReed.SendKey(guid, key)
}

func (p *paneReed) SendText(guid, text string, submit bool) error {
	runtime.Gosched()
	return p.fakeReed.SendText(guid, text, submit)
}

func (p *paneReed) CapturePane(guid string) (string, error) {
	runtime.Gosched()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.CallLog = append(p.CallLog, "CapturePane")
	lines := []string{"idle pane"}
	for _, call := range p.SendTextCalls {
		lines = append(lines, "❯ "+call.Text)
	}
	return strings.Join(lines, "\n"), nil
}

// typedCalls returns the pane-typing calls in the reed's call log, in order.
func (p *paneReed) typedCalls() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var typed []string
	for _, call := range p.CallLog {
		if strings.HasPrefix(call, "SendKey:") || strings.HasPrefix(call, "SendText:") {
			typed = append(typed, call)
		}
	}
	return typed
}

// onceStepClock is a fakeClock that runs step once, on the first Sleep, safe under concurrent sleepers.
type onceStepClock struct {
	*fakeClock
	once sync.Once
	step func()
}

func (c *onceStepClock) Sleep(d time.Duration) {
	c.fakeClock.Sleep(d)
	c.once.Do(c.step)
}

// gatedRunWithNotice drives a gated Wait that fails its gate once and then passes, with notice, when non-empty, sent through Run.Send beside it.
// It returns the Wait result, the reed and the notice's Send error.
func gatedRunWithNotice(t *testing.T, notice string) (Result, *paneReed, error) {
	t.Helper()
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	gateCalls := 0
	gate := func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: gateCalls > 1, Findings: "fix the thing"}, nil
	}
	reed := &paneReed{fakeReed: &fakeReed{StatusQueue: liveStrandStatus(true)}}
	fc := newFakeClock(time.Now())
	clock := &onceStepClock{fakeClock: fc, step: func() { appendEventsLine(t, eventsPath, "STOP:turn2") }}
	run := newFixture(t, reed, readyAgentEngine(), withConfig(gateConfig)).newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour, KeepPane: true},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(clock, fc.Now().Add(time.Hour)),
		withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

	var noticeErr error
	var noticeSent sync.WaitGroup
	if notice != "" {
		noticeSent.Add(1)
		go func() {
			defer noticeSent.Done()
			noticeErr = run.Send(notice)
		}()
	}
	result, err := run.Wait()
	noticeSent.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	return result, reed, noticeErr
}

// TestRun_Send_ConcurrentSendsTypeInSequence pins the per-run send lock: two in-process sends on one run, and a Send beside a gated Wait's re-prompt, type one after the other.
// Each send's Escape and text land together, each delivery verifies without a replay, and the notice costs the gate no attempt.
//
// It is not parallel: stubInputSleep replaces a package-level function.
func TestRun_Send_ConcurrentSendsTypeInSequence(t *testing.T) {
	const notice = "seat notice one"
	assertWholeLines := func(t *testing.T, typed []string, wantTexts int) {
		t.Helper()
		if len(typed) != 2*wantTexts {
			t.Fatalf("typed calls = %q, want %d Escape+text pairs", typed, wantTexts)
		}
		for i := 0; i < len(typed); i += 2 {
			if typed[i] != "SendKey:Escape" || !strings.HasPrefix(typed[i+1], "SendText:") {
				t.Fatalf("typed calls = %q, want each Escape followed at once by its own text", typed)
			}
		}
	}

	t.Run("two concurrent sends", func(t *testing.T) {
		stubInputSleep(t)
		reed := &paneReed{fakeReed: &fakeReed{StatusQueue: liveStrandStatus(true)}}
		clock := newFakeClock(time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC))
		run := newFixture(t, reed, readyAgentEngine(), withConfig(Config{})).newRun(Spec{}, withRunClock(clock, clock.Now().Add(time.Hour)))

		lines := []string{"first line", "second line"}
		errs := make([]error, len(lines))
		var sent sync.WaitGroup
		for i, line := range lines {
			sent.Add(1)
			go func() {
				defer sent.Done()
				errs[i] = run.Send(line)
			}()
		}
		sent.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("Send(%q) error: %v", lines[i], err)
			}
		}
		typed := reed.typedCalls()
		assertWholeLines(t, typed, len(lines))
		if got := []string{typed[1], typed[3]}; !(reflect.DeepEqual(got, []string{"SendText:first line", "SendText:second line"}) ||
			reflect.DeepEqual(got, []string{"SendText:second line", "SendText:first line"})) {
			t.Errorf("typed texts = %q, want both lines once each", got)
		}
	})

	t.Run("a send beside a gated re-prompt", func(t *testing.T) {
		stubInputSleep(t)
		alone, _, _ := gatedRunWithNotice(t, "")
		if alone.Gate == nil || alone.Gate.Attempts != 1 {
			t.Fatalf("baseline Gate = %+v, want one attempt", alone.Gate)
		}

		result, reed, noticeErr := gatedRunWithNotice(t, notice)
		if noticeErr != nil {
			t.Errorf("Send(notice) error: %v", noticeErr)
		}
		if result.Outcome != OutcomeDone || result.Gate == nil || !result.Gate.Passed {
			t.Errorf("result = %+v, want done with a passed gate", result)
		}
		if result.Gate != nil && result.Gate.Attempts != alone.Gate.Attempts {
			t.Errorf("Gate.Attempts = %d, want %d as with no concurrent send", result.Gate.Attempts, alone.Gate.Attempts)
		}
		typed := reed.typedCalls()
		assertWholeLines(t, typed, 2)
		if typed[1] != "SendText:"+notice && typed[3] != "SendText:"+notice {
			t.Errorf("typed calls = %q, want the notice typed once", typed)
		}
	})
}
