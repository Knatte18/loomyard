package shuttleengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// defaultConfig is the Config most tests that never poll use.
var defaultConfig = Config{StartupTimeoutS: 30, RunTimeoutMin: 5}

// fastConfig is defaultConfig with a poll interval and liveness cadence small enough that a scripted probe sequence runs at no real wall-clock cost.
// Its RunTimeoutMin is generous enough that Spec.validate's zero-Timeout default never binds the run deadline before a test's own scripted startup outcome does;
// a zero result would put run.deadline at start time, which the startup step's own run-deadline check would hit after the very first pending probe.
var fastConfig = Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1}

// shortStartupConfig is fastConfig with a one-second startup deadline and a poll interval long enough that a single fake-clock sleep crosses it.
var shortStartupConfig = Config{StartupTimeoutS: 1, RunTimeoutMin: 5, PollIntervalMS: 600, LivenessEveryNPolls: 1}

// sparseProbeConfig is fastConfig with a liveness probe only every hundredth poll.
var sparseProbeConfig = Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 100}

// gateConfig is fastConfig with a liveness cadence no gate test reaches, so a gate's scripted STOP and pane sequence alone decides the outcome.
var gateConfig = Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000}

// fixture is a Runner over a fresh temp worktree.
// Anchor is a real subpath of Worktree, never the same value, so a swapped NewRunner argument pair fails a test rather than passing.
// The Runner's paneCwd is Anchor by construction, since it is built through NewRunner rather than NewDetachedRunner.
type fixture struct {
	t        *testing.T
	Runner   *Runner
	Anchor   string
	Worktree string
	// DotLyx is where reed's own state file lives, derived the way Attach derives it.
	DotLyx string
	// RunRoot is the run-directory root the Runner resolves.
	RunRoot string
	// EventsPath is the events file of the run withStrand seeded, empty otherwise.
	EventsPath string
}

type fixtureSettings struct {
	cfg            Config
	clk            clock
	guid           string
	separateRunDir bool
}

type fixtureOpt func(*fixtureSettings)

// withConfig sets the Runner's Config, defaultConfig otherwise.
func withConfig(cfg Config) fixtureOpt {
	return func(s *fixtureSettings) { s.cfg = cfg }
}

// withClock sets the Runner's clock seam.
func withClock(clk clock) fixtureOpt {
	return func(s *fixtureSettings) { s.clk = clk }
}

// withStrand seeds run-1 under the run-dir root with guid as its strand and an events path inside it, so FindRun can resolve guid:
// Runner.Inject and ReadEvents, unlike (*Run).Interrupt/Send, have no in-process Run handle to draw StrandGUID from.
func withStrand(guid string) fixtureOpt {
	return func(s *fixtureSettings) { s.guid = guid }
}

// withSeparateRunDir points the run-dir root at its own temp directory through Config.RunDir, so a test can seed run.json files without reasoning about the anchor-relative default layout.
func withSeparateRunDir() fixtureOpt {
	return func(s *fixtureSettings) { s.separateRunDir = true }
}

func newFixture(t *testing.T, reed ReedOps, engine Engine, opts ...fixtureOpt) *fixture {
	t.Helper()
	s := fixtureSettings{cfg: defaultConfig}
	for _, opt := range opts {
		opt(&s)
	}
	fx := &fixture{t: t, Worktree: t.TempDir()}
	fx.Anchor = filepath.Join(fx.Worktree, "sub", "dir")
	if err := os.MkdirAll(fx.Anchor, 0o755); err != nil {
		t.Fatalf("mkdir anchor path: %v", err)
	}
	if s.separateRunDir {
		s.cfg.RunDir = t.TempDir()
	}
	fx.Runner = NewRunner(reed, engine, fx.Anchor, fx.Worktree, s.cfg)
	if s.clk != nil {
		fx.Runner.clock = s.clk
	}
	fx.DotLyx = filepath.Join(fx.Anchor, lyxdirs.DotLyxDirName)
	fx.RunRoot = runDirRoot(s.cfg, fx.Anchor)
	if s.guid != "" {
		runDir := filepath.Join(fx.RunRoot, "run-1")
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatalf("mkdir run dir: %v", err)
		}
		fx.EventsPath = filepath.Join(runDir, "events.jsonl")
		if err := saveRunState(runDir, RunState{RunID: "run-1", StrandGUID: s.guid, EventsPath: fx.EventsPath}); err != nil {
			t.Fatalf("saveRunState: %v", err)
		}
	}
	return fx
}

type runSettings struct {
	runDir   string
	state    RunState
	clk      clock
	deadline time.Time
	gate     GateSpec
}

type runOpt func(*testing.T, *runSettings)

// withRunState replaces the Run's state, which otherwise names strand-1 alone.
// It precedes withRunEvents, which writes into the state it leaves.
func withRunState(state RunState) runOpt {
	return func(_ *testing.T, s *runSettings) { s.state = state }
}

// withRunDir sets the directory the Run reads and writes its run files in, empty otherwise.
func withRunDir(dir string) runOpt {
	return func(_ *testing.T, s *runSettings) { s.runDir = dir }
}

// withRunGate sets the Run's completion gate.
func withRunGate(gate GateSpec) runOpt {
	return func(_ *testing.T, s *runSettings) { s.gate = gate }
}

// withRunEvents seeds an events file holding events in a fresh run directory and points the Run's state at it.
func withRunEvents(events string) runOpt {
	return func(t *testing.T, s *runSettings) {
		s.runDir = t.TempDir()
		s.state.EventsPath = filepath.Join(s.runDir, eventsFileName)
		if err := os.WriteFile(s.state.EventsPath, []byte(events), 0o644); err != nil {
			t.Fatalf("seed events: %v", err)
		}
	}
}

// withRunClock sets the Run's clock and the deadline it reads against.
func withRunClock(clk clock, deadline time.Time) runOpt {
	return func(_ *testing.T, s *runSettings) {
		s.clk = clk
		s.deadline = deadline
	}
}

// newRun returns a Run handle over the fixture's Runner with no Start/Wait machinery involved.
func (fx *fixture) newRun(spec Spec, opts ...runOpt) *Run {
	fx.t.Helper()
	s := runSettings{state: RunState{StrandGUID: "strand-1"}}
	for _, opt := range opts {
		opt(fx.t, &s)
	}
	// Production always has a clock and a run directory;
	// the wait mark reads both.
	if s.clk == nil {
		s.clk = realClock{}
	}
	if s.runDir == "" {
		s.runDir = fx.t.TempDir()
	}
	return &Run{
		runner:   fx.Runner,
		spec:     spec,
		runDir:   s.runDir,
		state:    s.state,
		clock:    s.clk,
		deadline: s.deadline,
		gate:     s.gate,
	}
}
