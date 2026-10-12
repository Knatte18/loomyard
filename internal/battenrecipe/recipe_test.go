// recipe_test.go builds through New and asserts the shape the batten recipe's rows must
// carry: their names, the OnDone chain, the load-bearing empty/self-route OnStuck edges and empty
// terminal OnDone edge, Run-Shed's in-call wait configuration, the absence of any Segment, and that the
// returned *shedengine.Shed carries ShedPaths' five values verbatim. It also asserts RecipeEngines()'s
// own shape, since a silently empty return would disable the cross-consumer coverage guard rather
// than fail it.

package battenrecipe

import (
	"context"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestNew_RowNamesMatchTheDurableIdentityConstants asserts New assembles exactly the rows the Name* constants name, in order.
// It is the durable-identity guard internal/loomrecipe's own row-name test mirrors.
func TestNew_RowNamesMatchTheDurableIdentityConstants(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	wantNames := []string{NameWorktreeCreate, NameSeedChild, NameRunShed, NameLand, NameWorktreeTeardown}
	if len(shed.Producers) != len(wantNames) {
		t.Fatalf("New() row count = %d, want %d", len(shed.Producers), len(wantNames))
	}
	for i, want := range wantNames {
		if got := shed.Producers[i].Name; got != want {
			t.Errorf("row %d name = %q; want %q", i, got, want)
		}
	}
}

// TestNew_OnDoneChainAndLoadBearingEdges asserts the Worktree-Create -> Seed-Child -> Run-Shed -> Land -> Worktree-Teardown OnDone chain.
// It also asserts Run-Shed's OnStuck self-route, Land's empty OnStuck, Worktree-Teardown's empty OnDone, and that no row declares a Segment.
func TestNew_OnDoneChainAndLoadBearingEdges(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	byName := make(map[string]int, len(shed.Producers))
	for i, p := range shed.Producers {
		byName[p.Name] = i
	}

	create := shed.Producers[byName[NameWorktreeCreate]]
	if create.OnDone != NameSeedChild {
		t.Errorf("Worktree-Create OnDone = %q; want %q", create.OnDone, NameSeedChild)
	}

	seedChild := shed.Producers[byName[NameSeedChild]]
	if seedChild.OnDone != NameRunShed {
		t.Errorf("Seed-Child OnDone = %q; want %q", seedChild.OnDone, NameRunShed)
	}

	runShed := shed.Producers[byName[NameRunShed]]
	if runShed.OnDone != NameLand {
		t.Errorf("Run-Shed OnDone = %q; want %q", runShed.OnDone, NameLand)
	}
	if runShed.OnStuck != NameRunShed {
		t.Errorf("Run-Shed OnStuck = %q; want %q -- a static self-route, not the empty escalate-to-human edge", runShed.OnStuck, NameRunShed)
	}

	land := shed.Producers[byName[NameLand]]
	if land.OnDone != NameWorktreeTeardown {
		t.Errorf("Land OnDone = %q; want %q", land.OnDone, NameWorktreeTeardown)
	}
	if land.OnStuck != "" {
		t.Errorf("Land OnStuck = %q; want empty -- a failed step halts batten blocked at Land", land.OnStuck)
	}

	teardown := shed.Producers[byName[NameWorktreeTeardown]]
	if teardown.OnDone != "" {
		t.Errorf("Worktree-Teardown OnDone = %q; want empty -- this is what ends the run quietly", teardown.OnDone)
	}

	for _, p := range shed.Producers {
		if p.Segment != "" {
			t.Errorf("row %q Segment = %q; want empty", p.Name, p.Segment)
		}
	}
}

// TestNew_RunShedWaitsInCallWithoutACountedBudget pins Run-Shed's wait in the recipe: a short check interval and a notice probe in its config, and no max_bounces.
// Every Stuck the row returns is budget-exempt, so a counted budget would only be a limit nothing spends; a recipe that reintroduced one would read as a time limit on a running child that the producer no longer has.
func TestNew_RunShedWaitsInCallWithoutACountedBudget(t *testing.T) {
	recipe, err := shedbuild.Parse(recipes.BattenRecipe)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	var row shedbuild.Row
	for _, r := range recipe.Producers {
		if r.Name == NameRunShed {
			row = r
		}
	}

	if row.MaxBounces != 0 {
		t.Errorf("Run-Shed max_bounces = %d; want it absent", row.MaxBounces)
	}
	for key, want := range map[string]int{"poll_interval_s": 2, "notice_probe_s": 30} {
		if got := row.Config[key]; got != want {
			t.Errorf("Run-Shed config %s = %v; want %d", key, got, want)
		}
	}
}

// TestNew_CarriesShedPathsVerbatim asserts the returned *shedengine.Shed carries ShedPaths' five
// values verbatim.
func TestNew_CarriesShedPathsVerbatim(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	if shed.StatusPath != paths.StatusPath {
		t.Errorf("StatusPath = %q; want %q", shed.StatusPath, paths.StatusPath)
	}
	if shed.LockPath != paths.LockPath {
		t.Errorf("LockPath = %q; want %q", shed.LockPath, paths.LockPath)
	}
	if shed.StatusLockPath != paths.StatusLockPath {
		t.Errorf("StatusLockPath = %q; want %q", shed.StatusLockPath, paths.StatusLockPath)
	}
	if shed.MaxBounces != paths.MaxBounces {
		t.Errorf("MaxBounces = %d; want %d", shed.MaxBounces, paths.MaxBounces)
	}
}

// TestNameRunShed_ValueIsPinnedAgainstASymmetryRename exists because shedengine persists
// CurrentProducer -- the row name, not the engine name -- into the status file, so renaming a row
// breaks resume for an in-flight run. This guard makes a later symmetry-minded rename of the
// durable identity (matching the engine's InnerRun rename onto the row) fail loudly here rather
// than silently breaking resume.
func TestNameRunShed_ValueIsPinnedAgainstASymmetryRename(t *testing.T) {
	if NameRunShed != "Run-Shed" {
		t.Errorf("NameRunShed = %q; want the durable value %q", NameRunShed, "Run-Shed")
	}

	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	wantNames := []string{NameWorktreeCreate, NameSeedChild, NameRunShed, NameLand, NameWorktreeTeardown}
	if len(shed.Producers) != len(wantNames) {
		t.Fatalf("New() row count = %d, want %d", len(shed.Producers), len(wantNames))
	}
	for i, want := range wantNames {
		if got := shed.Producers[i].Name; got != want {
			t.Errorf("row %d name = %q; want %q", i, got, want)
		}
	}
}

// TestRecipeEngines_ReportsEveryEngineNameSortedOnce asserts RecipeEngines() returns the sorted, de-duplicated engine names of battenRowEngines.
// It is the input the cross-consumer guard trusts, so a silently empty return would disable that guard rather than fail it.
func TestRecipeEngines_ReportsEveryEngineNameSortedOnce(t *testing.T) {
	got := RecipeEngines()
	var want []string
	for _, engine := range battenRowEngines {
		want = append(want, engine)
	}
	sort.Strings(want)
	want = slices.Compact(want)

	if len(got) != len(want) {
		t.Fatalf("RecipeEngines() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("RecipeEngines()[%d] = %q; want %q", i, got[i], w)
		}
	}
}

// TestNew_PauseDuringLandReentersTheRowThroughGotoThenRun asserts a pause requested during Land's land-lock wait halts batten blocked at Land with the pause flag still set.
// It asserts a goto onto Land leaves the run paused with the flag cleared, and the next step calls Land again, which returns Done once the lock frees.
func TestNew_PauseDuringLandReentersTheRowThroughGotoThenRun(t *testing.T) {
	env, paths := testEnv(t)
	lockPolls := 0
	env.Land.LandLock.Acquire = func() (func() error, bool, error) {
		lockPolls++
		if lockPolls > 1 {
			return func() error { return nil }, true, nil
		}
		err := state.UpdateJSON(paths.StatusPath, paths.StatusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
			cur.PauseRequested = true
			return cur, nil
		})
		return nil, false, err
	}
	env.Land.PauseRequested = func() (bool, error) {
		status, _, err := state.ReadJSONNoMkdir[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
		return status.PauseRequested, err
	}
	env.Land.Sleep = func(context.Context, time.Duration) {}
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	seed := shedengine.Status{CurrentProducer: NameLand, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{}}
	if err := state.WriteJSON(paths.StatusPath, paths.StatusLockPath, seed); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	step, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("first Step() error = %v, want nil", err)
	}
	if step.Producer != NameLand || step.Outcome != shedengine.Stuck || step.State != shedengine.StateBlocked || step.Next != NameLand {
		t.Fatalf("first Step() = %+v; want Land Stuck, halting blocked at Land", step)
	}
	blocked, _, _ := state.ReadJSONNoMkdir[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if !blocked.PauseRequested {
		t.Error("pause_requested after the blocked halt = false; want it still set")
	}

	if _, err := shedengine.Goto(shedengine.GotoRequest{
		StatusPath:     paths.StatusPath,
		LockPath:       paths.LockPath,
		StatusLockPath: paths.StatusLockPath,
		Producers:      shed.Producers,
		Target:         NameLand,
	}); err != nil {
		t.Fatalf("Goto() error = %v, want nil", err)
	}
	paused, _, _ := state.ReadJSONNoMkdir[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if paused.State != shedengine.StatePaused || paused.CurrentProducer != NameLand || paused.PauseRequested {
		t.Fatalf("status after Goto = %+v; want paused at Land with the flag cleared", paused)
	}

	step, err = shed.Step(context.Background())
	if err != nil {
		t.Fatalf("second Step() error = %v, want nil", err)
	}
	if step.Producer != NameLand || step.Outcome != shedengine.Done {
		t.Errorf("second Step() = %+v; want Land Done", step)
	}
}
