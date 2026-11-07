// shape_test.go holds the Ref-Shape Registry's self-enforcement tests.
// They keep the `refKind` enum in step with `allRefKinds`, and the `refGate` constants in step with the `ledger`,
// so a new kind or gate cannot ship without its ledger policy, and lookup fails closed on an undeclared kind.

package planparser

import (
	"go/ast"
	"go/token"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRefKindEnumMatchesAllRefKinds asserts that the `refKind` enum and `allRefKinds` hold the same set of names.
// The enum side is read from classify.go's const block, paired with its values by iota position.
// Per-name counts also catch a list that repeats one kind and drops another, even at the same length.
// A constant with no `allRefKinds` counterpart fails the test.
func TestRefKindEnumMatchesAllRefKinds(t *testing.T) {
	tiergate.Require(t, 0)
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

// refGateValuesDeclared returns the value of every constant in shape.go's `refGate` const block.
//
// It reads values rather than identifier names, because the `ledger` is keyed by `refGate` value.
// Comparing values catches a mistyped constant value, which a comparison of identifier names would miss.
//
// Only the block whose first `ValueSpec` carries the explicit `refGate` type is read.
// A spec whose value is not a plain string literal is skipped;
// the registry declares none, and a computed gate value has no stable identity.
func refGateValuesDeclared(t *testing.T) []refGate {
	t.Helper()

	astFile, shapePath := planparserProdFile(t, "shape.go")

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

// TestRefGateConstantsMatchLedger checks set equality between shape.go's `refGate` constants and the `ledger` keys.
// Each constant needs a `ledger` entry, each `ledger` key needs a constant, and no two constants share a value.
//
// The forward direction catches the panicking case.
// A gate declared and dispatched through lookup but never given a ledger entry yields a nil policy map.
// So lookupIn reads the disposition zero value and panics on the first ref that reaches the gate.
// It is a raw Go panic, not a finding or an error envelope.
// A check that ranges over ledger's own keys cannot see that case, since an absent key contributes no iteration.
//
// The reverse direction catches a ledger key that matches no declared constant.
// Such a key is a typo or stale entry, which leaves the real gate's policy undeclared and causes the same panic.
func TestRefGateConstantsMatchLedger(t *testing.T) {
	tiergate.Require(t, 0)
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
			t.Errorf("shape.go declares refGate %q but ledger registers no policy for it; lookup would panic on the first ref reaching that gate (see PATTERN-sole-parsers)", gate)
		}
	}
	for gate := range ledger {
		if !declaredSet[gate] {
			t.Errorf("ledger registers a policy for %q, which shape.go declares no refGate constant for; the gate it was meant to cover is left undeclared and lookup would panic on it", gate)
		}
	}
}

// TestLedgerCompleteness asserts that each policy in ledger is keyed by exactly the members of allRefKinds.
// A missing kind or an extra key fails that gate's policy.
// Adding a kind therefore fails every gate until each is re-acknowledged.
//
// It does not cover a gate missing from ledger entirely, since a map range cannot visit an absent key.
// `TestRefGateConstantsMatchLedger` covers that half.
func TestLedgerCompleteness(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestLookupFailsClosedOnUndeclaredKind proves that lookupIn and lookup panic on an undeclared kind.
// They panic rather than silently skip the ref.
// The happy path returns the classified kind and its declared disposition unchanged.
func TestLookupFailsClosedOnUndeclaredKind(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("PanicsOnMissingKindInPolicy", func(t *testing.T) {
		t.Parallel()

		// An incomplete policy declares every kind except refKindPath.
		// A path-shaped ref therefore classifies to a kind with no map entry and gets the disposition zero value.
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
