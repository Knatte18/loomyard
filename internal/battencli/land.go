// land.go wires the Land row's seams: the prime pull, the landing config, the deploy command and the board prune.
// Every seam resolves what it touches when it runs, never at wiring time.

package battencli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// pauseRequested returns the seam that reads the pause_requested flag of batten's own status file.
// `lym batten pause` writes that file.
// An absent status file reads as not paused.
func pauseRequested(location *lymcwd.Location, slug string) func() (bool, error) {
	return func() (bool, error) {
		status, found, err := state.ReadJSON[shedengine.Status](StatusFile(location, slug), StatusLock(location, slug))
		if err != nil || !found {
			return false, err
		}
		return status.PauseRequested, nil
	}
}

// landDeps returns the Land row's seams over the prime location and the task worktree locator.
func landDeps(location *lymcwd.Location, slug string, locateTask func() (*lymcwd.Location, error)) battenshed.LandDeps {
	landLockPath := LandLock(location)
	return battenshed.LandDeps{
		ChildLanded: func() (bool, error) {
			taskLocation, err := locateTask()
			if err != nil {
				return false, err
			}
			return childFinalized(taskLocation)
		},
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
		// PrimeDirty keeps only the paths under the prime's code worktree:
		// the records side is pulled non-fatally by fabric's pull, and its run records change under every batten.
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
		Pull: func(ctx context.Context) (string, error) {
			fab, err := fabricengine.Open(location)
			if err != nil {
				return "", err
			}
			res, err := fab.Pull(fabricengine.EnvSyncOptions())
			logger.Info("battencli: land pull", "slug", slug, "mutations", res.Mutated())
			return pullStopOutcome(err)
		},
		Landing: func() (battenshed.LandingSettings, error) {
			cfg, err := landingshed.LoadConfig(fabricengine.ConfigDirs(location), "landing")
			if err != nil {
				return battenshed.LandingSettings{}, err
			}
			return battenshed.LandingSettings{Deploy: cfg.Deploy, DeployBranches: cfg.DeployBranches}, nil
		},
		RunDeploy: func(ctx context.Context, command, logPath string, timeout time.Duration) (battenshed.DeployResult, error) {
			return RunDeploy(ctx, location.WorktreePath(), command, logPath, timeout)
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

// childFinalized reports whether the run in taskLocation finished a Finalize:
// its status history holds a Finalize with outcome Done.
// A run with no status file, or none that finalized, such as a motif run, landed no code.
func childFinalized(taskLocation *lymcwd.Location) (bool, error) {
	status, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(taskLocation, shedrun.SelfRunID), shedrun.StatusLock(taskLocation, shedrun.SelfRunID))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil || !found {
		return false, err
	}
	return slices.ContainsFunc(status.History, func(entry shedengine.HistoryEntry) bool {
		return entry.Producer == loomshed.NameFinalize && entry.Outcome == shedengine.Done
	}), nil
}

// pullStopOutcome splits a pull's error into the Land row's two outcomes.
// An err that is exactly a records-side stop is a records side that did not pull:
// its message comes back as recordsStop with a nil pullErr, so the landing goes on.
// Any other error comes back as pullErr, so an unpulled prime is never deployed.
// That includes an error joining a code-side failure to a records-side stop.
func pullStopOutcome(err error) (recordsStop string, pullErr error) {
	if message, ok := fabricengine.RecordsStopMessage(err); ok {
		return message, nil
	}
	return "", err
}

// RunDeploy runs command through the platform shell in primeDir with its output in the file logPath.
// It ends the command once timeout elapses.
// A command that outlives the timeout reports TimedOut;
// a shell that could not start, or a cancelled ctx, is the returned error.
func RunDeploy(ctx context.Context, primeDir, command, logPath string, timeout time.Duration) (battenshed.DeployResult, error) {
	logFile, err := os.Create(logPath)
	if err != nil {
		return battenshed.DeployResult{}, err
	}
	defer logFile.Close()

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	logger.Info("battencli: deploy starting", "dir", primeDir, "command", command, "log", logPath, "timeout", timeout)
	exitCode, err := verifyrun.Run(runCtx, command, primeDir, commandEnv(os.Environ()), logFile)
	logger.Info("battencli: deploy ended", "dir", primeDir, "exitCode", exitCode, "error", err)
	if err != nil {
		if ctx.Err() == nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return battenshed.DeployResult{TimedOut: true}, nil
		}
		return battenshed.DeployResult{}, err
	}
	return battenshed.DeployResult{ExitCode: exitCode}, nil
}

// runRowCommand runs command through the platform shell in dir with its output in the file logPath.
// It ends the command once timeout elapses.
// A command that outlives the timeout reports TimedOut;
// a shell that could not start, or a cancelled ctx, is the returned error.
func runRowCommand(ctx context.Context, dir, command, logPath string, timeout time.Duration) (shedadapters.CommandResult, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return shedadapters.CommandResult{}, err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return shedadapters.CommandResult{}, err
	}
	defer logFile.Close()

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	logger.Info("battencli: row command starting", "dir", dir, "command", command, "log", logPath, "timeout", timeout)
	exitCode, err := verifyrun.Run(runCtx, command, dir, commandEnv(os.Environ()), logFile)
	logger.Info("battencli: row command ended", "dir", dir, "exitCode", exitCode, "error", err)
	if err != nil {
		if ctx.Err() == nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return shedadapters.CommandResult{TimedOut: true}, nil
		}
		return shedadapters.CommandResult{}, err
	}
	return shedadapters.CommandResult{ExitCode: exitCode}, nil
}

// commandEnv returns environ without the strand-name variable,
// so a command a batten row runs never reads as an agent's own, and a gate call inside it runs in-process.
func commandEnv(environ []string) []string {
	return slices.DeleteFunc(slices.Clone(environ), func(entry string) bool {
		return strings.HasPrefix(entry, agentname.StrandNameEnv+"=")
	})
}
