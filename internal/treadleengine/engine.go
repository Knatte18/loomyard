// engine.go defines the RoundRunner-agnostic seams every caller wires through: the gate-command
// execution seam (CommandRunner), Options, and the Engine type and its constructor.
// Engine drives a caller-supplied RoundRunner for every round's attempt(s) and its own Shuttle seam
// (judge.go) for the two ephemeral judge/triage calls;
// it never routes a round through Shuttle itself.
// Engine is fabric-blind and geometry-blind: it never imports fabricengine, never imports lyxcwd
// directly, and never constructs a _lyx path itself — it operates on a caller-supplied absolute
// runDir, and GateDir (Profile) is what resolves the gate command's working directory.
package treadleengine

import (
	"fmt"
	"time"
)

// CommandRunner is the gate-command execution seam: it runs argv inside dir, killing the command
// after timeout, and reports the raw combined stdout+stderr output plus whether the command exited
// zero.
// A non-zero exit AND a timeout are both reported as (output, false, nil): ordinary gate failures
// the loop branches on (a hung command is an artifact signal — most plausibly the round's own fix
// deadlocked it — and its partial output feeds forward like any other failing gate).
// err is reserved for could-not-start failures only (binary not found, permission denied), where
// the gate never observed the artifact at all.
type CommandRunner func(argv []string, dir string, timeout time.Duration) (output []byte, exitZero bool, err error)

// Options carries optional seams a caller may override.
type Options struct {
	PauseRequested func() bool
	RunCommand     CommandRunner
	// ScratchDir is the directory this block's never-tracked artifacts —
	// run.lock, state.json.lock, and the pause flag — are written to. An
	// empty value defaults to runDir, for back-compat with a caller that has
	// not yet split its scratch tree from its durable one. The engine never
	// derives this path itself; the caller is told, never the engine (Cwd
	// Resolution Invariant) — treadleengine stays off internal/lyxcwd and
	// internal/lyxdirs so the caller, which knows its own
	// .lyx-anchored geometry, is the one deriving it.
	ScratchDir string
	// StencilsDir is the absolute stencils directory judge.go's and
	// targeting.go's read sites pass to stencilstore.Read at call time. The
	// engine never derives this path itself — same told-never-derives
	// posture as ScratchDir and GateDir — the caller
	// resolves it from its own geometry and hands it in.
	StencilsDir string
}

// Engine drives one treadle block's generalized round loop.
type Engine struct {
	name           string
	runner         RoundRunner
	shuttle        Shuttle
	pauseRequested func() bool
	runCommand     CommandRunner
	scratchDir     string
	stencilsDir    string
}

// New returns an Engine ready to run one treadle block's round loop.
func New(name string, runner RoundRunner, shuttle Shuttle, opts Options) *Engine {
	return &Engine{
		name:           name,
		runner:         runner,
		shuttle:        shuttle,
		pauseRequested: opts.PauseRequested,
		runCommand:     opts.RunCommand,
		scratchDir:     opts.ScratchDir,
		stencilsDir:    opts.StencilsDir,
	}
}

// errf composes a name-prefixed error.
func (e *Engine) errf(format string, args ...any) error {
	return fmt.Errorf(e.name+": "+format, args...)
}
