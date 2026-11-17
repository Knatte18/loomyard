// bridgepoll.go is the bridge poll: the loop that runs the relay's bridge round, backing off while nothing is new.

package slackengine

import (
	"context"
	"sync/atomic"
	"time"
)

const (
	// bridgePollBase is the wait after a round that posted or delivered something new, and the first wait of the loop.
	bridgePollBase = 5 * time.Second
	// bridgePollCeiling is the longest wait between two rounds.
	bridgePollCeiling = 60 * time.Second
)

// bridgeRound is one round of the relay's bridge work.
// It sends the machine mail creel holds.
// Then it looks for bridge channels the hub lacks when a look is due, and reads the channels.
// One goroutine runs it, the bridge poll's.
type bridgeRound struct {
	sender *bridgeSender
	reader *bridgeReader
	clock  Clock
	log    *relayLog
	// resyncWanted is set by the wake loop when the socket holder saw an event in a channel no map holds.
	resyncWanted *atomic.Bool
	// lastResync is when the previous look for bridge channels began, the zero time before the first.
	lastResync time.Time
}

// run makes one round and reports whether it posted or delivered something new.
// The look for bridge channels is due at the first round and when the wake loop marked one wanted.
// It is also due once bridgePollCeiling has passed since the previous look.
func (b *bridgeRound) run(ctx context.Context) bool {
	posted, err := b.sender.send(ctx)
	if err != nil {
		b.log.fail("bridge-send", err)
	} else {
		b.log.recovered("bridge-send")
	}
	now := b.clock.Now()
	if b.resyncWanted.Swap(false) || now.Sub(b.lastResync) >= bridgePollCeiling {
		b.lastResync = now
		if err := b.reader.resync(ctx); err != nil {
			b.log.fail("bridge-resync", err)
		} else {
			b.log.recovered("bridge-resync")
		}
	}
	delivered, err := b.reader.read(ctx)
	if err != nil {
		b.log.fail("bridge-read", err)
	} else {
		b.log.recovered("bridge-read")
	}
	return posted || delivered
}

// bridgePoll runs a round of the relay's bridge work in a loop.
type bridgePoll struct {
	clock Clock
	// woken holds one pending wake, which ends the loop's next wait at once.
	woken chan struct{}
}

// newBridgePoll returns a bridge poll that waits on clock.
func newBridgePoll(clock Clock) *bridgePoll {
	return &bridgePoll{clock: clock, woken: make(chan struct{}, 1)}
}

// wake ends the current wait at once, and the next one when no wait is current.
// The loop never starts a round within wakeFloor of the previous round's start.
func (p *bridgePoll) wake() {
	select {
	case p.woken <- struct{}{}:
	default:
	}
}

// run calls round until ctx ends, waiting between rounds.
// round reports whether it posted or delivered something new; a held message it read again is not new.
// The wait is bridgePollBase after such a round, and otherwise the previous wait doubled up to bridgePollCeiling.
func (p *bridgePoll) run(ctx context.Context, round func(ctx context.Context) bool) {
	delay := bridgePollBase
	for ctx.Err() == nil {
		started := p.clock.Now()
		progressed := round(ctx)
		if progressed {
			delay = bridgePollBase
		}
		if !p.await(ctx, delay, started) {
			return
		}
		if !progressed {
			delay = min(delay*2, bridgePollCeiling)
		}
	}
}

// await waits delay, or until a wake ends the wait once wakeFloor has passed since started.
// It reports false when ctx ended first.
func (p *bridgePoll) await(ctx context.Context, delay time.Duration, started time.Time) bool {
	select {
	case <-p.clock.After(delay):
		return true
	case <-p.woken:
		if rest := wakeFloor - p.clock.Now().Sub(started); rest > 0 {
			return waitFor(ctx, p.clock, rest)
		}
		return ctx.Err() == nil
	case <-ctx.Done():
		return false
	}
}
