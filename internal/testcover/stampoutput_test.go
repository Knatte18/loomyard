// stampoutput_test.go pins the line stamping of the collector that times a test run from its RUN line to its result line.
// It feeds fixed chunks to the collector, so no process runs.

package testcover_test

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestStampOutput pins which writes the output collector stamps.
// A line counts once it is complete, whatever the chunking.
func TestStampOutput(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const step = time.Second
	tests := []struct {
		name        string
		chunks      []string
		wantStarted bool
		wantEnded   bool
	}{
		{name: "ARunLineWithoutItsNewlineStampsNothing", chunks: []string{"=== RUN   TestX"}},
		{
			name:   "ARunLineAndAResultLineSplitAcrossWritesStampOnceComplete",
			chunks: []string{"=== RUN   Te", "stX\n", "    --- PASS: TestX/sub (0.00s)\n--- PA", "SS: TestX (0.00s)\n"}, wantStarted: true, wantEnded: true,
		},
		{name: "AnUnfinishedResultLineStampsNoEnd", chunks: []string{"=== RUN   TestX\n--- PASS: TestX (0.00s)"}, wantStarted: true},
		{name: "ASubtestResultLineAloneStampsNoEnd", chunks: []string{"=== RUN   TestX\n", "    --- PASS: TestX/sub (0.00s)\n"}, wantStarted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			started, ended := testcover.StampOutput("TestX", steppingClock(step), tt.chunks...)
			if got := !started.IsZero(); got != tt.wantStarted {
				t.Errorf("StampOutput(%q) stamped the run line = %v; want %v", tt.chunks, got, tt.wantStarted)
			}
			if got := !ended.IsZero(); got != tt.wantEnded {
				t.Errorf("StampOutput(%q) stamped the result line = %v; want %v", tt.chunks, got, tt.wantEnded)
			}
			if tt.wantEnded && ended.Sub(started) != step {
				t.Errorf("StampOutput(%q) span = %s; want %s, one clock step between the two stamps", tt.chunks, ended.Sub(started), step)
			}
		})
	}
}
