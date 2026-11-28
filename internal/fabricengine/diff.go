// diff.go implements Fabric.Diff and Fabric.Status, two read-only views that label each changed path with its side.
// Diff lists what changed on both sides since a warp SHA, using the correspondence index for the weft side.
// Status lists what is uncommitted now on both sides, using each repo's worktree changes.
// Neither classifies paths; both merge each repo's own changed-file primitive.

package fabricengine

import (
	"errors"
	"fmt"
)

// weftAnchorForWarpSHA resolves warpSHA to the weft SHA it corresponds to, without reverting anything.
// A warpSHA predating any correspondence is reported as found=false rather than an error;
// a diff from before fabric tracked this pair has no weft baseline.
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

// ChangeSide names which repo of a warp<->weft pair a changed path belongs to.
type ChangeSide string

// The two sides of a warp<->weft pair that a changed path can be labelled with.
const (
	SideWarp ChangeSide = "warp"
	SideWeft ChangeSide = "weft"
)

// ChangeEntry is one repo-relative path that changed on one side of a warp<->weft pair.
// Its Side field names that side.
type ChangeEntry struct {
	Path string
	Side ChangeSide
}

// DiffResult is the merged, side-labelled set of paths that changed across a warp<->weft pair since a warp SHA.
// NoWeftCorrespondence reports whether that warp SHA predates any recorded weft correspondence;
// in that case Entries holds only warp-side changes.
type DiffResult struct {
	Entries              []ChangeEntry
	NoWeftCorrespondence bool
}

// Diff reports what changed on both sides of the warp<->weft pair since sinceWarpSHA.
// It resolves sinceWarpSHA to the nearest correspondence at or before it when no exact match exists.
// When no weft correspondence exists for sinceWarpSHA, the weft side is empty;
// DiffResult.NoWeftCorrespondence is true rather than an error.
func (f *Fabric) Diff(sinceWarpSHA string) (DiffResult, error) {
	warpFiles, err := f.code.ChangedFilesSince(sinceWarpSHA)
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

	weftFiles, err := f.records.ChangedFilesSince(weftAnchor)
	if err != nil {
		return DiffResult{}, fmt.Errorf("fabricengine: diff weft side: %w", err)
	}
	for _, path := range weftFiles {
		entries = append(entries, ChangeEntry{Path: path, Side: SideWeft})
	}

	return DiffResult{Entries: entries}, nil
}

// DirtyPaths returns the tracked uncommitted paths of both checkouts of the pair, with no side label.
// Each path is absolute, under its own checkout.
// It reads the same tracked scope as the dirty-worktree merge guard;
// untracked files, which the guard never objected to, are not listed.
func (f *Fabric) DirtyPaths() ([]string, error) {
	warpPaths, err := trackedDirtyPaths(f.warpPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: dirty paths warp side: %w", err)
	}
	weftPaths, err := trackedDirtyPaths(f.weftPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: dirty paths weft side: %w", err)
	}
	return append(warpPaths, weftPaths...), nil
}

// CodeDirtyPaths returns the code checkout's absolute path and its tracked uncommitted paths.
// The paths are relative to that checkout, slash-separated and sorted.
// It is the read the dirty merge guard decides on; it never lists the records checkout.
// A read failure is returned wrapped.
func (f *Fabric) CodeDirtyPaths() (checkout string, paths []string, err error) {
	paths, err = codeDirtyPaths(f)
	if err != nil {
		return "", nil, fmt.Errorf("fabricengine: code checkout dirty paths: %w", err)
	}
	return f.warpPath, paths, nil
}

// CodeWorktreeChanges returns every uncommitted path of the code checkout's live worktree, untracked files included.
// The paths are relative to that checkout and slash-separated; it never lists the records checkout.
func (f *Fabric) CodeWorktreeChanges() ([]string, error) {
	paths, err := f.code.WorktreeChangedFiles()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: code checkout worktree changes: %w", err)
	}
	return paths, nil
}

// Status reports every uncommitted path on both sides of the warp<->weft pair, merged into one side-labelled slice.
// It reads each checkout's live worktree, so no correspondence is involved.
func (f *Fabric) Status() ([]ChangeEntry, error) {
	warpFiles, err := f.code.WorktreeChangedFiles()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: status warp side: %w", err)
	}

	weftFiles, err := f.records.WorktreeChangedFiles()
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
