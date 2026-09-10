// completionsignal_enforcement_test.go is the tripwire for the Completion Signal Invariant that
// wait.go's own file doc comment states and CONSTRAINTS.md cross-references: every code path in this
// package that finalizes a NEGATIVE answer to "did this run finish" must consult allOutputFilesExist
// over the run's OutputFiles first.
//
// It is deliberately NOT the shape internal/planparser/shape.go's ledger meta-tests take, and the
// difference is worth stating because the precedent is close enough to be misleading. Those tests
// prove COMPLETENESS, because their domain is a closed four-value enum they can range over. This one
// cannot prove anything of the sort: the domain here is "however many return statements a future diff
// adds", which is not enumerable in advance. What it does instead is force a human to look. A diff
// that adds a negative-verdict return, or deletes a file-contract check, fails this test with a
// message naming exactly what moved; a reader then confirms the new site consults the check and
// updates the audited set below. That is a tripwire, not a proof, and it is claimed as nothing more.
//
// TWO assertions, and neither subsumes the other:
//
//   - TestCompletionSignal_NegativeVerdictReturnSites pins the negative-verdict return sites. This is
//     the one that catches an UNGUARDED NEW EXIT, which is how all six known instances arrived. A
//     test that only counted allOutputFilesExist calls could not: an unguarded new return adds no
//     such call, so the count would still read "as expected" while the bug was live. Crucible round
//     opus5-high-r7's F1 is the proof — three negative exits in attach.go that never called the
//     check, present the whole time the audited call-site count was correct.
//   - TestCompletionSignal_FileContractCallSites pins the allOutputFilesExist call sites. This is the
//     one that catches the opposite mutation: a guard being DELETED from an exit that already has
//     one, which leaves the return-site set unchanged.
//
// Both scans are AST-based rather than textual so a mention inside a doc comment or a string literal
// can never trip them.

package shuttleengine

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// completionSignalScannedFiles are the two files that own every run-outcome verdict in this package:
// wait.go finalizes a live run's outcome, attach.go decides whether a persisted one is reconstructed
// or respawned over. A future file that grows a third kind of verdict belongs in this list, and the
// failure message says so.
var completionSignalScannedFiles = []string{"wait.go", "attach.go"}

// negativeVerdictMarkers are the identifiers whose appearance anywhere in a return statement's
// results makes that return a NEGATIVE answer to "did this run finish".
//
// The four the invariant names — OutcomeDied, OutcomeTimeout, a mechanism-failure error, and
// verdictRespawnEligible — plus three that are how two of those four are actually spelled at a
// return: Errorf for the mechanism-failure errors, and the two package-level sentinels
// checkLivenessTick returns for reed's own bookkeeping going away. verdictError is included because
// it is the third disposition of the same three-way choice verdictRespawnEligible belongs to, and a
// new one of those is exactly as worth a human look.
//
// OutcomeDone and OutcomeAsking are deliberately absent: they are the positive and the
// still-in-progress answers, and neither abandons a run.
var negativeVerdictMarkers = map[string]bool{
	"OutcomeDied":                 true,
	"OutcomeTimeout":              true,
	"verdictRespawnEligible":      true,
	"verdictError":                true,
	"errStrandNotTracked":         true,
	"errStrandPaneBindingCleared": true,
	"Errorf":                      true,
}

// auditedNegativeVerdictReturns is the audited set of negative-verdict return sites as of crucible
// round opus5-high-r7, keyed by "<function> [<markers>]" with the number of such returns in that
// function as the value.
//
// Keying on the enclosing function plus the marker set — rather than on file:line — is deliberate:
// line numbers drift on every edit above them, which would make this test fail for reasons that have
// nothing to do with the invariant and get it deleted. This key changes only when a return statement
// is added, removed, or changes which negative value it yields.
//
// Every entry carries its justification, because "the count is 7" is not something a future reader
// can check without one:
//
//   - Wait [Errorf] x4 — the events-unreadable cap and the three status-cap arms. All four sit behind
//     finishedDespiteMechanismFailure (wait.go), which consults the contract first.
//   - Wait [OutcomeTimeout] x1 — the run deadline, routed through classifyDeadlineExpiry.
//   - checkLivenessTick [OutcomeDied] x1 — the dead-pane branch, guarded by the allOutputFilesExist
//     directly above it.
//   - checkLivenessTick [errStrandNotTracked] / [errStrandPaneBindingCleared] x1 each — the two
//     mechanism-failure sentinels, each guarded by the allOutputFilesExist directly above it.
//   - checkLivenessTick [Errorf] x1 — the per-tick reed.Status error. NOT itself a finalization: Wait
//     counts it toward maxStatusRetries and only the cap finalizes, behind
//     finishedDespiteMechanismFailure.
//   - classifyStartupWindow [OutcomeDied] x1 — the expired startup window, routed through
//     classifyDeadlineExpiry.
//   - dispositionCandidate [verdictRespawnEligible] x2 — the tracked-and-live-but-terminal record and
//     the confirmed-dead-pane record. Both are reachable only past the function's own top-of-body
//     contract guard, so each is either a run that ALREADY ended or one whose files are not all
//     present; neither needs a further check.
//   - leftoverThenAgeVerdict [verdictRespawnEligible] x2, [verdictError] x1 — the leftover-then-age
//     rule, whose first line is the contract check.
//   - Attach [Errorf] x5 — the three reed-state gates (each now consulting soleFinishedCandidate
//     first, crucible round opus5-high-r7's F1) plus the two multiplicity refusals, which are
//     refusals to CHOOSE rather than verdicts on any run.
//   - normalizeAttachSpec [Errorf] x1, collectAttachCandidates [Errorf] x1, readEventsFrom [Errorf] x3,
//     finalize [Errorf] x1 — argument validation, a scan-root read failure, events-file I/O, and the
//     fork-audit failure. None classifies a run as unfinished; all are listed because an
//     AST scan that hand-excluded them would need a second, unenforceable judgement about which
//     Errorf "counts", and a human looking at a new one costs less than that.
var auditedNegativeVerdictReturns = map[string]int{
	"Attach [Errorf]":                                 5,
	"Wait [Errorf]":                                   4,
	"Wait [OutcomeTimeout]":                           1,
	"checkLivenessTick [Errorf]":                      1,
	"checkLivenessTick [OutcomeDied]":                 1,
	"checkLivenessTick [errStrandNotTracked]":         1,
	"checkLivenessTick [errStrandPaneBindingCleared]": 1,
	"classifyStartupWindow [OutcomeDied]":             1,
	"collectAttachCandidates [Errorf]":                1,
	"dispositionCandidate [verdictRespawnEligible]":   2,
	"finalize [Errorf]":                               1,
	"leftoverThenAgeVerdict [verdictError]":           1,
	"leftoverThenAgeVerdict [verdictRespawnEligible]": 2,
	"normalizeAttachSpec [Errorf]":                    1,
	"readEventsFrom [Errorf]":                         3,
}

// auditedFileContractCallSites is the audited set of allOutputFilesExist call sites as of crucible
// round opus5-high-r7, keyed by enclosing function.
//
// Eight sites: the seven rounds 4-6 left audited (wait.go's pollEventsTick, checkLivenessTick x2,
// classifyDeadlineExpiry, finishedDespiteMechanismFailure; attach.go's dispositionCandidate and
// leftoverThenAgeVerdict) plus soleFinishedCandidate, added by round opus5-high-r7's F1 for Attach's
// three reed-state gates.
var auditedFileContractCallSites = map[string]int{
	"checkLivenessTick":               2,
	"classifyDeadlineExpiry":          1,
	"dispositionCandidate":            1,
	"finishedDespiteMechanismFailure": 1,
	"leftoverThenAgeVerdict":          1,
	"pollEventsTick":                  1,
	"soleFinishedCandidate":           1,
}

// TestCompletionSignal_NegativeVerdictReturnSites trips when a return statement yielding a negative
// run verdict is added to or removed from wait.go or attach.go.
func TestCompletionSignal_NegativeVerdictReturnSites(t *testing.T) {
	got := scanNegativeVerdictReturns(t)
	assertAuditedSet(t, "negative-verdict return site", got, auditedNegativeVerdictReturns,
		"a NEW negative-verdict return must consult allOutputFilesExist over the run's OutputFiles "+
			"before it finalizes — directly, or via classifyDeadlineExpiry / finishedDespiteMechanismFailure / "+
			"soleFinishedCandidate. Confirm the new site does, then add it to auditedNegativeVerdictReturns "+
			"with its own justification line")
}

// TestCompletionSignal_FileContractCallSites trips when an allOutputFilesExist call is added to or
// removed from wait.go or attach.go.
func TestCompletionSignal_FileContractCallSites(t *testing.T) {
	got := scanFileContractCallSites(t)
	assertAuditedSet(t, "allOutputFilesExist call site", got, auditedFileContractCallSites,
		"a file-contract check disappearing from a function that had one is how a negative verdict "+
			"silently starts outranking a finished run again. Confirm the removal is intended, then update "+
			"auditedFileContractCallSites")
}

// assertAuditedSet reports every key whose count differs between got and audited, naming the kind of
// site and appending remedy as the instruction to the editor who tripped it.
func assertAuditedSet(t *testing.T, kind string, got, audited map[string]int, remedy string) {
	t.Helper()

	keys := map[string]bool{}
	for k := range got {
		keys[k] = true
	}
	for k := range audited {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	var drift []string
	for _, k := range sorted {
		if got[k] != audited[k] {
			drift = append(drift, fmt.Sprintf("  %s: found %d, audited %d", k, got[k], audited[k]))
		}
	}
	if len(drift) == 0 {
		return
	}
	t.Errorf("Completion Signal Invariant tripwire: the %s set in %v has drifted from its audited state.\n%s\n\n%s\n\n"+
		"See the Completion Signal Invariant in wait.go's file doc comment and in CONSTRAINTS.md.",
		kind, completionSignalScannedFiles, strings.Join(drift, "\n"), remedy)
}

// scanNegativeVerdictReturns parses the scanned files and returns, per "<function> [<markers>]" key,
// how many return statements in that function yield that exact set of negative markers.
func scanNegativeVerdictReturns(t *testing.T) map[string]int {
	t.Helper()
	found := map[string]int{}
	forEachScannedFunc(t, func(funcName string, body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			markers := map[string]bool{}
			for _, result := range ret.Results {
				collectIdentifiers(result, markers)
			}
			var present []string
			for name := range markers {
				if negativeVerdictMarkers[name] {
					present = append(present, name)
				}
			}
			if len(present) == 0 {
				return true
			}
			sort.Strings(present)
			found[fmt.Sprintf("%s [%s]", funcName, strings.Join(present, ","))]++
			return true
		})
	})
	return found
}

// scanFileContractCallSites parses the scanned files and returns, per enclosing function name, how
// many times allOutputFilesExist is called inside it. The function's own declaration is skipped, so
// the definition never counts as a call site.
func scanFileContractCallSites(t *testing.T) map[string]int {
	t.Helper()
	found := map[string]int{}
	forEachScannedFunc(t, func(funcName string, body *ast.BlockStmt) {
		if funcName == "allOutputFilesExist" {
			return
		}
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "allOutputFilesExist" {
				found[funcName]++
			}
			return true
		})
	})
	return found
}

// forEachScannedFunc parses each file in completionSignalScannedFiles and calls visit once per
// function declaration that has a body, with the declared name (the bare function or method name,
// receiver excluded, since no two of these share one) and that body.
func forEachScannedFunc(t *testing.T, visit func(funcName string, body *ast.BlockStmt)) {
	t.Helper()

	// runtime.Caller, so the scan finds the real package tree regardless of the working directory
	// go test was invoked from — the same resolution seam_enforcement_test.go uses.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	packageDir := filepath.Dir(thisFile)

	for _, name := range completionSignalScannedFiles {
		path := filepath.Join(packageDir, name)
		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range astFile.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			visit(fn.Name.Name, fn.Body)
		}
	}
}

// collectIdentifiers walks expr and records every bare identifier name and every selector's final
// name into into — so both `OutcomeDied` and `fmt.Errorf`'s `Errorf` are seen, without the scan
// needing to know which package qualifier a marker happens to carry.
func collectIdentifiers(expr ast.Expr, into map[string]bool) {
	ast.Inspect(expr, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			into[node.Name] = true
		case *ast.SelectorExpr:
			into[node.Sel.Name] = true
		}
		return true
	})
}
