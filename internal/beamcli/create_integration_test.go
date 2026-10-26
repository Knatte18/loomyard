//go:build tier1

// create_integration_test.go runs "lyx beam create" through RunCLIIn over a hubforge hub: from the parent beam of the entry it names, where it reaches the verb's own refusal, and from the prime, which it refuses naming the parent beam.

package beamcli_test

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/beamcli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestCreate_RunsFromABeamAndReachesTheVerbsOwnRefusal asserts the create of a task under the beam b, a refusal of the verb itself, is reached from b and refused from the prime, which names b as where to run it.
func TestCreate_RunsFromABeamAndReachesTheVerbsOwnRefusal(t *testing.T) {
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})

	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	board := boardengine.New(cfg)
	for _, entry := range []map[string]any{
		{"slug": "b", "kind": "project", "title": "b", "labels": []string{"bug"}},
		{"slug": "t", "kind": "task", "project": "b", "title": "t", "labels": []string{"bug"}},
	} {
		if _, err := board.UpsertTask(entry); err != nil {
			t.Fatalf("seed board entry %v: %v", entry["slug"], err)
		}
	}
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machine name: %v", err)
	}
	if _, err := boardengine.SetOwners(h.BoardDir(), boardengine.OwnerRequest{Machine: machine, Slugs: []string{"b"}, Beam: true}); err != nil {
		t.Fatalf("flag b as a beam: %v", err)
	}

	var fromBeam bytes.Buffer
	beamcli.RunCLIIn(h.PairCodeWorktree("b"), &fromBeam, []string{"create", "t"})
	envelope.RequireErr(t, fromBeam.String(), "is a task")

	var fromPrime bytes.Buffer
	beamcli.RunCLIIn(h.PrimeWorktree(), &fromPrime, []string{"create", "t"})
	envelope.RequireErr(t, fromPrime.String(), "is created from its parent's worktree")
}
