// resolve.go declares how a run-id becomes a run identity and a directory segment: SelfRunID is an
// alias for the worktree's own slug, and only this file interprets it, per the Shed Run-Directory
// Invariant.

package shedrun

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// ResolveRunID returns the run's identity: SelfRunID maps to l.WorktreeName unconditionally, and any
// other run-id passes through unchanged.
// It is the identity every envelope and prompt reports, kept apart from the directory segment paths
// join, which runSegment answers.
// It validates nothing: ValidateRunID still gates the result at every caller.
func ResolveRunID(l *lyxcwd.Location, runID string) string {
	if runID == SelfRunID {
		return l.WorktreeName
	}
	return runID
}

// runSegment returns the directory segment a run's paths join under l.
// It is ResolveRunID's answer, except under the legacy fallback: when _lyx/shed/<slug>/ is absent
// under l.AnchorPath() and _lyx/shed/self/ exists, both SelfRunID and the slug join the "self"
// segment, so a run started before the slug rename keeps working with no on-disk migration.
// The fallback only reads the filesystem and never creates or moves a directory.
func runSegment(l *lyxcwd.Location, runID string) string {
	resolved := ResolveRunID(l, runID)
	if resolved != l.WorktreeName {
		return resolved
	}
	shedDir := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, shedDirName)
	if _, err := os.Stat(filepath.Join(shedDir, resolved)); err == nil || !os.IsNotExist(err) {
		return resolved
	}
	if info, err := os.Stat(filepath.Join(shedDir, SelfRunID)); err == nil && info.IsDir() {
		return SelfRunID
	}
	return resolved
}
