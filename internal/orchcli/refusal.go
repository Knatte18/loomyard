// refusal.go declares the pure decisions behind orch's refusals: the pre-run's rule that every orch verb runs from the hub's prime worktree or a beam, and the bring-up's refusals of a worktree.
//
// refuseNonPrime is battencli's shape with orch wording, duplicated rather than imported because a <module>cli importing another <module>cli would couple two cobra seams.
// The caller runs fabricengine.RequireDrivableWorktree first, since the name comparison alone admits the prime's fabric sibling, whose own prime is itself.

package orchcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// beamFacts are what the bring-up's refusals read from the local Bolt and the worktree.
type beamFacts struct {
	// slug is the beam's slug, empty for the prime.
	slug string
	// here is this machine's name.
	here string
	// owner is the machine the worktree reads as owned by, empty for none.
	owner string
	// moveWayForward is the way forward of an anchor change standing on the worktree or the prime, empty when none stands.
	moveWayForward string
	// held is whether the worktree's entry is held by its run, and status is that entry's status.
	held   bool
	status string
}

// beamUpRefusal returns nil when an orch may come up in the beam slug, or in the prime when slug is empty, on the machine here.
// owner is the machine the worktree reads as owned by, empty for none, and moveWayForward the way forward of a standing anchor change, empty for none.
// A worktree another machine owns is refused naming the owner and the takeover, a standing anchor change is refused naming its way forward, and a beam whose entry its landing holds is refused naming the landing's status verb.
func beamUpRefusal(slug, here, owner string, moveWayForward string, held bool, status string) error {
	subject, adopted := fmt.Sprintf("beam %q", slug), slug
	if slug == "" {
		subject, adopted = "the prime", "root"
	}
	if boardengine.OwnedElsewhere(owner, here) {
		return fmt.Errorf("orch: %s is owned by machine %q, not this machine (%q); way forward: run `lyx adopt %s` from this machine's prime to take it over, then run `lyx beam start` again", subject, owner, here, adopted)
	}
	if moveWayForward != "" {
		return fmt.Errorf("orch: an anchor change stands on %s or the prime; way forward: %s", subject, moveWayForward)
	}
	if held {
		return fmt.Errorf("orch: %s is held by its landing run (status %q); way forward: watch it with `lyx batten status %s`, and run `lyx beam start` again once the landing has ended", subject, status, slug)
	}
	return nil
}

// refuseNonPrime returns nil only when primeNameErr is nil and worktreeName equals primeName or the worktree is a beam.
// An unresolvable prime name is a refusal, never a pass-through: it means the hub geometry is already broken,
// and orch must not create a session from an unverified vantage point.
func refuseNonPrime(worktreeName, primeName string, primeNameErr error, beam bool) error {
	if primeNameErr != nil {
		return fmt.Errorf("orch: cannot verify this is the hub's prime worktree: %w", primeNameErr)
	}
	if worktreeName == primeName || beam {
		return nil
	}
	return fmt.Errorf(
		"orch: lyx orch runs from the hub's prime worktree or a beam; %q is neither the prime worktree (%q is) nor a beam -- re-run it from the prime, or make a project a beam with `lyx beam create <slug>`",
		worktreeName, primeName,
	)
}
