// loopwatchdog.go is the loop's step watchdog: it watches a running child for activity and kills one that shows none for the idle window.

package shedverbs

import (
	"context"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// quitGrace is how long a child gets to exit after Quit, so its goroutine dump reaches its stderr file, before it is killed.
	quitGrace = 5 * time.Second
	// idleWindowFloor is the smallest idle window; a shorter configured one is raised to it.
	idleWindowFloor = time.Minute
	// idleWakeFloor is the watchdog's first wake and the wake it returns to when activity changed.
	idleWakeFloor = time.Second
	// idleWakeCeiling is the longest wake the watchdog backs off to while the reading is unchanged.
	idleWakeCeiling = time.Minute
)

// idleWatch is what watchIdle watches.
type idleWatch struct {
	// Started is when the child started; the idle time counts from it until the first activity.
	Started time.Time
	// Window is how long the child may show no activity.
	Window time.Duration
	// TraceFiles lists the child's trace files, which it opens after it starts, so the watch asks on each wake.
	TraceFiles func() []string
	// Activity is the arming module's reading of the run's agent activity.
	Activity func() (time.Time, bool, error)
	// Now is the clock.
	Now func() time.Time
	// Sleep waits for a duration or until its context ends.
	Sleep func(ctx context.Context, delay time.Duration) error
}

// watchIdle watches until the window passes with no activity, and returns the idle time and true.
// It returns false when ctx ends first, which is how a child that exited stops its watch.
// It wakes at most once a second, doubling from one second to a one-minute ceiling while the reading is unchanged and returning to one second on a change.
// It never sleeps past the end of the window.
func watchIdle(ctx context.Context, w idleWatch) (time.Duration, bool) {
	newest := w.Started
	var delay time.Duration
	for {
		var files []string
		if w.TraceFiles != nil {
			files = w.TraceFiles()
		}
		changed := false
		if seen, found := newestActivity(files, w.Activity); found && seen.After(newest) {
			newest, changed = seen, true
		}
		idle := w.Now().Sub(newest)
		if idle >= w.Window {
			return idle, true
		}
		if changed || delay == 0 {
			delay = idleWakeFloor
		} else {
			delay = min(delay*2, idleWakeCeiling)
		}
		if err := w.Sleep(ctx, max(min(delay, w.Window-idle), idleWakeFloor)); err != nil {
			return 0, false
		}
	}
}

// newestActivity returns the newest of the modification times of traceFiles and the agent reading, and false when neither answers.
// It stats only the paths it is told.
func newestActivity(traceFiles []string, agent func() (time.Time, bool, error)) (time.Time, bool) {
	var newest time.Time
	found := false
	for _, file := range traceFiles {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		if modified := info.ModTime(); !found || modified.After(newest) {
			newest, found = modified, true
		}
	}
	if agent != nil {
		at, answered, err := agent()
		if err != nil {
			logger.Warn("shed: the watchdog could not read the run's agent activity", "error", err.Error())
		} else if answered && (!found || at.After(newest)) {
			newest, found = at, true
		}
	}
	return newest, found
}

// idleWindowOf is the idle window loop arms: zero disarms the watchdog, and a window below idleWindowFloor is raised to it with one Warn.
func idleWindowOf(loop LoopSpec) time.Duration {
	if loop.IdleTimeout > 0 && loop.IdleTimeout < idleWindowFloor {
		logger.Warn("shed: the loop's idle window is below the floor and is raised to it", "window", loop.IdleTimeout.String(), "floor", idleWindowFloor.String())
		return idleWindowFloor
	}
	return max(loop.IdleTimeout, 0)
}

// waitForChild waits for child's exit, which the goroutine behind waitDone reports, and kills the child when it shows no activity for window.
// A window of zero waits for the exit alone.
// It returns the idle time of a child the watchdog killed, which is zero for one that exited on its own, and the error the child's Wait reported.
func waitForChild(ctx context.Context, loop LoopSpec, child Child, started time.Time, traceID string, window time.Duration, waitDone <-chan error) (time.Duration, error) {
	if window == 0 {
		return 0, <-waitDone
	}
	now := loop.Now
	if now == nil {
		now = time.Now
	}
	sleep := loop.Sleep
	if sleep == nil {
		sleep = sleepFor
	}
	watchCtx, stopWatching := context.WithCancel(ctx)
	defer stopWatching()
	type idleOutcome struct {
		idle  time.Duration
		found bool
	}
	outcome := make(chan idleOutcome, 1)
	go func() {
		idle, found := watchIdle(watchCtx, idleWatch{
			Started:    started,
			Window:     window,
			TraceFiles: func() []string { return traceFilesOf(loop, traceID) },
			Activity:   loop.Activity,
			Now:        now,
			Sleep:      sleep,
		})
		outcome <- idleOutcome{idle, found}
	}()

	select {
	case err := <-waitDone:
		stopWatching()
		<-outcome
		return 0, err
	case idle := <-outcome:
		if !idle.found {
			return 0, <-waitDone
		}
		return idle.idle, killIdleChild(ctx, sleep, child, waitDone, idle.idle)
	}
}

// killIdleChild asks child to dump its goroutines with Quit, gives it quitGrace to exit through sleep, then kills it with its descendants.
// It returns the error the child's Wait reported.
func killIdleChild(ctx context.Context, sleep func(context.Context, time.Duration) error, child Child, waitDone <-chan error, idle time.Duration) error {
	record := child.Record()
	if err := child.Quit(); err != nil {
		logger.Warn("shed: the watchdog could not ask the idle child to dump its goroutines", "pid", record.PID, "error", err.Error())
	}
	logger.Warn("shed: the watchdog asked the idle child to dump its goroutines", "pid", record.PID, "group", record.PGID, "idle", idle.String())

	graceCtx, endGrace := context.WithCancel(ctx)
	graceOver := make(chan struct{})
	go func() {
		_ = sleep(graceCtx, quitGrace)
		close(graceOver)
	}()
	var waitErr error
	exited := false
	select {
	case waitErr = <-waitDone:
		exited = true
	case <-graceOver:
		select {
		case waitErr = <-waitDone:
			exited = true
		default:
		}
	}
	endGrace()

	logger.Warn("shed: the watchdog kills the idle child", "pid", record.PID, "group", record.PGID, "idle", idle.String(), "exited_within_grace", exited)
	if err := child.Kill(); err != nil {
		logger.Warn("shed: the watchdog could not kill the idle child", "pid", record.PID, "error", err.Error())
	}
	if !exited {
		waitErr = <-waitDone
	}
	return waitErr
}

// sleepFor waits for delay, or until ctx ends and then returns its error.
func sleepFor(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
