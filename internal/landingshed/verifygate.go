// verifygate.go declares the clean-tree and post-merge verify checks both landing producers run around their parent merge-in:
// a thin adapter over internal/verifytree, which owns the clean-tree check, the verified-tree record and the run.
// A dirty tree or a failing verify is reported as a Stuck reason, so nothing leaves the worktree on a tree that is not committed and verified.

package landingshed

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// dirtyPathsShown caps how many dirty paths a Stuck reason names.
const dirtyPathsShown = 10

// verifyGate holds the told command closure, the verify paths and the two verifytree seams.
// The dirty and verify fields are in-package seams tests replace with fakes, so unit tests never spawn git or a shell.
// A zero dirty seam skips the clean-tree check and a nil command skips the verify.
type verifyGate struct {
	command func() (string, error)
	paths   verifytree.Paths
	dirty   func(worktree string) ([]string, error)
	verify  func(ctx context.Context, p verifytree.Paths, site verifytree.Site, command string) (verifytree.Result, error)
}

// newVerifyGate copies the gate's told values from deps and wires the real verifytree functions.
func newVerifyGate(deps Deps) verifyGate {
	return verifyGate{
		command: deps.VerifyCommand,
		paths:   verifytree.NewPaths(deps.WorktreeRoot, deps.VerifyDir),
		dirty:   verifytree.DirtyPaths,
		verify: func(ctx context.Context, p verifytree.Paths, site verifytree.Site, command string) (verifytree.Result, error) {
			return verifytree.Verify(ctx, p, site, command, verifytree.Timeout)
		},
	}
}

// clean checks that the task worktree has no uncommitted changes at point, a phrase such as "before the merge-in".
//
// It returns a non-empty Stuck reason naming the dirty paths, a non-nil error when git cannot report the status,
// and ("", nil) when the tree is clean.
func (g verifyGate) clean(producer, point string) (string, error) {
	if g.dirty == nil {
		return "", nil
	}
	paths, err := g.dirty(g.paths.Worktree)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: check the worktree is clean %s: %w", producer, point, err)
	}
	if len(paths) == 0 {
		return "", nil
	}
	return dirtyReason(point, paths), nil
}

// dirtyReason is the Stuck reason for a dirty tree found at point.
func dirtyReason(point string, paths []string) string {
	shown := paths
	if len(shown) > dirtyPathsShown {
		shown = shown[:dirtyPathsShown]
	}
	list := strings.Join(shown, ", ")
	if more := len(paths) - len(shown); more > 0 {
		list = fmt.Sprintf("%s and %d more", list, more)
	}
	return fmt.Sprintf("the task worktree has uncommitted changes %s: %s; commit or remove them on the task branch, then resume", point, list)
}

// check runs the verify for producer after a merge of parentBranch.
// verifytree skips the run when its record already names the tree and the command,
// so the verify runs exactly when the tree differs from the last verified one:
// after a resolved conflict, after new parent commits and after a crash between a merge and its verify.
//
// It returns a non-empty Stuck reason when the verify fails or the tree is dirty,
// a non-nil error for an infrastructure fault or a cancellation, and ("", nil) when the producer may proceed.
func (g verifyGate) check(ctx context.Context, producer, parentBranch string) (string, error) {
	if g.command == nil {
		return "", nil
	}

	command, err := g.command()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read verify command: %w", producer, err)
	}
	if command == "" {
		logger.Warn("landingshed: post-merge verify skipped because no verify command is configured", "producer", producer, "parentBranch", parentBranch)
		return "", nil
	}

	result, err := g.verify(ctx, g.paths, verifytree.Site{Label: producer}, command)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: %w", producer, err)
	}

	switch result.Status {
	case verifytree.StatusPassed:
		logger.Info("landingshed: post-merge verify passed", "producer", producer, "parentBranch", parentBranch)
	case verifytree.StatusSkipped:
		logger.Info("landingshed: post-merge verify skipped because the tree is already verified", "producer", producer, "parentBranch", parentBranch, "tree", result.Tree)
	case verifytree.StatusDirty:
		return dirtyReason("when the verify was about to run", result.Dirty), nil
	case verifytree.StatusFailed:
		if result.ExitCode < 0 {
			return fmt.Sprintf("verify could not start after merging parent branch %q: %s; output: %s", parentBranch, result.Detail, g.paths.Log), nil
		}
		return fmt.Sprintf("verify failed after merging parent branch %q (exit code %d); output: %s; fix forward on the task branch, then resume", parentBranch, result.ExitCode, g.paths.Log), nil
	default:
		return "", fmt.Errorf("landingshed: %s: unknown verify status %q", producer, result.Status)
	}
	return "", nil
}
