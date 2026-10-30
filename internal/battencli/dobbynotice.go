// dobbynotice.go fills dobbyengine's run notice seams over a real hub: a run's child status, driver and agent runs, its pause coverage, the API-error revive and the path of the run's notice marker.

package battencli

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/tendercli"
)

// DobbyRunNotices returns the seams of the run notices a dobby of prime's hub sends for the runs it drives, reading the pause record at pauseRecordPath, with quiet as the quiet window.
// Each run's task worktree is resolved once and kept, as batten's own wait keeps it.
func DobbyRunNotices(prime *lymcwd.Location, pauseRecordPath string, quiet time.Duration) dobbyengine.RunNoticeDeps {
	locators := map[string]func() (*lymcwd.Location, error){}
	locate := func(slug string) func() (*lymcwd.Location, error) {
		if locator, ok := locators[slug]; ok {
			return locator
		}
		locator := taskWorktreeLocator(prime, slug)
		locators[slug] = locator
		return locator
	}
	return dobbyengine.RunNoticeDeps{
		ReadChild: func(slug string) (dobbyengine.ChildReading, bool, error) {
			return readNoticeChild(prime, slug, locate(slug))
		},
		DriverAlive: func(_ context.Context, slug string) (bool, error) {
			return noticeDriverAlive(prime, slug, locate(slug))
		},
		Agents: func(_ context.Context, slug string) ([]dobbyengine.AgentRun, bool, error) {
			taskLocation, err := locate(slug)()
			if err != nil {
				return nil, false, err
			}
			return readAgentActivity(taskLocation)
		},
		Pause: func(slug string) (pauserecord.Coverage, error) {
			return pauserecord.ReadCoverage(pauseRecordPath, slug, time.Now())
		},
		ReviveAPIError: func(_ context.Context, slug string) error {
			return tendercli.ReviveStalled(prime, slug)
		},
		MarkerPath: func(slug string) string { return shedrun.NoticeEpisodeFile(prime, slug) },
		Quiet:      quiet,
	}
}

// readNoticeChild reads the child of slug for the run notices: its status, the status file's modification time, the task worktree's directory and whether batten's Run-Shed row respawned it the cap of times.
// An absent task worktree or child status reads as not found.
func readNoticeChild(prime *lymcwd.Location, slug string, locateTask func() (*lymcwd.Location, error)) (dobbyengine.ChildReading, bool, error) {
	present, err := taskWorktreePresent(prime, slug)
	if err != nil || !present {
		return dobbyengine.ChildReading{}, false, err
	}
	taskLocation, err := locateTask()
	if err != nil {
		return dobbyengine.ChildReading{}, false, err
	}
	statusPath := shedrun.StatusFile(taskLocation, shedrun.SelfRunID)
	info, err := os.Stat(statusPath)
	if os.IsNotExist(err) {
		return dobbyengine.ChildReading{}, false, nil
	}
	if err != nil {
		return dobbyengine.ChildReading{}, false, err
	}
	statusLockPath := shedrun.StatusLock(taskLocation, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		return dobbyengine.ChildReading{}, false, err
	}
	status, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	if err != nil || !found {
		return dobbyengine.ChildReading{}, false, err
	}
	// An unreadable memory record reads as not capped: the cap only words a driver-dead notice, so it must not hide the run's conditions.
	capped, err := battenshed.RespawnCapReached(shedrun.ScratchDir(prime, slug), battenrecipe.NameRunShed, info.ModTime())
	if err != nil {
		logger.Debug("battencli: the run's respawn memory is unreadable; the child reads as not capped", "slug", slug, "error", err)
		capped = false
	}
	child := dobbyengine.ChildReading{
		State:           string(status.State),
		CurrentProducer: status.CurrentProducer,
		Error:           status.Error,
		HistoryLen:      len(status.History),
		Modified:        info.ModTime(),
		PairDir:         taskLocation.AnchorPath(),
	}
	if capped {
		child.RespawnCap = battenshed.RespawnCap
	}
	if n := len(status.History); n > 0 {
		if at, err := time.Parse(time.RFC3339, status.History[n-1].At); err == nil {
			child.NewestHistory = at
		}
	}
	return child, true, nil
}

// noticeDriverAlive reports whether the child of slug has a live driver: a live driver strand, or a held driver lock or run lock.
// A lock that cannot be read reads as held, since a lock read must not make a working driver look dead.
func noticeDriverAlive(prime *lymcwd.Location, slug string, locateTask func() (*lymcwd.Location, error)) (bool, error) {
	alive, err := childDriverAlive(prime, slug, locateTask)
	if err != nil || alive {
		return alive, err
	}
	taskLocation, err := locateTask()
	if err != nil {
		return false, err
	}
	return childDriverLockHeld(slug, taskLocation), nil
}

// childDriverLockHeld reports whether the driver lock or the run lock of the child in taskLocation is held.
// A lock that cannot be read reads as held, since a lock read must not make a working driver look dead.
func childDriverLockHeld(slug string, taskLocation *lymcwd.Location) bool {
	for _, lockPath := range []string{shedrun.DriverLock(taskLocation, shedrun.SelfRunID), shedrun.RunLock(taskLocation, shedrun.SelfRunID)} {
		held, err := runLockHeld(lockPath)
		if err != nil {
			logger.Debug("battencli: a child lock is unreadable; the driver reads as alive", "slug", slug, "lock", lockPath, "error", err)
			return true
		}
		if held {
			return true
		}
	}
	return false
}
