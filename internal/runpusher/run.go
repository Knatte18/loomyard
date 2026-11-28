// run.go is the pusher's body: it takes the worktree's pusher lock and pushes until the worktree is level with origin.
// It pushes at most once per interval in force and keeps that timing in the push ledger.

package runpusher

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/pushledger"
)

const (
	// defaultInterval is the time between two pushes when the timing names none.
	defaultInterval = time.Minute
	// defaultCeiling is the longest wait before a retry when the timing names none.
	defaultCeiling = 5 * time.Minute
	// markWakeFloor is the least time from a wait's start to an early wake.
	markWakeFloor = time.Second
)

// errPauseCovers ends an attempt before the guard and the push, since a pause covers the run.
var errPauseCovers = errors.New("runpusher: a pause covers the run")

// stopError wraps the failure of a read or the guard that ends the pusher, so no retry treats it as an owed push.
type stopError struct {
	cause error
}

// Error returns the wrapped failure's message.
func (e *stopError) Error() string { return e.cause.Error() }

// Unwrap returns the wrapped failure.
func (e *stopError) Unwrap() error { return e.cause }

// Deps holds the seams Run drives, so a test drives its pass with fakes.
type Deps struct {
	// Lock tries the pusher lock without waiting.
	// It reports acquired false when another pusher holds it, and release frees a lock it took.
	Lock func() (release func(), acquired bool, err error)
	// Ahead reports whether the worktree holds commits origin lacks.
	Ahead func() (bool, error)
	// Conditions reports whether the start conditions but the lock still hold.
	Conditions func() (bool, error)
	// PauseCovers reports whether a pause covers the run, and is nil for the prime.
	PauseCovers func() (bool, error)
	// Guard runs before each push and stops the pusher with an error, and is nil for the prime.
	Guard func() error
	// Push pushes the worktree once.
	Push func() error
	// WorktreeGone reports whether the worktree no longer stands, holding no `.git` entry.
	WorktreeGone func() (bool, error)
	// Owed reports whether a failed push is owed, so the pusher retries it.
	Owed func(err error) bool
	// Timing is the push interval and its ceiling; a zero value reads as one minute and five minutes.
	Timing fabricengine.PushTiming
	// HubPath is the hub whose push ledger holds the target's entry.
	HubPath string
	// Key names the target's entry in the ledger.
	Key pushledger.Key
	// Now reads the clock the intervals are measured on.
	Now func() time.Time
	// Wait waits d, returning early with nil when wake delivers or the target's entry asks for a push now.
	// It returns ctx's error once ctx ends.
	Wait func(ctx context.Context, d time.Duration, wake <-chan struct{}) error
	// Parent holds the seams of the parent step, filled for a target runs land on.
	Parent ParentDeps
}

// NewDeps fills every seam of t that reads the worktree, the hub or the clock, pushing on timing.
// A pair gets the pause read of its run and the board the gate's, and the prime and the landing target none.
// The landing target and a beam pair also get their parent seams.
// The caller fills Push and Guard, and for a parent target its PushTip, HandOver and, for a beam, Refresh.
func NewDeps(t Target, timing fabricengine.PushTiming) Deps {
	hubPath := t.Location.HubPath
	key := ledgerKey(t)
	deps := Deps{
		Lock:  func() (func(), bool, error) { return tryLock(LockPath(t)) },
		Ahead: func() (bool, error) { return targetAhead(t) },
		Conditions: func() (bool, error) {
			stands, err := TargetRefused(t)
			if err != nil || stands {
				return false, err
			}
			if !t.Pair {
				return true, nil
			}
			marked, err := handedOver(t)
			return !marked, err
		},
		WorktreeGone: func() (bool, error) { return worktreeGone(t.Location.WorktreePath()) },
		Owed:         func(err error) bool { return errors.Is(err, errParentOwed) || fabricengine.IsPushOwed(err) },
		Timing:       timing,
		HubPath:      hubPath,
		Key:          key,
		Now:          time.Now,
		Wait:         waitForPushNow(hubPath, key),
	}
	switch {
	case t.Pair:
		deps.PauseCovers = func() (bool, error) { return pauseCovers(t) }
	case t.Kind == KindBoard:
		deps.PauseCovers = func() (bool, error) { return pauserecord.GateCovered(), nil }
	}
	if t.Kind == KindLanding || beamPair(t) {
		deps.Parent = parentSeams(t)
	}
	return deps
}

// waitForPushNow returns a wait that ends after d, or early once the entry of key carries the push-now mark.
// An early wake never comes within a second of the wait's start.
// A ledger directory that cannot be watched leaves the wait on its timer.
func waitForPushNow(hubPath string, key pushledger.Key) func(context.Context, time.Duration, <-chan struct{}) error {
	return func(ctx context.Context, d time.Duration, wake <-chan struct{}) error {
		started := time.Now()
		timer := time.NewTimer(d)
		defer timer.Stop()

		entryPath := pushledger.EntryPath(hubPath, key)
		var changes <-chan fswatch.Event
		watcher, err := fswatch.Watch(filepath.Dir(entryPath), filepath.Base(entryPath))
		if err != nil {
			logger.Warn("runpusher: could not watch the push ledger, waiting on the timer only", "target", key, "error", err)
		} else {
			defer watcher.Close()
			changes = watcher.Events()
		}
		marked := func() bool {
			entry, _, err := pushledger.Read(hubPath, key)
			if err != nil {
				logger.Debug("runpusher: push ledger entry not read during a wait", "target", key, "error", err)
			}
			return entry.PushNow
		}

		// The first read covers a mark written before the watch opened.
		for woken := marked(); !woken; {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			case <-wake:
				woken = true
			case _, open := <-changes:
				if !open {
					changes = nil
					continue
				}
				woken = marked()
			}
		}

		floor := time.NewTimer(markWakeFloor - time.Since(started))
		defer floor.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-floor.C:
			return nil
		}
	}
}

// Run is the pusher of t: it pushes the worktree's commits to origin in order, and returns once it is level or must stop.
// It takes the pusher lock and returns at once when another pusher holds it.
// It pushes at most once per interval in force, measured from the last attempt the ledger holds.
// A target's entry that asks for a push now skips that wait.
// Each push attempt reads the pause first, then runs the guard, then pushes.
// A pause covering the run ends the pusher before the guard and the push.
// A guard stop returns its error, pushing nothing.
// A failure deps.Owed does not read as owed returns, a refused push and a hand-over among them.
// An owed push is retried after an interval that doubles up to the ceiling, quietly.
// Once level it releases the lock and reads again.
// A commit made while it exits is pushed by it or by that commit's pusher.
func Run(ctx context.Context, t Target, deps Deps) error {
	release, acquired, err := deps.Lock()
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	for {
		stopped, err := pushUntilLevel(ctx, t, deps)
		release()
		if stopped || err != nil {
			return err
		}
		more, err := workRemains(deps)
		if err != nil || !more {
			return err
		}
		release, acquired, err = deps.Lock()
		if err != nil {
			return err
		}
		if !acquired {
			return nil
		}
	}
}

// pushUntilLevel pushes while the worktree is ahead and the conditions hold.
// It reports stopped when a condition, the pause or the guard ended the pass, and the failure of a push or a read as err.
func pushUntilLevel(ctx context.Context, t Target, deps Deps) (stopped bool, err error) {
	onMark := false
	for {
		if err := ctx.Err(); err != nil {
			return true, err
		}
		proceed, err := canPush(deps)
		if err != nil {
			return false, err
		}
		if !proceed {
			return true, nil
		}
		ahead, err := deps.Ahead()
		if err != nil {
			return false, err
		}
		if !ahead {
			return false, settleLevelEntry(t, deps)
		}
		if err := markOwed(t, deps); err != nil {
			return false, err
		}
		if onMark {
			proceed, err := awaitMark(ctx, deps)
			if err != nil || !proceed {
				return true, err
			}
		}
		if err := awaitInterval(ctx, t, deps); err != nil {
			return true, err
		}
		before, _, err := pushledger.Read(deps.HubPath, deps.Key)
		if err != nil {
			return false, err
		}

		pushErr := attemptPush(ctx, t, deps)
		var stop *stopError
		switch {
		case errors.Is(pushErr, errPauseCovers), errors.Is(pushErr, errParentRefused):
			return true, nil
		case errors.As(pushErr, &stop):
			return true, stop.cause
		}
		if err := recordAttempt(t, deps, pushErr); err != nil {
			return false, err
		}
		if pushErr != nil && !deps.Owed(pushErr) {
			return true, pushErr
		}

		onMark = false
		if pushErr == nil && deps.Key == pushledger.LandingKey {
			after, _, err := pushledger.Read(deps.HubPath, deps.Key)
			if err != nil {
				return false, err
			}
			onMark = len(after.Landings) > 0 && slices.Equal(before.Landings, after.Landings)
		}
	}
}

// markOwed writes the target's entry as owed from now at the base interval, unless it already reads so.
func markOwed(t Target, deps Deps) error {
	entry, found, err := pushledger.Read(deps.HubPath, deps.Key)
	if err != nil {
		return err
	}
	if found && !entry.FirstOwed.IsZero() {
		return nil
	}
	return updateEntry(t, deps, func(entry *pushledger.Entry) {
		entry.FirstOwed = deps.Now()
		entry.IntervalS = int(baseInterval(deps) / time.Second)
	})
}

// awaitMark waits until the target's entry asks for a push now or holds no landing.
// It reports proceed false when the start conditions stop holding while it waits.
func awaitMark(ctx context.Context, deps Deps) (proceed bool, err error) {
	for {
		entry, _, err := pushledger.Read(deps.HubPath, deps.Key)
		if err != nil {
			return false, err
		}
		if entry.PushNow || len(entry.Landings) == 0 {
			return true, nil
		}
		if err := deps.Wait(ctx, ceilingInterval(deps), nil); err != nil {
			return false, err
		}
		if proceed, err := canPush(deps); err != nil || !proceed {
			return false, err
		}
	}
}

// awaitInterval waits out what is left of the interval in force since the target's last attempt.
// An entry asking for a push now is cleared of the mark and waits no more.
func awaitInterval(ctx context.Context, t Target, deps Deps) error {
	for {
		entry, _, err := pushledger.Read(deps.HubPath, deps.Key)
		if err != nil {
			return err
		}
		if entry.PushNow {
			return updateEntry(t, deps, func(entry *pushledger.Entry) { entry.PushNow = false })
		}
		if entry.LastAttempt.IsZero() {
			return nil
		}
		interval := baseInterval(deps)
		if entry.IntervalS > 0 {
			interval = time.Duration(entry.IntervalS) * time.Second
		}
		remaining := entry.LastAttempt.Add(interval).Sub(deps.Now())
		if remaining <= 0 {
			return nil
		}
		if err := deps.Wait(ctx, remaining, nil); err != nil {
			return err
		}
	}
}

// recordAttempt writes the time of a push attempt into the target's entry, with the interval its result sets.
// A landed push returns the interval to the base, and an owed failure doubles it up to the ceiling.
func recordAttempt(t Target, deps Deps, pushErr error) error {
	return updateEntry(t, deps, func(entry *pushledger.Entry) {
		entry.LastAttempt = deps.Now()
		switch {
		case pushErr == nil:
			entry.IntervalS = int(baseInterval(deps) / time.Second)
		case deps.Owed(pushErr):
			current := max(time.Duration(entry.IntervalS)*time.Second, baseInterval(deps))
			entry.IntervalS = int(min(current*2, ceilingInterval(deps)) / time.Second)
		}
	})
}

// settleLevelEntry removes the entry of a level target.
// An entry holding landings is kept as it is.
// One holding only held records, dropped landings or owed hand-overs is kept with no timing,
// so it reads held and not owed.
func settleLevelEntry(t Target, deps Deps) error {
	_, found, err := pushledger.Read(deps.HubPath, deps.Key)
	if err != nil || !found {
		return err
	}
	return pushledger.Update(deps.HubPath, deps.Key, deps.Now(), func(entry pushledger.Entry, found bool) (pushledger.Entry, bool, error) {
		switch {
		case !found:
			return entry, false, nil
		case len(entry.Landings) > 0:
			return entry, true, nil
		case entry.HeldRecords == "" && len(entry.Dropped) == 0 && len(entry.HandOvers) == 0:
			return entry, false, nil
		}
		nameTarget(&entry, t, deps.Key)
		entry.FirstOwed = time.Time{}
		entry.LastAttempt = time.Time{}
		entry.IntervalS = 0
		return entry, true, nil
	})
}

// updateEntry writes the target's entry as mutate changes it, creating it when absent.
func updateEntry(t Target, deps Deps, mutate func(*pushledger.Entry)) error {
	return pushledger.Update(deps.HubPath, deps.Key, deps.Now(), func(entry pushledger.Entry, _ bool) (pushledger.Entry, bool, error) {
		nameTarget(&entry, t, deps.Key)
		mutate(&entry)
		return entry, true, nil
	})
}

// nameTarget sets the worktree and slug of a records target's entry; the board and landing entries name none here.
func nameTarget(entry *pushledger.Entry, t Target, key pushledger.Key) {
	if key == pushledger.BoardKey || key == pushledger.LandingKey {
		return
	}
	entry.Worktree = t.Location.WorktreePath()
	entry.Slug = t.Slug
}

func baseInterval(deps Deps) time.Duration {
	if deps.Timing.Interval > 0 {
		return deps.Timing.Interval
	}
	return defaultInterval
}

func ceilingInterval(deps Deps) time.Duration {
	if deps.Timing.Ceiling > 0 {
		return deps.Timing.Ceiling
	}
	return defaultCeiling
}

// workRemains reports whether the worktree still has a push to make.
func workRemains(deps Deps) (bool, error) {
	proceed, err := canPush(deps)
	if err != nil || !proceed {
		return false, err
	}
	return deps.Ahead()
}

// canPush reports whether the worktree stands and the start conditions but the lock hold.
func canPush(deps Deps) (bool, error) {
	gone, err := deps.WorktreeGone()
	if err != nil || gone {
		return false, err
	}
	return deps.Conditions()
}

// attemptPush makes one push attempt: the pause read, the guard, the parent step when it has work, then the push.
// A failed pause read, a covering pause and a guard stop are returned without pushing.
// A held-back or failed parent step is returned before the push, which would otherwise clear the parent's refusal.
func attemptPush(ctx context.Context, t Target, deps Deps) error {
	if deps.PauseCovers != nil {
		covered, err := deps.PauseCovers()
		if err != nil {
			return &stopError{cause: err}
		}
		if covered {
			return errPauseCovers
		}
	}
	if deps.Guard != nil {
		if err := deps.Guard(); err != nil {
			return &stopError{cause: err}
		}
	}
	if deps.Parent.PushTip != nil {
		work, err := hasParentWork(deps)
		if err != nil {
			return err
		}
		if work {
			if _, err := settleParent(ctx, t, deps); err != nil {
				return err
			}
		}
	}
	return deps.Push()
}
