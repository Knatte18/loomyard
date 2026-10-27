// refusal.go declares refuseNonPrime, the pure decision behind this package's pre-run refusal: a
// batten verb runs only when invoked from the hub's prime worktree.
//
// The check is added rather than inherited. internal/fabricengine's own topology layer refuses
// removing the hub's prime slug (refusePrimeSlug in remove.go), but that check compares a NAMED
// slug argument against the prime name -- it says nothing about which worktree the caller is
// standing in, and never refuses removing the worktree that IS the caller's own process working
// directory. Without a check of its own, a batten run driven from a task worktree could tear
// down the very worktree it is running inside of, which is exactly what this package's two bookend
// rows (create and teardown) must never do.
//
// refuseNonPrime is half of the guard. It compares names inside ONE repository, and the prime's own
// fabric sibling is a repository of its own whose prime is itself, so standing in that sibling the
// comparison passes while every path fabric would then derive is invented -- a sibling of the
// sibling. armAt (arm.go) therefore calls fabricengine.RequireDrivableWorktree first, which is the
// one place fabric's own vocabulary can tell the side batten drives from the sibling and the _board
// checkout.
//
// This is also why refuseNonPrime treats a non-nil primeNameErr as a refusal rather than passing it
// through unresolved, in deliberate contrast with refusePrimeSlug's own choice: refusePrimeSlug
// treats an unresolvable prime name as non-fatal because it is one guard among several that still
// refuse a bad slug on other grounds, while here it is the WHOLE check -- an unresolvable prime name
// means the hub geometry is already broken, and treating that as "probably fine, proceed" would
// drive the two bookend rows from an unverified vantage point, which is precisely what this check
// exists to prevent.

package battencli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// RefuseUnlessPrime is the whole prime-only guard as one call: fabric's own drivable-worktree check
// first, then the prime-name comparison, in that order for the reason armAt gives.
// It is exported for the one site outside this package that creates batten runs, "lym shed seed":
// a batten seed written anywhere but prime is a run no verb can ever drive, so the seeding site
// applies the same refusal every batten verb applies.
func RefuseUnlessPrime(location *lymcwd.Location) error {
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return fmt.Errorf("battencli: this verb runs from the hub's prime worktree only: %w", err)
	}
	primeName, primeNameErr := fabricengine.PrimeName(location)
	return refuseNonPrime(location.WorktreeName, primeName, primeNameErr)
}

// ResolveBattenPrime is the guard of every batten verb, `lym adopt` and `lym cleanup`: it returns the hub prime's location when location is the prime or a beam.
// A beam is a pair whose own origin record carries the beam mark, so a session in a beam acts on the hub as from the prime, and every path stays under the prime's anchor.
// Fabric's drivable-worktree check runs first.
// It refuses a beam whose board entry holds a run status, since only its landing works in it, naming the landing and the beam's parent worktree.
// It refuses every other worktree, a task pair that is not a beam included, naming it and the nearest beam above it, else the prime, as where to run the verb.
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

// beamLanding returns the run status of the board entry named after the beam at location; empty when the entry is absent or its run does not hold it.
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

// parentWorktreeOrPrime returns the parent worktree origin records, the prime's name when it records none.
func parentWorktreeOrPrime(origin fabricengine.Origin, primeName string) string {
	if origin.ParentWorktree == "" {
		return primeName
	}
	return origin.ParentWorktree
}

// nearestBeamAbove returns the name of the nearest worktree up from location's origin chain that exists here and is a beam, the prime's name when none is.
// A record that cannot be read, a parent that is gone and a cycle each end the walk at the prime, since the answer only names where to run a verb.
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

// refuseOwnPair refuses a verb that creates or destroys the pair of slug when the session acting runs in that pair itself, naming its parent worktree, or the prime, as where to run it.
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
// When primeNameErr is non-nil it returns a refusal naming that error -- never nil and never a hard
// error of its own, so the caller can always render it on the envelope.
//
// When the two names differ it returns a refusal naming both, stating that this verb runs from the
// hub's prime worktree only, and telling the operator to re-run it there.
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
