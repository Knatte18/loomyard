// spawnrole_test.go enforces `PATTERN-agent-name`'s role-constant rule:
// the module that spawns an agent owns its role name as a constant, so no spawn spec carries an inline role literal.
// It is a tripwire over production source: it flags a `shuttleengine.Spec` or `reedengine.AddSpec` composite literal whose `Role` field is a string literal.
// It does not see a spec built field by field, or a literal reached through a variable.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// spawnSpecTypes are the selector expressions, as "package.Type", whose composite literals start an agent.
var spawnSpecTypes = map[string]bool{
	"shuttleengine.Spec": true,
	"reedengine.AddSpec": true,
}

// roleLiteralFindings returns one finding per spawn-spec composite literal in file whose Role field is a string literal.
func roleLiteralFindings(fset *token.FileSet, file *ast.File, rel string) []string {
	var findings []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isSpawnSpecType(lit.Type) {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "Role" {
				continue
			}
			if basic, ok := kv.Value.(*ast.BasicLit); ok && basic.Kind == token.STRING {
				findings = append(findings, "`PATTERN-agent-name` violated: "+rel+":"+strconv.Itoa(fset.Position(basic.Pos()).Line)+
					" spawns an agent with the inline role literal "+basic.Value+"; declare the role as a constant in the spawning module")
			}
		}
		return true
	})
	return findings
}

// isSpawnSpecType reports whether expr names a spawn-spec type.
func isSpawnSpecType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && spawnSpecTypes[pkg.Name+"."+sel.Sel.Name]
}

// TestSpawnRole_NoInlineRoleLiteral fails when a production spawn spec names its role with a string literal.
//
//lyx:guard
func TestSpawnRole_NoInlineRoleLiteral(t *testing.T) {
	scanned := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
		for _, finding := range roleLiteralFindings(f.FileSet(), f.AST(t, 0), f.Rel) {
			t.Error(finding)
		}
	})
	scankit.RequireFloor(t, scanned, 100, "production Go files")
}

// TestSpawnRole_FixtureFlagsInlineLiteral proves the scan flags a literal role and passes a constant one.
//
//testtiming:keep proves the TestSpawnRole_NoInlineRoleLiteral scan flags a literal role and passes a constant one
func TestSpawnRole_FixtureFlagsInlineLiteral(t *testing.T) {
	const src = `package fixture

import (
	"example/reedengine"
	"example/shuttleengine"
)

const namedRole = "named"

var literal = shuttleengine.Spec{Role: "inline"}
var literalAdd = reedengine.AddSpec{Role: "inline"}
var constant = shuttleengine.Spec{Role: namedRole}
var other = struct{ Role string }{Role: "inline"}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	findings := roleLiteralFindings(fset, file, "fixture.go")
	if len(findings) != 2 {
		t.Fatalf("roleLiteralFindings() = %d findings %q; want the two literal spec roles", len(findings), findings)
	}
}
