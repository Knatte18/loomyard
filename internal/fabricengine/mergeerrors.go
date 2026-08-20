// mergeerrors.go declares the merge-verb typed error surface and the closed guard-reason set.
// Every message is fixed and side-free: none interpolates a branch name, path, or git error, and
// where a caller needs the offending branch it travels in a typed error's own field
// (ErrMergeInRequired.Source), never inside an Error() string.
// The closed reason set backing MergeGuardError is pinned verbatim from the discussion's
// safety-guards-are-aggregated-and-side-free decision; adding a member is a same-commit change to
// the constant list and to the vocabulary assertion covering it (mergevocab_test.go, batch 5), and
// no member may name a side, carry a path, or imply an order.

package fabricengine

import (
	"sort"
	"strings"
)

// The closed set of guard-reason strings a *MergeGuardError may carry.
// Pinned verbatim from the discussion's safety-guards-are-aggregated-and-side-free decision:
// adding a member is a same-commit change to this list and to the vocabulary assertion that covers
// it, and no member may name a side, carry a path, or imply an order.
const (
	mergeReasonAlreadyInProgress   = "merge already in progress"
	mergeReasonUnresolvedConflicts = "unresolved conflicts remain"
	mergeReasonNoMergeInProgress   = "no merge in progress"
	mergeReasonWorktreeDirty       = "worktree dirty"
	mergeReasonNotSynced           = "branch not synced to upstream"
	mergeReasonSourceNotFound      = "source branch not found"
	mergeReasonNotFabricManaged    = "source branch is not fabric-managed"
)

// MergeGuardError aggregates every failed merge precondition as a sorted, deduplicated list of
// fixed reason strings drawn from the closed set above.
// The list is aggregated rather than first-failure so two scenarios differing only in which
// precondition failed on which side produce byte-identical errors — no failure ordering ever
// discloses that two subjects were checked.
type MergeGuardError struct{ Reasons []string }

// Error implements the error interface, joining the aggregated reasons with "; ".
func (e *MergeGuardError) Error() string {
	return "fabricengine: merge preconditions failed: " + strings.Join(e.Reasons, "; ")
}

// WorktreeDirty reports whether e's aggregated reasons include the dirty-worktree guard reason.
// It is the supported way for a caller outside this package to recognize the condition: the
// guard-reason constants themselves are a closed, unexported set, pinned verbatim by this file's
// own comment and by two of this package's tests, so an outside caller has no literal to match
// against and would otherwise have to hardcode the reason text -- with nothing to catch that text
// drifting later. WorktreeDirty keeps the coupling inside the package that owns the constant, where
// the existing pinning tests already cover it.
func (e *MergeGuardError) WorktreeDirty() bool {
	for _, r := range e.Reasons {
		if r == mergeReasonWorktreeDirty {
			return true
		}
	}
	return false
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

// ErrMergeInRequired is returned by Merge when the merge attempt produced conflicts and Merge
// self-aborted both sides: conflict resolution belongs in the source pair's own worktree, so the
// caller must run MergeIn there first, then retry.
// Source names the offending branch — the one detail this error carries outside its fixed message.
type ErrMergeInRequired struct{ Source string }

// Error implements the error interface with a fixed, side-free message; Source travels only in the
// struct field, never interpolated into the string.
func (e *ErrMergeInRequired) Error() string {
	return "fabricengine: merge produced conflicts and was aborted; run MergeIn in the source branch's worktree first, then retry"
}

// ErrForeignMergeState is returned by every mutating merge verb when git-level merge state exists
// on either side that fabric did not itself start — the foreign state is left untouched.
type ErrForeignMergeState struct{}

// Error implements the error interface with a fixed message.
func (e *ErrForeignMergeState) Error() string {
	return "fabricengine: git merge state exists that fabric did not start; conclude or abort it with plain git, then retry"
}

// ErrNoMergeInProgress is returned by MergeContinue/MergeAbort when neither a fabric merge record
// nor foreign git merge state exists on either side — mirroring git's own "There is no merge in
// progress".
type ErrNoMergeInProgress struct{}

// Error implements the error interface with a fixed message.
func (e *ErrNoMergeInProgress) Error() string {
	return "fabricengine: no merge in progress"
}

// ErrMergeIncomplete is returned when the conclude phase did not finish landing both sides' commits;
// the caller re-runs MergeContinue to finish it.
type ErrMergeIncomplete struct{}

// Error implements the error interface with a fixed message.
func (e *ErrMergeIncomplete) Error() string {
	return "fabricengine: merge conclude did not finish; run MergeContinue again"
}

// ErrUnmergeableState is returned when a merge produced conflicts outside the fabric-managed
// worktree tree — a path unifyConflictPaths could not map to the single visible worktree — and the
// merge was aborted on both sides, restoring the pair.
type ErrUnmergeableState struct{}

// Error implements the error interface with a fixed message; the offending detail goes to the
// internal log only.
func (e *ErrUnmergeableState) Error() string {
	return "fabricengine: merge produced conflicts outside the fabric-managed tree; operator intervention required"
}

// ErrMergeInProgress is the single typed error every sibling mutating verb returns while a fabric
// merge record exists on the pair — the lock decision's one typed, side-free sibling-refusal error.
type ErrMergeInProgress struct{}

// Error implements the error interface with a fixed message.
func (e *ErrMergeInProgress) Error() string {
	return "fabricengine: a merge is in progress; run MergeContinue or MergeAbort first"
}
