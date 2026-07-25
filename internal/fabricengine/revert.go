// revert.go — RevertWithWeft, the all-or-nothing coordinated revert: reset
// both warp and weft to the point corresponding to a caller-supplied warp
// SHA. Resolution (finding which weft SHA corresponds) happens entirely
// before any mutation, so an unresolvable target leaves both repos
// untouched; classifyCorrespondence is the pure gap-classification core,
// extracted so it is testable without spawning git.

package fabricengine

import (
	"errors"
	"fmt"
)

// ErrRevertRollbackFailed is returned by RevertWithWeft when the weft reset
// fails after warp was already reset, AND the warp rollback attempting to
// undo that partial mutation also fails — the one case where RevertWithWeft
// cannot guarantee the all-or-nothing contract and must instead report both
// repos' actual, possibly-inconsistent state loudly rather than silently.
var ErrRevertRollbackFailed = errors.New("fabricengine: revert rollback failed, repository state may be inconsistent")

// RevertResult reports what RevertWithWeft resolved and did: Exact reports
// whether the requested warp SHA had its own recorded correspondence
// (versus falling back to the nearest older one); WarpSHA/WeftSHA are the
// SHAs both repos were reset to; GapFrom/GapTo are populated only when
// Exact is false, naming the warp-SHA range between the resolved
// correspondence and the originally requested warp SHA — the range a caller
// (raddle, a human) should treat as potentially stale in weft.
type RevertResult struct {
	Exact   bool
	WarpSHA string
	WeftSHA string
	GapFrom string
	GapTo   string
}

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

// resolveRevertTarget resolves warpSHA to the weft SHA RevertWithWeft should
// reset to, mutating nothing: it loads the correspondence index, classifies
// the target via classifyCorrespondence, and validates the resolved weft SHA
// with f.Weft.SHAExists — retrying once via RebuildIndex on a stale hit,
// exactly like WeftSHAForWarpSHA's self-correction. Returns wrapped
// ErrStaleSHA when the resolved weft SHA still fails to exist after the
// rebuild retry.
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

// RevertWithWeft resets both the warp and weft repos to the point
// corresponding to warpSHA. Resolution happens entirely before any
// mutation: warpSHA must exist in the warp repo (else wrapped ErrStaleSHA);
// the correspondence index resolves it to a weft SHA — exactly, or to the
// nearest older correspondence with the resulting gap reported in the
// result (see RevertResult); an unresolvable target (no correspondence at
// or before warpSHA at all) returns wrapped ErrNoCorrespondence, and a
// resolved weft SHA that still fails to exist after a rebuild retry returns
// wrapped ErrStaleSHA — in every resolution-failure case, neither repo is
// touched.
//
// Once resolved, RevertWithWeft owns both resets: warp first, then weft. If
// the weft reset fails after warp was already moved, warp is rolled back to
// its pre-revert SHA so a failed revert never leaves the pair straddling two
// different points (mirroring the all-or-nothing discipline a coordinated
// checkout needs). If that rollback itself also fails, RevertWithWeft
// returns wrapped ErrRevertRollbackFailed naming both repos' actual current
// SHAs, since the all-or-nothing contract can no longer be guaranteed and
// silence would hide a genuinely inconsistent pair.
func (f *Fabric) RevertWithWeft(warpSHA string) (RevertResult, error) {
	if !f.Warp.SHAExists(warpSHA) {
		return RevertResult{}, fmt.Errorf("%w: warp SHA %s does not exist in the warp repo", ErrStaleSHA, warpSHA)
	}

	targetSeq, err := f.warpSeq(warpSHA)
	if err != nil {
		return RevertResult{}, err
	}

	res, err := f.resolveRevertTarget(warpSHA, targetSeq)
	if err != nil {
		return RevertResult{}, err
	}
	weftSHA := res.Entry.WeftSHA

	// Resolution is complete and nothing has been mutated yet; capture the
	// warp repo's current position so a failed weft reset can be rolled
	// back to it below.
	preRevertSHA, err := f.Warp.CurrentSHA()
	if err != nil {
		return RevertResult{}, err
	}

	if err := f.Warp.ResetHard(warpSHA); err != nil {
		return RevertResult{}, err
	}

	if err := f.Weft.ResetHard(weftSHA); err != nil {
		if rollbackErr := f.Warp.ResetHard(preRevertSHA); rollbackErr != nil {
			warpNow, _ := f.Warp.CurrentSHA()
			weftNow, _ := f.Weft.CurrentSHA()
			return RevertResult{}, fmt.Errorf("%w: weft reset to %s failed (%v); warp rollback to %s also failed (%v); warp is now at %s, weft is now at %s",
				ErrRevertRollbackFailed, weftSHA, err, preRevertSHA, rollbackErr, warpNow, weftNow)
		}
		return RevertResult{}, fmt.Errorf("fabricengine: weft reset to %s failed (warp rolled back to %s): %w", weftSHA, preRevertSHA, err)
	}

	result := RevertResult{Exact: res.Exact, WarpSHA: warpSHA, WeftSHA: weftSHA}
	if !res.Exact {
		result.GapFrom = res.Entry.WarpSHA
		result.GapTo = warpSHA
	}
	return result, nil
}
