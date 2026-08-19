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
			want: "fabricengine: merge produced conflicts and was aborted; run MergeIn in the source branch's worktree first, then retry",
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
			want: "fabricengine: merge conclude did not finish; run MergeContinue again",
		},
		{
			name: "ErrUnmergeableState",
			err:  &ErrUnmergeableState{},
			want: "fabricengine: merge produced conflicts outside the fabric-managed tree; operator intervention required",
		},
		{
			name: "ErrMergeInProgress",
			err:  &ErrMergeInProgress{},
			want: "fabricengine: a merge is in progress; run MergeContinue or MergeAbort first",
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
	reasons := []string{
		mergeReasonAlreadyInProgress,
		mergeReasonUnresolvedConflicts,
		mergeReasonWorktreeDirty,
		mergeReasonNotSynced,
		mergeReasonSourceNotFound,
		mergeReasonNotFabricManaged,
	}
	for _, reason := range reasons {
		assertNoVocabularyLeak(t, reason, reason)
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
