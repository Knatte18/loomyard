// midmerge.go holds MidMerge, a read-only probe for an unfinished merge on a pair.
// It reports the merge state and the conflicted paths that remain.

package fabricengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// MidMerge reports whether l's pair carries an unfinished merge, and which conflicted paths remain.
//
// A probe failure is returned as an error, never as MidMergeNone.
// Each failure leaves the answer undetermined, and an undetermined answer is not evidence that the tree is clean.
//
// A fabric record wins over foreign-looking git state:
// a parked fabric merge leaves MERGE_HEAD and unmerged entries of its own.
func MidMerge(l *lymcwd.Location) (fabriclayout.MidMergeState, error) {
	f, err := Open(l)
	if err != nil {
		return fabriclayout.MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: open pair: %w", err)
	}
	record, err := f.loadMergeState()
	if err != nil {
		return fabriclayout.MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: read merge record: %w", err)
	}
	r, err := f.readForeignProbes()
	if err != nil {
		return fabriclayout.MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: read git merge state: %w", err)
	}

	state := fabriclayout.MidMergeState{Kind: fabriclayout.MidMergeNone, Conflicts: []string{}}
	switch {
	case record != nil:
		state.Kind = fabriclayout.MidMergeParked
		state.Verb = record.Verb
		state.Source = record.Source
		state.SourceSHA = record.WarpSource
		state.StartSHA = record.WarpStart
	case r.warpMergeHead || r.weftMergeHead || len(r.warpConflicted) > 0 || len(r.weftConflicted) > 0:
		state.Kind = fabriclayout.MidMergeForeign
	}

	if len(r.warpConflicted) == 0 && len(r.weftConflicted) == 0 {
		return state, nil
	}
	anchorRel, wiredNames, err := resolveMergeGeometry(l)
	if err != nil {
		return fabriclayout.MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: resolve conflict-path geometry: %w", err)
	}
	unified, unmappable := unifyConflictPaths(r.warpConflicted, r.weftConflicted, anchorRel, wiredNames)
	if unmappable {
		return fabriclayout.MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: a conflicted path is unmappable onto the one-repo form (warp %v, weft %v)", r.warpConflicted, r.weftConflicted)
	}
	state.Conflicts = unified
	return state, nil
}

// MidMerge reports the parked or foreign merge state of f's own pair.
// It resolves the location from f's code checkout,
// so the answer is about this pair and never about the worktree the caller runs in.
func (f *Fabric) MidMerge() (fabriclayout.MidMergeState, error) {
	l, err := lymcwd.ResolveWorktree(f.warpPath)
	if err != nil {
		return fabriclayout.MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: resolve layout for %s: %w", f.warpPath, err)
	}
	return MidMerge(l)
}
