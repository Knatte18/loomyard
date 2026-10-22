// hold.go declares PauseHold, the told pause hold a verify site gives Verify, and NewPauseHold, its one builder over the pause record.

package verifytree

import (
	"context"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// holdPollInterval is how often a built hold polls the record while a command runs, and the delay between the record reads of its wait.
const holdPollInterval = time.Second

// PauseHold is the pause hold a verify site tells Verify: when a covering pause stops the command and when the pause has lifted.
type PauseHold struct {
	// PastGrace reports whether a live entry covering the scope has passed its grace end.
	PastGrace func() bool
	// Wait returns once no live entry covers the scope, calls held once when it starts waiting, and returns ctx's error when ctx ends.
	Wait func(ctx context.Context, held func()) error
	// Poll is the interval Verify polls PastGrace at while its command runs.
	Poll time.Duration
	// Sleep waits d or until ctx ends, between the record reads of Wait.
	Sleep func(ctx context.Context, d time.Duration) error
}

// NewPauseHold builds the hold over the pause record geom names, judging geom's slug.
// While slugEntryExempt reports true only a hub entry counts, which serves a run holding its parent lock; nil never exempts.
// A geometry with no record reads uncovered, and an unreadable record never stops a verify.
// A test shortens the returned hold's Poll and Sleep.
func NewPauseHold(geom pauserecord.Geometry, slugEntryExempt func() bool) *PauseHold {
	hold := &PauseHold{Poll: holdPollInterval, Sleep: sleepOnTimer}
	var (
		mutex     sync.Mutex
		lastError string
	)
	judgedSlug := func() string {
		if slugEntryExempt != nil && slugEntryExempt() {
			return ""
		}
		return geom.Slug
	}
	hold.PastGrace = func() bool {
		now := time.Now()
		coverage, err := pauserecord.ReadCoverage(geom.RecordPath, judgedSlug(), now)
		mutex.Lock()
		defer mutex.Unlock()
		if err != nil {
			if err.Error() != lastError {
				lastError = err.Error()
				logger.Warn("verifytree: pause record unreadable, not stopping the verify", "cause", err)
			}
			return false
		}
		lastError = ""
		return coverage.Covered && !now.Before(coverage.Entry.GraceEnd)
	}
	hold.Wait = func(ctx context.Context, held func()) error {
		waiter := pauserecord.Waiter{
			Notify: func(line string) { logger.Info(line) },
			Sleep: func(ctx context.Context, d time.Duration) error {
				if held != nil {
					held()
					held = nil
				}
				return hold.Sleep(ctx, d)
			},
			SlugEntryExempt: slugEntryExempt,
		}
		return pauserecord.WaitUncovered(ctx, geom.RecordPath, geom.Slug, waiter)
	}
	return hold
}

// sleepOnTimer waits d on a timer, or until ctx ends.
func sleepOnTimer(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
