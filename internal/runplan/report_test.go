// report_test.go covers the batch report's optional journal: absent, or carried through a round-trip.
// Tier 0: no git, only t.TempDir().

package runplan_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReport_JournalIsOptional(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		journal string
	}{
		{"Absent", ""},
		{"Present", "built the parser; renamed one helper"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), runplan.ReportFileName(1, "journal"))
			want := &runplan.Report{Status: runplan.ReportStatusOK, HeadSHA: "abc123", Journal: tt.journal}
			if err := runplan.WriteReport(path, want); err != nil {
				t.Fatalf("WriteReport() error = %v; want nil", err)
			}
			got, err := runplan.ParseReport(path)
			if err != nil {
				t.Fatalf("ParseReport() error = %v; want nil", err)
			}
			if got.Journal != tt.journal {
				t.Errorf("ParseReport().Journal = %q; want %q", got.Journal, tt.journal)
			}
		})
	}
}
