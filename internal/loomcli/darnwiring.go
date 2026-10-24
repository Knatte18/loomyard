// darnwiring.go holds the wiring only a darn run needs: the tree scope reader the darn recipe tells its verify gate, the verify source its landing producers read, the Darn-Write row's seams, the board entry snapshot its discussion commit writes, the writer's and Describe's Spec sources, and the rename of retired darn rows in a run's status file.
// A loom run leaves all of it unwired, but shares the merge base readers `pairMergeBase` and `taskMergeBase` build.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
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
// baseCommand is the fix rows' `code-burler-fix` call, as in a loom run, and stays the base command of a tree-scoped site.
func darnVerifySource(l *lyxcwd.Location) verifySource {
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
func pairMergeBase(l *lyxcwd.Location) func() (string, error) {
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
func darnTreeScope(l *lyxcwd.Location) func(gateKey string) (string, bool, error) {
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
// Reason is the status file's error while the run is halted at the Darn-Write row, and empty otherwise.
// An absent status file and a history with no Darn-Write entry report not found.
func latestDarnOutcome(statusPath, statusLockPath string) (loomshed.DarnOutcome, bool, error) {
	st, found, err := state.ReadJSONNoMkdir[shedengine.Status](statusPath, statusLockPath)
	// A run directory not yet created fails the lock open with not-exist; that is an absent status file too.
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return loomshed.DarnOutcome{}, false, err
	}
	if err != nil || !found {
		return loomshed.DarnOutcome{}, false, nil
	}
	for i := len(st.History) - 1; i >= 0; i-- {
		if st.History[i].Producer != loomshed.NameDarnWrite || st.History[i].Outcome == shedengine.OutcomeReroute {
			continue
		}
		latest := loomshed.DarnOutcome{Outcome: st.History[i].Outcome}
		halted := st.State == shedengine.StateBlocked || st.State == shedengine.StateAwaiting || st.State == shedengine.StateFailed
		if halted && st.CurrentProducer == loomshed.NameDarnWrite {
			latest.Reason = st.Error
		}
		return latest, true, nil
	}

	return loomshed.DarnOutcome{}, false, nil
}

// darnDeps assembles the Darn-Write row's seams for location and the run addressed by runID.
// Every closure reads or writes on demand and opens nothing at build time.
func darnDeps(l *lyxcwd.Location, runID string) loomshed.DarnDeps {
	return loomshed.DarnDeps{
		// Commit mirrors CommitDescription, including its discard of (sha, committed), which makes a repeat commit over an unchanged directory a no-op.
		Commit: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), l, []string{loomengine.LandingDirRel()}, fmt.Sprintf("loom: change description for %s", seedSlug(l.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		LatestOutcome: func() (loomshed.DarnOutcome, bool, error) {
			return latestDarnOutcome(shedrun.StatusFile(l, runID), shedrun.StatusLock(l, runID))
		},
	}
}

// renameRetiredDarnRows rewrites each retired darn row name the run's status file carries onto the row's current name, so a run parked under an old name resumes.
// A live driver holding the run lock is left alone with a warning, since that driver runs the binary that wrote the old name; a later verb renames once it has stopped.
func (c *loomCLI) renameRetiredDarnRows() error {
	renamed, err := shedengine.RenameProducers(shedengine.RenameRequest{
		StatusPath:     c.shedPaths.StatusPath,
		LockPath:       c.shedPaths.LockPath,
		StatusLockPath: c.shedPaths.StatusLockPath,
		Renames:        loomrecipe.DarnRetiredRows(),
		RunID:          c.runID,
	})
	if errors.Is(err, shedengine.ErrShedBusy) {
		logger.Warn("loom: a live driver holds the run lock; left the retired darn row names in its status file", "status", c.shedPaths.StatusPath)
		return nil
	}
	if err != nil {
		return err
	}
	if renamed {
		logger.Info("loom: renamed retired darn rows in the status file", "status", c.shedPaths.StatusPath)
	}

	return nil
}

// commitDarnDiscussion writes the snapshot of location's task's board entry into the discussion directory, then commits the directory as commitDiscussion does.
// A snapshot that cannot be written fails the commit with nothing committed.
func commitDarnDiscussion(location *lyxcwd.Location) error {
	if err := writeBoardEntrySnapshot(location); err != nil {
		return err
	}
	return commitDiscussion(location)
}

// writeBoardEntrySnapshot writes the snapshot of location's task's board entry to the discussion directory.
// The snapshot is the darn Code-Review perch's answer key, kept apart from the writer's decision record; the run's board lock keeps the entry's body and scope fixed for the run.
// A board that cannot be read, or holds no entry for the task, is refused.
func writeBoardEntrySnapshot(location *lyxcwd.Location) error {
	board, err := boardengine.OpenHub(location.HubPath)
	if err != nil {
		return fmt.Errorf("loom: open the board to snapshot the task's entry: %w; way forward: fix the board, then hand off again", err)
	}
	slug := seedSlug(location.WorktreeName)
	task, found, err := board.GetTask(slug)
	if err != nil {
		return fmt.Errorf("loom: read board entry %q to snapshot it: %w; way forward: fix the board, then hand off again", slug, err)
	}
	if !found {
		return fmt.Errorf("loom: the board holds no entry %q to snapshot as the review's answer key; way forward: restore the entry with \"lyx board upsert\", then hand off again", slug)
	}
	if err := fsx.AtomicWriteBytes(loomengine.DiscussionBoardEntry(location), darnengine.BoardEntrySnapshot(slug, task.Title, task.Brief, task.Body)); err != nil {
		return fmt.Errorf("loom: write the board entry snapshot: %w", err)
	}
	return nil
}

// wireDarn fills the Env fields a darn run reads beyond the ones wire fills for every recipe, and replaces the loom DescribeTable with the darn one, which reads the decision record but no run record.
// It loads the hub-wide darn.yaml strictly, so start, run, step and resume refuse an absent or malformed file before any row builds.
// A malformed tree_scope key is refused with the editor way forward instead of the hub reconcile one, since --set cannot remove a key the hub file holds.
// budgets is the loom review segments' budgets, whose Code-Review entry the darn routing applies to the perch.
func (c *loomCLI) wireDarn(location *lyxcwd.Location, stencilsDir, specsDir, frictionDir string, landingCfg landingshed.Config, registry modelspec.Registry, budgets map[string]int) error {
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
		return loomrecipe.DarnRouting(budgets)
	})
	c.env.Darn = darnDeps(location, c.runID)
	// The overlap note Finalize writes on a large-merge reroute is the Code-Bouncer's review input.
	c.env.ReviewInput = overlapNoteInput(loomengine.LoomScratchDir(location))
	// The history seam turns the Darn-Write row's "discussion" gate into the darn gate, whose commit snapshots the board entry the perch reads.
	c.env.DiscussionHistory = discussionHistory(location)
	c.env.CommitDiscussion = func() error { return commitDarnDiscussion(location) }
	c.env.DarnTable = func(told loomshed.DarnTold) (seatengine.Table, error) {
		return darnengine.DarnTable(darnengine.DarnInputs{
			StencilsDir:        stencilsDir,
			SpecsDir:           specsDir,
			WorktreeRoot:       location.WorktreePath(),
			FrictionDir:        frictionDir,
			Slug:               seedSlug(location.WorktreeName),
			DescriptionPath:    summaryparser.Path(loomengine.LandingDir(location)),
			DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
			SupportLogPath:     loomengine.DiscussionSupportLog(location),
			PriorWork:          told.PriorWork,
			LanguageSkills:     c.languageSkills,
		}, cfg, registry)
	}
	reviewSubdir, isBouncer, err := loomrecipe.BouncerRunSubdir(loomshed.NameCodeBouncer)
	if err != nil {
		return err
	}
	if !isBouncer {
		return fmt.Errorf("loom: the embedded darn recipe's %s row is not a Bouncer with a run_subdir, so this lyx build is broken", loomshed.NameCodeBouncer)
	}
	// DescribeTable opens the fabric lazily, for the reason the loom DescribeTable gives.
	c.env.DescribeTable = func() (seatengine.Table, error) {
		handle, err := fabricengine.Open(location)
		if err != nil {
			return seatengine.Table{}, err
		}
		taskBranch, parentBranch, err := landingBranches(handle, location)
		if err != nil {
			return seatengine.Table{}, err
		}
		mergeBase, err := taskMergeBase(location)()
		if err != nil {
			return seatengine.Table{}, err
		}
		return landingshed.DarnDescribeTable(landingshed.DarnDescribeInputs{
			StencilsDir:        stencilsDir,
			ReviewDir:          filepath.Join(loomengine.LoomReviewsDir(location), reviewSubdir),
			DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
			DescriptionPath:    summaryparser.Path(loomengine.LandingDir(location)),
			TaskBranch:         taskBranch,
			ParentBranch:       parentBranch,
			MergeBase:          mergeBase,
			Slug:               seedSlug(location.WorktreeName),
		}, landingCfg, registry)
	}

	return nil
}
