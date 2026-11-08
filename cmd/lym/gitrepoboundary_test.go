// gitrepoboundary_test.go enforces the gitrepo Client Boundary Invariant of `PATTERN-gitrepo-client-boundary`.
// Only the pinned methods of internal/gitrepo's Repo type may call r.run or r.runChecked.
// Those two are the package's CLI choke points.
// The package's non-test source holds exactly one gitexec.RunGit call and one gitexec.Run call.
// Each lies inside one choke point.
// The go-git and CLI backends are both plain method bodies, so the boundary is otherwise invisible.
// Without this guard, a CLI call could seep back into a method one bugfix at a time.

package main

import (
	"fmt"
	"go/ast"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// gitrepoPinnedRunBoundMethods pins the internal/gitrepo methods whose body calls r.run or r.runChecked directly.
// It is not the set of methods that reach the CLI: a method that does so only through another method is absent.
// Push, for example, delegates to a rebase-retry helper and is absent for that reason.
// The set does not tell r.run from r.runChecked, so a listed method may call either.
var gitrepoPinnedRunBoundMethods = []string{
	"StageAndCommit",
	"CommitEmpty",
	"StageAllAndCommit",
	"Pull",
	"Fetch",
	"FetchWithArgs",
	"IsAncestor",
	"ResetHard",
	"ResetKeep",
	"CherryPick",
	"CherryPickAbort",
	"pushWithRebaseRetry",
	"pushPlainWithRetry",
	"resolvePushDestination",
	"recordPush",
	"moveTrackingRef",
	"HasUnpushed",
	"HasUnpulled",
	"DeleteRemoteBranch",
	"DeleteRemoteBranchLeased",
	"UpdateRemoteBranchLeased",
	"DropRemoteTracking",
	"CommitsNotIn",
	"BlameLines",
	"MergeStart",
	"MergeConclude",
	"ConflictedFiles",
	"MergeHeadPresent",
	"MergeTree",
	"MergeFFOnly",
	"StageResolved",
	"StageTrackedChanges",
}

// gitrepoBoundaryMinScannedFiles is the vacuous-scan floor for this guard's single-directory walk of internal/gitrepo.
// A walk that finds fewer files than this means the directory resolution is misconfigured, not that the package shrank.
const gitrepoBoundaryMinScannedFiles = 5

// TestGitrepoBoundary_PinnedRunCallSites walks internal/gitrepo's non-test .go files and asserts two things.
// First, the methods whose body calls r.run or r.runChecked equal gitrepoPinnedRunBoundMethods exactly.
// Second, exactly two gitexec.Run or gitexec.RunGit call expressions exist in the package's non-test source.
// One sits inside run's body and one inside runChecked's.
// The first assertion cannot see a new call added inside a method already on the list.
// Such methods, for example `StageAndCommit` and `CommitEmpty`, need their diffs reviewed by hand.
// The second assertion catches a bare gitexec call written directly in a method.
// Such a call would satisfy the first assertion while still violating the CLI/go-git boundary.
// A narrowed gate runs it only when a file of internal/gitrepo or scankit changes; a whole gate always runs it.
//
//lym:guard
//lym:watch internal/gitrepo/*.go internal/testkit/scankit/*.go
func TestGitrepoBoundary_PinnedRunCallSites(t *testing.T) {
	tiergate.Require(t, 0)
	const dir = "internal/gitrepo"

	runBoundMethods := map[string]bool{}
	var gitexecTotal int
	var runFound, runCheckedFound bool
	var runGitexecCalls, runCheckedGitexecCalls int

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{dir}, Shallow: true}, func(f *scankit.File) {
		file := f.AST(t, 0)

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
	})

	scankit.RequireFloor(t, scanned, gitrepoBoundaryMinScannedFiles, "gitrepo boundary guard")

	pinned := map[string]bool{}
	for _, name := range gitrepoPinnedRunBoundMethods {
		pinned[name] = true
	}
	if diff := diffMethodSets(pinned, runBoundMethods); diff != "" {
		t.Errorf("`PATTERN-gitrepo-client-boundary` violated: r.run(/r.runChecked(-containing method set drifted from the pinned list:\n%s", diff)
	}

	if !runFound {
		t.Fatalf("gitrepo boundary guard: no run method found on *Repo in %s -- the guard's own assumptions may be stale", dir)
	}
	if !runCheckedFound {
		t.Fatalf("gitrepo boundary guard: no runChecked method found on *Repo in %s -- the guard's own assumptions may be stale", dir)
	}
	if gitexecTotal != 2 {
		t.Errorf("`PATTERN-gitrepo-client-boundary` violated: expected exactly 2 gitexec.Run/RunGit call expressions in internal/gitrepo (the raw/checked pair), found %d", gitexecTotal)
	}
	if gitexecTotal == 2 {
		if runGitexecCalls != 1 {
			t.Errorf("`PATTERN-gitrepo-client-boundary` violated: expected exactly 1 gitexec call expression inside run's own body, found %d", runGitexecCalls)
		}
		if runCheckedGitexecCalls != 1 {
			t.Errorf("`PATTERN-gitrepo-client-boundary` violated: expected exactly 1 gitexec call expression inside runChecked's own body, found %d", runCheckedGitexecCalls)
		}
	}
}

// countGitexecCalls counts the *ast.CallExpr nodes under n whose callee is gitexec.Run or gitexec.RunGit.
// Those two are the entry points of internal/gitexec.
// It counts call nodes rather than text.
// A substring count would also match `var gitErr *gitexec.GitError` declarations.
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
