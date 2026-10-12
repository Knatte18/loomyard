// sleep.go detects machine sleep from the gap between wall and awake clock readings, and measures awake time net of sleeps and pauses.

package pauserecord

import "time"

// SleepTolerance is how far the wall clock may run ahead of the awake clock between two readings before the excess counts as a sleep.
// A shorter lag is clock drift or a scheduling delay and still counts as idle time.
const SleepTolerance = time.Minute

// SleepWatch records the sleeps seen between successive wall and awake readings.
// It is not safe for concurrent use.
type SleepWatch struct {
	seen      bool
	prevWall  int64
	prevAwake time.Duration
	sleeps    []Interval
}

// Observe folds one reading into the watch.
// From the second reading on, the wall delta less the awake delta is a sleep when it exceeds SleepTolerance, recorded as the interval ending at wall.
// Sleeps ending more than Retention before wall are dropped.
// The wall delta is taken from UnixNano values so no monotonic part of wall hides a sleep.
func (s *SleepWatch) Observe(wall time.Time, awake time.Duration) {
	wallNanos := wall.UnixNano()
	if s.seen {
		gap := time.Duration(wallNanos-s.prevWall) - (awake - s.prevAwake)
		if gap > SleepTolerance {
			s.sleeps = append(s.sleeps, Interval{Start: wall.Add(-gap), End: wall})
		}
	}
	s.seen, s.prevWall, s.prevAwake = true, wallNanos, awake

	cutoff := wall.Add(-Retention)
	var kept []Interval
	for _, sleep := range s.sleeps {
		if !sleep.End.Before(cutoff) {
			kept = append(kept, sleep)
		}
	}
	s.sleeps = kept
}

// Sleeps returns a copy of the recorded sleep intervals.
func (s *SleepWatch) Sleeps() []Interval {
	return append([]Interval(nil), s.sleeps...)
}

// AwakeSince returns the time from at to now less the union of every excluded list within [at, now], never below zero.
// A sleep is an interval, so an activity read at any time keeps exactly the slept time after it.
func AwakeSince(at, now time.Time, excluded ...[]Interval) time.Duration {
	var all []Interval
	for _, intervals := range excluded {
		all = append(all, intervals...)
	}
	awake := now.Sub(at) - Excluded(all, at, now)
	if awake < 0 {
		return 0
	}
	return awake
}

// MonotonicAwake returns a reading of Go's monotonic clock from an origin taken at the call.
// That clock stops during suspend on Linux and macOS and keeps running on Windows.
func MonotonicAwake() func() time.Duration {
	origin := time.Now()
	return func() time.Duration { return time.Since(origin) }
}
