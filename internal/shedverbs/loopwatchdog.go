// loopwatchdog.go is the loop's step watchdog: it watches a running child for activity and kills one that shows none for the idle window.

package shedverbs

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
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
	// noKillWindow is the window of a watch that only wakes its hook: far longer than any child runs, so the watch never ends the child.
	noKillWindow = 24 * 365 * 100 * time.Hour
)

// WatchdogKill is what the step watchdog read when it killed a child.
type WatchdogKill struct {
	// TraceID is the killed step's trace id.
	TraceID string
	// Window is the idle window the child exceeded.
	Window time.Duration
	// Idle is the awake, unpaused idle time at the kill.
	Idle time.Duration
	// LastActivity is the newest activity time the watch read, the child's start when it read none.
	LastActivity time.Time
}

// cause renders the interrupted cause of the killed step: what the watchdog read, then note when the kill's hook returned one.
func (k WatchdogKill) cause(note string) string {
	cause := fmt.Sprintf("watchdog: the step watchdog killed the step after %s of awake idle time (window %s, last activity %s)",
		k.Idle.Round(time.Second), k.Window.Round(time.Second), k.LastActivity.Format(time.RFC3339))
	if note != "" {
		cause += "; " + note
	}
	return cause
}

// idleWatch is what watchIdle watches.
type idleWatch struct {
	// TraceID is the watched child's trace id.
	TraceID string
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
	// Wake runs on each wake, before the idle time is judged; nil is skipped.
	Wake func()
	// Awake is the awake-clock reading the watch sets against Now to see a sleep.
	Awake func() time.Duration
	// Pause reports what the pause record says about the run; nil never covers.
	Pause func(now time.Time) (pauserecord.Coverage, error)
}

// watchIdle watches until the window passes with no activity, and returns what it read and true.
// The idle time is awake, unpaused time: the sleeps the watch sees between its own wakes and the intervals a pause covered the run do not count.
// While a pause covers the run it judges nothing, so the child is never killed, and it keeps waking on its backoff.
// It returns false when ctx ends first, which is how a child that exited stops its watch.
// It wakes at most once a second, doubling from one second to a one-minute ceiling while the reading is unchanged and returning to one second on a change.
// It never sleeps past the end of the window's remaining awake time.
func watchIdle(ctx context.Context, w idleWatch) (WatchdogKill, bool) {
	newest := w.Started
	var delay time.Duration
	var sleeps pauserecord.SleepWatch
	pauseReadFailed := false
	var activityFailures logger.Episode
	for {
		if w.Wake != nil {
			w.Wake()
		}
		var files []string
		if w.TraceFiles != nil {
			files = w.TraceFiles()
		}
		changed := false
		if seen, found := newestActivity(files, w.Activity, &activityFailures); found && seen.After(newest) {
			newest, changed = seen, true
		}
		now := w.Now()
		sleeps.Observe(now, w.Awake())
		var coverage pauserecord.Coverage
		if w.Pause != nil {
			var err error
			if coverage, err = w.Pause(now); err != nil {
				coverage = pauserecord.Coverage{}
				if !pauseReadFailed {
					logger.Warn("shed: the watchdog could not read the pause record and reads the run uncovered", "error", err.Error())
				}
				pauseReadFailed = true
			}
		}
		if changed || delay == 0 {
			delay = idleWakeFloor
		} else {
			delay = min(delay*2, idleWakeCeiling)
		}
		if coverage.Covered {
			if err := w.Sleep(ctx, delay); err != nil {
				return WatchdogKill{}, false
			}
			continue
		}
		idle := pauserecord.AwakeSince(newest, now, sleeps.Sleeps(), coverage.Intervals)
		if idle >= w.Window {
			return WatchdogKill{TraceID: w.TraceID, Window: w.Window, Idle: idle, LastActivity: newest}, true
		}
		if err := w.Sleep(ctx, max(min(delay, w.Window-idle), idleWakeFloor)); err != nil {
			return WatchdogKill{}, false
		}
	}
}

// newestActivity returns the newest of the modification times of traceFiles and the agent reading, and false when neither answers.
// It stats only the paths it is told.
// An agent read error is logged through failures, owned by the caller for the length of its loop: a Warn at the first failure, Debug while it repeats and an Info when the agent answers again.
func newestActivity(traceFiles []string, agent func() (time.Time, bool, error), failures *logger.Episode) (time.Time, bool) {
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
			failures.Fail("shed: the watchdog could not read the run's agent activity", "error", err.Error())
		} else {
			failures.Recover("shed: the watchdog reads the run's agent activity again")
			if answered && (!found || at.After(newest)) {
				newest, found = at, true
			}
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
// A window of zero waits for the exit alone, or, when loop.OnWake is set, runs the same watch with a window no child reaches, so the hook wakes until the child exits and nothing is killed.
// It returns what the watchdog read when it killed the child, nil for one that exited on its own, and the error the child's Wait reported.
func waitForChild(ctx context.Context, loop LoopSpec, child Child, started time.Time, traceID string, window time.Duration, waitDone <-chan error) (*WatchdogKill, error) {
	if window == 0 && loop.OnWake == nil {
		return nil, <-waitDone
	}
	if window == 0 {
		window = noKillWindow
	}
	now := loop.Now
	if now == nil {
		now = time.Now
	}
	sleep := loop.Sleep
	if sleep == nil {
		sleep = sleepFor
	}
	awake := loop.Awake
	if awake == nil {
		awake = pauserecord.MonotonicAwake()
	}
	watchCtx, stopWatching := context.WithCancel(ctx)
	defer stopWatching()
	type idleOutcome struct {
		kill  WatchdogKill
		found bool
	}
	outcome := make(chan idleOutcome, 1)
	go func() {
		kill, found := watchIdle(watchCtx, idleWatch{
			TraceID:    traceID,
			Started:    started,
			Window:     window,
			TraceFiles: func() []string { return traceFilesOf(loop, traceID) },
			Activity:   loop.Activity,
			Now:        now,
			Sleep:      sleep,
			Wake:       loop.OnWake,
			Awake:      awake,
			Pause:      loop.Pause,
		})
		outcome <- idleOutcome{kill, found}
	}()

	select {
	case err := <-waitDone:
		stopWatching()
		<-outcome
		return nil, err
	case watched := <-outcome:
		if !watched.found {
			return nil, <-waitDone
		}
		return &watched.kill, killIdleChild(ctx, sleep, child, waitDone, watched.kill)
	}
}

// killIdleChild asks child to dump its goroutines with Quit, gives it quitGrace to exit through sleep, then kills it with its descendants.
// Both its Warn lines carry what the watchdog read, so the step's trace names the reading.
// It returns the error the child's Wait reported.
func killIdleChild(ctx context.Context, sleep func(context.Context, time.Duration) error, child Child, waitDone <-chan error, kill WatchdogKill) error {
	record := child.Record()
	reading := []any{"window", kill.Window.String(), "last_activity", kill.LastActivity.Format(time.RFC3339), "idle", kill.Idle.String()}
	if err := child.Quit(); err != nil {
		logger.Warn("shed: the watchdog could not ask the idle child to dump its goroutines", "pid", record.PID, "error", err.Error())
	}
	logger.Warn("shed: the watchdog asked the idle child to dump its goroutines", append([]any{"pid", record.PID, "group", record.PGID}, reading...)...)

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

	logger.Warn("shed: the watchdog kills the idle child", append([]any{"pid", record.PID, "group", record.PGID}, append(reading, "exited_within_grace", exited)...)...)
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
