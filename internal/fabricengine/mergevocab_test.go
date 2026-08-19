// mergevocab_test.go is the explicit side-free vocabulary assertion the enforcement walk
// (internal/lyxcwd's TestEnforcement_FabricVocabulary) cannot provide for the merge surface:
// internal/fabricengine sits inside that walk's owner set, so a warp/weft leak into MergeResult,
// MergeOptions, or any named merge error would pass the enforcement walk silently. This file pins
// the three things the vocabulary decision requires stay side-free on the public merge surface: the
// two public result/options types, every named merge error's rendered message, and the closed
// guard-reason set itself.
//
// package fabricengine (in-package), not fabricengine_test, since the closed guard-reason constants
// (mergeReason*) are unexported.

package fabricengine

import (
	"reflect"
	"strings"
	"testing"
)

// mergeVocabHostPhrases mirrors internal/lyxcwd/enforcement_test.go's hostPhrases list: the
// fabric-sense "host X" phrases the vocabulary decision polices, checked case-insensitively in both
// spaced and hyphenated form. A bare "host" is never policed on its own -- see
// mergeVocabContainsHostPhrase.
var mergeVocabHostPhrases = []string{
	"host repo", "host repository", "host worktree", "host working tree",
	"host checkout", "host branch", "host junction", "host path", "host side", "host head",
}

// mergeVocabContainsBareToken reports whether s contains, case-insensitively, the bare token "weft"
// or "warp" anywhere as a substring -- fabric has no other meaning for either token in this repo, so
// substring matching (not whole-word matching) is deliberate, mirroring
// internal/lyxcwd/enforcement_test.go's bareVocabularyToken.
func mergeVocabContainsBareToken(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "weft") || strings.Contains(lower, "warp")
}

// mergeVocabContainsHostPhrase reports whether s contains a fabric-sense "host" phrase,
// case-insensitively, in either spaced or hyphenated form, mirroring
// internal/lyxcwd/enforcement_test.go's fabricSenseHostPhrase.
func mergeVocabContainsHostPhrase(s string) bool {
	lower := strings.ToLower(s)
	for _, phrase := range mergeVocabHostPhrases {
		if strings.Contains(lower, phrase) || strings.Contains(lower, strings.ReplaceAll(phrase, " ", "-")) {
			return true
		}
	}
	return false
}

// assertSideFree fails the test if s contains a bare weft/warp token or a fabric-sense host phrase,
// naming what and label for a useful failure message.
func assertSideFree(t *testing.T, label, s string) {
	t.Helper()
	if mergeVocabContainsBareToken(s) {
		t.Errorf("%s = %q; contains a bare weft/warp token, which the merge surface must never expose", label, s)
	}
	if mergeVocabContainsHostPhrase(s) {
		t.Errorf("%s = %q; contains a fabric-sense host phrase, which the merge surface must never expose", label, s)
	}
}

// assertStructFieldsSideFree reflects over v's exported field names and JSON tags, asserting each is
// side-free. v must be a struct value (not a pointer).
func assertStructFieldsSideFree(t *testing.T, v any) {
	t.Helper()

	typ := reflect.TypeOf(v)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		assertSideFree(t, typ.Name()+"."+field.Name+" field name", field.Name)
		if tag, ok := field.Tag.Lookup("json"); ok {
			assertSideFree(t, typ.Name()+"."+field.Name+" json tag", tag)
		}
	}
}

// TestMergeVocabulary_ResultAndOptionsFieldsAreSideFree reflects over MergeResult and MergeOptions:
// every exported field name and every JSON tag must contain no warp/weft token and no fabric-sense
// host phrase -- the enforcement walk permits warp/weft tokens inside fabricengine's own owner set,
// so this is the one place that still catches a leak onto the public merge surface.
func TestMergeVocabulary_ResultAndOptionsFieldsAreSideFree(t *testing.T) {
	assertStructFieldsSideFree(t, MergeResult{})
	assertStructFieldsSideFree(t, MergeOptions{})
}

// TestMergeVocabulary_ErrorsAreSideFree instantiates every named merge error and asserts its Error()
// output is side-free by the same token check, and that ErrMergeInRequired's message does not
// contain its own Source field's value -- the one detail that error carries outside its fixed
// message, and which must never leak into the message itself.
func TestMergeVocabulary_ErrorsAreSideFree(t *testing.T) {
	reasons := []string{
		mergeReasonAlreadyInProgress,
		mergeReasonUnresolvedConflicts,
		mergeReasonNoMergeInProgress,
		mergeReasonWorktreeDirty,
		mergeReasonNotSynced,
		mergeReasonSourceNotFound,
		mergeReasonNotFabricManaged,
		mergeReasonDetachedHead,
		mergeReasonAttemptIncomplete,
	}
	guardErr := newMergeGuardError(reasons)
	assertSideFree(t, "(*MergeGuardError).Error()", guardErr.Error())

	const mergeInRequiredSource = "some-warp-branch"
	mergeInRequiredErr := &ErrMergeInRequired{Source: mergeInRequiredSource}
	assertSideFree(t, "(*ErrMergeInRequired).Error()", mergeInRequiredErr.Error())
	if strings.Contains(mergeInRequiredErr.Error(), mergeInRequiredSource) {
		t.Errorf("(*ErrMergeInRequired).Error() = %q; must not contain its own Source value %q", mergeInRequiredErr.Error(), mergeInRequiredSource)
	}

	assertSideFree(t, "(*ErrForeignMergeState).Error()", (&ErrForeignMergeState{}).Error())
	assertSideFree(t, "(*ErrNoMergeInProgress).Error()", (&ErrNoMergeInProgress{}).Error())
	assertSideFree(t, "(*ErrMergeIncomplete).Error()", (&ErrMergeIncomplete{}).Error())
	assertSideFree(t, "(*ErrUnmergeableState).Error()", (&ErrUnmergeableState{}).Error())
	assertSideFree(t, "(*ErrMergeInProgress).Error()", (&ErrMergeInProgress{}).Error())
}

// TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree asserts every member of the closed
// guard-reason set is side-free, path-free (no "/" or "\"), and matches the pinned literal list
// verbatim -- so adding a member without updating this test fails loudly, per the guards decision's
// same-commit rule.
func TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree(t *testing.T) {
	want := []string{
		"merge already in progress",
		"unresolved conflicts remain",
		"no merge in progress",
		"worktree dirty",
		"branch not synced to upstream",
		"source branch not found",
		"source branch is not fabric-managed",
		"checkout is not on a branch",
		"merge attempt did not reach both sides",
	}
	got := []string{
		mergeReasonAlreadyInProgress,
		mergeReasonUnresolvedConflicts,
		mergeReasonNoMergeInProgress,
		mergeReasonWorktreeDirty,
		mergeReasonNotSynced,
		mergeReasonSourceNotFound,
		mergeReasonNotFabricManaged,
		mergeReasonDetachedHead,
		mergeReasonAttemptIncomplete,
	}
	if len(got) != len(want) {
		t.Fatalf("closed guard-reason set has %d members; want exactly %d -- update this test's pinned list in the same commit as any change to the set", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("guard-reason set member[%d] = %q; want %q verbatim", i, got[i], want[i])
		}
	}

	for _, reason := range got {
		assertSideFree(t, "guard reason "+reason, reason)
		if strings.ContainsAny(reason, `/\`) {
			t.Errorf("guard reason %q contains a path separator; the closed set must be path-free", reason)
		}
	}
}
