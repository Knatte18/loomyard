// wiring_request_test.go checks the two bases the capture and compare requests of a drift verb carry.

package driftcli

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWorktreeRequests_JoinOnTheWorktreeRootAndStartInTheAnchor covers the bases of a capture and a compare request for a worktree anchored at a subdirectory, where the worktree root and the anchored directory differ.
func TestWorktreeRequests_JoinOnTheWorktreeRootAndStartInTheAnchor(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	location := &lymcwd.Location{HubPath: filepath.Join(t.TempDir(), "hub"), WorktreeName: "pair", AnchorRel: "backend"}
	root, anchor := filepath.Join(location.HubPath, "pair"), filepath.Join(location.HubPath, "pair", "backend")
	w := &worktree{location: location, paths: driftengine.NewPaths(location.AnchorPath())}
	step := driftengine.CommandStep{Command: "p {baseline_dir}", Dir: "tests", TimeoutMin: 3}

	capture := w.captureRequest(step)
	if capture.WorktreeDir != root || capture.AnchorDir != anchor || capture.Timeout != 3*time.Minute || capture.Step.Dir != "tests" {
		t.Errorf("captureRequest() = %+v; want WorktreeDir %q, AnchorDir %q, a 3m timeout and the step", capture, root, anchor)
	}
	compare := w.compareRequest(step)
	if compare.WorktreeDir != root || compare.AnchorDir != anchor || compare.Timeout != 3*time.Minute || compare.Step.Dir != "tests" {
		t.Errorf("compareRequest() = %+v; want WorktreeDir %q, AnchorDir %q, a 3m timeout and the step", compare, root, anchor)
	}
}
