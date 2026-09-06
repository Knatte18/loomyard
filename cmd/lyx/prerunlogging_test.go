// prerunlogging_test.go guards a dependency the standalone-mode fix relies on but cannot pin
// behaviourally: root's PersistentPreRunE (newRoot, main.go) must not log at Info or above before its
// seedStencils(cmd) call. cobra.EnableTraverseRunHooks runs every module's own PersistentPreRunE
// (including webstercli's and burlercli's wireStandalone, which now redirect the durable trace sink
// to standalonegeom.LogsDir(stateDir) the moment standalonestate.Derive returns) AFTER root's own
// pre-run, so that redirect only binds if nothing in root's pre-run has already armed the sink by
// emitting an Info-or-above record — the sink is armed lazily on the first such record anywhere in
// the process.
//
// Only logger.Info and logger.Warn are guarded against: internal/logger exports Debug, Info, and
// Warn and no Error at all, and Debug sits below the Info-or-above threshold that arms the sink, so a
// Debug call ahead of seedStencils is harmless and must not fail this guard.
//
// This is a SOURCE-LEVEL guard, not a behavioural one, because no behavioural test can pin the same
// property: seedStencils returns before resolving anything under testing.Testing() (see
// stencilseed.go), so a test asserting "root pre-run emits no Info+ record in standalone" would pass
// through the test guard rather than through the standalone gate, and would keep passing after the
// dependency it claims to pin had already broken. This guard therefore parses main.go's source and
// asserts the ordering directly, following cmd/lyx/spawnobservability_test.go's own precedent of
// walking go/parser's AST rather than scanning for substrings: a doc-comment mention of logger.Info
// is not a call, and a substring guard would misfire on this file's own header comment above.
//
// The other half of this same dependency — stencilSeedTarget reporting ok == false for a non-hub
// location, which is what keeps seedStencils from reaching its own log calls in standalone in the
// first place — is already pinned by TestStencilSeedTarget_PlainRepoHasNoHub in
// cmd/lyx/stencilseed_integration_test.go, and is deliberately not duplicated here.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPersistentPreRunE_NoInfoOrWarnLoggingAheadOfSeedStencils parses cmd/lyx/main.go, locates root's
// PersistentPreRunE function literal inside newRoot's composite literal, and fails if any statement
// preceding its seedStencils(cmd) call contains a logger.Info or logger.Warn call.
func TestPersistentPreRunE_NoInfoOrWarnLoggingAheadOfSeedStencils(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)
	mainGoPath := filepath.Join(moduleRoot, "cmd", "lyx", "main.go")

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, mainGoPath, nil, 0)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", mainGoPath, err)
	}

	preRun := findPersistentPreRunELit(astFile)
	if preRun == nil {
		t.Fatal("root's PersistentPreRunE function literal not found in cmd/lyx/main.go's newRoot -- this guard's own target moved or was renamed")
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
				"webstercli's and burlercli's standalone sink redirect) AFTER root's, so any Info-or-above "+
				"record logged here arms the durable sink at the wrong (non-standalone) directory before "+
				"the redirect ever runs", name)
		}
	}
	if !foundSeedCall {
		t.Fatal("root's PersistentPreRunE never calls seedStencils(cmd) -- this guard's ordering assumption no longer holds")
	}
}

// findPersistentPreRunELit walks file looking for a composite-literal key "PersistentPreRunE" whose
// value is a function literal, returning that literal or nil if none is found.
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

// stmtCallsFunction reports whether stmt contains a call to a function named name, called as a bare
// identifier (not a selector) -- the shape a same-package call like seedStencils(cmd) takes.
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

// stmtCallsLoggerInfoOrWarn reports whether stmt contains a call to logger.Info or logger.Warn,
// matched as a selector expression off the bare identifier "logger" (this package's own import name
// for internal/logger, per main.go's import block). It returns the matched method name ("Info" or
// "Warn") alongside the bool for the caller's failure message.
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
