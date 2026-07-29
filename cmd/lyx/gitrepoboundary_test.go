// gitrepoboundary_test.go enforces the gitrepo Client Boundary Invariant: a pinned
// set of methods on internal/gitrepo's Repo type may call r.run (the CLI choke
// point), and exactly one gitexec.RunGit call site exists in the package's
// non-test source. The go-git/CLI boundary is otherwise invisible in the code --
// both backends are just method bodies -- so without this check a CLI call could
// seep back into a migrated method one bugfix at a time. See CONSTRAINTS.md's
// gitrepo Client Boundary Invariant.
//
// # The one blind spot this guard cannot see
//
// Set-equality on method names cannot detect a NEW r.run call added inside a
// method that is already on gitrepoPinnedRunBoundMethods. SnapshotSHA is
// precisely such a method: it hosts a migrated go-git ref read and a CLI-bound
// fetch side by side (see the package's Shared Decisions on call-granular
// classification), so a third, illegitimate r.run call slipped into SnapshotSHA
// would pass this guard's first assertion undetected. Reviewing SnapshotSHA's
// diff by hand remains necessary; this guard catches every other regression
// shape (a call added to a method not on the list, or the boundary's only
// gitexec.RunGit call site moving or duplicating).

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// gitrepoPinnedRunBoundMethods is the literal, pinned set of internal/gitrepo
// method names whose body contains at least one r.run( call, measured against
// the package's post-migration source. This is deliberately NOT "the CLI-bound
// methods" -- the two sets differ in both directions:
//
//   - SnapshotSHA appears here despite being a migrating method, because it
//     keeps a CLI-side fetch alongside its migrated go-git ref read (see the
//     call-granular classification decision).
//   - Push, PushCoalesced, and SetSnapshotSHA are CLI-bound by contract yet do
//     NOT appear here: they delegate to pushWithRebaseRetry /
//     advanceAndPushSnapshotRef (which do appear) or lost their own r.run
//     calls to go-git entirely.
var gitrepoPinnedRunBoundMethods = map[string]bool{
	"StageAndCommit":            true,
	"StageAllAndCommit":         true,
	"CheckoutDetached":          true,
	"RestoreBranch":             true,
	"Pull":                      true,
	"ResetHard":                 true,
	"pushWithRebaseRetry":       true,
	"SnapshotSHA":               true,
	"advanceAndPushSnapshotRef": true,
	"adoptSnapshotRef":          true,
	"hasUnpushed":               true,
}

// gitrepoBoundaryMinScannedFiles is the vacuous-scan floor for this guard's
// single-directory walk of internal/gitrepo. The package has 7 non-test .go
// files today (doc.go, gitrepo.go, gogit.go, pull.go, push.go, reset.go,
// snapshot.go); fewer than 5 found means the directory resolution is
// misconfigured rather than the package having genuinely shrunk.
const gitrepoBoundaryMinScannedFiles = 5

// TestGitrepoBoundary_PinnedRunCallSites walks internal/gitrepo's non-test .go
// files and asserts two things: (1) the set of methods whose body contains an
// r.run( call equals gitrepoPinnedRunBoundMethods exactly, and (2) after
// stripping comments, the token "gitexec." appears exactly once in the
// package's non-test source, inside the run method's own body. The second
// assertion closes the blind spot the first one has: a bare gitexec.RunGit
// call written directly inside a migrated method would satisfy an
// r.run(-keyed check while still violating the CLI/go-git boundary, since
// gitexec.RunGit is the CLI layer's own entry point, one level below r.run.
func TestGitrepoBoundary_PinnedRunCallSites(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH,
	// mirroring tierpurity_test.go and hermeticenv_test.go so this gate never
	// blocks a minimal environment.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the
	// test's working directory, exactly as tierpurity_test.go does, so the
	// walk is cwd-independent.
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	dir := filepath.Join(filepath.Dir(goMod), "internal", "gitrepo")

	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("read internal/gitrepo dir %s: %v", dir, readErr)
	}

	fset := token.NewFileSet()
	runBoundMethods := map[string]bool{}
	var scanned int
	var gitexecTotal int
	var runFound, runBodyHasGitexec bool

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		scanned++

		path := filepath.Join(dir, entry.Name())
		// Parsed with the default mode (no parser.ParseComments): the resulting
		// AST carries no comment text at all, so printing it back out via
		// go/printer below is what performs the "strip line and block comments"
		// step the gitexec. assertion needs -- more robust than a hand-rolled
		// comment stripper, since it cannot be confused by "//" or "/*" inside a
		// string literal.
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", entry.Name(), parseErr)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !isRepoPointerMethod(fn) {
				continue
			}
			recvName, named := receiverName(fn.Recv.List[0])
			if !named {
				continue
			}
			if bodyCallsMethodOnReceiver(fn.Body, recvName, "run") {
				runBoundMethods[fn.Name.Name] = true
			}
			if fn.Name.Name == "run" {
				runFound = true
				var body bytes.Buffer
				if printErr := printer.Fprint(&body, fset, fn.Body); printErr != nil {
					t.Fatalf("print run's body in %s: %v", entry.Name(), printErr)
				}
				if strings.Contains(body.String(), "gitexec.") {
					runBodyHasGitexec = true
				}
			}
		}

		var rendered bytes.Buffer
		if printErr := printer.Fprint(&rendered, fset, file); printErr != nil {
			t.Fatalf("print %s: %v", entry.Name(), printErr)
		}
		gitexecTotal += strings.Count(rendered.String(), "gitexec.")
	}

	// Vacuous-scan protection: a mis-resolved root that still finds a handful
	// of files must not pass.
	if scanned < gitrepoBoundaryMinScannedFiles {
		t.Fatalf("gitrepo boundary guard: only scanned %d non-test .go file(s) in %s; expected at least %d -- the directory resolution may be misconfigured", scanned, dir, gitrepoBoundaryMinScannedFiles)
	}

	if diff := diffMethodSets(gitrepoPinnedRunBoundMethods, runBoundMethods); diff != "" {
		t.Errorf("gitrepo Client Boundary Invariant violated (see CONSTRAINTS.md): r.run(-containing method set drifted from the pinned list:\n%s", diff)
	}

	if !runFound {
		t.Fatalf("gitrepo boundary guard: no run method found on *Repo in %s -- the guard's own assumptions may be stale", dir)
	}
	if gitexecTotal != 1 {
		t.Errorf("gitrepo Client Boundary Invariant violated (see CONSTRAINTS.md): expected exactly 1 non-comment occurrence of %q in internal/gitrepo, found %d", "gitexec.", gitexecTotal)
	}
	if gitexecTotal == 1 && !runBodyHasGitexec {
		t.Errorf("gitrepo Client Boundary Invariant violated (see CONSTRAINTS.md): the package's one gitexec. call site must live inside run's own body, not elsewhere")
	}
}

// isRepoPointerMethod reports whether fn is declared with a single, pointer-to-Repo
// receiver -- the shape every method on gitrepo's Repo type uses.
func isRepoPointerMethod(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "Repo"
}

// receiverName returns the declared receiver variable name for field, and false
// when the receiver is unnamed (e.g. "func (*Repo) X()") -- a shape that cannot
// call a method on itself by any name and is therefore never r.run-bound.
func receiverName(field *ast.Field) (string, bool) {
	if len(field.Names) == 0 || field.Names[0].Name == "" || field.Names[0].Name == "_" {
		return "", false
	}
	return field.Names[0].Name, true
}

// bodyCallsMethodOnReceiver reports whether body contains a call of the shape
// <recvName>.<methodName>(...) anywhere within it, including inside nested
// closures -- matching "every method containing an r.run( call" rather than
// only a top-level statement.
func bodyCallsMethodOnReceiver(body *ast.BlockStmt, recvName, methodName string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != methodName {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == recvName {
			found = true
		}
		return true
	})
	return found
}

// diffMethodSets returns a human-readable description of how got differs from
// want -- missing and unexpected method names, each sorted for a deterministic
// message -- or "" when the two sets are identical.
func diffMethodSets(want, got map[string]bool) string {
	var missing, unexpected []string
	for name := range want {
		if !got[name] {
			missing = append(missing, name)
		}
	}
	for name := range got {
		if !want[name] {
			unexpected = append(unexpected, name)
		}
	}
	if len(missing) == 0 && len(unexpected) == 0 {
		return ""
	}
	sort.Strings(missing)
	sort.Strings(unexpected)

	var b strings.Builder
	if len(missing) > 0 {
		fmt.Fprintf(&b, "  pinned but no longer r.run-bound: %s\n", strings.Join(missing, ", "))
	}
	if len(unexpected) > 0 {
		fmt.Fprintf(&b, "  r.run-bound but not pinned: %s\n", strings.Join(unexpected, ", "))
	}
	return b.String()
}
