// engine.go defines the caller-facing seams: CommandRunner, Options, and the Engine type with its constructor.
// Engine drives a caller-supplied RoundRunner for each round's attempts and a Shuttle for ephemeral judge calls.
// It never routes a round through the Shuttle.
// Engine is blind to fabric and geometry: it never imports fabricengine or lymcwd, and it never builds a _lym path.
// It operates on a caller-supplied absolute runDir, and GateDir in Profile sets the gate command's directory.
package treadleengine

import (
	"fmt"
	"time"
)

// CommandRunner is the gate-command execution seam.
// It runs argv inside dir and kills the command after timeout.
// It reports the raw combined stdout and stderr output and whether the command exited zero.
// A non-zero exit and a timeout are both reported as (output, false, nil): ordinary gate failures the loop branches on.
// A hung command is an artifact signal, most plausibly a deadlock in the round's own fix,
// and its partial output feeds forward like any other failing gate.
// err is reserved for a command that could not start, such as a missing binary or a permission error:
// the gate never observed the artifact at all.
type CommandRunner func(argv []string, dir string, timeout time.Duration) (output []byte, exitZero bool, err error)

// Options carries optional seams a caller may override.
type Options struct {
	PauseRequested func() bool
	RunCommand     CommandRunner
	// ScratchDir holds the block's never-tracked artifacts: run.lock, state.json.lock, and the pause flag.
	// An empty value defaults to runDir, so a caller that keeps both trees in one directory needs no change.
	// The engine never derives this path itself; the caller resolves it from its own geometry and passes it in.
	// The engine imports neither internal/lymcwd nor internal/lymdirs: the caller alone knows the .lym-anchored layout.
	ScratchDir string
	// StencilsDir is the absolute stencils directory.
	// The judge.go and targeting.go read sites pass it to stencilstore.Read at call time.
	// The engine never derives this path itself.
	// The caller resolves it from its own geometry;
	// the engine receives it as given.
	StencilsDir string
	// ParentName is the name of the session that the spawned judge and targeting roles escalate to.
	// `parentdirective.Directive` renders it into their prompts.
	// An empty value renders the no-parent variant.
	ParentName string
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
	parentName     string
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
		parentName:     opts.ParentName,
	}
}

// errf composes a name-prefixed error.
func (e *Engine) errf(format string, args ...any) error {
	return fmt.Errorf(e.name+": "+format, args...)
}
