// verifygatereport.go holds the way forward webster's verify gate names to Merriam.
// It also holds the wrap that records a run-level entry's failure as the shared verify-failure record.

package websterengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The fixers a verify-gate failure goes to.
const (
	// FixerFork is the fixer fork Merriam spawns in its own session, on its own model.
	FixerFork = "fork"
	// FixerCold is the cold fixer strand `lym webster fix-verify` spawns at the recovery role.
	FixerCold = "cold"
)

// verifyGateWayForward returns the one line that names Merriam's way forward from a verify-gate failure.
// The line tells Merriam not to fix the findings itself and, for a fork, to end the turn.
// For FixerCold it names `lym webster fix-verify`, with no fork spawned.
// Merriam runs it as the gate-failure section of its prompt says.
// Otherwise it names one fixer fork, with the prompt file at fixPromptPath.
func verifyGateWayForward(fixer, fixPromptPath string) string {
	if fixer == FixerCold {
		return fmt.Sprintf("Do not fix the findings yourself and spawn no fork; run `%s` as the gate-failure section of your prompt says.", fixVerifyCommand)
	}
	return fmt.Sprintf("Do not fix the findings yourself; spawn one fixer fork as the gate-failure section of your prompt says, with the prompt file at %s; then end your turn.", fixPromptPath)
}

// renderVerifyGateFindings returns the findings text the gate returns to Merriam:
// the shared verify-failure findings, opening with the way forward to the report's fixer.
func renderVerifyGateFindings(r VerifyGateReport, fixPromptPath string) string {
	return runplan.RenderVerifyGateFindings(r, verifyGateWayForward(r.Fixer, fixPromptPath))
}

// verifyGateReprompt returns the renderer for the verify entry's re-prompt line.
// The line opens with the `Gate findings recorded at` text that Merriam's gate-failure section triggers on.
// It then names the findings file and the way forward, which depends on the fixer that notes reports.
func verifyGateReprompt(fixPromptPath string, notes *VerifyGateNotes) func(findingsPath string) string {
	return func(findingsPath string) string {
		return fmt.Sprintf("Gate findings recorded at %s — %s", findingsPath, verifyGateWayForward(notes.Fixer(), fixPromptPath))
	}
}

// wrapRunLevelEntry returns entry with a gate that records a failed result as a verify-gate report at reportPath.
// It also sets the entry's re-prompt to the one the verify entry uses.
// The report carries the entry's name, its findings and the attempt number.
// The returned findings are the rendered report, so Merriam reads the way forward to the fixer beside them.
// A passing, pending, terminal or erroring result is returned as it came, and writes nothing.
// The wrap changes how a failure is reported, never whether the entry passes.
func wrapRunLevelEntry(entry shuttleengine.GateEntry, reportPath, fixPromptPath string, notes *VerifyGateNotes) shuttleengine.GateEntry {
	inner := entry.Gate
	failures := 0
	entry.Gate = func() (shuttleengine.GateResult, error) {
		result, err := inner()
		if err != nil || result.Passed || result.Pending || result.Terminal {
			return result, err
		}
		failures++
		report := VerifyGateReport{
			Attempt:  failures,
			Cap:      entry.Attempts,
			Fixer:    notes.Fixer(),
			Entry:    entry.Name,
			Findings: result.Findings,
		}
		if err := runplan.WriteVerifyGateReport(reportPath, report); err != nil {
			return shuttleengine.GateResult{}, err
		}
		result.Findings = renderVerifyGateFindings(report, fixPromptPath)
		return result, nil
	}
	entry.Reprompt = verifyGateReprompt(fixPromptPath, notes)
	return entry
}
