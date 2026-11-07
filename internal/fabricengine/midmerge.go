// midmerge.go holds MidMerge, a read-only probe for an unfinished merge on a pair.
// It reports the merge state and the conflicted paths that remain.

package fabricengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// MidMergeKind classifies the merge state MidMerge finds on a pair.
type MidMergeKind int

const (
	// MidMergeNone means no merge state exists anywhere on the pair.
	MidMergeNone MidMergeKind = iota
	// MidMergeParked means the fabric merge-state record exists: a merge verb stopped part-way and left it for a resolver.
	MidMergeParked
	// MidMergeForeign means no fabric record exists, but foreign git-level merge state sits on either side of the pair.
	MidMergeForeign
)

// The verbs a fabric merge-state record carries, as MidMergeState.Verb reports them.
const (
	// MergeVerbMergeIn is the verb of a record written by a merge-in of a source branch into the current pair.
	MergeVerbMergeIn = "merge-in"
	// MergeVerbMerge is the verb of a record written by a merge of a source branch into a target pair.
	MergeVerbMerge = "merge"
)

// MidMergeState is MidMerge's answer.
// Conflicts lists the still-conflicted paths in the pair's one-repo form, which is the form fabric merge verbs use,
// so every listed path is accepted by `lym fabric merge-stage`.
// It is an empty slice, never nil.
// Verb, Source, SourceSHA and StartSHA are filled only when Kind is MidMergeParked, and are empty otherwise:
// the parked record's verb (`merge-in` or `merge`), its caller-supplied source branch,
// and the code side's resolved source SHA and pre-merge HEAD SHA.
// The probe is read-only and carries no mutation record.
type MidMergeState struct {
	Kind      MidMergeKind
	Conflicts []string
	Verb      string
	Source    string
	SourceSHA string
	StartSHA  string
}

// MidMerge reports whether l's pair carries an unfinished merge, and which conflicted paths remain.
//
// A probe failure is returned as an error, never as MidMergeNone.
// Each failure leaves the answer undetermined, and an undetermined answer is not evidence that the tree is clean.
//
// A fabric record wins over foreign-looking git state:
// a parked fabric merge leaves MERGE_HEAD and unmerged entries of its own.
func MidMerge(l *lymcwd.Location) (MidMergeState, error) {
	f, err := Open(l)
	if err != nil {
		return MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: open pair: %w", err)
	}
	record, err := f.loadMergeState()
	if err != nil {
		return MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: read merge record: %w", err)
	}
	r, err := f.readForeignProbes()
	if err != nil {
		return MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: read git merge state: %w", err)
	}

	state := MidMergeState{Kind: MidMergeNone, Conflicts: []string{}}
	switch {
	case record != nil:
		state.Kind = MidMergeParked
		state.Verb = record.Verb
		state.Source = record.Source
		state.SourceSHA = record.WarpSource
		state.StartSHA = record.WarpStart
	case r.warpMergeHead || r.weftMergeHead || len(r.warpConflicted) > 0 || len(r.weftConflicted) > 0:
		state.Kind = MidMergeForeign
	}

	if len(r.warpConflicted) == 0 && len(r.weftConflicted) == 0 {
		return state, nil
	}
	anchorRel, wiredNames, err := resolveMergeGeometry(l)
	if err != nil {
		return MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: resolve conflict-path geometry: %w", err)
	}
	unified, unmappable := unifyConflictPaths(r.warpConflicted, r.weftConflicted, anchorRel, wiredNames)
	if unmappable {
		return MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: a conflicted path is unmappable onto the one-repo form (warp %v, weft %v)", r.warpConflicted, r.weftConflicted)
	}
	state.Conflicts = unified
	return state, nil
}

// MidMerge reports the parked or foreign merge state of f's own pair.
// It resolves the location from f's code checkout,
// so the answer is about this pair and never about the worktree the caller runs in.
func (f *Fabric) MidMerge() (MidMergeState, error) {
	l, err := lymcwd.ResolveWorktree(f.warpPath)
	if err != nil {
		return MidMergeState{}, fmt.Errorf("fabricengine: mid-merge probe: resolve layout for %s: %w", f.warpPath, err)
	}
	return MidMerge(l)
}
