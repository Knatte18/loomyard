// refusal.go holds the pre-run guard of the batten verbs: which worktrees they may run from.
// Every batten verb runs from the hub's prime worktree, and ResolveBattenPrime also allows a beam.

package battencli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// RefuseUnlessPrime reports an error unless location is the hub's prime worktree.
// It runs fabric's drivable-worktree check first.
// The prime-name comparison alone passes in a fabric sibling repository, whose prime is itself.
// Every path derived from that sibling would then be invented.
// It is exported for the seeding verb outside this package.
// A batten seed written anywhere but the prime is a run no batten verb can drive,
// so that verb applies this same refusal.
func RefuseUnlessPrime(location *lymcwd.Location) error {
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return fmt.Errorf("battencli: this verb runs from the hub's prime worktree only: %w", err)
	}
	primeName, primeNameErr := fabricengine.PrimeName(location)
	return refuseNonPrime(location.WorktreeName, primeName, primeNameErr)
}

// ResolveBattenPrime is the guard of every batten verb.
// It returns the hub prime's location when location is the prime or a beam.
// A beam is a pair whose origin record carries the beam mark.
// A session in a beam acts on the hub as from the prime, and every path stays under the prime's anchor.
// Fabric's drivable-worktree check runs first.
// It refuses a beam whose board entry holds a run status, since only its landing works in it.
// The refusal names the landing and the beam's parent worktree.
// It refuses every other worktree, a task pair that is not a beam included.
// The refusal names it and the nearest beam above it, else the prime, as where to run the verb.
func ResolveBattenPrime(location *lymcwd.Location) (*lymcwd.Location, error) {
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return nil, fmt.Errorf("battencli: this verb runs from the hub's prime worktree or a beam: %w", err)
	}
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return nil, fmt.Errorf("battencli: cannot verify this is the hub's prime worktree: %w", err)
	}
	if location.WorktreeName == primeName {
		return location, nil
	}

	origin, found, err := fabricengine.ReadOrigin(location)
	if err != nil {
		return nil, fmt.Errorf("battencli: cannot read the origin record of %q to tell whether it is a beam: %w", location.WorktreeName, err)
	}
	if !found || !origin.Beam {
		return nil, fmt.Errorf(
			"battencli: this verb runs from the hub's prime worktree or a beam; %q is neither -- re-run it from %q",
			location.WorktreeName, nearestBeamAbove(location, origin, primeName),
		)
	}

	landing, err := beamLanding(location)
	if err != nil {
		return nil, err
	}
	if landing != "" {
		return nil, fmt.Errorf(
			"battencli: the beam %q is being landed (%s) and its landing works in it, so no batten, adopt, cleanup or beam verb runs from it -- re-run this verb from %q",
			location.WorktreeName, landing, parentWorktreeOrPrime(origin, primeName),
		)
	}
	return lymcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName))
}

// beamLanding returns the run status of the board entry named after the beam at location.
// It returns empty when the entry is absent or its run does not hold it.
// A pawl run from Worktree-Create through Beam-Await holds nothing, so a verb runs inside the beam then.
func beamLanding(location *lymcwd.Location) (string, error) {
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(location.HubPath))
	if err != nil {
		return "", fmt.Errorf("battencli: read the board to tell whether the beam %q is being landed: %w", location.WorktreeName, err)
	}
	for _, task := range tasks {
		if task.Slug == location.WorktreeName && boardengine.RunHolds(task) {
			return *task.Status, nil
		}
	}
	return "", nil
}

// parentWorktreeOrPrime returns the parent worktree that origin records, or the prime's name when origin records none.
func parentWorktreeOrPrime(origin fabricengine.Origin, primeName string) string {
	if origin.ParentWorktree == "" {
		return primeName
	}
	return origin.ParentWorktree
}

// nearestBeamAbove returns the name of the nearest beam above location in its origin chain.
// A beam counts only when its worktree exists, and the prime's name is returned when there is none.
// A record that cannot be read, a parent that is gone, or a cycle ends the walk.
// The walk then returns the prime's name, since the answer only names where to run a verb.
func nearestBeamAbove(location *lymcwd.Location, origin fabricengine.Origin, primeName string) string {
	visited := map[string]bool{location.WorktreeName: true}
	for parent := origin.ParentWorktree; parent != "" && parent != primeName && !visited[parent]; {
		visited[parent] = true
		if _, err := os.Stat(filepath.Join(fabricengine.WorktreePath(location, parent), ".git")); err != nil {
			break
		}
		parentOrigin, found, err := fabricengine.ReadOriginFor(location, parent)
		if err != nil || !found {
			break
		}
		if parentOrigin.Beam {
			return parent
		}
		parent = parentOrigin.ParentWorktree
	}
	return primeName
}

// refuseOwnPair refuses a verb that creates or destroys the pair slug when the acting session runs in that pair.
// The refusal names the pair's parent worktree, or the prime, as where to run the verb.
func refuseOwnPair(acting, prime *lymcwd.Location, slug, verb string) error {
	if acting.WorktreeName == prime.WorktreeName || acting.WorktreeName != slug {
		return nil
	}
	where := prime.WorktreeName
	if origin, found, err := fabricengine.ReadOrigin(acting); err == nil && found {
		where = parentWorktreeOrPrime(origin, prime.WorktreeName)
	}
	return fmt.Errorf(
		"battencli: %s would create or destroy the pair %q it runs in -- re-run it from %q",
		verb, slug, where,
	)
}

// refuseNonPrime returns nil only when primeNameErr is nil and worktreeName equals primeName.
//
// An unresolvable prime name is a refusal, never a pass: the hub geometry is already broken.
// Proceeding would drive the create and teardown rows from an unverified vantage point.
// The returned refusal wraps primeNameErr, so the caller can always render it on the envelope.
//
// When the two names differ it returns a refusal naming both.
// The refusal states that this verb runs from the hub's prime worktree only.
// It tells the operator to re-run the verb there.
func refuseNonPrime(worktreeName, primeName string, primeNameErr error) error {
	if primeNameErr != nil {
		return fmt.Errorf("battencli: cannot verify this is the hub's prime worktree: %w", primeNameErr)
	}
	if worktreeName == primeName {
		return nil
	}
	return fmt.Errorf(
		"battencli: this verb runs from the hub's prime worktree only; %q is not the prime worktree (%q is) -- re-run it from there",
		worktreeName, primeName,
	)
}
