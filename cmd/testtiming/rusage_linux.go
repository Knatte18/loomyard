// rusage_linux.go reads a finished child's CPU time and largest single-process resident set from its rusage.

package main

import (
	"os"
	"syscall"
	"time"
)

// rusageStats returns the CPU time of a finished process and its waited descendants, and the largest max RSS among them in bytes.
func rusageStats(state *os.ProcessState) (cpu time.Duration, maxRSS int64) {
	cpu = state.UserTime() + state.SystemTime()
	if usage, ok := state.SysUsage().(*syscall.Rusage); ok {
		// Linux reports ru_maxrss in kibibytes.
		maxRSS = int64(usage.Maxrss) * 1024
	}
	return cpu, maxRSS
}
