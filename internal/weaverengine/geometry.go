// geometry.go declares Weaver's run and scratch directories, and Geometry, the paths and Git it is told at construction.
// Dir, ReportsDir, ScratchDir and PromptsDir build Weaver's paths from the told anchor root.
// Hub mode builds a Geometry in hubgeom.WeaverGeometry.

package weaverengine

import (
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

const (
	// journalFileName is the run journal's fixed filename inside the run directory.
	journalFileName = "journal.md"
	// outcomeFileName is the run's outcome record's fixed filename inside the run directory.
	outcomeFileName = "outcome.yaml"
	// reportsDirName is the reports directory's name inside the run directory.
	reportsDirName = "reports"
	// promptsDirName is the prompts directory's name inside the scratch directory.
	promptsDirName = "prompts"
)

// Dir returns Weaver's durable run directory, joined onto the told anchorRoot.
// It holds the state file, the reports, the journal, the summary and the outcome record.
// It lives under _lym so it is fabric-synced.
func Dir(anchorRoot string) string {
	return filepath.Join(anchorRoot, lymdirs.LymDirName, lymdirs.WeaverDirName)
}

// DirRel returns Dir's anchor-relative form, `_lym/weaver`, always with forward slashes.
// It is the pathspec the per-batch commit stages.
func DirRel() string {
	return path.Join(lymdirs.LymDirName, lymdirs.WeaverDirName)
}

// ReportsDir returns the directory holding Weaver's per-attempt report files and verify fixer replies.
func ReportsDir(anchorRoot string) string {
	return filepath.Join(Dir(anchorRoot), reportsDirName)
}

// ScratchDir returns the base directory for Weaver's never-tracked artifacts: the rendered prompts and every lock.
func ScratchDir(anchorRoot string) string {
	return filepath.Join(anchorRoot, lymdirs.DotLymDirName, lymdirs.WeaverDirName)
}

// PromptsDir returns the directory holding Weaver's rendered batch and verify fixer prompts.
func PromptsDir(anchorRoot string) string {
	return filepath.Join(ScratchDir(anchorRoot), promptsDirName)
}

// JournalPath returns the run journal's path inside runDir.
func JournalPath(runDir string) string {
	return filepath.Join(runDir, journalFileName)
}

// OutcomePath returns the run's outcome record's path inside runDir.
func OutcomePath(runDir string) string {
	return filepath.Join(runDir, outcomeFileName)
}

// Geometry is the set of paths Weaver is told at construction and never derives, plus the Git it reads them through.
// No method validates or recomputes a field: the caller populates every path field with a usable absolute path.
type Geometry struct {
	// AnchorRoot is the base every _lym/.lym join and every module config read hangs off.
	AnchorRoot string
	// WorktreeRoot is the git worktree's root; every git call opens the repository here.
	WorktreeRoot string
	// PaneCwd is the directory every Weaver strand's pane runs in.
	PaneCwd string
	// RunDir is the told path to Weaver's durable run directory.
	RunDir string
	// ReportsDir is the told path to the directory holding the per-attempt report files.
	ReportsDir string
	// PromptsDir is the told path to the directory holding the rendered prompts.
	PromptsDir string
	// ScratchDir is the told path to the base directory for Weaver's never-tracked artifacts.
	ScratchDir string
	// StencilsDir is the told absolute directory the prompt stencils are read from at call time.
	StencilsDir string
	// SpecsDir is the told absolute directory the deployed normative specs are read from.
	SpecsDir string
	// PlanDir is the told directory planparser parses.
	PlanDir string
	// VerifyDir is the told directory of the worktree's verified-tree record, running marker and verify log.
	VerifyDir string
	// GateSlots is the told pool every gate run acquires a gate slot from.
	GateSlots *gateslot.Pool
	// GateWaitDir is the told per-worktree directory a slotted gate run writes its wait record into.
	GateWaitDir string
	// GateConfig is the told reader of the gate config the plan-level verify renders its gate call from.
	GateConfig func() (gateslot.Config, error)
	// ParentName is the told parent agent name the prompts render their parent directive from.
	// Empty means no parent.
	ParentName string
	// DecisionRecordPath is the told decision record the prompts read the task's declared PATTERN changes from.
	DecisionRecordPath string
	// Git is the git surface every worktree read goes through.
	// Nil means the real repository on disk.
	Git websterengine.Git
	// Index is the code index the plan gates and the batch delta checks call.
	// The CLI layer sets it.
	Index planindex.Index
	// VerifyHold is the told pause hold the plan-level verify passes to every Verify.
	// Nil holds nothing.
	VerifyHold *verifytree.PauseHold
}

// runPlan returns the shared machinery's view of g.
// Weaver's own directories are the run, reports, prompts and scratch directories,
// so its state lock and state-mutation lease sit under its scratch directory.
func (g Geometry) runPlan() runplan.Geometry {
	return runplan.Geometry{
		WorktreeRoot: g.WorktreeRoot,
		PaneCwd:      g.PaneCwd,
		PlanDir:      g.PlanDir,
		RunDir:       g.RunDir,
		ReportsDir:   g.ReportsDir,
		PromptsDir:   g.PromptsDir,
		ScratchDir:   g.ScratchDir,
		Git:          g.git(),
		Index:        g.Index,
	}
}

// git returns the Git the geometry carries, or the real Git when it carries none.
func (g Geometry) git() websterengine.Git {
	if g.Git != nil {
		return g.Git
	}
	return websterengine.NewGit()
}
