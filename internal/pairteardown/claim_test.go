// claim_test.go pins the board-claim settlement of a removed pair for the entry statuses a finished run leaves behind.

package pairteardown

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSettleClaim_DraftEntryOfFinalizeLessRun asserts an entry a Finalize-less run released as draft is kept whatever batten's run reads,
// so a teardown after such a run neither clears nor marks done the entry the run left for its next release.
func TestSettleClaim_DraftEntryOfFinalizeLessRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	draft := boardengine.StatusDraft
	tests := []struct {
		name         string
		battenSeeded bool
		battenState  string
	}{
		{"BattenNeverSeeded", false, ""},
		{"BattenRunDone", true, "done"},
		{"BattenRunPaused", true, "paused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := settleClaim(&draft, false, tt.battenSeeded, tt.battenState, false); got != claimKeep {
				t.Errorf("settleClaim(draft, landed=false, seeded=%v, %q) = %q; want %q", tt.battenSeeded, tt.battenState, got, claimKeep)
			}
		})
	}
}
