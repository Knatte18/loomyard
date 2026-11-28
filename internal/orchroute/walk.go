// walk.go declares the lookups of an orch's strand name and the walk up a beam chain to the orch a notice goes to.
// It includes Asker, which answers the orch strand the agents of a worktree reach as their parent orch.

package orchroute

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// StrandOf answers the name of the orch strand of worktree and whether its orch state records one.
// The name is the one reed's state holds for the recorded GUID.
// A GUID that reed's state does not hold is answered under a fixed name.
// The fixed name is `<shortname>:orch` for the prime and `<shortname>:<slug>:orch` for a beam.
// Creel decides whether that strand is listed.
func StrandOf(worktree *lymcwd.Location) (name string, recorded bool, err error) {
	orch, err := orchengine.LoadState(Paths(worktree))
	if err != nil {
		return "", false, err
	}
	if orch.Strand == "" {
		return "", false, nil
	}
	held, found, err := reedStrandName(worktree, orch.Strand)
	if err != nil {
		return "", false, err
	}
	if found {
		return held, true, nil
	}
	fixed, err := FixedStrandName(worktree)
	if err != nil {
		return "", false, err
	}
	return fixed, true, nil
}

// reedStrandName answers the name reed's state of worktree holds for the strand guid, and whether it holds that strand.
func reedStrandName(worktree *lymcwd.Location, guid string) (name string, found bool, err error) {
	reedState, err := reedengine.LoadState(filepath.Join(worktree.AnchorPath(), lymdirs.DotLymDirName))
	if err != nil || reedState == nil {
		return "", false, err
	}
	for _, strand := range reedState.Strands {
		if strand.GUID == guid {
			return strand.Name, true, nil
		}
	}
	return "", false, nil
}

// holdsOrchStrand reports whether the orch state of worktree records a strand that reed's state still holds.
// Stopping an orch removes its strand from reed's state, while the orch state keeps the record.
func holdsOrchStrand(worktree *lymcwd.Location) (bool, error) {
	orch, err := orchengine.LoadState(Paths(worktree))
	if err != nil || orch.Strand == "" {
		return false, err
	}
	_, found, err := reedStrandName(worktree, orch.Strand)
	return found, err
}

// FixedStrandName answers the name the orch strand of worktree takes when reed holds none for it:
// `<shortname>:orch` for the prime and `<shortname>:<slug>:orch` for a beam.
func FixedStrandName(worktree *lymcwd.Location) (string, error) {
	shortname, found := fabricengine.ReadShortname(fabriclayout.BoltDir(worktree.HubPath))
	if !found {
		return "", fmt.Errorf("orch: the hub of %s has no usable shortname record to name the orch strand; way forward: run `lym fabric shortname <shortname>`", worktree.WorktreePath())
	}
	primeName, err := fabricengine.PrimeName(worktree)
	if err != nil {
		return "", err
	}
	slug := worktree.WorktreeName
	if slug == primeName {
		slug = ""
	}
	return agentname.Format(shortname, slug, agentname.RoleOrch)
}

// Prime resolves the prime worktree of the hub that location's worktree belongs to.
func Prime(location *lymcwd.Location) (*lymcwd.Location, error) {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return nil, err
	}
	return lymcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName))
}

// WorktreeOrPrime resolves the hub worktree named worktree.
// It falls back to the prime of the hub of location when worktree is empty or does not exist.
func WorktreeOrPrime(location *lymcwd.Location, worktree string) (*lymcwd.Location, error) {
	if worktree != "" {
		root := filepath.Join(location.HubPath, worktree)
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			return lymcwd.ResolveWorktree(root)
		}
	}
	return Prime(location)
}

// Destination answers the orch a notice starting at the worktree start goes to, as its agent name and worktree.
// start takes it while reed still holds its orch strand, live or not, since creel holds the mail of a down orch.
// Otherwise it is the nearest worktree up start's beam chain whose orch live reports live, else the prime.
// So a stopped orch, whose strand is removed, passes its notices up.
// The answer's Worktree is empty for the prime.
func Destination(start *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (hubgeom.Parent, error) {
	return hubgeom.NearestLiveOrch(start, func(worktree *lymcwd.Location) (bool, error) {
		if worktree.WorktreeName == start.WorktreeName {
			held, err := holdsOrchStrand(worktree)
			if err != nil || held {
				return held, err
			}
		}
		return live(worktree)
	})
}

// From answers the strand of the orch a notice starting at start goes to, and whether its state records one.
func From(start *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (name string, recorded bool, err error) {
	destination, err := Destination(start, live)
	if err != nil {
		return "", false, err
	}
	worktree, err := WorktreeOrPrime(start, destination.Worktree)
	if err != nil {
		return "", false, err
	}
	return StrandOf(worktree)
}

// FromPair answers the orch strand a notice of pair goes to.
// The walk to it starts at the pair's parent.
// The parent comes from the pair's origin record, and the prime stands in when it names none or does not exist.
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
