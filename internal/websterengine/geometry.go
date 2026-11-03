// geometry.go declares Geometry, the struct webster is told its coordinates through.
// It declares the type only — no constructor, no validator, and no default;
// populating every field with a usable absolute path is entirely the caller's obligation, exactly as internal/reedengine/geometry.go does for reed.
// hubgeom.WebsterGeometry is the hub-mode teller that builds a Geometry from a resolved *lymcwd.Location;
// internal/standalonegeom is the told-mode sibling that builds one from a standalone state tree.
// The dependency direction is one-way: those packages import websterengine, never the reverse.

package websterengine

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// errNoIndex is returned by a call that needs the code index when Geometry.Index was never wired.
var errNoIndex = errors.New("webster: the geometry carries no code index; the CLI layer must set Geometry.Index")

// Geometry is the set of paths webster is told, once, at construction, and never derives itself, plus the Git it reads them through.
// No method here validates or recomputes any field — populating every path field with a usable absolute
// path is entirely the caller's obligation.
type Geometry struct {
	// AnchorRoot is the base every _lym/.lym join and every module config read hangs off.
	AnchorRoot string
	// WorktreeRoot is the git worktree's root, the directory holding `.git`, PATTERN.md and go.mod.
	// Every git call opens the repository here, and the plan's card paths resolve against it.
	// In a hub whose anchor sits in a subdirectory it is above PaneCwd and AnchorRoot, never equal to them.
	// An empty WorktreeRoot yields no PATTERN directive.
	WorktreeRoot string
	// PaneCwd is the directory every webster session's pane runs in.
	// Transcript-relative write paths resolve against it, so it is the fork-audit workdir;
	// the gate runs use it as their working directory, the prompts render {{.worktree_root}} and the plan directory's spelling from it,
	// and the session's instruction files and its `_lym` and `.lym` are read under it.
	PaneCwd string
	// WebsterDir is the told path to webster's durable run state directory (state.json,
	// outcome.yaml).
	WebsterDir string
	// ReportsDir is the told path to the directory holding webster's per-batch report files.
	ReportsDir string
	// PromptsDir is the told path to the directory holding webster's rendered fork prompts.
	PromptsDir string
	// ScratchDir is the told path to the base directory for webster's never-tracked artifacts (the rendered fork prompts, every *.lock, and the plan-conflict record and last-routed conflict).
	ScratchDir string
	// StencilsDir is the told absolute directory the prompt stencils are read from at call time.
	StencilsDir string
	// SpecsDir is the told absolute directory the deployed normative specs are read from by the
	// agent the prompt is handed to — never by Go. Like every other field on this struct,
	// populating it with a usable absolute path is entirely the caller's obligation.
	SpecsDir string
	// PlanDir is the told directory planparser parses.
	PlanDir string
	// VerifyDir is the told directory of the worktree's verified-tree record, running marker and verify log, read by the webster verify gate.
	VerifyDir string
	// GateSlots is the told pool every webster gate run acquires a gate slot from.
	// Nil in standalone mode, where there is no hub, and runs unslotted.
	GateSlots *gateslot.Pool
	// GateWaitDir is the told per-worktree directory a slotted card-verify rerun writes its wait record into.
	// Empty in standalone mode, where no pool means no wait.
	GateWaitDir string
	// GateConfig is the told reader of the gate config the verify gate renders its `webster` gate call from, read at each call so an edit between two arrivals takes effect.
	// A refusal it returns names `lym config gate`.
	GateConfig func() (gateslot.Config, error)
	// ParentName is the told parent agent name the master and recovery prompts render their parent directive from.
	// Empty means no parent, which renders the no-parent variant.
	ParentName string
	// DecisionRecordPath is the told decision record the master and recovery prompts read the task's declared PATTERN changes from, and the master prompt the parent's added decisions.
	// Empty means none, which renders no declared changes and no parent decision; standalone mode leaves it empty.
	DecisionRecordPath string
	// Git is the git surface every worktree read goes through, such as the head-SHA capture and the dirty check.
	// Nil means the real repository on disk, which is what every constructor leaves.
	// It derives no path.
	// It is a field here because every function that reads git already receives the Geometry.
	Git Git
	// Index is the code index the plan gates and the batch delta checks call.
	// The CLI layer sets it; hubgeom and standalonegeom leave it empty because they sit below cliwire and must not link the index.
	// A call that reaches a nil Index returns an error naming the missing wiring.
	Index planindex.Index
	// VerifyHold is the told pause hold the verify gate passes to every Verify, so a pause covering the run holds the verify until the resume.
	// Nil holds nothing.
	// The CLI layer sets it; hubgeom and standalonegeom leave it nil.
	VerifyHold *verifytree.PauseHold
}

// index returns the geometry's code index, or errNoIndex when none was wired.
func (g Geometry) index() (planindex.Index, error) {
	if g.Index == nil {
		return nil, errNoIndex
	}
	return g.Index, nil
}
