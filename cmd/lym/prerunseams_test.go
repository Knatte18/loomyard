// prerunseams_test.go guards that root's PersistentPreRunE installs heddle's seams, which no behavioural test can pin because the installs are skipped under testing.Testing().

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPersistentPreRunE_InstallsSeams parses cmd/lym/main.go, finds root's PersistentPreRunE and fails, naming the missing call, unless its body installs the worktree seam and the passage reader and registers one Bolt replayer from boardengine and one from heddleengine.
func TestPersistentPreRunE_InstallsSeams(t *testing.T) {
	tiergate.Require(t, 0)
	mainGoPath := filepath.Join(scankit.Root(t), "cmd", "lym", "main.go")
	astFile, err := parser.ParseFile(token.NewFileSet(), mainGoPath, nil, 0)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", mainGoPath, err)
	}
	preRun := findPersistentPreRunELit(astFile)
	if preRun == nil {
		t.Fatal("root's PersistentPreRunE function literal not found in cmd/lym/main.go's newRoot -- this guard's own target moved or was renamed")
	}

	// calls maps each called selector to the package qualifiers of the composite-literal types handed to it.
	calls := map[string][]string{}
	ast.Inspect(preRun.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := selectorName(call.Fun)
		if name == "" {
			return true
		}
		calls[name] = append(calls[name], "")
		for _, arg := range call.Args {
			if literal, ok := arg.(*ast.CompositeLit); ok {
				if qualifier := selectorName(literal.Type); qualifier != "" {
					calls[name] = append(calls[name], qualifier)
				}
			}
		}
		return true
	})

	for _, want := range []string{"fabricengine.InstallWorktreeSeam", "shuttleengine.InstallPassageReader"} {
		if len(calls[want]) == 0 {
			t.Errorf("root's PersistentPreRunE never calls %s", want)
		}
	}
	for _, want := range []string{"boardengine.FabricReplay", "heddleengine.Replayer"} {
		found := 0
		for _, qualifier := range calls["fabricengine.RegisterBoltReplayer"] {
			if qualifier == want {
				found++
			}
		}
		if found != 1 {
			t.Errorf("root's PersistentPreRunE calls fabricengine.RegisterBoltReplayer with a %s value %d times; want once", want, found)
		}
	}
}

// selectorName returns "pkg.Name" for a package-qualified selector expression, and "" for any other expression.
func selectorName(expr ast.Expr) string {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return pkg.Name + "." + selector.Sel.Name
}
