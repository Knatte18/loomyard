// deps.go fills the verbs' deps over a real hub: the board reads, the owner write and the merge task's filing, the batten run's status and first pass, the ready record, the landing's refusal and land request, the hub's Code-Review budget and the beam's change description.

package beamcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// hubCreateDeps fills createDeps over the hub the prime belongs to.
func hubCreateDeps(prime *lymcwd.Location) (createDeps, error) {
	config, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(prime))
	if err != nil {
		return createDeps{}, err
	}
	topology := fabricengine.NewTopology(config)
	boardDir := fabricengine.BoardDir(prime.HubPath)

	return createDeps{
		PrimeName:  prime.WorktreeName,
		Machine:    machinename.Name,
		ReadOwners: func() (boardengine.Owners, error) { return boardengine.ReadOwners(boardDir) },
		ParentOf: func(slug string) (string, error) {
			return hubgeom.ParentOf(prime, slug)
		},
		ParentBeam: func(slug string) (string, error) {
			tasks, err := boardengine.ReadTasks(boardDir)
			if err != nil {
				return "", err
			}
			return boardengine.ParentBeam(tasks, slug), nil
		},
		PairPresent: func(slug string) bool { return pairWorktreePresent(prime, slug) },
		BranchOnOrigin: func(slug string) (bool, error) {
			return topology.TaskBranchOnOrigin(prime, slug)
		},
		SetOwners: func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
			return boardengine.SetOwners(boardDir, req)
		},
		RunStatus: func(slug string) (shedengine.State, bool, error) {
			status, found, err := state.ReadJSON[shedengine.Status](battencli.StatusFile(prime, slug), shedrun.StatusLock(prime, slug))
			if errors.Is(err, fs.ErrNotExist) {
				return "", false, nil
			}
			return status.State, found, err
		},
		ClearMotifRun: func(slug string) (bool, error) {
			return battencli.ClearMotifRun(prime, slug)
		},
		StartRun: func(slug, anchor string) (map[string]any, error) {
			return battencli.StartFirstPass(prime, slug, battencli.FirstPassOptions{Anchor: anchor})
		},
		WorktreePath: func(slug string) string { return fabricengine.WorktreePath(prime, slug) },
	}, nil
}

// hubLandDeps fills landDeps over the hub the prime belongs to.
func hubLandDeps(prime *lymcwd.Location) landDeps {
	beamOf := func(slug string) (*lymcwd.Location, error) {
		return lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
	}
	// beamOrigin returns the beam's location with the origin record of its pair.
	beamOrigin := func(slug string) (*lymcwd.Location, fabricengine.Origin, error) {
		beam, err := beamOf(slug)
		if err != nil {
			return nil, fabricengine.Origin{}, err
		}
		origin, found, err := fabricengine.ReadOriginFor(prime, slug)
		if err != nil {
			return nil, fabricengine.Origin{}, err
		}
		if !found {
			return nil, fabricengine.Origin{}, fmt.Errorf("the pair of the beam %s has no readable origin record; way forward: run `lym fabric reconcile` to restore its records worktree, then run the verb again", slug)
		}
		return beam, origin, nil
	}
	// beamAndParent returns the beam's location with the parent branch its origin record names.
	beamAndParent := func(slug string) (*lymcwd.Location, string, error) {
		beam, origin, err := beamOrigin(slug)
		if err != nil {
			return nil, "", err
		}
		return beam, origin.ParentBranch, nil
	}
	// openAPI opens the pull request API of the repository originURL names.
	openAPI := func(originURL string) (pullrequest.API, error) {
		client, err := pullrequest.Open(originURL)
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	// beamSideFacts reads the beam pair's origin facts, branch and parent branch, and leaves the API unopened.
	beamSideFacts := func(slug string) (beamPullRequestSide, error) {
		beam, parentBranch, err := beamAndParent(slug)
		if err != nil {
			return beamPullRequestSide{}, err
		}
		pair, err := fabricengine.Open(beam)
		if err != nil {
			return beamPullRequestSide{}, err
		}
		url, err := pair.OriginURL()
		if err != nil {
			logger.Warn("beamcli: the beam pair's origin URL is unreadable, reading the pair as having no origin", "beam", slug, "cause", err)
			url = ""
		}
		tips, err := pair.ReadParentTips(parentBranch, false)
		if err != nil {
			return beamPullRequestSide{}, err
		}
		branch, err := pair.CurrentBranch()
		if err != nil {
			return beamPullRequestSide{}, err
		}
		return beamPullRequestSide{
			Origin:       landingshed.Origin{URL: url, Upstream: tips.Upstream != "", PushSkipped: fabricengine.EnvSyncOptions().SkipPush},
			Branch:       branch,
			ParentBranch: parentBranch,
		}, nil
	}
	return landDeps{
		Machine: machinename.Name,
		ReadOwners: func() (boardengine.Owners, error) {
			return boardengine.ReadOwners(fabricengine.BoardDir(prime.HubPath))
		},
		ParentOf: func(slug string) (string, error) {
			return hubgeom.ParentOf(prime, slug)
		},
		PairPresent: func(slug string) bool { return pairWorktreePresent(prime, slug) },
		OriginRecord: func(slug string) (fabricengine.Origin, bool, error) {
			return fabricengine.ReadOriginFor(prime, slug)
		},
		ChildStatus: func(slug string) (shedengine.Status, bool, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return shedengine.Status{}, false, err
			}
			statusPath := shedrun.StatusFile(beam, shedrun.SelfRunID)
			if _, err := os.Stat(statusPath); errors.Is(err, fs.ErrNotExist) {
				return shedengine.Status{}, false, nil
			}
			lockPath := shedrun.StatusLock(beam, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
				return shedengine.Status{}, false, err
			}
			return state.ReadJSON[shedengine.Status](statusPath, lockPath)
		},
		RunID: func(slug string) (string, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return "", err
			}
			seed, found, err := shedrun.ReadSeed(beam, shedrun.SelfRunID)
			switch {
			case err != nil:
				return "", err
			case !found || seed.StartedAt == "":
				return "", fmt.Errorf("the pawl run of %s has no seed start time", slug)
			}
			return seed.StartedAt, nil
		},
		Ready: func(slug string) (bool, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return false, err
			}
			return pawlengine.ReadyFor(loomengine.BeamReadyPath(beam), loomengine.BeamReadyLockPath(beam), slug)
		},
		MarkReady: func(slug string, record pawlengine.ReadyRecord) (string, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return "", err
			}
			path := loomengine.BeamReadyPath(beam)
			flagged, err := pawlengine.ReadyFor(path, loomengine.BeamReadyLockPath(beam), slug)
			if err != nil {
				return "", err
			}
			if !flagged {
				if err := pawlengine.WriteReady(path, loomengine.BeamReadyLockPath(beam), record); err != nil {
					return "", err
				}
			}
			_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), beam, []string{loomengine.LandingDirRel()}, fmt.Sprintf("beam: %s is ready to merge", slug), fabricengine.EnvSyncOptions())
			return path, err
		},
		Refusal: func(slug string) error { return battencli.BeamLandRefusal(prime, slug) },
		WriteRequest: func(slug string, req pawlengine.LandRequest) (string, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return "", err
			}
			paths := hubgeom.PawlPaths(beam)
			return paths.LandRequest, pawlengine.WriteLandRequest(paths, req)
		},
		Now: time.Now,
		Tasks: func() ([]boardengine.Task, error) {
			return boardengine.ReadTasks(fabricengine.BoardDir(prime.HubPath))
		},
		// The board gives a new task the draft status, so the filing releases it with a second write.
		// The writes carry no strand name, so the run-agent task guard does not judge them.
		FileTask: func(fields map[string]any) error {
			board, err := boardengine.OpenHub(prime.HubPath)
			if err != nil {
				return err
			}
			task, err := board.UpsertTask(fields)
			if err != nil {
				return err
			}
			if err := board.SetStatus(task.Slug, nil); err != nil {
				return fmt.Errorf("the task %s was created but not released and stands as a draft with no run: remove it with `lym board remove %s` before filing again: %w", task.Slug, task.Slug, err)
			}
			return nil
		},
		// The budget is the prime's loom config, the hub's own, which no run profile reaches.
		CodeReviewCaps: func() (reviewbudget.Caps, error) {
			cfg, err := loomengine.LoadConfig(fabricengine.ConfigDirs(prime), "loom")
			if err != nil {
				return reviewbudget.Caps{}, err
			}
			return cfg.Caps(loomengine.PerchCode), nil
		},
		// The stamp runs inside the description's own commit, after the description validated, so one commit holds both files and a refused description stamps nothing.
		Describe: func(slug string, at time.Time) (string, error) {
			beam, parentBranch, err := beamAndParent(slug)
			if err != nil {
				return "", err
			}
			write := beamDescribeDeps(beam, slug, parentBranch, loomengine.ProjectDecisionRecord(beam, slug))
			commit := write.Commit
			write.Commit = func() error {
				stamp := func(rec *pawlengine.LandingRecord) error {
					rec.DescribedAt = at
					return nil
				}
				if err := pawlengine.UpdateLanding(loomengine.BeamLandingPath(beam), loomengine.BeamLandingLockPath(beam), slug, stamp); err != nil {
					return err
				}
				return commit()
			}
			if err := landingshed.WriteBeamDescription(write); err != nil {
				return "", err
			}
			return write.DescriptionPath, nil
		},
		// The API opens only when the origin allows a pull request, so a missing origin fact reaches the verb as a refusal naming the fact.
		PullRequestSide: func(slug string) (beamPullRequestSide, error) {
			side, err := beamSideFacts(slug)
			if err != nil {
				return beamPullRequestSide{}, err
			}
			// A failure to open the API is the side's, so a verb that needs no API, such as `ready` for a beam with none recorded, is not stopped by an unusable token.
			if landingshed.OriginMissing(side.Origin) == "" {
				side.API, side.APIErr = openAPI(side.Origin.URL)
			}
			return side, nil
		},
		PushBeam: func(slug string) (string, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return "", err
			}
			if _, err := fabricengine.PushPairAnchored(beam, fabricengine.EnvSyncOptions(), fabricengine.StatusPushLockWait); err != nil {
				return "", err
			}
			pair, err := fabricengine.Open(beam)
			if err != nil {
				return "", err
			}
			return pair.HeadSHA()
		},
		// The read is the one Finalize makes of a task pair, so every verb that judges an empty beam judges it alike, records-only commits counted.
		BeamHasCommits: func(slug string) (bool, error) {
			beam, parentBranch, err := beamAndParent(slug)
			if err != nil {
				return false, err
			}
			pair, err := fabricengine.Open(beam)
			if err != nil {
				return false, err
			}
			return pair.HeadHoldsCommitsNotIn("refs/heads/" + parentBranch)
		},
		BeamHead: func(slug string) (string, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return "", err
			}
			pair, err := fabricengine.Open(beam)
			if err != nil {
				return "", err
			}
			return pair.HeadSHA()
		},
		// The parent pair is the one the beam's origin record names, so the receiving branch's own beam.yaml decides, as it does for a run that lands on it.
		PullRequestMode: func(slug string) (string, bool, error) {
			_, origin, err := beamOrigin(slug)
			if err != nil {
				return "", false, err
			}
			parent := prime
			if origin.ParentWorktree != "" {
				if parent, err = lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, origin.ParentWorktree)); err != nil {
					return "", false, err
				}
			}
			cfg, err := dobbyengine.LoadConfig(fabricengine.ConfigDirs(parent))
			if err != nil {
				return "", false, err
			}
			side, err := beamSideFacts(slug)
			if err != nil {
				return "", false, err
			}
			required, err := parentRuleRequired(context.Background(), side.Origin, side.ParentBranch, openAPI)
			return cfg.PullRequest, required, err
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
		// The parent's tips are read after a fetch, under the remote-refs lock, when the parent has an upstream and the hub's push skip is unset, so the judgment reads the state origin holds.
		ParentUnmerged: func(slug string) (bool, error) {
			beam, parentBranch, err := beamAndParent(slug)
			if err != nil {
				return false, err
			}
			pair, err := fabricengine.Open(beam)
			if err != nil {
				return false, err
			}
			tips, err := pair.ReadParentTips(parentBranch, false)
			if err != nil {
				return false, err
			}
			if tips.Upstream != "" && !fabricengine.EnvSyncOptions().SkipPush {
				if tips, err = pair.ReadParentTips(parentBranch, true); err != nil {
					return false, err
				}
			}
			head, err := pair.HeadSHA()
			if err != nil {
				return false, err
			}
			for _, tip := range []string{tips.Local, tips.Upstream} {
				if tip == "" {
					continue
				}
				held, err := pair.IsAncestor(tip, head)
				if err != nil {
					return false, err
				}
				if !held {
					return true, nil
				}
			}
			return false, nil
		},
		DescriptionFindings: func(slug string) ([]string, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return nil, err
			}
			findings, err := summaryparser.ValidateDescription(summaryparser.Path(loomengine.LandingDir(beam)))
			if err != nil {
				return nil, err
			}
			var lines []string
			for _, finding := range findings {
				lines = append(lines, finding.Error())
			}
			return lines, nil
		},
		// A pawl run's Finalize reads a merged pull request through the same list, so ready and Finalize agree on what one lacks.
		MergedLacks: func(slug, mergedHead string) ([]string, error) {
			beam, parentBranch, err := beamAndParent(slug)
			if err != nil {
				return nil, err
			}
			return fabricengine.BeamCommitsPast(beam, parentBranch, mergedHead)
		},
		Landing: func(slug string) (pawlengine.LandingRecord, bool, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return pawlengine.LandingRecord{}, false, err
			}
			return pawlengine.ReadLanding(loomengine.BeamLandingPath(beam), loomengine.BeamLandingLockPath(beam), slug)
		},
		WriteLanding: func(slug string, mutate func(rec *pawlengine.LandingRecord) error) error {
			beam, err := beamOf(slug)
			if err != nil {
				return err
			}
			if err := pawlengine.UpdateLanding(loomengine.BeamLandingPath(beam), loomengine.BeamLandingLockPath(beam), slug, mutate); err != nil {
				return err
			}
			_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), beam, []string{loomengine.LandingDirRel()}, fmt.Sprintf("beam: record the pull request of %s", slug), fabricengine.EnvSyncOptions())
			return err
		},
		Description: func(slug string) (string, string, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return "", "", err
			}
			summary, err := summaryparser.Parse(summaryparser.Path(loomengine.LandingDir(beam)))
			if err != nil {
				return "", "", err
			}
			return summary.Title, strings.TrimSpace(summary.Body), nil
		},
	}
}

// beamDescribeDeps builds the description writer's deps over the beam pair at location, whose slug names its board entry, whose parentBranch is the branch it lands on and whose project record is at recordPath.
// Every closure reads or writes on demand and opens nothing at build time.
// Commits lists the beam's line without the records-only commits, so a description lists neither its own commits nor the run's status commits.
// An empty parentBranch, a pair with no origin record, is refused when the beam's commits are read.
func beamDescribeDeps(location *lymcwd.Location, slug, parentBranch, recordPath string) landingshed.BeamDescribeDeps {
	return landingshed.BeamDescribeDeps{
		Slug:            slug,
		DescriptionPath: summaryparser.Path(loomengine.LandingDir(location)),
		ProjectDecisions: func() ([]string, error) {
			return discussionparser.AddedDecisionTexts(recordPath)
		},
		Entry: func() (string, string, error) {
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				return "", "", err
			}
			task, found, err := board.GetTask(slug)
			if err != nil {
				return "", "", err
			}
			if !found {
				return "", "", fmt.Errorf("the board holds no entry %q; way forward: restore the entry with \"lym board upsert\", then run \"lym beam describe %s\" again", slug, slug)
			}
			return task.Title, task.Brief, nil
		},
		Commits: func() ([]landingshed.BeamCommit, error) {
			line, err := fabricengine.BeamLineCommits(location, parentBranch)
			if err != nil {
				return nil, err
			}
			var commits []landingshed.BeamCommit
			for _, commit := range line {
				if !commit.RecordsOnly {
					commits = append(commits, landingshed.BeamCommit{SHA: commit.SHA, Message: commit.Message, Committed: commit.Committed})
				}
			}
			return commits, nil
		},
		// Commit makes a repeat commit over an unchanged directory a no-op.
		Commit: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.LandingDirRel()}, fmt.Sprintf("beam: change description for %s", slug), fabricengine.EnvSyncOptions())
			return err
		},
	}
}

// pairWorktreePresent reports whether this machine holds the worktree of slug's pair.
func pairWorktreePresent(prime *lymcwd.Location, slug string) bool {
	info, err := os.Stat(fabricengine.WorktreePath(prime, slug))
	return err == nil && info.IsDir()
}
