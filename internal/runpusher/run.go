// run.go is the pusher's body: it takes the worktree's pusher lock and pushes until the worktree is level with origin.

package runpusher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// repushInterval is the shortest time between two pushes of one pass.
const repushInterval = time.Second

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
	// WorktreeGone reports whether the worktree no longer exists.
	WorktreeGone func() (bool, error)
	// Owed reports whether a failed push is owed, so the pusher retries it.
	Owed func(err error) bool
	// Notify posts one line about a push that stays owed.
	Notify func(line string) error
	// Sleep waits between the retries of an owed push and between two pushes of one pass.
	Sleep func(time.Duration)
}

// NewDeps fills every seam of t that reads the worktree, the hub or the clock.
// A pair gets the pause read, and the prime none.
// The caller fills Push, Guard and Notify.
func NewDeps(t Target) Deps {
	deps := Deps{
		Lock:  func() (func(), bool, error) { return tryLock(LockPath(t)) },
		Ahead: func() (bool, error) { return fabricengine.RunCommitsAhead(t.Location) },
		Conditions: func() (bool, error) {
			stands, err := fabricengine.RefusedPushStands(t.Location)
			if err != nil || stands {
				return false, err
			}
			if !t.Pair {
				return true, nil
			}
			marked, err := handedOver(t)
			return !marked, err
		},
		WorktreeGone: func() (bool, error) { return worktreeGone(t) },
		Owed:         fabricengine.IsPushOwed,
		Sleep:        time.Sleep,
	}
	if t.Pair {
		deps.PauseCovers = func() (bool, error) { return pauseCovers(t) }
	}
	return deps
}

// Run is the pusher of t: it pushes the worktree's commits to origin in order, and returns once it is level or must stop.
// It takes the pusher lock and returns at once when another pusher holds it.
// Each push attempt reads the pause first, then runs the guard, then pushes.
// A pause covering the run ends the pusher before the guard and the push.
// A guard stop returns its error, pushing nothing.
// A failure deps.Owed does not read as owed returns, a refused push and a hand-over among them.
// An owed push is retried through PushUntilLanded, and one notice goes out once the retry wait reaches a minute.
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
	for first := true; ; first = false {
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
			return false, nil
		}
		if !first {
			deps.Sleep(repushInterval)
		}
		err = PushUntilLanded(ctx, attemptOf(deps), ownedOutsideStops(deps), deps.Sleep, noticeOf(t, deps))
		var stop *stopError
		switch {
		case errors.Is(err, errPauseCovers):
			return true, nil
		case errors.As(err, &stop):
			return true, stop.cause
		case err != nil:
			return true, err
		}
	}
}

// workRemains reports whether the worktree still has a push to make.
func workRemains(deps Deps) (bool, error) {
	proceed, err := canPush(deps)
	if err != nil || !proceed {
		return false, err
	}
	return deps.Ahead()
}

// canPush reports whether the worktree exists and the start conditions but the lock hold.
func canPush(deps Deps) (bool, error) {
	gone, err := deps.WorktreeGone()
	if err != nil || gone {
		return false, err
	}
	return deps.Conditions()
}

// attemptOf returns one push attempt: the pause read, then the guard, then the push.
func attemptOf(deps Deps) func() error {
	return func() error {
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
		return deps.Push()
	}
}

// ownedOutsideStops returns the owed test of a push attempt: a stop is never owed.
func ownedOutsideStops(deps Deps) func(error) bool {
	return func(err error) bool {
		var stop *stopError
		if errors.Is(err, errPauseCovers) || errors.As(err, &stop) {
			return false
		}
		return deps.Owed(err)
	}
}

// noticeOf returns the call that posts one notice naming the worktree and the error its pushes keep failing with.
func noticeOf(t Target, deps Deps) func(error) {
	return func(pushErr error) {
		line := fmt.Sprintf("the pusher of %s keeps failing to push, and retries every minute: %v", t.Location.WorktreePath(), pushErr)
		if err := deps.Notify(line); err != nil {
			logger.Warn("runpusher: the owed-push notice was not posted", "worktree", t.Location.WorktreePath(), "error", err)
		}
	}
}

// worktreeGone reports whether the worktree of t no longer exists.
func worktreeGone(t Target) (bool, error) {
	_, err := os.Stat(t.Location.WorktreePath())
	if os.IsNotExist(err) {
		return true, nil
	}
	return false, err
}
