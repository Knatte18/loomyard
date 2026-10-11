// engine.go defines Engine, the library's entry object, and the Half identity both halves of a round report; Engine.Review (review.go) and Engine.Fix (fix.go) are its two entry points.
// Today the caller of both is internal/shedadapters, which wraps each call as a Shed row.

package burlerengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// burlerSkills are the skills both halves' seats load, in order.
var burlerSkills = []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}

// Engine drives the review and fix halves of burler rounds through a seat engine, resolving Profile paths against geom.WorktreeRoot
// and Profile.ClusterFan against cfg's lens/fan library.
type Engine struct {
	seats       *seatengine.Engine
	geom        Geometry
	cfg         Config
	stencilsDir string
	frictionDir string
}

// New returns an Engine ready to run halves, resolving relative Profile paths
// against geom.WorktreeRoot and any Profile.ClusterFan against cfg (the burler.yaml lens/fan
// library, loaded via LoadConfig).
// It builds its own seat engine over shuttle, told geom's paths and names and stencilsDir.
// geom is the told geometry the caller supplies (hubgeom.BurlerGeometry in hub mode).
// stencilsDir is the absolute stencils directory (see fabricengine.StencilsDir) the halves' seat stencils and instruction stencils are read from at call time via stencilstore.Read.
// frictionDir is told rather than derived: burlerengine must not import loomengine, and
// burlerengine.Geometry is internal/hubgeom's/internal/standalonegeom's to construct under the
// Told-Geometry Invariant, so an explicit constructor parameter is the remaining told seam. An empty
// value means Tier 2 is off for this engine.
func New(shuttle seatengine.Shuttle, geom Geometry, cfg Config, stencilsDir, frictionDir string) *Engine {
	seats := seatengine.New(shuttle, seatengine.Geometry{
		WorktreeRoot: geom.WorktreeRoot,
		AnchorPath:   geom.AnchorPath,
		StencilsDir:  stencilsDir,
		ParentName:   geom.ParentName,
		Shortname:    geom.Shortname,
		Slug:         geom.Slug,
	})
	return &Engine{seats: seats, geom: geom, cfg: cfg, stencilsDir: stencilsDir, frictionDir: frictionDir}
}

// Half is one half of a round: the identities, last message and kept run directory a caller needs to act on a non-done outcome further.
type Half struct {
	SessionID            string
	StrandGUID           string
	LastAssistantMessage string
	// RunDir is the kept shuttle run directory a caller surfaces when the half dies or times out,
	// so it can point an operator at the run's artifacts for inspection.
	RunDir string
	// StartError is the text of the start error of a half that never started, which names the run dir, the strand and whether the strand was removed.
	// It is empty for a half that started.
	StartError string
	// StartedAt and EndedAt are the half's shuttle run times, zero for a half that produced no terminal result.
	StartedAt time.Time
	EndedAt   time.Time
	// Usage is the half's session token reading, forks included, unknown unless the half's run was done on an engine that reads usage.
	Usage shuttleengine.SessionUsage
}

// repairReportBeforeGate wraps told, a fix half's own gate closure, in a per-round closure that additionally instructs the fixer to rewrite fixerReportPath when the gate fails.
//
// It exists because the fixer writes its fixer-report BEFORE its gate runs.
// A gate that fails, re-prompts, and then passes would otherwise leave the round trusting a report written against the pre-repair artifact.
// Such a report claims a fix over a state that has since changed, which is exactly what the segment's judge then consumes.
//
// It calls told exactly once.
// On a non-nil error or a passing result it returns that verbatim.
// On a failing result it appends to GateResult.Findings a blank line and an instruction naming fixerReportPath, requiring it to be rewritten to reflect the repair the agent is about to make.
// The instruction rides the findings FILE and never the Send line, which must stay a single line — see the "findings always ride a file" decision.
//
// It is composed in fixTable, rather than in the closure the caller built, because only the round's profile knows its own fixer-report path.
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
