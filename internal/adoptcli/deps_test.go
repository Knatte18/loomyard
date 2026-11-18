// deps_test.go covers the adopt check's liveness reading.

package adoptcli

import (
	"math"
	"os"
	"runtime"
	"testing"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCheckAlive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	self := os.Getpid()
	ticks, _ := proc.StartTime(self)
	tests := []struct {
		name   string
		record CheckRecord
		want   bool
		// needsTicks marks a row that needs a readable start time, which only Linux gives,
		// so no other platform generates a subtest for it.
		needsTicks bool
	}{
		{name: "OwnPidAndTicksIsAlive", record: CheckRecord{PID: self, StartTicks: ticks}, want: true},
		{name: "ReusedPidWithOtherTicksIsDead", record: CheckRecord{PID: self, StartTicks: ticks + "1"}, needsTicks: true},
		{name: "PidAloneWithNoTicksIsAlive", record: CheckRecord{PID: self}, want: true},
		{name: "PidNoProcessHoldsIsDead", record: CheckRecord{PID: math.MaxInt32 - 1, StartTicks: ticks}},
	}
	for _, tt := range tests {
		if tt.needsTicks && runtime.GOOS != "linux" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := checkAlive(tt.record); got != tt.want {
				t.Errorf("checkAlive(%+v) = %v; want %v", tt.record, got, tt.want)
			}
		})
	}
}
