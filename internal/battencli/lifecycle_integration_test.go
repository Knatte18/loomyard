//go:build tier1

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
)

// shortPollBattenRecipe is contracts/recipes/batten-recipe.yaml, byte-for-byte the same rows
// and the same on_stuck self-route with no max_bounces, with poll_interval_s dropped from 2 to 1.
// It is the recipe copy the tier1 steps build their shed from.
// Its poll_interval_s reaches only the floor check.
// This tier exercises the assembled wiring: the recipe's own config value flows through shedbuild into innerRunEntry.
// The InnerRun producer in isolation is covered by battenshed's own untagged tests.
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

// wireForHub builds a *battenCLI wired for real against h's prime Location and slug -- a real
// CreateWorktree and a real Teardown, both driving fabricengine's topology holder against h's own
// hub -- then overrides Env.InnerRun.Spawn and Env.InnerRun.ReadStatus with readStatus, per this
// file's own header.
//
// The Run-Shed check stats the child's status file, so a stubbed read moves that file into a temporary directory and also creates it when it is absent.
// A pass ends by itself, with a Yield while a stubbed child stays as it is, so no sleep or pause seam is replaced.
func wireForHub(t *testing.T, h *hubforge.Hub, slug string, readStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)) *battenCLI {
	t.Helper()
	// The board default layer holds every Command row's command empty, so a stepped row passes on and no test runs the test binary through {lym}.
	writeBoardBattenCommands(t, h, nil)
	c := &battenCLI{location: h.Location}
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
	return c
}

// commandRowKeys are the command keys of batten.yaml's three Command rows, in row order.
var commandRowKeys = []string{"open_window_command", "seed_ide_command", "open_ide_command"}

// writeBoardBattenCommands writes the hub's board default batten.yaml: the template with each Command row's command key set to commands[key], empty for a key commands lacks.
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
// With a fresh stamp the first call is a no-op, so the new pair losing the prime's retired key can only come from the unconditional pair call after the create row.
// Against an absent stamp, `arm` for run reconciles ahead of armAt's non-prime refusal and its auto-seed.
func TestBattenIntegration_RunReconcilesTheHubBeforeAndThePairAfterCreating(t *testing.T) {
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
	if err := c.env.CreateWorktree(context.Background()); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	pair := h.PairCodeWorktree(slug)
	if err := c.env.CreateWorktree(context.Background()); err != nil {
		t.Fatalf("CreateWorktree re-entered on the complete pair: %v", err)
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
//
//testtiming:keep the start-cost step pins the decision record's D2 acceptance criterion, that a Command row costs under 100 ms beyond its status commit and its process, a wall-clock bound no other test asserts
func TestBattenIntegration_Rows(t *testing.T) {
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
		{"CommandRows_RunTheBoardDefaultCommandsWithEverySwitchOn", stepCommandRows_RunTheBoardDefaultCommandsWithEverySwitchOn},
		{"CommandRows_IgnoreACommandSetInTheTasksTrackedLayer", stepCommandRows_IgnoreACommandSetInTheTasksTrackedLayer},
		{"CommandRows_FollowTheTasksLocalSwitch", stepCommandRows_FollowTheTasksLocalSwitch},
		{"CommandRows_ResumeAtARowAnOlderBinaryLeftStanding", stepCommandRows_ResumeAtARowAnOlderBinaryLeftStanding},
		{"RunRowCommand_ReportsATimeoutAndAnExitCode", stepRunRowCommand_ReportsATimeoutAndAnExitCode},
		{"StartCost_CommandRowsCostUnderTheBoundBeyondTheCommitAndTheProcess", stepStartCost_CommandRowsCostUnderTheBoundBeyondTheCommitAndTheProcess},
		// Last, since it leaves the prime dirty for every step after it.
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
	// first "lym loom start" in that worktree. It must be a no-op, never a refusal.
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

// stepFourRowRun_SeedsChildCommitsAndTearsDown drives a spawn stub plus a read-status answering StateDone through the whole row list one row at a time: Worktree-Create, Seed-Child, the three Command rows, Run-Shed, Land, Worktree-Teardown.
// It asserts the pair exists on disk after the create row, that the child's own _lym/shed/<slug>/seed.json exists after the seed row, names the Board task's own "type" as its recipe, carries the pair's recorded parent branch as params.parent, and is committed (not merely written) on the child's own records pair, and that the pair is gone after the teardown row.
// It runs once per recipe a child may run.
func stepFourRowRun_SeedsChildCommitsAndTearsDown(t *testing.T, h *hubforge.Hub) {
	for _, recipe := range []string{shedrun.RecipeLoom, shedrun.RecipeDarn} {
		t.Run(recipe, func(t *testing.T) {
			fourRowRunForRecipe(t, h, recipe)
		})
	}
}

// fourRowRunForRecipe is the whole-row-list run over a Board task whose recipe is recipe.
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

	// open_ide is off by default, so its entry gate skips Open-IDE and the same step runs Run-Shed.
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
	// Only the teardown request's landed flag can turn it done, since the stubbed child status holds no Finalize Done and batten's own run is still running.
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

	// Teardown's own top.Remove call passes remote: true -- a batten-driven teardown is the task's
	// final removal, and nothing will ever re-adopt its records branch, so the remote copy must not
	// linger where "lym fabric cleanup" (its own enumeration is local-branches-only) can never reach
	// it. See F-CLEANUP-REMOTE-ORPHAN.
	recordsBranch := fabricengine.RecordsBranchName(slug)
	if err := exec.Command("git", "-C", h.RecordsBare, "rev-parse", "--verify", "refs/heads/"+recordsBranch).Run(); err == nil {
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
			recordsBranch := fabricengine.RecordsBranchName(slug)
			if !remoteBranchExists(h.RecordsBare, recordsBranch) {
				t.Fatalf("precondition: %q not on the remote after the create", recordsBranch)
			}
			// Both worktrees removed, the remote copy kept -- the state a kill between the local
			// and remote deletions leaves.
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
	recordsBranch := fabricengine.RecordsBranchName(slug)
	restore := refuseRemoteBranchDeletions(t, h.RecordsBare)

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

// stepTeardown_UnreachableRemoteHaltsBeforeRemovalResumably breaks the records origin before teardown and asserts the removal half halts on the failed archive with the pair still in place and the resume named, then that a resume once the remote is reachable again archives the records and finishes the teardown.
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
	if !pathExists(h.PairCodeWorktree(slug)) {
		t.Errorf("task worktree removed; want the pair left in place when the archive fails")
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

// stepStepDrivenRunShed_ReturnsOnTheChildsStateChange proves Run-Shed's one check per pass through a step: a running child yields from every "lym batten step" without blocking or persisting anything, and the step after the child's state changes ends Stuck, naming the change.
//
// The steps run on the real clock with no sleep: a pass returns at once, and the change, not a poll interval, is what ends one with a verdict.
func stepStepDrivenRunShed_ReturnsOnTheChildsStateChange(t *testing.T, h *hubforge.Hub) {
	slug := "batten-step-reentrant"
	// The pair must already exist on disk: InnerRun's ResolveStatus resolves the child worktree's own *lymcwd.Location, which requires a real git worktree there, standing in for a Worktree-Create that already completed on an earlier step -- exactly as stepMidListResume_SkipsTheCompletedCreateRow's own fixture does.
	hubforge.AddPair(t, h, slug)

	childState := shedengine.StateRunning
	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: childState, CurrentProducer: "Plan-Review"}, true, nil
	})
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
	if !pathExists(h.PairCodeWorktree(slug)) {
		t.Errorf("task worktree missing after the idempotent create row: %s", h.PairCodeWorktree(slug))
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

// stepCreateRow_LeftoverBranchIsRewordedForPrime pins createRefusal's own wiring into the CreateWorktree closure: a leftover code branch from an earlier torn-down pair must reach the closure's caller worded for an operator standing in prime, never fabric's own raw "lym fabric checkout" advice, which would switch prime itself onto the task's branch.
// Calls c.env.CreateWorktree directly, the production closure, rather than createRefusal in isolation, so a future edit that drops the reword call fails here.
func stepCreateRow_LeftoverBranchIsRewordedForPrime(t *testing.T, h *hubforge.Hub) {
	slug := "batten-leftover-branch"
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", slug)
	seedBoardTask(t, h, slug, "loom")

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the create row must fail before the poll row ever runs")
		return shedengine.Status{}, false, nil
	})
	c.owner = hubOwnerSeams(h.Location)

	err := c.env.CreateWorktree(context.Background())
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

// stepCreateRow_IncompletePairRefusesRatherThanSkippingAdd pins taskWorktreeComplete's own fix: a code worktree a SIGKILL-interrupted Add left behind, with no sibling and no junctions wired, must not be mistaken for a finished create.
// Reproduces the state a process killed right after Add's own first step leaves -- the code worktree and branch exist, nothing else does -- by driving the same git command Add's own createGitWorktree issues, rather than stubbing anything: this proves taskWorktreeComplete's real filesystem check, not a fake of it.
func stepCreateRow_IncompletePairRefusesRatherThanSkippingAdd(t *testing.T, h *hubforge.Hub) {
	slug := "batten-incomplete-pair"
	target := h.PairCodeWorktree(slug)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "add", "-b", slug, target)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the create row must refuse before the poll row ever runs")
		return shedengine.Status{}, false, nil
	})

	err := c.env.CreateWorktree(context.Background())
	if err == nil {
		t.Fatal("CreateWorktree() error = nil; want a refusal naming the incomplete pair, not silent success")
	}
	for _, want := range []string{slug, "not fully created", "lym fabric remove --force " + slug, "git branch -D " + slug} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CreateWorktree() error = %q; want it to contain %q", err.Error(), want)
		}
	}
	// The refusal must be read-only: no repair attempt, no partial Add left further along than it
	// started.
	if pathExists(h.PairRecordsSibling(slug)) {
		t.Errorf("the pair's other-side worktree exists after the refusal; want CreateWorktree to have made no repair attempt")
	}
}

// TestBattenIntegration_CreateRow_IncompletePairRemedyWorksVerbatimOnAPrefixedHub follows the
// incomplete-pair remedy exactly as worded, from prime, for a SIGKILL landing after Add created the
// pair's other side: the remedy must name the prefixed branch Add created, and doing
// what it says must leave nothing that makes the resumed create refuse.
func TestBattenIntegration_CreateRow_IncompletePairRemedyWorksVerbatimOnAPrefixedHub(t *testing.T) {
	// Not a step of TestBattenIntegration_Rows: seeding a branch prefix into the fabric config changes the hub for every pair created after it.
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.SeedFabricConfig(t, h, "branch_prefix: r4/\npathspec: \"\"\n")
	slug := "batten-incomplete-prefixed"
	branch := "r4/" + slug
	target := h.PairCodeWorktree(slug)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "add", "-b", branch, target)

	// Stands in for Add's own later step -- the other side's worktree -- run
	// directly rather than through Add so the junctions this test needs missing stay missing.
	recordsRepoRoot, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("resolve records repo root: %v", err)
	}
	gitkit.MustRun(t, recordsRepoRoot, "git", "worktree", "add", "-b", fabricengine.RecordsBranchName(branch), h.PairRecordsSibling(slug))

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called by the create row")
		return shedengine.Status{}, false, nil
	})

	err = c.env.CreateWorktree(context.Background())
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

	if err := c.env.CreateWorktree(context.Background()); err != nil {
		t.Fatalf("CreateWorktree() after the remedy = %v; want the fresh create to succeed", err)
	}
	if !pathExists(h.PairRecordsSibling(slug)) {
		t.Errorf("the pair's other side is missing after the resumed create: %s", h.PairRecordsSibling(slug))
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
	if pathExists(h.PairCodeWorktree(slug)) {
		t.Errorf("pair exists even though the create row blocked before creating anything: %s", h.PairCodeWorktree(slug))
	}
}

// newCommandRowFixture prepares a pair for slug and a wired battenCLI whose board default batten.yaml holds, for each of the three Command rows, a command writing the row's marker file with the slug.
// It returns the CLI, the marker path of each command key and the pair's code worktree.
// The board default is set after the wiring, since the executable lookup reads it when the first row runs, and is emptied again when the step ends.
func newCommandRowFixture(t *testing.T, h *hubforge.Hub, slug string) (*battenCLI, map[string]string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the commands below are written for sh")
	}
	seedBoardTask(t, h, slug, "loom")
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

// stepCommandRows_RunTheBoardDefaultCommandsWithEverySwitchOn steps Open-Window, Seed-IDE and Open-IDE with their switches on, and asserts each wrote its own marker.
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

// stepCommandRows_IgnoreACommandSetInTheTasksTrackedLayer asserts a command in the task worktree's tracked batten.yaml is not run, the board default's is, and the ignored value is warned about.
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

// stepGatedOffCommandRow seeds a running status at row with pause_before naming next, the row's on_done row, and steps once.
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

// stepCommandRows_FollowTheTasksLocalSwitch asserts Open-IDE runs its command with open_ide on in the task worktree's local layer and is skipped by its entry gate with it off, and that the Open-Window row is skipped by its entry gate with open_window off.
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

// stepCommandRows_ResumeAtARowAnOlderBinaryLeftStanding asserts a run whose status stands at Open-Window, or at Seed-IDE, with its commit marker naming that row resumes at that row by name and writes that row's marker first.
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

// startCostBound is the decision record's D2 acceptance criterion: a Command row costs less than this beyond the status commit and the command's own process.
const startCostBound = 100 * time.Millisecond

// stepStartCost_CommandRowsCostUnderTheBoundBeyondTheCommitAndTheProcess steps Seed-IDE, whose command is empty so it spawns nothing, and Open-Window whose command only starts a background process, and asserts each costs under startCostBound beyond the status commit and the process.
// A history entry's timestamp has one-second resolution, so a row's cost is the wall time of its one step.
// The status commit's time is measured by wrapping the wired CommitStatus seam in a timer, and the process's time by running the same command once through runRowCommand, whose marker the row's own run then overwrites.
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

// stepRunRowCommand_ReportsATimeoutAndAnExitCode asserts runRowCommand ends a command that outlives its timeout well before the command would, and reports a non-zero exit code.
func stepRunRowCommand_ReportsATimeoutAndAnExitCode(t *testing.T, h *hubforge.Hub) {
	if runtime.GOOS == "windows" {
		t.Skip("the commands below are written for sh")
	}
	dir := t.TempDir()

	start := time.Now()
	result, err := runRowCommand(context.Background(), dir, "sleep 5", filepath.Join(dir, "sleep.log"), 200*time.Millisecond)
	if err != nil || !result.TimedOut {
		t.Fatalf("runRowCommand(sleep 5, 200ms) = %+v, %v; want TimedOut and nil", result, err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("runRowCommand(sleep 5, 200ms) took %v; want it ended well before the sleep", elapsed)
	}

	result, err = runRowCommand(context.Background(), dir, "exit 3", filepath.Join(dir, "exit.log"), time.Minute)
	if err != nil || result.TimedOut || result.ExitCode != 3 {
		t.Errorf("runRowCommand(exit 3) = %+v, %v; want exit code 3 and nil", result, err)
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
	if pathExists(h.PairCodeWorktree(slug)) {
		t.Errorf("pair still exists after the resumed run's teardown row completed: %s", h.PairCodeWorktree(slug))
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

// stepRecordsPrimeRefusal pins the other half of the Bookend guard: the records sibling of the prime is a repository of its own whose prime is itself, so a name comparison alone admits it, and both bookend rows would then drive fabric's topology against the records repository.
// Every verb must refuse there before arming anything -- no seed written under the records prime, nothing created -- and the refusal must say which checkout the operator is standing in.
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
	if pathExists(h.PairCodeWorktree(slug)) {
		t.Error("task worktree still present after the teardown; want it removed")
	}
}

// stepTeardown_SiblingDirtOutsideRecordPathsRefusesShutdown dirties a created pair's sibling worktree outside the record pathspec and asserts the shutdown half refuses with fabric's sibling-dirty error, never names --force, and leaves the pair in place.
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

// stepPairDirNamesTheTaskWorktree pins the InnerRun.PairDir seam to the task worktree's own anchor, resolved on Call over a real pair.
func stepPairDirNamesTheTaskWorktree(t *testing.T, h *hubforge.Hub) {
	slug := "batten-pair-dir"
	hubforge.AddPair(t, h, slug)

	c := wireForHub(t, h, slug, nil)

	got, err := c.env.InnerRun.PairDir()
	if err != nil {
		t.Fatalf("PairDir() error = %v; want nil", err)
	}
	taskLocation, err := taskWorktreeLocation(h.Location, slug)
	if err != nil {
		t.Fatalf("resolve task location: %v", err)
	}
	if got != taskLocation.AnchorPath() {
		t.Errorf("PairDir() = %q; want %q", got, taskLocation.AnchorPath())
	}
}

// stepMarkWatched_HoldsTheMarkerOnlyWhileItsNoticesReachTheDriversParent drives the watched-marker seam over a real pair:
// the marker holds batten's pid, scratch directory and row name for a pair created from the prime while the prime's orch state records a strand,
// and is removed when the strand is gone; a pair whose origin names a worktree that is absent here walks to the prime, so the marker stays.
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

// stepStopReport_ReadsTheParkMarkersContentAndTime drives the stop-report seam over a real pair: it reports none while the driver has not parked, and the path and time of the park marker once it has.
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
	// The Go driver's start report is written before its tackler or arbiter runs; it is no park marker, so it cannot end the eligible-stop hold.
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

// stepTaskWorktreeSeams_RunGitOnlyUntilTheFirstResolution drives the seams Run-Shed's wait reads on every poll check over a real pair.
// Once one has resolved the task worktree, they keep answering after git can no longer resolve it, so no check spawns git, while a pair gone from disk is still refused by name.
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

// stepStatusFlagsAHaltedBattenOverADoneChild pins the status surface over a real pair: a failed batten status beside a done child status carries child_done_batten_halted naming the run that closes the pair, and the key is omitted once the child status file is gone.
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

// stepSeedChild_WritesTheRunSettingsIntoThePairsLocalLayer drives Worktree-Create and Seed-Child over a task carrying run settings:
// the pair's local loom file holds the key, the loom loader resolves it, and the per-worktree config files show no change since the fork point.
// A second task whose board entry carries a key the local layer closes, written past `lym board upsert`, stands Seed-Child Stuck naming the module, the key and the way forward, with no child seed and no local file written.
func stepSeedChild_WritesTheRunSettingsIntoThePairsLocalLayer(t *testing.T, h *hubforge.Hub) {
	runWorktreeCreateAndSeedChild := func(t *testing.T, slug string, settings map[string]any) shedengine.StepResult {
		t.Helper()
		seedBoardTask(t, h, slug, "loom")
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
