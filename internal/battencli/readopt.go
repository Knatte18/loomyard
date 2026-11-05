// readopt.go holds a batten resume's re-adoption of a live pair whose task worktree is gone: the seams it reaches git and origin through, and the pre-run step that judges origin, re-makes the pair and resumes.

package battencli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/statuscommit"
)

// readoptSeams carries what the re-adoption of a gone pair needs from git and origin.
// The zero value reads as no re-adoption, which only a receiver that calls no run-verb arm has.
type readoptSeams struct {
	// Gone reports whether slug's task worktree is gone from this machine.
	Gone func(slug string) (bool, error)
	// Reconcile runs the run-scope origin reconcile of slug, which may judge a run whose pair is absent.
	Reconcile func(slug string) (fabricengine.OriginReport, error)
	// Prune drops the worktree registrations whose directories are gone, so a stale registration holds no branch.
	Prune func() error
	// Add re-makes slug's pair over its live branches, adopting them and forking nothing.
	Add func(slug string) error
	// SpawnPush starts the detached push of slug's re-made pair.
	SpawnPush func(slug string) error
	// Notice posts line to the run's parent; nil posts nothing.
	Notice func(slug, line string) error
	// CommitFailed commits batten's status file after the failed stop of the row producer and spawns its push; nil commits nothing.
	CommitFailed func(slug, producer string) error
	// RecordsRoot returns the root of the records repository, which a way forward names.
	RecordsRoot func() (string, error)
	// Tags lists the archive and set-aside tags that keep slug's records, each with the files readoptRecordRels names.
	Tags func(slug string) ([]fabricengine.RecordsTag, error)
	// BranchFile reads one anchored file of a records branch.
	BranchFile func(branch, anchoredRel string) ([]byte, bool, error)
	// Upstream reports whether the records branch has an upstream setting.
	Upstream func(branch string) (bool, error)
	// TaskBranch returns slug's task branch name.
	TaskBranch func(slug string) (string, error)
	// TaskBranchGone reports whether slug's task branch is gone both locally and on origin.
	TaskBranchGone func(slug string) (bool, error)
}

// hubTaskBranch returns slug's task branch name, read from what is left of its pair, or from origin when nothing is, under the hub's fabric config, with that config.
func hubTaskBranch(location *lymcwd.Location, slug string) (string, fabricengine.Config, error) {
	cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(location))
	if err != nil {
		return "", fabricengine.Config{}, err
	}
	branch, err := fabricengine.NewTopology(cfg).PairTaskBranchOrOrigin(location, slug)
	return branch, cfg, err
}

// hubReadoptSeams builds the re-adoption seams over the prime location of a hub.
func hubReadoptSeams(location *lymcwd.Location) readoptSeams {
	return readoptSeams{
		Gone: func(slug string) (bool, error) {
			present, err := taskWorktreePresent(location, slug)
			return !present, err
		},
		Reconcile: func(slug string) (fabricengine.OriginReport, error) {
			return reconcileGoneRun(location, slug)
		},
		Prune: func() error { return fabricengine.PruneRegistrations(location) },
		Add: func(slug string) error {
			cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(location))
			if err != nil {
				return err
			}
			_, err = fabricengine.NewTopology(cfg).Add(location, slug, fabricengine.AddOptions{AdoptLocal: true, AdoptOnly: true})
			return err
		},
		SpawnPush: func(slug string) error {
			taskLocation, err := taskWorktreeLocation(location, slug)
			if err != nil {
				return err
			}
			return spawnPairPush(taskLocation)
		},
		Notice: func(slug, line string) error {
			_, err := notifyRunParent(location, slug, line)
			return err
		},
		CommitFailed: func(slug, producer string) error {
			deps := battenCommitStatusDeps(location, slug)
			if _, err := deps.Commit(statuscommit.Message(failedStopCommitPrefix, producer, string(shedengine.StateFailed))); err != nil {
				return err
			}
			return deps.Spawn()
		},
		RecordsRoot: func() (string, error) { return fabricengine.RecordsRepoRoot(location) },
		Tags: func(slug string) ([]fabricengine.RecordsTag, error) {
			rels, err := readoptRecordRels(slug)
			if err != nil {
				return nil, err
			}
			return fabricengine.SlugRecordsTags(location, slug, rels)
		},
		BranchFile: func(branch, anchoredRel string) ([]byte, bool, error) {
			return fabricengine.RecordsBranchFile(location, branch, anchoredRel)
		},
		Upstream: func(branch string) (bool, error) { return fabricengine.RecordsBranchHasUpstream(location, branch) },
		TaskBranch: func(slug string) (string, error) {
			branch, _, err := hubTaskBranch(location, slug)
			return branch, err
		},
		TaskBranchGone: func(slug string) (bool, error) {
			branch, cfg, err := hubTaskBranch(location, slug)
			if err != nil {
				return false, err
			}
			local, err := gitrepo.New(location.WorktreePath()).BranchExists(branch)
			if err != nil || local {
				return false, err
			}
			onOrigin, err := fabricengine.NewTopology(cfg).TaskBranchOnOrigin(location, slug)
			return !onOrigin, err
		},
	}
}

// reconcileGoneRun runs the run-scope origin reconcile of slug from the prime, judging a run whose pair worktree is absent.
// The run scope tears no pair down and moves no ref of the absent pair.
// Each entry of the report's mutation record is logged at Info, since a resume has no envelope to carry it.
// The report is returned with the error, so a partial record is still logged.
func reconcileGoneRun(location *lymcwd.Location, slug string) (fabricengine.OriginReport, error) {
	teardown, err := pairteardown.New(location)
	if err != nil {
		return fabricengine.OriginReport{}, err
	}
	report, err := fabricengine.ReconcileOrigin(location, fabricengine.ReconcileScope{Run: slug, PairMayBeAbsent: true}, teardown.OriginSeams(pairteardown.HubSeamOptions(location)), fabricengine.ReconcileOptions{})
	for _, mutation := range report.Mutations.Entries() {
		logger.Info("battencli: origin reconcile mutation", "slug", slug, "kind", string(mutation.Kind), "target", mutation.Target, "detail", mutation.Detail)
	}
	return report, err
}

// readoptGonePair resumes a run whose task worktree is gone: it judges origin, prunes the stale registrations, re-makes the pair over its live branches, spawns the pair's push, and then runs the resume reconcile over the re-made pair.
// An origin that ended, stopped or gave the run to another machine refuses, as any resume does, before the pair is touched.
// An unreachable origin refuses without writing the origin-owed marker, since a pair is never re-made unjudged.
// A pair that can never be re-made, one of readoptPermanent's answers, fails the run once with a way forward by row.
// Any other failure refuses the pass and leaves the status as it is.
func (c *battenCLI) readoptGonePair() error {
	rerun := fmt.Sprintf("re-run lym batten run %s", c.slug)
	report, err := c.readopt.Reconcile(c.slug)
	if err != nil {
		return fmt.Errorf("battencli: origin reconcile of %q failed: %w; fix the cause it names, then %s", c.slug, err, rerun)
	}
	if report.Unreachable {
		return fmt.Errorf("battencli: origin is unreachable, so the gone pair of %q cannot be judged; %s once origin answers", c.slug, rerun)
	}
	if err := resumeRefusal(c.slug, report); err != nil {
		return err
	}
	if err := c.readopt.Prune(); err != nil {
		return fmt.Errorf("battencli: pruning the stale worktree registrations of %q failed: %w; fix the cause it names, then %s", c.slug, err, rerun)
	}
	if err := c.readopt.Add(c.slug); err != nil {
		if !readoptPermanent(err) {
			return fmt.Errorf("battencli: re-making the pair of %q failed: %w; %s", c.slug, err, rerun)
		}
		status, found, readErr := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
		if readErr != nil || !found {
			return fmt.Errorf("battencli: re-making the pair of %q failed: %w; its status could not be read to record the stop (found %v, %v); %s", c.slug, err, found, readErr, rerun)
		}
		return c.failReadopt(status, err)
	}
	logger.Info("battencli: re-adopted the pair of a run whose task worktree was gone", "slug", c.slug)
	if err := c.readopt.SpawnPush(c.slug); err != nil {
		logger.Warn("battencli: spawn the re-made pair's push failed; its next push carries the branches", "slug", c.slug, "error", err)
	}
	return c.reconcileResume()
}
