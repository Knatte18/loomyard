// backoff.go holds the waits the relay's outbound loop takes between reads:
// a floor, doubled while a read finds nothing, and raised by a rate limit that the outbound loop or the wake loop meets.

package slackengine

import (
	"sync"
	"time"
)

const (
	// waitFloor is the wait after a pass that did something.
	waitFloor = 5 * time.Second
	// waitCeiling is the longest wait a loop takes between passes.
	waitCeiling = 60 * time.Second
)

// sharedFloor is the shortest wait of the outbound loop and the first read's retries.
// A rate limit met by the outbound loop or the wake loop doubles it for the rest of the run.
type sharedFloor struct {
	mu    sync.Mutex
	value time.Duration
}

func newSharedFloor() *sharedFloor {
	return &sharedFloor{value: waitFloor}
}

func (f *sharedFloor) get() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value
}

// rateLimited doubles the floor, up to waitCeiling.
func (f *sharedFloor) rateLimited() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.value = min(f.value*2, waitCeiling)
}

// loopWait is one loop's wait between passes.
type loopWait struct {
	floor   *sharedFloor
	current time.Duration
}

func newLoopWait(floor *sharedFloor) *loopWait {
	return &loopWait{floor: floor, current: floor.get()}
}

// afterActivity returns the wait after a pass that did something: the shared floor.
func (w *loopWait) afterActivity() time.Duration {
	w.current = w.floor.get()
	return w.current
}

// afterIdle returns the wait after a pass that found nothing: the last wait doubled, never past waitCeiling.
func (w *loopWait) afterIdle() time.Duration {
	w.current = min(max(w.current*2, w.floor.get()), waitCeiling)
	return w.current
}

// afterRateLimit raises the shared floor and returns the wait limit asks for.
func (w *loopWait) afterRateLimit(limit *RateLimitError) time.Duration {
	w.floor.rateLimited()
	w.current = w.floor.get()
	return limit.RetryAfter
}
