// mergevocab_test.go pins that the merge surface stays side-free:
// the public result and options types, every named merge error's rendered message, and the closed guard-reason set.
// The enforcement walk `TestEnforcement_FabricVocabulary` in internal/lymcwd cannot cover this surface:
// internal/fabricengine sits inside that walk's owner set;
// a warp/weft leak into MergeResult, MergeOptions, or a named merge error would pass it silently.
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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// pinnedMergeReasons is the hand-pinned copy of the closed guard-reason set, mapping each constant name
// to its verbatim string.
// Every vocabulary and leak assertion over the set iterates this map.
// TestMergeVocabulary_GuardReasonSetMatchesConstBlock parses the const block in mergeerrors.go.
// It fails when this map and the source disagree, so changing a member needs a same-commit update here.
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

// mergeReasonHomeFile names the one file that declares the closed guard-reason set.
// The const-block godoc in mergeerrors.go says so as well.
// The const-block test compares each declaration's file against this name.
const mergeReasonHomeFile = "mergeerrors.go"

// mergeReasonConstDecl is one mergeReason* constant as the source declares it:
// its verbatim string value and the package file it was found in.
type mergeReasonConstDecl struct {
	value string
	file  string
}

// mergeReasonConstsFromSource returns every package-level constant whose name carries the mergeReason prefix.
// It maps each name to its declaration.
// It parses every non-test .go file in this package's own directory with go/ast.
// So the closed set is the set the source declares, and no hand-maintained list can drift from it.
//
// The scan is package-wide, not limited to one filename.
// A mergeReason* constant may be declared in any file, such as mergeguards.go beside the guard that consumes it.
// A scan of one file would leave every assertion the pinned map drives blind to that constant.
// The closed-set equality, the side-free check, and the path-free check would all miss it.
//
// Test files are excluded because the closed set is a production declaration.
// A test's own fixture constant is not a member of it.
func mergeReasonConstsFromSource(t *testing.T) map[string]mergeReasonConstDecl {
	t.Helper()

	got := map[string]mergeReasonConstDecl{}
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/fabricengine"}, Shallow: true}, func(f *scankit.File) {
		collectMergeReasonConsts(t, path.Base(f.Rel), f.AST(t, parser.SkipObjectResolution), got)
	})
	scankit.RequireFloor(t, scanned, mergeVocabScanMinFiles, "merge-vocabulary closed-set scan")
	return got
}

// mergeVocabScanMinFiles is a lower bound on the production .go files internal/fabricengine holds.
// Below it, the package-wide scan has read the wrong directory.
const mergeVocabScanMinFiles = 20

// collectMergeReasonConsts adds every package-level mergeReason* constant that file declares into got.
// It fails the test on a duplicate declaration.
// It also fails on a member whose value is not a plain string literal.
// The closed set must pin every member verbatim.
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

// TestMergeVocabulary_GuardReasonSetMatchesConstBlock proves pinnedMergeReasons equals the mergeReason* constants.
// The equality holds in both directions, by names and verbatim values.
// The constants are scanned across every production file, not one.
// A member added to the source without a matching change to pinnedMergeReasons fails here, wherever it was added.
// Every member must also live in mergeerrors.go, as that file's const-block godoc states, and this check enforces it.
// The scan reads the whole package, so a stray member cannot hide from the pinned-map equality.
func TestMergeVocabulary_GuardReasonSetMatchesConstBlock(t *testing.T) {
	tiergate.Require(t, 0)
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

// mergeVocabHostPhrases lists the fabric-sense "host X" phrases the vocabulary decision polices.
// It mirrors hostPhrases in internal/lymcwd/enforcement_test.go.
// Each phrase is checked case-insensitively, in spaced and hyphenated form.
// A bare "host" is never policed on its own; see mergeVocabContainsHostPhrase.
var mergeVocabHostPhrases = []string{
	"host repo", "host repository", "host worktree", "host working tree",
	"host checkout", "host branch", "host junction", "host path", "host side", "host head",
}

// mergeVocabContainsBareToken reports whether s contains the bare token "weft" or "warp" as a substring.
// The match is case-insensitive.
// Substring matching, not whole-word matching, is deliberate: fabric has no other meaning for either token in this repo.
// The token set mirrors BareToken in internal/fabricvocab.
func mergeVocabContainsBareToken(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "weft") || strings.Contains(lower, "warp")
}

// mergeVocabContainsHostPhrase reports whether s contains a fabric-sense "host" phrase.
// The match is case-insensitive and covers the spaced and hyphenated forms.
// It mirrors fabricSenseHostPhrase in internal/lymcwd/enforcement_test.go.
func mergeVocabContainsHostPhrase(s string) bool {
	lower := strings.ToLower(s)
	for _, phrase := range mergeVocabHostPhrases {
		if strings.Contains(lower, phrase) || strings.Contains(lower, strings.ReplaceAll(phrase, " ", "-")) {
			return true
		}
	}
	return false
}

// assertSideFree fails the test if s contains a bare weft/warp token or a fabric-sense host phrase.
// The label names s in the failure message.
func assertSideFree(t *testing.T, label, s string) {
	t.Helper()
	if mergeVocabContainsBareToken(s) {
		t.Errorf("%s = %q; contains a bare weft/warp token, which the merge surface must never expose", label, s)
	}
	if mergeVocabContainsHostPhrase(s) {
		t.Errorf("%s = %q; contains a fabric-sense host phrase, which the merge surface must never expose", label, s)
	}
}

// assertStructFieldsSideFree asserts that each exported field name and JSON tag of the struct value v is side-free.
// v must be a struct value, not a pointer.
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

// TestMergeVocabulary_ResultAndOptionsFieldsAreSideFree checks MergeResult and MergeOptions.
// Their exported field names and JSON tags must contain no warp/weft token and no fabric-sense host phrase.
// The enforcement walk permits warp/weft tokens inside fabricengine's own owner set.
// So this is the one check that catches a leak onto the public merge surface.
//
//testtiming:keep every exported field name and JSON tag of MergeResult and MergeOptions being free of warp/weft and host phrases; coverage of its blocks by other tests does not show an assertion of this
//lym:seam internal/fabricengine#MergeResult
//lym:seam internal/fabricengine#MergeOptions
func TestMergeVocabulary_ResultAndOptionsFieldsAreSideFree(t *testing.T) {
	tiergate.Require(t, 0)
	assertStructFieldsSideFree(t, MergeResult{})
	assertStructFieldsSideFree(t, MergeOptions{})
}

// TestMergeVocabulary_ErrorsAreSideFree instantiates every named merge error.
// It checks that each Error() output is side-free by the same token check.
// It also checks that the message of ErrMergeInRequired does not contain its Source field's value.
// Source is the one detail that error carries outside its fixed message, and it must never leak into the message.
//
//testtiming:keep every named merge error rendering side-free and ErrMergeInRequired never echoing its Source; coverage of its blocks by other tests does not show an assertion of this
func TestMergeVocabulary_ErrorsAreSideFree(t *testing.T) {
	tiergate.Require(t, 0)
	reasons := make([]string, 0, len(pinnedMergeReasons))
	for _, reason := range pinnedMergeReasons {
		reasons = append(reasons, reason)
	}
	guardErr := newMergeGuardError(reasons).withDirt("/hub/parent", []string{"a b.go"})
	assertSideFree(t, "(*MergeGuardError).Error()", guardErr.Error())
	for _, detail := range []string{"/hub/parent", "a b.go"} {
		if strings.Contains(guardErr.Error(), detail) {
			t.Errorf("(*MergeGuardError).Error() = %q; must not contain the dirt detail %q", guardErr.Error(), detail)
		}
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

// TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree asserts every member of pinnedMergeReasons.
// The closed guard-reason set must be side-free, including the bare "host " token, and path-free, with no "/" or "\".
// The set is pinned to the const block by parsing the source.
// So a member added to it cannot escape these assertions.
//
//testtiming:keep every member of the closed guard-reason set being side-free and path-free; coverage of its blocks by other tests does not show an assertion of this
func TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree(t *testing.T) {
	tiergate.Require(t, 0)
	for name, reason := range pinnedMergeReasons {
		assertSideFree(t, "guard reason "+name, reason)
		assertNoVocabularyLeak(t, name, reason)
		if strings.ContainsAny(reason, `/\`) {
			t.Errorf("guard reason %s = %q contains a path separator; the closed set must be path-free", name, reason)
		}
	}
}
