// target.go resolves the beam a `lym beam start` or `lym beam stop` acts on, and refuses a worktree that is no beam with the way forward, before any reconcile or orch step runs.

package beamcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// rootSlug is the slug that names the hub's prime, which has no beam slug of its own.
const rootSlug = "root"

// beamTarget is the worktree a start or stop acts on: the hub's prime or a beam's pair.
// Slug is empty for the prime, and Label is dobbyengine.PrimeLabel for the prime and the slug for a beam.
type beamTarget struct {
	Location *lymcwd.Location
	Slug     string
	Label    string
}

// primeTarget is the target of the hub's prime at prime.
func primeTarget(prime *lymcwd.Location) beamTarget {
	return beamTarget{Location: prime, Label: dobbyengine.PrimeLabel}
}

// resolveBeamTarget resolves the beam a verb run from the worktree at location acts on, and refuses before anything is reconciled or started.
// An empty slug names the acting worktree's beam, the prime or a marked beam pair, and the slug `root` names the prime.
// Any other slug names that beam from anywhere in the hub.
// A task pair, a worktree whose origin record carries no beam mark and a slug that names no local beam pair are refused, each with the command that reaches the beam.
func resolveBeamTarget(location *lymcwd.Location, slug string) (beamTarget, error) {
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return beamTarget{}, fmt.Errorf("lym beam: run it from the hub's prime worktree or from a beam: %w", err)
	}
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return beamTarget{}, fmt.Errorf("lym beam: cannot verify the hub's prime worktree: %w", err)
	}
	prime, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName))
	if err != nil {
		return beamTarget{}, fmt.Errorf("lym beam: resolve the hub's prime worktree: %w", err)
	}

	switch {
	case slug == rootSlug || slug == primeName:
		return primeTarget(prime), nil
	case slug == "" && location.WorktreeName == primeName:
		return primeTarget(prime), nil
	case slug == "":
		return markedTarget(prime, location, location.WorktreeName)
	case !pairWorktreePresent(prime, slug):
		return beamTarget{}, absentBeamRefusal(prime, slug)
	}
	pair, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
	if err != nil {
		return beamTarget{}, fmt.Errorf("lym beam: resolve the worktree of %s: %w", slug, err)
	}
	return markedTarget(prime, pair, slug)
}

// markedTarget is the target of the pair slug at pair, refused unless its own origin record carries the beam mark.
func markedTarget(prime, pair *lymcwd.Location, slug string) (beamTarget, error) {
	origin, found, err := fabricengine.ReadOrigin(pair)
	if err != nil {
		return beamTarget{}, fmt.Errorf("lym beam: read the origin record of %s to tell whether it is a beam: %w", slug, err)
	}
	if !found || !origin.Beam {
		return beamTarget{}, notABeamRefusal(prime, slug)
	}
	return beamTarget{Location: pair, Slug: slug, Label: slug}, nil
}

// notABeamRefusal refuses the pair slug as no beam, naming the beam it belongs to and that beam's worktree, where the verb runs.
// The prime stands for a pair that belongs to no beam, named `root`.
func notABeamRefusal(prime *lymcwd.Location, slug string) error {
	parent, err := hubgeom.ParentOf(prime, slug)
	if err != nil {
		return fmt.Errorf("lym beam: %s is not a beam, and the beam it belongs to is unreadable: %w", slug, err)
	}
	owner, worktree := rootSlug, prime.WorktreeName
	if parent != "" {
		owner, worktree = parent, parent
	}
	return fmt.Errorf("lym beam: %s is a task pair, not a beam; it belongs to %s; way forward: run `lym beam start` or `lym beam stop` in %s", slug, owner, fabricengine.WorktreePath(prime, worktree))
}

// absentBeamRefusal refuses a slug with no pair on this machine, naming `lym adopt` when its task branch is on origin and `lym beam create` from its parent worktree otherwise.
func absentBeamRefusal(prime *lymcwd.Location, slug string) error {
	config, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(prime))
	if err != nil {
		return fmt.Errorf("lym beam: %s has no pair on this machine, and the fabric config that tells whether its branch is on origin does not load: %w", slug, err)
	}
	onOrigin, err := fabricengine.NewTopology(config).TaskBranchOnOrigin(prime, slug)
	if err != nil {
		return fmt.Errorf("lym beam: %s has no pair on this machine, and origin cannot be asked for its branch: %w", slug, err)
	}
	if onOrigin {
		return fmt.Errorf("lym beam: %s has no pair on this machine, though its branch is on origin; way forward: run `lym adopt %s` to take it over", slug, slug)
	}
	parent, err := hubgeom.ParentOf(prime, slug)
	if err != nil {
		return fmt.Errorf("lym beam: %s has no pair on this machine, and its parent is unreadable: %w", slug, err)
	}
	if parent == "" {
		parent = prime.WorktreeName
	}
	return fmt.Errorf("lym beam: %s has no pair on this machine; way forward: run `lym beam create %s` from its parent worktree %s", slug, slug, parent)
}
