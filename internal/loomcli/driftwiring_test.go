// driftwiring_test.go checks the request the loomcli Drift seams hand the detached capture child's start.

package loomcli_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCaptureSpawnRequest_StartsInTheAnchoredDirectory covers the directory the capture child is started in for a hub anchored at a subdirectory.
func TestCaptureSpawnRequest_StartsInTheAnchoredDirectory(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	location := &lymcwd.Location{HubPath: filepath.Join(t.TempDir(), "hub"), WorktreeName: "pair", AnchorRel: "backend"}
	request := loomcli.CaptureSpawnRequest(location, driftengine.NewPaths(location.AnchorPath()), "lym")

	if want := filepath.Join(location.HubPath, "pair", "backend"); request.AnchorDir != want {
		t.Errorf("CaptureSpawnRequest() AnchorDir = %q; want the anchored directory %q", request.AnchorDir, want)
	}
	if request.Owner != "pair" || request.Executable != "lym" {
		t.Errorf("CaptureSpawnRequest() = %+v; want owner pair and executable lym", request)
	}
}
