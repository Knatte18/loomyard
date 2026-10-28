// walk.go declares the lookups of an orch's strand name and the walk up a beam chain to the nearest live orch, including Asker, the rule a worktree's agents reach their parent orch by.

package orchroute

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// StrandOf answers the name of the orch strand of worktree and whether its orch state records one.
// The name is the one reed's state holds for the recorded GUID; a GUID reed no longer holds is answered under the fixed name, `<shortname>:orch` for the prime and `<shortname>:<slug>:orch` for a beam, so creel decides whether that strand is listed.
func StrandOf(worktree *lymcwd.Location) (name string, recorded bool, err error) {
	orch, err := orchengine.LoadState(Paths(worktree))
	if err != nil {
		return "", false, err
	}
	if orch.Strand == "" {
		return "", false, nil
	}
	reedState, err := reedengine.LoadState(filepath.Join(worktree.AnchorPath(), lymdirs.DotLymDirName))
	if err != nil {
		return "", false, err
	}
	if reedState != nil {
		for _, strand := range reedState.Strands {
			if strand.GUID == orch.Strand {
				return strand.Name, true, nil
			}
		}
	}
	shortname, found := fabricengine.ReadShortname(fabricengine.BoardDir(worktree.HubPath))
	if !found {
		return "", false, fmt.Errorf("orch: the hub of %s has no usable shortname record to name the orch strand; way forward: run `lym fabric shortname <shortname>`", worktree.WorktreePath())
	}
	primeName, err := fabricengine.PrimeName(worktree)
	if err != nil {
		return "", false, err
	}
	slug := worktree.WorktreeName
	if slug == primeName {
		slug = ""
	}
	fixed, err := agentname.Format(shortname, slug, agentname.RoleOrch)
	if err != nil {
		return "", false, err
	}
	return fixed, true, nil
}

// Prime resolves the prime worktree of the hub that location's worktree belongs to.
func Prime(location *lymcwd.Location) (*lymcwd.Location, error) {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return nil, err
	}
	return lymcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName))
}

// WorktreeOrPrime resolves the hub worktree named worktree, and the prime of the hub of location when worktree is empty or no longer exists.
func WorktreeOrPrime(location *lymcwd.Location, worktree string) (*lymcwd.Location, error) {
	if worktree != "" {
		root := filepath.Join(location.HubPath, worktree)
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			return lymcwd.ResolveWorktree(root)
		}
	}
	return Prime(location)
}

// From answers the orch strand of the nearest worktree, from start up its beam chain, whose orch live reports live, else the prime's.
func From(start *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (name string, recorded bool, err error) {
	destination, err := hubgeom.NearestLiveOrch(start, live)
	if err != nil {
		return "", false, err
	}
	worktree, err := WorktreeOrPrime(start, destination.Worktree)
	if err != nil {
		return "", false, err
	}
	return StrandOf(worktree)
}

// FromPair answers the orch strand a notice of pair goes to: the nearest worktree up from the pair's parent whose orch live reports live, else the prime's.
// The parent worktree comes from the pair's origin record, and the prime stands in when it names none or is gone.
func FromPair(pair *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (name string, recorded bool, err error) {
	parent, err := hubgeom.ResolveParent(pair)
	if err != nil {
		return "", false, err
	}
	start, err := WorktreeOrPrime(pair, parent.Worktree)
	if err != nil {
		return "", false, err
	}
	return From(start, live)
}

// Asker answers the orch strand the agents of location reach as their parent orch, and whether its orch state records one.
// From the prime or a beam the walk starts at that worktree itself; from any other worktree, a pair, it starts at the pair's recorded parent.
func Asker(location *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (name string, recorded bool, err error) {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return "", false, err
	}
	if location.WorktreeName == primeName {
		return From(location, live)
	}
	origin, found, err := fabricengine.ReadOriginFor(location, location.WorktreeName)
	if err != nil {
		return "", false, err
	}
	if found && origin.Beam {
		return From(location, live)
	}
	return FromPair(location, live)
}
