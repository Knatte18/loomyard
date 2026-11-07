// cleangate.go implements the clean gate:
// it holds a producing session at its turn end until its worktree has no uncommitted or untracked path.

package loomshed

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// NewCleanGate returns a must-pass shuttleengine.Gate.
// The gate fails while git reports an uncommitted or untracked path in worktreeRoot; ignored files do not count.
// It exists so a writing session cannot end with a stray file, such as a shell redirect's output.
// Such a file would halt a later row, such as Finalize.
// The findings name every path and ask the session to commit what belongs to its change and delete the rest.
// A failure to read git is a returned error, since it is no defect the session can fix.
func NewCleanGate(worktreeRoot string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		dirty, err := verifytree.DirtyPaths(worktreeRoot)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: clean gate: %w", err)
		}
		if len(dirty) == 0 {
			return shuttleengine.GateResult{Passed: true}, nil
		}
		return shuttleengine.GateResult{Findings: verifytree.UncommittedFindings(dirty)}, nil
	}
}
