// darnwiring.go holds the wiring only a darn run needs: the tree scope reader the darn recipe tells its verify gate, the verify source its landing producers read, the Darn-Write row's seams, the writer's Spec source, and the rename of retired darn rows in a run's status file.
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
	"github.com/Knatte18/loomyard/internal/loomrecipe"
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
// command resolves at each call, never at construction, so an edit between steps is seen:
// the key's call over the touched trees' targets when the task diff is wholly under the hub's configured trees, else the key's call alone;
// a gate config refusal and a malformed tree key are errors naming the way forward.
// baseCommand is the fix rows' `code-burler-fix` call, as in a tapestry run, and stays the base command of a tree-scoped site.
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
// The fabric is opened and the parent branch read on each call, so a parent that moved after bootstrap is read as it stands.
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

// darnTreeScope returns the darn recipe's tree scope reader: the tree-scoped command and true for a task diff wholly under the hub's configured trees, and ("", false, nil) otherwise.
// The command is the call of the row's gate key followed by the touched trees' targets, so a tree-scoped site runs its own key's tiers over those targets in place of the key's scope.
// Each call reads the hub-wide darn.yaml afresh, so an edit between steps is seen, and reads the diff from the pair's merge base to HEAD with a rename counted at both its paths.
// A load error is returned with its way forward: the editor one for a malformed tree key, the hub reconcile one otherwise.
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

// latestDarnOutcome returns the latest Darn-Write history entry of the status file at statusPath, locked by statusLockPath.
// FindingsPath is findingsPath when that file exists, and empty otherwise:
// the kept findings outlive the resume write that clears the status file's error before the row is called.
// Reroute, goto and skipped entries record no spawn that did work, so they are passed over.
// An absent status file and a history with no other Darn-Write entry report not found.
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

// darnDeps assembles the Darn-Write row's seams for location and the run addressed by runID.
// Every closure reads or writes on demand and opens nothing at build time.
func darnDeps(l *lymcwd.Location, runID string) loomshed.DarnDeps {
	return loomshed.DarnDeps{
		RecordPath:     loomengine.DiscussionDecisionRecord(l),
		SupportLogPath: loomengine.DiscussionSupportLog(l),
		StopMarkerPath: shedrun.StopMarker(l, runID),
		CommittedRecord: func() (bool, error) {
			_, found, err := fabricengine.CommittedAnchoredFile(l, loomengine.DiscussionDecisionRecordRel())
			return found, err
		},
		Commit: func() error { return commitDescription(l) },
		LatestOutcome: func() (loomshed.DarnOutcome, bool, error) {
			return latestDarnOutcome(shedrun.StatusFile(l, runID), shedrun.StatusLock(l, runID), shedrun.GateFindingsFile(l, runID, loomshed.NameDarnWrite))
		},
		CheckRecord: func() ([]discussionparser.Finding, error) {
			return loomshed.ValidateDiscussion(loomshed.DiscussionCheck{
				RecordPath:     loomengine.DiscussionDecisionRecord(l),
				SupportLogPath: loomengine.DiscussionSupportLog(l),
				History:        recordHistory(l, runRecordOf(l, shedrun.RecipeDarn)),
			})
		},
	}
}

// renameRetiredRows rewrites each retired row name of renames that the run's status file carries onto the row's current name, so a run parked under an old name resumes.
// A live driver holding the run lock is left alone with a warning, since that driver runs the binary that wrote the old name; a later verb renames once it has stopped.
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

// wireDarn fills the Env fields a darn run reads beyond the ones wire fills for every recipe.
// It loads the hub-wide darn.yaml strictly, so start, run, step and resume refuse an absent or malformed file before any row builds.
// A malformed tree_scope key is refused with the editor way forward instead of the hub reconcile one, since --set cannot remove a key the hub file holds.
// budgets is the loom review segments' budgets, whose Code-Review entry the darn routing applies to the perch.
func (c *loomCLI) wireDarn(location *lymcwd.Location, stencilsDir, specsDir, frictionDir string, registry modelspec.Registry, budgets map[string]int) error {
	dirs := fabricengine.ConfigDirs(location)
	cfg, err := darnengine.LoadConfig(dirs, "darn")
	if err != nil {
		if errors.Is(err, darnengine.ErrTreeScopeKey) {
			return darnengine.WithEditorWayForward(err)
		}
		return fmt.Errorf("%w; %s", err, hubConfigWayForward)
	}

	// Each gate row's told command is the call of its own gate key;
	// the row key's call over the touched trees' targets reaches the gate through VerifyTreeScope, never through this source.
	c.env.VerifyCommand = gateCallSource(dirs)
	c.env.VerifyMergeBase = taskMergeBase(location)
	c.env.VerifyTreeScope = darnTreeScope(location)
	c.env.DarnVerifyAttempts = cfg.VerifyAttempts
	// The perch's CIRCLING Reason names the budget the darn routing applies.
	c.env.SegmentBounces = segmentBounces(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, func() (shedengine.Routing, error) {
		return loomrecipe.Routing(c.recipe, budgets)
	})
	c.env.Darn = darnDeps(location, c.runID)
	c.env.DarnTable = func(told loomshed.DarnTold) (seatengine.Table, error) {
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
		}, cfg, registry)
		if err != nil {
			return seatengine.Table{}, err
		}
		return withProjectDirective(table, location, stencilsDir)
	}

	return nil
}
