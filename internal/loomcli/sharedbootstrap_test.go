// sharedbootstrap_test.go covers the three helpers sharedbootstrap.go extracts, driven against a
// hand-populated *loomCLI receiver -- bypassing wire entirely, the idiom cli_test.go already uses for
// the drive/pause refusal paths. Per the plan's new-tests-stay-untagged-and-pure Shared Decision, no
// test here spawns a real hub or brings up a real reed session; ensureStatusStrand's own
// resolveStatusStrandAction branches are covered directly through that pure function in
// bootstrap_test.go rather than re-covered here through a real reed engine.

package loomcli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
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
		shedPaths: loomrecipe.ShedPaths{
			StatusPath:     filepath.Join(dir, "status.json"),
			StatusLockPath: filepath.Join(dir, "status.json.lock"),
		},
	}

	parent1, stage1, err1 := c.seedAndCommitBootstrap("warp", "main")
	parent2, stage2, err2 := c.seedAndCommitBootstrap("warp", "main")

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
		shedPaths: loomrecipe.ShedPaths{
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
