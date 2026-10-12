// land.go wires the Land row: the seams that pull the prime, read the landing config, run the deploy command, prune the board and list the ready tasks.
// Every seam resolves what it touches on Call, never at wiring time.

package battencli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// pauseRequested returns the seam that reads the pause_requested flag of batten's own status file, the one `lyx batten pause` writes.
// An absent status file reads as not paused.
func pauseRequested(location *lyxcwd.Location, slug string) func() (bool, error) {
	return func() (bool, error) {
		status, found, err := state.ReadJSONStrict[shedengine.Status](StatusFile(location, slug), StatusLock(location, slug))
		if err != nil || !found {
			return false, err
		}
		return status.PauseRequested, nil
	}
}

// landDeps returns the Land row's seams over the prime location and the task worktree locator.
func landDeps(location *lyxcwd.Location, slug string, locateTask func() (*lyxcwd.Location, error)) battenshed.LandDeps {
	landLockPath := LandLock(location)
	return battenshed.LandDeps{
		ParentBranch: func() (string, error) {
			taskLocation, err := locateTask()
			if err != nil {
				return "", err
			}
			origin, found, err := fabricengine.ReadOrigin(taskLocation)
			if err != nil {
				return "", err
			}
			if !found {
				return "", fmt.Errorf("the task worktree has no origin record to read its parent branch from")
			}
			return origin.ParentBranch, nil
		},
		PrimeBranch: func() (string, error) {
			fab, err := fabricengine.Open(location)
			if err != nil {
				return "", err
			}
			return fab.CurrentBranch()
		},
		// PrimeDirty keeps only the paths under the prime's code worktree: the records side is pulled non-fatally by fabric's pull, and its run records change under every batten.
		PrimeDirty: func() ([]string, error) {
			fab, err := fabricengine.Open(location)
			if err != nil {
				return nil, err
			}
			paths, err := fab.DirtyPaths()
			if err != nil {
				return nil, err
			}
			codeRoot := location.WorktreePath() + string(filepath.Separator)
			var dirty []string
			for _, path := range paths {
				if strings.HasPrefix(path, codeRoot) {
					dirty = append(dirty, path)
				}
			}
			return dirty, nil
		},
		Pull: func(ctx context.Context) error {
			fab, err := fabricengine.Open(location)
			if err != nil {
				return err
			}
			res, err := fab.Pull(fabricengine.EnvSyncOptions())
			logger.Info("battencli: land pull", "slug", slug, "mutations", res.Mutated())
			return err
		},
		Landing: func() (battenshed.LandingSettings, error) {
			cfg, err := landingshed.LoadConfig(fabricengine.ConfigDirs(location), "landing")
			if err != nil {
				return battenshed.LandingSettings{}, err
			}
			return battenshed.LandingSettings{Deploy: cfg.Deploy, RequirePRToBase: cfg.RequirePRToBase}, nil
		},
		RunDeploy: func(ctx context.Context, command, logPath string, timeout time.Duration) (battenshed.DeployResult, error) {
			return runDeploy(ctx, location.WorktreePath(), command, logPath, timeout)
		},
		ReadyTasks: func() ([]string, error) {
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				return nil, err
			}
			tasks, err := board.ListTasksFull()
			if err != nil {
				return nil, err
			}
			return readySlugs(tasks), nil
		},
		PruneBoard: func() error {
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				return err
			}
			removed, err := board.Prune(true)
			logger.Info("battencli: land board prune", "slug", slug, "removed", removed)
			return err
		},
		LandLock: battenshed.PrimeLock{
			Path: landLockPath,
			Acquire: func() (release func() error, ok bool, err error) {
				if err := os.MkdirAll(filepath.Dir(landLockPath), 0o755); err != nil {
					return nil, false, err
				}
				fl, acquired, err := lock.TryAcquireWriteLock(landLockPath)
				if err != nil {
					return nil, false, err
				}
				if !acquired {
					return nil, false, nil
				}
				return fl.Release, true, nil
			},
		},
		PauseRequested: pauseRequested(location, slug),
		Notices:        rowNotices(location, slug),
	}
}

// readySlugs returns, sorted, the slugs of the tasks that can start now: kind task, unclaimed and with no dependency.
func readySlugs(tasks []boardengine.Task) []string {
	var ready []string
	for _, task := range tasks {
		if task.Kind == boardengine.KindTask && task.Status == nil && len(task.DependsOn) == 0 {
			ready = append(ready, task.Slug)
		}
	}
	slices.Sort(ready)
	return ready
}

// runDeploy runs command through the platform shell in primeDir with its output in the file logPath, ending it after timeout.
// A command that outlives the timeout reports TimedOut; a shell that could not start, or a cancelled ctx, is the returned error.
func runDeploy(ctx context.Context, primeDir, command, logPath string, timeout time.Duration) (battenshed.DeployResult, error) {
	logFile, err := os.Create(logPath)
	if err != nil {
		return battenshed.DeployResult{}, err
	}
	defer logFile.Close()

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	logger.Info("battencli: deploy starting", "dir", primeDir, "command", command, "log", logPath, "timeout", timeout)
	exitCode, err := verifyrun.Run(runCtx, command, primeDir, nil, logFile)
	logger.Info("battencli: deploy ended", "dir", primeDir, "exitCode", exitCode, "error", err)
	if err != nil {
		if ctx.Err() == nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return battenshed.DeployResult{TimedOut: true}, nil
		}
		return battenshed.DeployResult{}, err
	}
	return battenshed.DeployResult{ExitCode: exitCode}, nil
}
