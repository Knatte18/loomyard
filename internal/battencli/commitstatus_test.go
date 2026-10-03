// commitstatus_test.go pins batten's per-transition status seam: the on-disk no-op-transition skip
// added on top of the shared statuscommit core's dispositions, which statuscommit's own tests pin.
// Every test here drives newCommitStatusSeam against injected commitStatusDeps stub closures and a
// real temp-file marker path, spawning no git and no process, so the file stays Tier 1 with no hub
// fixture.
package battencli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// runStatusRel is the status file's relative path for runID.
func runStatusRel(loc *lyxcwd.Location, runID string) string {
	return shedrun.StatusRel(loc, runID)
}

// TestNewCommitStatusSeam_RepeatedPairCommitsOnce is the disproportionate-weight assertion named in
// the batch: a repeated (producer, state) pair commits and pushes exactly once, not twice, across
// two calls to the SAME seam instance. It also pins batten's own commit-message prefix, which the
// shared core's tests cannot, since they render whatever prefix they are given.
func TestNewCommitStatusSeam_RepeatedPairCommitsOnce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "last-commit")
	markerLockPath := markerPath + ".lock"

	var commits, pushes int
	var msg string
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(m string) error { commits++; msg = m; return nil },
		Push:        func() error { pushes++; return nil },
	}

	seam := newCommitStatusSeam(deps, markerPath, markerLockPath)

	if err := seam("Run-Shed", "running"); err != nil {
		t.Fatalf("seam() first call = %v; want nil", err)
	}
	if err := seam("Run-Shed", "running"); err != nil {
		t.Fatalf("seam() second call = %v; want nil", err)
	}

	if commits != 1 {
		t.Errorf("commits = %d; want exactly 1", commits)
	}
	if pushes != 1 {
		t.Errorf("pushes = %d; want exactly 1", pushes)
	}
	if want := "batten: Run-Shed -> running"; msg != want {
		t.Errorf("Commit msg = %q; want %q", msg, want)
	}
}

// TestNewCommitStatusSeam_SkipSurvivesARebuiltSeam is the disproportionate-weight assertion named in
// the batch: the skip still holds when the seam is rebuilt from scratch between calls -- the one
// assertion that actually proves the on-disk marker rather than an in-closure variable. A test
// exercising only one long-lived seam instance (as above) would pass against a broken in-memory
// design and proves nothing on its own.
func TestNewCommitStatusSeam_SkipSurvivesARebuiltSeam(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "last-commit")
	markerLockPath := markerPath + ".lock"

	var commits, pushes int
	newDeps := func() commitStatusDeps {
		return commitStatusDeps{
			MergeActive: func() (bool, error) { return false, nil },
			Commit:      func(msg string) error { commits++; return nil },
			Push:        func() error { pushes++; return nil },
		}
	}

	// First seam instance, first call: commits and pushes.
	firstSeam := newCommitStatusSeam(newDeps(), markerPath, markerLockPath)
	if err := firstSeam("Run-Shed", "running"); err != nil {
		t.Fatalf("firstSeam() = %v; want nil", err)
	}

	// A brand new seam instance, over the same marker path -- the shape a fresh `lyx batten step`
	// process actually takes. It must still see the marker on disk and skip.
	secondSeam := newCommitStatusSeam(newDeps(), markerPath, markerLockPath)
	if err := secondSeam("Run-Shed", "running"); err != nil {
		t.Fatalf("secondSeam() = %v; want nil", err)
	}

	if commits != 1 {
		t.Errorf("commits = %d; want exactly 1 -- the second, freshly-built seam instance must still see the marker", commits)
	}
	if pushes != 1 {
		t.Errorf("pushes = %d; want exactly 1", pushes)
	}
}

// TestNewCommitStatusSeam_ChangedPairCommitsAgain asserts a changed (producer, state) pair commits
// and pushes again rather than being absorbed by the skip.
func TestNewCommitStatusSeam_ChangedPairCommitsAgain(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "last-commit")
	markerLockPath := markerPath + ".lock"

	var commits, pushes int
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) error { commits++; return nil },
		Push:        func() error { pushes++; return nil },
	}

	seam := newCommitStatusSeam(deps, markerPath, markerLockPath)

	if err := seam("Run-Shed", "running"); err != nil {
		t.Fatalf("seam() first call = %v; want nil", err)
	}
	if err := seam("Run-Shed", "paused"); err != nil {
		t.Fatalf("seam() second call = %v; want nil", err)
	}

	if commits != 2 {
		t.Errorf("commits = %d; want exactly 2 -- a changed pair must not be absorbed by the skip", commits)
	}
	if pushes != 2 {
		t.Errorf("pushes = %d; want exactly 2", pushes)
	}
}

// TestNewCommitStatusSeam_MissingMarkerCommitsOnce asserts a missing marker file falls back to
// committing once rather than erroring.
func TestNewCommitStatusSeam_MissingMarkerCommitsOnce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "does-not-exist", "last-commit")
	markerLockPath := markerPath + ".lock"

	var commits int
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) error { commits++; return nil },
		Push:        func() error { return nil },
	}

	seam := newCommitStatusSeam(deps, markerPath, markerLockPath)
	if err := seam("Run-Shed", "running"); err != nil {
		t.Fatalf("seam() = %v; want nil", err)
	}
	if commits != 1 {
		t.Errorf("commits = %d; want exactly 1", commits)
	}
}

// TestNewCommitStatusSeam_CorruptMarkerCommitsOnce asserts a corrupt (undecodable) marker file also
// falls back to committing once rather than erroring, exactly as a missing marker does: the marker
// is a cache, and losing it costs one redundant commit, never correctness.
func TestNewCommitStatusSeam_CorruptMarkerCommitsOnce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "last-commit")
	markerLockPath := markerPath + ".lock"
	if err := os.WriteFile(markerPath, []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatalf("seed corrupt marker: %v", err)
	}

	var commits int
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) error { commits++; return nil },
		Push:        func() error { return nil },
	}

	seam := newCommitStatusSeam(deps, markerPath, markerLockPath)
	if err := seam("Run-Shed", "running"); err != nil {
		t.Fatalf("seam() = %v; want nil", err)
	}
	if commits != 1 {
		t.Errorf("commits = %d; want exactly 1", commits)
	}
}

// TestBattenRunCommitPaths asserts a status transition commits the run's seed alongside its status
// once one exists, and commits the status alone while none does.
// A run directory is durable, fabric-synced state: a status committed without its seed leaves a
// resumed machine able to read how far the run came but not what it is running.
func TestBattenRunCommitPaths(t *testing.T) {
	const runID = "some-slug"
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}

	got := battenRunCommitPaths(loc, runID)
	if len(got) != 1 || got[0] != runStatusRel(loc, runID) {
		t.Fatalf("battenRunCommitPaths with no seed on disk = %v; want just %q", got, runStatusRel(loc, runID))
	}

	if err := shedrun.WriteSeed(loc, runID, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	got = battenRunCommitPaths(loc, runID)
	want := []string{runStatusRel(loc, runID), shedrun.SeedRel(loc, runID)}
	if len(got) != len(want) {
		t.Fatalf("battenRunCommitPaths with a seed on disk = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("battenRunCommitPaths(...)[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}
