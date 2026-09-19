// recipe_test.go builds through New and asserts the shape the lifecycle recipe's three rows must
// carry: their names, the OnDone chain, the load-bearing empty OnStuck/OnDone edges, the absence
// of any Segment, and that the returned *shedengine.Shed carries ShedPaths' five values verbatim.
// It also asserts RecipeEngines()'s own shape, since a silently empty return would disable the
// cross-consumer coverage guard rather than fail it.

package lifecyclerecipe

import (
	"testing"
)

// TestNew_RowNamesMatchTheDurableIdentityConstants asserts New assembles exactly three rows, named
// exactly the three Name* constants -- the durable-identity guard internal/loomrecipe's own
// row-name test mirrors.
func TestNew_RowNamesMatchTheDurableIdentityConstants(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	if len(shed.Producers) != 3 {
		t.Fatalf("New() row count = %d, want 3", len(shed.Producers))
	}

	wantNames := []string{NameWorktreeCreate, NameLoomRun, NameWorktreeTeardown}
	for i, want := range wantNames {
		if got := shed.Producers[i].Name; got != want {
			t.Errorf("row %d name = %q; want %q", i, got, want)
		}
	}
}

// TestNew_OnDoneChainAndLoadBearingEmptyEdges asserts the Worktree-Create -> Loom-Run ->
// Worktree-Teardown OnDone chain, Loom-Run's OnStuck is empty, Worktree-Teardown's OnDone is
// empty, and no row declares a Segment.
func TestNew_OnDoneChainAndLoadBearingEmptyEdges(t *testing.T) {
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
	if create.OnDone != NameLoomRun {
		t.Errorf("Worktree-Create OnDone = %q; want %q", create.OnDone, NameLoomRun)
	}

	loomRun := shed.Producers[byName[NameLoomRun]]
	if loomRun.OnDone != NameWorktreeTeardown {
		t.Errorf("Loom-Run OnDone = %q; want %q", loomRun.OnDone, NameWorktreeTeardown)
	}
	if loomRun.OnStuck != "" {
		t.Errorf("Loom-Run OnStuck = %q; want empty -- this edge escalates to a human with the task worktree intact", loomRun.OnStuck)
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

// TestNameLoomRun_ValueIsPinnedAgainstASymmetryRename exists because shedengine persists
// CurrentProducer -- the row name, not the engine name -- into the status file, so renaming a row
// breaks resume for an in-flight run. This guard makes a later symmetry-minded rename of the
// durable identity (matching the engine's InnerRun rename onto the row) fail loudly here rather
// than silently breaking resume.
func TestNameLoomRun_ValueIsPinnedAgainstASymmetryRename(t *testing.T) {
	if NameLoomRun != "Loom-Run" {
		t.Errorf("NameLoomRun = %q; want the durable value %q", NameLoomRun, "Loom-Run")
	}

	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	wantNames := []string{NameWorktreeCreate, NameLoomRun, NameWorktreeTeardown}
	if len(shed.Producers) != len(wantNames) {
		t.Fatalf("New() row count = %d, want %d", len(shed.Producers), len(wantNames))
	}
	for i, want := range wantNames {
		if got := shed.Producers[i].Name; got != want {
			t.Errorf("row %d name = %q; want %q", i, got, want)
		}
	}
}

// TestRecipeEngines_ReportsExactlyTheThreeEngineNamesSorted asserts RecipeEngines() returns
// exactly the three engine names, sorted -- this is the input the cross-consumer guard trusts, so
// a silently empty return would disable that guard rather than fail it.
func TestRecipeEngines_ReportsExactlyTheThreeEngineNamesSorted(t *testing.T) {
	got := RecipeEngines()
	want := []string{"InnerRun", "WorktreeCreate", "WorktreeTeardown"}

	if len(got) != len(want) {
		t.Fatalf("RecipeEngines() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("RecipeEngines()[%d] = %q; want %q", i, got[i], w)
		}
	}
}
