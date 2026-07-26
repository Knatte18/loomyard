// poll.go implements webster's own long-poll loop: a webster-local copy of
// builderengine.PollUntilTerminal (builderengine/poll.go) retargeted to
// webster's Digest, plus the unexported clock seam (clock, realClock,
// pollTick) that copy relies on so a test can inject a fake clock and
// replay a whole poll sequence instantly. The long-poll IS the
// notification (mirroring builder's own `poll` semantics decision): the
// loop blocks inside Go on a fixed tick, costing the caller nothing per
// tick, and returns the instant a batch reaches a terminal classification.

package websterengine

import "time"

// clock abstracts time.Now/time.Sleep so PollUntilTerminal's wait loop runs
// instantly under test: a fake clock's Sleep advances a virtual "now"
// rather than blocking, letting a test replay a whole poll sequence
// instantly.
type clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// realClock is the production clock: real wall-clock time, real sleeping.
type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

// pollTick is PollUntilTerminal's fixed re-run interval. Blocking inside Go
// on a short tick keeps the loop responsive without hammering gather's own
// I/O.
const pollTick = 1 * time.Second

// PollUntilTerminal repeatedly calls gather on pollTick's cadence until it
// reports terminal or wait elapses, timing itself via clk (realClock{} in
// production; a fake clock replays an entire poll sequence instantly under
// test). A terminal gather result returns immediately. If wait elapses
// first, PollUntilTerminal returns gather's last non-terminal ("running")
// digest with a nil error — the snapshot the caller's next poll call
// re-polls from; a deadline is an ordinary long-poll return, never a
// failure. A gather error propagates immediately: a tick that cannot even
// determine whether the batch is terminal yet has nothing safe to report.
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
