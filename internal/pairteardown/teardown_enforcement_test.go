package pairteardown

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

const (
	fabricEnginePath = "internal/fabricengine"
	retiredHelper    = "RemovePairBranch"
	removeArgCount   = 4
	minScannedFiles  = 100
)

// TestEnforcement_PairTeardownChokepoint is the Pair Teardown Invariant's tripwire, not a completeness proof:
// it fails on a four-argument Remove call in a production file importing internal/fabricengine outside the two owning packages, and on any remaining reference to the retired RemovePairBranch helper.
func TestEnforcement_PairTeardownChokepoint(t *testing.T) {
	scanned := scankit.Walk(t, scankit.Options{Filter: scankit.All}, func(f *scankit.File) {
		for _, finding := range scanSource(t, f) {
			t.Error(finding)
		}
	})
	scankit.RequireFloor(t, scanned, minScannedFiles, "Go files")
}

func scanSource(t *testing.T, f *scankit.File) []string {
	t.Helper()
	var findings []string
	if f.Rel != "internal/pairteardown/teardown_enforcement_test.go" && strings.Contains(string(f.Data), retiredHelper) {
		findings = append(findings, f.Rel+": references the retired "+retiredHelper)
	}
	if strings.HasSuffix(f.Rel, "_test.go") || ownsRemove(f.Rel) {
		return findings
	}
	file := f.AST(t, parser.SkipObjectResolution)
	if !importsFabricEngine(file) {
		return findings
	}
	for range removeCalls(file) {
		findings = append(findings, f.Rel+": calls Remove with four arguments outside internal/pairteardown and internal/fabricengine")
	}
	return findings
}

// removeCalls returns every call of a method named Remove with four arguments.
func removeCalls(file *ast.File) []*ast.CallExpr {
	var calls []*ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Remove" && len(call.Args) == removeArgCount {
			calls = append(calls, call)
		}
		return true
	})
	return calls
}

func ownsRemove(rel string) bool {
	return strings.HasPrefix(rel, fabricEnginePath+"/") || strings.HasPrefix(rel, "internal/pairteardown/")
}

func importsFabricEngine(file *ast.File) bool {
	for _, imp := range file.Imports {
		if strings.HasSuffix(strings.Trim(imp.Path.Value, `"`), "/"+fabricEnginePath) {
			return true
		}
	}
	return false
}

// TestEnforcement_PairTeardownScanIsNotVacuous feeds the scan one offending and one clean snippet.
func TestEnforcement_PairTeardownScanIsNotVacuous(t *testing.T) {
	const offending = `package x
import "github.com/Knatte18/loomyard/internal/fabricengine"
func f(top *fabricengine.Topology) { top.Remove(nil, "s", false, false) }
`
	const clean = `package x
import "github.com/Knatte18/loomyard/internal/fabricengine"
func f(top *fabricengine.Topology) { _ = top.RemoveRefusal(nil, "s", false) }
`
	if got := snippetRemoveCalls(t, offending); got != 1 {
		t.Errorf("offending snippet Remove calls = %d, want 1", got)
	}
	if got := snippetRemoveCalls(t, clean); got != 0 {
		t.Errorf("clean snippet Remove calls = %d, want 0", got)
	}
}

func snippetRemoveCalls(t *testing.T, src string) int {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse snippet: %v", err)
	}
	if !importsFabricEngine(file) {
		t.Fatal("snippet does not import internal/fabricengine")
	}
	return len(removeCalls(file))
}
