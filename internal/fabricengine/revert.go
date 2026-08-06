// revert.go resolves a warp SHA to its corresponding weft SHA via the
// correspondence index: classifyCorrespondence is the pure gap-classification
// core, extracted so it is testable without spawning git; resolveRevertTarget
// wraps it with the git-backed staleness retry. Fabric.Diff (diff.go) is the
// surviving caller of this resolution path.

package fabricengine

import (
	"fmt"
)

// revertResolution is classifyCorrespondence's pure result: whether
// targetSHA had an exact entry in the index, and the entry resolution
// settled on — the exact match itself, or the nearest at-or-before one.
type revertResolution struct {
	Exact bool
	Entry corrEntry
}

// classifyCorrespondence resolves targetSHA (whose ordering position is
// targetSeq) against ix: an exact match always wins; otherwise the nearest
// at-or-before entry is used, with Exact reported false so the caller can
// flag the resulting gap. Returns wrapped ErrNoCorrespondence when ix has no
// entry at or before targetSeq at all — there is nothing to revert weft to.
// This function is pure (no git, no I/O), so it is unit-testable against a
// hand-built index without spawning git, per the discussion's TDD-candidate
// decision for gap classification.
func classifyCorrespondence(ix *corrIndex, targetSeq int, targetSHA string) (revertResolution, error) {
	if entry, ok := ix.exact(targetSHA); ok {
		return revertResolution{Exact: true, Entry: entry}, nil
	}
	if entry, ok := ix.nearestAtOrBefore(targetSeq); ok {
		return revertResolution{Exact: false, Entry: entry}, nil
	}
	return revertResolution{}, fmt.Errorf("%w: warp SHA %s", ErrNoCorrespondence, targetSHA)
}

// resolveRevertTarget resolves warpSHA to the weft SHA Fabric.Diff's
// weftAnchorForWarpSHA should bridge to, mutating nothing: it loads the
// correspondence index, classifies the target via classifyCorrespondence,
// and validates the resolved weft SHA with f.Weft.SHAExists — retrying once
// via RebuildIndex on a stale hit, exactly like WeftSHAForWarpSHA's
// self-correction. Returns wrapped ErrStaleSHA when the resolved weft SHA
// still fails to exist after the rebuild retry.
func (f *Fabric) resolveRevertTarget(warpSHA string, targetSeq int) (revertResolution, error) {
	path, err := f.corrIndexPath()
	if err != nil {
		return revertResolution{}, err
	}
	ix, err := loadCorrIndex(path)
	if err != nil {
		return revertResolution{}, err
	}

	res, err := classifyCorrespondence(ix, targetSeq, warpSHA)
	if err != nil {
		return revertResolution{}, err
	}
	if f.Weft.SHAExists(res.Entry.WeftSHA) {
		return res, nil
	}

	staleWeftSHA := res.Entry.WeftSHA
	if err := f.RebuildIndex(); err != nil {
		return revertResolution{}, err
	}
	ix, err = loadCorrIndex(path)
	if err != nil {
		return revertResolution{}, err
	}
	if res, err = classifyCorrespondence(ix, targetSeq, warpSHA); err == nil && f.Weft.SHAExists(res.Entry.WeftSHA) {
		return res, nil
	}

	return revertResolution{}, fmt.Errorf("%w: warp SHA %s, weft SHA %s", ErrStaleSHA, warpSHA, staleWeftSHA)
}
