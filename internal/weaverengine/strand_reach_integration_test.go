//go:build !windows

// strand_reach_integration_test.go drives the gate entries of a batch attempt past the reach turn.
// A stand-in `lym` on PATH exits 0 over a real scratch repo and real shell processes.

package weaverengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// TestBatchGateSpec_ReachTurnOnTheBatchEntry pins that the reach turn wraps the batch entry alone.
// It is not parallel: it sets the process's PATH.
func TestBatchGateSpec_ReachTurnOnTheBatchEntry(t *testing.T) {
	tiergate.Require(t, 1)

	worktree, verifyDir, bin := t.TempDir(), t.TempDir(), t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")
	if err := os.WriteFile(filepath.Join(bin, "lym"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	pending := verifytree.ReachPending{
		Note:      "gate test: reach: no mapped test reaches x/x.go#F",
		Functions: map[string]string{"x/x.go#F": "h1"},
	}
	var asked []string
	geom := weaverengine.Geometry{
		PaneCwd:    worktree,
		VerifyDir:  verifyDir,
		GateConfig: func() (gateslot.Config, error) { return testGateConfig(), nil },
		ReachPending: func(gateKey string) (verifytree.ReachPending, bool, error) {
			asked = append(asked, gateKey)
			return pending, true, nil
		},
	}
	spec, err := weaverengine.BatchGateSpec(geom, batchTestCards()[:1], false, nil)
	if err != nil {
		t.Fatalf("BatchGateSpec() error = %v; want none", err)
	}
	if len(spec) != 2 || spec[0].Name != "card 1-one" || spec[1].Name != "batch" {
		t.Fatalf("BatchGateSpec() entries = %+v; want the card entry, then the batch entry", spec)
	}

	if got, err := spec[0].Gate(); err != nil || !got.Passed {
		t.Fatalf("card Gate() = %+v, %v; want a pass", got, err)
	}
	if len(asked) != 0 {
		t.Errorf("the card entry asked for a reach report %v; want it to carry no turn", asked)
	}

	got, err := spec[1].Gate()
	if err != nil || got.Passed || !got.WriterOnly || !strings.Contains(got.Findings, pending.Note) {
		t.Errorf("batch Gate() = %+v, %v; want a writer-only failure carrying the note", got, err)
	}
	if got, err := spec[1].Gate(); err != nil || !got.Passed {
		t.Errorf("batch Gate() on the next arrival = %+v, %v; want a pass", got, err)
	}
	if want := []string{"batch", "batch"}; !slices.Equal(asked, want) {
		t.Errorf("reach reports asked for keys %v; want %v", asked, want)
	}
}
