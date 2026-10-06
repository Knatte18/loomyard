package loomcli

import (
	"strings"
	"testing"
)

// TestDriverResumeLine asserts the line names the run-id and the report path verbatim, tells the driver to take a baseline and reset its budgets, and is a non-empty single line, which shuttleengine.Runner.Send requires.
//
//testtiming:keep pins the resume line naming the run-id, the report path, the baseline and budget instructions, and being a non-empty single line; the covering test sends the line without asserting its text
func TestDriverResumeLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		runID      string
		reportPath string
		wantInLine []string
	}{
		{
			name:       "names run id, report path, baseline and budget",
			runID:      "worktree",
			reportPath: "/hub/worktree/.lyx/shed/worktree/drive-report-20260920-120000-cafe.md",
			wantInLine: []string{"baseline", "budget"},
		},
		{name: "short inputs", runID: "worktree", reportPath: "/hub/report.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := driverResumeLine(tc.runID, tc.reportPath)

			for _, want := range append([]string{tc.runID, tc.reportPath}, tc.wantInLine...) {
				if !strings.Contains(got, want) {
					t.Errorf("driverResumeLine() = %q; want it to contain %q", got, want)
				}
			}
			if got == "" {
				t.Fatal("driverResumeLine() is empty; want a non-empty line")
			}
			if strings.ContainsAny(got, "\n\r") {
				t.Errorf("driverResumeLine() = %q; want no newline or carriage return", got)
			}
		})
	}
}
