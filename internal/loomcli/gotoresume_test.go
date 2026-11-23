// gotoresume_test.go pins that an admitted goto sets aside the interrupted shuttle runs once,
// and that a failed set-aside leaves the goto standing.

package loomcli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestGotoAdmitted_SetsAsideInterruptedRuns pins that the hook sets the runs aside once per goto.
// A failed set-aside never fails the goto, whose move is already written.
func TestGotoAdmitted_SetsAsideInterruptedRuns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		retireErr error
	}{
		{"SetAsideSucceeds", nil},
		{"SetAsideFailsAndTheGotoStands", errors.New("reed config unreadable")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			c := &loomCLI{
				location: &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."},
				runID:    "self",
				retireRuns: func() (int, error) {
					calls++
					return 1, tt.retireErr
				},
			}

			hook := c.gotoAdmitted()
			if hook == nil {
				t.Fatal("gotoAdmitted() = nil; want a hook for a receiver with a location")
			}
			if err := hook("Darn-Write"); err != nil {
				t.Errorf("hook(Darn-Write) error = %v; want nil", err)
			}
			if calls != 1 {
				t.Errorf("set-aside calls = %d; want 1", calls)
			}
		})
	}
}
