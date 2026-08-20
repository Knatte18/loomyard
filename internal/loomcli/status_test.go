// status_test.go is a table over renderStatusLine, pinning its exact rendered line for each shape of
// Activity the composed status file can carry.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestRenderStatusLine covers renderStatusLine's three shapes: an empty last and wait, a populated
// last only, and both populated -- each asserting the exact expected line, since the format is pinned
// rather than left to judgment.
func TestRenderStatusLine(t *testing.T) {
	tests := []struct {
		name string
		st   shedengine.Status
		want string
	}{
		{
			name: "EmptyLastAndWait",
			st: shedengine.Status{
				State:    shedengine.StateRunning,
				Activity: shedengine.Activity{Now: "Preflight", Last: "", Wait: ""},
			},
			want: "loom running | now Preflight",
		},
		{
			name: "LastOnly",
			st: shedengine.Status{
				State:    shedengine.StateRunning,
				Activity: shedengine.Activity{Now: "Discussion-Write", Last: "Preflight → done", Wait: ""},
			},
			want: "loom running | now Discussion-Write | last Preflight → done",
		},
		{
			name: "LastAndWait",
			st: shedengine.Status{
				State:    shedengine.StateBlocked,
				Activity: shedengine.Activity{Now: "Plan-Validate", Last: "Plan-Validate → stuck", Wait: "plan validation failed"},
			},
			want: "loom blocked | now Plan-Validate | last Plan-Validate → stuck | wait plan validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderStatusLine(tt.st); got != tt.want {
				t.Errorf("renderStatusLine(%+v) = %q; want %q", tt.st, got, tt.want)
			}
		})
	}
}
