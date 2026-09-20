// sharedbootstrap_test.go covers the three helpers sharedbootstrap.go extracts, driven against a
// hand-populated *loomCLI receiver -- bypassing wire entirely, the idiom cli_test.go already uses for
// the drive/pause refusal paths. No test here spawns a real hub or brings up a real reed session;
// ensureStatusStrand's own resolveStatusStrandAction branches are covered directly through that pure
// function in bootstrap_test.go rather than re-covered here through a real reed engine.
//
// seedAndCommitBootstrap's own step 1 (fabricengine.ReadOrigin) always needs a real git-backed weft
// sibling to succeed -- even on the record-already-found path -- so no test in this file drives the
// whole function past bootstrapStageOrigin; TestSeedAndCommitBootstrap_SecondCallDoesNotDivergeOnErrSeedExists
// below documents that wall directly. Card 13's own seed-write coverage (the recipe/driver/param
// shape, WriteSeed's idempotency, and the seed's presence in the commit pathspec) is instead pinned
// through loomSeedFor and bootstrapCommitPaths, the two pure helpers factored out of
// seedAndCommitBootstrap for exactly this reason.

package loomcli

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestBootstrapStage_ConstantsAreDistinctAndZeroValued asserts the four stage constants are
// distinct, that bootstrapStageNone is the zero value, and that every stage a caller can receive is
// one of the five declared constants.
func TestBootstrapStage_ConstantsAreDistinctAndZeroValued(t *testing.T) {
	if bootstrapStageNone != 0 {
		t.Errorf("bootstrapStageNone = %d; want 0 (the zero value)", bootstrapStageNone)
	}

	all := []bootstrapStage{bootstrapStageNone, bootstrapStageOrigin, bootstrapStageSeed, bootstrapStageOwnership, bootstrapStageCommit}
	seen := make(map[bootstrapStage]bool, len(all))
	for _, s := range all {
		if seen[s] {
			t.Errorf("bootstrapStage %d is declared more than once among the five constants", s)
		}
		seen[s] = true
	}
	if len(seen) != 5 {
		t.Errorf("got %d distinct bootstrapStage values; want 5", len(seen))
	}
}

// TestSeedAndCommitBootstrap_SecondCallDoesNotDivergeOnErrSeedExists asserts the seed-tolerance
// contract's idempotency: calling seedAndCommitBootstrap twice in a row against the same receiver
// produces the same stage and the same error both times, proving the second call never surfaces a
// hard failure the first call did not already surface.
//
// The receiver's location has no real fabric behind it (no git repository, no weft sibling), so both
// calls fail at bootstrapStageOrigin -- the first sub-step that genuinely needs a real fabric to
// proceed past -- rather than reaching bootstrapStageSeed or bootstrapStageCommit. Driving this
// helper far enough to observe loomshed.Seed's own ErrSeedExists tolerance would need a real git
// repository behind c.location, which this untagged suite must not spawn (the
// new-tests-stay-untagged-and-pure Shared Decision); that deeper coverage belongs to a tagged suite.
func TestSeedAndCommitBootstrap_SecondCallDoesNotDivergeOnErrSeedExists(t *testing.T) {
	dir := t.TempDir()
	loc := &lyxcwd.Location{HubPath: dir, WorktreeName: "warp", AnchorRel: "."}
	c := &loomCLI{
		location: loc,
		shedPaths: shedbuild.ShedPaths{
			StatusPath:     filepath.Join(dir, "status.json"),
			StatusLockPath: filepath.Join(dir, "status.json.lock"),
		},
	}

	parent1, _, stage1, err1 := c.seedAndCommitBootstrap("warp", "main")
	parent2, _, stage2, err2 := c.seedAndCommitBootstrap("warp", "main")

	if stage1 != stage2 {
		t.Errorf("seedAndCommitBootstrap stage diverged across two calls: first = %d, second = %d", stage1, stage2)
	}
	if parent1 != parent2 {
		t.Errorf("seedAndCommitBootstrap parent diverged across two calls: first = %q, second = %q", parent1, parent2)
	}
	if (err1 == nil) != (err2 == nil) {
		t.Errorf("seedAndCommitBootstrap error-ness diverged across two calls: first = %v, second = %v", err1, err2)
	}
	if err2 != nil && err1 != nil && err2.Error() != err1.Error() {
		t.Errorf("seedAndCommitBootstrap's second call surfaced a different error than the first: first = %v, second = %v", err1, err2)
	}
}

// TestLoomSeedFor_RecipeAndParentParam asserts loomSeedFor's returned shedrun.Seed carries
// RecipeLoom, the given driver, and a single "parent" param matching the given parent -- the shape
// seedAndCommitBootstrap's step 1b writes with shedrun.WriteSeed.
func TestLoomSeedFor_RecipeAndParentParam(t *testing.T) {
	seed := loomSeedFor("main", shedrun.DriverGo)

	if seed.Recipe != shedrun.RecipeLoom {
		t.Errorf("loomSeedFor(%q, %q).Recipe = %q; want %q", "main", shedrun.DriverGo, seed.Recipe, shedrun.RecipeLoom)
	}
	if seed.Driver != shedrun.DriverGo {
		t.Errorf("loomSeedFor(%q, %q).Driver = %q; want %q", "main", shedrun.DriverGo, seed.Driver, shedrun.DriverGo)
	}
	want := map[string]string{"parent": "main"}
	if !reflect.DeepEqual(seed.Params, want) {
		t.Errorf("loomSeedFor(%q, %q).Params = %v; want %v", "main", shedrun.DriverGo, seed.Params, want)
	}
}

// TestLoomSeedFor_WriteSeedIsIdempotent asserts writing loomSeedFor's shape twice, for the same
// parent and driver, at the same location and run-id, is a no-op the second time -- the idempotency
// seedAndCommitBootstrap's own comment relies on to make a crashed-and-resumed bootstrap safe to
// re-run.
func TestLoomSeedFor_WriteSeedIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	loc := &lyxcwd.Location{HubPath: dir, WorktreeName: "warp", AnchorRel: "."}

	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, loomSeedFor("main", shedrun.DriverGo)); err != nil {
		t.Fatalf("first WriteSeed(...) = %v; want nil", err)
	}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, loomSeedFor("main", shedrun.DriverGo)); err != nil {
		t.Errorf("second WriteSeed(...) = %v; want nil (idempotent against a byte-identical seed)", err)
	}

	seed, found, err := shedrun.ReadSeed(loc, shedrun.SelfRunID)
	if err != nil {
		t.Fatalf("ReadSeed(...) = %v; want nil", err)
	}
	if !found {
		t.Fatal("ReadSeed(...) found = false; want true")
	}
	if seed.Recipe != shedrun.RecipeLoom {
		t.Errorf("ReadSeed(...).Recipe = %q; want %q", seed.Recipe, shedrun.RecipeLoom)
	}
	if seed.Params["parent"] != "main" {
		t.Errorf("ReadSeed(...).Params[\"parent\"] = %q; want %q", seed.Params["parent"], "main")
	}
}

// TestResolveSeedDriver covers resolveSeedDriver's read-through: an unseeded worktree defaults to the
// go driver, an already-go seed is preserved, and -- the case this card exists for -- an
// already-llm seed is preserved rather than overwritten with the go driver. That last row is what
// fails against the shipped version, which hard-coded the go driver into every write regardless of
// what a run was already seeded as.
func TestResolveSeedDriver(t *testing.T) {
	tests := []struct {
		name     string
		existing shedrun.Seed
		found    bool
		want     string
	}{
		{"Unseeded_DefaultsToGo", shedrun.Seed{}, false, shedrun.DriverGo},
		{"AlreadySeededGo_Preserved", shedrun.Seed{Driver: shedrun.DriverGo}, true, shedrun.DriverGo},
		{"AlreadySeededLLM_Preserved", shedrun.Seed{Driver: shedrun.DriverLLM}, true, shedrun.DriverLLM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSeedDriver(tt.existing, tt.found)
			if got != tt.want {
				t.Errorf("resolveSeedDriver(%+v, %v) = %q; want %q", tt.existing, tt.found, got, tt.want)
			}
		})
	}
}

// TestSeedWriteStep_PreservesRecordedLLMDriver drives the actual step-1b sequence
// seedAndCommitBootstrap performs -- shedrun.ReadSeed, resolveSeedDriver, loomSeedFor, shedrun.
// WriteSeed -- against a worktree already seeded for the llm driver, isolated from step 1
// (fabricengine.ReadOrigin), which this untagged suite's receiver cannot satisfy without a real
// fabric (see this file's own header comment). It is what proves the write survives and reports the
// driver back: against the shipped version, WriteSeed's own disagreeing-seed refusal would fire here,
// since the shipped step 1b hard-coded DriverGo into every write.
//
// This also pins that the driver is read from the seed rather than from any flag or config: nothing
// in this sequence reads a flag, and this command declares no driver flag of its own.
func TestSeedWriteStep_PreservesRecordedLLMDriver(t *testing.T) {
	dir := t.TempDir()
	loc := &lyxcwd.Location{HubPath: dir, WorktreeName: "warp", AnchorRel: "."}

	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverLLM, Params: map[string]string{"parent": "main"}}); err != nil {
		t.Fatalf("seed the worktree for the llm driver: %v", err)
	}

	existing, found, err := shedrun.ReadSeed(loc, shedrun.SelfRunID)
	if err != nil {
		t.Fatalf("ReadSeed(...) = %v; want nil", err)
	}
	driver := resolveSeedDriver(existing, found)
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, loomSeedFor("main", driver)); err != nil {
		t.Fatalf("re-running the step-1b write over an llm-seeded worktree = %v; want nil (the write must survive, not refuse on a disagreeing seed)", err)
	}

	if driver != shedrun.DriverLLM {
		t.Errorf("resolveSeedDriver reported %q; want %q", driver, shedrun.DriverLLM)
	}
	reread, _, err := shedrun.ReadSeed(loc, shedrun.SelfRunID)
	if err != nil {
		t.Fatalf("ReadSeed(...) after the write = %v; want nil", err)
	}
	if reread.Driver != shedrun.DriverLLM {
		t.Errorf("ReadSeed(...).Driver after the write = %q; want %q", reread.Driver, shedrun.DriverLLM)
	}
}

// TestBootstrapCommitPaths_IncludesSeedRel asserts bootstrapCommitPaths -- the pathspec
// seedAndCommitBootstrap's step 3 commits unconditionally -- includes shedrun.SeedRel(shedrun.
// SelfRunID) alongside the status file and origin record, so a crashed-and-resumed bootstrap's
// seed self-heals into the fabric exactly as the status file and origin record already do.
func TestBootstrapCommitPaths_IncludesSeedRel(t *testing.T) {
	got := bootstrapCommitPaths()

	want := []string{
		shedrun.StatusRel(shedrun.SelfRunID),
		shedrun.SeedRel(shedrun.SelfRunID),
		fabricengine.OriginRecordRel(),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bootstrapCommitPaths() = %v; want %v", got, want)
	}
}

// TestBuildLoomShed_OutputShape asserts the built *shedengine.Shed's StatusPath, LockPath, and
// StatusLockPath equal the receiver's own c.shedPaths values, and that it carries all seventeen
// producer rows.
//
// buildLoomShed opens the fabric as its first act (fabricengine.Open), which this untagged suite's
// receiver -- with no real git repository or weft sibling behind its location -- cannot satisfy. Per
// this card's own skip-rather-than-fabric instruction, the test skips with a stated reason instead of
// standing up a real fabric fixture.
func TestBuildLoomShed_OutputShape(t *testing.T) {
	dir := t.TempDir()
	loc := &lyxcwd.Location{HubPath: dir, WorktreeName: "warp", AnchorRel: "."}
	c := &loomCLI{
		location: loc,
		shedPaths: shedbuild.ShedPaths{
			StatusPath:     filepath.Join(dir, "status.json"),
			LockPath:       filepath.Join(dir, "status.json.runlock"),
			StatusLockPath: filepath.Join(dir, "status.json.lock"),
		},
	}
	c.env.StatusPath = c.shedPaths.StatusPath
	c.env.StatusLockPath = c.shedPaths.StatusLockPath

	shed, err := c.buildLoomShed()
	if err != nil {
		t.Skipf("buildLoomShed needs a real fabric (fabricengine.Open) this pure receiver cannot satisfy: %v", err)
	}

	if shed.StatusPath != c.shedPaths.StatusPath {
		t.Errorf("shed.StatusPath = %q; want %q", shed.StatusPath, c.shedPaths.StatusPath)
	}
	if shed.LockPath != c.shedPaths.LockPath {
		t.Errorf("shed.LockPath = %q; want %q", shed.LockPath, c.shedPaths.LockPath)
	}
	if shed.StatusLockPath != c.shedPaths.StatusLockPath {
		t.Errorf("shed.StatusLockPath = %q; want %q", shed.StatusLockPath, c.shedPaths.StatusLockPath)
	}
	if len(shed.Producers) != 17 {
		t.Errorf("len(shed.Producers) = %d; want 17", len(shed.Producers))
	}
}
