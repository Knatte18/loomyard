//go:build integration

// records_integration_test.go is the end-to-end check that batten's hand-off keeps the run's records: a child that goes awaiting, is resumed by the operator, reaches done and is torn down leaves its friction notes and drive report reachable from the archive tag on the records origin.
//
// It stays a white-box "package battencli" test for the same reason lifecycle_integration_test.go does: it stubs InnerRun's seams at the field level after a real wire() call.
// The Spawn stub's own commit stands in for the child's transition commit and "lyx loom commit-records"; battencli cannot reach loomcli's unexported commit seam, so the real records pathspec is proved by loomcli's own integration tests, and this test proves the rest of the chain, from a committed records tip to an archive tag on the records origin through batten's teardown.
// No real provider or driver is spawned.

package battencli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// archiveTipHexLen is how many leading hex digits of the records tip the archive tag name carries.
const archiveTipHexLen = 12

func stepAwaitingChildDoneTeardown_ArchivesTheRunRecords(t *testing.T, h *hubforge.Hub) {
	slug := "batten-records"
	hubforge.AddPair(t, h, slug)

	childLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve child location: %v", err)
	}

	spawns := 0
	childDone := false
	var frictionRel, reportRel, tip string

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		if !childDone {
			return shedengine.Status{State: shedengine.StateAwaiting, Error: "awaiting a review decision"}, true, nil
		}
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	c.env.InnerRun.DriverAlive = func(ctx context.Context) (bool, error) { return false, nil }
	c.env.InnerRun.Spawn = func(ctx context.Context) error {
		spawns++
		return nil
	}
	// commitChildRecords stands in for the child's own transition commit after the operator resumes it.
	commitChildRecords := func() error {
		frictionPath := filepath.Join(loomengine.LoomFrictionDir(childLocation), "agent.md")
		reportPath := filepath.Join(shedrun.DriveReportsDir(childLocation, shedrun.SelfRunID), "drive-1.md")
		for path, body := range map[string]string{frictionPath: "friction note\n", reportPath: "drive report\n"} {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return err
			}
		}
		var relErr error
		if frictionRel, relErr = filepath.Rel(childLocation.AnchorPath(), frictionPath); relErr != nil {
			return relErr
		}
		if reportRel, relErr = filepath.Rel(childLocation.AnchorPath(), reportPath); relErr != nil {
			return relErr
		}
		_, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), childLocation,
			[]string{frictionRel, reportRel}, "test: child run records", fabricengine.EnvSyncOptions())
		if err != nil {
			return err
		}
		if !committed {
			t.Errorf("the stubbed child commit recorded nothing")
		}
		out, err := exec.Command("git", "-C", h.PairRecordsSibling(slug), "rev-parse", "HEAD").Output()
		if err != nil {
			return err
		}
		tip = strings.TrimSpace(string(out))
		return nil
	}

	seedEntryStatus(t, c, battenrecipe.NameRunShed, shedengine.StateRunning, []shedengine.HistoryEntry{
		{Producer: battenrecipe.NameWorktreeCreate, Outcome: shedengine.Done},
		{Producer: battenrecipe.NameSeedChild, Outcome: shedengine.Done},
	})
	shed, err := shedbuild.NewShed([]byte(shortPollBattenRecipe), c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("shedbuild.NewShed: %v", err)
	}
	ctx := context.Background()

	// Awaiting: a wait, and batten never resumes the child itself.
	step, err := shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step (awaiting): %v", err)
	}
	if step.Outcome != shedengine.Stuck || spawns != 0 {
		t.Fatalf("awaiting child: outcome %q, spawns %d; want a Stuck wait and no spawn", step.Outcome, spawns)
	}

	// The operator resumes the child, which commits its records and reaches done.
	if err := commitChildRecords(); err != nil {
		t.Fatalf("commit the child's records: %v", err)
	}
	childDone = true

	// Done, driver gone: the row completes with no spawn.
	step, err = shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step (done): %v", err)
	}
	if step.Outcome != shedengine.Done {
		t.Fatalf("done step outcome = %q; want %q", step.Outcome, shedengine.Done)
	}
	if spawns != 0 {
		t.Errorf("spawns after done = %d; want 0", spawns)
	}

	step, err = shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step (Land): %v", err)
	}
	if step.Producer != battenrecipe.NameLand || step.Outcome != shedengine.Done {
		t.Fatalf("land step = producer %q outcome %q reason %q; want %q Done", step.Producer, step.Outcome, step.Reason, battenrecipe.NameLand)
	}

	step, err = shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step (Worktree-Teardown): %v", err)
	}
	if step.Producer != battenrecipe.NameWorktreeTeardown || step.Outcome != shedengine.Done || step.State != shedengine.StateDone {
		t.Errorf("teardown step = producer %q outcome %q state %q; want %q done with the run done", step.Producer, step.Outcome, step.State, battenrecipe.NameWorktreeTeardown)
	}
	if pathExists(h.PairCodeWorktree(slug)) || pathExists(h.PairRecordsSibling(slug)) {
		t.Errorf("the task pair still exists after Worktree-Teardown")
	}

	tag := "archive/" + slug + "/" + tip[:archiveTipHexLen]
	anchorRel := filepath.ToSlash(childLocation.AnchorRel)
	for _, rel := range []string{frictionRel, reportRel} {
		path := filepath.ToSlash(filepath.Join(anchorRel, rel))
		if got := gitShow(t, h.RecordsBare, tag+":"+path); len(got) == 0 {
			t.Errorf("%s:%s is empty on the records origin", tag, path)
		}
	}
}
