// darnwiring.go holds the wiring only a tree-scoped or darn run needs.
// It holds the tree scope reader for `verify_source: tree_scope`, the verify source, and the Darn-Write row's seams.
// It also holds the writer's Spec source, the agent rows' record check, and the rename of retired darn rows.
// A tapestry run leaves all of it unwired, but shares the merge base readers `pairMergeBase` and `taskMergeBase` build.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// darnVerifySource returns the darn recipe's verify source: the call of the gate key a landing row names.
// The command resolves at each call, never at construction, so an edit between steps is seen.
// For a task diff wholly under the hub's configured trees, it is the key's call over the touched trees' targets.
// Otherwise it is the key's call alone.
// A gate config refusal and a malformed tree key are errors naming the way forward.
// baseCommand is the call of the `code-burler-fix` gate key, the fix rows' base command.
// A tree-scoped site keeps this base command.
func darnVerifySource(l *lymcwd.Location) verifySource {
	dirs := fabricengine.ConfigDirs(l)
	treeScope := darnTreeScope(l)
	call := gateCallSource(dirs)
	return verifySource{
		command: func(gateKey string) (string, error) {
			command, scoped, err := treeScope(gateKey)
			if err != nil {
				return "", err
			}
			if scoped {
				return command, nil
			}
			return call(gateKey)
		},
		baseCommand: func() (string, error) { return call("code-burler-fix") },
		gateTiers:   gateTiersLookup(dirs),
	}
}

// pairMergeBase returns the reader of the task branch's merge base with its recorded parent branch.
// The fabric is opened and the parent branch read on each call,
// so a parent that moved after bootstrap is read as it stands.
func pairMergeBase(l *lymcwd.Location) func() (string, error) {
	return func() (string, error) {
		parent, err := readRecordedParentBranch(l)
		if err != nil {
			return "", err
		}
		handle, err := fabricengine.Open(l)
		if err != nil {
			return "", err
		}
		return handle.MergeBase(parent)
	}
}

// darnTreeScope returns the darn recipe's tree scope reader.
// The reader returns the tree-scoped command and true for a task diff wholly under the hub's configured trees;
// otherwise it returns ("", false, nil).
// The command is the call of the row's gate key followed by the touched trees' targets,
// so a tree-scoped site runs its own key's tiers over those targets in place of the key's scope.
// Each call reads the hub-wide darn.yaml afresh, so an edit between steps is seen.
// It reads the diff from the pair's merge base to HEAD, counting a rename at both its paths.
// A load error carries its way forward: the editor's for a malformed tree key, the hub reconcile's otherwise.
// A scoped diff whose trees contribute no target answers an empty command, which runs nothing.
func darnTreeScope(l *lymcwd.Location) func(gateKey string) (string, bool, error) {
	mergeBase := taskMergeBase(l)
	call := gateCallSource(fabricengine.ConfigDirs(l))
	return func(gateKey string) (string, bool, error) {
		dirs := fabricengine.ConfigDirs(l)
		cfg, err := darnengine.LoadConfig(dirs, "darn")
		if err != nil {
			if errors.Is(err, darnengine.ErrTreeScopeKey) {
				return "", false, darnengine.WithEditorWayForward(err)
			}
			return "", false, fmt.Errorf("%w; %s", err, hubConfigWayForward)
		}
		if len(cfg.TreeScope) == 0 {
			return "", false, nil
		}
		base, err := mergeBase()
		if err != nil {
			return "", false, err
		}
		handle, err := fabricengine.Open(l)
		if err != nil {
			return "", false, err
		}
		paths, err := handle.ChangedPathsBothSides(base, "HEAD")
		if err != nil {
			return "", false, err
		}
		targets, scoped := darnengine.TreeScopedTargets(cfg, paths)
		if !scoped || len(targets) == 0 {
			return "", scoped, nil
		}
		keyCall, err := call(gateKey)
		if err != nil {
			return "", false, err
		}
		return keyCall + " " + strings.Join(targets, " "), true, nil
	}
}

// latestDarnOutcome returns the latest Darn-Write history entry of the status file at statusPath.
// The status file is read under the lock at statusLockPath.
// The returned `FindingsPath` is findingsPath when that file exists, and empty otherwise:
// the kept findings outlive the resume write that clears the status file's error before the row is called.
// Reroute, goto and skipped entries record no spawn that did work, so they are passed over.
// An absent status file, or a history with no Darn-Write entry besides the passed-over ones, reports not found.
func latestDarnOutcome(statusPath, statusLockPath, findingsPath string) (loomshed.DarnOutcome, bool, error) {
	st, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	// A run directory not yet created fails the lock open with not-exist; that is an absent status file too.
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return loomshed.DarnOutcome{}, false, err
	}
	if err != nil || !found {
		return loomshed.DarnOutcome{}, false, nil
	}
	for i := len(st.History) - 1; i >= 0; i-- {
		entry := st.History[i]
		if entry.Producer != loomshed.NameDarnWrite || entry.Outcome == shedengine.OutcomeReroute || entry.Outcome == shedengine.OutcomeGoto || entry.Outcome == shedengine.OutcomeSkipped {
			continue
		}
		latest := loomshed.DarnOutcome{Outcome: entry.Outcome}
		if _, err := os.Stat(findingsPath); err == nil {
			latest.FindingsPath = findingsPath
		} else if !errors.Is(err, fs.ErrNotExist) {
			return loomshed.DarnOutcome{}, false, err
		}
		return latest, true, nil
	}

	return loomshed.DarnOutcome{}, false, nil
}

// discussionRecordCheck returns the record check a recipe's agent rows run over the decision record.
// The check also reads the support log and the committed history of the record that recipe keeps.
// It reads on demand and opens nothing at build time.
func discussionRecordCheck(l *lymcwd.Location, recipe string) func() ([]discussionparser.Finding, error) {
	return func() ([]discussionparser.Finding, error) {
		return loomshed.ValidateDiscussion(loomshed.DiscussionCheck{
			RecordPath:     loomengine.DiscussionDecisionRecord(l),
			SupportLogPath: loomengine.DiscussionSupportLog(l),
			History:        recordHistory(l, runRecordOf(l, recipe)),
		})
	}
}

// darnDeps assembles the Darn-Write row's seams for location and the run addressed by runID, whose recipe is recipe.
// Every closure reads or writes on demand and opens nothing at build time.
func darnDeps(l *lymcwd.Location, runID, recipe string) loomshed.DarnDeps {
	return loomshed.DarnDeps{
		RecordPath:              loomengine.DiscussionDecisionRecord(l),
		SupportLogPath:          loomengine.DiscussionSupportLog(l),
		StopMarkerPath:          shedrun.StopMarker(l, runID),
		ConditionStopMarkerPath: shedrun.ConditionStopMarker(l, runID),
		CommittedRecord: func() (bool, error) {
			_, found, err := fabricengine.CommittedAnchoredFile(l, loomengine.DiscussionDecisionRecordRel())
			return found, err
		},
		Commit: func() error { return commitDescription(l) },
		LatestOutcome: func() (loomshed.DarnOutcome, bool, error) {
			return latestDarnOutcome(shedrun.StatusFile(l, runID), shedrun.StatusLock(l, runID), shedrun.GateFindingsFile(l, runID, loomshed.NameDarnWrite))
		},
		CheckRecord: discussionRecordCheck(l, recipe),
	}
}

// renameRetiredRows rewrites each retired row name in the run's status file onto its current name, as renames maps it.
// The rename lets a run parked under an old name resume.
// A live driver holding the run lock is left alone with a warning, since it runs the binary that wrote the old name;
// a later verb renames them once that driver has stopped.
func (c *loomCLI) renameRetiredRows(renames map[string]string) error {
	renamed, err := shedengine.RenameProducers(shedengine.RenameRequest{
		StatusPath:     c.shedPaths.StatusPath,
		LockPath:       c.shedPaths.LockPath,
		StatusLockPath: c.shedPaths.StatusLockPath,
		Renames:        renames,
		RunID:          c.runID,
	})
	if errors.Is(err, shedengine.ErrShedBusy) {
		logger.Warn("loom: a live driver holds the run lock; left the retired row names in its status file", "status", c.shedPaths.StatusPath)
		return nil
	}
	if err != nil {
		return err
	}
	if renamed {
		logger.Info("loom: renamed retired rows in the status file", "status", c.shedPaths.StatusPath)
	}

	return nil
}

// wireTreeScope fills the verify Env fields that a recipe whose verify_source is tree_scope reads.
// Those are in addition to the fields filled for every recipe.
// It loads the hub-wide darn.yaml strictly.
// The start, run, step and resume verbs therefore refuse an absent or malformed file before any row builds.
// A malformed tree_scope key gets the editor way forward instead of the hub reconcile one.
// The reason is that `--set` cannot remove a key the hub file holds.
func (c *loomCLI) wireTreeScope(location *lymcwd.Location) error {
	dirs := fabricengine.ConfigDirs(location)
	cfg, err := darnengine.LoadConfig(dirs, "darn")
	if err != nil {
		if errors.Is(err, darnengine.ErrTreeScopeKey) {
			return darnengine.WithEditorWayForward(err)
		}
		return fmt.Errorf("%w; %s", err, hubConfigWayForward)
	}

	// Each gate row's told command is the call of its own gate key;
	// the tree-scoped command reaches the gate through `VerifyTreeScope`, never through this source.
	c.env.VerifyCommand = gateCallSource(dirs)
	c.env.VerifyMergeBase = taskMergeBase(location)
	c.env.VerifyTreeScope = darnTreeScope(location)
	c.env.DarnVerifyAttempts = cfg.VerifyAttempts

	return nil
}

// wireDarn fills the Env fields that a recipe with a Darn-Write row reads:
// the writer's seams, its table, and Preflight's stencils check.
// That check builds the table a first spawn reads and refuses a marker of its stencils no value fills.
// A stencil newer than the running binary, or a blank value, halts the run at Preflight, before any agent row.
// It loads the hub-wide darn.yaml strictly.
// So start, run, step and resume refuse an absent or malformed file before any row builds.
// A malformed tree_scope key is refused with the editor way forward, since --set cannot remove a key the hub file holds.
// subagentModel is loom.yaml's subagent_model: the model the writer's prompt names for the subagents it hands work to.
// pairSlug is the worktree's pair slug, empty on the prime.
// When it is set, a verify gate whose fixer setting holds a model-spec has the writer's prompt drop its gate runs.
func (c *loomCLI) wireDarn(location *lymcwd.Location, stencilsDir, specsDir, frictionDir, subagentModel, pairSlug string, registry modelspec.Registry) error {
	dirs := fabricengine.ConfigDirs(location)
	cfg, err := darnengine.LoadConfig(dirs, "darn")
	if err != nil {
		if errors.Is(err, darnengine.ErrTreeScopeKey) {
			return darnengine.WithEditorWayForward(err)
		}
		return fmt.Errorf("%w; %s", err, hubConfigWayForward)
	}

	c.env.Darn = darnDeps(location, c.runID, c.recipe)
	darnTable := func(told loomshed.DarnTold) (seatengine.Table, error) {
		gateFixed, err := writerFixerOn(c.recipe, darnWriteEngine, darnGateEntry, c.env.Setting, pairSlug)
		if err != nil {
			return seatengine.Table{}, err
		}
		table, err := darnengine.DarnTable(darnengine.DarnInputs{
			StencilsDir:        stencilsDir,
			SpecsDir:           specsDir,
			WorktreeRoot:       location.WorktreePath(),
			FrictionDir:        frictionDir,
			Slug:               seedSlug(location.WorktreeName),
			DescriptionPath:    summaryparser.Path(loomengine.LandingDir(location)),
			DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
			StopMarkerPath:     shedrun.StopMarker(location, c.runID),
			PriorWork:          told.PriorWork,
			LanguageSkills:     c.languageSkills,
			SubagentModel:      subagentModel,
			GateFixed:          gateFixed,

			ConditionStopMarkerPath: shedrun.ConditionStopMarker(location, c.runID),
		}, cfg, registry)
		if err != nil {
			return seatengine.Table{}, err
		}
		return withProjectDirective(table, location, stencilsDir)
	}
	c.env.DarnTable = darnTable
	c.env.PreflightStencils = func() error {
		table, err := darnTable(loomshed.DarnTold{})
		if err != nil {
			return fmt.Errorf("build the darn writer table: %w; way forward: fix the config or model spec it names, then re-step", err)
		}
		return table.CheckMarkers(stencilsDir, c.shuttleCfg.Engine)
	}

	return nil
}
