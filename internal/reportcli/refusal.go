// refusal.go declares refuseNonPrime, the pure decision behind the report's prime-only rule: the convergence report reads the hub's history from the prime worktree only.
//
// The caller runs fabricengine.RequireDrivableWorktree first, since the name comparison alone admits the prime's records-side sibling, whose own prime is itself.

package reportcli

import "fmt"

// refuseNonPrime returns nil only when primeNameErr is nil and worktreeName equals primeName.
// An unresolvable prime name is a refusal, never a pass-through: it means the hub geometry is already broken, and the report must not read history from an unverified vantage point.
func refuseNonPrime(worktreeName, primeName string, primeNameErr error) error {
	if primeNameErr != nil {
		return fmt.Errorf("report: cannot verify this is the hub's prime worktree: %w", primeNameErr)
	}
	if worktreeName == primeName {
		return nil
	}
	return fmt.Errorf(
		"report: lyx report convergence runs from the hub's prime worktree only; %q is not the prime worktree (%q is) -- re-run it from there",
		worktreeName, primeName,
	)
}
