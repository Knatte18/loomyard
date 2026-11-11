// loopkeeper.go implements the loop's keeper: the detached process a waiter spawns.
// It starts the loop process and records in the loop log how that process ended.

package shedverbs

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// keepLoop is the keeper of loop loopID.
// It starts spec.Loop.Executable with loopArgv as the loop process, in a session of its own, and waits for it to end.
// The loop process writes its output to out, the loop log the keeper itself writes to.
// Once the loop process ends, the keeper writes a mark of the loop to out.
// The mark names the process's pid and its exit status, or the signal that ended it;
// a loop process that does not start gets a mark naming the start error instead.
// The keeper outlives the waiter that spawned it, so the mark is written even when no waiter is left to reap the keeper.
// It catches the terminating signals and keeps waiting.
// A signal that ends the run's processes therefore ends the keeper only through the loop's end.
// LoopSpec.OnStart runs once after the loop process starts.
// It returns 0 when the loop process exited with status 0, and 1 otherwise.
func keepLoop(spec *Spec, loopID string, loopArgv []string, out io.Writer) int {
	loop := spec.Loop
	signals := loop.Signals
	if signals == nil {
		signals = notifyTerminatingSignals
	}
	received, stopSignals := signals()
	defer stopSignals()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case sig := <-received:
				logger.Info("shed: the loop's keeper received a terminating signal and waits for the loop to end", "loop_id", loopID, "signal", sig.String())
			case <-done:
				return
			}
		}
	}()

	cmd := exec.Command(loop.Executable, loopArgv...)
	cmd.Stdout, cmd.Stderr = out, out
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		logger.Warn("shed: the loop's keeper could not start the loop", "loop_id", loopID, "executable", loop.Executable, "error", err.Error())
		writeKeeperMark(out, loop.LogPath, loopID, "could not start: "+err.Error())
		return 1
	}
	pid := cmd.Process.Pid
	logger.Info("shed: the loop's keeper started the loop", "loop_id", loopID, "pid", pid, "executable", loop.Executable)
	if loop.OnStart != nil {
		loop.OnStart()
	}
	waitErr := cmd.Wait()
	status := "exit status 0"
	if waitErr != nil {
		status = waitErr.Error()
	}
	logger.Info("shed: the loop process ended", "loop_id", loopID, "pid", pid, "status", status)
	writeKeeperMark(out, loop.LogPath, loopID, fmt.Sprintf("process %d exited: %s", pid, status))
	if waitErr != nil {
		return 1
	}
	return 0
}

// writeKeeperMark writes the mark text of loop loopID to out, the open loop log at logPath, on a line of its own.
// It writes through the open file rather than the path, so a log whose directory is gone is never created again.
func writeKeeperMark(out io.Writer, logPath, loopID, text string) {
	if _, err := io.WriteString(out, newlineIfUnterminated(logPath)+loopLogMark(loopID, text)); err != nil {
		logger.Warn("shed: the loop's keeper could not mark the loop log", "loop_id", loopID, "error", err.Error())
	}
}
