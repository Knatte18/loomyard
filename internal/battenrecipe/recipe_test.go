// recipe_test.go builds through New and asserts the shape the batten recipe's four rows must
// carry: their names, the OnDone chain, the load-bearing empty/self-route OnStuck edges and empty
// terminal OnDone edge, Run-Shed's in-call wait configuration, the absence of any Segment, and that the
// returned *shedengine.Shed carries ShedPaths' five values verbatim. It also asserts RecipeEngines()'s
// own shape, since a silently empty return would disable the cross-consumer coverage guard rather
// than fail it.

package battenrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// TestNew_RowNamesMatchTheDurableIdentityConstants asserts New assembles exactly four rows, named
// exactly the four Name* constants -- the durable-identity guard internal/loomrecipe's own
// row-name test mirrors.
func TestNew_RowNamesMatchTheDurableIdentityConstants(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	if len(shed.Producers) != 4 {
		t.Fatalf("New() row count = %d, want 4", len(shed.Producers))
	}

	wantNames := []string{NameWorktreeCreate, NameSeedChild, NameRunShed, NameWorktreeTeardown}
	for i, want := range wantNames {
		if got := shed.Producers[i].Name; got != want {
			t.Errorf("row %d name = %q; want %q", i, got, want)
		}
	}
}

// TestNew_OnDoneChainAndLoadBearingEdges asserts the Worktree-Create -> Seed-Child -> Run-Shed ->
// Worktree-Teardown OnDone chain, Run-Shed's OnStuck self-route, Worktree-Teardown's empty OnDone,
// and that no row declares a Segment.
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
	if runShed.OnDone != NameWorktreeTeardown {
		t.Errorf("Run-Shed OnDone = %q; want %q", runShed.OnDone, NameWorktreeTeardown)
	}
	if runShed.OnStuck != NameRunShed {
		t.Errorf("Run-Shed OnStuck = %q; want %q -- a static self-route, not the empty escalate-to-human edge", runShed.OnStuck, NameRunShed)
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

	wantNames := []string{NameWorktreeCreate, NameSeedChild, NameRunShed, NameWorktreeTeardown}
	if len(shed.Producers) != len(wantNames) {
		t.Fatalf("New() row count = %d, want %d", len(shed.Producers), len(wantNames))
	}
	for i, want := range wantNames {
		if got := shed.Producers[i].Name; got != want {
			t.Errorf("row %d name = %q; want %q", i, got, want)
		}
	}
}

// TestRecipeEngines_ReportsExactlyTheFourEngineNamesSorted asserts RecipeEngines() returns
// exactly the four engine names, sorted -- this is the input the cross-consumer guard trusts, so
// a silently empty return would disable that guard rather than fail it.
func TestRecipeEngines_ReportsExactlyTheFourEngineNamesSorted(t *testing.T) {
	got := RecipeEngines()
	want := []string{"InnerRun", "SeedChild", "WorktreeCreate", "WorktreeTeardown"}

	if len(got) != len(want) {
		t.Fatalf("RecipeEngines() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("RecipeEngines()[%d] = %q; want %q", i, got[i], w)
		}
	}
}
