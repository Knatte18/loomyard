// engine.go defines the RoundRunner-agnostic seams every caller wires through: the gate-command execution seam (CommandRunner), Options, and the Engine type and its constructor.
// Engine drives a caller-supplied RoundRunner for every round's attempt(s) and its own Shuttle seam (judge.go) for the two ephemeral judge/triage calls;
// it never routes a round through Shuttle itself.
// Engine is weft-blind and geometry-blind: it never imports weftengine/warpengine/lyxcwd and never constructs a _lyx path itself — it operates on a caller-supplied absolute runDir,
// and GateDir (Profile) is what resolves the gate command's working directory.
package treadleengine

import (
	"fmt"
	"time"
)

// CommandRunner is the gate-command execution seam: it runs argv inside dir, killing the command after timeout, and reports the raw combined stdout+stderr output plus whether the command exited zero.
// A non-zero exit AND a timeout are both reported as (output, false, nil): ordinary gate failures the loop branches on (a hung command is an artifact signal — most plausibly the round's own fix deadlocked it — and its partial output feeds forward like any other failing gate).
// err is reserved for could-not-start failures only (binary not found, permission denied), where the gate never observed the artifact at all. (Doc text carried over verbatim from perchengine's CommandRunner.)
type CommandRunner func(argv []string, dir string, timeout time.Duration) (output []byte, exitZero bool, err error)

// Options carries optional seams a caller may override.
type Options struct {
	PauseRequested func() bool
	RunCommand     CommandRunner
}

// Engine drives one treadle block's generalized round loop.
type Engine struct {
	name           string
	runner         RoundRunner
	shuttle        Shuttle
	pauseRequested func() bool
	runCommand     CommandRunner
}

// New returns an Engine ready to run one treadle block's round loop.
func New(name string, runner RoundRunner, shuttle Shuttle, opts Options) *Engine {
	return &Engine{
		name:           name,
		runner:         runner,
		shuttle:        shuttle,
		pauseRequested: opts.PauseRequested,
		runCommand:     opts.RunCommand,
	}
}

// errf composes a name-prefixed error.
func (e *Engine) errf(format string, args ...any) error {
	return fmt.Errorf(e.name+": "+format, args...)
}
