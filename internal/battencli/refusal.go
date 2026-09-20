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
// This is also why refuseNonPrime treats a non-nil primeNameErr as a refusal rather than passing it
// through unresolved, in deliberate contrast with refusePrimeSlug's own choice: refusePrimeSlug
// treats an unresolvable prime name as non-fatal because it is one guard among several that still
// refuse a bad slug on other grounds, while here it is the WHOLE check -- an unresolvable prime name
// means the hub geometry is already broken, and treating that as "probably fine, proceed" would
// drive the two bookend rows from an unverified vantage point, which is precisely what this check
// exists to prevent.

package battencli

import "fmt"

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
