// poll.go implements the long-poll loop over Digest, and the clock seam the loop runs on.
// The long-poll itself is the notification.
// The loop blocks inside Go on a fixed tick, so the caller pays nothing per tick.
// It returns as soon as a batch reaches a terminal classification.
// The clock seam lets a test replay a whole poll sequence instantly.

package websterengine

import "time"

// clock abstracts time.Now and time.Sleep.
// A fake clock's Sleep advances a virtual now instead of blocking,
// so a test can replay a whole poll sequence instantly.
type clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// realClock is the production clock: real wall-clock time, real sleeping.
type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

// pollTick is the fixed interval the poll loop waits between gather calls.
const pollTick = 1 * time.Second

// PollUntilTerminal calls gather repeatedly until it reports terminal or wait elapses.
// It returns the terminal digest at once, or the last running digest when wait elapses.
// A gather error is returned with an empty Digest.
func PollUntilTerminal(gather func() (Digest, bool, error), wait time.Duration, clk clock) (Digest, error) {
	deadline := clk.Now().Add(wait)

	for {
		digest, terminal, err := gather()
		if err != nil {
			return Digest{}, err
		}
		if terminal {
			return digest, nil
		}
		if clk.Now().After(deadline) {
			return digest, nil
		}
		clk.Sleep(pollTick)
	}
}
