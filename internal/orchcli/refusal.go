// refusal.go declares refuseNonPrime, the pure decision behind orch's pre-run refusal: every orch verb runs only from the hub's prime worktree.
//
// It is battencli's shape with orch wording, duplicated rather than imported because a <module>cli importing another <module>cli would couple two cobra seams.
// The caller runs fabricengine.RequireDrivableWorktree first, since the name comparison alone admits the prime's fabric sibling, whose own prime is itself.

package orchcli

import "fmt"

// refuseNonPrime returns nil only when primeNameErr is nil and worktreeName equals primeName.
// An unresolvable prime name is a refusal, never a pass-through: it means the hub geometry is already broken,
// and orch must not create a session from an unverified vantage point.
func refuseNonPrime(worktreeName, primeName string, primeNameErr error) error {
	if primeNameErr != nil {
		return fmt.Errorf("orch: cannot verify this is the hub's prime worktree: %w", primeNameErr)
	}
	if worktreeName == primeName {
		return nil
	}
	return fmt.Errorf(
		"orch: lyx orch runs from the hub's prime worktree only; %q is not the prime worktree (%q is) -- re-run it from there",
		worktreeName, primeName,
	)
}
