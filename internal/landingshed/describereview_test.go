// describereview_test.go — tier0 unit tests for the Describe table's reviewer seat and its two gates.

package landingshed

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDescribeTable_ReviewerSeat(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name       string
		cfg        Config
		wantModel  string
		wantSkip   string
		wantEffort string
		// wantLeadTimeout is the lead's own limit; zero leaves it the table's.
		wantLeadTimeout                   time.Duration
		wantReviewTimeout, wantTableLimit time.Duration
	}{
		{
			name:              "AConfiguredReviewerRunsOnItsOwnModel",
			cfg:               Config{Describe: "claude:sonnet[effort=high]", DescribeTimeoutMin: 30, DescribeReview: "claude:opus[effort=medium]", DescribeReviewTimeoutMin: 12},
			wantModel:         "opus",
			wantEffort:        "medium",
			wantLeadTimeout:   42 * time.Minute,
			wantReviewTimeout: 42 * time.Minute,
			wantTableLimit:    30 * time.Minute,
		},
		{
			name:              "AnEmptyReviewerKeyKeepsTheReviewerFromStarting",
			cfg:               Config{Describe: "claude:sonnet[effort=high]", DescribeTimeoutMin: 30, DescribeReviewTimeoutMin: 12},
			wantModel:         "sonnet",
			wantEffort:        "high",
			wantSkip:          "landing.describe_review is empty, so no reviewer runs",
			wantReviewTimeout: 42 * time.Minute,
			wantTableLimit:    30 * time.Minute,
		},
		{
			name:       "AnUnsetWriterBudgetLeavesBothRunsTheShuttleDefault",
			cfg:        Config{Describe: "claude:sonnet[effort=high]", DescribeReview: "claude:opus[effort=medium]", DescribeReviewTimeoutMin: 12},
			wantModel:  "opus",
			wantEffort: "medium",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := newDescribeInputs(t)
			table, err := DescribeTable(in, tt.cfg, modelspec.Registry{})
			if err != nil {
				t.Fatalf("DescribeTable() = _, %v; want nil error", err)
			}
			if err := table.Validate(in.StencilsDir, ""); err != nil {
				t.Fatalf("table.Validate() = %v; want nil", err)
			}
			requireDescribeLead(t, table, in.DescriptionPath)

			reviewer := table.Seats[1]
			wantReport := "/x/.lym/landing/describe-review/describe-review.md"
			wantMarker := "/x/.lym/landing/describe-review/describe-review.end"
			wantReady := "/x/.lym/landing/describe-review/describe-review.ready"
			if !reviewer.Assistant || reviewer.EndMarker != wantMarker || !slices.Equal(reviewer.Outputs, []string{wantReport}) || reviewer.Values["ready_path"] != wantReady {
				t.Errorf("reviewer assistant/marker/outputs/ready = %v/%q/%v/%q; want true/%q/[%s]/%q", reviewer.Assistant, reviewer.EndMarker, reviewer.Outputs, reviewer.Values["ready_path"], wantMarker, wantReport, wantReady)
			}
			if reviewer.Stencil != "landing-template-describe-review" || reviewer.Timeout != tt.wantReviewTimeout {
				t.Errorf("reviewer stencil/timeout = %q/%v; want landing-template-describe-review and %v", reviewer.Stencil, reviewer.Timeout, tt.wantReviewTimeout)
			}
			if lead := table.Chair(); lead.Timeout != tt.wantLeadTimeout || table.Timeout != tt.wantTableLimit {
				t.Errorf("lead timeout, table timeout = %v, %v; want %v, %v", lead.Timeout, table.Timeout, tt.wantLeadTimeout, tt.wantTableLimit)
			}
			if reviewer.Model != tt.wantModel || reviewer.Effort != tt.wantEffort || reviewer.SkipReason != tt.wantSkip {
				t.Errorf("reviewer model/effort/skip = %q/%q/%q; want %q/%q/%q", reviewer.Model, reviewer.Effort, reviewer.SkipReason, tt.wantModel, tt.wantEffort, tt.wantSkip)
			}
			if !slices.Contains(table.Chair().Inputs, wantReport) {
				t.Errorf("lead inputs = %v; want them to hold the reviewer's report %s", table.Chair().Inputs, wantReport)
			}
			if len(table.Gate) != 1 || table.Gate[0].Name != "Describe-Review" || !table.Gate[0].PassOnCap || table.Gate[0].Final == nil {
				t.Errorf("table gate = %+v; want the one Describe-Review entry, passing on its cap, with a Final closure", table.Gate)
			}
		})
	}
}

func TestDescribeReviewGateEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const findings = "# Describe review: findings\n\n- \"Run lym loom resume.\" names lym.\n"
	released := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		// ending is the end marker's text; empty leaves no marker.
		ending string
		// report is the report's text; empty leaves no report.
		report string
		// markerIsDir and reportIsDir put a directory where the file would stand, so it cannot be read.
		markerIsDir, reportIsDir bool
		// reviewDirIsFile puts a file where the review directory would stand, so nothing in it can be written.
		reviewDirIsFile bool
		// undelivered leaves a failed call's findings unreported as delivered.
		undelivered bool
		// at is the time since the first call at which each call is made; a call past its end is made at the first call's time.
		at []time.Duration
		// rebuild builds the entry again before each call, as a re-attached run does.
		rebuild bool
		// reset resets the table's review before the last call, as a fresh spawn does.
		reset bool
		// want is the verdict of each call in turn: passed, pending, failed or error.
		want []string
	}{
		{name: "AMissingMarkerHoldsTheTurnEnd", want: []string{"pending", "pending"}},
		{name: "AReviewPastItsTimeOfUnbrokenWaitingPasses", at: []time.Duration{0, 89 * time.Second, 90 * time.Second}, want: []string{"pending", "pending", "passed"}},
		{name: "ALongGapBetweenChecksStartsTheTimeOver", at: []time.Duration{0, time.Hour, time.Hour + 89*time.Second}, want: []string{"pending", "pending", "pending"}},
		{name: "UndeliveredFindingsAreOfferedAgain", ending: "done", report: findings, undelivered: true, want: []string{"failed", "failed"}},
		{name: "AnUnwritableReadyRecordIsAnError", reviewDirIsFile: true, want: []string{"error"}},
		{name: "AReviewThatDidNotFinishPasses", ending: "timeout", report: findings, want: []string{"passed"}},
		{name: "AnApprovingReviewPasses", ending: "done", report: "# Describe review: approved\n", want: []string{"passed"}},
		{name: "FindingsGoToTheWriterOnceThenPass", ending: "done", report: findings, want: []string{"failed", "passed"}},
		{name: "AnEntryBuiltAgainDoesNotSendTheFindingsAgain", ending: "done", report: findings, rebuild: true, want: []string{"failed", "passed"}},
		{name: "AResetReviewSendsItsFindingsAgain", ending: "done", report: findings, reset: true, want: []string{"failed", "failed"}},
		{name: "ADoneReviewWithNoReportPasses", ending: "done", want: []string{"passed"}},
		{name: "AnUnreadableEndMarkerIsAnError", markerIsDir: true, want: []string{"error"}},
		{name: "AnUnreadableReportIsAnError", ending: "done", reportIsDir: true, want: []string{"error"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reviewDir := t.TempDir()
			if tt.reviewDirIsFile {
				reviewDir = filepath.Join(reviewDir, "file")
				writeFile(t, reviewDir, "x")
			}
			files := describeReviewFilesIn(reviewDir)
			if tt.ending != "" {
				writeFile(t, files.marker, tt.ending+"\n")
			}
			if tt.report != "" {
				writeFile(t, files.report, tt.report)
			}
			for path, isDir := range map[string]bool{files.marker: tt.markerIsDir, files.report: tt.reportIsDir} {
				if !isDir {
					continue
				}
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("Mkdir(%q) = %v; want nil", path, err)
				}
			}
			now := released
			build := func() shuttleengine.GateEntry {
				return describeReviewGateEntry(files, 90*time.Second, func() time.Time { return now })
			}
			entry := build()
			for i, want := range tt.want {
				if i < len(tt.at) {
					now = released.Add(tt.at[i])
				}
				if tt.rebuild {
					entry = build()
				}
				if tt.reset && i == len(tt.want)-1 {
					table := seatengine.Table{Seats: []seatengine.Seat{{Name: DescribeReviewSeatName, EndMarker: files.marker}}}
					if err := ResetDescribeReview(table); err != nil {
						t.Fatalf("ResetDescribeReview() = %v; want nil", err)
					}
				}
				result, err := entry.Gate()
				if want == "error" {
					if err == nil {
						t.Errorf("call %d: Gate() = %+v, nil; want an error", i, result)
					}
					continue
				}
				if err != nil {
					t.Fatalf("call %d: Gate() = _, %v; want nil error", i, err)
				}
				if got := gateVerdict(result); got != want {
					t.Errorf("call %d: Gate() verdict = %s; want %s (%+v)", i, got, want, result)
				}
				if want == "failed" && (!result.WriterOnly || !strings.HasSuffix(result.Findings, "\n\n"+findings)) {
					t.Errorf("call %d: Gate() = %+v; want a writer-only failure ending in the report", i, result)
				}
				if result.Reprompted != nil && !tt.undelivered {
					result.Reprompted.RecordReprompt()
				}
			}
			if _, err := os.Stat(files.ready); !tt.reviewDirIsFile && err != nil {
				t.Errorf("Stat(%q) = %v; want the record that releases the reviewer", files.ready, err)
			}
			final, err := entry.Final()
			if err != nil || !final.Passed {
				t.Errorf("Final() = %+v, %v; want passed", final, err)
			}
		})
	}
}

func TestResetDescribeReview_AnUnremovableRecordIsAnError(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	files := describeReviewFilesIn(t.TempDir())
	// A directory holding a file cannot be removed by one remove call.
	writeFile(t, filepath.Join(files.ready, "held"), "x")
	table := seatengine.Table{Seats: []seatengine.Seat{{Name: DescribeReviewSeatName, EndMarker: files.marker}}}
	if err := ResetDescribeReview(table); err == nil {
		t.Error("ResetDescribeReview() = nil; want an error naming the record it could not remove")
	}
}

func TestDescribeReportGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// report is the report's text; empty leaves no report.
		report     string
		wantPassed bool
		wantErr    bool
	}{
		{name: "Approved", report: "\n# Describe review: approved\n", wantPassed: true},
		{name: "Findings", report: "# Describe review: findings\n\n- one\n", wantPassed: true},
		{name: "NoVerdictLine", report: "# Review\n\n- one\n"},
		{name: "NoReport", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			report := filepath.Join(t.TempDir(), "describe-review.md")
			if tt.report != "" {
				writeFile(t, report, tt.report)
			}
			result, err := describeReportGate(report)()
			if (err != nil) != tt.wantErr || result.Passed != tt.wantPassed {
				t.Errorf("describeReportGate()() = %+v, %v; want passed %v, error %v", result, err, tt.wantPassed, tt.wantErr)
			}
		})
	}
}

// gateVerdict names a gate result's verdict: passed, pending or failed.
func gateVerdict(result shuttleengine.GateResult) string {
	switch {
	case result.Passed:
		return "passed"
	case result.Pending:
		return "pending"
	}
	return "failed"
}

// writeFile writes content at path, creating its directory.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}
