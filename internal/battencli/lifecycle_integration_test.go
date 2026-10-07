//go:build integration

// lifecycle_integration_test.go is the end-to-end suite over a real hub built by
// internal/hubforge through its fabric fixture entry point, per the hubforge Fabric-Fixture
// Invariant. It stays a white-box "package battencli" test, not an external "_test" package,
// because most tests here stub Env.InnerRun.Spawn and Env.InnerRun.ReadStatus at the field level
// after a real wire() call -- a no-op spawn and a read-status answering a chosen state -- so the row
// routing and the persisted-state branching can be driven without a real child, and that stubbing
// needs the unexported wire method and the battenCLI receiver.
//
// Stubbing those two fields removes the real child bootstrap and the real status read over the
// child's own paths, so the two tests named RealReadStatus and SeedChild_WritesASeedTheChildBootstrapAgreesWith
// deliberately do not stub, and hold those seams instead.
// Nothing here spawns a real provider: per batten's crucible cost declaration that belongs in
// manual CLI driving, never inside go test.
//
// It lives at the integration tier rather than Tier 1 because the prime-name lookup this package's
// own pre-run refusal performs reaches a real git worktree listing, and getting there at all needs
// the resolver -- both barred from untagged files by the Test Tier Purity Invariant.

package battencli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// shortPollBattenRecipe is contracts/recipes/batten-recipe.yaml, byte-for-byte the same four rows
// and the same on_stuck self-route with no max_bounces, with poll_interval_s dropped from 2 to 1 so
// the step-driven re-entrancy test below spends whole seconds only on the checks it needs. It is built here, not by
// faking Env.InnerRun.Sleep, because this tier exists to exercise the assembled wiring -- the
// embedded recipe's own config value flowing through shedbuild into innerRunEntry -- rather than
// the InnerRun producer in isolation, which battenshed's own untagged tests already cover.
const shortPollBattenRecipe = `
version: 1
entry: Worktree-Create
terminals:
  - Worktree-Teardown

producers:
  - name: Worktree-Create
    engine: WorktreeCreate
    on_done: Seed-Child

  - name: Seed-Child
    engine: SeedChild
    on_done: Run-Shed

  - name: Run-Shed
    engine: InnerRun
    on_done: Worktree-Teardown
    on_stuck: Run-Shed
    config:
      poll_interval_s: 1
      driver_exit_grace_s: 900

  - name: Worktree-Teardown
    engine: WorktreeTeardown
    on_done: ""
`

// wireForHub builds a *battenCLI wired for real against h's prime Location and slug -- a real
// CreateWorktree and a real Teardown, both driving fabricengine's topology holder against h's own
// hub -- then overrides Env.InnerRun.Spawn and Env.InnerRun.ReadStatus with readStatus, per this
// file's own header.
//
// The Run-Shed wait stats the child's status file, so a stubbed read moves that file into a temporary directory and also creates it when it is absent.
// The wait's sleep is a counter and the pause seam reports a pause from the first check on, so a stubbed child that never changes makes the wait return after one check; a test that wants a longer wait replaces either.
func wireForHub(t *testing.T, h *hubforge.Hub, slug string, readStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)) *battenCLI {
	t.Helper()
	c := &battenCLI{}
	if err := c.wire(h.Location, slug); err != nil {
		t.Fatalf("wire(%s): %v", slug, err)
	}
	c.env.InnerRun.Spawn = func(ctx context.Context) error { return nil }
	c.env.InnerRun.ReadStatus = nil
	if readStatus != nil {
		// The child's status file lives outside the repository, so the file the wait stats leaves nothing for a teardown to commit.
		childStatus := filepath.Join(t.TempDir(), "child-status.json")
		c.env.InnerRun.ResolveStatus = func() (string, string, error) { return childStatus, childStatus + ".lock", nil }
		c.env.InnerRun.ReadStatus = func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
			ensureStatusFile(t, statusPath)
			return readStatus(statusPath, statusLockPath)
		}
	}
	sleeps := 0
	c.env.InnerRun.Sleep = func(ctx context.Context, d time.Duration) { sleeps++ }
	c.env.InnerRun.PauseRequested = func() (bool, error) { return sleeps > 0, nil }
	return c
}

// ensureStatusFile creates the child's status file at path when it is absent, standing in for the file the real read decodes.
func ensureStatusFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir child status dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write child status file: %v", err)
	}
}

// seedEntryStatus writes c's status file with CurrentProducer/State as given, an empty non-nil
// History, under c's own StatusPath/StatusLockPath.
func seedEntryStatus(t *testing.T, c *battenCLI, producer string, rowState shedengine.State, history []shedengine.HistoryEntry) {
	t.Helper()
	if history == nil {
		history = []shedengine.HistoryEntry{}
	}
	// state.WriteJSON only MkdirAlls StatusPath's own (durable) directory, never StatusLockPath's
	// (ephemeral) one -- the two no longer share a directory now that the status file is durable
	// and its lock is ephemeral, so a fixture seeding the file directly, ahead of any CLI pre-run
	// that would otherwise ensure it (see battenPreRun's own MkdirAll), must ensure it here.
	if err := os.MkdirAll(filepath.Dir(c.shedPaths.StatusLockPath), 0o755); err != nil {
		t.Fatalf("mkdir status lock dir: %v", err)
	}
	if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, shedengine.Status{
		CurrentProducer: producer,
		State:           rowState,
		History:         history,
	}); err != nil {
		t.Fatalf("seed status: %v", err)
	}
}

// seedBoardTask upserts a Board task named slug carrying recipeType as its own "recipe" field,
// fataling on error. Seed-Child reads this fresh at Call time -- never a value captured earlier --
// to choose the child worktree's own recipe, so every test that walks the recipe from
// Worktree-Create through Seed-Child needs one seeded first.
func seedBoardTask(t *testing.T, h *hubforge.Hub, slug, recipeType string) {
	t.Helper()
	cfg, err := boardengine.LoadConfig(h.BoardDir(), "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	b := boardengine.New(cfg)
	if _, err := b.UpsertTask(map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []string{"bug"}, "recipe": recipeType}); err != nil {
		t.Fatalf("seed board task %q: %v", slug, err)
	}
}

// pathExists reports whether path exists on disk.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// gitShow runs `git show <spec>` in dir and returns its stdout, fataling on failure -- used to
// assert a path is committed, not merely written, on a pair's records worktree: an uncommitted seed is
// as lost to a fresh clone as one never written, the machine-switch case the Seed-Child row exists
// for.
func gitShow(t *testing.T, dir, spec string) []byte {
	t.Helper()
	cmd := exec.Command("git", "show", spec)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s (in %s): %v", spec, dir, err)
	}
	return out
}

// TestBattenIntegration_Rows runs the end-to-end rows over one hubforge hub, each step on its own slug so its pair, branches and records stay apart from every other step's.
// The steps share only the hub itself -- its prime worktree, records origin and board -- and a step that changes it for the next one restores it before returning, except the last, which dirties the prime worktree and so relies on running after every step that creates a pair.
// The test calls t.Parallel and no step does: the steps serialise on that one shared hub.
func TestBattenIntegration_Rows(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	steps := []struct {
		name string
		run  func(t *testing.T, h *hubforge.Hub)
	}{
		{"SeedChild_WritesASeedTheChildBootstrapAgreesWith", stepSeedChild_WritesASeedTheChildBootstrapAgreesWith},
		{"SeedChild_WriteSeedRewrapsARealDisagreeingChildSeed", stepSeedChild_WriteSeedRewrapsARealDisagreeingChildSeed},
		{"RealReadStatus_OnAFreshPairReportsAbsentRatherThanErroring", stepRealReadStatus_OnAFreshPairReportsAbsentRatherThanErroring},
		{"FourRowRun_SeedsChildCommitsAndTearsDown", stepFourRowRun_SeedsChildCommitsAndTearsDown},
		{"Teardown_AlreadyGonePairFinishesItsBranchDeletion", stepTeardown_AlreadyGonePairFinishesItsBranchDeletion},
		{"Teardown_FailedRemoteDeletionHaltsResumably", stepTeardown_FailedRemoteDeletionHaltsResumably},
		{"Teardown_UnreachableRemoteHaltsBeforeRemovalResumably", stepTeardown_UnreachableRemoteHaltsBeforeRemovalResumably},
		{"Teardown_AlreadyGonePairWithNoBranchIsDone", stepTeardown_AlreadyGonePairWithNoBranchIsDone},
		{"StepDrivenRunShed_ReturnsOnTheChildsStateChange", stepStepDrivenRunShed_ReturnsOnTheChildsStateChange},
		{"RunShedPausedChild_WaitsThenTearsDownOnceDone", stepRunShedPausedChild_WaitsThenTearsDownOnceDone},
		{"CreateRow_IsIdempotentAgainstAnAlreadyPresentWorktree", stepCreateRow_IsIdempotentAgainstAnAlreadyPresentWorktree},
		{"CreateRow_PairWithoutOriginRecordIsIncomplete", stepCreateRow_PairWithoutOriginRecordIsIncomplete},
		{"CreateRow_LeftoverBranchIsRewordedForPrime", stepCreateRow_LeftoverBranchIsRewordedForPrime},
		{"CreateRow_IncompletePairRefusesRatherThanSkippingAdd", stepCreateRow_IncompletePairRefusesRatherThanSkippingAdd},
		{"MidListResume_SkipsTheCompletedCreateRow", stepMidListResume_SkipsTheCompletedCreateRow},
		{"NonPrimeRefusal", stepNonPrimeRefusal},
		{"RecordsPrimeRefusal", stepRecordsPrimeRefusal},
		{"Teardown_ReEntryAfterCompletedRemovalIsDone", stepTeardown_ReEntryAfterCompletedRemovalIsDone},
		{"Teardown_SiblingDirtOutsideRecordPathsRefusesShutdown", stepTeardown_SiblingDirtOutsideRecordPathsRefusesShutdown},
		{"AttachDirNamesTheTaskWorktree", stepAttachDirNamesTheTaskWorktree},
		{"AwaitingApprovalResumeDoneTeardown_ArchivesTheRunRecords", stepAwaitingApprovalResumeDoneTeardown_ArchivesTheRunRecords},
		{"AwaitingRejectionResumesTheChildOnce", stepAwaitingRejectionResumesTheChildOnce},
		{"SeedChild_IgnoresPrimesCommittedBattenRecords", stepSeedChild_IgnoresPrimesCommittedBattenRecords},
		{"RealReadStatus_IgnoresPrimesCommittedStatusForTheSameSlug", stepRealReadStatus_IgnoresPrimesCommittedStatusForTheSameSlug},
		{"MarkWatched_HoldsTheMarkerOnlyWhileItsNoticesReachTheDriversParent", stepMarkWatched_HoldsTheMarkerOnlyWhileItsNoticesReachTheDriversParent},
		{"DirtyPrime_CreateRowBlocksBeforeAnythingCreated", stepDirtyPrime_CreateRowBlocksBeforeAnythingCreated},
	}
	for _, step := range steps {
		if !t.Run(step.name, func(t *testing.T) { step.run(t, h) }) {
			return
		}
	}
}

// stepSeedChild_WritesASeedTheChildBootstrapAgreesWith drives Worktree-Create and Seed-Child for real, then asserts the property the child's own bootstrap depends on: re-writing the seed the way that bootstrap will must be accepted by shedrun.WriteSeed, not refused as a disagreement.
//
// It reconstructs that seed's shape from the recorded origin rather than importing internal/loomcli, and asserts the recorded parent is what landed in the param, so the shape cannot drift into agreeing with itself while disagreeing with loom.
func stepSeedChild_WritesASeedTheChildBootstrapAgreesWith(t *testing.T, h *hubforge.Hub) {
	slug := "batten-seed-agrees"
	seedBoardTask(t, h, slug, "loom")

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	seedEntryStatus(t, c, battenrecipe.NameWorktreeCreate, shedengine.StateRunning, nil)

	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}
	ctx := context.Background()
	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (Worktree-Create): %v", err)
	}
	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (Seed-Child): %v", err)
	}

	childLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve child location: %v", err)
	}
	origin, originFound, err := fabricengine.ReadOrigin(childLocation)
	if err != nil {
		t.Fatalf("read child origin: %v", err)
	}
	if !originFound || origin.ParentBranch == "" {
		t.Fatalf("child pair has no recorded parent branch; Worktree-Create is expected to record one")
	}

	seed, seedFound, err := shedrun.ReadSeed(childLocation, shedrun.SelfRunID)
	if err != nil {
		t.Fatalf("read child seed: %v", err)
	}
	if !seedFound {
		t.Fatalf("Seed-Child wrote no child seed")
	}
	if origin.ParentWorktree != h.Location.WorktreeName {
		t.Errorf("child origin ParentWorktree = %q; want the prime %q", origin.ParentWorktree, h.Location.WorktreeName)
	}
	if got := seed.Params["parent"]; got != origin.ParentBranch {
		t.Errorf("child seed params[parent] = %q; want the recorded parent branch %q", got, origin.ParentBranch)
	}

	// The load-bearing assertion: this is byte-for-byte what loom's own bootstrap writes on its
	// first "lyx loom start" in that worktree. It must be a no-op, never a refusal.
	bootstrapSeed := shedrun.Seed{
		Recipe: seed.Recipe,
		Driver: seed.Driver,
		Params: map[string]string{"parent": origin.ParentBranch},
	}
	if err := shedrun.WriteSeed(childLocation, shedrun.SelfRunID, bootstrapSeed); err != nil {
		t.Fatalf("the child's own bootstrap seed was refused against Seed-Child's seed: %v", err)
	}
}

// stepSeedChild_WriteSeedRewrapsARealDisagreeingChildSeed pins the WriteSeed closure's own rewrap of shedrun.ErrDisagreeingSeed into battenshed.ErrDisagreeingChildSeed (wire.go): plants a real, pre-existing child seed that disagrees with what the closure is about to write, then calls c.env.SeedChild.WriteSeed directly, the production closure, rather than stubbing the sentinel already wrapped -- so a future edit that drops the rewrap fails here rather than only in a test that never reaches the real shedrun.WriteSeed call.
func stepSeedChild_WriteSeedRewrapsARealDisagreeingChildSeed(t *testing.T, h *hubforge.Hub) {
	slug := "batten-seed-disagrees"
	seedBoardTask(t, h, slug, "loom")
	hubforge.AddPair(t, h, slug)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the disagreement refusal fires before any poll")
		return shedengine.Status{}, false, nil
	})

	childLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve child location: %v", err)
	}
	// A driver of "llm" disagrees with the "go" driver this test's own ChildDriver seam (below)
	// resolves to, the same way an operator's hand-seeded or otherwise pre-existing child seed
	// might.
	if err := shedrun.WriteSeed(childLocation, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverLLM}); err != nil {
		t.Fatalf("plant a disagreeing child seed: %v", err)
	}

	err = c.env.SeedChild.WriteSeed(context.Background(), shedrun.RecipeLoom, shedrun.DriverGo)
	if !errors.Is(err, battenshed.ErrDisagreeingChildSeed) {
		t.Fatalf("WriteSeed() error = %v; want it to wrap battenshed.ErrDisagreeingChildSeed", err)
	}
}

// stepRealReadStatus_OnAFreshPairReportsAbsentRatherThanErroring drives the two InnerRun seams every other test in this file replaces -- the real Env.InnerRun.ResolveStatus and Env.InnerRun.ReadStatus -- against a freshly created pair that has never run.
//
// It asserts the producer's own contract: no status file yet reports found == false with a nil error, which is what tells innerRunProducer.Call to spawn.
func stepRealReadStatus_OnAFreshPairReportsAbsentRatherThanErroring(t *testing.T, h *hubforge.Hub) {
	slug := "batten-real-readstatus"
	hubforge.AddPair(t, h, slug)

	c := &battenCLI{}
	if err := c.wire(h.Location, slug); err != nil {
		t.Fatalf("wire(%s): %v", slug, err)
	}

	statusPath, statusLockPath, err := c.env.InnerRun.ResolveStatus()
	if err != nil {
		t.Fatalf("ResolveStatus on a fresh pair: %v", err)
	}
	if !pathExists(filepath.Dir(statusLockPath)) {
		t.Errorf("ResolveStatus left the child's status-lock directory %s absent; the read that follows it needs one", filepath.Dir(statusLockPath))
	}

	st, found, err := c.env.InnerRun.ReadStatus(statusPath, statusLockPath)
	if err != nil {
		t.Fatalf("ReadStatus on a fresh pair = %v; want a nil error reporting the status file simply absent", err)
	}
	if found {
		t.Errorf("ReadStatus on a fresh pair reported found = true (state %q); want false", st.State)
	}
}

// stepFourRowRun_SeedsChildCommitsAndTearsDown drives a spawn stub plus a read-status answering StateDone through the whole four-row list one row at a time: Worktree-Create, Seed-Child, Run-Shed, Worktree-Teardown.
// It asserts the pair exists on disk after the create row, that the child's own _lyx/shed/<slug>/seed.json exists after the seed row, names the Board task's own "type" as its recipe, and is committed (not merely written) on the child's own records pair, and that the pair is gone after the teardown row.
func stepFourRowRun_SeedsChildCommitsAndTearsDown(t *testing.T, h *hubforge.Hub) {
	slug := "batten-four-row"
	seedBoardTask(t, h, slug, "loom")

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	seedEntryStatus(t, c, battenrecipe.NameWorktreeCreate, shedengine.StateRunning, nil)

	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}

	ctx := context.Background()
	pairPath := h.PairWarpWorktree(slug)

	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (Worktree-Create): %v", err)
	}
	if !pathExists(pairPath) {
		t.Fatalf("pair does not exist after Worktree-Create: %s", pairPath)
	}

	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (Seed-Child): %v", err)
	}

	childLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve child location: %v", err)
	}
	seedPath := shedrun.SeedFile(childLocation, shedrun.SelfRunID)
	seedData, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read child seed %s: %v", seedPath, err)
	}
	var seed shedrun.Seed
	if err := json.Unmarshal(seedData, &seed); err != nil {
		t.Fatalf("decode child seed %s: %v", seedPath, err)
	}
	if seed.Recipe != "loom" {
		t.Errorf("child seed recipe = %q; want %q (the Board task's own type)", seed.Recipe, "loom")
	}

	recordsPath := h.PairWeftSibling(slug)
	seedRel := filepath.ToSlash(shedrun.SeedRel(childLocation, shedrun.SelfRunID))
	committedData := gitShow(t, recordsPath, "HEAD:"+seedRel)
	if !bytes.Equal(committedData, seedData) {
		t.Errorf("committed seed at HEAD:%s = %q; want it to match the working-tree seed %q", seedRel, committedData, seedData)
	}

	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (Run-Shed): %v", err)
	}
	if !pathExists(pairPath) {
		t.Fatalf("pair does not exist after Run-Shed: %s", pairPath)
	}

	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (Worktree-Teardown): %v", err)
	}
	if pathExists(pairPath) {
		t.Errorf("pair still exists after Worktree-Teardown: %s", pairPath)
	}

	// Teardown's own top.Remove call passes remote: true -- a batten-driven teardown is the task's
	// final removal, and nothing will ever re-adopt its records branch, so the remote copy must not
	// linger where "lyx fabric cleanup" (its own enumeration is local-branches-only) can never reach
	// it. See F-CLEANUP-REMOTE-ORPHAN.
	recordsBranch := fabricengine.WeftBranchName(slug)
	if err := exec.Command("git", "-C", h.WeftBare, "rev-parse", "--verify", "refs/heads/"+recordsBranch).Run(); err == nil {
		t.Errorf("records branch %q still present on the remote after teardown; want it deleted alongside the local copy", recordsBranch)
	}
}

// remoteBranchExists reports whether branch exists in the bare repository at bareDir.
func remoteBranchExists(bareDir, branch string) bool {
	return exec.Command("git", "-C", bareDir, "rev-parse", "--verify", "refs/heads/"+branch).Run() == nil
}

// stepTeardown_AlreadyGonePairFinishesItsBranchDeletion re-enters the removal half with both worktrees already gone but the pair's other-side branch left behind -- a removal killed after its worktree removals, before its branch deletions finished -- and asserts the row deletes that branch locally and on the remote rather than reporting done over it.
// A branch surviving on the remote makes a later create of the slug refuse its push.
func stepTeardown_AlreadyGonePairFinishesItsBranchDeletion(t *testing.T, h *hubforge.Hub) {
	tests := []struct {
		name      string
		keepLocal bool
	}{
		{"LocalAndRemoteLeft", true},
		{"RemoteOnlyLeft", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug := "batten-branch-left-" + tt.name
			hubforge.AddPair(t, h, slug)
			recordsBranch := fabricengine.WeftBranchName(slug)
			if !remoteBranchExists(h.WeftBare, recordsBranch) {
				t.Fatalf("precondition: %q not on the remote after the create", recordsBranch)
			}
			// Both worktrees removed, the remote copy kept -- the state a kill between the local
			// and remote deletions leaves.
			if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
				t.Fatalf("remove the pair without its remote copy: %v", err)
			}
			recordsRepoRoot, err := fabricengine.WeftRepoRoot(h.Location)
			if err != nil {
				t.Fatalf("resolve records repo root: %v", err)
			}
			if tt.keepLocal {
				gitkit.MustRun(t, recordsRepoRoot, "git", "fetch", "origin", recordsBranch)
				gitkit.MustRun(t, recordsRepoRoot, "git", "branch", recordsBranch, "FETCH_HEAD")
			}

			c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
				return shedengine.Status{State: shedengine.StateDone}, true, nil
			})
			if err := c.env.Teardown.Remove(context.Background()); err != nil {
				t.Fatalf("Teardown.Remove() = %v; want nil", err)
			}
			if remoteBranchExists(h.WeftBare, recordsBranch) {
				t.Errorf("%q still on the remote after the re-entered teardown; want it deleted", recordsBranch)
			}
			if exec.Command("git", "-C", recordsRepoRoot, "rev-parse", "--verify", "refs/heads/"+recordsBranch).Run() == nil {
				t.Errorf("%q still present locally after the re-entered teardown; want it deleted", recordsBranch)
			}
		})
	}
}

// refuseRemoteBranchDeletions installs a pre-receive hook on the bare repo at bareDir that rejects every branch deletion while accepting every other update, so a teardown's archive-tag push lands but its remote branch deletion fails.
// The returned func removes the hook.
func refuseRemoteBranchDeletions(t *testing.T, bareDir string) func() {
	t.Helper()
	hook := filepath.Join(bareDir, "hooks", "pre-receive")
	script := "#!/bin/sh\nwhile read old new ref; do\n  case \"$new\" in\n    *[!0]*) ;;\n    *) echo \"deletion of $ref refused\" >&2; exit 1 ;;\n  esac\ndone\nexit 0\n"
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatalf("create hooks dir: %v", err)
	}
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatalf("write pre-receive hook: %v", err)
	}
	return func() {
		if err := os.Remove(hook); err != nil {
			t.Fatalf("remove pre-receive hook: %v", err)
		}
	}
}

// remoteArchiveTagExists reports whether any archive/<slug>/ tag is present on the bare repo at bareDir.
func remoteArchiveTagExists(bareDir, slug string) bool {
	out, err := exec.Command("git", "-C", bareDir, "for-each-ref", "--format=%(refname)", "refs/tags/archive/"+slug+"/").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// stepTeardown_FailedRemoteDeletionHaltsResumably makes the records origin refuse branch deletions during teardown and asserts the removal half archives the records, removes the pair and reports the failed remote deletion instead of done, then that a resume once the remote accepts the deletion finishes it.
func stepTeardown_FailedRemoteDeletionHaltsResumably(t *testing.T, h *hubforge.Hub) {
	slug := "batten-remote-fails"
	hubforge.AddPair(t, h, slug)
	recordsBranch := fabricengine.WeftBranchName(slug)
	restore := refuseRemoteBranchDeletions(t, h.WeftBare)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	err := c.env.Teardown.Remove(context.Background())
	if err == nil {
		t.Fatal("Teardown.Remove() = nil with a remote refusing the deletion; want the failed remote deletion reported")
	}
	if !strings.Contains(err.Error(), "resume this run") {
		t.Errorf("Teardown.Remove() error = %q; want it to name the resume", err.Error())
	}
	if pathExists(h.PairWarpWorktree(slug)) {
		t.Errorf("task worktree still present; want the pair removed before the remote deletion failed")
	}
	if !remoteArchiveTagExists(h.WeftBare, slug) {
		t.Errorf("no archive/%s/ tag on the remote; want the records archived before the pair was removed", slug)
	}

	restore()
	if err := c.env.Teardown.Remove(context.Background()); err != nil {
		t.Fatalf("Teardown.Remove() on resume = %v; want nil", err)
	}
	if remoteBranchExists(h.WeftBare, recordsBranch) {
		t.Errorf("%q still on the remote after the resumed teardown; want it deleted", recordsBranch)
	}
}

// stepTeardown_UnreachableRemoteHaltsBeforeRemovalResumably breaks the records origin before teardown and asserts the removal half halts on the failed archive with the pair still in place and the resume named, then that a resume once the remote is reachable again archives the records and finishes the teardown.
func stepTeardown_UnreachableRemoteHaltsBeforeRemovalResumably(t *testing.T, h *hubforge.Hub) {
	slug := "batten-remote-unreachable"
	hubforge.AddPair(t, h, slug)
	recordsBranch := fabricengine.WeftBranchName(slug)
	recordsRepoRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("resolve records repo root: %v", err)
	}
	gitkit.MustRun(t, recordsRepoRoot, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	err = c.env.Teardown.Remove(context.Background())
	if err == nil {
		t.Fatal("Teardown.Remove() = nil with an unreachable remote; want the failed archive reported")
	}
	if !errors.Is(err, fabricengine.ErrArchiveFailed) {
		t.Errorf("Teardown.Remove() error = %v; want it to wrap fabricengine.ErrArchiveFailed", err)
	}
	if !strings.Contains(err.Error(), "lyx batten run "+slug) {
		t.Errorf("Teardown.Remove() error = %q; want it to name the resume", err.Error())
	}
	if !pathExists(h.PairWarpWorktree(slug)) {
		t.Errorf("task worktree removed; want the pair left in place when the archive fails")
	}

	gitkit.MustRun(t, recordsRepoRoot, "git", "remote", "set-url", "origin", h.WeftBare)
	if err := c.env.Teardown.Remove(context.Background()); err != nil {
		t.Fatalf("Teardown.Remove() on resume = %v; want nil", err)
	}
	if pathExists(h.PairWarpWorktree(slug)) {
		t.Errorf("task worktree still present after the resumed teardown; want it removed")
	}
	if !remoteArchiveTagExists(h.WeftBare, slug) {
		t.Errorf("no archive/%s/ tag on the remote after the resumed teardown; want the records archived", slug)
	}
	if remoteBranchExists(h.WeftBare, recordsBranch) {
		t.Errorf("%q still on the remote after the resumed teardown; want it deleted", recordsBranch)
	}
}

// stepTeardown_AlreadyGonePairWithNoBranchIsDone re-enters the removal half after a removal that finished completely -- the transition just not yet persisted -- and asserts done.
func stepTeardown_AlreadyGonePairWithNoBranchIsDone(t *testing.T, h *hubforge.Hub) {
	slug := "batten-teardown-finished"
	hubforge.AddPair(t, h, slug)
	if _, err := h.Topology.Remove(h.Location, slug, false, true); err != nil {
		t.Fatalf("remove the pair: %v", err)
	}
	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	if err := c.env.Teardown.Remove(context.Background()); err != nil {
		t.Errorf("Teardown.Remove() = %v; want nil for a pair already fully removed", err)
	}
}

// stepStepDrivenRunShed_ReturnsOnTheChildsStateChange proves Run-Shed's in-call wait through a step: a running child yields a "lyx batten step" that checks every poll_interval_s and returns once the child's state changes, naming the change, rather than after one poll interval.
//
// The wait runs on the real sleep with no pause: the step BLOCKS across the checks the change takes, and the bound is the change, not the poll interval.
func stepStepDrivenRunShed_ReturnsOnTheChildsStateChange(t *testing.T, h *hubforge.Hub) {
	slug := "batten-step-reentrant"
	// The pair must already exist on disk: InnerRun's ResolveStatus resolves the child worktree's own *lyxcwd.Location, which requires a real git worktree there, standing in for a Worktree-Create that already completed on an earlier step -- exactly as stepMidListResume_SkipsTheCompletedCreateRow's own fixture does.
	hubforge.AddPair(t, h, slug)

	var mu sync.Mutex
	childState := shedengine.StateRunning
	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		mu.Lock()
		defer mu.Unlock()
		return shedengine.Status{State: childState, CurrentProducer: "Plan-Review"}, true, nil
	})
	c.env.InnerRun.Sleep = nil
	c.env.InnerRun.PauseRequested = nil
	// The Board task's own Seed-Child row is not exercised by this test: Run-Shed is already the
	// persisted current producer, standing in for a Worktree-Create + Seed-Child that already
	// completed on an earlier step.
	seedEntryStatus(t, c, battenrecipe.NameRunShed, shedengine.StateRunning, []shedengine.HistoryEntry{
		{Producer: battenrecipe.NameWorktreeCreate, Outcome: shedengine.Done},
		{Producer: battenrecipe.NameSeedChild, Outcome: shedengine.Done},
	})

	shed, err := shedbuild.NewShed([]byte(shortPollBattenRecipe), c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("shedbuild.NewShed: %v", err)
	}
	childStatusPath, _, err := c.env.InnerRun.ResolveStatus()
	if err != nil {
		t.Fatalf("resolve the child's status path: %v", err)
	}
	ensureStatusFile(t, childStatusPath)

	// The child blocks between the first and the second check of the 1s poll.
	time.AfterFunc(1500*time.Millisecond, func() {
		mu.Lock()
		childState = shedengine.StateBlocked
		mu.Unlock()
		later := time.Now().Add(time.Hour)
		if err := os.Chtimes(childStatusPath, later, later); err != nil {
			t.Errorf("rewrite the child's status file: %v", err)
		}
	})

	start := time.Now()
	result, err := shed.Step(context.Background())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}

	if result.Producer != battenrecipe.NameRunShed {
		t.Errorf("Producer = %q; want %q", result.Producer, battenrecipe.NameRunShed)
	}
	if result.Outcome != shedengine.Stuck {
		t.Errorf("Outcome = %q; want %q (the change, self-routed)", result.Outcome, shedengine.Stuck)
	}
	if result.Next != battenrecipe.NameRunShed {
		t.Errorf("Next = %q; want %q (self-route)", result.Next, battenrecipe.NameRunShed)
	}
	if result.State != shedengine.StateRunning {
		t.Errorf("State = %q; want %q (self-routed Stuck stays running, never blocked)", result.State, shedengine.StateRunning)
	}
	if want := "child running → blocked at Plan-Review"; result.Output != want {
		t.Errorf("Output = %q; want the change named: %q", result.Output, want)
	}

	// The step returned on the change, past the first check at 1s, not after one poll_interval_s.
	if elapsed < 1800*time.Millisecond {
		t.Errorf("Step returned after %s; want it to have waited past the first check for the change (>= 1.8s)", elapsed)
	}
	if elapsed > 10*time.Second {
		t.Errorf("Step returned after %s; want it to return on the change, not hold for a much longer wait", elapsed)
	}
}

// stepRunShedPausedChild_WaitsThenTearsDownOnceDone drives a child that is paused, then resumed, then done, asserting each change ends a Run-Shed step with the pair intact and that the run then reaches Worktree-Teardown, which removes the pair.
//
// A halted child is a budget-exempt wait out of InnerRun.Call, never a hard error:
// the pair keeps the watcher that lands and tears it down once the operator resumes the child.
func stepRunShedPausedChild_WaitsThenTearsDownOnceDone(t *testing.T, h *hubforge.Hub) {
	slug := "batten-paused"
	hubforge.AddPair(t, h, slug)

	childState := shedengine.StatePaused
	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: childState, CurrentProducer: "loom-side-producer", Error: "loom session paused"}, true, nil
	})
	childStatusPath, _, err := c.env.InnerRun.ResolveStatus()
	if err != nil {
		t.Fatalf("resolve the child's status path: %v", err)
	}
	ensureStatusFile(t, childStatusPath)
	// Each check moves the child on one state and rewrites its status file, so the first step ends on the resume and the second on the finish.
	moves := []shedengine.State{shedengine.StateRunning, shedengine.StateDone}
	rewrites := 0
	c.env.InnerRun.Sleep = func(ctx context.Context, d time.Duration) {
		if len(moves) == 0 {
			return
		}
		childState, moves = moves[0], moves[1:]
		rewrites++
		at := time.Now().Add(time.Duration(rewrites) * time.Hour)
		if err := os.Chtimes(childStatusPath, at, at); err != nil {
			t.Errorf("rewrite the child's status file: %v", err)
		}
	}
	c.env.InnerRun.PauseRequested = nil
	seedEntryStatus(t, c, battenrecipe.NameRunShed, shedengine.StateRunning, []shedengine.HistoryEntry{
		{Producer: battenrecipe.NameWorktreeCreate, Outcome: shedengine.Done},
		{Producer: battenrecipe.NameSeedChild, Outcome: shedengine.Done},
	})

	shed, err := shedbuild.NewShed([]byte(shortPollBattenRecipe), c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("shedbuild.NewShed: %v", err)
	}
	ctx := context.Background()
	pairPath := h.PairWarpWorktree(slug)

	for i, wantOutput := range []string{"child paused → running at loom-side-producer", "child running → done at loom-side-producer"} {
		res, err := shed.Step(ctx)
		if err != nil {
			t.Fatalf("Step %d (Run-Shed over a changing child): %v", i, err)
		}
		if res.Outcome != shedengine.Stuck || res.Next != battenrecipe.NameRunShed || res.State != shedengine.StateRunning || res.Output != wantOutput {
			t.Errorf("Step %d = outcome %q next %q state %q output %q; want a Stuck self-route that stays running with output %q", i, res.Outcome, res.Next, res.State, res.Output, wantOutput)
		}
		if !pathExists(pairPath) {
			t.Fatalf("pair does not exist after step %d; want it left intact: %s", i, pairPath)
		}
	}

	for i := 0; i < 5 && pathExists(pairPath); i++ {
		if _, err := shed.Step(ctx); err != nil {
			t.Fatalf("Step %d after the child finished: %v", i, err)
		}
	}
	if pathExists(pairPath) {
		t.Errorf("pair still exists after the child reached done; want Worktree-Teardown to remove it: %s", pairPath)
	}
}

// stepCreateRow_IsIdempotentAgainstAnAlreadyPresentWorktree proves the create row's own idempotency: a task worktree that already exists satisfies the row's post-condition, so the row must report done and let the run advance rather than asking fabric to create it twice.
//
// The state it reconstructs is the one a process killed between Topology.Add succeeding and shedengine persisting the transition leaves behind: the worktree on disk, the status still naming the create row.
// Without the probe the row takes fabric's pre-existing-branch refusal, whose two named remedies both refuse in exactly this state, leaving the run unresumable.
func stepCreateRow_IsIdempotentAgainstAnAlreadyPresentWorktree(t *testing.T, h *hubforge.Hub) {
	slug := "batten-create-idempotent"
	seedBoardTask(t, h, slug, "loom")
	// The pair the killed drive already created, with nothing recording it.
	hubforge.AddPair(t, h, slug)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	seedEntryStatus(t, c, battenrecipe.NameWorktreeCreate, shedengine.StateRunning, nil)

	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}

	step, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step (Worktree-Create against an already-present worktree): %v", err)
	}
	if step.Outcome != shedengine.Done {
		t.Errorf("Outcome = %q; want %q -- an already-present worktree satisfies the row", step.Outcome, shedengine.Done)
	}
	if step.Next != battenrecipe.NameSeedChild {
		t.Errorf("Next = %q; want %q -- the run must advance rather than halt", step.Next, battenrecipe.NameSeedChild)
	}
	if !pathExists(h.PairWarpWorktree(slug)) {
		t.Errorf("task worktree missing after the idempotent create row: %s", h.PairWarpWorktree(slug))
	}
}

// stepCreateRow_PairWithoutOriginRecordIsIncomplete pins the create row's completeness check against a pair Add wired but never recorded the parent branch of -- a SIGKILL between junction wiring and the origin record's write.
// Reporting done there seeds the child with no parent param, which the child's own bootstrap refuses on every resume.
func stepCreateRow_PairWithoutOriginRecordIsIncomplete(t *testing.T, h *hubforge.Hub) {
	slug := "batten-create-no-origin"
	hubforge.AddPair(t, h, slug)
	if err := os.Remove(fabricengine.OriginRecordPathFor(h.Location, slug)); err != nil {
		t.Fatalf("remove origin record: %v", err)
	}

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the create row must refuse before the poll row ever runs")
		return shedengine.Status{}, false, nil
	})

	err := c.env.CreateWorktree(context.Background())
	if err == nil {
		t.Fatal("CreateWorktree() error = nil; want the incomplete-pair refusal for a pair with no origin record")
	}
	if !strings.Contains(err.Error(), "not fully created") {
		t.Errorf("CreateWorktree() error = %q; want it to contain %q", err.Error(), "not fully created")
	}
}

// stepCreateRow_LeftoverBranchIsRewordedForPrime pins createRefusal's own wiring into the CreateWorktree closure: a leftover code branch from an earlier torn-down pair (or a rolled-back create) must reach the closure's caller worded for an operator standing in prime, never fabric's own raw "lyx fabric checkout" advice, which would switch prime itself onto the task's branch.
// Calls c.env.CreateWorktree directly, the production closure, rather than createRefusal in isolation, so a future edit that drops the reword call fails here.
func stepCreateRow_LeftoverBranchIsRewordedForPrime(t *testing.T, h *hubforge.Hub) {
	slug := "batten-leftover-branch"
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", slug)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the create row must fail before the poll row ever runs")
		return shedengine.Status{}, false, nil
	})

	err := c.env.CreateWorktree(context.Background())
	if err == nil {
		t.Fatal("CreateWorktree() error = nil; want the leftover-branch refusal")
	}
	// The reworded text names "lyx fabric checkout" only inside its own "never do this" warning, not
	// as a suggested remedy -- the same distinction TestCreateRefusal_LeftoverBranchRemedyNeverNamesCheckout
	// (wire_test.go) draws for createRefusal in isolation.
	if strings.Contains(err.Error(), "lyx fabric checkout") && !strings.Contains(err.Error(), `never "lyx fabric checkout"`) {
		t.Errorf("CreateWorktree() error = %q; want it to never suggest \"lyx fabric checkout\" from prime", err.Error())
	}
	for _, want := range []string{slug, "git branch -D " + slug, "resume this run"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CreateWorktree() error = %q; want it to contain %q", err.Error(), want)
		}
	}
}

// stepCreateRow_IncompletePairRefusesRatherThanSkippingAdd pins taskWorktreeComplete's own fix: a code worktree a SIGKILL-interrupted Add left behind, with no sibling and no junctions wired, must not be mistaken for a finished create.
// Reproduces the state a process killed right after Add's own first step leaves -- the code worktree and branch exist, nothing else does -- by driving the same git command Add's own createGitWorktree issues, rather than stubbing anything: this proves taskWorktreeComplete's real filesystem check, not a fake of it.
func stepCreateRow_IncompletePairRefusesRatherThanSkippingAdd(t *testing.T, h *hubforge.Hub) {
	slug := "batten-incomplete-pair"
	target := h.PairWarpWorktree(slug)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "add", "-b", slug, target)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the create row must refuse before the poll row ever runs")
		return shedengine.Status{}, false, nil
	})

	err := c.env.CreateWorktree(context.Background())
	if err == nil {
		t.Fatal("CreateWorktree() error = nil; want a refusal naming the incomplete pair, not silent success")
	}
	for _, want := range []string{slug, "not fully created", "lyx fabric remove --force " + slug, "git branch -D " + slug} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CreateWorktree() error = %q; want it to contain %q", err.Error(), want)
		}
	}
	// The refusal must be read-only: no repair attempt, no partial Add left further along than it
	// started.
	if pathExists(h.PairWeftSibling(slug)) {
		t.Errorf("the pair's other-side worktree exists after the refusal; want CreateWorktree to have made no repair attempt")
	}
}

// TestBattenIntegration_CreateRow_IncompletePairRemedyWorksVerbatimOnAPrefixedHub follows the
// incomplete-pair remedy exactly as worded, from prime, for a SIGKILL landing after Add created the
// pair's other side and its portal: the remedy must name the prefixed branch Add created, and doing
// what it says must leave nothing that makes the resumed create refuse.
func TestBattenIntegration_CreateRow_IncompletePairRemedyWorksVerbatimOnAPrefixedHub(t *testing.T) {
	// Not a step of TestBattenIntegration_Rows: seeding a branch prefix into the fabric config changes the hub for every pair created after it.
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	hubforge.SeedFabricConfig(t, h, "branch_prefix: r4/\npathspec: \"\"\n")
	slug := "batten-incomplete-prefixed"
	branch := "r4/" + slug
	target := h.PairWarpWorktree(slug)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "add", "-b", branch, target)

	// Stands in for Add's own later steps -- the other side's worktree and the portal -- run
	// directly rather than through Add so the junctions this test needs missing stay missing.
	recordsRepoRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("resolve records repo root: %v", err)
	}
	gitkit.MustRun(t, recordsRepoRoot, "git", "worktree", "add", "-b", fabricengine.WeftBranchName(branch), h.PairWeftSibling(slug))
	portal := fabricengine.PortalLink(h.Location, slug)
	if err := os.MkdirAll(filepath.Dir(portal), 0o755); err != nil {
		t.Fatalf("mkdir portals: %v", err)
	}
	if err := os.Symlink(filepath.Join(target, h.Location.AnchorRel, lyxdirs.LyxDirName), portal); err != nil {
		t.Fatalf("plant portal: %v", err)
	}

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called by the create row")
		return shedengine.Status{}, false, nil
	})

	err = c.env.CreateWorktree(context.Background())
	if err == nil {
		t.Fatal("CreateWorktree() error = nil; want the incomplete-pair refusal")
	}
	removeCommand := "lyx fabric remove --force " + slug
	branchCommand := "git branch -D " + branch
	for _, want := range []string{removeCommand, branchCommand} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("CreateWorktree() error = %q; want it to contain %q", err.Error(), want)
		}
	}

	// The remedy, verbatim: the fabric verb's own engine call with --force, then the branch, only if it survived the removal.
	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(h.Location.HubPath))
	if err != nil {
		t.Fatalf("load fabric config: %v", err)
	}
	if _, err := fabricengine.NewTopology(cfg).Remove(h.Location, slug, true, false); err != nil {
		t.Fatalf("%s: %v", removeCommand, err)
	}
	if exec.Command("git", "-C", h.PrimeWorktree(), "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil {
		gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "-D", branch)
	}
	if exec.Command("git", "-C", h.PrimeWorktree(), "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil {
		t.Fatalf("%q still present after the remedy; want it gone", branch)
	}

	if err := c.env.CreateWorktree(context.Background()); err != nil {
		t.Fatalf("CreateWorktree() after the remedy = %v; want the fresh create to succeed", err)
	}
	if !pathExists(h.PairWeftSibling(slug)) {
		t.Errorf("the pair's other side is missing after the resumed create: %s", h.PairWeftSibling(slug))
	}
}

// stepDirtyPrime_CreateRowBlocksBeforeAnythingCreated dirties a tracked file in the hub's prime worktree, asserting the create row halts blocked before anything is created -- this refusal fires on every batten run and is invisible to the unit tests' fakes.
func stepDirtyPrime_CreateRowBlocksBeforeAnythingCreated(t *testing.T, h *hubforge.Hub) {
	slug := "batten-dirty-prime"

	readmePath := filepath.Join(h.PrimeWorktree(), "README")
	if err := os.WriteFile(readmePath, []byte("dirtied for the test\n"), 0o644); err != nil {
		t.Fatalf("dirty prime README: %v", err)
	}

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the create row must block before the poll row ever runs")
		return shedengine.Status{}, false, nil
	})
	seedEntryStatus(t, c, battenrecipe.NameWorktreeCreate, shedengine.StateRunning, nil)

	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Errorf("Outcome = %q; want %q", result.Outcome, shedengine.RunBlocked)
	}
	if result.HaltedProducer != battenrecipe.NameWorktreeCreate {
		t.Errorf("HaltedProducer = %q; want %q", result.HaltedProducer, battenrecipe.NameWorktreeCreate)
	}
	if pathExists(h.PairWarpWorktree(slug)) {
		t.Errorf("pair exists even though the create row blocked before creating anything: %s", h.PairWarpWorktree(slug))
	}
}

// stepMidListResume_SkipsTheCompletedCreateRow proves mid-list resume: it creates the pair directly (standing in for a create row that already completed before a crash), seeds a status file whose current producer is Run-Shed, and re-invokes the run verb -- asserting CreateWorktree is never called again and the pair is torn down once the poll row answers Done.
func stepMidListResume_SkipsTheCompletedCreateRow(t *testing.T, h *hubforge.Hub) {
	slug := "batten-resume"
	hubforge.AddPair(t, h, slug)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	c.env.CreateWorktree = func(ctx context.Context) error {
		t.Fatal("CreateWorktree must not run again: the create row already completed before the crash this test simulates")
		return nil
	}
	seedEntryStatus(t, c, battenrecipe.NameRunShed, shedengine.StateBlocked, []shedengine.HistoryEntry{
		{Producer: battenrecipe.NameWorktreeCreate, Outcome: shedengine.Done},
		{Producer: battenrecipe.NameSeedChild, Outcome: shedengine.Done},
	})

	var out bytes.Buffer
	exitCode := clihelp.Execute(battenVerbCommand(c, "run"), &out, []string{slug})
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d; want 0; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Errorf("run() output missing ok:true envelope; got: %q", out.String())
	}
	if pathExists(h.PairWarpWorktree(slug)) {
		t.Errorf("pair still exists after the resumed run's teardown row completed: %s", h.PairWarpWorktree(slug))
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; output: %s", err, out.String())
	}
	// history_length is new in this task: the resumed run's persisted history already carries the
	// two pre-seeded entries (create, seed) plus whatever this invocation appended, so it must be
	// at least 3.
	gotLen, ok := envelope["history_length"].(float64)
	if !ok {
		t.Fatalf("envelope[\"history_length\"] = %v (%T); want a number", envelope["history_length"], envelope["history_length"])
	}
	if gotLen < 3 {
		t.Errorf("envelope[\"history_length\"] = %v; want at least 3 (the two pre-seeded entries plus this run's own)", gotLen)
	}
}

// stepNonPrimeRefusal covers all four verbs' non-prime refusal, driven through RunCLIIn with an injected cwd pointing at a real task worktree -- the runtime check standing in for the Bookend invariant's missing enforcing test.
// Mirrors stepRecordsPrimeRefusal's own four-verb completeness below.
func stepNonPrimeRefusal(t *testing.T, h *hubforge.Hub) {
	taskSlug := "batten-task-cwd"
	hubforge.AddPair(t, h, taskSlug)

	taskCwd := h.PairWarpWorktree(taskSlug)
	primeName := h.Location.WorktreeName

	for _, verb := range []string{"run", "step", "status", "pause"} {
		t.Run(verb, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := RunCLIIn(taskCwd, &out, []string{verb, "some-slug"})

			if exitCode != 1 {
				t.Fatalf("RunCLIIn(%s) exit code = %d; want 1; output: %s", verb, exitCode, out.String())
			}
			if !strings.Contains(out.String(), taskSlug) {
				t.Errorf("%s refusal = %q; want it to name the task worktree %q", verb, out.String(), taskSlug)
			}
			if !strings.Contains(out.String(), primeName) {
				t.Errorf("%s refusal = %q; want it to name the prime worktree %q", verb, out.String(), primeName)
			}
		})
	}
}

// stepRecordsPrimeRefusal pins the other half of the Bookend guard: the records sibling of the prime is a repository of its own whose prime is itself, so a name comparison alone admits it, and both bookend rows would then drive fabric's topology against the records repository.
// Every verb must refuse there before arming anything -- no seed written under the records prime, nothing created -- and the refusal must say which checkout the operator is standing in.
func stepRecordsPrimeRefusal(t *testing.T, h *hubforge.Hub) {
	recordsPrimeCwd := h.PrimeWeft()

	for _, verb := range []string{"run", "step", "status", "pause"} {
		t.Run(verb, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := RunCLIIn(recordsPrimeCwd, &out, []string{verb, "some-slug"})

			if exitCode != 1 {
				t.Fatalf("RunCLIIn(%s) from the records prime exit code = %d; want 1; output: %s", verb, exitCode, out.String())
			}
			if !strings.Contains(out.String(), "weft sibling") {
				t.Errorf("%s refusal = %q; want it to name the weft sibling", verb, out.String())
			}
			if !strings.Contains(out.String(), "prime worktree only") {
				t.Errorf("%s refusal = %q; want the prime-only wording", verb, out.String())
			}
		})
	}

	if pathExists(filepath.Join(recordsPrimeCwd, "_lyx", "shed", "some-slug")) {
		t.Errorf("a run directory was seeded under the records prime; the refusal must land before the auto-seed")
	}
}

// stepTeardown_ReEntryAfterCompletedRemovalIsDone tears a created pair down through both halves, then runs both again, and asserts the second pass returns nil from each.
// That second pass is the state a process killed right after the removal leaves, since shedengine persists the row's transition only after the producer returns.
func stepTeardown_ReEntryAfterCompletedRemovalIsDone(t *testing.T, h *hubforge.Hub) {
	slug := "batten-reentry"
	hubforge.AddPair(t, h, slug)
	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})

	for _, pass := range []string{"first", "re-entered"} {
		if _, err := c.env.Teardown.Shutdown(context.Background()); err != nil {
			t.Fatalf("%s Teardown.Shutdown() = %v; want nil", pass, err)
		}
		if err := c.env.Teardown.Remove(context.Background()); err != nil {
			t.Fatalf("%s Teardown.Remove() = %v; want nil", pass, err)
		}
	}
	if pathExists(h.PairWarpWorktree(slug)) {
		t.Error("task worktree still present after the teardown; want it removed")
	}
}

// stepTeardown_SiblingDirtOutsideRecordPathsRefusesShutdown dirties a created pair's sibling worktree outside the record pathspec and asserts the shutdown half refuses with fabric's sibling-dirty error, never names --force, and leaves the pair in place.
func stepTeardown_SiblingDirtOutsideRecordPathsRefusesShutdown(t *testing.T, h *hubforge.Hub) {
	slug := "batten-sibling-dirt"
	hubforge.AddPair(t, h, slug)
	stray := filepath.Join(fabricengine.WeftWorktreePath(h.Location, slug), "stray.txt")
	if err := os.WriteFile(stray, []byte("not a record\n"), 0o644); err != nil {
		t.Fatalf("write the out-of-pathspec file: %v", err)
	}
	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})

	_, err := c.env.Teardown.Shutdown(context.Background())
	if err == nil {
		t.Fatal("Teardown.Shutdown() = nil; want the sibling-dirt refusal")
	}
	if !errors.Is(err, fabricengine.ErrPairSiblingDirty) {
		t.Errorf("Teardown.Shutdown() error = %v; want it to wrap ErrPairSiblingDirty", err)
	}
	if strings.Contains(err.Error(), "--force") {
		t.Errorf("Teardown.Shutdown() error = %q; want it to never name --force", err.Error())
	}
	if !pathExists(h.PairWarpWorktree(slug)) {
		t.Error("task worktree is gone after the refused shutdown; want the pair left in place")
	}
	if !pathExists(stray) {
		t.Error("the sibling's stray file is gone after the refused shutdown; want the sibling untouched")
	}
}

// stepAttachDirNamesTheTaskWorktree pins the InnerRun.AttachDir seam to the task worktree's own anchor, resolved on Call over a real pair.
func stepAttachDirNamesTheTaskWorktree(t *testing.T, h *hubforge.Hub) {
	slug := "batten-attach-dir"
	hubforge.AddPair(t, h, slug)

	c := wireForHub(t, h, slug, nil)

	got, err := c.env.InnerRun.AttachDir()
	if err != nil {
		t.Fatalf("AttachDir() error = %v; want nil", err)
	}
	taskLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve task location: %v", err)
	}
	if got != taskLocation.AnchorPath() {
		t.Errorf("AttachDir() = %q; want %q", got, taskLocation.AnchorPath())
	}
}

// stepMarkWatched_HoldsTheMarkerOnlyWhileItsNoticesReachTheDriversParent drives the watched-marker seam over a real pair:
// the marker holds batten's pid for a pair created from the prime while the prime's orch state records a strand,
// and is removed when the strand is gone and when the pair's origin names another worktree.
func stepMarkWatched_HoldsTheMarkerOnlyWhileItsNoticesReachTheDriversParent(t *testing.T, h *hubforge.Hub) {
	slug := "batten-watched"
	hubforge.AddPair(t, h, slug)
	c := wireForHub(t, h, slug, nil)

	childLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve child location: %v", err)
	}
	marker := shedrun.BattenWatchedMarker(childLocation, shedrun.SelfRunID)
	orchPaths := orchcli.PrimePaths(h.Location)
	origin, found, err := fabricengine.ReadOriginFor(h.Location, slug)
	if err != nil || !found {
		t.Fatalf("read origin = %+v, found=%v, %v", origin, found, err)
	}
	setOrigin := func(parentWorktree string) {
		t.Helper()
		changed := origin
		changed.ParentWorktree = parentWorktree
		if err := fabricengine.WriteOrigin(fabricengine.NewMutations(""), h.Location, slug, changed); err != nil {
			t.Fatalf("write origin: %v", err)
		}
	}
	setStrand := func(strand string) {
		t.Helper()
		if err := orchengine.SaveState(orchPaths, orchengine.State{Strand: strand, Phase: orchengine.PhaseIdle}); err != nil {
			t.Fatalf("save orch state: %v", err)
		}
	}

	steps := []struct {
		name      string
		strand    string
		parent    string
		wantHeld  bool
		wantPidIn bool
	}{
		{name: "StrandRecordedAndPairFromThePrime", strand: "orch-strand", parent: origin.ParentWorktree, wantHeld: true, wantPidIn: true},
		{name: "PairFromAnotherWorktreeRemovesIt", strand: "orch-strand", parent: "another-worktree"},
		{name: "StrandRecordedAgainWritesItAgain", strand: "orch-strand", parent: origin.ParentWorktree, wantHeld: true, wantPidIn: true},
		{name: "NoStrandRecordedRemovesIt", strand: "", parent: origin.ParentWorktree},
	}
	for _, step := range steps {
		setStrand(step.strand)
		setOrigin(step.parent)
		held, err := c.env.InnerRun.MarkWatched(context.Background())
		if err != nil || held != step.wantHeld {
			t.Fatalf("%s: MarkWatched() = %v, %v; want held=%v", step.name, held, err, step.wantHeld)
		}
		raw, readErr := os.ReadFile(marker)
		if step.wantPidIn {
			if want := strconv.Itoa(os.Getpid()) + "\n"; readErr != nil || string(raw) != want {
				t.Errorf("%s: marker = %q, %v; want batten's pid %q", step.name, raw, readErr, want)
			}
		} else if !os.IsNotExist(readErr) {
			t.Errorf("%s: marker read = %q, %v; want it absent", step.name, raw, readErr)
		}
	}
}
