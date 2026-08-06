// digest_test.go covers distill's Report->Digest mapping: OK maps to
// DigestStatusDone and FAILED maps to DigestStatusStuck, HeadSHA and
// Deviations carry straight through, and a large deviation list never
// changes the mapped status (the deviation-list-is-informational Shared
// Decision). Tier 1: no git, pure in-memory construction.

package websterengine

import (
	"fmt"
	"testing"
)

func TestDistill_StatusMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		reportStat string
		want       string
	}{
		{"OKMapsToDone", ReportStatusOK, DigestStatusDone},
		{"FAILEDMapsToStuck", ReportStatusFailed, DigestStatusStuck},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &Report{Status: tt.reportStat, HeadSHA: "abc123"}
			got := distill(r)
			if got.Status != tt.want {
				t.Errorf("distill(%q).Status = %q; want %q", tt.reportStat, got.Status, tt.want)
			}
		})
	}
}

func TestDistill_CarriesHeadSHAAndDeviations(t *testing.T) {
	t.Parallel()

	r := &Report{
		Status:     ReportStatusOK,
		HeadSHA:    "deadbeef",
		Deviations: []string{"internal/foo.go", "docs/bar.md"},
	}

	got := distill(r)

	if got.HeadSHA != r.HeadSHA {
		t.Errorf("distill().HeadSHA = %q; want %q", got.HeadSHA, r.HeadSHA)
	}
	if len(got.Deviations) != len(r.Deviations) {
		t.Fatalf("distill().Deviations = %v; want %v", got.Deviations, r.Deviations)
	}
	for i, d := range r.Deviations {
		if got.Deviations[i] != d {
			t.Errorf("distill().Deviations[%d] = %q; want %q", i, got.Deviations[i], d)
		}
	}
}

func TestDistill_LargeDeviationListNeverChangesStatus(t *testing.T) {
	t.Parallel()

	var many []string
	for i := 0; i < 500; i++ {
		many = append(many, fmt.Sprintf("internal/pkg%d/file.go", i))
	}

	okReport := &Report{Status: ReportStatusOK, HeadSHA: "abc", Deviations: many}
	if got := distill(okReport); got.Status != DigestStatusDone {
		t.Errorf("distill(OK with %d deviations).Status = %q; want %q", len(many), got.Status, DigestStatusDone)
	}

	failedReport := &Report{Status: ReportStatusFailed, HeadSHA: "def", Deviations: many}
	if got := distill(failedReport); got.Status != DigestStatusStuck {
		t.Errorf("distill(FAILED with %d deviations).Status = %q; want %q", len(many), got.Status, DigestStatusStuck)
	}
}
