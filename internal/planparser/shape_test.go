// shape_test.go is the Ref-Shape Registry's own self-enforcement suite: it proves the registry
// stays in sync with classify.go's refKind enum (TestRefKindEnumMatchesAllRefKinds), that every
// ledger policy covers the whole kind domain (TestLedgerCompleteness), and that the lookup path
// fails closed on an undeclared disposition rather than silently skipping the ref
// (TestLookupFailsClosedOnUndeclaredKind). The AST-parsing test follows the idiom of
// internal/cliwire/bannedecl_enforcement_test.go: stdlib go/parser only, repo root resolved from
// runtime.Caller(0), production files only (this file itself is a _test.go file and is never
// parsed as a scan target).

package planparser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

// refKindConstNamesInDeclarationOrder parses internal/planparser/classify.go and returns the
// identifiers declared in its refKind-typed const block, in declaration order -- the same order
// that fixes each identifier's iota-derived refKind value.
func refKindConstNamesInDeclarationOrder(t *testing.T) []string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine planparser source directory location")
	}
	classifyPath := filepath.Join(filepath.Dir(thisFile), "classify.go")

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, classifyPath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", classifyPath, err)
	}

	var names []string
	for _, decl := range astFile.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		// A refKind const block's first ValueSpec carries the explicit "refKind = iota" type;
		// subsequent ValueSpecs in the same block inherit it implicitly (no repeated Type). Only
		// the block whose FIRST spec names refKind is the one this test cares about.
		if len(gd.Specs) == 0 {
			continue
		}
		first, ok := gd.Specs[0].(*ast.ValueSpec)
		if !ok || first.Type == nil {
			continue
		}
		ident, ok := first.Type.(*ast.Ident)
		if !ok || ident.Name != "refKind" {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, n := range vs.Names {
				names = append(names, n.Name)
			}
		}
	}

	return names
}

// TestRefKindEnumMatchesAllRefKinds is the enum<->slice sync meta-test: it asserts set equality,
// not mere cardinality, between the rendered names of classify.go's refKind const block (bridged
// to refKind values by iota position) and the rendered names of allRefKinds' own members. Building
// map[string]int counts on both sides -- rather than sorting and comparing -- additionally catches
// a duplicate-plus-omission hand edit of allRefKinds that would otherwise still pass at matching
// length, and a fifth enum constant with no allRefKinds counterpart fails immediately.
func TestRefKindEnumMatchesAllRefKinds(t *testing.T) {
	t.Parallel()

	constNames := refKindConstNamesInDeclarationOrder(t)
	if len(constNames) == 0 {
		t.Fatal("refKindConstNamesInDeclarationOrder found no refKind const block in classify.go")
	}

	enumRendered := make(map[string]int, len(constNames))
	for i, name := range constNames {
		rendered := refKindName(refKind(i))
		enumRendered[rendered]++
		if enumRendered[rendered] > 1 {
			t.Errorf("classify.go's refKind const block renders %q more than once (via %s at iota position %d); every enum member must render a unique refKindName", rendered, name, i)
		}
	}

	sliceRendered := make(map[string]int, len(allRefKinds))
	for _, k := range allRefKinds {
		rendered := refKindName(k)
		sliceRendered[rendered]++
		if sliceRendered[rendered] > 1 {
			t.Errorf("allRefKinds renders %q more than once; every allRefKinds member must be a distinct refKind", rendered)
		}
	}

	for rendered, count := range enumRendered {
		if sliceRendered[rendered] != count {
			t.Errorf("classify.go's refKind const block renders %q %d time(s), but allRefKinds renders it %d time(s); allRefKinds has fallen out of sync with the enum", rendered, count, sliceRendered[rendered])
		}
	}
	for rendered, count := range sliceRendered {
		if enumRendered[rendered] != count {
			t.Errorf("allRefKinds renders %q %d time(s), but classify.go's refKind const block renders it %d time(s); allRefKinds has fallen out of sync with the enum", rendered, count, enumRendered[rendered])
		}
	}
}

// TestLedgerCompleteness asserts, for every refGate registered in ledger, that the policy's key
// set equals allRefKinds' members exactly -- no missing kind, no extra key -- so adding a fifth
// kind fails every gate's policy until each is re-acknowledged.
func TestLedgerCompleteness(t *testing.T) {
	t.Parallel()

	want := make(map[refKind]bool, len(allRefKinds))
	for _, k := range allRefKinds {
		want[k] = true
	}

	for gate, policy := range ledger {
		got := make(map[refKind]bool, len(policy))
		for k := range policy {
			got[k] = true
		}

		for k := range want {
			if !got[k] {
				t.Errorf("ledger[%q] is missing a disposition for kind %s", gate, refKindName(k))
			}
		}
		for k := range got {
			if !want[k] {
				t.Errorf("ledger[%q] declares a disposition for kind %s, which is not a member of allRefKinds", gate, refKindName(k))
			}
		}
	}
}

// TestLookupFailsClosedOnUndeclaredKind proves lookupIn/lookup panic rather than silently
// skipping a ref whose classified kind carries no declared disposition, and separately proves the
// happy path returns the classified kind and its declared disposition unchanged.
func TestLookupFailsClosedOnUndeclaredKind(t *testing.T) {
	t.Parallel()

	t.Run("PanicsOnMissingKindInPolicy", func(t *testing.T) {
		t.Parallel()

		// A deliberately incomplete policy: every kind except refKindPath is declared, so a
		// path-shaped ref classifies to a kind with no entry in the map, i.e. the disposition
		// zero value.
		incomplete := map[refKind]disposition{
			refKindSymbol: dispSkip,
			refKindGlyph:  dispSkip,
			refKindHandle: dispSkip,
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("lookupIn() did not panic on a ref classifying to a kind absent from the policy map")
			}
		}()
		lookupIn("test-incomplete-policy", incomplete, "internal/boardcli/list.go")
	})

	t.Run("PanicsOnGateAbsentFromLedger", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("lookup() did not panic on a refGate absent from ledger")
			}
		}()
		lookup(refGate("no-such-gate-registered-anywhere"), "internal/boardcli/list.go")
	})

	t.Run("HappyPathReturnsClassifiedKindAndDeclaredDisposition", func(t *testing.T) {
		t.Parallel()

		complete := map[refKind]disposition{
			refKindPath:   dispKeep,
			refKindSymbol: dispSkip,
			refKindGlyph:  dispSkip,
			refKindHandle: dispSkip,
		}

		gotKind, gotDisp := lookupIn("test-complete-policy", complete, "internal/boardcli/list.go")
		if gotKind != refKindPath {
			t.Errorf("lookupIn() kind = %v; want refKindPath", gotKind)
		}
		if gotDisp != dispKeep {
			t.Errorf("lookupIn() disposition = %v; want dispKeep", gotDisp)
		}
	})
}
