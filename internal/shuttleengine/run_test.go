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
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
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
			runner := NewRunner(&fakeReed{}, &fakeEngine{}, tt.anchorPath, tt.worktreeRoot, Config{RunTimeoutMin: 5})

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
			if err := runner.Inject("strand-1", []PaneInput{{Key: "Escape"}}); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
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
			runner := NewDetachedRunner(&fakeReed{}, &fakeEngine{}, tt.anchorPath, tt.worktreeRoot, tt.paneCwd, Config{RunTimeoutMin: 5})

			if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Start() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Interrupt("strand-1"); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Interrupt() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Send("strand-1", "hi"); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Send() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Inject("strand-1", []PaneInput{{Key: "Escape"}}); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
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
			runner := NewDetachedRunner(reed, engine, anchor, worktree, tt.paneCwd, Config{RunTimeoutMin: 5})
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
//testtiming:keep pins Start's exact AddSpec wiring, its single ready probe, the persisted Started and running Outcome, and RunDir naming the created directory
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
		Parent:      "parent-guid",
		Display:     render.Display{Anchor: render.AnchorBelowParent},
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

			runner := NewRunner(reed, engine, anchorPath, worktree, cfg)
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

// TestRun_Interrupt_PlaysEscape covers Interrupt on a live pane: exactly one Escape and no text.
// One capture can land mid-redraw and classify a healthy TUI as still booting, so the ready probe
// must retry within agentPaneProbeAttempts and pass once a later capture classifies Ready, rather
// than refusing on the first inconclusive frame.
func TestRun_Interrupt_PlaysEscape(t *testing.T) {
	tests := []struct {
		name   string
		script []StartupState
	}{
		{name: "ready pane", script: []StartupState{StartupReady}},
		{name: "ready probe retries a transient boot", script: []StartupState{StartupPending, StartupReady}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubInputSleep(t)
			reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
			engine := &fakeEngine{StartupScript: tt.script}
			run := newFixture(t, reed, engine, withConfig(Config{})).newRun(Spec{})

			if err := run.Interrupt(); err != nil {
				t.Fatalf("Interrupt() error: %v", err)
			}

			if len(reed.SendKeyCalls) != 1 || reed.SendKeyCalls[0].Key != "Escape" {
				t.Errorf("SendKey calls = %+v, want exactly one Escape", reed.SendKeyCalls)
			}
			if len(reed.SendTextCalls) != 0 {
				t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
			}
		})
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
			stubInputSleep(t)
			reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: tt.captures}
			run := newFixture(t, reed, readyAgentEngine(), withConfig(Config{})).newRun(Spec{})

			err := run.Send(tt.text)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Send() = nil error, want a delivery failure")
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
	composed := e.ComposeSend(text)
	return composed[:len(composed)-1]
}

// TestRun_Send_ConfirmsSubmission drives Send's submission confirmation through an engine with the InputBoxReader capability.
// The box is read only after a settle,
// an extra Enter goes out only while the box holds the sent text,
// and at most sendExtraEnters of them.
// It is not parallel: each row replaces the package-level inputSleep.
func TestRun_Send_ConfirmsSubmission(t *testing.T) {
	const settle = 300 * time.Millisecond
	const shortText = "run the suite"
	const longText = "please review the whole change set carefully and report every finding you can substantiate"
	const sleepLine = "Sleep:300ms"
	prefix := func(text string) []string {
		return []string{"Status", "CapturePane", "CapturePane", "SendKey:Escape", "SendText:" + text, "CapturePane"}
	}
	read := []string{sleepLine, "CapturePane"}
	enterThenRead := []string{"SendKey:Enter", sleepLine, "CapturePane"}
	join := func(parts ...[]string) []string {
		var out []string
		for _, part := range parts {
			out = append(out, part...)
		}
		return out
	}

	tests := []struct {
		name    string
		text    string
		boxes   []inputBoxAnswer
		wantErr string
		// wantTail is the CallLog after the appearance check.
		wantTail []string
	}{
		{
			// Also the empty box under a running turn: nothing pending,
			// so no Enter.
			name:     "box clears at once",
			text:     shortText,
			boxes:    []inputBoxAnswer{{"", true}},
			wantTail: read,
		},
		{
			name:     "box holds the text once then clears",
			text:     shortText,
			boxes:    []inputBoxAnswer{{shortText, true}, {"", true}},
			wantTail: join(read, enterThenRead),
		},
		{
			name:     "box holds a long text's wrapped draft once then clears",
			text:     longText,
			boxes:    []inputBoxAnswer{{"please review the whole change set carefully and report every finding you can substantiate", true}, {"", true}},
			wantTail: join(read, enterThenRead),
		},
		{
			name:     "box never clears",
			text:     shortText,
			boxes:    []inputBoxAnswer{{shortText, true}},
			wantErr:  "still pending in the input box after 2 extra Enter(s)",
			wantTail: join(read, enterThenRead, enterThenRead),
		},
		{
			name:     "box holds a draft of other text",
			text:     shortText,
			boxes:    []inputBoxAnswer{{"something else entirely", true}},
			wantTail: read,
		},
		{
			name:     "box holds a longer draft containing the short sent text",
			text:     shortText,
			boxes:    []inputBoxAnswer{{"run the suite and then deploy", true}},
			wantTail: read,
		},
		{
			name:     "box holds a collapsed paste placeholder",
			text:     longText,
			boxes:    []inputBoxAnswer{{"[Pasted text #1 +4 lines]", true}},
			wantTail: read,
		},
		{
			name:     "no readable box",
			text:     shortText,
			boxes:    []inputBoxAnswer{{"", false}},
			wantTail: read,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"❯ ", "❯ ", "❯ " + tt.text}}
			orig := inputSleep
			inputSleep = func(d time.Duration) {
				reed.mu.Lock()
				defer reed.mu.Unlock()
				reed.CallLog = append(reed.CallLog, "Sleep:"+d.String())
			}
			t.Cleanup(func() { inputSleep = orig })
			engine := &inputBoxEngine{fakeEngine: readyAgentEngine(), boxes: tt.boxes, settle: settle}
			run := newFixture(t, reed, engine, withConfig(Config{})).newRun(Spec{})

			err := run.Send(tt.text)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Send() error: %v, want the send confirmed", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Send() error = %v, want it to name %q", err, tt.wantErr)
			}
			if want := join(prefix(tt.text), tt.wantTail); !reflect.DeepEqual(reed.CallLog, want) {
				t.Errorf("call order = %v, want %v", reed.CallLog, want)
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
