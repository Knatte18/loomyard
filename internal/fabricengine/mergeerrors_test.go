// mergeerrors_test.go asserts the pinned Error() strings, newMergeGuardError's sort/dedup
// behaviour, and that no closed-set constant or Error() output leaks warp/weft/host vocabulary
// across the merge error surface's public boundary.

package fabricengine

import (
	"strings"
	"testing"
)

func TestMergeErrors_PinnedStrings(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "MergeGuardError",
			err:  &MergeGuardError{Reasons: []string{"a", "b"}},
			want: "fabricengine: merge preconditions failed: a; b",
		},
		{
			name: "ErrMergeInRequired",
			err:  &ErrMergeInRequired{Source: "some-branch"},
			want: `fabricengine: merge produced conflicts and was aborted; run "lyx fabric merge-in" in the source branch's own worktree first, then retry`,
		},
		{
			name: "ErrForeignMergeState",
			err:  &ErrForeignMergeState{},
			want: "fabricengine: git merge state exists that fabric did not start; conclude or abort it with plain git, then retry",
		},
		{
			name: "ErrNoMergeInProgress",
			err:  &ErrNoMergeInProgress{},
			want: "fabricengine: no merge in progress",
		},
		{
			name: "ErrMergeIncomplete",
			err:  &ErrMergeIncomplete{},
			want: `fabricengine: merge conclude did not finish; run "lyx fabric merge --continue" again`,
		},
		{
			name: "ErrUnmergeableState",
			err:  &ErrUnmergeableState{},
			want: "fabricengine: merge produced conflicts outside the fabric-managed tree; operator intervention required",
		},
		{
			name: "ErrMergeInProgress",
			err:  &ErrMergeInProgress{},
			want: `fabricengine: a merge is in progress; run "lyx fabric merge --continue" or "lyx fabric merge --abort" first`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("%s.Error() = %q; want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestMergeErrors_ErrMergeInRequiredSourceNotInMessage(t *testing.T) {
	err := &ErrMergeInRequired{Source: "the-offending-branch"}
	if strings.Contains(err.Error(), "the-offending-branch") {
		t.Errorf("ErrMergeInRequired.Error() = %q; must not interpolate Source", err.Error())
	}
}

func TestMergeErrors_NewMergeGuardErrorSortsAndDeduplicates(t *testing.T) {
	tests := []struct {
		name    string
		reasons []string
		want    []string
	}{
		{
			name:    "unsorted",
			reasons: []string{mergeReasonWorktreeDirty, mergeReasonAlreadyInProgress},
			want:    []string{mergeReasonAlreadyInProgress, mergeReasonWorktreeDirty},
		},
		{
			name:    "duplicates",
			reasons: []string{mergeReasonNotSynced, mergeReasonNotSynced, mergeReasonSourceNotFound},
			want:    []string{mergeReasonNotSynced, mergeReasonSourceNotFound},
		},
		{
			name:    "empty",
			reasons: nil,
			want:    []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newMergeGuardError(tt.reasons)
			if len(got.Reasons) != len(tt.want) {
				t.Fatalf("newMergeGuardError(%v).Reasons = %v; want %v", tt.reasons, got.Reasons, tt.want)
			}
			for i := range tt.want {
				if got.Reasons[i] != tt.want[i] {
					t.Errorf("newMergeGuardError(%v).Reasons = %v; want %v", tt.reasons, got.Reasons, tt.want)
				}
			}
		})
	}
}

// mergeVocabularyLeakTokens are the case-insensitive tokens that must never appear in a
// fabric-crossing merge error string or closed-set reason: warp/weft naming is internal
// vocabulary, and any "host " phrase is a git-internal spelling neither belongs on the public
// error surface.
var mergeVocabularyLeakTokens = []string{"warp", "weft", "host "}

func TestMergeErrors_NoVocabularyLeakInReasons(t *testing.T) {
	// pinnedMergeReasons (mergevocab_test.go) is the whole closed set, proven equal to the real
	// const block by TestMergeVocabulary_GuardReasonSetMatchesConstBlock -- iterating it here means
	// a newly added member can never sit outside this leak check, the drift a hand-copied subset
	// already suffered once (two of nine members were missing).
	for name, reason := range pinnedMergeReasons {
		assertNoVocabularyLeak(t, name, reason)
	}
}

func TestMergeErrors_NoVocabularyLeakInErrorStrings(t *testing.T) {
	errs := map[string]error{
		"MergeGuardError":      &MergeGuardError{Reasons: []string{mergeReasonWorktreeDirty}},
		"ErrMergeInRequired":   &ErrMergeInRequired{Source: "warp-branch-name"},
		"ErrForeignMergeState": &ErrForeignMergeState{},
		"ErrNoMergeInProgress": &ErrNoMergeInProgress{},
		"ErrMergeIncomplete":   &ErrMergeIncomplete{},
		"ErrUnmergeableState":  &ErrUnmergeableState{},
		"ErrMergeInProgress":   &ErrMergeInProgress{},
	}
	for name, err := range errs {
		assertNoVocabularyLeak(t, name, err.Error())
	}
}

// assertNoVocabularyLeak fails the test if s contains any token in mergeVocabularyLeakTokens,
// case-insensitively.
func assertNoVocabularyLeak(t *testing.T, label, s string) {
	t.Helper()
	lower := strings.ToLower(s)
	for _, token := range mergeVocabularyLeakTokens {
		if strings.Contains(lower, token) {
			t.Errorf("%s = %q contains vocabulary-leak token %q", label, s, token)
		}
	}
}

// TestMergeGuardError_WorktreeDirty pins the accessor's coupling to the unexported
// mergeReasonWorktreeDirty constant: a guard error built with the dirty-worktree reason reports
// true, one built with any other single reason reports false, and one carrying the dirty reason
// alongside others still reports true. Placing this coverage here, rather than in the consumer
// (internal/landingshed), is what makes the cross-package coupling pinned rather than merely
// conventional -- this package's own tier asserts the accessor tracks the constant, and the
// consumer's own tier asserts it branches on the accessor.
func TestMergeGuardError_WorktreeDirty(t *testing.T) {
	tests := []struct {
		name    string
		reasons []string
		want    bool
	}{
		{
			name:    "dirty reason alone",
			reasons: []string{mergeReasonWorktreeDirty},
			want:    true,
		},
		{
			name:    "a different single reason",
			reasons: []string{mergeReasonAlreadyInProgress},
			want:    false,
		},
		{
			name:    "dirty reason alongside others",
			reasons: []string{mergeReasonAlreadyInProgress, mergeReasonWorktreeDirty, mergeReasonNotSynced},
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &MergeGuardError{Reasons: tt.reasons}
			if got := err.WorktreeDirty(); got != tt.want {
				t.Errorf("WorktreeDirty() = %v; want %v", got, tt.want)
			}
		})
	}
}
