// completionsignal_enforcement_test.go is a tripwire for the Completion Signal Invariant.
// The invariant covers every path in this package that finalizes a negative answer to "did this run finish":
// such a path must consult allOutputFilesExist over the run's OutputFiles first.
//
// The tripwire forces a human to look; it does not prove the invariant holds.
// A diff that adds a negative-verdict return or deletes a file-contract check fails a test naming what moved.
// The reader then confirms the new site consults the check and updates the audited set below.
//
// Two assertions cover different mutations, and neither subsumes the other.
//   - TestCompletionSignal_NegativeVerdictReturnSites catches an unguarded new exit.
//     A count of allOutputFilesExist calls cannot catch one, since an unguarded return adds no such call.
//   - TestCompletionSignal_FileContractCallSites catches a guard deleted from an exit that already has one,
//     which leaves the return-site set unchanged.
//
// Both scans are AST-based, so a mention in a doc comment or a string literal cannot trip them.

package shuttleengine

import (
	"fmt"
	"go/ast"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// completionSignalScannedFiles are the files that own every run-outcome verdict in this package:
// wait.go finalizes a live run's outcome, attach.go decides whether a persisted one is reconstructed
// or respawned over, and stop.go records a stop over a run that has not finished.
// A future file that grows a third kind of verdict belongs in this list, and the failure message says so.
var completionSignalScannedFiles = []string{"wait.go", "attach.go", "stop.go", "apierrorretry.go"}

// negativeVerdictMarkers are the identifiers whose appearance anywhere in a return statement's
// results makes that return a negative answer to "did this run finish".
//
// The invariant names four: OutcomeDied, OutcomeTimeout, a mechanism-failure error, and verdictRespawnEligible.
// Three more are how two of those four are actually spelled at a return.
// Errorf is the spelling of the mechanism-failure errors.
// The two package-level sentinels checkLivenessTick returns, when reed's bookkeeping is gone, are also markers.
// verdictError is the third disposition of the three-way choice that verdictRespawnEligible belongs to.
// A new return of it is worth a human look too.
//
// OutcomeDone is absent: it is the positive answer and abandons no run.
var negativeVerdictMarkers = map[string]bool{
	"OutcomeDied":                 true,
	"OutcomeTimeout":              true,
	"OutcomeAPIError":             true,
	"verdictRespawnEligible":      true,
	"verdictError":                true,
	"runOutcomeStopped":           true,
	"errStrandNotTracked":         true,
	"errStrandPaneBindingCleared": true,
	"Errorf":                      true,
}

// auditedNegativeVerdictReturns is the audited set of return sites that give a negative verdict.
// It is keyed by "<function> [<markers>]", with the number of such returns in that function as the value.
//
// The key names the enclosing function and the marker set, not file:line.
// Line numbers shift with every edit above them, and a test that fails on a shift gets deleted.
// A key changes only when a return is added or removed, or when it yields a different negative value.
//
// Each entry carries its justification, because a bare count cannot be checked by a future reader:
//
//   - Wait [Errorf] — the events-unreadable cap and the three status-cap arms, all behind
//     finishedDespiteMechanismFailure, which consults the contract first.
//   - handleGatedBoundary [Errorf] — the gate-evaluation error returned when run.evaluateGate()
//     fails at a turn boundary. It is an infrastructure fault that never burns an attempt and never
//     reaches the LLM, so it needs no file-contract consultation of its own. The run already reached
//     a positive OutcomeDone, and the gate runs strictly after it, at a turn boundary with no
//     in-progress turn to interrupt.
//   - Wait [OutcomeTimeout] — the run deadline, routed through classifyDeadlineExpiry.
//   - checkLivenessTick [OutcomeDied] — the dead-pane branch, the gone-session branch, and the untracked
//     strand of a handle that Stop marked.
//     Each is guarded by the allOutputFilesExist call directly above it.
//   - checkLivenessTick [errStrandNotTracked] and [errStrandPaneBindingCleared] — the two
//     mechanism-failure sentinels, each guarded by the allOutputFilesExist call directly above it.
//   - checkLivenessTick [Errorf] — the per-tick reed.Status error. It is not itself a finalization:
//     Wait counts it toward maxStatusRetries, and only the cap finalizes, behind
//     finishedDespiteMechanismFailure.
//   - classifyStartupWindow [OutcomeDied] — the expired startup window, routed through classifyDeadlineExpiry.
//   - dispositionCandidate [verdictRespawnEligible] — the tracked-and-live-but-terminal record and the
//     confirmed-dead-pane record. Both are reachable only past the function's top-of-body contract guard,
//     so each is either a run that already ended or one whose files are not all present;
//     neither needs a further check.
//   - leftoverThenAgeVerdict [verdictRespawnEligible] and [verdictError] — the leftover-then-age rule,
//     whose first line is the contract check. The terminal-Outcome check, which returns
//     respawn-eligible, sits after the file-contract check, so it is reached only when the output
//     files are not all present.
//   - stopOutcome [runOutcomeStopped] — the one site that chooses the stopped outcome, behind the
//     allOutputFilesExist call that opens the function.
//   - Stop [Errorf] — the failed record write, returned with the strand untouched. It is a refusal to
//     stop, not a verdict on whether the run finished; the write itself goes through stopOutcome.
//   - stopRecorded [Errorf] — the unreadable record and the failed record write, both returned before
//     any strand removal. They are refusals to stop, with the write behind stopOutcome.
//   - removeStrandIfPresent [Errorf] — the failed reed probe and the failed removal, after the record was
//     written. They are infrastructure faults of the removal, not verdicts on any run.
//   - attach [Errorf] — the returns in attach's body, which Attach, AttachGated and AttachIfLive share.
//     Three are the reed-state gates, each consulting soleFinishedCandidate first;
//     two are multiplicity refusals, which refuse to choose a candidate rather than judge any run.
//   - removeSupersededStrands [Errorf]: the failed removal of a respawn-eligible candidate's strand.
//     It is reached only past the zero-attachable answer, and it refuses to start a run beside a live strand.
//     It is not a verdict on whether any run finished.
//   - normalizeAttachSpec [Errorf], collectAttachCandidates [Errorf], readEventsFrom [Errorf]: argument validation,
//     a scan-root read failure, and events-file I/O. None classifies a run as unfinished.
//     All are listed: a scan that hand-excluded them would need an unenforceable judgement about which Errorf counts,
//     and reviewing a new site by hand costs less.
//   - finalize [Errorf]: sites that each fire after the run already reached a positive verdict.
//     None is a negative verdict on whether the run finished; each is an infrastructure fault surfacing after the fact.
//     They are the fork-audit failure; the refusal to finish a fork-authorized done run whose engine has no fork audit;
//     and the error finalize returns when run.evaluateGate() fails for an OutcomeDone it is about to finalize.
//   - awaitStartup [Errorf,OutcomeDied] — the three retry-cap arms, each the status-cap teardown
//     behind the direct allOutputFilesExist check.
//   - awaitStartup [OutcomeDied] — the not-ready answer of checkLivenessTick and the tick-cap exhaustion
//     fallback, each reached only past a direct or upstream allOutputFilesExist check.
//   - awaitStartup [OutcomeTimeout] — the run-deadline exit, reached only after classifyDeadlineExpiry
//     has already answered not-done.
//   - abandonStartup [Errorf] — its own ErrNotStarted-wrapping return, the not-ready teardown's single
//     finalization site: every caller reaches it only past one of awaitStartup's guarded checks.
var auditedNegativeVerdictReturns = map[string]int{
	"attach [Errorf]":                                 5,
	"removeSupersededStrands [Errorf]":                1,
	"abandonStartup [Errorf]":                         1,
	"awaitStartup [Errorf,OutcomeDied]":               3,
	"awaitStartup [OutcomeDied]":                      2,
	"awaitStartup [OutcomeTimeout]":                   1,
	"Wait [Errorf]":                                   4,
	"handleGatedBoundary [Errorf]":                    1,
	"Wait [OutcomeTimeout]":                           1,
	"checkLivenessTick [Errorf]":                      1,
	"checkLivenessTick [OutcomeDied]":                 3,
	"checkLivenessTick [errStrandNotTracked]":         1,
	"checkLivenessTick [errStrandPaneBindingCleared]": 1,
	"classifyStartupWindow [OutcomeDied]":             1,
	"collectAttachCandidates [Errorf]":                1,
	"dispositionCandidate [verdictRespawnEligible]":   2,
	"finalize [Errorf]":                               3,
	"leftoverThenAgeVerdict [verdictError]":           1,
	"leftoverThenAgeVerdict [verdictRespawnEligible]": 3,
	"normalizeAttachSpec [Errorf]":                    1,
	"readEventsFrom [Errorf]":                         3,
	"removeStrandIfPresent [Errorf]":                  2,
	"Stop [Errorf]":                                   1,
	"stopOutcome [runOutcomeStopped]":                 1,
	"stopRecorded [Errorf]":                           2,
}

// auditedFileContractCallSites is the audited set of allOutputFilesExist call sites, keyed by enclosing function.
//
// The sites are:
//   - in wait.go: pollEventsTick, classifyDeadlineExpiry, finishedDespiteMechanismFailure;
//   - checkLivenessTick, two of its three sites;
//   - in attach.go: dispositionCandidate and leftoverThenAgeVerdict;
//   - soleFinishedCandidate, which the three reed-state gates consult first;
//   - awaitStartup (x2: its retry-cap guard and its tick-cap answer);
//   - expiredTurnEnd: a waiting turn end counts as a Done only after the files are consulted;
//     otherwise it keeps waiting.
//   - pollEventsTick, second site: an ungated waiting turn end with nothing outstanding counts as a Done;
//     the site consults the files before it does so.
//   - pollEventsTick, third site: a gated waiting turn end on a non-awaited shell waits while an output file is missing;
//     otherwise the Done goes to expiredTurnEnd, which consults the files.
//   - stopOutcome, whose stopped answer consults the files first.
//
// checkLivenessTick holds a third site, its gone-session branch, which consults the files before answering died.
// attach holds a site that skips the requirements check for a run whose files are all on disk.
// The skip is safe because harvesting such a run drives no further turn.
var auditedFileContractCallSites = map[string]int{
	"attach":                          1,
	"awaitStartup":                    2,
	"checkLivenessTick":               3,
	"classifyDeadlineExpiry":          1,
	"dispositionCandidate":            1,
	"expiredTurnEnd":                  1,
	"finishedDespiteMechanismFailure": 1,
	"leftoverThenAgeVerdict":          1,
	"pollEventsTick":                  3,
	"soleFinishedCandidate":           1,
	"stopOutcome":                     1,
}

// TestCompletionSignal_NegativeVerdictReturnSites trips when a negative-verdict return is added or removed.
// The scan covers the return sites in wait.go and attach.go.
//
//testtiming:keep scans wait.go and attach.go for the negative-verdict return sites, catching an unguarded new exit, which no other test reads
func TestCompletionSignal_NegativeVerdictReturnSites(t *testing.T) {
	tiergate.Require(t, 0)
	got := scanNegativeVerdictReturns(t)
	assertAuditedSet(t, "negative-verdict return site", got, auditedNegativeVerdictReturns,
		"a NEW negative-verdict return must consult allOutputFilesExist over the run's OutputFiles "+
			"before it finalizes — directly, or via classifyDeadlineExpiry / finishedDespiteMechanismFailure / "+
			"soleFinishedCandidate. Confirm the new site does, then add it to auditedNegativeVerdictReturns "+
			"with its own justification line")
}

// TestCompletionSignal_FileContractCallSites trips when an allOutputFilesExist call is added or removed.
// The scan covers the call sites in wait.go and attach.go.
//
//testtiming:keep scans the allOutputFilesExist call sites, catching a guard deleted from an exit that already has one, which the return-site scan cannot see
func TestCompletionSignal_FileContractCallSites(t *testing.T) {
	tiergate.Require(t, 0)
	got := scanFileContractCallSites(t)
	assertAuditedSet(t, "allOutputFilesExist call site", got, auditedFileContractCallSites,
		"a file-contract check disappearing from a function that had one is how a negative verdict "+
			"silently starts outranking a finished run again. Confirm the removal is intended, then update "+
			"auditedFileContractCallSites")
}

// assertAuditedSet reports every key whose count differs between got and audited.
// It names the site kind in the failure and appends remedy as the instruction to the editor who tripped it.
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
		"See the Completion Signal Invariant in wait.go's file doc comment and in PATTERN-shuttle-provider-seam.",
		kind, completionSignalScannedFiles, strings.Join(drift, "\n"), remedy)
}

// scanNegativeVerdictReturns parses the scanned files and returns a count per "<function> [<markers>]" key.
// Each count is the number of return statements in that function that yield exactly that set of negative markers.
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

// scanFileContractCallSites parses the scanned files and returns a count per enclosing function name.
// Each count is the number of allOutputFilesExist calls inside that function.
// The function's own declaration is skipped, so the definition never counts as a call site.
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

// forEachScannedFunc parses each file in completionSignalScannedFiles.
// It calls visit once per function declaration that has a body, with the bare function or method name and that body.
// The receiver is left out, since no two of these share one.
func forEachScannedFunc(t *testing.T, visit func(funcName string, body *ast.BlockStmt)) {
	t.Helper()

	wanted := map[string]bool{}
	for _, name := range completionSignalScannedFiles {
		wanted["internal/shuttleengine/"+name] = true
	}

	parsed := 0
	scankit.Walk(t, scankit.Options{Roots: []string{"internal/shuttleengine"}, Shallow: true}, func(f *scankit.File) {
		if !wanted[f.Rel] {
			return
		}
		parsed++
		for _, decl := range f.AST(t, 0).Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			visit(fn.Name.Name, fn.Body)
		}
	})
	scankit.RequireFloor(t, parsed, len(completionSignalScannedFiles), "completion-signal scan")
}

// collectIdentifiers walks expr and records every bare identifier name and every selector's final name in into.
// Both `OutcomeDied` and `fmt.Errorf`'s `Errorf` are seen.
// The scan need not know which package qualifier a marker carries.
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
