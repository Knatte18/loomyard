// darnwiring.go holds the wiring only a darn run needs: the verify source and merge base the darn recipe tells its verify gate and landing producers, the Darn row's seams, and the writer's Spec source.
// A loom run leaves all of it unwired.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// reworkRowFor returns the name of the row that answers an operator rejection in recipe: the Darn row for the darn recipe, PR-Rework otherwise.
func reworkRowFor(recipe string) string {
	if recipe == shedrun.RecipeDarn {
		return loomshed.NameDarn
	}

	return loomshed.NamePRRework
}

// darnVerifySource returns the darn recipe's verify source: darn.yaml's verify command, and the Darn route back to a fix.
// command reads the hub-wide file each time it is called, never at construction, so an edit between steps is seen;
// a missing file and an empty value are errors naming the way forward.
func darnVerifySource(l *lyxcwd.Location) verifySource {
	return verifySource{
		command: func() (string, error) {
			return darnengine.VerifyCommand(fabricengine.BoardDir(l.HubPath))
		},
		failedWayForward: `run "lyx loom goto --to Darn", then "lyx loom resume", in the task worktree; the Darn spawn renders the Publish failure record and its gate runs the failing tests`,
	}
}

// darnMergeBase returns the reader of the task branch's merge base with its recorded parent branch.
// The fabric is opened and the parent branch read on each call, so a parent that moved after bootstrap is read as it stands.
func darnMergeBase(l *lyxcwd.Location) func() (string, error) {
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

// latestDarnOutcome returns the latest Darn history entry of the status file at statusPath, locked by statusLockPath.
// Reason is the status file's error while the run is halted at the Darn row, and empty otherwise.
// An absent status file and a history with no Darn entry report not found.
func latestDarnOutcome(statusPath, statusLockPath string) (loomshed.DarnOutcome, bool, error) {
	st, found, err := state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
	// A run directory not yet created fails the lock open with not-exist; that is an absent status file too.
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return loomshed.DarnOutcome{}, false, err
	}
	if err != nil || !found {
		return loomshed.DarnOutcome{}, false, nil
	}
	for i := len(st.History) - 1; i >= 0; i-- {
		if st.History[i].Producer != loomshed.NameDarn {
			continue
		}
		latest := loomshed.DarnOutcome{Outcome: st.History[i].Outcome}
		halted := st.State == shedengine.StateBlocked || st.State == shedengine.StateAwaiting || st.State == shedengine.StateFailed
		if halted && st.CurrentProducer == loomshed.NameDarn {
			latest.Reason = st.Error
		}
		return latest, true, nil
	}

	return loomshed.DarnOutcome{}, false, nil
}

// darnDeps assembles the Darn row's seams for location and the run addressed by runID.
// Every closure reads or writes on demand and opens nothing at build time.
func darnDeps(l *lyxcwd.Location, runID, verifyDir string) loomshed.DarnDeps {
	paths := verifytree.NewPaths(l.WorktreePath(), verifyDir)
	return loomshed.DarnDeps{
		ReadRejection:  pendingRejectionReader(l),
		ClearRejection: rejectionClearer(l),
		// Commit mirrors CommitDescription, including its discard of (sha, committed), which makes a repeat commit over an unchanged directory a no-op.
		Commit: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), l, []string{loomengine.LandingDirRel()}, fmt.Sprintf("loom: change description for %s", seedSlug(l.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		LatestOutcome: func() (loomshed.DarnOutcome, bool, error) {
			return latestDarnOutcome(shedrun.StatusFile(l, runID), shedrun.StatusLock(l, runID))
		},
		PublishFailure: func() (string, bool, error) {
			_, found, err := verifytree.ReadPublishFailure(paths)
			if err != nil || !found {
				return "", false, err
			}
			return paths.PublishFailure, true, nil
		},
	}
}

// wireDarn fills the Env fields a darn run reads beyond the ones wire fills for every recipe.
// It loads the hub-wide darn.yaml strictly and reads its verify command once, so start, run, step and resume refuse an absent or empty command before any row builds.
func (c *loomCLI) wireDarn(location *lyxcwd.Location, stencilsDir, specsDir, frictionDir string, registry modelspec.Registry) error {
	boardDir := fabricengine.BoardDir(location.HubPath)
	cfg, err := darnengine.LoadConfig(boardDir, "darn")
	if err != nil {
		return fmt.Errorf("%w; %s", err, hubConfigWayForward)
	}
	if _, err := darnengine.VerifyCommand(boardDir); err != nil {
		return err
	}

	c.env.VerifyCommand = darnVerifySource(location).command
	c.env.VerifyMergeBase = darnMergeBase(location)
	c.env.DarnVerifyAttempts = cfg.VerifyAttempts
	c.env.Darn = darnDeps(location, c.runID, c.env.VerifyDir)
	c.env.DarnSpec = func(told loomshed.DarnTold) (shuttleengine.Spec, error) {
		return darnengine.DarnSpec(darnengine.DarnInputs{
			StencilsDir:       stencilsDir,
			SpecsDir:          specsDir,
			WorktreeRoot:      location.WorktreePath(),
			FrictionDir:       frictionDir,
			ParentName:        c.parentName,
			Slug:              seedSlug(location.WorktreeName),
			DescriptionPath:   summaryparser.Path(loomengine.LandingDir(location)),
			RejectionFindings: told.RejectionFindings,
			PriorWork:         told.PriorWork,
		}, cfg, registry)
	}

	return nil
}
