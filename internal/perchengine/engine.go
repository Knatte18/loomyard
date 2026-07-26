// engine.go defines perch's own seam over burlerengine (Burler) and the
// gate-command execution seam (CommandRunner), plus the Engine type, its
// constructor, and the thin Run method that adapts one perch block onto
// internal/treadleengine's generalized round loop. perch -> burler ->
// shuttle is a strict chain: Engine drives burlerengine.Engine (or a fake,
// via Burler) for every round's review/fix pair, and separately drives its
// own package-local Shuttle seam (a type alias of treadleengine.Shuttle)
// for the two ephemeral judge/triage utility calls — burler reaches shuttle
// itself for its own round; perch never routes a round through its own
// Shuttle. Engine is weft-blind and geometry-blind: it never imports
// weftengine/warpengine and never constructs a _lyx path itself; it
// operates on a caller-supplied absolute runDir (the *hubgeometry.Layout it
// holds is used only to resolve the gate command's working directory,
// layout.WorktreeRoot, which becomes treadleengine.Profile.GateDir).

package perchengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/treadleengine"
)

// Burler is the seam Engine drives one round's review/fix pair through: the
// subset of burlerengine's API a round needs, satisfied as-is by
// *burlerengine.Engine in production and by a fake in unit tests. Kept
// package-local, mirroring burlerengine's own Shuttle seam rationale: it
// lets perchengine stay burler-agnostic and testable without wiring a real
// shuttle or LLM provider. adapter.go's burlerAdapter is what actually
// drives this seam, as treadleengine's RoundRunner.
type Burler interface {
	Run(burlerengine.Profile, burlerengine.RunOpts) (burlerengine.Result, error)
}

// var _ Burler = (*burlerengine.Engine)(nil) is the compile-time proof that
// *burlerengine.Engine satisfies Burler as-is, so production wiring
// (perchcli) never needs an adapter type.
var _ Burler = (*burlerengine.Engine)(nil)

// Shuttle is perch's own name for the seam judge/triage calls ride,
// aliased directly onto treadleengine.Shuttle (not a distinct interface)
// so existing fakes across the codebase satisfy it unchanged — the
// byte-identical-perch-api shared decision.
type Shuttle = treadleengine.Shuttle

// CommandRunner is the gate-command execution seam: it runs argv inside dir,
// killing the command after timeout, and reports the raw combined
// stdout+stderr output plus whether the command exited zero. A non-zero
// exit AND a timeout are both reported as (output, false, nil): ordinary
// gate failures the loop branches on (a hung command is an artifact signal
// — most plausibly the round's own fix deadlocked it — and its partial
// output feeds forward like any other failing gate). err is reserved for
// could-not-start failures only (binary not found, permission denied),
// where the gate never observed the artifact at all. perch-owned (not an
// alias of treadleengine.CommandRunner), converted to treadle's identical
// function type at wiring time inside Run.
type CommandRunner func(argv []string, dir string, timeout time.Duration) (output []byte, exitZero bool, err error)

// Options carries the two seams a caller may override; both fields default
// when left zero-valued. A nil PauseRequested means "no pause source
// wired" (the loop is never paused). A nil RunCommand means "use the real
// exec runner", treadleengine's execGateCommand. New stores both fields
// verbatim, nils included — Run is the single place that constructs
// treadleengine.Options from them.
type Options struct {
	PauseRequested func() bool
	RunCommand     CommandRunner
}

// Engine drives one perch block's round loop: burler is the round driver,
// shuttle is the seam the ephemeral judge/triage calls use, cfg holds the
// resolved perch.yaml defaults, layout resolves the gate command's working
// directory, and pauseRequested/runCommand are Options' fields stored
// verbatim (see Options and New). Run itself is a thin adapter that builds
// a treadleengine.Profile and a burler-backed treadleengine.RoundRunner
// (adapter.go) and delegates to treadleengine.Engine.Run.
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
// verbatim (nil allowed); Run substitutes their defaults at its entry.
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

// Run drives one perch block's round loop for Profile p, reading and
// persisting state at runDir. It computes the block's identity hash
// (ProfileHash, identity.go) over p exactly as supplied, validates p
// against e.cfg (p.validate, profile.go — unchanged), builds the burler
// adapter (adapter.go) closing over p's content fields, builds a
// treadleengine.Profile from p's resolved gate/caps/tuning fields
// (GateDir: e.layout.WorktreeRoot; Gate converted field-for-field), and
// delegates to treadleengine.New("perch", adapter, e.shuttle, ...).Run —
// then maps the treadleengine.Result back onto perch's own
// Result/RoundSummary. treadleengine.Engine.Run owns creating runDir itself
// (MkdirAll); Run must not duplicate that here.
func (e *Engine) Run(p Profile, runDir string) (Result, error) {
	hash, err := ProfileHash(p)
	if err != nil {
		return Result{}, err
	}

	if err := p.validate(e.cfg); err != nil {
		return Result{}, err
	}

	adapter := &burlerAdapter{burler: e.burler, profile: p}

	tp := treadleengine.Profile{
		ProfileHash: hash,
		Gate: treadleengine.Gate{
			Mode:    treadleengine.GateMode(p.Gate.Mode),
			Command: p.Gate.Command,
			Timeout: p.Gate.Timeout,
		},
		GateDir:     e.layout.WorktreeRoot,
		RoundCaps:   p.RoundCaps,
		JudgeModel:  p.JudgeModel,
		JudgeEffort: p.JudgeEffort,
		Model:       p.Model,
		Effort:      p.Effort,
		Timeout:     p.Timeout,
	}

	var runCommand treadleengine.CommandRunner
	if e.runCommand != nil {
		runCommand = treadleengine.CommandRunner(e.runCommand)
	}

	te := treadleengine.New("perch", adapter, e.shuttle, treadleengine.Options{
		PauseRequested: e.pauseRequested,
		RunCommand:     runCommand,
	})

	result, err := te.Run(tp, runDir)
	if err != nil {
		return Result{}, err
	}
	return mapResult(result), nil
}

// mapResult converts a treadleengine.Result onto perch's own byte-identical
// Result/RoundSummary vocabulary, converting Verdict onto
// burlerengine.Verdict (perch's RoundSummary.Verdict type — unchanged, see
// result.go) field-for-field.
func mapResult(r treadleengine.Result) Result {
	rounds := make([]RoundSummary, 0, len(r.Rounds))
	for _, rs := range r.Rounds {
		rounds = append(rounds, RoundSummary{
			Round:           rs.Round,
			Attempts:        rs.Attempts,
			Verdict:         burlerengine.Verdict(rs.Verdict),
			BlockingCount:   rs.BlockingCount,
			ReviewPath:      rs.ReviewPath,
			FixerReportPath: rs.FixerReportPath,
			JudgePath:       rs.JudgePath,
			GatePath:        rs.GatePath,
			TriagePath:      rs.TriagePath,
			JudgeVerdict:    rs.JudgeVerdict,
			GatePassed:      rs.GatePassed,
		})
	}
	return Result{
		Outcome:     Outcome(r.Outcome),
		StuckReason: StuckReason(r.StuckReason),
		RoundsRun:   r.RoundsRun,
		Rounds:      rounds,
	}
}
