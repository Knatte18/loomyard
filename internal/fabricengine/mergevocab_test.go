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
	"path"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
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
	"mergeReasonRecordedMergeGone":   "checkout no longer carries the recorded merge",
}

// mergeReasonHomeFile is the one file the closed guard-reason set is declared in, per
// mergeerrors.go's own const-block godoc. mergeReasonConstsFromSource scans the WHOLE package for
// mergeReason* declarations and reports each one's file, so this name is what
// MatchesConstBlock compares against rather than a scope the
// scan itself silently imposes.
const mergeReasonHomeFile = "mergeerrors.go"

// mergeReasonConstDecl is one mergeReason* constant as the source declares it: its verbatim string
// value and the package file it was found in.
type mergeReasonConstDecl struct {
	value string
	file  string
}

// mergeReasonConstsFromSource parses every non-test .go file in this package's own directory and
// returns every package-level constant whose name carries the mergeReason prefix, name to
// declaration — the closed set as the source actually declares it, read with go/ast so no
// hand-maintained list can drift from it (the cmd/lyx/registration_test.go precedent applied to a
// const block).
//
// The scan is package-wide rather than a hardcoded filename on purpose. Scoping it to
// mergeerrors.go made every assertion the pinned map drives — the closed-set equality, the
// side-free check, the path-free check — blind to a mergeReason* constant declared anywhere else in
// the package, and mergeguards.go is the natural place for one to appear, right beside the guard
// that would consume it. A closure test that cannot see outside one file cannot detect the
// violation it exists to detect.
//
// Test files are excluded because the closed set is a production declaration; a test's own fixture
// constant is not a member of it.
func mergeReasonConstsFromSource(t *testing.T) map[string]mergeReasonConstDecl {
	t.Helper()

	got := map[string]mergeReasonConstDecl{}
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/fabricengine"}, Shallow: true}, func(f *scankit.File) {
		collectMergeReasonConsts(t, path.Base(f.Rel), f.AST(t, parser.SkipObjectResolution), got)
	})
	scankit.RequireFloor(t, scanned, mergeVocabScanMinFiles, "merge-vocabulary closed-set scan")
	return got
}

// mergeVocabScanMinFiles is the plausible floor for how many production .go files internal/fabricengine holds;
// below it the package-wide scan has read the wrong directory.
const mergeVocabScanMinFiles = 20

// collectMergeReasonConsts adds every package-level mergeReason* constant file declares into got, failing the test on a duplicate declaration or on a member whose value is not a plain string literal (the closed set must pin every member verbatim).
func collectMergeReasonConsts(t *testing.T, fileName string, file *ast.File, got map[string]mergeReasonConstDecl) {
	t.Helper()

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
					t.Fatalf("const %s in %s has no value literal; the closed set must pin every member verbatim", name.Name, fileName)
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("const %s in %s is not a string literal; the closed set must pin every member verbatim", name.Name, fileName)
				}
				value, unquoteErr := strconv.Unquote(lit.Value)
				if unquoteErr != nil {
					t.Fatalf("unquote const %s value %s in %s: %v", name.Name, lit.Value, fileName, unquoteErr)
				}
				if prior, duplicate := got[name.Name]; duplicate {
					t.Fatalf("const %s is declared in both %s and %s; the closed set must declare each member once", name.Name, prior.file, fileName)
				}
				got[name.Name] = mergeReasonConstDecl{value: value, file: fileName}
			}
		}
	}
}

// TestMergeVocabulary_GuardReasonSetMatchesConstBlock proves pinnedMergeReasons equal — both
// directions, names and verbatim values — to the mergeReason* constants the package really
// declares, scanned across every production file rather than one. This is what makes the
// same-commit rule detectable: a member added to the source without touching pinnedMergeReasons
// fails here, wherever in the package it was added.
// Every member must also live in mergeerrors.go, which is what that file's own const-block godoc
// claims and what the closed set's same-commit rule assumes when it names one const list to
// update; the scan deliberately reads the whole package so a stray member cannot hide from the
// pinned-map equality, and this check turns "declared in mergeerrors.go" from an unenforced
// convention into a checked one.
func TestMergeVocabulary_GuardReasonSetMatchesConstBlock(t *testing.T) {
	got := mergeReasonConstsFromSource(t)

	for name, decl := range got {
		if decl.file != mergeReasonHomeFile {
			t.Errorf("const %s is declared in %s; every mergeReason* member must be declared in %s, beside the rest of the closed set", name, decl.file, mergeReasonHomeFile)
		}
		pinnedValue, pinned := pinnedMergeReasons[name]
		if !pinned {
			t.Errorf("%s declares %s = %q, which pinnedMergeReasons does not pin -- update the pinned map in the same commit as any change to the closed set", decl.file, name, decl.value)
			continue
		}
		if decl.value != pinnedValue {
			t.Errorf("%s declares %s = %q; pinnedMergeReasons pins %q -- the two must match verbatim", decl.file, name, decl.value, pinnedValue)
		}
	}
	for name := range pinnedMergeReasons {
		if _, declared := got[name]; !declared {
			t.Errorf("pinnedMergeReasons pins %s, which no production file in this package declares -- update the pinned map in the same commit as any change to the closed set", name)
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
//
//testtiming:keep every exported field name and JSON tag of MergeResult and MergeOptions being free of warp/weft and host phrases; coverage of its blocks by other tests does not show an assertion of this
func TestMergeVocabulary_ResultAndOptionsFieldsAreSideFree(t *testing.T) {
	assertStructFieldsSideFree(t, MergeResult{})
	assertStructFieldsSideFree(t, MergeOptions{})
}

// TestMergeVocabulary_ErrorsAreSideFree instantiates every named merge error and asserts its Error()
// output is side-free by the same token check, and that ErrMergeInRequired's message does not
// contain its own Source field's value -- the one detail that error carries outside its fixed
// message, and which must never leak into the message itself.
//
//testtiming:keep every named merge error rendering side-free and ErrMergeInRequired never echoing its Source; coverage of its blocks by other tests does not show an assertion of this
func TestMergeVocabulary_ErrorsAreSideFree(t *testing.T) {
	reasons := make([]string, 0, len(pinnedMergeReasons))
	for _, reason := range pinnedMergeReasons {
		reasons = append(reasons, reason)
	}
	guardErr := newMergeGuardError(reasons)
	assertSideFree(t, "(*MergeGuardError).Error()", guardErr.Error())
	if !IsWorktreeDirtyReason(guardErr.Error()) {
		t.Errorf("IsWorktreeDirtyReason(%q) = false; want true for a guard error carrying the dirty-worktree reason", guardErr.Error())
	}
	if other := newMergeGuardError([]string{mergeReasonNotSynced}).Error(); IsWorktreeDirtyReason(other) {
		t.Errorf("IsWorktreeDirtyReason(%q) = true; want false for another guard reason", other)
	}

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
// guard-reason set is side-free (including the bare "host " token) and path-free (no "/" or "\"), iterating pinnedMergeReasons --
// whose equality with the real const block TestMergeVocabulary_GuardReasonSetMatchesConstBlock
// proves by parsing the source, so a member added to the set cannot escape these assertions.
//
//testtiming:keep every member of the closed guard-reason set being side-free and path-free; coverage of its blocks by other tests does not show an assertion of this
func TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree(t *testing.T) {
	for name, reason := range pinnedMergeReasons {
		assertSideFree(t, "guard reason "+name, reason)
		assertNoVocabularyLeak(t, name, reason)
		if strings.ContainsAny(reason, `/\`) {
			t.Errorf("guard reason %s = %q contains a path separator; the closed set must be path-free", name, reason)
		}
	}
}
