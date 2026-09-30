//go:build integration

// records_integration_test.go is the end-to-end check that batten's hand-off keeps the run's
// records: a child that goes awaiting, is approved, is resumed by batten, reaches done and is torn
// down leaves its friction notes and drive report reachable from the archive tag on the weft origin.
//
// It stays a white-box "package battencli" test for the same reason lifecycle_integration_test.go
// does: it stubs InnerRun's seams at the field level after a real wire() call.
// The Spawn stub's own commit stands in for the child's transition commit and "lyx loom
// commit-records"; battencli cannot reach loomcli's unexported commit seam, so the real records
// pathspec is proved by loomcli's own integration tests, and this test proves the rest of the
// chain, from a committed weft tip to an archive tag on the weft origin through batten's teardown.
// No real provider or driver is spawned.

package battencli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// archiveTipHexLen is how many leading hex digits of the weft tip the archive tag name carries.
const archiveTipHexLen = 12

func TestBattenIntegration_AwaitingApprovalResumeDoneTeardown_ArchivesTheRunRecords(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	slug := "batten-records"
	hubforge.AddPair(t, h, slug)

	childLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve child location: %v", err)
	}

	spawns := 0
	var frictionRel, reportRel, tip string

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		if spawns == 0 {
			return shedengine.Status{State: shedengine.StateAwaiting, Error: "awaiting pull-request approval"}, true, nil
		}
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	c.env.InnerRun.Sleep = func(ctx context.Context, d time.Duration) {}
	c.env.InnerRun.DriverAlive = func(ctx context.Context) (bool, error) { return false, nil }
	c.env.InnerRun.Spawn = func(ctx context.Context) error {
		spawns++
		if spawns > 1 {
			return nil
		}
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
		out, err := exec.Command("git", "-C", h.PairWeftSibling(slug), "rev-parse", "HEAD").Output()
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

	// Awaiting with no approval: a wait, no spawn.
	step, err := shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step (awaiting, no approval): %v", err)
	}
	if step.Outcome != shedengine.Stuck || spawns != 0 {
		t.Fatalf("awaiting without an approval: outcome %q, spawns %d; want a Stuck wait and no spawn", step.Outcome, spawns)
	}

	if err := landingshed.WriteApproval(loomengine.LoomApprovalPath(childLocation), landingshed.Approval{
		PRNumber: 7, HeadSHA: "abc123", ApprovedAt: "2026-01-02T03:04:05Z",
	}); err != nil {
		t.Fatalf("write approval: %v", err)
	}

	// The approval resumes the child once.
	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (approved): %v", err)
	}
	if spawns != 1 {
		t.Fatalf("spawns after the approval = %d; want 1", spawns)
	}

	// Done, driver gone: the row completes, and the same approval never spawns again.
	step, err = shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step (done): %v", err)
	}
	if step.Outcome != shedengine.Done {
		t.Fatalf("done step outcome = %q; want %q", step.Outcome, shedengine.Done)
	}
	if spawns != 1 {
		t.Errorf("spawns after done = %d; want 1 (the same approval must not spawn again)", spawns)
	}

	step, err = shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step (Worktree-Teardown): %v", err)
	}
	if step.Producer != battenrecipe.NameWorktreeTeardown || step.Outcome != shedengine.Done || step.State != shedengine.StateDone {
		t.Errorf("teardown step = producer %q outcome %q state %q; want %q done with the run done", step.Producer, step.Outcome, step.State, battenrecipe.NameWorktreeTeardown)
	}
	if pathExists(h.PairWarpWorktree(slug)) || pathExists(h.PairWeftSibling(slug)) {
		t.Errorf("the task pair still exists after Worktree-Teardown")
	}

	tag := "archive/" + slug + "/" + tip[:archiveTipHexLen]
	anchorRel := filepath.ToSlash(childLocation.AnchorRel)
	for _, rel := range []string{frictionRel, reportRel} {
		path := filepath.ToSlash(filepath.Join(anchorRel, rel))
		if got := gitShow(t, h.WeftBare, tag+":"+path); len(got) == 0 {
			t.Errorf("%s:%s is empty on the weft origin", tag, path)
		}
	}
}
