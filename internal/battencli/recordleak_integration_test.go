//go:build integration

// recordleak_integration_test.go pins, end to end through batten's own rows, that a task pair no longer inherits the batten run records prime has committed.
// It stays a white-box "package battencli" test on a hubforge hub, for the reasons lifecycle_integration_test.go's header gives.

package battencli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// commitPrimeBattenRecords writes and commits, on prime's own pair, a batten seed and status for slug plus a batten seed for otherSlug, the state a finished or running prime-side batten run leaves.
func commitPrimeBattenRecords(t *testing.T, h *hubforge.Hub, slug, otherSlug string) {
	t.Helper()
	seed := shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}
	for _, id := range []string{slug, otherSlug} {
		if err := shedrun.WriteSeed(h.Location, id, seed); err != nil {
			t.Fatalf("write prime batten seed %q: %v", id, err)
		}
	}
	statusRel := shedrun.StatusRel(h.Location, slug)
	body, err := json.Marshal(shedengine.Status{
		CurrentProducer: battenrecipe.NameRunShed,
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{},
	})
	if err != nil {
		t.Fatalf("marshal prime status: %v", err)
	}
	statusPath := filepath.Join(h.Location.AnchorPath(), statusRel)
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		t.Fatalf("mkdir prime status dir: %v", err)
	}
	if err := os.WriteFile(statusPath, body, 0o644); err != nil {
		t.Fatalf("write prime status: %v", err)
	}
	rels := []string{
		shedrun.SeedRel(h.Location, slug),
		shedrun.SeedRel(h.Location, otherSlug),
		statusRel,
	}
	_, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), h.Location,
		rels, "test: prime batten records", fabricengine.EnvSyncOptions())
	if err != nil {
		t.Fatalf("commit prime batten records: %v", err)
	}
	if !committed {
		t.Fatalf("the prime batten records were not committed")
	}
}

// TestBattenIntegration_SeedChild_IgnoresPrimesCommittedBattenRecords reproduces the reported "task worktree already seeded with a disagreeing seed" refusal:
// prime tracks a batten seed for the child's own slug, and the child must still get its own loom seed and none of prime's other runs.
func TestBattenIntegration_SeedChild_IgnoresPrimesCommittedBattenRecords(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	slug := "batten-leak-seed"
	otherSlug := "batten-leak-other"
	seedBoardTask(t, h, slug, "loom")
	commitPrimeBattenRecords(t, h, slug, otherSlug)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	seedEntryStatus(t, c, battenrecipe.NameWorktreeCreate, shedengine.StateRunning, nil)

	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}
	ctx := context.Background()
	create, err := shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step (Worktree-Create): %v", err)
	}
	if create.Outcome != shedengine.Done {
		t.Fatalf("Worktree-Create outcome = %q (%s); want done", create.Outcome, create.Reason)
	}
	step, err := shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step (Seed-Child): %v", err)
	}
	if step.Outcome != shedengine.Done {
		t.Fatalf("Seed-Child outcome = %q (%s); want done", step.Outcome, step.Reason)
	}

	childLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve child location: %v", err)
	}
	seed, found, err := shedrun.ReadSeed(childLocation, shedrun.SelfRunID)
	if err != nil || !found {
		t.Fatalf("read child seed: found=%v err=%v", found, err)
	}
	if seed.Recipe != "loom" {
		t.Errorf("child seed recipe = %q; want loom", seed.Recipe)
	}
	if pathExists(shedrun.RunDir(childLocation, otherSlug)) {
		t.Errorf("child carries prime's run directory for %q", otherSlug)
	}
}

// TestBattenIntegration_RealReadStatus_IgnoresPrimesCommittedStatusForTheSameSlug holds the unstubbed status read:
// prime tracks a batten status for the child's slug, and the child must report its own status as simply absent.
func TestBattenIntegration_RealReadStatus_IgnoresPrimesCommittedStatusForTheSameSlug(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	slug := "batten-leak-status"
	seedBoardTask(t, h, slug, "loom")
	commitPrimeBattenRecords(t, h, slug, "batten-leak-status-other")

	c := &battenCLI{}
	if err := c.wire(h.Location, slug); err != nil {
		t.Fatalf("wire(%s): %v", slug, err)
	}
	seedEntryStatus(t, c, battenrecipe.NameWorktreeCreate, shedengine.StateRunning, nil)
	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}
	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("Step (Worktree-Create): %v", err)
	}

	statusPath, statusLockPath, err := c.env.InnerRun.ResolveStatus()
	if err != nil {
		t.Fatalf("ResolveStatus: %v", err)
	}
	st, found, err := c.env.InnerRun.ReadStatus(statusPath, statusLockPath)
	if err != nil {
		t.Fatalf("ReadStatus = %v; want a nil error", err)
	}
	if found {
		t.Errorf("ReadStatus reported found = true (state %q); want false", st.State)
	}
}
