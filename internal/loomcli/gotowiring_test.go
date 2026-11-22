// gotowiring_test.go pins that each path that moves a run by goto hands it the PR-Review reset hook.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestGotoWiring_HandsTheGotoPathsThePRReviewResetHook pins that the goto spec and the relaunch request carry the hook.
// A path with no hook leaves a run moved onto PR-Review keeping its recorded autonomy and session.
func TestGotoWiring_HandsTheGotoPathsThePRReviewResetHook(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	c := &loomCLI{
		location: &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."},
		runID:    "self",
	}
	tests := []struct {
		name string
		hook func(target string) error
	}{
		{"the goto spec", c.specFor("goto").GotoAdmitted},
		{"the relaunch request", c.relaunchRequest("run-1", nil).Admitted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.hook == nil {
				t.Errorf("%s carries no Admitted hook; want the PR-Review reset", tt.name)
			}
		})
	}
}
