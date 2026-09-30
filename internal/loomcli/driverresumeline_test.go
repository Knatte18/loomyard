package loomcli

import (
	"strings"
	"testing"
)

// TestDriverResumeLine_NamesRunIDReportPathBaselineAndBudget asserts the line names the run-id and
// the report path verbatim and tells the driver to take a baseline and reset its budgets.
func TestDriverResumeLine_NamesRunIDReportPathBaselineAndBudget(t *testing.T) {
	runID := "worktree"
	reportPath := "/hub/worktree/.lyx/shed/worktree/drive-report-20260920-120000-cafe.md"

	got := driverResumeLine(runID, reportPath)

	for _, want := range []string{runID, reportPath, "baseline", "budget"} {
		if !strings.Contains(got, want) {
			t.Errorf("driverResumeLine() = %q; want it to contain %q", got, want)
		}
	}
}

// TestDriverResumeLine_SingleLine asserts the line is non-empty and carries no line break, which
// shuttleengine.Runner.Send rejects.
func TestDriverResumeLine_SingleLine(t *testing.T) {
	got := driverResumeLine("worktree", "/hub/report.md")

	if got == "" {
		t.Fatal("driverResumeLine() is empty; want a non-empty line")
	}
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("driverResumeLine() = %q; want no newline or carriage return", got)
	}
}
