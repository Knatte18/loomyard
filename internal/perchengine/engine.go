// engine.go defines perch's own seam over burlerengine (Burler) and the
// gate-command execution seam (CommandRunner), plus the Engine type and its
// constructor. perch -> burler -> shuttle is a strict chain: Engine drives
// burlerengine.Engine (or a fake, via Burler) for every round's review/fix
// pair, and separately drives its own package-local Shuttle seam (judge.go)
// for the two ephemeral judge/triage utility calls — burler reaches shuttle
// itself for its own round; perch never routes a round through its own
// Shuttle. Engine is weft-blind and geometry-blind: it never imports
// weftengine/warpengine and never constructs a _lyx path itself; it operates
// on a caller-supplied absolute runDir (the *hubgeometry.Layout it holds is
// used only to resolve the gate command's working directory,
// layout.WorktreeRoot).

package perchengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// Burler is the seam Engine drives one round's review/fix pair through: the
// subset of burlerengine's API a round needs, satisfied as-is by
// *burlerengine.Engine in production and by a fake in unit tests. Kept
// package-local, mirroring burlerengine's own Shuttle seam rationale: it
// lets perchengine stay burler-agnostic and testable without wiring a real
// shuttle or LLM provider.
type Burler interface {
	Run(burlerengine.Profile, burlerengine.RunOpts) (burlerengine.Result, error)
}

// var _ Burler = (*burlerengine.Engine)(nil) is the compile-time proof that
// *burlerengine.Engine satisfies Burler as-is, so production wiring
// (perchcli) never needs an adapter type.
var _ Burler = (*burlerengine.Engine)(nil)

// CommandRunner is the gate-command execution seam: it runs argv inside dir,
// killing the command after timeout, and reports the raw combined
// stdout+stderr output plus whether the command exited zero. A non-zero
// exit AND a timeout are both reported as (output, false, nil): ordinary
// gate failures the loop branches on (a hung command is an artifact signal
// — most plausibly the round's own fix deadlocked it — and its partial
// output feeds forward like any other failing gate). err is reserved for
// could-not-start failures only (binary not found, permission denied),
// where the gate never observed the artifact at all.
type CommandRunner func(argv []string, dir string, timeout time.Duration) (output []byte, exitZero bool, err error)

// Options carries the two seams a caller may override; both fields default
// when left zero-valued. A nil PauseRequested means "no pause source
// wired" (the loop is never paused). A nil RunCommand means "use the real
// exec runner", execGateCommand (gate.go). New stores both fields verbatim,
// nils included — run.go is the single place that substitutes
// these defaults, at the top of Run, which is what keeps this file free of
// any compile dependency on gate.go's execGateCommand.
type Options struct {
	PauseRequested func() bool
	RunCommand     CommandRunner
}

// Engine drives one perch block's round loop: burler is the round driver,
// shuttle is the seam judge.go's ephemeral judge/triage calls use, cfg holds
// the resolved perch.yaml defaults, layout resolves the gate command's
// working directory, and pauseRequested/runCommand are Options' fields
// stored verbatim (see Options and New).
type Engine struct {
	burler         Burler
	shuttle        Shuttle
	cfg            Config
	layout         *hubgeometry.Layout
	pauseRequested func() bool
	runCommand     CommandRunner
}

// New returns an Engine ready to run one perch block's round loop, driving
// burler for every round and shuttle for the ephemeral judge/triage calls,
// tuned by cfg and resolving paths against layout. opts' fields are stored
// verbatim (nil allowed); Engine.Run substitutes their defaults at its
// entry.
func New(burler Burler, shuttle Shuttle, cfg Config, layout *hubgeometry.Layout, opts Options) *Engine {
	return &Engine{
		burler:         burler,
		shuttle:        shuttle,
		cfg:            cfg,
		layout:         layout,
		pauseRequested: opts.PauseRequested,
		runCommand:     opts.RunCommand,
	}
}
