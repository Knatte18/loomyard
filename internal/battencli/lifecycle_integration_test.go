// lifecycle_integration_test.go is the end-to-end suite over a real hub;
// internal/hubforge builds that hub through its fabric fixture entry point, per the hubforge Fabric-Fixture Invariant.
// It is a white-box test in package battencli because most tests stub Env.InnerRun after a real wire() call.
// That stubbing needs the unexported wire method and the battenCLI receiver.
//
// The RealReadStatus_ steps and SeedChild_WritesASeedTheChildBootstrapAgreesWith keep those seams unstubbed.
// Nothing here spawns a real provider; such a run is costly and belongs in manual CLI driving.
//
// It sits at the integration tier because the package's own pre-run refusal performs a prime-name lookup:
// the lookup reaches a real git worktree listing, and getting there also needs the resolver.
// The Test Tier Purity Invariant bars both from untagged files.

package battencli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// shortPollBattenRecipe is a copy of contracts/recipes/batten-recipe.yaml with byte-for-byte the same rows.
// It differs only in poll_interval_s, which is 1 here and 2 in the recipe.
// That value reaches only the floor check.
// The tier1 steps build their shed from this copy.
// The integration tier exercises the assembled wiring:
// the recipe's config value flows through shedbuild into innerRunEntry.
// Battenshed's own untagged tests cover the InnerRun producer in isolation.
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
    on_done: Land
    on_stuck: Run-Shed
    config:
      poll_interval_s: 1
      driver_exit_grace_s: 900

  - name: Land
    engine: Land
    on_done: Worktree-Teardown

  - name: Worktree-Teardown
    engine: WorktreeTeardown
    on_done: ""
`

// wireForHub builds a *battenCLI wired for real against h's prime Location and slug:
// a real CreateWorktree and a real Teardown, both driving fabricengine's topology holder against h's hub.
// It then stubs Env.InnerRun: Spawn becomes a no-op, and a non-nil readStatus answers the status read.
//
// The Run-Shed check stats the child's status file,
// so a stubbed read moves that file into a temporary directory and also creates it when it is absent.
// A pass ends by itself, with a Yield while a stubbed child stays as it is, so no sleep or pause seam is replaced.
func wireForHub(t *testing.T, h *hubforge.Hub, slug string, readStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)) *battenCLI {
	t.Helper()
	// The board default layer holds every Command row's command empty,
	// so a stepped row passes on and no test runs the test binary through {lym}.
	writeBoardBattenCommands(t, h, nil)
	c := &battenCLI{location: h.Location}
	if err := c.wire(h.Location, slug); err != nil {
		t.Fatalf("wire(%s): %v", slug, err)
	}
	c.env.InnerRun.Spawn = func(ctx context.Context) error { return nil }
	c.env.InnerRun.ReadStatus = nil
	if readStatus != nil {
		// The child's status file lives outside the repository,
		// so the file the wait stats leaves nothing for a teardown to commit.
		childStatus := filepath.Join(t.TempDir(), "child-status.json")
		c.env.InnerRun.ResolveStatus = func() (string, string, error) { return childStatus, childStatus + ".lock", nil }
		c.env.InnerRun.ReadStatus = func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
			ensureStatusFile(t, statusPath)
			return readStatus(statusPath, statusLockPath)
		}
	}
	return c
}

// commandRowKeys are the command keys of batten.yaml's Command rows, in row order.
var commandRowKeys = []string{"open_window_command", "seed_ide_command", "open_ide_command"}

// writeBoardBattenCommands writes the hub's board default batten.yaml from battenshed's config template.
// Each Command row's command key is set to commands[key], or to an empty string when commands lacks the key.
func writeBoardBattenCommands(t *testing.T, h *hubforge.Hub, commands map[string]string) {
	t.Helper()
	lines := strings.Split(battenshed.ConfigTemplate(), "\n")
	for i, line := range lines {
		for _, key := range commandRowKeys {
			if strings.HasPrefix(line, key+": ") {
				lines[i] = key + ": " + strconv.Quote(commands[key])
			}
		}
	}
	path := configengine.ConfigFile(h.BoardDir(), "batten")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write the board default batten.yaml: %v", err)
	}
}

// ensureStatusFile creates the child's status file at path when it is absent.
// The file stands in for the one the real read decodes.
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

// seedEntryStatus writes c's status file with the given CurrentProducer and State.
// A nil history becomes an empty, non-nil slice.
// It writes under c's own StatusPath and StatusLockPath.
func seedEntryStatus(t *testing.T, c *battenCLI, producer string, rowState shedengine.State, history []shedengine.HistoryEntry) {
	t.Helper()
	if history == nil {
		history = []shedengine.HistoryEntry{}
	}
	// state.WriteJSON creates the directory of StatusPath, which is durable, but not that of StatusLockPath.
	// The lock's directory is ephemeral, so a fixture that seeds the file directly must create it itself.
	// A CLI pre-run would create it, but the seed runs before any pre-run does.
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

// seedBoardTask upserts a Board task named slug carrying recipeType as its "recipe" field.
// It fails the test on error.
// Seed-Child reads that field at Call time, never a value captured earlier, to choose the child worktree's recipe.
// So every test that walks the recipe from Worktree-Create through Seed-Child needs a seeded task first.
func seedBoardTask(t *testing.T, h *hubforge.Hub, slug, recipeType string) {
	t.Helper()
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	b := boardengine.New(cfg)
	if _, err := b.UpsertTask(map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []string{"bug"}, "recipe": recipeType}); err != nil {
		t.Fatalf("seed board task %q: %v", slug, err)
	}
	// A new task is a draft, which a fresh run refuses.
	if err := b.SetStatus(slug, nil); err != nil {
		t.Fatalf("release board task %q: %v", slug, err)
	}
}

// pathExists reports whether path exists on disk.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestBattenIntegration_RunReconcilesTheHubBeforeAndThePairAfterCreating pins both reconcile calls of `lym batten run`.
// With a fresh stamp the first call is a no-op,
// so the new pair losing the prime's retired key can only come from the unconditional pair call after the create row.
// Against an absent stamp, `arm` for run reconciles ahead of armAt's non-prime refusal and its auto-seed.
func TestBattenIntegration_RunReconcilesTheHubBeforeAndThePairAfterCreating(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "batten-reconcile-pair"
	stampPath := hubreconcile.Geometry{BoardDir: h.BoardDir()}.StampPath()
	stamp, err := json.Marshal(map[string]string{"build_key": hubreconcile.BuildKey(buildvcs.Running(), configreg.Fingerprint())})
	if err != nil {
		t.Fatalf("marshal stamp: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(stampPath), 0o755); err != nil {
		t.Fatalf("mkdir stamp dir: %v", err)
	}
	if err := os.WriteFile(stampPath, stamp, 0o644); err != nil {
		t.Fatalf("write stamp: %v", err)
	}

	batcher, _ := configreg.Lookup("batcher")
	retired := strings.Replace(batcher.Template(), "orientation: 31400", "master_base: 52000", 1)
	gitkit.CommitFile(t, h.PrimeRecords(), configengine.ConfigFileRel("batcher"), retired, "fixture: retired key")

	c := &battenCLI{location: h.Location}
	if err := c.wire(h.Location, slug); err != nil {
		t.Fatalf("wire(%s): %v", slug, err)
	}
	holds := &countingHold{}
	if err := c.env.CreateWorktree(context.Background(), holds.hold); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	// The hold reached Add and was released; a slug outside a beam takes none for a held member's removal.
	if holds.takes != 1 || holds.releases != 1 {
		t.Errorf("prime lock takes/releases after the create = %d/%d; want 1/1", holds.takes, holds.releases)
	}

	pair := h.PairCodeWorktree(slug)
	if err := c.env.CreateWorktree(context.Background(), holds.hold); err != nil {
		t.Fatalf("CreateWorktree re-entered on the complete pair: %v", err)
	}
	if holds.takes != 1 {
		t.Errorf("prime lock takes after the re-entry over the complete pair = %d; want no further take", holds.takes)
	}
	// The create row spawns the new pair's push once, and the re-entered row on the complete pair spawns none.
	if got := pairPushCounts.count(pair); got != 1 {
		t.Errorf("pair pushes spawned for %s = %d; want 1", pair, got)
	}
	if data, err := os.ReadFile(configengine.ConfigFile(pair, "batcher")); err != nil || strings.Contains(string(data), "master_base") {
		t.Errorf("new pair's batcher.yaml still carries the retired key (err %v)", err)
	}
	if status := gitkit.GitStatusPorcelain(t, h.PairRecordsSibling(slug)); status != "" {
		t.Errorf("new pair's records worktree is dirty after the create row:\n%s", status)
	}

	if err := os.Remove(stampPath); err != nil {
		t.Fatalf("remove stamp: %v", err)
	}
	_, err = (&battenCLI{}).arm(pair, "run", []string{slug})
	if err == nil {
		t.Fatal("arm for run from a pair error = nil; want the non-prime refusal")
	}
	if _, statErr := os.Stat(stampPath); statErr != nil {
		t.Errorf("stamp after arm for run: %v; want the reconcile to have run ahead of armAt's refusal", statErr)
	}
	pairLocation, err := lymcwd.Resolve(pair)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", pair, err)
	}
	for name, location := range map[string]*lymcwd.Location{"prime": h.Location, "pair": pairLocation} {
		if _, found, err := shedrun.ReadSeed(location, slug); err != nil || found {
			t.Errorf("%s holds a seed for %q (found %v, err %v); want none before the refusal", name, slug, found, err)
		}
	}
}

// gitShow runs `git show <spec>` in dir and returns its stdout, and it fails the test on error.
// Tests use it to assert that a path is committed, not merely written, on a pair's records worktree.
// An uncommitted seed is as lost to a fresh clone as one never written.
// That is the machine-switch case the Seed-Child row exists for.
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

// TestBattenIntegration_Rows runs the end-to-end rows over one hubforge hub.
// Each step gets its own slug, so its pair, branches and records stay apart from every other step's.
// The steps share only the hub itself: its prime worktree, records origin and board.
// A step that changes the hub for the next one restores it before returning.
// The last step is the exception: it dirties the prime worktree.
// It therefore relies on running after every step that creates a pair.
// The test calls t.Parallel, but no step does: the steps serialise on that one shared hub.
//
//testtiming:keep the start-cost step pins the start-cost bound on a Command row beyond its status commit and its process, a wall-clock bound no other test asserts
func TestBattenIntegration_Rows(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	steps := []struct {
		name string
		run  func(t *testing.T, h *hubforge.Hub)
	}{
		{"SeedChild_WritesASeedTheChildBootstrapAgreesWith", stepSeedChild_WritesASeedTheChildBootstrapAgreesWith},
		{"SeedChild_WriteSeedRewrapsARealDisagreeingChildSeed", stepSeedChild_WriteSeedRewrapsARealDisagreeingChildSeed},
		{"SeedChild_WritesTheRunSettingsIntoThePairsLocalLayer", stepSeedChild_WritesTheRunSettingsIntoThePairsLocalLayer},
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
		{"PairDirNamesTheTaskWorktree", stepPairDirNamesTheTaskWorktree},
		{"AwaitingChildDoneTeardown_ArchivesTheRunRecords", stepAwaitingChildDoneTeardown_ArchivesTheRunRecords},
		{"SeedChild_IgnoresPrimesCommittedBattenRecords", stepSeedChild_IgnoresPrimesCommittedBattenRecords},
		{"RealReadStatus_IgnoresPrimesCommittedStatusForTheSameSlug", stepRealReadStatus_IgnoresPrimesCommittedStatusForTheSameSlug},
		{"MarkWatched_HoldsTheMarkerOnlyWhileItsNoticesReachTheDriversParent", stepMarkWatched_HoldsTheMarkerOnlyWhileItsNoticesReachTheDriversParent},
		{"StopReport_ReadsTheParkMarkersContentAndTime", stepStopReport_ReadsTheParkMarkersContentAndTime},
		{"TaskWorktreeSeams_RunGitOnlyUntilTheFirstResolution", stepTaskWorktreeSeams_RunGitOnlyUntilTheFirstResolution},
		{"Status_FlagsAHaltedBattenOverADoneChild", stepStatusFlagsAHaltedBattenOverADoneChild},
		{"CommandRows_RunTheBoardDefaultCommandsWithEverySwitchOn", posixOnly(stepCommandRows_RunTheBoardDefaultCommandsWithEverySwitchOn)},
		{"CommandRows_IgnoreACommandSetInTheTasksTrackedLayer", posixOnly(stepCommandRows_IgnoreACommandSetInTheTasksTrackedLayer)},
		{"CommandRows_FollowTheTasksLocalSwitch", posixOnly(stepCommandRows_FollowTheTasksLocalSwitch)},
		{"CommandRows_ResumeAtARowAnOlderBinaryLeftStanding", posixOnly(stepCommandRows_ResumeAtARowAnOlderBinaryLeftStanding)},
		{"RunRowCommand_ReportsATimeoutAndAnExitCode", posixOnly(stepRunRowCommand_ReportsATimeoutAndAnExitCode)},
		{"StartCost_CommandRowsCostUnderTheBoundBeyondTheCommitAndTheProcess", posixOnly(stepStartCost_CommandRowsCostUnderTheBoundBeyondTheCommitAndTheProcess)},
		// It must run last: it leaves the prime dirty for every step after it.
		{"DirtyPrime_CreateRowBlocksBeforeAnythingCreated", stepDirtyPrime_CreateRowBlocksBeforeAnythingCreated},
	}
	for _, step := range steps {
		if step.run == nil {
			continue
		}
		if !t.Run(step.name, func(t *testing.T) { step.run(t, h) }) {
			return
		}
	}
}

// stepSeedChild_WritesASeedTheChildBootstrapAgreesWith drives Worktree-Create and Seed-Child for real.
// It then asserts the property the child's own bootstrap depends on:
// writing the seed the way that bootstrap will must be accepted by shedrun.WriteSeed, not refused as a disagreement.
//
// It rebuilds that seed's shape from the recorded origin rather than importing internal/loomcli,
// and it asserts the recorded parent is what lands in the seed's params.
// That keeps the shape from drifting into agreeing with itself while disagreeing with loom.
func stepSeedChild_WritesASeedTheChildBootstrapAgreesWith(t *testing.T, h *hubforge.Hub) {
	slug := "batten-seed-agrees"
	seedBoardTask(t, h, slug, "tapestry")

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
	// Batten's own run directory keeps the recipe the child was seeded with, which a done run needs once the pair is gone.
	if got, err := shedrun.ReadChildRecipe(h.Location, slug); err != nil || got != seed.Recipe {
		t.Errorf("batten's child-recipe record = %q, %v; want the child seed's recipe %q and nil", got, err, seed.Recipe)
	}
	if origin.ParentWorktree != h.Location.WorktreeName {
		t.Errorf("child origin ParentWorktree = %q; want the prime %q", origin.ParentWorktree, h.Location.WorktreeName)
	}
	if got := seed.Params["parent"]; got != origin.ParentBranch {
		t.Errorf("child seed params[parent] = %q; want the recorded parent branch %q", got, origin.ParentBranch)
	}

	// This seed is byte-for-byte what loom's own bootstrap writes on its first "lym loom start" in that worktree.
	// Writing it must be a no-op, never a refusal.
	bootstrapSeed := shedrun.Seed{
		Recipe: seed.Recipe,
		Driver: seed.Driver,
		Params: map[string]string{"parent": origin.ParentBranch},
	}
	if err := shedrun.WriteSeed(childLocation, shedrun.SelfRunID, bootstrapSeed); err != nil {
		t.Fatalf("the child's own bootstrap seed was refused against Seed-Child's seed: %v", err)
	}
}

// stepSeedChild_WriteSeedRewrapsARealDisagreeingChildSeed pins the rewrap in the WriteSeed closure (wire.go).
// The closure re-wraps shedrun.ErrDisagreeingSeed as battenshed.ErrDisagreeingChildSeed.
// It plants a real, pre-existing child seed that disagrees with what the closure is about to write.
// It then calls c.env.SeedChild.WriteSeed directly:
// the production closure runs, not a stub of the already-wrapped sentinel.
// A future edit that drops the rewrap fails here, not only in a test that never reaches the real shedrun.WriteSeed call.
func stepSeedChild_WriteSeedRewrapsARealDisagreeingChildSeed(t *testing.T, h *hubforge.Hub) {
	slug := "batten-seed-disagrees"
	seedBoardTask(t, h, slug, "tapestry")
	hubforge.AddPair(t, h, slug)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the disagreement refusal fires before any poll")
		return shedengine.Status{}, false, nil
	})

	childLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve child location: %v", err)
	}
	// The planted seed's driver of "llm" disagrees with the "go" driver that the closure below is about to write.
	// An operator's hand-seeded or otherwise pre-existing child seed might disagree in the same way.
	if err := shedrun.WriteSeed(childLocation, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeTapestry, Driver: shedrun.DriverLLM}); err != nil {
		t.Fatalf("plant a disagreeing child seed: %v", err)
	}

	err = c.env.SeedChild.WriteSeed(context.Background(), shedrun.RecipeTapestry, shedrun.DriverGo)
	if !errors.Is(err, battenshed.ErrDisagreeingChildSeed) {
		t.Fatalf("WriteSeed() error = %v; want it to wrap battenshed.ErrDisagreeingChildSeed", err)
	}
	if got, err := shedrun.ReadChildRecipe(h.Location, slug); err != nil || got != "" {
		t.Errorf("batten's child-recipe record = %q, %v after a refused seed; want none and nil", got, err)
	}
}

// stepRealReadStatus_OnAFreshPairReportsAbsentRatherThanErroring drives the real InnerRun seams:
// Env.InnerRun.ResolveStatus and Env.InnerRun.ReadStatus, against a freshly created pair that has never run.
//
// It asserts that a status file that does not exist yet reports found == false with a nil error.
// That result tells innerRunProducer.Call to spawn.
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

// stepFourRowRun_SeedsChildCommitsAndTearsDown runs the whole row list one row at a time.
// The spawn is a stub, and the read-status answers StateDone.
// The rows are Worktree-Create, Seed-Child, the Command rows, Run-Shed, Land and Worktree-Teardown.
// It asserts that the pair exists on disk after the create row,
// and that the child's own _lym/shed/<slug>/seed.json exists after the seed row.
// That seed names the Board task's own "type" as its recipe.
// It carries the pair's recorded parent branch as params.parent.
// The seed is committed, not merely written, on the child's own records pair.
// It asserts that the pair is gone after the teardown row.
// It runs once per recipe a child may run.
func stepFourRowRun_SeedsChildCommitsAndTearsDown(t *testing.T, h *hubforge.Hub) {
	for _, recipe := range []string{shedrun.RecipeTapestry, shedrun.RecipeDarn} {
		t.Run(recipe, func(t *testing.T) {
			fourRowRunForRecipe(t, h, recipe)
		})
	}
}

// fourRowRunForRecipe runs the whole row list over a Board task whose recipe is recipe.
func fourRowRunForRecipe(t *testing.T, h *hubforge.Hub, recipe string) {
	slug := "batten-four-row-" + recipe
	seedBoardTask(t, h, slug, recipe)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	seedEntryStatus(t, c, battenrecipe.NameWorktreeCreate, shedengine.StateRunning, nil)

	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}

	ctx := context.Background()
	pairPath := h.PairCodeWorktree(slug)

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
	if seed.Recipe != recipe {
		t.Errorf("child seed recipe = %q; want %q (the Board task's own type)", seed.Recipe, recipe)
	}
	origin, found, err := fabricengine.ReadOrigin(childLocation)
	if err != nil || !found {
		t.Fatalf("ReadOrigin(child) = %+v, %v, %v; want the pair's recorded origin", origin, found, err)
	}
	if got := seed.Params["parent"]; got == "" || got != origin.ParentBranch {
		t.Errorf("child seed params.parent = %q; want the pair's recorded parent branch %q", got, origin.ParentBranch)
	}

	recordsPath := h.PairRecordsSibling(slug)
	seedRel := filepath.ToSlash(shedrun.SeedRel(childLocation, shedrun.SelfRunID))
	committedData := gitShow(t, recordsPath, "HEAD:"+seedRel)
	if !bytes.Equal(committedData, seedData) {
		t.Errorf("committed seed at HEAD:%s = %q; want it to match the working-tree seed %q", seedRel, committedData, seedData)
	}

	for _, row := range []string{battenrecipe.NameOpenWindow, battenrecipe.NameSeedIDE} {
		step, err := shed.Step(ctx)
		if err != nil || step.Producer != row || step.Outcome != shedengine.Done {
			t.Fatalf("Step (%s) = producer %q outcome %q, %v; want %q Done", row, step.Producer, step.Outcome, err, row)
		}
	}

	// open_ide is off by default, so the Open-IDE entry gate skips that row and the same step runs Run-Shed.
	step, err := shed.Step(ctx)
	if err != nil || step.Producer != battenrecipe.NameRunShed {
		t.Fatalf("Step (Open-IDE gated off) = producer %q, %v; want %q", step.Producer, err, battenrecipe.NameRunShed)
	}
	if skipped := slices.IndexFunc(step.History, func(entry shedengine.HistoryEntry) bool {
		return entry.Producer == battenrecipe.NameOpenIDE && entry.Outcome == shedengine.OutcomeSkipped
	}); skipped < 0 {
		t.Errorf("history after the gated-off Open-IDE = %+v; want a skipped %s entry", step.History, battenrecipe.NameOpenIDE)
	}
	if !pathExists(pairPath) {
		t.Fatalf("pair does not exist after Run-Shed: %s", pairPath)
	}

	// The entry stands at a run status, as a run holds it.
	// Only the teardown request's landed flag can turn it done:
	// the stubbed child status holds no Finalize Done, and batten's own run is still running.
	boardConfig, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	boardConfig.Path = h.BoardDir()
	board := boardengine.New(boardConfig)
	runStatus := boardengine.RunStatus("running", "Run-Shed")
	if err := board.SetStatus(slug, &runStatus); err != nil {
		t.Fatalf("hold the board entry at a run status: %v", err)
	}

	// Land runs over a prime at its remote head with an empty landing deploy command, and leaves the pair for the teardown.
	landStep, err := shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step (Land): %v", err)
	}
	if landStep.Producer != battenrecipe.NameLand || landStep.Outcome != shedengine.Done {
		t.Fatalf("Land step = producer %q outcome %q reason %q; want %q Done", landStep.Producer, landStep.Outcome, landStep.Reason, battenrecipe.NameLand)
	}
	if !pathExists(pairPath) {
		t.Fatalf("pair does not exist after Land: %s", pairPath)
	}

	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (Worktree-Teardown): %v", err)
	}
	if pathExists(pairPath) {
		t.Errorf("pair still exists after Worktree-Teardown: %s", pairPath)
	}
	if task, found, err := board.GetTask(slug); err != nil || !found || task.Status == nil || *task.Status != "done" {
		t.Errorf("board entry after Worktree-Teardown = (%+v, %v, %v); want status done", task, found, err)
	}

	// Teardown's own top.Remove call passes remote: true.
	// A batten-driven teardown is the task's final removal, and nothing will ever re-adopt its records branch.
	// The remote copy must therefore not linger.
	// "lym fabric cleanup" enumerates local branches only, so it can never reach a remote copy.
	recordsBranch := fabricengine.RecordsBranchName(slug)
	if err := exec.Command("git", "-C", h.RecordsBare, "rev-parse", "--verify", "refs/heads/"+recordsBranch).Run(); err == nil {
		t.Errorf("records branch %q still present on the remote after teardown; want it deleted alongside the local copy", recordsBranch)
	}
}

// remoteBranchExists reports whether branch exists in the bare repository at bareDir.
func remoteBranchExists(bareDir, branch string) bool {
	return exec.Command("git", "-C", bareDir, "rev-parse", "--verify", "refs/heads/"+branch).Run() == nil
}

// stepTeardown_AlreadyGonePairFinishesItsBranchDeletion re-enters the removal half with both worktrees gone.
// It stands in for a removal killed after its worktree removals and before its branch deletions finished.
// It asserts the row deletes the records branch locally and on the remote, rather than reporting done over it.
// A branch left on the remote makes a later create of the slug refuse its push.
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
			recordsBranch := fabricengine.RecordsBranchName(slug)
			if !remoteBranchExists(h.RecordsBare, recordsBranch) {
				t.Fatalf("precondition: %q not on the remote after the create", recordsBranch)
			}
			// Both worktrees are removed and the remote copy kept: the state a kill between the local and remote deletions leaves.
			if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
				t.Fatalf("remove the pair without its remote copy: %v", err)
			}
			recordsRepoRoot, err := fabricengine.RecordsRepoRoot(h.Location)
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
			if remoteBranchExists(h.RecordsBare, recordsBranch) {
				t.Errorf("%q still on the remote after the re-entered teardown; want it deleted", recordsBranch)
			}
			if exec.Command("git", "-C", recordsRepoRoot, "rev-parse", "--verify", "refs/heads/"+recordsBranch).Run() == nil {
				t.Errorf("%q still present locally after the re-entered teardown; want it deleted", recordsBranch)
			}
		})
	}
}

// refuseRemoteBranchDeletions installs a pre-receive hook on the bare repo at bareDir.
// The hook rejects every branch deletion and accepts every other update.
// A teardown's archive-tag push then lands while its remote branch deletion fails.
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

// stepTeardown_FailedRemoteDeletionHaltsResumably makes the records origin refuse branch deletions during teardown.
// It asserts the removal half archives the records and removes the pair.
// It asserts the removal then reports the failed remote deletion instead of done.
// It then asserts a resume, once the remote accepts the deletion, finishes the teardown.
func stepTeardown_FailedRemoteDeletionHaltsResumably(t *testing.T, h *hubforge.Hub) {
	slug := "batten-remote-fails"
	hubforge.AddPair(t, h, slug)
	recordsBranch := fabricengine.RecordsBranchName(slug)
	restore := refuseRemoteBranchDeletions(t, h.RecordsBare)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	err := c.env.Teardown.Remove(context.Background())
	if err == nil {
		t.Fatal("Teardown.Remove() = nil with a remote refusing the deletion; want the failed remote deletion reported")
	}
	if !strings.Contains(err.Error(), "re-run lym batten run "+slug+" to retry the deletion") {
		t.Errorf("Teardown.Remove() error = %q; want it to name the resume", err.Error())
	}
	if pathExists(h.PairCodeWorktree(slug)) {
		t.Errorf("task worktree still present; want the pair removed before the remote deletion failed")
	}
	if !remoteArchiveTagExists(h.RecordsBare, slug) {
		t.Errorf("no archive/%s/ tag on the remote; want the records archived before the pair was removed", slug)
	}

	restore()
	if err := c.env.Teardown.Remove(context.Background()); err != nil {
		t.Fatalf("Teardown.Remove() on resume = %v; want nil", err)
	}
	if remoteBranchExists(h.RecordsBare, recordsBranch) {
		t.Errorf("%q still on the remote after the resumed teardown; want it deleted", recordsBranch)
	}
}

// stepTeardown_UnreachableRemoteHaltsBeforeRemovalResumably breaks the records origin before teardown.
// It asserts the removal half halts on the failed archive.
// The local teardown is done ahead of the archive, and the resume is named.
// It then asserts a resume, once the remote is reachable again, archives the records and finishes the teardown.
func stepTeardown_UnreachableRemoteHaltsBeforeRemovalResumably(t *testing.T, h *hubforge.Hub) {
	slug := "batten-remote-unreachable"
	hubforge.AddPair(t, h, slug)
	recordsBranch := fabricengine.RecordsBranchName(slug)
	recordsRepoRoot, err := fabricengine.RecordsRepoRoot(h.Location)
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
	if !strings.Contains(err.Error(), "lym batten run "+slug) {
		t.Errorf("Teardown.Remove() error = %q; want it to name the resume", err.Error())
	}
	if pathExists(h.PairCodeWorktree(slug)) {
		t.Errorf("task worktree present; want the local teardown done before the remote archive failed")
	}

	gitkit.MustRun(t, recordsRepoRoot, "git", "remote", "set-url", "origin", h.RecordsBare)
	if err := c.env.Teardown.Remove(context.Background()); err != nil {
		t.Fatalf("Teardown.Remove() on resume = %v; want nil", err)
	}
	if pathExists(h.PairCodeWorktree(slug)) {
		t.Errorf("task worktree still present after the resumed teardown; want it removed")
	}
	if !remoteArchiveTagExists(h.RecordsBare, slug) {
		t.Errorf("no archive/%s/ tag on the remote after the resumed teardown; want the records archived", slug)
	}
	if remoteBranchExists(h.RecordsBare, recordsBranch) {
		t.Errorf("%q still on the remote after the resumed teardown; want it deleted", recordsBranch)
	}
}

// stepTeardown_AlreadyGonePairWithNoBranchIsDone re-enters the removal half after a removal finished.
// That removal finished completely, but its transition was not yet persisted.
// It asserts the row reports done.
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

// stepStepDrivenRunShed_ReturnsOnTheChildsStateChange proves that Run-Shed makes one check per pass through a step.
// A running child makes every "lym batten step" yield without blocking or persisting anything.
// The step after the child's state changes ends Stuck, naming the change.
//
// The steps run on the real clock with no sleep.
// A pass returns at once, and the child's state change, not a poll interval, ends a pass with a verdict.
func stepStepDrivenRunShed_ReturnsOnTheChildsStateChange(t *testing.T, h *hubforge.Hub) {
	slug := "batten-step-reentrant"
	// The pair must already exist on disk.
	// InnerRun's ResolveStatus resolves the child worktree's own *lymcwd.Location, which needs a real git worktree there.
	// The pair stands in for a Worktree-Create that already completed on an earlier step.
	hubforge.AddPair(t, h, slug)

	childState := shedengine.StateRunning
	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: childState, CurrentProducer: "Plan-Review"}, true, nil
	})
	// The Board task's Seed-Child row is not exercised by this test.
	// The seeded status makes Run-Shed the current producer.
	// It stands in for a Worktree-Create and Seed-Child that completed on an earlier step.
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

	// Passes over the running child yield, leaving the run at Run-Shed, running, with nothing added to its history.
	for pass := 0; pass < 3; pass++ {
		result, err := shed.Step(context.Background())
		if err != nil {
			t.Fatalf("Step %d: %v", pass, err)
		}
		if result.Outcome != shedengine.Yield || result.Next != battenrecipe.NameRunShed || result.State != shedengine.StateRunning || len(result.History) != 2 {
			t.Fatalf("Step %d = outcome %q next %q state %q history %d; want a Yield at Run-Shed, running, with the two seeded entries", pass, result.Outcome, result.Next, result.State, len(result.History))
		}
	}

	// The child blocks, and the next pass ends on the change.
	childState = shedengine.StateBlocked
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(childStatusPath, later, later); err != nil {
		t.Fatalf("rewrite the child's status file: %v", err)
	}
	result, err := shed.Step(context.Background())
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
}

// stepRunShedPausedChild_WaitsThenTearsDownOnceDone drives a child through paused, resumed and done.
// It asserts each change ends a Run-Shed step with the pair intact.
// It then asserts the run reaches Worktree-Teardown, which removes the pair.
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
	// A minute passes between two passes on the row's clock, so every pass is a probe check and a done child's driver is read.
	clockStart := time.Now()
	passes := 0
	c.env.InnerRun.Now = func() time.Time { return clockStart.Add(time.Duration(passes) * time.Minute) }
	seedEntryStatus(t, c, battenrecipe.NameRunShed, shedengine.StateRunning, []shedengine.HistoryEntry{
		{Producer: battenrecipe.NameWorktreeCreate, Outcome: shedengine.Done},
		{Producer: battenrecipe.NameSeedChild, Outcome: shedengine.Done},
	})

	shed, err := shedbuild.NewShed([]byte(shortPollBattenRecipe), c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("shedbuild.NewShed: %v", err)
	}
	ctx := context.Background()
	pairPath := h.PairCodeWorktree(slug)

	step := func() shedengine.StepResult {
		t.Helper()
		passes++
		res, err := shed.Step(ctx)
		if err != nil {
			t.Fatalf("Step %d (Run-Shed over a changing child): %v", passes, err)
		}
		return res
	}
	if res := step(); res.Outcome != shedengine.Yield || res.Next != battenrecipe.NameRunShed || res.State != shedengine.StateRunning {
		t.Fatalf("first Step = outcome %q next %q state %q; want a Yield over the paused child", res.Outcome, res.Next, res.State)
	}
	// Each move rewrites the child's status file, so the next step ends on the resume and the one after on the finish.
	moves := []shedengine.State{shedengine.StateRunning, shedengine.StateDone}
	for i, wantOutput := range []string{"child paused → running at loom-side-producer", "child running → done at loom-side-producer"} {
		childState, moves = moves[0], moves[1:]
		at := time.Now().Add(time.Duration(i+1) * time.Hour)
		if err := os.Chtimes(childStatusPath, at, at); err != nil {
			t.Fatalf("rewrite the child's status file: %v", err)
		}
		res := step()
		if res.Outcome != shedengine.Stuck || res.Next != battenrecipe.NameRunShed || res.State != shedengine.StateRunning || res.Output != wantOutput {
			t.Errorf("Step %d = outcome %q next %q state %q output %q; want a Stuck self-route that stays running with output %q", i, res.Outcome, res.Next, res.State, res.Output, wantOutput)
		}
		if !pathExists(pairPath) {
			t.Fatalf("pair does not exist after step %d; want it left intact: %s", i, pairPath)
		}
	}

	for i := 0; i < 5 && pathExists(pairPath); i++ {
		step()
	}
	if pathExists(pairPath) {
		t.Errorf("pair still exists after the child reached done; want Worktree-Teardown to remove it: %s", pairPath)
	}
}

// stepCreateRow_IsIdempotentAgainstAnAlreadyPresentWorktree proves the create row is idempotent.
// A task worktree that already exists satisfies the row's post-condition,
// so the row must report done and let the run advance rather than asking fabric to create it twice.
//
// The state it reconstructs is the one left when a process is killed after Topology.Add succeeds.
// shedengine has not yet persisted the transition: the worktree is on disk and the status still names the create row.
// Without the probe, the row takes fabric's pre-existing-branch refusal.
// Both remedies that refusal names also refuse in this state, which leaves the run unresumable.
func stepCreateRow_IsIdempotentAgainstAnAlreadyPresentWorktree(t *testing.T, h *hubforge.Hub) {
	slug := "batten-create-idempotent"
	seedBoardTask(t, h, slug, "tapestry")
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
	if !pathExists(h.PairCodeWorktree(slug)) {
		t.Errorf("task worktree missing after the idempotent create row: %s", h.PairCodeWorktree(slug))
	}
}

// stepCreateRow_PairWithoutOriginRecordIsIncomplete pins that the create row refuses a pair with no origin record.
// Such a pair is left by a SIGKILL between junction wiring and the origin record's write.
// Reporting done at that point seeds the child with no parent param,
// and the child's bootstrap refuses that seed on every resume.
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

	err := c.env.CreateWorktree(context.Background(), holdNothing)
	if err == nil {
		t.Fatal("CreateWorktree() error = nil; want the incomplete-pair refusal for a pair with no origin record")
	}
	if !strings.Contains(err.Error(), "not fully created") {
		t.Errorf("CreateWorktree() error = %q; want it to contain %q", err.Error(), "not fully created")
	}
}

// stepCreateRow_LeftoverBranchIsRewordedForPrime pins createRefusal's wiring into the CreateWorktree closure.
// A leftover code branch from an earlier torn-down pair must reach the caller worded for an operator in prime.
// Fabric's raw "lym fabric checkout" advice would switch prime itself onto the task's branch, so it must not appear.
// It calls c.env.CreateWorktree, the production closure, rather than createRefusal alone,
// so an edit that drops the reword call fails here.
func stepCreateRow_LeftoverBranchIsRewordedForPrime(t *testing.T, h *hubforge.Hub) {
	slug := "batten-leftover-branch"
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", slug)
	seedBoardTask(t, h, slug, "tapestry")

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the create row must fail before the poll row ever runs")
		return shedengine.Status{}, false, nil
	})
	c.owner = hubOwnerSeams(h.Location)

	err := c.env.CreateWorktree(context.Background(), holdNothing)
	if err == nil {
		t.Fatal("CreateWorktree() error = nil; want the leftover-branch refusal")
	}
	// The claim written before the pair is created stays on the refused row.
	board, openErr := boardengine.OpenHub(h.Location.HubPath)
	if openErr != nil {
		t.Fatalf("OpenHub: %v", openErr)
	}
	task, found, getErr := board.GetTask(slug)
	if getErr != nil || !found || task.Status == nil || *task.Status != "running · Worktree-Create" {
		t.Errorf("board entry after the refused row = %+v, found %v, err %v; want status %q", task, found, getErr, "running · Worktree-Create")
	}
	var precondition *battenshed.PreconditionError
	if !errors.As(err, &precondition) {
		t.Errorf("CreateWorktree() error = %v; want a *battenshed.PreconditionError", err)
	}
	if strings.Contains(err.Error(), "lym fabric checkout") {
		t.Errorf("CreateWorktree() error = %q; want it to never name \"lym fabric checkout\" from prime", err.Error())
	}
	for _, want := range []string{slug, "lym batten cancel " + slug + " --remove"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CreateWorktree() error = %q; want it to contain %q", err.Error(), want)
		}
	}
}

// stepCreateRow_IncompletePairRefusesRatherThanSkippingAdd pins that a half-made pair is refused as incomplete.
// That pair is a code worktree and branch left by a SIGKILL after Add's first step: no sibling, no junctions.
// The test makes that state with the git command Add's createGitWorktree issues, rather than a stub,
// so the completeness check runs against the real filesystem.
func stepCreateRow_IncompletePairRefusesRatherThanSkippingAdd(t *testing.T, h *hubforge.Hub) {
	slug := "batten-incomplete-pair"
	target := h.PairCodeWorktree(slug)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "add", "-b", slug, target)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the create row must refuse before the poll row ever runs")
		return shedengine.Status{}, false, nil
	})

	err := c.env.CreateWorktree(context.Background(), holdNothing)
	if err == nil {
		t.Fatal("CreateWorktree() error = nil; want a refusal naming the incomplete pair, not silent success")
	}
	for _, want := range []string{slug, "not fully created", "lym fabric remove --force " + slug, "git branch -D " + slug} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CreateWorktree() error = %q; want it to contain %q", err.Error(), want)
		}
	}
	// The refusal is read-only: it makes no repair attempt and leaves no partial Add further along than where it started.
	if pathExists(h.PairRecordsSibling(slug)) {
		t.Errorf("the pair's other-side worktree exists after the refusal; want CreateWorktree to have made no repair attempt")
	}
}

// TestBattenIntegration_CreateRow_IncompletePairRemedyWorksVerbatimOnAPrefixedHub follows the remedy as worded.
// It runs from prime after a SIGKILL lands once Add has created the pair's other side.
// The remedy must name the prefixed branch Add created.
// Doing what it says must leave nothing that makes the resumed create refuse.
func TestBattenIntegration_CreateRow_IncompletePairRemedyWorksVerbatimOnAPrefixedHub(t *testing.T) {
	tiergate.Require(t, 1)
	// Not a step of TestBattenIntegration_Rows.
	// Seeding a branch prefix into the fabric config changes the hub for every pair created after it.
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.SeedFabricConfig(t, h, "branch_prefix: r4/\npathspec: \"\"\n")
	slug := "batten-incomplete-prefixed"
	branch := "r4/" + slug
	target := h.PairCodeWorktree(slug)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "add", "-b", branch, target)

	// Stands in for Add's later step, which creates the other side's worktree.
	// It runs directly rather than through Add, so the junctions this test needs stay missing.
	recordsRepoRoot, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("resolve records repo root: %v", err)
	}
	gitkit.MustRun(t, recordsRepoRoot, "git", "worktree", "add", "-b", fabricengine.RecordsBranchName(branch), h.PairRecordsSibling(slug))

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called by the create row")
		return shedengine.Status{}, false, nil
	})

	err = c.env.CreateWorktree(context.Background(), holdNothing)
	if err == nil {
		t.Fatal("CreateWorktree() error = nil; want the incomplete-pair refusal")
	}
	removeCommand := "lym fabric remove --force " + slug
	branchCommand := "git branch -D " + branch
	for _, want := range []string{removeCommand, branchCommand} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("CreateWorktree() error = %q; want it to contain %q", err.Error(), want)
		}
	}

	// The remedy, verbatim: the fabric verb's own engine call with --force, then the branch, only if it survived the removal.
	cfg, err := fabricengine.LoadConfig(configengine.Dirs{Board: fabricengine.BoardDir(h.Location.HubPath)})
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

	if err := c.env.CreateWorktree(context.Background(), holdNothing); err != nil {
		t.Fatalf("CreateWorktree() after the remedy = %v; want the fresh create to succeed", err)
	}
	if !pathExists(h.PairRecordsSibling(slug)) {
		t.Errorf("the pair's other side is missing after the resumed create: %s", h.PairRecordsSibling(slug))
	}
}

// stepDirtyPrime_CreateRowBlocksBeforeAnythingCreated dirties a tracked file in the hub's prime worktree.
// It asserts that the create row halts blocked before anything is created.
// This refusal fires on every batten run, and the unit tests' fakes do not reproduce it.
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
	if pathExists(h.PairCodeWorktree(slug)) {
		t.Errorf("pair exists even though the create row blocked before creating anything: %s", h.PairCodeWorktree(slug))
	}
}

// newCommandRowFixture prepares a pair for slug and a wired battenCLI.
// The board default batten.yaml holds one command per row in commandRowKeys.
// Each command writes the row's marker file with the slug.
// It returns the CLI, the marker path of each command key and the pair's code worktree.
// The board default is set after the wiring, since the executable lookup reads it when the first row runs.
// It is emptied again when the step ends.
func newCommandRowFixture(t *testing.T, h *hubforge.Hub, slug string) (*battenCLI, map[string]string, string) {
	t.Helper()
	seedBoardTask(t, h, slug, "tapestry")
	hubforge.AddPair(t, h, slug)
	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})

	markerDir := t.TempDir()
	markers := map[string]string{}
	commands := map[string]string{}
	for _, key := range commandRowKeys {
		markers[key] = filepath.Join(markerDir, key)
		commands[key] = "printf %s {slug} > " + markers[key]
	}
	writeBoardBattenCommands(t, h, commands)
	t.Cleanup(func() { writeBoardBattenCommands(t, h, nil) })
	return c, markers, h.PairCodeWorktree(slug)
}

// writeLocalBatten writes the task worktree's local batten.yaml layer, as Seed-Child writes a task's run_settings.
func writeLocalBatten(t *testing.T, pair, document string) {
	t.Helper()
	path := configengine.LocalFile(pair, "batten")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatalf("write the local batten.yaml: %v", err)
	}
}

// readCommandMarker returns the content of the marker file at path and whether it exists.
func readCommandMarker(t *testing.T, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read marker %s: %v", path, err)
	}
	return string(data), true
}

// stepCommandRow resumes a fresh shed at row and steps it once, fataling unless that row ends Done.
func stepCommandRow(t *testing.T, c *battenCLI, row string) {
	t.Helper()
	seedEntryStatus(t, c, row, shedengine.StateRunning, nil)
	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}
	step, err := shed.Step(context.Background())
	if err != nil || step.Producer != row || step.Outcome != shedengine.Done {
		t.Fatalf("Step (%s) = producer %q outcome %q, %v; want %q Done", row, step.Producer, step.Outcome, err, row)
	}
}

// stepCommandRows_RunTheBoardDefaultCommandsWithEverySwitchOn steps the three command rows with their switches on.
// Each row must write its own marker.
func stepCommandRows_RunTheBoardDefaultCommandsWithEverySwitchOn(t *testing.T, h *hubforge.Hub) {
	const slug = "batten-command-rows"
	c, markers, pair := newCommandRowFixture(t, h, slug)
	writeLocalBatten(t, pair, "open_ide: true\n")

	seedEntryStatus(t, c, battenrecipe.NameOpenWindow, shedengine.StateRunning, nil)
	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}
	for _, row := range []string{battenrecipe.NameOpenWindow, battenrecipe.NameSeedIDE, battenrecipe.NameOpenIDE} {
		step, err := shed.Step(context.Background())
		if err != nil || step.Producer != row || step.Outcome != shedengine.Done {
			t.Fatalf("Step (%s) = producer %q outcome %q, %v; want %q Done", row, step.Producer, step.Outcome, err, row)
		}
	}
	for _, key := range commandRowKeys {
		if got, found := readCommandMarker(t, markers[key]); !found || got != slug {
			t.Errorf("marker of %s = %q (found %v); want %q", key, got, found, slug)
		}
	}
}

// stepCommandRows_IgnoreACommandSetInTheTasksTrackedLayer asserts a command set in the tracked layer does not run.
// The board default's command still runs, and the ignored value is warned about.
func stepCommandRows_IgnoreACommandSetInTheTasksTrackedLayer(t *testing.T, h *hubforge.Hub) {
	const slug = "batten-command-tracked"
	c, markers, pair := newCommandRowFixture(t, h, slug)
	trackedMarker := filepath.Join(t.TempDir(), "tracked")
	tracked := "open_window_command: " + strconv.Quote("printf %s tracked > "+trackedMarker) + "\n"
	if err := os.WriteFile(configengine.ConfigFile(pair, "batten"), []byte(tracked), 0o644); err != nil {
		t.Fatalf("write the tracked batten.yaml: %v", err)
	}
	logs := logcapture.Capture(t)

	stepCommandRow(t, c, battenrecipe.NameOpenWindow)

	if got, found := readCommandMarker(t, markers["open_window_command"]); !found || got != slug {
		t.Errorf("board default marker = %q (found %v); want %q", got, found, slug)
	}
	if got, found := readCommandMarker(t, trackedMarker); found {
		t.Errorf("the tracked layer's command ran and wrote %q; want it ignored", got)
	}
	if !strings.Contains(logs.String(), "ignores a command set in another layer") {
		t.Errorf("trace = %q; want the ignored-value warning", logs.String())
	}
}

// stepGatedOffCommandRow steps row once from a running status, with pause_before set to next, its on_done row.
// It fatals unless the history ends in a skipped entry for row and the run is paused at next, so no later row runs.
func stepGatedOffCommandRow(t *testing.T, c *battenCLI, row, next string) {
	t.Helper()
	seedEntryStatus(t, c, row, shedengine.StateRunning, nil)
	err := state.UpdateJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
		cur.PauseBefore = next
		return cur, nil
	})
	if err != nil {
		t.Fatalf("record pause_before %s: %v", next, err)
	}
	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}
	step, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step (%s gated off) = %v; want nil", row, err)
	}
	if n := len(step.History); n == 0 || step.History[n-1].Producer != row || step.History[n-1].Outcome != shedengine.OutcomeSkipped {
		t.Errorf("Step (%s gated off) history = %+v; want it to end in a skipped %s entry", row, step.History, row)
	}
	if step.State != shedengine.StatePaused || step.Next != next || step.Producer != "" {
		t.Errorf("Step (%s gated off) = producer %q, state %q, next %q; want no producer called and the run paused at %q", row, step.Producer, step.State, step.Next, next)
	}
}

// stepCommandRows_FollowTheTasksLocalSwitch asserts Open-IDE runs its command with open_ide on.
// The switches are read from the task worktree's local layer.
// With them off, Open-IDE and Open-Window are each skipped by their entry gates.
func stepCommandRows_FollowTheTasksLocalSwitch(t *testing.T, h *hubforge.Hub) {
	const slug = "batten-command-switch"
	c, markers, pair := newCommandRowFixture(t, h, slug)

	writeLocalBatten(t, pair, "open_ide: true\n")
	stepCommandRow(t, c, battenrecipe.NameOpenIDE)
	if got, found := readCommandMarker(t, markers["open_ide_command"]); !found || got != slug {
		t.Errorf("Open-IDE marker with open_ide on = %q (found %v); want %q", got, found, slug)
	}
	if err := os.Remove(markers["open_ide_command"]); err != nil {
		t.Fatalf("remove the Open-IDE marker: %v", err)
	}

	writeLocalBatten(t, pair, "open_ide: false\nopen_window: false\n")
	stepGatedOffCommandRow(t, c, battenrecipe.NameOpenIDE, battenrecipe.NameRunShed)
	if got, found := readCommandMarker(t, markers["open_ide_command"]); found {
		t.Errorf("Open-IDE marker with open_ide off = %q; want none", got)
	}

	stepGatedOffCommandRow(t, c, battenrecipe.NameOpenWindow, battenrecipe.NameSeedIDE)
	if got, found := readCommandMarker(t, markers["open_window_command"]); found {
		t.Errorf("Open-Window marker with open_window off = %q; want none", got)
	}
}

// stepCommandRows_ResumeAtARowAnOlderBinaryLeftStanding asserts a run resumes at its standing row.
// That row is Open-Window or Seed-IDE, its commit marker names it, and it writes its own marker first.
func stepCommandRows_ResumeAtARowAnOlderBinaryLeftStanding(t *testing.T, h *hubforge.Hub) {
	rows := []struct{ row, key string }{
		{battenrecipe.NameOpenWindow, "open_window_command"},
		{battenrecipe.NameSeedIDE, "seed_ide_command"},
	}
	for _, tt := range rows {
		t.Run(tt.row, func(t *testing.T) {
			slug := "batten-cmd-resume-" + strings.ToLower(tt.row)
			c, markers, _ := newCommandRowFixture(t, h, slug)

			markerPath := shedrun.LastCommitMarker(h.Location, slug)
			if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", filepath.Dir(markerPath), err)
			}
			if err := state.WriteJSON(markerPath, markerPath+".lock", commitStatusMarker{Producer: tt.row, State: string(shedengine.StateRunning)}); err != nil {
				t.Fatalf("write the commit marker: %v", err)
			}

			stepCommandRow(t, c, tt.row)
			if got, found := readCommandMarker(t, markers[tt.key]); !found || got != slug {
				t.Errorf("%s marker after the resume = %q (found %v); want %q", tt.row, got, found, slug)
			}
		})
	}
}

// startCostBound bounds what a Command row may cost beyond its status commit and its command's own process.
// It catches a row that waits tens of seconds on a remote or a fixed delay while the row's own work takes milliseconds.
// It sits far above what scheduling on a loaded machine adds to the row's own work.
const startCostBound = 30 * time.Second

// stepStartCost_CommandRowsCostUnderTheBoundBeyondTheCommitAndTheProcess asserts each cost is under startCostBound.
// A row's cost is its step's wall time less the status commit and the command's own process.
// History timestamps have one-second resolution, so the test times each step directly.
// The commit is timed by wrapping the CommitStatus seam, and the process by one run through runRowCommand.
func stepStartCost_CommandRowsCostUnderTheBoundBeyondTheCommitAndTheProcess(t *testing.T, h *hubforge.Hub) {
	const slug = "batten-start-cost"
	c, _, _ := newCommandRowFixture(t, h, slug)
	marker := filepath.Join(t.TempDir(), "open-window-start")
	command := "date +%s%N > " + marker + "; sleep 0 &"
	writeBoardBattenCommands(t, h, map[string]string{"open_window_command": command})

	var commitTime time.Duration
	commitStatus := c.shedPaths.CommitStatus
	c.shedPaths.CommitStatus = func(producer, st string) error {
		began := time.Now()
		defer func() { commitTime += time.Since(began) }()
		return commitStatus(producer, st)
	}

	processBegan := time.Now()
	if result, err := runRowCommand(context.Background(), t.TempDir(), command, filepath.Join(t.TempDir(), "process.log"), time.Minute); err != nil || result.ExitCode != 0 {
		t.Fatalf("runRowCommand(%q) = %+v, %v; want exit code 0 and nil", command, result, err)
	}
	processTime := time.Since(processBegan)
	if err := os.Remove(marker); err != nil {
		t.Fatalf("remove the marker the process run wrote: %v", err)
	}

	rows := []struct {
		row     string
		process time.Duration
	}{
		{battenrecipe.NameSeedIDE, 0},
		{battenrecipe.NameOpenWindow, processTime},
	}
	for _, tt := range rows {
		seedEntryStatus(t, c, tt.row, shedengine.StateRunning, nil)
		shed, err := battenrecipe.New(c.env, c.shedPaths)
		if err != nil {
			t.Fatalf("battenrecipe.New: %v", err)
		}
		commitTime = 0
		began := time.Now()
		step, err := shed.Step(context.Background())
		elapsed := time.Since(began)
		if err != nil || step.Producer != tt.row || step.Outcome != shedengine.Done {
			t.Fatalf("Step (%s) = producer %q outcome %q, %v; want %q Done", tt.row, step.Producer, step.Outcome, err, tt.row)
		}
		if cost := elapsed - commitTime - tt.process; cost >= startCostBound {
			t.Errorf("%s cost %v beyond its commit (%v) and process (%v); want under %v", tt.row, cost, commitTime, tt.process, startCostBound)
		}
		t.Logf("%s: step %v, commit %v, process %v", tt.row, elapsed, commitTime, tt.process)
	}
	if _, found := readCommandMarker(t, marker); !found {
		t.Errorf("the Open-Window command wrote no marker at %s", marker)
	}
}

// stepRunRowCommand_ReportsATimeoutAndAnExitCode asserts runRowCommand ends a command at its timeout.
// The command must end well before it would finish, and a non-zero exit code must be reported.
// The sleep outlasts the minute the test allows, so a run that waits on it fails however loaded the machine.
func stepRunRowCommand_ReportsATimeoutAndAnExitCode(t *testing.T, h *hubforge.Hub) {
	dir := t.TempDir()

	start := time.Now()
	result, err := runRowCommand(context.Background(), dir, "sleep 300", filepath.Join(dir, "sleep.log"), 200*time.Millisecond)
	if err != nil || !result.TimedOut {
		t.Fatalf("runRowCommand(sleep 300, 200ms) = %+v, %v; want TimedOut and nil", result, err)
	}
	if elapsed := time.Since(start); elapsed > time.Minute {
		t.Errorf("runRowCommand(sleep 300, 200ms) took %v; want it ended well before the sleep", elapsed)
	}

	result, err = runRowCommand(context.Background(), dir, "exit 3", filepath.Join(dir, "exit.log"), time.Minute)
	if err != nil || result.TimedOut || result.ExitCode != 3 {
		t.Errorf("runRowCommand(exit 3) = %+v, %v; want exit code 3 and nil", result, err)
	}
}

// stepMidListResume_SkipsTheCompletedCreateRow asserts a resumed run does not create the pair again.
// The pair is created directly, standing in for a create row that completed before a crash.
// The status is seeded at Run-Shed, and the pair must be torn down once the poll row answers Done.
func stepMidListResume_SkipsTheCompletedCreateRow(t *testing.T, h *hubforge.Hub) {
	slug := "batten-resume"
	hubforge.AddPair(t, h, slug)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	c.env.CreateWorktree = func(context.Context, battenshed.HoldPrimeLock) error {
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
	if pathExists(h.PairCodeWorktree(slug)) {
		t.Errorf("pair still exists after the resumed run's teardown row completed: %s", h.PairCodeWorktree(slug))
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; output: %s", err, out.String())
	}
	// The resumed run's persisted history already holds the two pre-seeded entries (create, seed).
	// It also holds whatever this invocation appended, so history_length is at least 3.
	gotLen, ok := envelope["history_length"].(float64)
	if !ok {
		t.Fatalf("envelope[\"history_length\"] = %v (%T); want a number", envelope["history_length"], envelope["history_length"])
	}
	if gotLen < 3 {
		t.Errorf("envelope[\"history_length\"] = %v; want at least 3 (the two pre-seeded entries plus this run's own)", gotLen)
	}
}

// stepNonPrimeRefusal asserts all four verbs refuse when run from a task worktree.
// The refusal exits 1 and names both the task worktree and the prime worktree.
func stepNonPrimeRefusal(t *testing.T, h *hubforge.Hub) {
	taskSlug := "batten-task-cwd"
	hubforge.AddPair(t, h, taskSlug)

	taskCwd := h.PairCodeWorktree(taskSlug)
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

// stepRecordsPrimeRefusal asserts all four verbs refuse when run from the records prime.
// The records sibling is a repository whose own prime is itself, so a name comparison alone admits it.
// Each verb must refuse before arming anything, and the refusal must name the records sibling.
func stepRecordsPrimeRefusal(t *testing.T, h *hubforge.Hub) {
	recordsPrimeCwd := h.PrimeRecords()

	for _, verb := range []string{"run", "step", "status", "pause"} {
		t.Run(verb, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := RunCLIIn(recordsPrimeCwd, &out, []string{verb, "some-slug"})

			if exitCode != 1 {
				t.Fatalf("RunCLIIn(%s) from the records prime exit code = %d; want 1; output: %s", verb, exitCode, out.String())
			}
			if !strings.Contains(out.String(), "records sibling") {
				t.Errorf("%s refusal = %q; want it to name the records sibling", verb, out.String())
			}
			if !strings.Contains(out.String(), "this verb runs from the hub's prime worktree or a beam") {
				t.Errorf("%s refusal = %q; want the prime-or-beam wording", verb, out.String())
			}
		})
	}

	if pathExists(filepath.Join(recordsPrimeCwd, "_lym", "shed", "some-slug")) {
		t.Errorf("a run directory was seeded under the records prime; the refusal must land before the auto-seed")
	}
}

// stepTeardown_ReEntryAfterCompletedRemovalIsDone asserts Shutdown and Remove return nil on a second run.
// A process killed right after the removal leaves this state.
// shedengine persists the row's transition only after the producer returns.
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
	if pathExists(h.PairCodeWorktree(slug)) {
		t.Error("task worktree still present after the teardown; want it removed")
	}
}

// stepTeardown_SiblingDirtOutsideRecordPathsRefusesShutdown asserts Shutdown refuses a dirty sibling worktree.
// The dirt lies outside the record pathspec, and the refusal is fabric's sibling-dirty error.
// It never names --force and leaves the pair in place.
func stepTeardown_SiblingDirtOutsideRecordPathsRefusesShutdown(t *testing.T, h *hubforge.Hub) {
	slug := "batten-sibling-dirt"
	hubforge.AddPair(t, h, slug)
	stray := filepath.Join(fabricengine.RecordsWorktreePath(h.Location, slug), "stray.txt")
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
	if !pathExists(h.PairCodeWorktree(slug)) {
		t.Error("task worktree is gone after the refused shutdown; want the pair left in place")
	}
	if !pathExists(stray) {
		t.Error("the sibling's stray file is gone after the refused shutdown; want the sibling untouched")
	}
}

// stepPairDirNamesTheTaskWorktree pins InnerRun.PairDir to the task worktree's root.
// PairDir resolves on each call over a real pair anchored at a subdirectory, so the root and the anchor differ.
func stepPairDirNamesTheTaskWorktree(t *testing.T, h *hubforge.Hub) {
	slug := "batten-pair-dir"
	hubforge.AddPair(t, h, slug)
	hubforge.ReanchorPair(t, h, slug, "backend")

	c := wireForHub(t, h, slug, nil)

	got, err := c.env.InnerRun.PairDir()
	if err != nil {
		t.Fatalf("PairDir() error = %v; want nil", err)
	}
	taskLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve task location: %v", err)
	}
	if taskLocation.AnchorPath() == taskLocation.WorktreePath() {
		t.Fatalf("task location anchor %q equals its worktree root; want a subdirectory anchor", taskLocation.AnchorPath())
	}
	if got != taskLocation.WorktreePath() {
		t.Errorf("PairDir() = %q; want the task worktree root %q", got, taskLocation.WorktreePath())
	}
}

// stepMarkWatched_HoldsTheMarkerOnlyWhileItsNoticesReachTheDriversParent drives the seam over a real pair.
// The marker holds batten's pid, scratch directory and row name for a pair created from the prime.
// It is held while the prime's orch state records a strand, and removed once the strand is gone.
// A pair whose origin names a worktree that is absent here walks to the prime, so the marker stays.
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
		{name: "PairFromAnAbsentWorktreeWalksToThePrimeAndHoldsIt", strand: "orch-strand", parent: "another-worktree", wantHeld: true, wantPidIn: true},
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
			if want := shedrun.BattenWatchedContent(os.Getpid(), BattenDir(h.Location, slug), battenrecipe.NameRunShed); readErr != nil || !bytes.Equal(raw, want) {
				t.Errorf("%s: marker = %q, %v; want batten's pid, scratch directory and row %q", step.name, raw, readErr, want)
			}
		} else if !os.IsNotExist(readErr) {
			t.Errorf("%s: marker read = %q, %v; want it absent", step.name, raw, readErr)
		}
	}

	// The pid written is the one passed, the scheduling dobby's while it lives.
	setStrand("orch-strand")
	setOrigin(origin.ParentWorktree)
	const dobbyPID = 424242
	if held, err := markBattenWatched(h.Location, childLocation, BattenDir(h.Location, slug), battenrecipe.NameRunShed, dobbyPID); err != nil || !held {
		t.Fatalf("markBattenWatched() = %v, %v; want held", held, err)
	}
	raw, err := os.ReadFile(marker)
	if want := shedrun.BattenWatchedContent(dobbyPID, BattenDir(h.Location, slug), battenrecipe.NameRunShed); err != nil || !bytes.Equal(raw, want) {
		t.Errorf("marker after a pid was passed = %q, %v; want %q", raw, err, want)
	}
}

// stepStopReport_ReadsTheParkMarkersContentAndTime drives the stop-report seam over a real pair.
// It reports none while the driver has not parked, and the path and time of the park marker once it has.
func stepStopReport_ReadsTheParkMarkersContentAndTime(t *testing.T, h *hubforge.Hub) {
	slug := "batten-stop-report"
	hubforge.AddPair(t, h, slug)
	c := wireForHub(t, h, slug, nil)

	if path, _, found, err := c.env.InnerRun.StopReport(); err != nil || found {
		t.Fatalf("StopReport() = %q, found=%v, %v before the driver parked; want none", path, found, err)
	}

	childLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve child location: %v", err)
	}
	// The Go driver's start report is written before its tackler or arbiter runs;
	// it is no park marker, so it cannot end the eligible-stop hold.
	reportsDir := shedrun.DriveReportsDir(childLocation, shedrun.SelfRunID)
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reportsDir, "drive-1.md"), []byte("start report\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if path, _, found, err := c.env.InnerRun.StopReport(); err != nil || found {
		t.Fatalf("StopReport() = %q, found=%v, %v with only a drive report on disk; want none", path, found, err)
	}

	marker := shedrun.ParkMarker(childLocation, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("/wt/_lym/drive-reports/drive-1.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parkedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := os.Chtimes(marker, parkedAt, parkedAt); err != nil {
		t.Fatal(err)
	}

	path, at, found, err := c.env.InnerRun.StopReport()
	if err != nil || !found || path != "/wt/_lym/drive-reports/drive-1.md" || !at.Equal(parkedAt) {
		t.Errorf("StopReport() = %q, %s, found=%v, %v; want the marker's path and file time", path, at, found, err)
	}
}

// stepTaskWorktreeSeams_RunGitOnlyUntilTheFirstResolution drives the seams Run-Shed's wait reads over a real pair.
// After one of them has resolved the task worktree, the seams keep answering once git can no longer resolve it,
// so no check spawns git.
// A pair gone from disk is still refused by name.
func stepTaskWorktreeSeams_RunGitOnlyUntilTheFirstResolution(t *testing.T, h *hubforge.Hub) {
	slug := "batten-locate-once"
	hubforge.AddPair(t, h, slug)
	c := wireForHub(t, h, slug, nil)

	wantDir, err := c.env.InnerRun.PairDir()
	if err != nil {
		t.Fatalf("PairDir() error = %v; want nil", err)
	}

	worktreePath := fabricengine.WorktreePath(h.Location, slug)
	gitFile := filepath.Join(worktreePath, ".git")
	if err := os.Rename(gitFile, gitFile+".hidden"); err != nil {
		t.Fatal(err)
	}
	if _, err := taskWorktreeLocation(h.Location, slug); err == nil {
		t.Fatal("taskWorktreeLocation() with the pair's .git hidden = nil error; want git's resolution to fail")
	}
	if got, err := c.env.InnerRun.PairDir(); err != nil || got != wantDir {
		t.Errorf("PairDir() = %q, %v with git unable to resolve the pair; want %q from the first resolution", got, err, wantDir)
	}
	if _, found, err := c.env.InnerRun.ReadDecision(); err != nil || found {
		t.Errorf("ReadDecision() found=%v, %v with git unable to resolve the pair; want none, nil", found, err)
	}
	if _, _, found, err := c.env.InnerRun.StopReport(); err != nil || found {
		t.Errorf("StopReport() found=%v, %v with git unable to resolve the pair; want none, nil", found, err)
	}
	if err := os.Rename(gitFile+".hidden", gitFile); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(worktreePath, worktreePath+".moved"); err != nil {
		t.Fatal(err)
	}
	_, absentErr := c.env.InnerRun.PairDir()
	if err := os.Rename(worktreePath+".moved", worktreePath); err != nil {
		t.Fatal(err)
	}
	if absentErr == nil || !strings.Contains(absentErr.Error(), "is not present at") {
		t.Errorf("PairDir() error = %v with the pair gone from disk; want the absent-worktree refusal", absentErr)
	}
}

// stepStatusFlagsAHaltedBattenOverADoneChild pins the status surface over a real pair.
// A failed batten status beside a done child status carries child_done_batten_halted.
// The flag names the run that closes the pair, and the key is omitted once the child status file is gone.
func stepStatusFlagsAHaltedBattenOverADoneChild(t *testing.T, h *hubforge.Hub) {
	slug := "batten-status-child-done"
	hubforge.AddPair(t, h, slug)
	c := wireForHub(t, h, slug, nil)
	c.location, c.slug = h.Location, slug
	seedEntryStatus(t, c, battenrecipe.NameRunShed, shedengine.StateFailed, nil)

	task, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve task location: %v", err)
	}
	childStatus, childLock := shedrun.StatusFile(task, shedrun.SelfRunID), shedrun.StatusLock(task, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(childLock), 0o755); err != nil {
		t.Fatalf("mkdir child status lock dir: %v", err)
	}
	if err := state.WriteJSON(childStatus, childLock, shedengine.Status{State: shedengine.StateDone}); err != nil {
		t.Fatalf("write child status: %v", err)
	}

	statusEnvelope := func() map[string]any {
		t.Helper()
		var out bytes.Buffer
		if exitCode := clihelp.Execute(battenVerbCommand(c, "status"), &out, []string{slug}); exitCode != 0 {
			t.Fatalf("status() exit code = %d; want 0; output: %s", exitCode, out.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatalf("decode envelope: %v; output: %s", err, out.String())
		}
		return envelope
	}

	flag, _ := statusEnvelope()[childDoneFlagKey].(string)
	if !strings.Contains(flag, `"lym batten run `+slug+`"`) {
		t.Errorf("status envelope %s = %q; want it to name \"lym batten run %s\"", childDoneFlagKey, flag, slug)
	}

	if err := os.Remove(childStatus); err != nil {
		t.Fatalf("remove child status: %v", err)
	}
	if _, present := statusEnvelope()[childDoneFlagKey]; present {
		t.Errorf("status envelope carries %s with the child status file gone; want it omitted", childDoneFlagKey)
	}
}

// stepSeedChild_WritesTheRunSettingsIntoThePairsLocalLayer drives Worktree-Create and Seed-Child.
// The task carries run settings: the pair's local loom file holds the key, the loom loader resolves it,
// and the per-worktree config files show no change since the fork point.
// A second task whose board entry carries a key the local layer closes stands Seed-Child Stuck.
// The entry is written past `lym board upsert`, and the stuck reason names the module, the key and the way forward.
// No child seed and no local file is written.
func stepSeedChild_WritesTheRunSettingsIntoThePairsLocalLayer(t *testing.T, h *hubforge.Hub) {
	runWorktreeCreateAndSeedChild := func(t *testing.T, slug string, settings map[string]any) shedengine.StepResult {
		t.Helper()
		seedBoardTask(t, h, slug, "tapestry")
		cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
		if err != nil {
			t.Fatalf("load board config: %v", err)
		}
		cfg.Path = h.BoardDir()
		if _, err := boardengine.New(cfg).UpsertTask(map[string]any{"slug": slug, "run_settings": settings}); err != nil {
			t.Fatalf("set run settings of %q: %v", slug, err)
		}

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
		result, err := shed.Step(ctx)
		if err != nil {
			t.Fatalf("Step (Seed-Child): %v", err)
		}
		return result
	}

	t.Run("a valid setting lands in the pair's local layer", func(t *testing.T) {
		const slug = "batten-run-settings"
		result := runWorktreeCreateAndSeedChild(t, slug, map[string]any{"loom": map[string]any{"review_max_bounces": 2}})
		if result.Outcome != shedengine.Done {
			t.Fatalf("Seed-Child outcome = %q (reason %q); want done", result.Outcome, result.Reason)
		}
		pair, err := taskWorktreeLocation(h.Location, slug)
		if err != nil {
			t.Fatalf("resolve child location: %v", err)
		}
		local, err := os.ReadFile(configengine.LocalFile(pair.AnchorPath(), "loom"))
		if err != nil || !strings.Contains(string(local), "review_max_bounces: 2") {
			t.Fatalf("pair loom.local.yaml = %q, %v; want review_max_bounces: 2", local, err)
		}
		cfg, err := loomengine.LoadConfig(fabricengine.ConfigDirs(pair), "loom")
		if err != nil {
			t.Fatalf("load the pair's loom config: %v", err)
		}
		if cfg.ReviewMaxBounces != 2 {
			t.Errorf("resolved review_max_bounces = %d; want 2", cfg.ReviewMaxBounces)
		}

		pairFabric, err := fabricengine.Open(pair)
		if err != nil {
			t.Fatalf("open the pair: %v", err)
		}
		taskBranch, err := pairFabric.CurrentBranch()
		if err != nil {
			t.Fatalf("task branch: %v", err)
		}
		origin, found, err := fabricengine.ReadOrigin(pair)
		if err != nil || !found {
			t.Fatalf("read origin: found %v, %v", found, err)
		}
		var rels []string
		for _, module := range configreg.Modules() {
			if !module.HubWide {
				rels = append(rels, configengine.ConfigFileRel(module.Name))
			}
		}
		changes, err := fabricengine.ReadConfigChanges(pair, taskBranch, origin.ParentBranch, rels)
		if err != nil {
			t.Fatalf("read config changes: %v", err)
		}
		if len(changes.Files) != 0 {
			t.Errorf("per-worktree config changes = %v; want none, the local layer is untracked", changes.Files)
		}
	})

	t.Run("a key the local layer closes stands Seed-Child Stuck", func(t *testing.T) {
		const slug = "batten-run-settings-closed"
		result := runWorktreeCreateAndSeedChild(t, slug, map[string]any{"darn": map[string]any{"verify": "x"}})
		if result.Outcome != shedengine.Stuck {
			t.Fatalf("Seed-Child outcome = %q; want stuck", result.Outcome)
		}
		for _, want := range []string{"darn", "verify", "lym board upsert", "lym batten run " + slug} {
			if !strings.Contains(result.Reason, want) {
				t.Errorf("stuck reason %q lacks %q", result.Reason, want)
			}
		}
		pair, err := taskWorktreeLocation(h.Location, slug)
		if err != nil {
			t.Fatalf("resolve child location: %v", err)
		}
		if pathExists(configengine.LocalFile(pair.AnchorPath(), "darn")) {
			t.Error("a darn local file was written despite the refusal")
		}
		if _, found, err := shedrun.ReadSeed(pair, shedrun.SelfRunID); err != nil || found {
			t.Errorf("child seed found = %v, err %v; want none written", found, err)
		}
	})
}

// posixOnly returns run off Windows and nil on it.
// On Windows the step loop generates no subtest for a step whose commands are written for sh.
func posixOnly(run func(*testing.T, *hubforge.Hub)) func(*testing.T, *hubforge.Hub) {
	if runtime.GOOS == "windows" {
		return nil
	}
	return run
}
