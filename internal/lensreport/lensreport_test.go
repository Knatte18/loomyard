// lensreport_test.go pins the lens report path pattern, the report file name test and the outcome read.

package lensreport_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lensreport"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPathPattern(t *testing.T) {
	tiergate.Require(t, 0)
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

func TestPath_NamesOneLensReportTheNameTestAccepts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	record := filepath.Join("runs", "round-2-review.md")
	want := filepath.Join("runs", "round-2-review.lens-code-tests.md")

	got := lensreport.Path(record, "code-tests")

	if got != want {
		t.Errorf("Path(%q, code-tests) = %q; want %q", record, got, want)
	}
	if !lensreport.IsReportName(filepath.Base(record), filepath.Base(got)) {
		t.Errorf("IsReportName(%q, %q) = false; want true", filepath.Base(record), filepath.Base(got))
	}
}

func TestReturned(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"Reported", "```yaml\nlens: code-tests\noutcome: reported\n```\n\nThe final message.\n", true},
		{"NoReport", "```yaml\nlens: code-tests\noutcome: no report\n```\n", false},
		{"NoBlock", "A final message pasted with no block.\n", false},
		{"CRLFLineEndings", "```yaml\r\nlens: code-tests\r\noutcome: reported\r\n```\r\n\r\nThe final message.\r\n", true},
		{"TrailingWhitespace", "```yaml\nlens: code-tests\noutcome: reported  \n```\n", true},
		{"NoBlockWhoseMessageHoldsTheLine", "A final message.\noutcome: reported\n", false},
		{"LineAfterTheBlockCloses", "```yaml\nlens: code-tests\n```\n\noutcome: reported\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "review.lens-code-tests.md")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("WriteFile() = %v; want nil", err)
			}
			if got := lensreport.Returned(path); got != tt.want {
				t.Errorf("Returned(%q) = %v; want %v", tt.content, got, tt.want)
			}
		})
	}

	t.Run("AbsentFile", func(t *testing.T) {
		t.Parallel()
		if lensreport.Returned(filepath.Join(t.TempDir(), "absent.md")) {
			t.Error("Returned(absent file) = true; want false")
		}
	})
}

func TestIsReportName(t *testing.T) {
	tiergate.Require(t, 0)
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
