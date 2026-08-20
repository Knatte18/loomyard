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
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// pinnedMergeReasons is the single hand-pinned copy of the closed guard-reason set, constant name
// to verbatim string: every vocabulary and leak assertion over the set iterates this map, and
// TestMergeVocabulary_GuardReasonSetMatchesConstBlock proves it equal to the real const block in
// mergeerrors.go by parsing the source — so adding, removing, or rewording a member without
// updating this map in the same commit fails that test, making the guards decision's same-commit
// rule mechanically real rather than asserted.
var pinnedMergeReasons = map[string]string{
	"mergeReasonAlreadyInProgress":   "merge already in progress",
	"mergeReasonUnresolvedConflicts": "unresolved conflicts remain",
	"mergeReasonWorktreeDirty":       "worktree dirty",
	"mergeReasonNotSynced":           "branch not synced to upstream",
	"mergeReasonSourceNotFound":      "source branch not found",
	"mergeReasonNotFabricManaged":    "source branch is not fabric-managed",
	"mergeReasonDetachedHead":        "checkout is not on a branch",
	"mergeReasonAttemptIncomplete":   "merge attempt did not reach both sides",
	"mergeReasonConcludeLanded":      "merge conclude already landed",
}

// mergeReasonConstsFromSource parses mergeerrors.go and returns every package-level constant whose
// name carries the mergeReason prefix, name to string value — the closed set as the source actually
// declares it, read with go/ast so no hand-maintained list can drift from it (the
// cmd/lyx/registration_test.go precedent applied to a const block).
func mergeReasonConstsFromSource(t *testing.T) map[string]string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "mergeerrors.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse mergeerrors.go: %v", err)
	}

	got := map[string]string{}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if !strings.HasPrefix(name.Name, "mergeReason") {
					continue
				}
				if i >= len(valueSpec.Values) {
					t.Fatalf("const %s has no value literal; the closed set must pin every member verbatim", name.Name)
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("const %s is not a string literal; the closed set must pin every member verbatim", name.Name)
				}
				value, unquoteErr := strconv.Unquote(lit.Value)
				if unquoteErr != nil {
					t.Fatalf("unquote const %s value %s: %v", name.Name, lit.Value, unquoteErr)
				}
				got[name.Name] = value
			}
		}
	}
	return got
}

// TestMergeVocabulary_GuardReasonSetMatchesConstBlock proves pinnedMergeReasons equal — both
// directions, names and verbatim values — to the mergeReason* const block mergeerrors.go really
// declares. This is what makes the same-commit rule detectable: a member added to the source
// without touching pinnedMergeReasons fails here, which the closure test's former
// two-hand-maintained-lists comparison could never do.
func TestMergeVocabulary_GuardReasonSetMatchesConstBlock(t *testing.T) {
	got := mergeReasonConstsFromSource(t)

	for name, value := range got {
		pinnedValue, pinned := pinnedMergeReasons[name]
		if !pinned {
			t.Errorf("mergeerrors.go declares %s = %q, which pinnedMergeReasons does not pin -- update the pinned map in the same commit as any change to the closed set", name, value)
			continue
		}
		if value != pinnedValue {
			t.Errorf("mergeerrors.go declares %s = %q; pinnedMergeReasons pins %q -- the two must match verbatim", name, value, pinnedValue)
		}
	}
	for name := range pinnedMergeReasons {
		if _, declared := got[name]; !declared {
			t.Errorf("pinnedMergeReasons pins %s, which mergeerrors.go no longer declares -- update the pinned map in the same commit as any change to the closed set", name)
		}
	}
}

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
	reasons := make([]string, 0, len(pinnedMergeReasons))
	for _, reason := range pinnedMergeReasons {
		reasons = append(reasons, reason)
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
// guard-reason set is side-free and path-free (no "/" or "\"), iterating pinnedMergeReasons --
// whose equality with the real const block TestMergeVocabulary_GuardReasonSetMatchesConstBlock
// proves by parsing the source, so a member added to the set cannot escape these assertions.
func TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree(t *testing.T) {
	for name, reason := range pinnedMergeReasons {
		assertSideFree(t, "guard reason "+name, reason)
		if strings.ContainsAny(reason, `/\`) {
			t.Errorf("guard reason %s = %q contains a path separator; the closed set must be path-free", name, reason)
		}
	}
}
