package testcover_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// scanSourceHeader and scanSourceHelpers frame the test functions of every row of the scan test.
// The helpers hold a call cycle that reaches the kit and one that does not.
const scanSourceHeader = `package p

import (
	"testing"

	kit "m/internal/testkit/scankit"
	other "m/other"
)
`

const scanSourceHelpers = `
func outer(t *testing.T) { inner(t); kit.Walk(t) }

func inner(t *testing.T) { outer(t) }

func (scanner) Run(t *testing.T) { kit.Walk(t) }

func scanAll(t *testing.T) { kit.Walk(t) }

type scanner struct{}

func (scanner) run(t *testing.T) { scanAll(t) }

func helper() {}

func ping() { pong() }

func pong() { ping() }

var _ = other.Run
`

// cycleEntryPairs is the number of kit-reaching call cycles of the row that holds both entries of each.
const cycleEntryPairs = 16

// cycleEntryTests returns the source of n call cycles that reach the scan kit, each with a test entering at either side.
func cycleEntryTests(n int) string {
	var src strings.Builder
	for i := range n {
		fmt.Fprintf(&src, "func TestKitBehindCycleFromOuter%[1]d(t *testing.T) { outer%[1]d(t) }\n\n", i)
		fmt.Fprintf(&src, "func TestKitBehindCycleFromInner%[1]d(t *testing.T) { inner%[1]d(t) }\n\n", i)
		fmt.Fprintf(&src, "func outer%[1]d(t *testing.T) { inner%[1]d(t); kit.Walk(t) }\n\n", i)
		fmt.Fprintf(&src, "func inner%[1]d(t *testing.T) { outer%[1]d(t) }\n\n", i)
	}
	return src.String()
}

// cycleEntryWant returns the tests cycleEntryTests(n) writes, all of which reach the kit.
func cycleEntryWant(n int) map[string]bool {
	want := map[string]bool{}
	for i := range n {
		want[fmt.Sprintf("TestKitBehindCycleFromOuter%d", i)] = true
		want[fmt.Sprintf("TestKitBehindCycleFromInner%d", i)] = true
	}
	return want
}

func TestScanSourceReaders_MarksTestsReachingTheScanKit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// A walk starts only from a test, so a package holding one cycle test is walked from one known function:
	// each row pins one entry of the cycle.
	tests := []struct {
		name  string
		tests string
		want  map[string]bool
	}{
		{
			name:  "KitBehindCycleEnteredFromOuter",
			tests: "func TestKitBehindCycleFromOuter(t *testing.T) { outer(t) }\n",
			want:  map[string]bool{"TestKitBehindCycleFromOuter": true},
		},
		{
			name:  "KitBehindCycleEnteredFromInner",
			tests: "func TestKitBehindCycleFromInner(t *testing.T) { inner(t) }\n",
			want:  map[string]bool{"TestKitBehindCycleFromInner": true},
		},
		{
			// A package holding both entries of many cycles.
			// A memo that keeps a false answer met under a followed function leaves the inner test unmarked.
			// That happens whenever the outer test is walked first.
			// Each cycle's order is random, so the row catches that regression with probability 1 − 2⁻ⁿ for n cycles.
			name:  "KitBehindManyCyclesEnteredFromBothSides",
			tests: cycleEntryTests(cycleEntryPairs),
			want:  cycleEntryWant(cycleEntryPairs),
		},
		{
			// A cycle with no kit stays unmarked, and an imported call is not read as a method call.
			name: "DirectHelperMethodImportedAndKitlessCycle",
			tests: `func TestDirect(t *testing.T) { kit.Walk(t) }

func TestImportedFunction(t *testing.T) { other.Run(t) }

func TestThroughHelper(t *testing.T) { scanAll(t) }

func TestThroughMethod(t *testing.T) { var s scanner; s.run(t) }

func TestNoScan(t *testing.T) { helper() }

func TestCycle(t *testing.T) { ping() }
`,
			want: map[string]bool{"TestDirect": true, "TestThroughHelper": true, "TestThroughMethod": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src := scanSourceHeader + "\n" + tt.tests + scanSourceHelpers
			if err := os.WriteFile(filepath.Join(dir, "p_test.go"), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := testcover.ScanSourceReaders("m", dir)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ScanSourceReaders = %v; want %v", got, tt.want)
			}
		})
	}
}
