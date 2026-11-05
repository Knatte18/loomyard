// commitstatus_test.go pins batten's per-transition status seam: the on-disk no-op-transition skip added on top of the shared statuscommit core's dispositions, which statuscommit's own tests pin.
// Every test here drives newCommitStatusSeam against injected commitStatusDeps stub closures and a real temp-file marker path, spawning no git and no process,
// so the file stays Tier 1 with no hub fixture.
package battencli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// runStatusRel is the status file's relative path for runID.
func runStatusRel(loc *lymcwd.Location, runID string) string {
	return shedrun.StatusRel(loc, runID)
}

// TestNewCommitStatusSeam covers the on-disk no-op-transition skip over a real temp-file marker:
//   - a repeated (producer, state) pair commits and spawns the push exactly once, not twice, across two calls to the same seam instance;
//   - the skip still holds when the seam is rebuilt from scratch between calls, the shape a fresh "lym batten step" process takes -- the one case that proves the on-disk marker rather than an in-closure variable;
//   - a changed pair commits and spawns again rather than being absorbed by the skip;
//   - a spawn that fails to start still returns nil;
//   - a missing or corrupt marker falls back to committing once rather than erroring, since the marker is a cache and losing it costs one redundant commit, never correctness.
//
// The commit message pins batten's own prefix, which the shared core's tests cannot, since they render whatever prefix they are given.
func TestNewCommitStatusSeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	type transition struct{ producer, state string }

	tests := []struct {
		name string
		// markerRel is the marker path under the test's temp dir.
		markerRel     string
		corruptMarker bool
		// rebuildSeam builds a fresh seam instance, over the same marker, for every call.
		rebuildSeam bool
		calls       []transition
		spawnErr    error
		wantCommits int
	}{
		{name: "RepeatedPairCommitsOnce", markerRel: "last-commit", calls: []transition{{"Run-Shed", "running"}, {"Run-Shed", "running"}}, wantCommits: 1},
		{name: "FailingSpawnStillReturnsNil", markerRel: "last-commit", spawnErr: errors.New("spawn failed"), calls: []transition{{"Run-Shed", "running"}}, wantCommits: 1},
		{name: "SkipSurvivesARebuiltSeam", markerRel: "last-commit", rebuildSeam: true, calls: []transition{{"Run-Shed", "running"}, {"Run-Shed", "running"}}, wantCommits: 1},
		{name: "ChangedPairCommitsAgain", markerRel: "last-commit", calls: []transition{{"Run-Shed", "running"}, {"Run-Shed", "paused"}}, wantCommits: 2},
		{name: "MissingMarkerCommitsOnce", markerRel: filepath.Join("does-not-exist", "last-commit"), calls: []transition{{"Run-Shed", "running"}}, wantCommits: 1},
		{name: "CorruptMarkerCommitsOnce", markerRel: "last-commit", corruptMarker: true, calls: []transition{{"Run-Shed", "running"}}, wantCommits: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			markerPath := filepath.Join(t.TempDir(), tt.markerRel)
			markerLockPath := markerPath + ".lock"
			if tt.corruptMarker {
				if err := os.WriteFile(markerPath, []byte("not valid json{{{"), 0o644); err != nil {
					t.Fatalf("seed corrupt marker: %v", err)
				}
			}

			var commits, spawns int
			var msg string
			newSeam := func() func(producer, state string) error {
				return newCommitStatusSeam(commitStatusDeps{
					MergeActive: func() (bool, error) { return false, nil },
					Commit:      func(m string) (bool, error) { commits++; msg = m; return true, nil },
					Spawn:       func() error { spawns++; return tt.spawnErr },
				}, markerPath, markerLockPath)
			}

			seam := newSeam()
			for i, call := range tt.calls {
				if tt.rebuildSeam && i > 0 {
					seam = newSeam()
				}
				if err := seam(call.producer, call.state); err != nil {
					t.Fatalf("seam(%q, %q) call %d = %v; want nil", call.producer, call.state, i+1, err)
				}
			}

			if commits != tt.wantCommits {
				t.Errorf("commits = %d; want exactly %d", commits, tt.wantCommits)
			}
			if spawns != tt.wantCommits {
				t.Errorf("spawns = %d; want exactly %d", spawns, tt.wantCommits)
			}
			last := tt.calls[len(tt.calls)-1]
			if want := "batten: " + last.producer + " -> " + last.state; msg != want {
				t.Errorf("Commit msg = %q; want %q", msg, want)
			}
		})
	}
}

// TestNewCommitStatusSeam_WritesBoardStatusBeforeRunShed pins the seam's board write.
// A transition at a row before Run-Shed writes "<state> · <row>" on the entry.
// A transition at Run-Shed or a later row writes nothing, since the inner run writes the entry's status from its start on.
func TestNewCommitStatusSeam_WritesBoardStatusBeforeRunShed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	markerPath := filepath.Join(t.TempDir(), "last-commit")
	var written []string
	seam := newCommitStatusSeam(commitStatusDeps{
		MergeActive:    func() (bool, error) { return false, nil },
		Commit:         func(string) (bool, error) { return true, nil },
		Spawn:          func() error { return nil },
		SetBoardStatus: func(status string) error { written = append(written, status); return nil },
	}, markerPath, markerPath+".lock")

	for _, call := range []struct{ producer, state string }{
		{"Worktree-Create", "running"},
		{"Seed-Child", "blocked"},
		{"Open-IDE", "running"},
		{"Run-Shed", "running"},
		{"Land", "running"},
		{"Worktree-Teardown", "done"},
	} {
		if err := seam(call.producer, call.state); err != nil {
			t.Fatalf("seam(%q, %q) = %v; want nil", call.producer, call.state, err)
		}
	}

	want := []string{"running · Worktree-Create", "blocked · Seed-Child", "running · Open-IDE"}
	if !reflect.DeepEqual(written, want) {
		t.Errorf("board statuses written = %q; want %q", written, want)
	}
}

// TestNewCommitStatusSeam_BoardWriteFailureDoesNotStopTheCommit pins that a failing board write at a row before Run-Shed is only warned about:
// the seam returns nil and still commits and spawns the push.
func TestNewCommitStatusSeam_BoardWriteFailureDoesNotStopTheCommit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	markerPath := filepath.Join(t.TempDir(), "last-commit")
	var commits, spawns int
	seam := newCommitStatusSeam(commitStatusDeps{
		MergeActive:    func() (bool, error) { return false, nil },
		Commit:         func(string) (bool, error) { commits++; return true, nil },
		Spawn:          func() error { spawns++; return nil },
		SetBoardStatus: func(string) error { return errors.New("board unreadable") },
	}, markerPath, markerPath+".lock")

	if err := seam("Worktree-Create", "running"); err != nil {
		t.Fatalf("seam(Worktree-Create, running) = %v; want nil", err)
	}

	if commits != 1 || spawns != 1 {
		t.Errorf("commits = %d, spawns = %d; want 1 and 1", commits, spawns)
	}
}

// TestBattenCommitStatusDeps_SetBoardStatusWritesTheRunsEntry pins that the real SetBoardStatus writes the status onto the board entry whose slug is the run id.
// It sets the board skip variables, which are process-global, so it does not call t.Parallel.
func TestBattenCommitStatusDeps_SetBoardStatusWritesTheRunsEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Setenv("BOARD_SKIP_GIT", "1")
	t.Setenv("BOARD_SKIP_PUSH", "1")

	const slug = "some-slug"
	hub := boardkit.HubWithBoardConfig(t)
	board, err := boardengine.OpenHub(hub)
	if err != nil {
		t.Fatalf("OpenHub: %v", err)
	}
	if _, err := board.UpsertTask(map[string]any{"slug": slug, "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	location := &lymcwd.Location{HubPath: hub, WorktreeName: "code", AnchorRel: "."}

	status := boardengine.RunStatus("running", "Worktree-Create")
	if err := battenCommitStatusDeps(location, slug).SetBoardStatus(status); err != nil {
		t.Fatalf("SetBoardStatus(%q) = %v; want nil", status, err)
	}

	task, found, err := board.GetTask(slug)
	if err != nil || !found {
		t.Fatalf("GetTask(%q) = %v, %v; want the entry", slug, found, err)
	}
	if task.Status == nil || *task.Status != status {
		t.Errorf("board status = %v; want %q", task.Status, status)
	}
}

// TestStatusPushSpawns_TellTheirLocationAndGateSlug pins what batten's two push spawns ask fabric for: the status seam spawns for its location under the run's slug as gate slug, and cleanup spawns for its location with no gate slug.
// It swaps the package's spawnStatusPush for a recorder, so it is serial: no parallel test may run while the variable is swapped.
func TestStatusPushSpawns_TellTheirLocationAndGateSlug(t *testing.T) {
	tiergate.Require(t, 0)
	type spawnRequest struct {
		location *lymcwd.Location
		gateSlug string
	}
	var requests []spawnRequest
	original := spawnStatusPush
	spawnStatusPush = func(l *lymcwd.Location, gateSlug string) (fabricengine.SpawnResult, error) {
		requests = append(requests, spawnRequest{l, gateSlug})
		return fabricengine.SpawnResult{}, nil
	}
	t.Cleanup(func() { spawnStatusPush = original })

	location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}

	if err := battenCommitStatusDeps(location, "a-slug").Spawn(); err != nil {
		t.Fatalf("status seam Spawn() = %v; want nil", err)
	}
	if want := []spawnRequest{{location, "a-slug"}}; !reflect.DeepEqual(requests, want) {
		t.Errorf("status seam requests = %v; want %v", requests, want)
	}

	requests = nil
	if err := fabricCleanupDeps(location).Spawn(); err != nil {
		t.Fatalf("cleanup Spawn() = %v; want nil", err)
	}
	if want := []spawnRequest{{location, ""}}; !reflect.DeepEqual(requests, want) {
		t.Errorf("cleanup requests = %v; want %v", requests, want)
	}
}

// TestBattenRunCommitPaths asserts a commit of the run's records stages the run's seed, dobby's run notice marker, the settle records, the child-recipe record and each driver directory holding a file alongside its status once each exists, and commits the status alone while none does.
// A run directory is durable, fabric-synced state: a status committed without its seed leaves a resumed machine able to read how far the run came but not what it is running.
//
//testtiming:keep pins the seed-alongside-status commit paths, which the end-to-end rows never assert
func TestBattenRunCommitPaths(t *testing.T) {
	tiergate.Require(t, 0)
	const runID = "some-slug"
	loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}

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

	if err := os.WriteFile(shedrun.NoticeEpisodeFile(loc, runID), []byte("driver-dead|running|1\n"), 0o644); err != nil {
		t.Fatalf("write notice marker: %v", err)
	}
	got = battenRunCommitPaths(loc, runID)
	if want = append(want, shedrun.NoticeEpisodeRel(loc, runID)); !reflect.DeepEqual(got, want) {
		t.Errorf("battenRunCommitPaths with a notice marker on disk = %v; want %v", got, want)
	}

	if err := os.WriteFile(shedrun.SettleRecords(loc, runID), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write settle records: %v", err)
	}
	got = battenRunCommitPaths(loc, runID)
	if want = append(want, shedrun.SettleRecordsRel(loc, runID)); !reflect.DeepEqual(got, want) {
		t.Errorf("battenRunCommitPaths with settle records on disk = %v; want %v", got, want)
	}

	if err := shedrun.WriteChildRecipe(loc, runID, shedrun.RecipeMotif); err != nil {
		t.Fatalf("write child recipe: %v", err)
	}
	got = battenRunCommitPaths(loc, runID)
	if want = append(want, shedrun.ChildRecipeRel(loc, runID)); !reflect.DeepEqual(got, want) {
		t.Errorf("battenRunCommitPaths with a child-recipe record on disk = %v; want %v", got, want)
	}

	// A driver directory holding only an empty attempt directory is nothing to commit.
	if err := os.MkdirAll(shedrun.TacklerAttemptDir(loc, runID, 1), 0o755); err != nil {
		t.Fatalf("mkdir empty tackler attempt: %v", err)
	}
	got = battenRunCommitPaths(loc, runID)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("battenRunCommitPaths with an empty tackler attempt directory = %v; want %v", got, want)
	}

	driverFiles := []string{
		filepath.Join(shedrun.DriveReportsDir(loc, runID), "drive-envelope.json"),
		filepath.Join(shedrun.ArbiterAttemptDir(loc, runID, 1), "verdict.yaml"),
		filepath.Join(shedrun.TacklerAttemptDir(loc, runID, 1), "verdict.yaml"),
	}
	for _, path := range driverFiles {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	got = battenRunCommitPaths(loc, runID)
	want = append(want, shedrun.DriveReportsRel(loc, runID), shedrun.ArbiterRel(loc, runID), shedrun.TacklerRel(loc, runID))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("battenRunCommitPaths with driver records on disk = %v; want %v", got, want)
	}
}

// TestCommitRunRecords asserts the hand-back and done commit of the run's records:
// it commits and starts the push, starts no push when nothing was committed, commits nothing while the pair is mid-merge, returns a failed commit or merge-state read, and never returns a push that failed to start.
func TestCommitRunRecords(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name        string
		mergeActive bool
		mergeErr    error
		committed   bool
		commitErr   error
		spawnErr    error
		wantCommits int
		wantSpawns  int
		wantErr     error
	}{
		{name: "CommitsAndStartsThePush", committed: true, wantCommits: 1, wantSpawns: 1},
		{name: "NothingCommittedStartsNoPush", wantCommits: 1},
		{name: "MidMergeCommitsNothing", mergeActive: true},
		{name: "FailedMergeStateReadIsReturned", mergeErr: boom, wantErr: boom},
		{name: "FailedCommitIsReturned", commitErr: boom, wantCommits: 1, wantErr: boom},
		{name: "FailedPushStartIsNotReturned", committed: true, spawnErr: boom, wantCommits: 1, wantSpawns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var commits, spawns int
			var msg string
			err := commitRunRecords(commitStatusDeps{
				MergeActive: func() (bool, error) { return tt.mergeActive, tt.mergeErr },
				Commit:      func(m string) (bool, error) { commits++; msg = m; return tt.committed, tt.commitErr },
				Spawn:       func() error { spawns++; return tt.spawnErr },
			}, "a-slug")

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("commitRunRecords() = %v; want %v", err, tt.wantErr)
			}
			if commits != tt.wantCommits {
				t.Errorf("commits = %d; want %d", commits, tt.wantCommits)
			}
			if spawns != tt.wantSpawns {
				t.Errorf("spawns = %d; want %d", spawns, tt.wantSpawns)
			}
			if want := "batten: run records for a-slug"; commits > 0 && msg != want {
				t.Errorf("Commit msg = %q; want %q", msg, want)
			}
		})
	}
}
