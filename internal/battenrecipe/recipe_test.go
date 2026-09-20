// recipe_test.go builds through New and asserts the shape the batten recipe's four rows must
// carry: their names, the OnDone chain, the load-bearing empty/self-route OnStuck edges and empty
// terminal OnDone edge, Run-Shed's own bounce budget, the absence of any Segment, and that the
// returned *shedengine.Shed carries ShedPaths' five values verbatim. It also asserts RecipeEngines()'s
// own shape, since a silently empty return would disable the cross-consumer coverage guard rather
// than fail it.

package battenrecipe

import (
	"testing"
	"time"
)

// wantRunShedMaxBounces and wantRunShedPollIntervalS are Run-Shed's own pinned bounce budget: 1440
// bounces at 30 seconds encodes a 12-hour watch window, the same wall clock the retired Go
// constants once expressed as an attempt count. Asserting against the wall clock they encode,
// rather than as bare numbers, is what catches a value drifted to encode a different window instead
// of a silently dropped budget.
const (
	wantRunShedMaxBounces    = 1440
	wantRunShedPollIntervalS = 30
	wantRunShedWatchWindow   = 12 * time.Hour
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

// TestNew_RunShedBounceBudgetEncodesTheTwelveHourWindow pins Run-Shed's own MaxBounces and its
// config's poll_interval_s together, asserted against the 12-hour wall clock they encode rather
// than as bare numbers -- a dropped max_bounces would silently inherit the engine's default budget
// of ten bounces and turn every long child run into a spurious StateBlocked, the
// highest-consequence, lowest-visibility failure this recipe can carry.
func TestNew_RunShedBounceBudgetEncodesTheTwelveHourWindow(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	byName := make(map[string]int, len(shed.Producers))
	for i, p := range shed.Producers {
		byName[p.Name] = i
	}
	row := shed.Producers[byName[NameRunShed]]

	if row.MaxBounces != wantRunShedMaxBounces {
		t.Errorf("Run-Shed MaxBounces = %d; want %d", row.MaxBounces, wantRunShedMaxBounces)
	}

	gotWindow := time.Duration(wantRunShedMaxBounces) * time.Duration(wantRunShedPollIntervalS) * time.Second
	if gotWindow != wantRunShedWatchWindow {
		t.Errorf("Run-Shed's pinned max_bounces (%d) * poll_interval_s (%ds) = %s; want the 12-hour watch window %s", wantRunShedMaxBounces, wantRunShedPollIntervalS, gotWindow, wantRunShedWatchWindow)
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
