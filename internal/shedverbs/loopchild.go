// loopchild.go is the production ChildRunner of the loop: it starts a step child tied to the loop's life.

package shedverbs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
