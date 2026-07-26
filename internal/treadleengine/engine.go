// engine.go defines the RoundRunner-agnostic seams every caller wires
// through: the gate-command execution seam (CommandRunner), Options, and the
// Engine type and its constructor. Engine drives a caller-supplied
// RoundRunner for every round's attempt(s) and its own Shuttle seam
// (judge.go) for the two ephemeral judge/triage calls; it never routes a
// round through Shuttle itself. Engine is weft-blind and geometry-blind: it
// never imports weftengine/warpengine/hubgeometry and never constructs a
// _lyx path itself — it operates on a caller-supplied absolute runDir, and
// GateDir (Profile) is what resolves the gate command's working directory.
package treadleengine

import (
	"fmt"
	"time"
)

// CommandRunner is the gate-command execution seam: it runs argv inside dir,
// killing the command after timeout, and reports the raw combined
// stdout+stderr output plus whether the command exited zero. A non-zero
// exit AND a timeout are both reported as (output, false, nil): ordinary
// gate failures the loop branches on (a hung command is an artifact signal
// — most plausibly the round's own fix deadlocked it — and its partial
// output feeds forward like any other failing gate). err is reserved for
// could-not-start failures only (binary not found, permission denied),
// where the gate never observed the artifact at all. (Doc text carried over
// verbatim from perchengine's CommandRunner.)
type CommandRunner func(argv []string, dir string, timeout time.Duration) (output []byte, exitZero bool, err error)

// Options carries the two seams a caller may override; both fields default
// when left zero-valued. A nil PauseRequested means "no pause source
// wired" (the loop is never paused). A nil RunCommand means "use the real
// exec runner" (execGateCommand, gate.go). New stores both fields verbatim,
// nils included — run.go is the single place that substitutes these
// defaults, at the top of Run, which is what keeps this file free of any
// compile dependency on gate.go's execGateCommand.
type Options struct {
	PauseRequested func() bool
	RunCommand     CommandRunner
}

// Engine drives one treadle block's generalized round loop: name identifies
// the calling engine for diagnostics (perch passes "perch") — every error
// and Warn string this package produces is prefixed with name + ": ",
// mirroring perch's own literal "perch: " prefix today (the
// name-parameterized-diagnostics shared decision) — runner is the
// RoundRunner seam driven once per attempt, shuttle is the seam judge.go's
// ephemeral judge/triage calls use, and pauseRequested/runCommand are
// Options' fields stored verbatim (see Options and New).
type Engine struct {
	name           string
	runner         RoundRunner
	shuttle        Shuttle
	pauseRequested func() bool
	runCommand     CommandRunner
}

// New returns an Engine ready to run one treadle block's round loop under
// name, driving runner for every round's attempt(s) and shuttle for the
// ephemeral judge/triage calls. opts' fields are stored verbatim (nil
// allowed); Engine.Run substitutes their defaults at its entry.
func New(name string, runner RoundRunner, shuttle Shuttle, opts Options) *Engine {
	return &Engine{
		name:           name,
		runner:         runner,
		shuttle:        shuttle,
		pauseRequested: opts.PauseRequested,
		runCommand:     opts.RunCommand,
	}
}

// errf composes a name-prefixed error, the mechanism behind the
// name-parameterized-diagnostics shared decision: every error string this
// package produces reads "<e.name>: <message>", exactly matching perch's
// own literal "perch: " prefix today when e.name is "perch", and giving a
// future caller (e.g. "tenter") the same diagnostics for free.
func (e *Engine) errf(format string, args ...any) error {
	return fmt.Errorf(e.name+": "+format, args...)
}
