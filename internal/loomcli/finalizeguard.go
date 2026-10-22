// finalizeguard.go refuses a goto onto Finalize while the parent's code checkout holds tracked dirt, and composes that arm with the run's other goto guards.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// finalizeDirtGuard refuses a goto onto the Finalize row while the parent's code checkout holds tracked dirt, since Finalize's parent-side merge would refuse the same dirt.
// Every other target is admitted without a parent read.
// An isFinalize error is returned, which Shed turns into a refusal.
// A parentDirt error is logged and admitted, since Finalize's own steps then report the real problem;
// a fabric merge record on the parent pair and an empty path list are admitted too, since Finalize's own guard words those.
// The guard only decides; it moves no run and writes no file.
func finalizeDirtGuard(isFinalize func(row string) (bool, error), parentDirt func() (checkout string, paths []string, mergeRecord bool, err error)) shedengine.GotoGuard {
	return func(target string) (string, error) {
		finalize, err := isFinalize(target)
		if err != nil || !finalize {
			return "", err
		}
		checkout, paths, mergeRecord, err := parentDirt()
		if err != nil {
			logger.Warn("loom: could not read the parent's code checkout for the Finalize goto guard", "target", target, "cause", err)
			return "", nil
		}
		if mergeRecord || len(paths) == 0 {
			return "", nil
		}
		return landingshed.DirtyParentRefusal(checkout, paths), nil
	}
}

// composeGotoGuards runs guards in order and returns the first refusal or error, admitting when every guard admits.
func composeGotoGuards(guards ...shedengine.GotoGuard) shedengine.GotoGuard {
	return func(target string) (string, error) {
		for _, guard := range guards {
			if refusal, err := guard(target); refusal != "" || err != nil {
				return refusal, err
			}
		}
		return "", nil
	}
}

// parentCodeDirtFromFabric reads the parent pair's code checkout, its tracked uncommitted paths, and whether fabric holds a merge record on that pair.
// It reads the code checkout alone, never the records checkout, and returns every read error.
func (c *loomCLI) parentCodeDirtFromFabric() (checkout string, paths []string, mergeRecord bool, err error) {
	parentBranch, err := c.recordedParentBranch(c.location)
	if err != nil {
		return "", nil, false, err
	}
	parent, err := fabricengine.OpenParent(c.location, parentBranch)
	if err != nil {
		return "", nil, false, err
	}
	mergeRecord, err = parent.MergeInProgress()
	if err != nil {
		return "", nil, false, err
	}
	checkout, paths, err = parent.CodeDirtyPaths()
	if err != nil {
		return "", nil, false, err
	}
	return checkout, paths, mergeRecord, nil
}

// finalizeGuard is the Finalize arm of the run's goto guard over the parent read the receiver's seam answers.
func (c *loomCLI) finalizeGuard() shedengine.GotoGuard {
	return finalizeDirtGuard(loomrecipe.IsFinalizeRow, c.parentCodeDirt)
}
