// verifygate.go declares the post-merge verify gate both landing producers run after their parent merge-in:
// it runs the plan's verify command in the task worktree and reports a failure as a Stuck reason,
// so nothing leaves the worktree on a merged tree that does not verify.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// verifyGate holds the told values the gate needs and the runner seam it calls the command through.
// The run field is the in-package seam tests replace with a fake, so unit tests never spawn a shell.
type verifyGate struct {
	command     func() (string, error)
	pendingPath string
	outputPath  string
	dir         string
	run         func(ctx context.Context, command, dir string, out io.Writer) (int, error)
}

// newVerifyGate copies the gate's told values from deps and wires the real shared runner.
func newVerifyGate(deps Deps) verifyGate {
	return verifyGate{
		command:     deps.VerifyCommand,
		pendingPath: deps.VerifyPendingPath,
		outputPath:  deps.VerifyOutputPath,
		dir:         deps.WorktreeRoot,
		run:         verifyrun.Run,
	}
}

// check runs the gate for producer after a merge of parentBranch.
// treeChanged is whether the merge changed the task tree.
//
// It returns a non-empty Stuck reason when the verify fails,
// a non-nil error for an infrastructure fault or a cancellation, and ("", nil) when the producer may proceed.
// When the tree changed, the pending marker is written before anything else can fail,
// and a failure to write it halts the producer with an error.
// The marker is removed on a pass or when no verify command is configured,
// so a failed or interrupted verify, or a failed command read, keeps the next run's gate armed.
func (g verifyGate) check(ctx context.Context, producer, parentBranch string, treeChanged bool) (string, error) {
	if g.command == nil {
		return "", nil
	}

	markerPresent := true
	if _, err := os.Stat(g.pendingPath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("landingshed: %s: stat verify pending marker %s: %w", producer, g.pendingPath, err)
		}
		markerPresent = false
	}
	if !treeChanged && !markerPresent {
		return "", nil
	}

	if treeChanged {
		if err := os.MkdirAll(filepath.Dir(g.pendingPath), 0o755); err != nil {
			return "", fmt.Errorf("landingshed: %s: create verify pending marker directory: %w", producer, err)
		}
		if err := os.WriteFile(g.pendingPath, []byte(parentBranch), 0o644); err != nil {
			return "", fmt.Errorf("landingshed: %s: write verify pending marker %s: %w", producer, g.pendingPath, err)
		}
	}

	command, err := g.command()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read verify command: %w", producer, err)
	}
	if command == "" {
		args := []any{"producer", producer, "parentBranch", parentBranch}
		if markerPresent {
			args = append(args, "pendingMarker", g.pendingPath)
		}
		logger.Warn("landingshed: post-merge verify skipped because no verify command is configured", args...)
		if err := os.Remove(g.pendingPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("landingshed: %s: remove verify pending marker %s: %w", producer, g.pendingPath, err)
		}
		return "", nil
	}

	if err := os.MkdirAll(filepath.Dir(g.outputPath), 0o755); err != nil {
		return "", fmt.Errorf("landingshed: %s: create verify output directory: %w", producer, err)
	}
	outFile, err := os.Create(g.outputPath)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: create verify output file %s: %w", producer, g.outputPath, err)
	}
	code, runErr := g.run(ctx, command, g.dir, outFile)
	if closeErr := outFile.Close(); closeErr != nil && runErr == nil {
		logger.Warn("landingshed: close verify output file", "producer", producer, "path", g.outputPath, "cause", closeErr)
	}

	if runErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("landingshed: %s: verify cancelled: %w", producer, ctxErr)
		}
		return fmt.Sprintf("verify could not start after merging parent branch %q: %v; output: %s", parentBranch, runErr, g.outputPath), nil
	}
	if code != 0 {
		return fmt.Sprintf("verify failed after merging parent branch %q (exit code %d); output: %s; fix forward on the task branch, then resume", parentBranch, code, g.outputPath), nil
	}

	if err := os.Remove(g.pendingPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("landingshed: %s: remove verify pending marker %s: %w", producer, g.pendingPath, err)
	}
	logger.Info("landingshed: post-merge verify passed", "producer", producer, "parentBranch", parentBranch)
	return "", nil
}
