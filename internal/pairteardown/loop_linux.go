// loop_linux.go kills a loop process by pid, guarded against a reused pid.

package pairteardown

import (
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// killLoopProcessPlatform kills the loop process rec records, only while the loop lock at lockPath is held and the pid still reports the recorded start time.
// A free lock means the loop is gone, and a different start time means the pid now belongs to another process.
func killLoopProcessPlatform(rec shedverbs.LoopPIDRecord, lockPath, _ string) (bool, error) {
	if rec.Loop.PID <= 0 {
		return false, nil
	}
	free, err := runLockFree(lockPath)
	if err != nil || free {
		return false, err
	}
	if start, ok := proc.StartTime(rec.Loop.PID); !ok || start != rec.Loop.StartTime {
		return false, nil
	}
	if err := proc.KillPID(rec.Loop.PID); err != nil {
		return false, err
	}
	return true, nil
}
