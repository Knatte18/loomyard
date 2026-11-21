// resolve.go declares how a run-id becomes a run identity and a directory segment: SelfRunID is an
// alias for the worktree's own slug, and only this file interprets it, per the Shed Run-Directory
// Invariant.

package shedrun

import (
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// ResolveRunID returns the run's identity: SelfRunID maps to l.WorktreeName unconditionally, and any
// other run-id passes through unchanged.
// It is the identity every envelope and prompt reports, and the directory segment a run's paths join.
// It validates nothing: ValidateRunID still gates the result at every caller.
func ResolveRunID(l *lymcwd.Location, runID string) string {
	if runID == SelfRunID {
		return l.WorktreeName
	}
	return runID
}

// runSegment returns the directory segment a run's paths join under l, which is the run's identity.
func runSegment(l *lymcwd.Location, runID string) string {
	return ResolveRunID(l, runID)
}
