// lensreport_test.go pins the lens report path pattern and the report file name test.

package lensreport_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lensreport"
)

func TestPathPattern(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		record string
		want   string
	}{
		{"PlanLensReview", filepath.Join("a", "plan", "lens-review.md"), filepath.Join("a", "plan", "lens-review.lens-<lens>.md")},
		{"BurlerRoundReview", filepath.Join("runs", "round-2-review.md"), filepath.Join("runs", "round-2-review.lens-<lens>.md")},
		{"NoExtension", filepath.Join("a", "record"), filepath.Join("a", "record.lens-<lens>.md")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := lensreport.PathPattern(tt.record); got != tt.want {
				t.Errorf("PathPattern(%q) = %q; want %q", tt.record, got, tt.want)
			}
		})
	}
}

func TestIsReportName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		record string
		file   string
		want   bool
	}{
		{"ReportOfRecord", "lens-review.md", "lens-review.lens-pattern-fit.md", true},
		{"RecordItself", "lens-review.md", "lens-review.md", false},
		{"EmptyLens", "lens-review.md", "lens-review.lens-.md", false},
		{"OtherExtension", "lens-review.md", "lens-review.lens-generic.txt", false},
		{"OtherRecordsReport", "round-1-review.md", "round-1-fixer-report.lens-generic.md", false},
		{"CardFile", "lens-review.md", "01-card.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := lensreport.IsReportName(tt.record, tt.file); got != tt.want {
				t.Errorf("IsReportName(%q, %q) = %v; want %v", tt.record, tt.file, got, tt.want)
			}
		})
	}
}
