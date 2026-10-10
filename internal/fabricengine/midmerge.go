// midmerge.go holds MidMerge, the read-only, vocabulary-neutral probe `lyx loom start` consults to learn whether a pair carries an unfinished merge before putting a driver to work on it.

package fabricengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// MidMergeKind classifies the merge state MidMerge finds on a pair.
type MidMergeKind int

const (
	// MidMergeNone means no merge state exists anywhere on the pair.
	MidMergeNone MidMergeKind = iota
	// MidMergeParked means the fabric merge-state record exists: a fabric merge verb stopped part-way and left it for a resolver.
	MidMergeParked
	// MidMergeForeign means no fabric record exists, but a live MERGE_HEAD or unmerged index entries sit on either side of the pair: git-level merge state fabric did not start.
	MidMergeForeign
)

// MidMergeState is MidMerge's answer.
// Conflicts lists the still-conflicted paths in the pair's one-repo form, the same form a fabric merge verb's conflicts use, so every listed path is accepted by `lyx fabric merge-stage`;
// it is empty-never-nil.
// Verb, Source, SourceSHA and StartSHA are filled only when Kind is MidMergeParked, and are empty otherwise:
// the parked record's verb (`merge-in` or `merge`), its caller-supplied source branch,
// and the code side's resolved source SHA and pre-merge HEAD SHA.
// It carries no MutationRecord: a read-only probe must not (Mutation Record Invariant).
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
// Neither existing probe serves this question: MergeStateActive is weft-only and Fabric.MergeInProgress is record-only,
// and each misses the incident shape — a conflicted warp-side merge-in, or foreign warp-side state.
//
// Every probe failure is returned as an error, never as MidMergeNone: a pair Open cannot open, a record location that cannot be resolved, a conflict-path geometry error and an unmappable conflicted path are all undetermined answers,
// and an undetermined answer is not evidence the tree is clean.
// This is deliberately unlike mergeBlocksMutation, which swallows an unopenable pair to false.
//
// A fabric record wins over foreign-looking git state, since a parked fabric merge leaves MERGE_HEAD and unmerged entries of its own.
func MidMerge(l *lyxcwd.Location) (MidMergeState, error) {
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
