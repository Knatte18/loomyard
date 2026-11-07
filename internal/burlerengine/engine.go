// engine.go defines Engine, the package's entry object, and the Half identity both halves of a round report.
// Engine.Review (review.go) and Engine.Fix (fix.go) are its two entry points.

package burlerengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// burlerSkills are the skills both halves' seats load, in order.
var burlerSkills = []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}

// Engine drives the review and fix halves of burler rounds through a seat engine.
// It resolves Profile paths against geom.WorktreeRoot and Profile.ClusterFan against cfg's lens/fan library.
type Engine struct {
	seats       *seatengine.Engine
	geom        Geometry
	cfg         lenses.Config
	stencilsDir string
	frictionDir string
	// languageSkills are the hub's language skills a code round's seats load after burlerSkills; empty for none.
	languageSkills []string
}

// seatSkills returns the skills every seat of p's half loads:
// burlerSkills alone, or burlerSkills followed by the engine's language skills when p marks a code round.
func (e *Engine) seatSkills(p *Profile) []string {
	if !p.LanguageSkills {
		return burlerSkills
	}
	return languageconfig.AppendSkills(burlerSkills, e.languageSkills)
}

// New returns an Engine ready to run halves.
// It resolves relative Profile paths against geom.WorktreeRoot, and any Profile.ClusterFan against cfg.
// cfg is the lens and fan library in lenses.yaml, loaded by lenses.Load.
// It builds its own seat engine over shuttle, told geom's paths and names and stencilsDir.
// geom is the told geometry the caller supplies, built by hubgeom.BurlerGeometry in hub mode.
// stencilsDir is the absolute stencils directory.
// The halves read their seat and instruction stencils from it at call time.
// frictionDir is told rather than derived: burlerengine must not import loomengine.
// internal/hubgeom and internal/standalonegeom construct burlerengine.Geometry under the Told-Geometry Invariant,
// so an explicit constructor parameter is the remaining told seam.
// An empty value means Tier 2 is off for this engine.
// languageSkills are the hub's language skills a code round's seats load after burlerSkills, empty for none.
func New(shuttle seatengine.Shuttle, geom Geometry, cfg lenses.Config, stencilsDir, frictionDir string, languageSkills []string) *Engine {
	seats := seatengine.New(shuttle, seatengine.Geometry{
		WorktreeRoot:  geom.WorktreeRoot,
		AnchorPath:    geom.AnchorPath,
		StencilsDir:   stencilsDir,
		DefaultEngine: geom.DefaultEngine,
		ParentName:    geom.ParentName,
		Shortname:     geom.Shortname,
		Slug:          geom.Slug,
	})
	return &Engine{seats: seats, geom: geom, cfg: cfg, stencilsDir: stencilsDir, frictionDir: frictionDir, languageSkills: languageSkills}
}

// seatEngine returns the engine a half runs on, and that its instruction files are rendered for.
// It is the engine choice names, else the geometry's DefaultEngine.
func (e *Engine) seatEngine(choice shuttleengine.ModelChoice) string {
	if choice.Engine != "" {
		return choice.Engine
	}
	return e.geom.DefaultEngine
}

// Half is one half of a round.
// It carries the identities, last message and kept run directory a caller needs when the outcome is not done.
type Half struct {
	SessionID            string
	StrandGUID           string
	LastAssistantMessage string
	// RunDir is the kept shuttle run directory.
	// A caller surfaces it when the half dies or times out, so an operator can inspect the run's artifacts.
	RunDir string
	// StartError is the start error of a half that never started, empty for a half that started.
	// It names the run dir, the strand, and whether the strand was removed.
	StartError string
	// StartedAt and EndedAt are the half's shuttle run times, zero for a half that produced no terminal result.
	StartedAt time.Time
	EndedAt   time.Time
	// Usage is the half's session token reading, forks included; unknown unless the run's engine reads usage.
	Usage shuttleengine.SessionUsage
	// Engine is the name of the engine the half's run used, empty for a half that never started.
	Engine string
}

// repairReportBeforeGate wraps told, a fix half's own gate closure, in a per-round closure.
// That closure also instructs the fixer to rewrite fixerReportPath when the gate fails.
//
// It exists because the fixer writes its fixer-report before its gate runs.
// If a gate fails, re-prompts, and then passes, the round would trust a report written before the repair.
// That report claims a fix over a state that has since changed, and the judge consumes it.
//
// It calls told exactly once.
// On a non-nil error or a passing result it returns that result verbatim.
// On a failing result it appends to GateResult.Findings a blank line and an instruction naming fixerReportPath.
// The instruction requires the report to be rewritten to reflect the repair the agent is about to make.
// The instruction rides the findings file and never the Send line, which must stay a single line.
//
// It is composed in fixTable, not in the caller's closure; only the round's profile knows its own fixer-report path.
func repairReportBeforeGate(told shuttleengine.Gate, fixerReportPath string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		result, err := told()
		if err != nil || result.Passed {
			return result, err
		}
		result.Findings += fmt.Sprintf(
			"\n\nThis round's own fixer report (%s) was written before this gate ran. Rewrite it to reflect the repair you are about to make.",
			fixerReportPath,
		)
		return result, nil
	}
}
