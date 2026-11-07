// Package locationkit builds synthetic, unresolved locations for path-arithmetic tests.
//
// A location built here never touches disk and imports only internal/lymcwd.
// It is no substitute for lymcwd.Resolve where a test needs a real worktree.
package locationkit

import "github.com/Knatte18/loomyard/internal/lymcwd"

// Location returns a *lymcwd.Location with HubPath, WorktreeName and AnchorRel set from its arguments.
// RepoName stays empty.
// It spawns no git command and reads no disk.
func Location(hubPath, worktreeName, anchorRel string) *lymcwd.Location {
	return &lymcwd.Location{
		HubPath:      hubPath,
		WorktreeName: worktreeName,
		AnchorRel:    anchorRel,
	}
}
