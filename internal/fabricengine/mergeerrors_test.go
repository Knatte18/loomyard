// mergeerrors_test.go asserts the pinned Error() strings and the sort and dedup of newMergeGuardError.
// Error output and closed-set constants on the merge error surface must not leak warp/weft/host vocabulary.

package fabricengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

//testtiming:keep the exact Error() string of each named merge error and of MergeGuardError; coverage of its blocks by other tests does not show an assertion of this
func TestMergeErrors_PinnedStrings(t *testing.T) {
	tiergate.Require(t, 0)
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
			want: `fabricengine: merge produced conflicts and was aborted; run "lym fabric merge-in" in the source branch's own worktree first, then retry`,
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
			want: `fabricengine: merge conclude did not finish; run "lym fabric merge --continue" again`,
		},
		{
			name: "ErrUnmergeableState",
			err:  &ErrUnmergeableState{},
			want: "fabricengine: merge produced conflicts outside the fabric-managed tree; operator intervention required",
		},
		{
			name: "ErrMergeInProgress",
			err:  &ErrMergeInProgress{},
			want: `fabricengine: a merge is in progress; run "lym fabric merge --continue" or "lym fabric merge --abort" first`,
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

func TestMergeErrors_NewMergeGuardErrorSortsAndDeduplicates(t *testing.T) {
	tiergate.Require(t, 0)
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

// mergeVocabularyLeakTokens lists the case-insensitive tokens banned from fabric-crossing merge errors and reasons.
// Warp/weft naming is internal vocabulary, and a "host " phrase is a git-internal spelling;
// neither belongs on the public error surface.
var mergeVocabularyLeakTokens = []string{"warp", "weft", "host "}

// assertNoVocabularyLeak fails the test if s contains any mergeVocabularyLeakTokens token, case-insensitively.
func assertNoVocabularyLeak(t *testing.T, label, s string) {
	t.Helper()
	lower := strings.ToLower(s)
	for _, token := range mergeVocabularyLeakTokens {
		if strings.Contains(lower, token) {
			t.Errorf("%s = %q contains vocabulary-leak token %q", label, s, token)
		}
	}
}

// TestMergeGuardError_WorktreeDirty pins the accessor's coupling to the unexported mergeReasonWorktreeDirty constant.
// A guard error with the dirty-worktree reason reports true; one with any other single reason reports false.
// One carrying the dirty reason alongside others still reports true.
// The coverage lives here rather than in the consumer (internal/landingshed),
// so the coupling is pinned rather than merely conventional.
// This package's tier asserts the accessor tracks the constant;
// the consumer's tier asserts it branches on the accessor.
func TestMergeGuardError_WorktreeDirty(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name       string
		reasons    []string
		dirty      bool
		inProgress bool
		notSynced  bool
		detached   bool
	}{
		{
			name:    "dirty reason alone",
			reasons: []string{mergeReasonWorktreeDirty},
			dirty:   true,
		},
		{
			name:       "a different single reason",
			reasons:    []string{mergeReasonAlreadyInProgress},
			inProgress: true,
		},
		{
			name:      "not-synced reason alone",
			reasons:   []string{mergeReasonNotSynced},
			notSynced: true,
		},
		{
			name:     "detached reason alone",
			reasons:  []string{mergeReasonDetachedHead},
			detached: true,
		},
		{
			name:       "dirty reason alongside others",
			reasons:    []string{mergeReasonAlreadyInProgress, mergeReasonWorktreeDirty, mergeReasonNotSynced},
			dirty:      true,
			inProgress: true,
			notSynced:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &MergeGuardError{Reasons: tt.reasons}
			if got := err.WorktreeDirty(); got != tt.dirty {
				t.Errorf("WorktreeDirty() = %v; want %v", got, tt.dirty)
			}
			if got := err.AlreadyInProgress(); got != tt.inProgress {
				t.Errorf("AlreadyInProgress() = %v; want %v", got, tt.inProgress)
			}
			if got := err.NotSynced(); got != tt.notSynced {
				t.Errorf("NotSynced() = %v; want %v", got, tt.notSynced)
			}
			if got := err.DetachedHead(); got != tt.detached {
				t.Errorf("DetachedHead() = %v; want %v", got, tt.detached)
			}
		})
	}
}
