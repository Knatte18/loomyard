// parent.go implements ResolveParent, the one code path that turns a task worktree into its parent's agent name.

package hubgeom

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// Parent is the resolved parent of a task worktree's runs.
type Parent struct {
	// Name is the parent's full agent name, or empty for no parent.
	Name string
	// Worktree is the parent worktree's name, or empty when Name came from the legacy seed or there is no parent.
	Worktree string
}

// ResolveParent resolves the parent agent name of the pair l addresses.
// The pair's origin record names the worktree it was created from, and the parent is that worktree's orch role;
// a pair whose origin records no parent worktree falls back to the Parent its default run's seed carries, written before the origin record held one.
// Neither the origin record nor the seed is created or modified, and no liveness is checked:
// the name is returned even when no orch strand is live.
//
// A hub with no recorded shortname yields no parent and a warning, since no name can be formed.
// The name depends only on the recorded worktree name and the shortname, never on the parent worktree still existing.
// Origin and seed read errors, and any other error telling the prime from a pair, are returned.
func ResolveParent(l *lyxcwd.Location) (Parent, error) {
	origin, originFound, err := fabricengine.ReadOriginFor(l, l.WorktreeName)
	if err != nil {
		return Parent{}, fmt.Errorf("hubgeom: read origin record for %q: %w", l.WorktreeName, err)
	}
	seed, seedFound, err := shedrun.ReadSeed(l, shedrun.SelfRunID)
	if err != nil {
		return Parent{}, fmt.Errorf("hubgeom: read default run seed for %q: %w", l.WorktreeName, err)
	}
	legacy := ""
	if seedFound {
		legacy = seed.Parent
	}
	shortname, shortnameFound := fabricengine.ReadShortname(fabricengine.BoardDir(l.HubPath))
	if !shortnameFound {
		logger.Warn("hubgeom: hub has no recorded shortname; the run gets no parent", "worktree", l.WorktreeName)
		return Parent{}, nil
	}
	parentIsPrime := false
	if originFound && origin.ParentWorktree != "" {
		parentIsPrime, err = parentWorktreeIsPrime(filepath.Join(l.HubPath, origin.ParentWorktree))
		if err != nil {
			return Parent{}, err
		}
	}
	return decideParent(shortname, origin, originFound, legacy, parentIsPrime)
}

// parentWorktreeIsPrime reports whether the worktree at root is the prime.
// A root with no .git entry is a parent pair since removed, which is not the prime:
// the prime exists while any pair does.
func parentWorktreeIsPrime(root string) (bool, error) {
	if _, err := os.Stat(filepath.Join(root, gitEntryName)); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return isPrimeWorktree(root)
}

// decideParent is ResolveParent's decision over already-read values, so its table needs no filesystem.
// An empty shortname yields no parent.
// parentIsPrime is read only when origin names a parent worktree.
func decideParent(shortname string, origin fabricengine.Origin, originFound bool, legacySeedParent string, parentIsPrime bool) (Parent, error) {
	if shortname == "" {
		return Parent{}, nil
	}
	if originFound && origin.ParentWorktree != "" {
		slug := origin.ParentWorktree
		if parentIsPrime {
			slug = ""
		}
		name, err := agentname.Format(shortname, slug, agentname.RoleOrch)
		if err != nil {
			return Parent{}, fmt.Errorf("hubgeom: form parent agent name for worktree %q: %w", origin.ParentWorktree, err)
		}
		return Parent{Name: name, Worktree: origin.ParentWorktree}, nil
	}
	return Parent{Name: legacySeedParent}, nil
}
