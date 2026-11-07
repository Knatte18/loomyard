// selvagepane_enforcement_test.go enforces that every identifier naming Selvage lives in selvagepane.go.
// One narrow carve-out is allowed elsewhere: state.go may declare the SelvagePaneID field that ReedState carries.
// That file's subject is the persisted record in general, not Selvage.
//
// The match is case-insensitive.
// A case-sensitive rule would silently permit selvagePaneID-shaped parameters and locals.
// Go identifiers conventionally start a local or parameter with a lowercase letter.
//
// Honest residual: the call-position exemption below covers same-package calls only.
// A Selvage-named function exported from another package is exempt at its call site and unscanned at its declaration.
// The gap is latent: internal/reedengine/render/ exports no Selvage-named identifier.

package reedengine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// selvageScanMinFiles is the floor on how many non-test .go files internal/reedengine holds;
// a scan below it has missed files.
const selvageScanMinFiles = 10

// selvagePaneAllowlist names the files in this package that may carry a Selvage-naming identifier.
// Elsewhere, such an identifier is allowed only in a call position or as a composite-literal key.
var selvagePaneAllowlist = []scankit.Entry{
	{Key: "internal/reedengine/selvagepane.go", Why: "owns the whole Selvage seam"},
	{Key: "internal/reedengine/state.go", Why: "carries the SelvagePaneID field declaration of ReedState"},
}

// TestSelvageIdentifiersConfinedToSelvagePane checks that rule over internal/reedengine.
// Outside selvagePaneAllowlist, no file may use a Selvage-naming identifier except as selvageIdentViolations exempts.
// It spawns no process, so it carries no build tag.
// It parses every non-test .go file directly inside internal/reedengine;
// it never enters internal/reedengine/render/ or any sibling package.
func TestSelvageIdentifiersConfinedToSelvagePane(t *testing.T) {
	tiergate.Require(t, 0)
	allow := scankit.NewAllowlist(selvagePaneAllowlist)

	var failures []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{reedengineScanDir}, Shallow: true}, func(f *scankit.File) {
		if allow.Allowed(f.Rel) {
			return
		}
		astFile := f.AST(t, 0)
		for _, violation := range selvageIdentViolations(f.FileSet(), astFile) {
			failures = append(failures, f.Rel+" "+violation)
		}
	})

	scankit.RequireFloor(t, scanned, selvageScanMinFiles, "selvage confinement")
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("Selvage-confinement check violated: %d identifier(s) outside selvagepane.go/state.go carry \"selvage\" (case-insensitive): %v -- move the code owning them into selvagepane.go", len(failures), failures)
	}
}

// TestSelvageIdentViolations_ExemptionsAndFlags drives selvageIdentViolations over synthetic in-memory source.
// It checks both exemptions and each flagged form, so a scanner that stops exempting or stops flagging fails here.
//
//testtiming:keep pins the Selvage identifier scanner's exemptions and flags over a synthetic source, so the confinement scan cannot pass vacuously; its covering tests run this code without asserting it
func TestSelvageIdentViolations_ExemptionsAndFlags(t *testing.T) {
	tiergate.Require(t, 0)
	const src = `package fake

func example(e *engine, st *state, selvagePaneID string) {
	_ = render.Selvage{}
	_ = e.cfg.Selvage
	_ = st.SelvagePaneID
	_ = render.Params{Selvage: x}
	e.ensureSelvagePaneLocked(st)
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fake.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	got := selvageIdentViolations(fset, astFile)

	countSuffix := func(suffix string) int {
		n := 0
		for _, v := range got {
			if strings.HasSuffix(v, suffix) {
				n++
			}
		}
		return n
	}

	// The render.Selvage{} composite-literal type and the e.cfg.Selvage selector each produce a ": Selvage" violation.
	// The composite-literal key in render.Params{Selvage: x} produces none, because it is exempt.
	if n := countSuffix(": Selvage"); n != 2 {
		t.Errorf("selvageIdentViolations() = %v; want exactly 2 violations ending \": Selvage\" (the render.Selvage{} composite-literal type and the e.cfg.Selvage selector), got %d", got, n)
	}
	if n := countSuffix(": SelvagePaneID"); n != 1 {
		t.Errorf("selvageIdentViolations() = %v; want exactly 1 violation ending \": SelvagePaneID\" (the st.SelvagePaneID selector), got %d", got, n)
	}
	if n := countSuffix(": selvagePaneID"); n != 1 {
		t.Errorf("selvageIdentViolations() = %v; want exactly 1 violation ending \": selvagePaneID\" (the parameter), got %d", got, n)
	}
	if len(got) != 4 {
		t.Errorf("selvageIdentViolations() = %v (%d violations); want exactly 4 -- the render.Params{Selvage: x} composite-literal KEY and the e.ensureSelvagePaneLocked(st) call-position identifier must both be exempt", got, len(got))
	}
}

// selvageIdentViolations returns one "position: identifier" string per flagged identifier in astFile.
// An identifier is flagged when its name contains "selvage" in any case, unless one of two exemptions applies.
//
// Exemption (i) is the function position of a call expression:
// the Fun of an *ast.CallExpr, whether a bare f(...) identifier or the Sel of an x.f(...) selector.
// A file may call a Selvage-named helper.
//
// Exemption (ii) is a composite-literal field key:
// the Key of an *ast.KeyValueExpr inside an *ast.CompositeLit.
// A file may populate a struct field named Selvage, as in render.Params{Selvage: x}.
// The field's declaration carries the name, not the key.
//
// Every other occurrence is flagged:
// field selectors, composite-literal types, func/method/type/var declarations, struct fields, parameters and locals.
// Comments and string literals are never read, since go/parser does not turn either into an *ast.Ident.
// A comment naming Selvage and a `yaml:"selvage"` struct tag both pass.
func selvageIdentViolations(fset *token.FileSet, astFile *ast.File) []string {
	exempt := make(map[*ast.Ident]bool)

	ast.Inspect(astFile, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			switch fun := node.Fun.(type) {
			case *ast.Ident:
				exempt[fun] = true
			case *ast.SelectorExpr:
				exempt[fun.Sel] = true
			}
		case *ast.CompositeLit:
			for _, elt := range node.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok {
					exempt[key] = true
				}
			}
		}
		return true
	})

	var violations []string
	ast.Inspect(astFile, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if exempt[ident] {
			return true
		}
		if !strings.Contains(strings.ToLower(ident.Name), "selvage") {
			return true
		}
		violations = append(violations, fset.Position(ident.Pos()).String()+": "+ident.Name)
		return true
	})

	return violations
}
