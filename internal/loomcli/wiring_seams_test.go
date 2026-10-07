package loomcli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
)

// intentionallyNil maps each shedrecipe.Env field path wire leaves nil to the reason.
// envkit.NilSeams already skips the seams whose nil is a documented default, so only wire's own gaps are listed.
var intentionallyNil = map[string]string{
	"Landing.PushBranch":        "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.RemoteOnlyCommits": "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.OpenFabric":        "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.OpenParentFabric":  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.TaskHead":          "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.Shuttle":           "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"CreateWorktree":            "batten-only seam, filled by battencli",
	"InnerRun.Spawn":            "batten-only seam, filled by battencli",
	"InnerRun.ResolveStatus":    "batten-only seam, filled by battencli",
	"InnerRun.ReadStatus":       "batten-only seam, filled by battencli",
	"InnerRun.ReadDecision":     "batten-only seam, filled by battencli",
	"InnerRun.DriverAlive":      "batten-only seam, filled by battencli",
	"SeedChild.ReadBoardType":   "batten-only seam, filled by battencli",
	"SeedChild.ChildDriver":     "batten-only seam, filled by battencli",
	"SeedChild.WriteSeed":       "batten-only seam, filled by battencli",
	"SeedChild.CommitSeed":      "batten-only seam, filled by battencli",
	"SeedChild.PushSeed":        "batten-only seam, filled by battencli",
	"Teardown.Shutdown":         "batten-only seam, filled by battencli",
	"Teardown.Remove":           "batten-only seam, filled by battencli",
	"PrimeLock.Acquire":         "batten-only seam, filled by battencli",
}

func TestWire_EverySeamFilled(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "pair", ".")

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	nils := envkit.NilSeams(c.env)
	for _, path := range nils {
		if _, ok := intentionallyNil[path]; !ok {
			t.Errorf("c.env.%s is nil after wire(); fill it or list it in intentionallyNil with a reason", path)
		}
	}
	for path := range intentionallyNil {
		if !slices.Contains(nils, path) {
			t.Errorf("intentionallyNil lists c.env.%s but wire() now fills it; remove the entry", path)
		}
	}
}
