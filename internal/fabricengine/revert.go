// revert.go resolves a warp SHA to the weft SHA it corresponds to, through the correspondence index.
// It holds the pure gap classification and the git-backed resolution with its staleness retry.

package fabricengine

import (
	"fmt"
)

// revertResolution is the result of classifying a warp SHA against the correspondence index.
// It reports whether the target SHA had an exact entry, and the entry the resolution settled on:
// the exact match itself, or the nearest at-or-before one.
type revertResolution struct {
	Exact bool
	Entry corrEntry
}

// classifyCorrespondence resolves targetSHA, whose ordering position is targetSeq, against ix.
// An exact match always wins.
// Otherwise the nearest at-or-before entry is used, and Exact is reported false so the caller can flag the gap.
// It returns a wrapped ErrNoCorrespondence when ix has no entry at or before targetSeq.
// Weft has nothing to revert to in that case.
// It takes no git and no I/O, so a test can run it against a hand-built index.
func classifyCorrespondence(ix *corrIndex, targetSeq int, targetSHA string) (revertResolution, error) {
	if entry, ok := ix.exact(targetSHA); ok {
		return revertResolution{Exact: true, Entry: entry}, nil
	}
	if entry, ok := ix.nearestAtOrBefore(targetSeq); ok {
		return revertResolution{Exact: false, Entry: entry}, nil
	}
	return revertResolution{}, fmt.Errorf("%w: warp SHA %s", ErrNoCorrespondence, targetSHA)
}

// resolveRevertTarget resolves warpSHA to the weft SHA that a diff since warpSHA bridges to.
// It mutates nothing.
// It checks that the resolved weft SHA still exists, and on a stale hit it rebuilds the index once and resolves again.
// It returns a wrapped ErrStaleSHA when the resolved weft SHA still does not exist after the rebuild.
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
	if f.records.SHAExists(res.Entry.WeftSHA) {
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
	if res, err = classifyCorrespondence(ix, targetSeq, warpSHA); err == nil && f.records.SHAExists(res.Entry.WeftSHA) {
		return res, nil
	}

	return revertResolution{}, fmt.Errorf("%w: warp SHA %s, weft SHA %s", ErrStaleSHA, warpSHA, staleWeftSHA)
}
