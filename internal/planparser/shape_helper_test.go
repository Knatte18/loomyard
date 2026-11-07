// shape_helper_test.go holds the production-file parsers the ref-shape enforcement tests of every test tier share.

package planparser

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// planparserProdFile parses the named production file directly under internal/planparser through scankit.
// It returns the file's AST and its module-relative path.
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

// refKindConstNamesInDeclarationOrder returns the names in classify.go's `refKind` const block, in declaration order.
// Go cannot reflect over a package's constants, so the names come from the AST.
// The declaration order fixes each name's iota-derived `refKind` value.
func refKindConstNamesInDeclarationOrder(t *testing.T) []string {
	t.Helper()

	astFile, _ := planparserProdFile(t, "classify.go")

	var names []string
	for _, decl := range astFile.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		// The block's first `ValueSpec` carries the explicit `refKind` type, as in `name refKind = iota`.
		// Later `ValueSpec`s in the block inherit that type and repeat none.
		// Only the block whose first spec names `refKind` is read.
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
