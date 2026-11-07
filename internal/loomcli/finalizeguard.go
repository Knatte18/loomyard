// finalizeguard.go holds the goto guard that refuses Finalize while the parent's code checkout has tracked dirt.
// It also composes the run's goto guards into one.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// finalizeDirtGuard refuses a goto onto the Finalize row while the parent's code checkout holds tracked dirt.
// The merge into the parent at Finalize would refuse the same dirt, so the goto is refused first.
// Every other target is admitted without a parent read.
// An isFinalize error is returned as the guard's error, which refuses the move.
// A parentDirt error is logged and the move admitted, since Finalize's own steps then report the real problem.
// A merge record on the parent pair, or an empty path list, admits the move too.
// Finalize's own guard words those cases.
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

// parentCodeDirtFromFabric reads the parent pair's code checkout and its tracked uncommitted paths.
// It also reports whether fabric holds a merge record on that pair.
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

// finalizeGuard returns the goto guard that refuses Finalize while the parent's code checkout has tracked dirt.
func (c *loomCLI) finalizeGuard() shedengine.GotoGuard {
	return finalizeDirtGuard(loomrecipe.IsFinalizeRow, c.parentCodeDirt)
}
