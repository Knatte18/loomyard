//go:build tier1

// create_integration_test.go runs "lyx beam create" through RunCLIIn over a hubforge hub, from a marked beam's anchor.

package beamcli_test

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/beamcli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestCreate_RunsFromABeamAndReachesTheVerbsOwnRefusal(t *testing.T) {
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})

	var out bytes.Buffer
	beamcli.RunCLIIn(h.PairCodeWorktree("b"), &out, []string{"create", "ghost"})

	envelope.RequireErr(t, out.String(), "not a project entry")
}
