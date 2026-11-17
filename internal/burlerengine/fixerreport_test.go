// fixerreport_test.go tests the fixer-report frontmatter contract.
// It covers the parse, the check against a review and the gate entry.

package burlerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckFixerReport covers every refusal of ParseFixerReport and CheckFixerReport, each with its way forward.
// It also covers the reports they let through and the reports the engine writes itself.
// It covers the gate entry over an unparseable review too.
func TestCheckFixerReport(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const wayForward = "way forward: rewrite the frontmatter of the fixer-report"
	design := Finding{ID: "F1", Class: ClassDesign}
	scope := Finding{ID: "F2", Class: ClassScope}
	decision := Finding{ID: "D1", Class: ClassDecision}
	frontmatter := func(header string) string { return "---\n" + header + "---\n\n# Fixer report\n" }

	const (
		fixedF1      = "  - id: F1\n    disposition: fixed\n"
		fixedF2      = "  - id: F2\n    disposition: fixed\n"
		disputedHead = "  - id: F1\n    disposition: disputed\n"
		disputedRest = "    ground: premise false\n    rebuttal: the code already checks it\n    evidence:\n      - target.txt:3\n"
		rulingD1     = "  - id: D1\n    disposition: ruling\n    reason: the parent ruled the cache scope\n"
	)

	tests := []struct {
		name     string
		report   string
		findings []Finding
		routed   bool
		// wantErr is a text the refusal holds, besides the way forward; empty means the report passes.
		wantErr string
	}{
		{name: "a complete report passes", report: frontmatter("dispositions:\n" + fixedF1 + fixedF2), findings: []Finding{design, scope}},
		{name: "a review with no findings and empty dispositions passes", report: frontmatter("dispositions: []\n")},
		{name: "a routed round whose decision finding has no entry passes", report: frontmatter("dispositions:\n" + fixedF1), findings: []Finding{design, decision}, routed: true},
		{name: "a routed round with a ruling entry passes", report: frontmatter("dispositions:\n" + fixedF1 + rulingD1), findings: []Finding{design, decision}, routed: true},
		{name: "a dispute with a ground, a rebuttal and evidence passes", report: frontmatter("dispositions:\n" + disputedHead + disputedRest), findings: []Finding{design}},

		{name: "no frontmatter", report: "# Fixer report\n", wantErr: "must open with"},
		{name: "an unclosed frontmatter", report: "---\ndispositions: []\n", wantErr: "closing"},
		{name: "invalid YAML", report: frontmatter("dispositions: [\n"), wantErr: "not valid YAML"},
		{name: "a disposition outside the five", report: frontmatter("dispositions:\n  - id: F1\n    disposition: done\n"), findings: []Finding{design}, wantErr: `"done"`},
		{name: "a misspelled dispositions key", report: frontmatter("dispositons:\n" + fixedF1), findings: []Finding{design}, wantErr: `"dispositons"`},
		{name: "a spawned key", report: frontmatter("spawned: true\ndispositions: []\n"), wantErr: "`spawned` key"},
		{name: "an id the review lacks", report: frontmatter("dispositions:\n" + fixedF1 + "  - id: X9\n    disposition: fixed\n"), findings: []Finding{design}, wantErr: `"X9"`},
		{name: "an id listed twice", report: frontmatter("dispositions:\n" + fixedF1 + fixedF1), findings: []Finding{design}, wantErr: "twice"},
		{name: "a finding with no entry", report: frontmatter("dispositions:\n" + fixedF1), findings: []Finding{design, scope}, wantErr: `no entry for finding "F2"`},
		{name: "an unrouted round missing its decision finding", report: frontmatter("dispositions:\n" + fixedF1), findings: []Finding{design, decision}, wantErr: `no entry for finding "D1"`},
		{name: "a dispute without a legal ground", report: frontmatter("dispositions:\n" + disputedHead + "    ground: taste\n    rebuttal: r\n    evidence:\n      - a.go:1\n"), findings: []Finding{design}, wantErr: `ground "taste"`},
		{name: "a dispute without a rebuttal", report: frontmatter("dispositions:\n" + disputedHead + "    ground: every fix worse\n    evidence:\n      - a.go:1\n"), findings: []Finding{design}, wantErr: "no rebuttal"},
		{name: "a dispute with no evidence", report: frontmatter("dispositions:\n" + disputedHead + "    ground: premise false\n    rebuttal: r\n"), findings: []Finding{design}, wantErr: "no evidence"},
		{name: "evidence without a colon", report: frontmatter("dispositions:\n" + disputedHead + "    ground: premise false\n    rebuttal: r\n    evidence:\n      - a.go\n"), findings: []Finding{design}, wantErr: `"a.go"`},
		{name: "evidence with a zero line", report: frontmatter("dispositions:\n" + disputedHead + "    ground: premise false\n    rebuttal: r\n    evidence:\n      - a.go:0\n"), findings: []Finding{design}, wantErr: `"a.go:0"`},
		{name: "evidence with a non-numeric line", report: frontmatter("dispositions:\n" + disputedHead + "    ground: premise false\n    rebuttal: r\n    evidence:\n      - a.go:x\n"), findings: []Finding{design}, wantErr: `"a.go:x"`},
		{name: "evidence with an empty path", report: frontmatter("dispositions:\n" + disputedHead + "    ground: premise false\n    rebuttal: r\n    evidence:\n      - \":4\"\n"), findings: []Finding{design}, wantErr: `":4"`},
		{name: "fixed-differently without a reason", report: frontmatter("dispositions:\n  - id: F1\n    disposition: fixed-differently\n"), findings: []Finding{design}, wantErr: "no reason"},
		{name: "deferred without a reason", report: frontmatter("dispositions:\n  - id: F1\n    disposition: deferred\n"), findings: []Finding{design}, wantErr: "no reason"},
		{name: "ruling without a reason", report: frontmatter("dispositions:\n  - id: D1\n    disposition: ruling\n"), findings: []Finding{decision}, routed: true, wantErr: "no reason"},
		{name: "a ruling on a finding that is not decision-class", report: frontmatter("dispositions:\n  - id: F1\n    disposition: ruling\n    reason: r\n"), findings: []Finding{design}, routed: true, wantErr: "a ruling"},
		{name: "a ruling on a round that routes no decisions", report: frontmatter("dispositions:\n" + rulingD1), findings: []Finding{decision}, wantErr: "a ruling"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := parseAndCheckFixerReport(tt.report, tt.findings, tt.routed)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ParseFixerReport and CheckFixerReport = %v; want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseFixerReport and CheckFixerReport = nil; want an error holding %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), wayForward) {
				t.Errorf("error = %q; want it to hold %q and %q", err, tt.wantErr, wayForward)
			}
			if tt.name == "a misspelled dispositions key" && !strings.HasSuffix(err.Error(), "with only the dispositions and spawned keys") {
				t.Errorf("error = %q; want it to end with the stray-key way forward", err)
			}
		})
	}

	t.Run("the engine's skipped reports parse as not spawned with no dispositions", func(t *testing.T) {
		t.Parallel()
		for name, content := range map[string]string{"skippedFixerReport": skippedFixerReport, "skippedRoutedFixerReport": skippedRoutedFixerReport} {
			report, err := ParseFixerReport([]byte(content))
			if err != nil {
				t.Fatalf("ParseFixerReport(%s) = %v; want nil", name, err)
			}
			if report.Spawned == nil || *report.Spawned || len(report.Dispositions) != 0 {
				t.Errorf("ParseFixerReport(%s) = %+v; want spawned false and no dispositions", name, report)
			}
		}
	})

	t.Run("the gate names the review's parse error and the report to rewrite", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		reviewPath, reportPath := filepath.Join(dir, "review.md"), filepath.Join(dir, "fixer-report.md")
		if err := os.WriteFile(reviewPath, []byte(malformedReview), 0o644); err != nil {
			t.Fatalf("WriteFile(review) = %v; want nil", err)
		}
		if err := os.WriteFile(reportPath, []byte(frontmatter("dispositions: []\n")), 0o644); err != nil {
			t.Fatalf("WriteFile(report) = %v; want nil", err)
		}

		entry := FixerReportGateEntry(reportPath, reviewPath, false)
		result, err := entry.Gate()

		if err != nil || result.Passed {
			t.Fatalf("gate = %+v, %v; want a failed result and no error", result, err)
		}
		if !strings.Contains(result.Findings, "review file must open with") || !strings.Contains(result.Findings, reportPath) {
			t.Errorf("gate findings = %q; want the review's parse error and the report path", result.Findings)
		}
	})
}

// parseAndCheckFixerReport runs the parse and then the check over a report, returning the first refusal.
func parseAndCheckFixerReport(content string, findings []Finding, routed bool) error {
	report, err := ParseFixerReport([]byte(content))
	if err != nil {
		return err
	}
	return CheckFixerReport(report, findings, routed)
}
