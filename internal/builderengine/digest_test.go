// digest_test.go covers Distill's scope/drift matrix: in-scope changes,
// directory-prefix scope coverage, a justified out-of-scope entry,
// unreported drift, the "internal/foo must not cover internal/foobar"
// boundary case, and the dirty flag's straight pass-through.

package builderengine_test

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
)

func TestDistill_ScopeDriftMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		report    *builderengine.Report
		changed   []string
		scope     []string
		wantDrift []string
	}{
		{
			name:    "in-scope changes produce no drift",
			report:  &builderengine.Report{Batch: "01-x", Status: builderengine.ReportStatusDone, Tests: builderengine.ReportTestsGreen},
			changed: []string{"internal/foo/a.go"},
			scope:   []string{"internal/foo/a.go"},
		},
		{
			name:    "directory-prefix scope covers nested files",
			report:  &builderengine.Report{Batch: "01-x", Status: builderengine.ReportStatusDone, Tests: builderengine.ReportTestsGreen},
			changed: []string{"internal/foo/a.go", "internal/foo/sub/b.go"},
			scope:   []string{"internal/foo"},
		},
		{
			name: "justified out-of-scope entry is not drift",
			report: &builderengine.Report{
				Batch: "01-x", Status: builderengine.ReportStatusDone, Tests: builderengine.ReportTestsGreen,
				OutOfScope: []builderengine.OutOfScopeEntry{{Path: "internal/bar/c.go", Why: "needed a shared helper"}},
			},
			changed: []string{"internal/bar/c.go"},
			scope:   []string{"internal/foo"},
		},
		{
			name:      "unreported drift is flagged",
			report:    &builderengine.Report{Batch: "01-x", Status: builderengine.ReportStatusDone, Tests: builderengine.ReportTestsGreen},
			changed:   []string{"internal/bar/c.go"},
			scope:     []string{"internal/foo"},
			wantDrift: []string{"internal/bar/c.go"},
		},
		{
			name:      "boundary: internal/foo must not cover internal/foobar",
			report:    &builderengine.Report{Batch: "01-x", Status: builderengine.ReportStatusDone, Tests: builderengine.ReportTestsGreen},
			changed:   []string{"internal/foobar/x.go"},
			scope:     []string{"internal/foo"},
			wantDrift: []string{"internal/foobar/x.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := builderengine.Distill(tt.report, tt.changed, tt.scope, false)
			if !reflect.DeepEqual(d.DriftUnreported, tt.wantDrift) {
				t.Errorf("DriftUnreported = %v; want %v", d.DriftUnreported, tt.wantDrift)
			}
			if d.FilesChanged != len(tt.changed) {
				t.Errorf("FilesChanged = %d; want %d", d.FilesChanged, len(tt.changed))
			}
			if d.Batch != tt.report.Batch {
				t.Errorf("Batch = %q; want %q", d.Batch, tt.report.Batch)
			}
			if d.Status != tt.report.Status {
				t.Errorf("Status = %q; want %q", d.Status, tt.report.Status)
			}
		})
	}
}

func TestDistill_DirtyPassThrough(t *testing.T) {
	t.Parallel()

	report := &builderengine.Report{Batch: "01-x", Status: builderengine.ReportStatusDone, Tests: builderengine.ReportTestsGreen}

	if d := builderengine.Distill(report, nil, nil, true); !d.Dirty {
		t.Errorf("Distill(..., dirty=true).Dirty = false; want true")
	}
	if d := builderengine.Distill(report, nil, nil, false); d.Dirty {
		t.Errorf("Distill(..., dirty=false).Dirty = true; want false")
	}
}

func TestDistill_StuckReportCarriesStuckReason(t *testing.T) {
	t.Parallel()

	report := &builderengine.Report{
		Batch: "02-y", Status: builderengine.ReportStatusStuck, Tests: builderengine.ReportTestsRed,
		StuckReason: "compile error persists after 2 self-fix attempts",
	}

	d := builderengine.Distill(report, nil, nil, false)
	if d.StuckReason != report.StuckReason {
		t.Errorf("StuckReason = %q; want %q", d.StuckReason, report.StuckReason)
	}
	if d.Status != builderengine.DigestStatusStuck {
		t.Errorf("Status = %q; want %q", d.Status, builderengine.DigestStatusStuck)
	}
}
