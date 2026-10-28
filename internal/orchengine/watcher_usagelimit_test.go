//go:build tier0

// watcher_usagelimit_test.go covers the watcher under a usage limit at the orch's own turn end: the hold on its deliveries, no cycle, the wake line and its retries, and the release once the orch answers.

package orchengine

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

func TestWatcher_UsageLimitHoldsTheOrchAndWakesItAfterTheReset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// resetIn is how long after the limit is read it resets; zero for a message stating no reset time.
		resetIn time.Duration
		// firstWake is how long after the limit is read the first wake line goes.
		firstWake time.Duration
		wantLine  string
	}{
		{"AReadableResetTime", time.Hour, time.Hour + usageLimitWakeDelay, "reset at "},
		{"NoReadableResetTime", 0, usageLimitRetry, "could not be read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.tick()
			readAt := e.clock.now
			var resetsAt time.Time
			if tt.resetIn > 0 {
				resetsAt = readAt.Add(tt.resetIn)
			}
			// The limited turn end is over the hard threshold, so only the limit keeps a cycle from starting.
			e.s.usage["limited"] = 2000
			e.s.limits = map[string]shuttleengine.UsageLimit{"limited": {Limited: true, ResetsAt: resetsAt}}
			e.s.events = append(e.s.events, stop("limited"))
			wakes := func() int { return e.s.count("send:") }

			e.tick()
			st := e.state()
			if !st.UsageLimitAt.Equal(readAt) || !st.UsageLimitResetsAt.Equal(resetsAt) || st.Phase != PhaseIdle || wakes() != 0 || !slices.Contains(e.c.calls, "hold:"+fakeOrchStrandName) {
				t.Fatalf("state = %+v, sends = %v, creel calls = %v; want the limit recorded, the orch held, idle and nothing sent", st, e.s.calls, e.c.calls)
			}

			e.clock.now = readAt.Add(tt.firstWake - time.Second)
			e.tick()
			if wakes() != 0 {
				t.Fatalf("sends = %v; want none before the wake time", e.s.calls)
			}
			e.clock.now = readAt.Add(tt.firstWake)
			e.tick()
			if wakes() != 1 || !strings.Contains(e.s.calls[0], tt.wantLine) || !strings.Contains(e.s.calls[0], "lym tender revive") {
				t.Fatalf("sends = %v; want one wake line containing %q", e.s.calls, tt.wantLine)
			}

			// With no answer the line goes again one retry interval later.
			e.clock.now = e.clock.now.Add(usageLimitRetry - time.Second)
			e.tick()
			e.clock.now = e.clock.now.Add(time.Second)
			e.tick()
			if wakes() != 2 {
				t.Fatalf("sends = %v; want a second wake line after %s with no answer", e.s.calls, usageLimitRetry)
			}

			e.s.events = append(e.s.events, stop("answer"))
			e.tick()
			st = e.state()
			if !st.UsageLimitAt.IsZero() || !st.UsageLimitWakeAt.IsZero() || !slices.Contains(e.c.calls, "release:"+fakeOrchStrandName) {
				t.Fatalf("state = %+v, creel calls = %v; want the limit cleared and the hold released once the orch answered", st, e.c.calls)
			}
		})
	}
}

// limitedWatchEnv returns an env whose orch has just stopped on a usage limit stating resetsAt, with the limit read at the returned time.
func limitedWatchEnv(t *testing.T, resetsAt func(readAt time.Time) time.Time) (*watchEnv, time.Time) {
	t.Helper()
	e := newWatchEnv(t)
	e.tick()
	readAt := e.clock.now
	e.s.usage["limited"] = 2000
	e.s.limits = map[string]shuttleengine.UsageLimit{"limited": {Limited: true, ResetsAt: resetsAt(readAt)}}
	e.s.events = append(e.s.events, stop("limited"))
	e.tick()
	return e, readAt
}

func TestWatcher_UsageLimitReadAfterItsResetWakesTheOrchAtOnce(t *testing.T) {
	t.Parallel()
	e, readAt := limitedWatchEnv(t, func(readAt time.Time) time.Time { return readAt.Add(-time.Hour) })
	e.tick()
	if got := e.s.count("send:"); got != 1 {
		t.Fatalf("sends = %v; want one wake line without waiting out a retry interval", e.s.calls)
	}
	if want := readAt.Add(usageLimitRetry); !e.state().UsageLimitWakeAt.Equal(want) {
		t.Errorf("UsageLimitWakeAt = %v; want %v, one retry interval after the wake", e.state().UsageLimitWakeAt, want)
	}
}

func TestWatcher_UsageLimitWakeFailuresRetryTheWake(t *testing.T) {
	t.Parallel()
	noReset := func(time.Time) time.Time { return time.Time{} }
	t.Run("WakeLineThatDoesNotRender", func(t *testing.T) {
		t.Parallel()
		e, readAt := limitedWatchEnv(t, noReset)
		stencilkit.Remove(t, e.stDir, usageLimitWakeStencil)
		e.clock.now = readAt.Add(usageLimitRetry)
		e.tick()
		if e.s.count("send:") != 0 {
			t.Fatalf("sends = %v; want nothing sent for a wake line that does not render", e.s.calls)
		}
		if want := e.clock.now.Add(usageLimitRetry); !e.state().UsageLimitWakeAt.Equal(want) {
			t.Errorf("UsageLimitWakeAt = %v; want %v, one retry interval on", e.state().UsageLimitWakeAt, want)
		}
	})
	t.Run("CreelInjectThatFails", func(t *testing.T) {
		t.Parallel()
		e, readAt := limitedWatchEnv(t, noReset)
		e.clock.now = readAt.Add(usageLimitRetry)
		e.c.failNext("inject", errors.New("creel is down"))
		e.tick()
		if e.s.count("send:") != 0 {
			t.Fatalf("sends = %v; want none from a failed inject", e.s.calls)
		}
		if got := e.state().UsageLimitWakeAt; !got.Equal(readAt.Add(usageLimitRetry)) {
			t.Fatalf("UsageLimitWakeAt = %v; want it left due at %v", got, readAt.Add(usageLimitRetry))
		}
		e.tick()
		if got := e.s.count("send:"); got != 1 {
			t.Errorf("sends = %v; want the next tick to send the wake line", e.s.calls)
		}
	})
}
