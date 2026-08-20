// engine.go defines perch's own seam over burlerengine (Burler) and the gate-command execution seam
// (CommandRunner), plus the Engine type, its constructor, and the thin Run method that adapts one
// perch block onto internal/treadleengine's generalized round loop.
// perch -> burler -> shuttle is a strict chain: Engine drives burlerengine.Engine (or a fake, via
// Burler) for every round's review/fix pair, and separately drives its own package-local Shuttle
// seam (a type alias of treadleengine.Shuttle) for the two ephemeral judge/triage utility calls —
// burler reaches shuttle itself for its own round;
// perch never routes a round through its own Shuttle.
// Engine is fabric-blind and geometry-blind: it never imports fabricengine and never constructs a
// _lyx path itself;
// it operates on a caller-supplied absolute runDir (the Geometry it holds carries the gate command's
// working directory, geom.GateDir, which becomes treadleengine.Profile.GateDir, alongside an
// AnchorPath the engine itself never reads).

package perchengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/treadleengine"
)

// Burler is the seam Engine drives one round's review/fix pair through.
type Burler interface {
	Run(burlerengine.Profile, burlerengine.RunOpts) (burlerengine.Result, error)
}

// Compile-time proof that *burlerengine.Engine satisfies Burler.
var _ Burler = (*burlerengine.Engine)(nil)

// Shuttle is perch's name for the seam judge/triage calls ride, aliased onto treadleengine.Shuttle.
type Shuttle = treadleengine.Shuttle

// CommandRunner is the gate-command execution seam: runs argv inside dir, killing after timeout,
// reports output and exit code.
type CommandRunner func(argv []string, dir string, timeout time.Duration) (output []byte, exitZero bool, err error)

// Options carries the two seams a caller may override; both fields default when left zero-valued.
type Options struct {
	PauseRequested func() bool
	RunCommand     CommandRunner
}

// Engine drives one perch block's round loop over burler rounds.
type Engine struct {
	burler         Burler
	shuttle        Shuttle
	cfg            Config
	geom           Geometry
	pauseRequested func() bool
	runCommand     CommandRunner
}

// New returns an Engine ready to run one perch block's round loop.
func New(burler Burler, shuttle Shuttle, cfg Config, geom Geometry, opts Options) *Engine {
	return &Engine{
		burler:         burler,
		shuttle:        shuttle,
		cfg:            cfg,
		geom:           geom,
		pauseRequested: opts.PauseRequested,
		runCommand:     opts.RunCommand,
	}
}

// Run drives one perch block's round loop for Profile p, reading and persisting state at runDir,
// with never-tracked artifacts (run.lock, state.json.lock, the pause flag) written to scratchDir.
// It computes the block's identity hash (ProfileHash, identity.go) over p exactly as supplied,
// validates p against e.cfg (p.validate, profile.go — unchanged), builds the burler adapter
// (adapter.go) closing over p's content fields, builds a treadleengine.Profile from p's resolved
// gate/caps/tuning fields (GateDir: e.geom.GateDir; Gate converted field-for-field), and
// delegates to treadleengine.New("perch", adapter, e.shuttle, ...).Run — then maps the
// treadleengine.Result back onto perch's own Result/RoundSummary.
// Run stays fabric-blind and geometry-blind and constructs neither path itself — runDir,
// scratchDir, and stencilsDir are all caller-supplied absolutes; treadleengine.Engine.Run owns
// creating runDir and scratchDir, so Run must not duplicate that here. stencilsDir is the absolute
// stencils directory treadleengine's judge and utility prompts read from at call time, resolved by
// perchcli, the caller that holds the hub path, via fabricengine.StencilsDir.
func (e *Engine) Run(p Profile, runDir, scratchDir, stencilsDir string) (Result, error) {
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
		GateDir:     e.geom.GateDir,
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
		ScratchDir:     scratchDir,
		StencilsDir:    stencilsDir,
	})

	result, err := te.Run(tp, runDir)
	if err != nil {
		logger.Warn("perch: round loop failed", "profileHash", hash, "runDir", runDir, "scratchDir", scratchDir, "err", err)
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
