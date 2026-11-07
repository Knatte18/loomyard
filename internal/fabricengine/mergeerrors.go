// mergeerrors.go declares the typed errors of the merge verbs.
// It also declares the closed set of guard reasons that MergeGuardError carries.
// Every error message is a fixed string: none interpolates a branch name, a path or a git error.
// A caller that needs the offending branch reads it from a typed error's field, such as ErrMergeInRequired.Source.
// MergeGuardError.Checkout and MergeGuardError.DirtyPaths carry the dirty-guard detail in the same way.

package fabricengine

import (
	"sort"
	"strings"
)

// The closed set of guard-reason strings a *MergeGuardError may carry.
// A member is an aggregatable precondition, one of several reasons a single *MergeGuardError may carry at once.
// A terminal disposition with nothing to aggregate alongside it belongs in the typed errors below.
// No member may name a side, carry a path or imply an order.
// Adding a member means changing this list and pinnedMergeReasons in mergevocab_test.go.
// TestMergeVocabulary_GuardReasonSetMatchesConstBlock enforces this.
// It parses every production file in the package for mergeReason* constants.
// It also enforces that this file is the set's only home.
const (
	mergeReasonAlreadyInProgress   = "merge already in progress"
	mergeReasonUnresolvedConflicts = "unresolved conflicts remain"
	mergeReasonWorktreeDirty       = "worktree dirty"
	mergeReasonNotSynced           = "branch not synced to upstream"
	mergeReasonSourceNotFound      = "source branch not found"
	mergeReasonNotFabricManaged    = "source branch is not fabric-managed"
	mergeReasonDetachedHead        = "checkout is not on a branch"
	mergeReasonAttemptIncomplete   = "merge attempt did not reach both sides"
	mergeReasonConcludeLanded      = "merge conclude already landed"
	mergeReasonRecordedMergeGone   = "checkout no longer carries the recorded merge"
)

// MergeGuardError aggregates every failed merge precondition as a sorted, deduplicated list of
// fixed reason strings drawn from the package's closed set of guard reasons.
// It reports every failure rather than the first.
// Two scenarios that differ only in which precondition failed on which side therefore produce byte-identical errors.
// The error then discloses no failure order.
// Checkout and DirtyPaths carry the detail of the dirty-worktree reason, outside Error():
// Checkout is the absolute path of the code checkout the guard read.
// DirtyPaths lists its tracked dirty paths relative to it, slash-separated and sorted.
// Both are set only beside that reason, and DirtyPaths is then never empty.
type MergeGuardError struct {
	Reasons    []string
	Checkout   string
	DirtyPaths []string
}

// Error implements the error interface, joining the aggregated reasons with "; ".
func (e *MergeGuardError) Error() string {
	return "fabricengine: merge preconditions failed: " + strings.Join(e.Reasons, "; ")
}

// has reports whether e's aggregated reasons include reason.
func (e *MergeGuardError) has(reason string) bool {
	for _, r := range e.Reasons {
		if r == reason {
			return true
		}
	}
	return false
}

// WorktreeDirty reports whether e's aggregated reasons include the dirty-worktree guard reason.
// Callers outside this package use it to recognize the condition, since the guard-reason constants are unexported.
// Matching the reason text from outside would drift unnoticed,
// so this method keeps the match in the package that owns the constant.
func (e *MergeGuardError) WorktreeDirty() bool {
	return e.has(mergeReasonWorktreeDirty)
}

// AlreadyInProgress reports whether e's aggregated reasons include the merge-already-in-progress guard reason.
func (e *MergeGuardError) AlreadyInProgress() bool {
	return e.has(mergeReasonAlreadyInProgress)
}

// NotSynced reports whether e's aggregated reasons include the branch-not-synced-to-upstream guard reason.
func (e *MergeGuardError) NotSynced() bool {
	return e.has(mergeReasonNotSynced)
}

// DetachedHead reports whether e's aggregated reasons include the checkout-is-not-on-a-branch guard reason.
func (e *MergeGuardError) DetachedHead() bool {
	return e.has(mergeReasonDetachedHead)
}

// withDirt sets Checkout and DirtyPaths on e when paths is not empty, and returns e.
func (e *MergeGuardError) withDirt(checkout string, paths []string) *MergeGuardError {
	if len(paths) > 0 {
		e.Checkout = checkout
		e.DirtyPaths = paths
	}
	return e
}

// newMergeGuardError builds a *MergeGuardError from reasons, sorting and deduplicating them first
// so the reported list never reveals evaluation order or arity.
func newMergeGuardError(reasons []string) *MergeGuardError {
	seen := make(map[string]bool, len(reasons))
	unique := make([]string, 0, len(reasons))
	for _, r := range reasons {
		if seen[r] {
			continue
		}
		seen[r] = true
		unique = append(unique, r)
	}
	sort.Strings(unique)
	return &MergeGuardError{Reasons: unique}
}

// ErrMergeInRequired reports a merge that produced conflicts and was aborted on both sides.
// Conflicts are resolved in the source branch's own worktree.
// The caller runs `lym fabric merge-in` there first, then retries.
// Source names the offending branch, the one detail this error carries outside its fixed message.
//
// Errors that carry a remedy name the CLI command, not a Go method.
// An operator or an agent reads the message verbatim and cannot run a Go method.
type ErrMergeInRequired struct{ Source string }

// Error implements the error interface with a fixed message that does not include Source.
func (e *ErrMergeInRequired) Error() string {
	return `fabricengine: merge produced conflicts and was aborted; run "lym fabric merge-in" in the source branch's own worktree first, then retry`
}

// ErrForeignMergeState reports git merge state on either side that fabric did not start.
// Every mutating merge verb returns it, and the foreign state is left untouched.
type ErrForeignMergeState struct{}

// Error implements the error interface with a fixed message.
func (e *ErrForeignMergeState) Error() string {
	return "fabricengine: git merge state exists that fabric did not start; conclude or abort it with plain git, then retry"
}

// ErrNoMergeInProgress reports that neither a fabric merge record nor foreign git merge state exists on either side.
// The verbs that continue or abort a merge return it, mirroring git's own "There is no merge in progress".
type ErrNoMergeInProgress struct{}

// Error implements the error interface with a fixed message.
func (e *ErrNoMergeInProgress) Error() string {
	return "fabricengine: no merge in progress"
}

// ErrMergeIncomplete reports that the conclude phase did not land the commits of both sides.
// Running `lym fabric merge --continue` again finishes the merge.
type ErrMergeIncomplete struct{}

// Error implements the error interface with a fixed message.
func (e *ErrMergeIncomplete) Error() string {
	return `fabricengine: merge conclude did not finish; run "lym fabric merge --continue" again`
}

// ErrUnmergeableState reports a merge that produced conflicts outside the fabric-managed worktree tree.
// That is a conflicted path that cannot map to the single visible worktree.
// The merge is aborted on both sides, which restores the pair.
type ErrUnmergeableState struct{}

// Error implements the error interface with a fixed message.
// The offending detail goes to the internal log only.
func (e *ErrUnmergeableState) Error() string {
	return "fabricengine: merge produced conflicts outside the fabric-managed tree; operator intervention required"
}

// ErrMergeInProgress reports that a fabric merge record exists on the pair.
// Every other mutating verb returns it while the record exists.
type ErrMergeInProgress struct{}

// Error implements the error interface with a fixed message.
func (e *ErrMergeInProgress) Error() string {
	return `fabricengine: a merge is in progress; run "lym fabric merge --continue" or "lym fabric merge --abort" first`
}
