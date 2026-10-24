// shape_helper_test.go holds the production-file parsers the ref-shape enforcement tests of every test tier share.

package planparser

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// planparserProdFile parses the named production file directly under internal/planparser through scankit and returns its AST with its module-relative path.
func planparserProdFile(t *testing.T, name string) (*ast.File, string) {
	t.Helper()

	want := "internal/planparser/" + name
	var astFile *ast.File
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/planparser"}, Shallow: true}, func(f *scankit.File) {
		if f.Rel == want {
			astFile = f.AST(t, 0)
		}
	})
	scankit.RequireFloor(t, scanned, 1, "planparser production-file lookup")
	if astFile == nil {
		t.Fatalf("%s not found", want)
	}
	return astFile, want
}

// refKindConstNamesInDeclarationOrder parses internal/planparser/classify.go and returns the identifiers declared in its refKind-typed const block, in declaration order.
// That is the same order that fixes each identifier's iota-derived refKind value.
func refKindConstNamesInDeclarationOrder(t *testing.T) []string {
	t.Helper()

	astFile, _ := planparserProdFile(t, "classify.go")

	var names []string
	for _, decl := range astFile.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		// A refKind const block's first ValueSpec carries the explicit "refKind = iota" type;
		// subsequent ValueSpecs in the same block inherit it implicitly (no repeated Type).
		// Only the block whose FIRST spec names refKind is the one this test cares about.
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
