// land.go wires the Land row: the seams that pull the prime, read the landing config, run the deploy command, prune the board, list the ready tasks and route the newly ready ones to the orch that may start them.
// Every seam resolves what it touches on Call, never at wiring time.

package battencli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// pauseRequested returns the seam that reads the pause_requested flag of batten's own status file, the one `lyx batten pause` writes.
// An absent status file reads as not paused.
func pauseRequested(location *lyxcwd.Location, slug string) func() (bool, error) {
	return func() (bool, error) {
		status, found, err := state.ReadJSONNoMkdir[shedengine.Status](StatusFile(location, slug), StatusLock(location, slug))
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
		ReadyRoutes: func(ready []string) ([]battenshed.ReadyRoute, error) {
			return landReadyRoutes(location, slug, ready)
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

// pullStopOutcome splits a pull's error into the Land row's two outcomes.
// An err that is exactly a records-side stop is a records side that did not pull: its message comes back as recordsStop with a nil pullErr, so the landing goes on.
// Any other error, one joining a code-side failure to a records-side stop included, comes back as pullErr, so an unpulled prime is never deployed.
func pullStopOutcome(err error) (recordsStop string, pullErr error) {
	if message, ok := fabricengine.RecordsStopMessage(err); ok {
		return message, nil
	}
	return "", err
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

// readyRoutes splits the newly ready slugs by the orch that may start each: the prime for a slug with no beam, and a beam for one in a beam whose orch is live.
// A slug in a beam whose orch beamOrchLive reports not live is in neither answer, since no orch it would reach may start it.
// A liveness error is returned.
func readyRoutes(ready []string, parentBeam func(slug string) string, beamOrchLive func(beam string) (bool, error)) (prime []string, beams map[string][]string, err error) {
	beams = map[string][]string{}
	live := map[string]bool{}
	for _, slug := range ready {
		beam := parentBeam(slug)
		if beam == "" {
			prime = append(prime, slug)
			continue
		}
		isLive, known := live[beam]
		if !known {
			isLive, err = beamOrchLive(beam)
			if err != nil {
				return nil, nil, err
			}
			live[beam] = isLive
		}
		if isLive {
			beams[beam] = append(beams[beam], slug)
		}
	}
	return prime, beams, nil
}

// landReadyRoutes builds the Land row's ready routes for the newly ready slugs over the board as the row reads it.
// The prime's route is walked when the landing line, which starts at the run slug's pair, ends at the prime's orch.
// A beam's route needs the beam's worktree present here and its orch live, and posts to that orch with no walk.
func landReadyRoutes(prime *lyxcwd.Location, slug string, ready []string) ([]battenshed.ReadyRoute, error) {
	board, err := boardengine.OpenHub(prime.HubPath)
	if err != nil {
		return nil, err
	}
	tasks, err := board.ListTasksFull()
	if err != nil {
		return nil, err
	}
	beamLocations := map[string]*lyxcwd.Location{}
	primeSlugs, beams, err := readyRoutes(ready,
		func(slug string) string { return boardengine.ParentBeam(tasks, slug) },
		func(beam string) (bool, error) {
			location, found, err := presentWorktree(prime, beam)
			if err != nil || !found {
				return false, err
			}
			beamLocations[beam] = location
			return orchcli.OrchLive(location)
		})
	if err != nil {
		return nil, err
	}
	var routes []battenshed.ReadyRoute
	if len(primeSlugs) > 0 {
		walked, err := landingLineReachesPrime(prime, slug)
		if err != nil {
			return nil, err
		}
		routes = append(routes, battenshed.ReadyRoute{Key: "", Slugs: primeSlugs, Walked: walked, Notify: notifyOrchAt(prime)})
	}
	for _, beam := range slices.Sorted(maps.Keys(beams)) {
		routes = append(routes, battenshed.ReadyRoute{Key: beam, Slugs: beams[beam], Notify: notifyOrchAt(beamLocations[beam])})
	}
	return routes, nil
}

// notifyOrchAt returns a route's notify seam: batten's line to the orch of worktree itself.
func notifyOrchAt(worktree *lyxcwd.Location) func(ctx context.Context, line string) (bool, error) {
	return func(_ context.Context, line string) (bool, error) {
		return orchcli.NotifyOrchAt(worktree, battenProgram, line)
	}
}

// landingLineReachesPrime reports whether the walk a notice of the run slug takes ends at the prime's orch.
func landingLineReachesPrime(prime *lyxcwd.Location, slug string) (bool, error) {
	site, isPair, err := runNoticeSite(prime, slug)
	if err != nil {
		return false, err
	}
	start := site
	if isPair {
		parent, err := hubgeom.ResolveParent(site)
		if err != nil {
			return false, err
		}
		start = prime
		if parent.Worktree != "" {
			location, found, err := presentWorktree(prime, parent.Worktree)
			if err != nil {
				return false, err
			}
			if found {
				start = location
			}
		}
	}
	destination, err := hubgeom.NearestLiveOrch(start, orchcli.OrchLive)
	if err != nil {
		return false, err
	}
	return destination.Worktree == "", nil
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

// runRowCommand runs command through the platform shell in dir with its output in the file logPath, ending it after timeout.
// A command that outlives the timeout reports TimedOut; a shell that could not start, or a cancelled ctx, is the returned error.
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
	exitCode, err := verifyrun.Run(runCtx, command, dir, nil, logFile)
	logger.Info("battencli: row command ended", "dir", dir, "exitCode", exitCode, "error", err)
	if err != nil {
		if ctx.Err() == nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return shedadapters.CommandResult{TimedOut: true}, nil
		}
		return shedadapters.CommandResult{}, err
	}
	return shedadapters.CommandResult{ExitCode: exitCode}, nil
}
