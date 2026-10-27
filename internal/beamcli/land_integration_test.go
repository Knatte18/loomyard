//go:build tier1

// land_integration_test.go runs "lym beam land" through RunCLIIn over a hubforge hub, from inside a task pair.

package beamcli_test

import (
	"bytes"
	"strings"
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

// TestLand_RunsFromATaskPair asserts a land of the beam b run from inside a task pair reaches the verb's own refusal of a beam with no pawl run, never the refusal of a worktree that is neither the prime nor a beam.
func TestLand_RunsFromATaskPair(t *testing.T) {
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "t", fabricengine.AddOptions{SkipPush: true, Parent: "b"})

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

	var out bytes.Buffer
	beamcli.RunCLIIn(h.PairCodeWorktree("t"), &out, []string{"land", "b"})

	envelope.RequireErr(t, out.String(), "has no pawl run")
	if strings.Contains(out.String(), "neither the prime nor a beam") {
		t.Errorf("land of b from the task pair t = %q; want the verb's own refusal, not the prime-or-beam refusal", out.String())
	}
}
