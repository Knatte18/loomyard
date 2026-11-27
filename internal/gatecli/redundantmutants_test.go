// redundantmutants_test.go pins how many memory-capped test runs the mutation check lets go side by side.

package gatecli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestClampParallelToMemory(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const limit = 2048 << 20
	tests := []struct {
		name       string
		parallel   int
		coversTree bool
		headroom   int64
		limited    bool
		want       int
	}{
		{"RoomForEveryRun", 4, true, 10 * limit, true, 4},
		{"TheMemoryLeftHoldsFewerCaps", 4, true, 2*limit + limit/2, true, 2},
		{"LessThanOneCapLeftStillRunsOne", 4, true, limit / 2, true, 1},
		{"ALimitAlreadyPassedStillRunsOne", 4, true, -limit, true, 1},
		{"NoLimitInTheAncestryKeepsTheParallel", 4, true, 0, false, 4},
		{"ACapOnEachProcessAloneRunsOne", 4, false, 10 * limit, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := clampParallelToMemory(tt.parallel, limit, tt.coversTree, tt.headroom, tt.limited); got != tt.want {
				t.Errorf("clampParallelToMemory(%d, %d, %v, %d, %v) = %d; want %d", tt.parallel, limit, tt.coversTree, tt.headroom, tt.limited, got, tt.want)
			}
		})
	}
}
