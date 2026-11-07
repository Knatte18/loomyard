// prerunlogging_test.go guards root's PersistentPreRunE (newRoot, main.go).
// It must log nothing at Info or above before its seedStencils(cmd) call.
// cobra.EnableTraverseRunHooks runs every module's PersistentPreRunE after root's own pre-run.
// The shared standalone prologue, internal/cliwire.ResolveStandalone, redirects the durable trace sink.
// The target is standalonegeom.LogsDir(stateDir), set once standalonestate.Derive returns.
// That redirect binds only if nothing earlier has armed the sink.
// The sink arms lazily on the first Info-or-above record anywhere in the process.
//
// Only logger.Info and logger.Warn are guarded.
// internal/logger exports Debug, Info and Warn and no Error, and Debug sits below the arming threshold.
// A Debug call ahead of seedStencils is therefore harmless and does not fail the guard.
//
// The guard is source-level because seedStencils returns under testing.Testing() before it resolves anything.
// The early return is in stencilseed.go.
// A behavioural test would exercise the test guard rather than the standalone gate,
// and it would keep passing after the dependency broke.
// The guard parses main.go's AST and asserts the ordering, as cmd/lym/spawnobservability_test.go does.
// A substring scan would misfire on a doc-comment mention of logger.Info, which is not a call.
//
// The other half of the dependency is that stencilSeedTarget reports ok == false for a non-hub location.
// That keeps seedStencils from logging in standalone.
// TestStencilSeedTarget_PlainRepoHasNoHub in cmd/lym/stencilseed_integration_test.go pins that.

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

// TestPersistentPreRunE_NoInfoOrWarnLoggingAheadOfSeedStencils parses cmd/lym/main.go.
// It fails if a statement preceding seedStencils(cmd) in root's PersistentPreRunE calls logger.Info or logger.Warn.
//
//testtiming:keep pins that no Info or Warn log runs ahead of seedStencils in the root pre-run, a guard that fires on a source parse no other test reads
func TestPersistentPreRunE_NoInfoOrWarnLoggingAheadOfSeedStencils(t *testing.T) {
	tiergate.Require(t, 0)
	mainGoPath := filepath.Join(scankit.Root(t), "cmd", "lym", "main.go")

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, mainGoPath, nil, 0)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", mainGoPath, err)
	}

	preRun := findPersistentPreRunELit(astFile)
	if preRun == nil {
		t.Fatal("root's PersistentPreRunE function literal not found in cmd/lym/main.go's newRoot -- this guard's own target moved or was renamed")
	}

	foundSeedCall := false
	for _, stmt := range preRun.Body.List {
		if stmtCallsFunction(stmt, "seedStencils") {
			foundSeedCall = true
			break
		}
		if name, ok := stmtCallsLoggerInfoOrWarn(stmt); ok {
			t.Fatalf("root's PersistentPreRunE calls logger.%s before seedStencils(cmd): "+
				"cobra.EnableTraverseRunHooks runs every module's own PersistentPreRunE (including "+
				"webstercli's and burlercli's standalone sink redirect, owned by "+
				"internal/cliwire.ResolveStandalone) AFTER root's, so any Info-or-above "+
				"record logged here arms the durable sink at the wrong (non-standalone) directory before "+
				"the redirect ever runs", name)
		}
	}
	if !foundSeedCall {
		t.Fatal("root's PersistentPreRunE never calls seedStencils(cmd) -- this guard's ordering assumption no longer holds")
	}
}

// findPersistentPreRunELit returns the function literal under a PersistentPreRunE key in file, or nil if there is none.
func findPersistentPreRunELit(file *ast.File) *ast.FuncLit {
	var found *ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "PersistentPreRunE" {
			return true
		}
		lit, ok := kv.Value.(*ast.FuncLit)
		if !ok {
			return true
		}
		found = lit
		return false
	})
	return found
}

// stmtCallsFunction reports whether stmt contains a call to the function name, written as a bare identifier.
// A selector call such as pkg.name does not match; a same-package call such as `seedStencils(cmd)` does.
func stmtCallsFunction(stmt ast.Stmt, name string) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if ok && ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

// stmtCallsLoggerInfoOrWarn reports whether stmt contains a call to logger.Info or logger.Warn.
// It matches a selector whose receiver is the bare identifier logger, the name main.go imports internal/logger under.
// It returns the matched method name ("Info" or "Warn") alongside the bool for the caller's failure message.
func stmtCallsLoggerInfoOrWarn(stmt ast.Stmt) (string, bool) {
	name := ""
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "logger" {
			return true
		}
		if sel.Sel.Name == "Info" || sel.Sel.Name == "Warn" {
			name = sel.Sel.Name
			found = true
			return false
		}
		return true
	})
	return name, found
}
