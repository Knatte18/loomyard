// refusal.go declares the pure decisions behind orch's refusals: the pre-run's rule that every orch verb runs from the hub's prime worktree or a beam, and the start-time refusals of a beam.
//
// refuseNonPrime is battencli's shape with orch wording, duplicated rather than imported because a <module>cli importing another <module>cli would couple two cobra seams.
// The caller runs fabricengine.RequireDrivableWorktree first, since the name comparison alone admits the prime's fabric sibling, whose own prime is itself.

package orchcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

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
		"orch: lyx orch runs from the hub's prime worktree or a beam; %q is neither the prime worktree (%q is) nor a beam -- re-run it from the prime, or make this pair a beam with `lyx beam create <slug>` for a project or `lyx beam promote <slug>` for a task",
		worktreeName, primeName,
	)
}

// refuseBeamStart returns nil when an orch may start in the beam slug on the machine here.
// owner is the machine the beam's entry reads as owned by, empty for none, and status is the entry's status.
// A beam another machine owns is refused naming the owner and the takeover, and a beam whose entry a run holds, its landing, is refused naming the landing's status verb.
func refuseBeamStart(slug, here, owner string, status *string) error {
	if boardengine.OwnedElsewhere(owner, here) {
		return fmt.Errorf("orch: beam %q is owned by machine %q, not this machine (%q); way forward: run `lyx adopt %s` from this machine's prime to take it over, then run `lyx orch start` again", slug, owner, here, slug)
	}
	if boardengine.IsRunStatus(status) {
		return fmt.Errorf("orch: beam %q is held by its landing run (status %q); way forward: watch it with `lyx batten status %s`, and run `lyx orch start` again once the landing has ended", slug, *status, slug)
	}
	return nil
}
