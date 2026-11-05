// checkresult_test.go — tier 0 test for the verdict `lym drift check` reports, which must agree with the drift gate's.

package driftcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckResultWouldPass covers which reports the drift gate would fail, so `would_pass` names the gate's verdict.
func TestCheckResultWouldPass(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	unexpected := []driftengine.ClassifiedCell{{Cell: driftengine.Cell{Case: "a.txt", Kind: driftengine.CellChanged}}}
	tests := []struct {
		name   string
		report driftengine.CheckReport
		want   bool
	}{
		{"AnUnexpectedCellFails", driftengine.CheckReport{Cells: unexpected}, false},
		{"AnUnexpectedCellBesideARecordsFaultFails", driftengine.CheckReport{RecordsFault: "records differ", Cells: unexpected}, false},
		{"AnUnexpectedCellBesideASealFaultFails", driftengine.CheckReport{SealFault: "seal edited", Cells: unexpected}, false},
		{"ACaptureFaultBesideASealFaultFails", driftengine.CheckReport{SealFault: "seal edited", CaptureFault: "capture failed"}, false},
		{"AnUnexpectedCellBesideABaselineFaultPasses", driftengine.CheckReport{BaselineFault: "baseline edited", Cells: unexpected}, true},
		{"AnUnexpectedCellUnderASkipPasses", driftengine.CheckReport{Skip: "scope off", Cells: unexpected}, true},
		{"ARecordsFaultAlonePasses", driftengine.CheckReport{RecordsFault: "records differ"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := checkResult(tt.report)["would_pass"]; got != tt.want {
				t.Errorf("checkResult(%+v) would_pass = %v; want %v", tt.report, got, tt.want)
			}
		})
	}
}
