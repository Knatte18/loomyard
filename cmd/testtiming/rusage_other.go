//go:build !linux

// rusage_other.go reads a finished child's CPU time where the largest resident set is not available; it reports zero for memory.

package main

import (
	"os"
	"time"
)

// rusageStats returns the CPU time of a finished process and its waited descendants; the max RSS is not read on this platform.
func rusageStats(state *os.ProcessState) (cpu time.Duration, maxRSS int64) {
	return state.UserTime() + state.SystemTime(), 0
}
