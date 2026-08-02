// diff.go implements the unified Fabric.Diff and Fabric.Status: two
// read-only, side-labelled views over what changed across a warp<->weft
// pair, distinct from status.go's Topology.Status (the paired host<->weft
// topology/branch-drift view). Fabric.Diff answers "what changed since this
// warp SHA, on both sides" via the correspondence index; Fabric.Status
// answers "what is currently uncommitted, on both sides" via
// gitrepo.Repo.WorktreeChangedFiles. Neither classifies paths or calls
// WiredNames — both are pure merges over each repo's own changed-file
// primitive.

package fabricengine

import (
	"errors"
	"fmt"
)

// weftAnchorForWarpSHA resolves warpSHA to the weft SHA Fabric.Diff should
// anchor its weft-side comparison to, via the same exact-then-nearest-older
// resolveRevertTarget resolver — but bridging, not reverting: nothing is
// reset here. A warpSHA older than the
// first recorded correspondence is a valid pre-lyx state, not an error, so
// that case is reported as found=false rather than propagating
// ErrNoCorrespondence: a caller diffing since before fabric started tracking
// this pair has no weft baseline to compare against, and that is expected,
// not exceptional.
func (f *Fabric) weftAnchorForWarpSHA(warpSHA string) (weftSHA string, found bool, err error) {
	targetSeq, err := f.warpSeq(warpSHA)
	if err != nil {
		return "", false, err
	}

	res, err := f.resolveRevertTarget(warpSHA, targetSeq)
	if err != nil {
		if errors.Is(err, ErrNoCorrespondence) {
			return "", false, nil
		}
		return "", false, err
	}
	return res.Entry.WeftSHA, true, nil
}

// ChangeSide names which repo of a warp<->weft pair a ChangeEntry's path
// changed in.
type ChangeSide string

// The two sides a ChangeEntry can be labelled with.
const (
	SideWarp ChangeSide = "warp"
	SideWeft ChangeSide = "weft"
)

// ChangeEntry is one repo-relative path that changed on one side of a
// warp<->weft pair, labelled with which side it changed on. Named
// ChangeEntry (not reusing status.go's PairStatus/StatusResult, a different
// paired-topology view) to keep this unified "what changed in my worktree"
// surface distinct from that paired-topology view — two separate surfaces,
// not variations of one.
type ChangeEntry struct {
	Path string
	Side ChangeSide
}

// DiffResult is Fabric.Diff's result: the merged, side-labelled set of
// changed paths, plus NoWeftCorrespondence reporting whether sinceWarpSHA
// predates any recorded weft correspondence (in which case Entries carries
// only warp-side changes).
type DiffResult struct {
	Entries              []ChangeEntry
	NoWeftCorrespondence bool
}

// Diff reports what changed on both sides of the warp<->weft pair since
// sinceWarpSHA: warp-side changes are sinceWarpSHA..HEAD in the warp repo
// (via Warp.ChangedFilesSince); weft-side changes are computed against the
// nearest-at-or-before weft SHA correspondence resolves sinceWarpSHA to (via
// weftAnchorForWarpSHA's underlying resolveRevertTarget resolver), not an
// exact match, since an exact correspondence entry for sinceWarpSHA need not
// exist. When no weft correspondence exists at or before
// sinceWarpSHA at all, the weft side is empty and
// DiffResult.NoWeftCorrespondence is true rather than an error: a diff since
// before fabric started tracking this pair has no weft baseline, which is a
// valid answer, not a failure.
func (f *Fabric) Diff(sinceWarpSHA string) (DiffResult, error) {
	warpFiles, err := f.Warp.ChangedFilesSince(sinceWarpSHA)
	if err != nil {
		return DiffResult{}, fmt.Errorf("fabricengine: diff warp side: %w", err)
	}

	var entries []ChangeEntry
	for _, path := range warpFiles {
		entries = append(entries, ChangeEntry{Path: path, Side: SideWarp})
	}

	weftAnchor, found, err := f.weftAnchorForWarpSHA(sinceWarpSHA)
	if err != nil {
		return DiffResult{}, fmt.Errorf("fabricengine: resolve weft anchor: %w", err)
	}
	if !found {
		return DiffResult{Entries: entries, NoWeftCorrespondence: true}, nil
	}

	weftFiles, err := f.Weft.ChangedFilesSince(weftAnchor)
	if err != nil {
		return DiffResult{}, fmt.Errorf("fabricengine: diff weft side: %w", err)
	}
	for _, path := range weftFiles {
		entries = append(entries, ChangeEntry{Path: path, Side: SideWeft})
	}

	return DiffResult{Entries: entries}, nil
}

// Status reports every currently-uncommitted path across both sides of the
// warp<->weft pair, merged into one side-labelled slice via each repo's
// gitrepo.Repo.WorktreeChangedFiles. Unlike Diff, there is no correspondence
// anchor involved — this is a live worktree read, not a since-SHA comparison
// — so there is no NoWeftCorrespondence case to report.
func (f *Fabric) Status() ([]ChangeEntry, error) {
	warpFiles, err := f.Warp.WorktreeChangedFiles()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: status warp side: %w", err)
	}

	weftFiles, err := f.Weft.WorktreeChangedFiles()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: status weft side: %w", err)
	}

	var entries []ChangeEntry
	for _, path := range warpFiles {
		entries = append(entries, ChangeEntry{Path: path, Side: SideWarp})
	}
	for _, path := range weftFiles {
		entries = append(entries, ChangeEntry{Path: path, Side: SideWeft})
	}
	return entries, nil
}
