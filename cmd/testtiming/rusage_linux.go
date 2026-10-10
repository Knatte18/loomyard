// rusage_linux.go reads the largest single-process resident set of a finished child from its rusage.

package main

import (
	"os"
	"syscall"
)

// maxRSSBytes returns the largest max RSS among a finished process and its waited descendants, in bytes.
func maxRSSBytes(state *os.ProcessState) int64 {
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	// Linux reports ru_maxrss in kibibytes.
	return int64(usage.Maxrss) * 1024
}
