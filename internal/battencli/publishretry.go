// publishretry.go wires the seams of Run-Shed's automatic Publish verify retry: the child's Publish failure record, its HEAD and its goto verb.

package battencli

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// readChildPublishFailure reads the verify failure record of the task worktree and maps the fields the retry reads.
// found is false when the task worktree holds no record.
func readChildPublishFailure(taskLocation *lyxcwd.Location) (battenshed.ChildPublishFailure, bool, error) {
	paths := verifytree.NewPaths(taskLocation.WorktreePath(), verifytree.Dir(taskLocation.AnchorPath()))
	record, found, err := verifytree.ReadPublishFailure(paths)
	if err != nil || !found {
		return battenshed.ChildPublishFailure{}, false, err
	}
	return battenshed.ChildPublishFailure{Row: record.Row, Head: record.Head, Target: record.Target, WrittenAt: record.WrittenAt}, true, nil
}

// childHead returns the HEAD commit of the task worktree.
func childHead(taskLocation *lyxcwd.Location) (string, error) {
	fab, err := fabricengine.Open(taskLocation)
	if err != nil {
		return "", err
	}
	return fab.HeadSHA()
}

// childGoto runs `lyx loom goto --to <target>` in the task worktree and folds the child's output into a failure through childSpawnError.
func childGoto(ctx context.Context, taskLocation *lyxcwd.Location, slug, target string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "loom", "goto", "--to", target)
	cmd.Dir = taskLocation.AnchorPath()
	var childOutput bytes.Buffer
	cmd.Stdout = &childOutput
	cmd.Stderr = &childOutput
	logger.Info("battencli: sending the loom run to its Publish failure target", "slug", slug, "dir", cmd.Dir, "target", target)
	runErr := cmd.Run()
	logger.Info("battencli: loom goto wait complete", "slug", slug, "dir", cmd.Dir, "target", target)
	return childSpawnError(runErr, childOutput.String())
}
