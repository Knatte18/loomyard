// loop_windows.go ends a loop through its named job object.

package pairteardown

import (
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// killLoopProcessPlatform terminates the loop's named job, which ends the loop, its step child and their descendants at once.
func killLoopProcessPlatform(_ shedverbs.LoopPIDRecord, _, jobName string) (bool, error) {
	return proc.TerminateNamedJob(jobName)
}
