// darnwiring.go holds the wiring only a darn run needs: the verify source and whole-diff form the darn recipe tells its verify gate and landing producers, the Darn-Write row's seams, the writer's and Describe's Spec sources, and the rename of retired darn rows in a run's status file.
// A loom run leaves all of it unwired, but shares the merge base reader `pairMergeBase` builds.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"gopkg.in/yaml.v3"
)

// reworkRowFor returns the name of the row that answers an operator rejection in recipe: the Darn-Write row for the darn recipe, PR-Rework otherwise.
func reworkRowFor(recipe string) string {
	if recipe == shedrun.RecipeDarn {
		return loomshed.NameDarnWrite
	}

	return loomshed.NamePRRework
}

// darnVerifySource returns the darn recipe's verify source: darn.yaml's verify command, and the Darn-Write route back to a fix.
// command reads the hub-wide file each time it is called, never at construction, so an edit between steps is seen;
// a missing file and an empty value are errors naming the way forward.
func darnVerifySource(l *lyxcwd.Location) verifySource {
	return verifySource{
		command: func() (string, error) {
			return darnengine.VerifyCommand(fabricengine.ConfigDirs(l))
		},
		failedWayForward: `run "lyx loom goto --to Darn-Write", then "lyx loom resume", in the task worktree; the Darn-Write spawn renders the Publish failure record and its gate runs the failing tests`,
		failureTarget:    loomshed.NameDarnWrite,
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
		if st.History[i].Producer != loomshed.NameDarnWrite {
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

// darnReviewBudget returns darn.yaml's review_max_bounces for the routing projection.
// The full wiring has already loaded darn.yaml into c.darnCfg; the lightweight wiring loads no config, so it reads the hub-wide file here.
// An unreadable file degrades to the template's default with a warning, because the lightweight verbs (status, pause) must not refuse over a config they otherwise never read.
func (c *loomCLI) darnReviewBudget() (int, error) {
	if c.darnCfg != nil {
		return c.darnCfg.ReviewMaxBounces, nil
	}

	cfg, err := darnengine.LoadConfig(fabricengine.ConfigDirs(c.location), "darn")
	if err == nil {
		return cfg.ReviewMaxBounces, nil
	}
	logger.Warn("loom: could not read darn.yaml for the review bounce budget; reporting the template default", "error", err)

	var template darnengine.Config
	if err := yaml.Unmarshal([]byte(darnengine.ConfigTemplate()), &template); err != nil {
		return 0, err
	}

	return template.ReviewMaxBounces, nil
}

// wireDarn fills the Env fields a darn run reads beyond the ones wire fills for every recipe, and replaces the loom DescribeSpec with the darn one, which reads no decision record or run record.
// It loads the hub-wide darn.yaml strictly and reads its verify command once, so start, run, step and resume refuse an absent or empty command before any row builds.
func (c *loomCLI) wireDarn(location *lyxcwd.Location, stencilsDir, specsDir, frictionDir string, landingCfg landingshed.Config, registry modelspec.Registry) error {
	dirs := fabricengine.ConfigDirs(location)
	cfg, err := darnengine.LoadConfig(dirs, "darn")
	if err != nil {
		return fmt.Errorf("%w; %s", err, hubConfigWayForward)
	}
	if _, err := darnengine.VerifyCommand(dirs); err != nil {
		return err
	}

	c.env.VerifyCommand = darnVerifySource(location).command
	c.env.VerifyMergeBase = pairMergeBase(location)
	c.env.VerifyWholeDiff = true
	c.env.DarnVerifyAttempts = cfg.VerifyAttempts
	c.env.DarnReviewMaxBounces = cfg.ReviewMaxBounces
	c.darnCfg = &cfg
	// The Darn-Review perch's CIRCLING Reason names the budget the darn routing applies, not loom's.
	c.env.SegmentBounces = segmentBounces(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, func() (shedengine.Routing, error) {
		return loomrecipe.DarnRouting(cfg.ReviewMaxBounces)
	})
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
	reviewSubdir, isBouncer, err := loomrecipe.BouncerRunSubdir(loomshed.NameDarnBouncer)
	if err != nil {
		return err
	}
	if !isBouncer {
		return fmt.Errorf("loom: the embedded darn recipe's %s row is not a Bouncer with a run_subdir, so this lyx build is broken", loomshed.NameDarnBouncer)
	}
	// DescribeSpec opens the fabric lazily, for the reason the loom DescribeSpec gives.
	c.env.DescribeSpec = func() (shuttleengine.Spec, error) {
		handle, err := fabricengine.Open(location)
		if err != nil {
			return shuttleengine.Spec{}, err
		}
		taskBranch, parentBranch, err := landingBranches(handle, location)
		if err != nil {
			return shuttleengine.Spec{}, err
		}
		return landingshed.DarnDescribeSpec(landingshed.DarnDescribeInputs{
			StencilsDir:     stencilsDir,
			ParentName:      c.parentName,
			ReviewDir:       filepath.Join(loomengine.LoomReviewsDir(location), reviewSubdir),
			DescriptionPath: summaryparser.Path(loomengine.LandingDir(location)),
			TaskBranch:      taskBranch,
			ParentBranch:    parentBranch,
			Slug:            seedSlug(location.WorktreeName),
		}, landingCfg, registry)
	}

	return nil
}
