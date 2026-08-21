// gitrepoboundary_test.go enforces the gitrepo Client Boundary Invariant: a pinned
// set of methods on internal/gitrepo's Repo type may call r.run or r.runChecked (the
// package's two CLI choke points), and exactly two gitexec entry-point call
// expressions -- one gitexec.RunGit, one gitexec.Run -- exist in the package's
// non-test source, one inside each chokepoint's own body. The go-git/CLI boundary is
// otherwise invisible in the code -- both backends are just method bodies -- so
// without this check a CLI call could seep back into a migrated method one bugfix at
// a time. See CONSTRAINTS.md's gitrepo Client Boundary Invariant.
//
// This guard is keyed by **method name** and answers which methods may reach the CLI
// at all; CONSTRAINTS.md's gitexec Checked-Call Invariant is keyed by **call site**
// and answers which sites, package-wide, may use the raw form. The two invariants are
// complementary, not redundant.
//
// # The one blind spot this guard cannot see
//
// Set-equality on method names cannot detect a NEW r.run call added inside a
// method that is already on gitrepoPinnedRunBoundMethods. StageAndCommit is
// precisely such a method: it hosts three CLI-bound r.run calls (add,
// diff --cached, commit) followed by a migrated go-git CurrentSHA read side
// by side, so a fourth, illegitimate r.run call slipped into StageAndCommit
// would pass this guard's first assertion undetected. Reviewing
// StageAndCommit's diff by hand remains necessary; this guard catches every
// other regression shape (a call added to a method not on the list, or the
// boundary's only gitexec.RunGit call site moving or duplicating).
//
// CommitEmpty is a second worked example of the same blind spot: it hosts two
// r.run calls (the dirty-index pre-check and the commit itself), which this
// guard's set-equality check counts as one pinned method either way, exactly
// like StageAndCommit's three.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// gitrepoPinnedRunBoundMethods is the literal, pinned set of internal/gitrepo
// method names whose body contains at least one r.run( or r.runChecked( call,
// measured against the package's post-migration source. This is deliberately NOT
// "the CLI-bound methods" -- the two sets differ in both directions:
//
//   - StageAndCommit appears here as a mixed method: three CLI-bound
//     r.runChecked calls (add, diff --cached, commit) sit alongside a migrated
//     go-git CurrentSHA read at the end -- presence in this list says nothing
//     about whether a method also has a migrated read.
//   - CommitEmpty is the same mixed shape: two CLI-bound r.runChecked calls (the
//     dirty-index pre-check and the commit) sit alongside a migrated go-git
//     CurrentSHA read on both the entry pre-check and the return path.
//   - Push and PushCoalesced are CLI-bound by contract yet do NOT appear
//     here: they delegate to pushWithRebaseRetry (which does appear)
//     rather than calling r.run or r.runChecked directly themselves.
//
// Only Pull and Fetch still call the raw r.run; every other entry here calls
// r.runChecked instead -- the raw/checked split is invisible to this set-equality
// check, which cares only that a pinned method calls one chokepoint or the other.
var gitrepoPinnedRunBoundMethods = map[string]bool{
	"StageAndCommit":      true,
	"CommitEmpty":         true,
	"StageAllAndCommit":   true,
	"CheckoutDetached":    true,
	"RestoreBranch":       true,
	"Pull":                true,
	"Fetch":               true,
	"IsAncestor":          true,
	"ResetHard":           true,
	"pushWithRebaseRetry": true,
	"PushRebaseFree":      true,
	"HasUnpushed":         true,
	"MergeStart":          true,
	"MergeConclude":       true,
	"ConflictedFiles":     true,
	"MergeHeadPresent":    true,
	"MergeHeads":          true,
	"MergeFFOnly":         true,
	"StageResolved":       true,
}

// gitrepoBoundaryMinScannedFiles is the vacuous-scan floor for this guard's
// single-directory walk of internal/gitrepo. The package has 10 non-test .go
// files today (ancestry.go, blobread.go, doc.go, gitrepo.go, gogit.go,
// merge.go, pull.go, push.go, reset.go, worktree.go); fewer than 5 found
// means the directory resolution is misconfigured rather than the package
// having genuinely shrunk.
const gitrepoBoundaryMinScannedFiles = 5

// TestGitrepoBoundary_PinnedRunCallSites walks internal/gitrepo's non-test .go files and asserts
// two things: (1) the set of methods whose body contains an r.run( or r.runChecked( call equals
// gitrepoPinnedRunBoundMethods exactly, and (2) exactly two gitexec.Run/RunGit call expressions
// exist in the package's non-test source, one inside run's own body and one inside runChecked's.
// The second assertion closes the blind spot the first one has: a bare gitexec.RunGit or
// gitexec.Run call written directly inside a migrated method would satisfy an r.run(/r.runChecked(-
// keyed check while still violating the CLI/go-git boundary, since gitexec's entry points are the
// CLI layer's own, one level below run and runChecked.
func TestGitrepoBoundary_PinnedRunCallSites(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH,
	// mirroring tierpurity_test.go and hermeticenv_test.go so this gate never
	// blocks a minimal environment.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the test's working directory.
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
	var runFound, runCheckedFound bool
	var runGitexecCalls, runCheckedGitexecCalls int

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		scanned++

		path := filepath.Join(dir, entry.Name())
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
			if bodyCallsMethodOnReceiver(fn.Body, recvName, "run") || bodyCallsMethodOnReceiver(fn.Body, recvName, "runChecked") {
				runBoundMethods[fn.Name.Name] = true
			}
			switch fn.Name.Name {
			case "run":
				runFound = true
				runGitexecCalls = countGitexecCalls(fn.Body)
			case "runChecked":
				runCheckedFound = true
				runCheckedGitexecCalls = countGitexecCalls(fn.Body)
			}
		}

		gitexecTotal += countGitexecCalls(file)
	}

	// Vacuous-scan protection: fewer than minimum found means misconfiguration.
	if scanned < gitrepoBoundaryMinScannedFiles {
		t.Fatalf("gitrepo boundary guard: only scanned %d non-test .go file(s) in %s; expected at least %d -- the directory resolution may be misconfigured", scanned, dir, gitrepoBoundaryMinScannedFiles)
	}

	if diff := diffMethodSets(gitrepoPinnedRunBoundMethods, runBoundMethods); diff != "" {
		t.Errorf("gitrepo Client Boundary Invariant violated (see CONSTRAINTS.md): r.run(/r.runChecked(-containing method set drifted from the pinned list:\n%s", diff)
	}

	if !runFound {
		t.Fatalf("gitrepo boundary guard: no run method found on *Repo in %s -- the guard's own assumptions may be stale", dir)
	}
	if !runCheckedFound {
		t.Fatalf("gitrepo boundary guard: no runChecked method found on *Repo in %s -- the guard's own assumptions may be stale", dir)
	}
	if gitexecTotal != 2 {
		t.Errorf("gitrepo Client Boundary Invariant violated (see CONSTRAINTS.md): expected exactly 2 gitexec.Run/RunGit call expressions in internal/gitrepo (the raw/checked pair), found %d", gitexecTotal)
	}
	if gitexecTotal == 2 {
		if runGitexecCalls != 1 {
			t.Errorf("gitrepo Client Boundary Invariant violated (see CONSTRAINTS.md): expected exactly 1 gitexec call expression inside run's own body, found %d", runGitexecCalls)
		}
		if runCheckedGitexecCalls != 1 {
			t.Errorf("gitrepo Client Boundary Invariant violated (see CONSTRAINTS.md): expected exactly 1 gitexec call expression inside runChecked's own body, found %d", runCheckedGitexecCalls)
		}
	}
}

// countGitexecCalls counts the *ast.CallExpr nodes under n whose callee is
// gitexec.Run or gitexec.RunGit -- internal/gitexec's two entry points -- so the
// boundary guard can pin the package's checked/raw call-site count by AST shape
// rather than by a whole-file substring count, which would also match the roughly
// half-dozen `var gitErr *gitexec.GitError` declarations the runChecked migration
// introduced.
func countGitexecCalls(n ast.Node) int {
	count := 0
	ast.Inspect(n, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "gitexec" {
			return true
		}
		if sel.Sel.Name == "Run" || sel.Sel.Name == "RunGit" {
			count++
		}
		return true
	})
	return count
}

// isRepoPointerMethod reports whether fn is a method with a pointer-to-Repo receiver.
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

// receiverName returns the receiver variable name for field, or false if unnamed.
func receiverName(field *ast.Field) (string, bool) {
	if len(field.Names) == 0 || field.Names[0].Name == "" || field.Names[0].Name == "_" {
		return "", false
	}
	return field.Names[0].Name, true
}

// bodyCallsMethodOnReceiver reports whether body calls recvName.methodName anywhere.
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

// diffMethodSets returns a description of how got differs from want, or "" if identical.
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
