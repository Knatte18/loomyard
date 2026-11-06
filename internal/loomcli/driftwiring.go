// driftwiring.go builds the seams the Drift rows are told: the run status, the git facts, the records commit, the expect and judge agents' tables and the capture and compare runs.
// Every seam opens the fabric or reads a file on demand, since wire also runs for status and pause.

package loomcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// driftCaptureRecordWait bounds how long a detached capture start waits for its child's running record.
	driftCaptureRecordWait = 30 * time.Second
	// driftCaptureRecordPoll is the interval a detached capture start reads that record at.
	driftCaptureRecordPoll = 50 * time.Millisecond
	// driftCaptureLockRetry bounds how long a capture retries a capture lock a reader's probe holds.
	driftCaptureLockRetry = 2 * time.Second
	// driftCompareLockRetry bounds how long a compare retries a compare marker a reader's probe holds.
	driftCompareLockRetry = 2 * time.Second
)

// driftTables carries the values the expect and judge agents' tables are built from, which wire resolves once.
type driftTables struct {
	// registry resolves the agents' model-specs.
	registry modelspec.Registry
	// subject is driftengine.SubjectDecisionRecord or driftengine.SubjectBoardEntry.
	subject string
	// slug is the board entry's slug, read for the board-entry subject.
	slug string
	// decisionRecordPath is the absolute path of the task's decision record.
	decisionRecordPath string
	// planDir is the absolute path of the task's plan directory, which the expect agent reads, and empty for a run that has no plan.
	planDir string
	// rows is the run's recipe row order, which the seal check reads against the status row.
	rows []string
	// runID is the run's id, whose shed directory loom writes during a row.
	runID string
	// endRoles ends this worktree's live strands of the told roles, which a run of Drift-Baseline and Drift-Verify tells the roles of the removed drift agents.
	endRoles func(roles []string) error
}

// driftRecordsDeps builds the drift Deps whose seams read the committed drift records over location's worktree, which is all the verdict reads.
func driftRecordsDeps(location *lymcwd.Location) driftengine.Deps {
	return driftengine.Deps{
		Paths: driftengine.NewPaths(location.AnchorPath()),
		TrailerCommit: func(key string, newest bool) (fabricengine.TrailerCommit, bool, error) {
			return fabricengine.FindTrailerCommit(location, key, newest)
		},
		FileAt: func(rev, relPath string) ([]byte, bool, error) {
			return fabricengine.AnchoredFileAt(location, rev, relPath)
		},
	}
}

// driftDeps builds the seams the Drift rows read over location's worktree.
// statusPath and statusLockPath locate the run status.
// The seat runner, clock and gate findings file are left unset, since the registry entries fill them from the recipe's environment.
func driftDeps(location *lymcwd.Location, statusPath, statusLockPath string, tables driftTables) driftengine.Deps {
	paths := driftengine.NewPaths(location.AnchorPath())
	pool, waitDir := hubgeom.GateSlots(location), gateslot.WaitDir(location.AnchorPath())
	config := func() (driftengine.Config, error) {
		return driftengine.LoadConfig(fabricengine.ConfigDirs(location))
	}
	status := func() (shedengine.Status, error) {
		st, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
		if err != nil {
			return shedengine.Status{}, err
		}
		if !found {
			return shedengine.Status{}, errors.New("drift: this worktree has no run status; way forward: lym loom resume")
		}
		return st, nil
	}
	head := func() (string, error) {
		fabric, err := fabricengine.Open(location)
		if err != nil {
			return "", err
		}
		return fabric.HeadSHA()
	}
	branchPoint := pairMergeBase(location)
	dirtyPaths := func() ([]string, error) { return fabricengine.SourceDirtyPaths(location) }
	checkMain := func() (string, error) {
		st, err := status()
		if err != nil {
			return "", err
		}
		headSHA, err := head()
		if err != nil {
			return "", err
		}
		base, err := branchPoint()
		if err != nil {
			return "", err
		}
		dirty, err := dirtyPaths()
		if err != nil {
			return "", err
		}
		if err := driftengine.CheckMainCapture(st, headSHA, base, dirty); err != nil {
			return "", err
		}
		return headSHA, nil
	}
	stepOf := func(name string) (driftengine.CommandStep, error) {
		cfg, err := config()
		if err != nil {
			return driftengine.CommandStep{}, err
		}
		return cfg.StepWithHubEnv(name, fabricengine.ConfigDirs(location))
	}

	return driftengine.Deps{
		Paths:        paths,
		WorktreeRoot: location.WorktreePath(),
		Config:       config,
		Status:       status,
		Rows:         tables.rows,
		Head:         head,
		BranchPoint:  branchPoint,
		DirtyPaths:   dirtyPaths,
		Commit: func(relPaths []string) error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, relPaths, fmt.Sprintf("loom: drift records for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		CommitWithTrailers: func(relPaths []string, trailers []fabricengine.Trailer) error {
			message := fmt.Sprintf("loom: drift records for %s", seedSlug(location.WorktreeName))
			_, _, err := fabricengine.CommitAnchoredPathsWithTrailers(fabricengine.NewMutations(""), location, relPaths, message, trailers, fabricengine.EnvSyncOptions())
			return err
		},
		TrailerCommit: func(key string, newest bool) (fabricengine.TrailerCommit, bool, error) {
			return fabricengine.FindTrailerCommit(location, key, newest)
		},
		FileAt: func(rev, relPath string) ([]byte, bool, error) {
			return fabricengine.AnchoredFileAt(location, rev, relPath)
		},
		CommitsSince: func(base string) ([]string, error) {
			fabric, err := fabricengine.Open(location)
			if err != nil {
				return nil, err
			}
			return fabric.CommitLog(base, "HEAD")
		},
		ExpectTable: func(listing string) (seatengine.Table, error) {
			cfg, err := config()
			if err != nil {
				return seatengine.Table{}, err
			}
			scope, _, err := driftengine.ReadScope(paths)
			if err != nil {
				return seatengine.Table{}, err
			}
			mode := scope.Diff
			if mode == "" {
				mode = cfg.Diff
			}
			return driftengine.ExpectTable(driftengine.ExpectInputs{
				Subject:            tables.subject,
				Slug:               tables.slug,
				DecisionRecordPath: tables.decisionRecordPath,
				PlanDir:            tables.planDir,
				BaselineDir:        paths.BaselineDir,
				ListingPath:        listing,
				ExpectedPath:       paths.Expected,
				DiffMode:           mode,
			}, cfg, tables.registry)
		},
		JudgeTable: func(request string) (seatengine.Table, error) {
			cfg, err := config()
			if err != nil {
				return seatengine.Table{}, err
			}
			return driftengine.JudgeTable(driftengine.JudgeInputs{
				Subject:            tables.subject,
				Slug:               tables.slug,
				DecisionRecordPath: tables.decisionRecordPath,
				RequestPath:        request,
				JudgementPath:      paths.Judgement,
			}, cfg, tables.registry)
		},
		Checkouts: func() (fabricengine.CheckoutState, fabricengine.CheckoutState, error) {
			return fabricengine.CheckoutStates(location)
		},
		RecordsChangedSince: func(rev string) ([]string, error) {
			return fabricengine.RecordsPathsChangedSince(location, rev)
		},
		StartCapture: func() error {
			executable, err := os.Executable()
			if err != nil {
				return fmt.Errorf("drift: resolve this binary: %w", err)
			}
			_, err = driftengine.StartCapture(captureSpawnRequest(location, paths, executable))
			return err
		},
		RunCapture: func(ctx context.Context) (driftengine.CaptureRecord, error) {
			step, err := stepOf("capture")
			if err != nil {
				return driftengine.CaptureRecord{}, err
			}
			request := driftCaptureRequest(location, paths, pool, waitDir, step)
			request.Check = checkMain
			request.LockRetry = driftCaptureLockRetry
			return driftengine.RunCapture(ctx, request)
		},
		CaptureCurrent: func(ctx context.Context) (driftengine.CurrentCapture, error) {
			step, err := stepOf("capture")
			if err != nil {
				return driftengine.CurrentCapture{}, err
			}
			request := driftCaptureRequest(location, paths, pool, waitDir, step)
			return driftengine.CaptureCurrent(ctx, request)
		},
		Sleep: sleepContext,
		Compare: func(ctx context.Context) (driftengine.CompareResult, error) {
			step, err := stepOf("compare")
			if err != nil {
				return driftengine.CompareResult{}, err
			}
			request := driftCompareRequest(location, paths, pool, waitDir, step)
			request.LockRetry = driftCompareLockRetry
			return driftengine.RunCompare(ctx, request)
		},
		JudgeSnapshotDir: filepath.Join(fabricengine.HubScratchDir(location.HubPath), lymdirs.DriftJudgeDirName, seedSlug(location.WorktreeName)),
		LoomRecordsDirs:  loomRecordsDirs(location, tables.runID),
		EndRoles:         tables.endRoles,
	}
}

// strandRoleStopper returns the seam that ends the chairs of the told roles, built over two seams so a test needs no tmux:
// status reads reed's strand table, which is this worktree's own, and stop ends one strand by guid.
// The table is read at call time, and every non-retiring strand whose role is exactly one of the told roles, a numbered form of it excluded, is stopped when it is live or an own-window strand whose pane is present.
// The first stop that fails ends the pass and is returned, naming the strand.
func strandRoleStopper(
	status func() (reedengine.StatusResult, error),
	stop func(guid string) error,
) func(roles []string) error {
	return func(roles []string) error {
		result, err := status()
		if err != nil {
			return err
		}
		for _, strand := range result.Strands {
			if !(strand.Live || strand.OwnWindow && strand.Present) || strand.Retiring {
				continue
			}
			name, err := agentname.Parse(strand.Name)
			if err != nil || !slices.Contains(roles, name.Role) {
				continue
			}
			if err := stop(strand.GUID); err != nil {
				return fmt.Errorf("loomcli: stop the strand %s: %w", strand.GUID, err)
			}
		}
		return nil
	}
}

// loomRecordsDirs returns the records paths loom itself writes during a row, relative to the records checkout's root and slash-separated:
// the run's shed directory, the usage directory every session of the run records into, loom's friction directory and the prefix of its timestamped archive siblings, each under the location's anchor.
func loomRecordsDirs(location *lymcwd.Location, runID string) []string {
	underAnchor := func(rel string) string { return filepath.ToSlash(filepath.Join(location.AnchorRel, rel)) }
	friction := underAnchor(loomengine.LoomFrictionDirRel())
	return []string{underAnchor(shedrun.RunDirRel(location, runID)), underAnchor(shedrun.UsageDirRel()), friction, friction + "-"}
}

// driftCaptureRequest describes a capture of step over location's worktree, which the caller completes with its precondition and lock bound.
// A relative step dir is joined onto the worktree root, and a step with no dir runs in the anchored directory.
// InheritedSlot is the slot a gate this process runs inside holds, read from the process environment because the engine never reads it.
func driftCaptureRequest(location *lymcwd.Location, paths driftengine.Paths, pool *gateslot.Pool, waitDir string, step driftengine.CommandStep) driftengine.CaptureRequest {
	return driftengine.CaptureRequest{
		Paths:         paths,
		WorktreeDir:   location.WorktreePath(),
		AnchorDir:     location.AnchorPath(),
		Step:          step,
		Pool:          pool,
		WaitDir:       waitDir,
		InheritedSlot: os.Getenv(gateslot.InheritEnv),
		Timeout:       time.Duration(step.TimeoutMin) * time.Minute,
		Now:           time.Now,
	}
}

// driftCompareRequest describes a compare of step over location's worktree, which the caller completes with its lock bound.
// A relative step dir is joined onto the worktree root, and a step with no dir runs in the anchored directory.
func driftCompareRequest(location *lymcwd.Location, paths driftengine.Paths, pool *gateslot.Pool, waitDir string, step driftengine.CommandStep) driftengine.CompareRequest {
	return driftengine.CompareRequest{
		Paths:       paths,
		WorktreeDir: location.WorktreePath(),
		AnchorDir:   location.AnchorPath(),
		Step:        step,
		Pool:        pool,
		WaitDir:     waitDir,
		Timeout:     time.Duration(step.TimeoutMin) * time.Minute,
		Now:         time.Now,
	}
}

// captureSpawnRequest describes the start of the detached capture child of executable over location.
// The child runs in the anchored directory, since every lym verb resolves its worktree from a cwd equal to it.
func captureSpawnRequest(location *lymcwd.Location, paths driftengine.Paths, executable string) driftengine.SpawnRequest {
	return driftengine.SpawnRequest{
		Executable: executable,
		AnchorDir:  location.AnchorPath(),
		Paths:      paths,
		RunProcDir: fabricengine.RunProcDir(location.HubPath),
		Owner:      location.WorktreeName,
		RecordWait: driftCaptureRecordWait,
		Poll:       driftCaptureRecordPoll,
	}
}

// sleepContext waits d, or returns the context's error once ctx ends first.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
