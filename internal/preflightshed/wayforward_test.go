// wayforward_test.go covers wayForward's per-check mapping over in-memory Reports.
// The rows reached from a real hub (a dirty warp, a weft off its paired branch) belong to the integration suite;
// this file stays offline.

package preflightshed

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/preflight"
)

func TestWayForward(t *testing.T) {
	for _, tt := range []struct {
		name    string
		failure preflight.Failure
		want    string
	}{
		{"WorktreeClean", preflight.Failure{Check: preflight.CheckWorktreeClean, Reason: "dirty"}, "lyx fabric commit"},
		{"FabricSync", preflight.Failure{Check: preflight.CheckFabricSync, Reason: "off branch"}, "lyx fabric checkout"},
		{"FabricReady", preflight.Failure{Check: preflight.CheckFabricReady, Reason: "no weft"}, "lyx fabric reconcile"},
		{"Junction", preflight.Failure{Check: preflight.CheckJunction, Reason: "broken link"}, "lyx fabric reconcile"},
		{"JunctionConfigLoad", preflight.Failure{Check: preflight.CheckJunction, Reason: "junction check unavailable: cannot load fabric.yaml: bad"}, ""},
		{"Geometry", preflight.Failure{Check: preflight.CheckGeometry, Reason: "no repo"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var r preflight.Report
			r.AddFailure(tt.failure.Check, tt.failure.Reason)
			got := wayForward(r)
			if tt.want == "" {
				if got != "" {
					t.Errorf("wayForward() = %q; want none", got)
				}
				return
			}
			if !strings.HasPrefix(got, "; way forward: ") || !strings.Contains(got, tt.want) || !strings.Contains(got, "re-step") {
				t.Errorf("wayForward() = %q; want a trailing way forward naming %q", got, tt.want)
			}
		})
	}

	var both preflight.Report
	both.AddFailure(preflight.CheckFabricReady, "a")
	both.AddFailure(preflight.CheckJunction, "b")
	if got := wayForward(both); strings.Count(got, "lyx fabric reconcile") != 1 {
		t.Errorf("wayForward(ready+junction) = %q; want reconcile named once", got)
	}
}
