// loopchild.go starts the loop's step children: the retry of a start refused for a busy executable, and the production ChildRunner, which ties a child to the loop's life.

package shedverbs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// procChildRunner starts the request's command with proc.StartTied, so the child dies with the loop and never with the driver's shell.
// The child's standard error is captured into req.StderrPath when it is set.
func procChildRunner(ctx context.Context, req ChildRequest) (Child, error) {
	cmd := exec.CommandContext(ctx, req.Argv[0], req.Argv[1:]...)
	cmd.Env = req.Env
	cmd.Dir = req.Dir
	cmd.Stdout = req.Stdout
	if req.StderrPath != "" {
		if err := os.MkdirAll(filepath.Dir(req.StderrPath), 0o755); err != nil {
			return nil, fmt.Errorf("shedverbs: create the child's stderr directory: %w", err)
		}
		stderr, err := os.Create(req.StderrPath)
		if err != nil {
			return nil, fmt.Errorf("shedverbs: create the child's stderr file: %w", err)
		}
		defer stderr.Close()
		cmd.Stderr = stderr
	}
	tied, err := proc.StartTied(cmd)
	if err != nil {
		return nil, err
	}
	logger.Info("shed: loop child process started", "pid", tied.Record().PID, "executable", req.Argv[0])
	return tied, nil
}

// childStartBusyAttempts is how many times a child start refused because its executable was open for writing is tried in all.
const childStartBusyAttempts = 5

// childStartBusyFirstDelay is the wait before the first retry of such a start; each later wait doubles it.
const childStartBusyFirstDelay = 100 * time.Millisecond

// startChild starts req with runner, and starts it again while the start is refused because the executable is open for writing, as while a deploy rewrites the binary in place, up to childStartBusyAttempts tries in all.
// Such a refusal comes before the child runs, so no step runs twice.
// The waits between tries go through loop.Sleep, a real wait when it is nil, and a wait that ctx ends returns the last refusal.
// It returns the tries made, one for a start that needed no retry.
func startChild(ctx context.Context, loop LoopSpec, runner ChildRunner, req ChildRequest, traceID string) (Child, int, error) {
	sleep := loop.Sleep
	if sleep == nil {
		sleep = sleepFor
	}
	delay := childStartBusyFirstDelay
	for attempt := 1; ; attempt++ {
		child, err := runner(ctx, req)
		if err == nil || !proc.IsSpawnBusy(err) || attempt == childStartBusyAttempts {
			return child, attempt, err
		}
		logger.Warn("shed: loop child start found its executable busy; retrying", "trace_id", traceID, "attempt", attempt, "delay", delay.String(), "error", err.Error())
		if sleep(ctx, delay) != nil {
			return nil, attempt, err
		}
		delay *= 2
	}
}
