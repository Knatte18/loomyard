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
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
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

// childGoVerifyRetrySpent reads whether the task worktree's per-run settle records show its Go driver already spent a verify retry.
func childGoVerifyRetrySpent(taskLocation *lyxcwd.Location) (bool, error) {
	records, err := sheddriver.ReadSettleRecords(shedrun.SettleRecords(taskLocation, shedrun.SelfRunID))
	if err != nil {
		return false, err
	}
	return sheddriver.GoVerifyRetrySpent(records), nil
}

// childHead returns the HEAD commit of the task worktree.
func childHead(taskLocation *lyxcwd.Location) (string, error) {
	fab, err := fabricengine.Open(taskLocation)
	if err != nil {
		return "", err
	}
	return fab.HeadSHA()
}

// childGotoArgv returns the arguments childGoto spawns the lyx executable with: `loom goto --to <target> --by shed`, so the automatic retry labels its goto as lyx's own.
func childGotoArgv(target string) []string {
	return []string{"loom", "goto", "--to", target, "--by", shedengine.CallerShed}
}

// childGoto runs `lyx loom goto --to <target> --by shed` in the task worktree and folds the child's output into a failure through childSpawnError.
func childGoto(ctx context.Context, taskLocation *lyxcwd.Location, slug, target string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, childGotoArgv(target)...)
	cmd.Dir = taskLocation.AnchorPath()
	var childOutput bytes.Buffer
	cmd.Stdout = &childOutput
	cmd.Stderr = &childOutput
	logger.Debug("battencli: sending the loom run to its Publish failure target", "slug", slug, "dir", cmd.Dir, "target", target)
	runErr := cmd.Run()
	logger.Debug("battencli: loom goto wait complete", "slug", slug, "dir", cmd.Dir, "target", target)
	return childSpawnError(runErr, childOutput.String())
}
