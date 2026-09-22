// commitstatus_test.go pins batten's per-transition status seam: the on-disk no-op-transition skip
// added on top of loomcli's own three dispositions -- commit-hard-errors, push-warns,
// skip-while-mid-merge -- carried over unchanged. Every test here drives newCommitStatusSeam
// against injected commitStatusDeps stub closures and a real temp-file marker path, spawning no git
// and no process, so the file stays Tier 1 with no hub fixture.
package battencli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestNewCommitStatusSeam_RepeatedPairCommitsOnce is the disproportionate-weight assertion named in
// the batch: a repeated (producer, state) pair commits and pushes exactly once, not twice, across
// two calls to the SAME seam instance.
func TestNewCommitStatusSeam_RepeatedPairCommitsOnce(t *testing.T) {
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
	if err := seam("Run-Shed", "running"); err != nil {
		t.Fatalf("seam() second call = %v; want nil", err)
	}

	if commits != 1 {
		t.Errorf("commits = %d; want exactly 1", commits)
	}
	if pushes != 1 {
		t.Errorf("pushes = %d; want exactly 1", pushes)
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

// TestCommitStatusMessage renders exactly "batten: <producer> -> <state>" for a table of
// producer/state pairs, so a regression to a bare constant fails here.
func TestCommitStatusMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		producer string
		state    string
	}{
		{"WorktreeCreateRunning", "Worktree-Create", "running"},
		{"RunShedPaused", "Run-Shed", "paused"},
		{"WorktreeTeardownDone", "Worktree-Teardown", "done"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitStatusMessage(tt.producer, tt.state)
			want := "batten: " + tt.producer + " -> " + tt.state
			if got != want {
				t.Errorf("commitStatusMessage(%q, %q) = %q; want %q", tt.producer, tt.state, got, want)
			}
		})
	}
}

// TestNewCommitStatusSeam_CommitErrorPropagates asserts a Commit error propagates out of the seam
// unchanged, and that Push is never called and no marker is written.
func TestNewCommitStatusSeam_CommitErrorPropagates(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "last-commit")
	markerLockPath := markerPath + ".lock"

	commitErr := errors.New("commit failed")
	pushCalled := false
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) error { return commitErr },
		Push:        func() error { pushCalled = true; return nil },
	}

	seam := newCommitStatusSeam(deps, markerPath, markerLockPath)
	if err := seam("Run-Shed", "running"); !errors.Is(err, commitErr) {
		t.Errorf("seam(...) = %v; want %v", err, commitErr)
	}
	if pushCalled {
		t.Error("Push was called; want it skipped when Commit errors")
	}
}

// TestNewCommitStatusSeam_CommitFailsAfterMergeWentLive_TakesTheSkip pins the other end of the
// unlocked probe window, mirroring loomcli's own test of the identical property.
func TestNewCommitStatusSeam_CommitFailsAfterMergeWentLive_TakesTheSkip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "last-commit")
	markerLockPath := markerPath + ".lock"

	probes := 0
	pushCalled := false
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) {
			probes++
			return probes > 1, nil
		},
		Commit: func(msg string) error {
			return errors.New("gitrepo: git commit: fatal: cannot do a partial commit during a merge")
		},
		Push: func() error { pushCalled = true; return nil },
	}

	seam := newCommitStatusSeam(deps, markerPath, markerLockPath)
	if err := seam("Run-Shed", "running"); err != nil {
		t.Errorf("seam(...) = %v; want nil -- a commit failure a live merge explains takes the skip disposition, never the halt", err)
	}
	if probes != 2 {
		t.Errorf("MergeActive called %d time(s); want exactly 2", probes)
	}
	if pushCalled {
		t.Error("Push was called; want it skipped -- nothing was committed to push")
	}
}

// TestNewCommitStatusSeam_PushErrorReturnsNil asserts a Push error returns nil from the seam, while
// Commit still ran and the marker was still written.
func TestNewCommitStatusSeam_PushErrorReturnsNil(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "last-commit")
	markerLockPath := markerPath + ".lock"

	commitCalled := false
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) error { commitCalled = true; return nil },
		Push:        func() error { return errors.New("push failed") },
	}

	seam := newCommitStatusSeam(deps, markerPath, markerLockPath)
	if err := seam("Run-Shed", "running"); err != nil {
		t.Errorf("seam(...) = %v; want nil", err)
	}
	if !commitCalled {
		t.Error("Commit was not called; want it to have run before Push")
	}
}

// TestNewCommitStatusSeam_PushRejectedReturnsNil asserts gitrepo.ErrPushRejected specifically
// returns nil from the seam, since a rejection is the routine multi-machine case this feature
// creates.
func TestNewCommitStatusSeam_PushRejectedReturnsNil(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "last-commit")
	markerLockPath := markerPath + ".lock"

	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) error { return nil },
		Push:        func() error { return gitrepo.ErrPushRejected },
	}

	seam := newCommitStatusSeam(deps, markerPath, markerLockPath)
	if err := seam("Run-Shed", "running"); err != nil {
		t.Errorf("seam(...) = %v; want nil on a rejected push", err)
	}
}

// TestNewCommitStatusSeam_MergeActiveSkips asserts MergeActive reporting true skips both Commit and
// Push and returns nil, and that MergeActive returning a non-nil error does exactly the same.
func TestNewCommitStatusSeam_MergeActiveSkips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mergeActive func() (bool, error)
	}{
		{"ReportsTrue", func() (bool, error) { return true, nil }},
		{"ProbeErrors", func() (bool, error) { return false, errors.New("probe unreadable") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			markerPath := filepath.Join(dir, "last-commit")
			markerLockPath := markerPath + ".lock"

			commitCalled := false
			pushCalled := false
			deps := commitStatusDeps{
				MergeActive: tt.mergeActive,
				Commit:      func(msg string) error { commitCalled = true; return nil },
				Push:        func() error { pushCalled = true; return nil },
			}

			seam := newCommitStatusSeam(deps, markerPath, markerLockPath)
			if err := seam("Run-Shed", "running"); err != nil {
				t.Errorf("seam(...) = %v; want nil", err)
			}
			if commitCalled {
				t.Error("Commit was called; want it skipped while mid-merge")
			}
			if pushCalled {
				t.Error("Push was called; want it skipped while mid-merge")
			}
		})
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
	if len(got) != 1 || got[0] != shedrun.StatusRel(runID) {
		t.Fatalf("battenRunCommitPaths with no seed on disk = %v; want just %q", got, shedrun.StatusRel(runID))
	}

	if err := shedrun.WriteSeed(loc, runID, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	got = battenRunCommitPaths(loc, runID)
	want := []string{shedrun.StatusRel(runID), shedrun.SeedRel(runID)}
	if len(got) != len(want) {
		t.Fatalf("battenRunCommitPaths with a seed on disk = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("battenRunCommitPaths(...)[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}
