// remotecontrol_integration_test.go proves over a real hub the remote session's name of an unlisted orch strand.
// The name forms from the worktree's fixed strand name.

package orchcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLaunchRemoteControlName_UnlistedStrandTakesTheFixedStrandName pins the name a launch passes.
// Reed lists no orch strand in this case.
func TestLaunchRemoteControlName_UnlistedStrandTakesTheFixedStrandName(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	c := newTestCLI(t, &fakeStrands{})
	c.cfg.RemoteControl, c.cfg.RemoteControlName = true, remoteControlTemplate
	c.machineName = func() (string, error) { return "neve", nil }
	c.location = h.Location

	if got, want := c.launchRemoteControlName(reedengine.StrandStatus{}, false), "neve::"+hubforge.TestShortname+":orch"; got != want {
		t.Errorf("launchRemoteControlName() = %q; want %q", got, want)
	}
}
