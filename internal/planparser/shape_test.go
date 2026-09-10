// shape_test.go is the Ref-Shape Registry's own self-enforcement suite. The registry's fail-closed
// promise rests on TWO independent syncs, and this file proves both, in both directions:
//
//   - classify.go's refKind enum <-> allRefKinds (TestRefKindEnumMatchesAllRefKinds), so a fifth
//     kind cannot be added without every gate's policy being re-acknowledged; and
//   - shape.go's own refGate const block <-> ledger's key set (TestRefGateConstantsMatchLedger), so
//     a fourteenth gate cannot be declared -- nor a ledger key typo'd -- without a ledger entry.
//
// It further proves every ledger policy covers the whole kind domain (TestLedgerCompleteness) and
// that the lookup path fails closed on an undeclared disposition rather than silently skipping the
// ref (TestLookupFailsClosedOnUndeclaredKind).
//
// Both sync tests parse a const block out of the AST rather than ranging a Go value, and for the
// same reason: Go cannot reflect over a package's constants, so a const block absent from its
// companion collection is invisible to any test that ranges only the collection. That asymmetry is
// exactly the gap TestRefGateConstantsMatchLedger closes -- TestLedgerCompleteness ranges ledger,
// so a refGate constant with NO ledger entry contributed no iteration and passed, leaving lookup to
// panic at runtime inside a live CLI verb (crucible round opus5-high-r1, F1).
//
// The AST-parsing tests follow the idiom of internal/cliwire/bannedecl_enforcement_test.go: stdlib
// go/parser only, repo root resolved from runtime.Caller(0), production files only (this file
// itself is a _test.go file and is never parsed as a scan target).

package planparser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
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

// refGateValuesDeclared parses internal/planparser/shape.go and returns the VALUE of every constant
// declared in its refGate-typed const block -- "bare-symbol-target", "directory-target", and so on.
//
// It reads values rather than identifier names because ledger is keyed by the refGate value, so a
// value is what the two sides actually have in common: a constant whose identifier is spelled
// correctly but whose string value is typo'd is precisely one of the two defects this enables
// TestRefGateConstantsMatchLedger to catch, and a name-based comparison would miss it.
//
// Only the const block whose first ValueSpec carries the explicit refGate type is read, mirroring
// refKindConstNamesInDeclarationOrder's own block selection; a spec whose value is not a plain
// string literal is skipped, since the registry declares none and a computed gate value would have
// no stable identity to compare against anyway.
func refGateValuesDeclared(t *testing.T) []refGate {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine planparser source directory location")
	}
	shapePath := filepath.Join(filepath.Dir(thisFile), "shape.go")

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, shapePath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", shapePath, err)
	}

	var gates []refGate
	for _, decl := range astFile.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST || len(gd.Specs) == 0 {
			continue
		}
		first, ok := gd.Specs[0].(*ast.ValueSpec)
		if !ok || first.Type == nil {
			continue
		}
		ident, ok := first.Type.(*ast.Ident)
		if !ok || ident.Name != "refGate" {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, value := range vs.Values {
				lit, ok := value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				unquoted, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote refGate constant value %s in %s: %v", lit.Value, shapePath, err)
				}
				gates = append(gates, refGate(unquoted))
			}
		}
	}

	return gates
}

// TestRefGateConstantsMatchLedger asserts set equality, in both directions, between the refGate
// constants shape.go declares and the gates ledger registers a policy for.
//
// The forward direction is the one with teeth: a gate declared and dispatched through lookup but
// never given a ledger entry yields a nil policy map, so lookupIn reads the disposition zero value
// and PANICS -- out of `lyx loom validate-plan` or `lyx webster begin-batch`, on the first plan
// whose refs reach that gate, as a raw Go panic rather than a finding or an error envelope.
// TestLedgerCompleteness cannot see that case at all, because it iterates ledger's own keys and an
// absent key contributes no iteration.
//
// The reverse direction catches the same defect approached from the other side: a ledger key that
// matches no declared constant is a typo'd or stale entry, which leaves the real gate's policy
// undeclared and produces the identical runtime panic.
func TestRefGateConstantsMatchLedger(t *testing.T) {
	t.Parallel()

	declared := refGateValuesDeclared(t)
	if len(declared) == 0 {
		t.Fatal("refGateValuesDeclared found no refGate const block in shape.go")
	}

	declaredSet := make(map[refGate]bool, len(declared))
	for _, gate := range declared {
		if declaredSet[gate] {
			t.Errorf("shape.go declares the refGate value %q more than once; every gate must have a distinct value", gate)
		}
		declaredSet[gate] = true
	}

	for gate := range declaredSet {
		if _, ok := ledger[gate]; !ok {
			t.Errorf("shape.go declares refGate %q but ledger registers no policy for it; lookup would panic on the first ref reaching that gate (see CONSTRAINTS.md's Ref-Shape Registry Invariant)", gate)
		}
	}
	for gate := range ledger {
		if !declaredSet[gate] {
			t.Errorf("ledger registers a policy for %q, which shape.go declares no refGate constant for; the gate it was meant to cover is left undeclared and lookup would panic on it", gate)
		}
	}
}

// TestLedgerCompleteness asserts, for every refGate registered in ledger, that the policy's key
// set equals allRefKinds' members exactly -- no missing kind, no extra key -- so adding a fifth
// kind fails every gate's policy until each is re-acknowledged.
//
// It says nothing about a gate MISSING from ledger entirely: a map range cannot visit an absent
// key. TestRefGateConstantsMatchLedger above owns that half.
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
