//go:build tier0

// pausegeometry_test.go pins that the orch module wires its shuttle runner with no pause geometry.

package orchcli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLI_OrchRunnerReadsNoPause pins that no production file of the package tells a runner a pause geometry,
// so the runner the orch command wires reads every target uncovered and `lyx orch start` types its start lines under a pause entry.
// The wired runner is built inside the command's pre-run and is unreachable from a test, so the pin is a scan for the call that would give it a geometry.
func TestCLI_OrchRunnerReadsNoPause(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			if selector, isSelector := call.Fun.(*ast.SelectorExpr); isSelector && selector.Sel.Name == "SetPause" {
				t.Errorf("%s: %s tells a runner a pause geometry; the orch runner must read no pause", fset.Position(call.Pos()), selector.Sel.Name)
			}
			return true
		})
	}
}
