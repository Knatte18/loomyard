// commitstatus_test.go pins batten's status commits: the per-transition seam, and the run's commit paths and records.
// The seam tests use stubbed commitStatusDeps closures and a temp-file marker; every test here runs in tier0.
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
//   - a repeated (producer, state) pair commits and pushes once, not twice, across two calls to the same seam;
//   - the skip still holds when the seam is rebuilt between calls, as a fresh "lym batten step" process does;
//     it is the one case that proves the on-disk marker rather than an in-closure variable;
//   - a changed pair commits and pushes again rather than being absorbed by the skip;
//   - a missing or corrupt marker falls back to committing once rather than erroring;
//     the marker is a cache, so losing it costs one redundant commit, never correctness;
//   - an uninterrupted start commits each row from Seed-Child to Run-Shed without a push,
//     while Worktree-Create, a resumed or halted start row and every later row push;
//   - a start row rewrites the marker, so a repeat of it skips, and a start row entered from Run-Shed or later pushes;
//   - a push that takes no gate admission is a start row's, so admissions equal pushes.
//
// The commit message pins batten's own prefix, since the shared core renders whatever prefix its caller gives.
func TestNewCommitStatusSeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	type transition struct{ producer, state string }

	tests := []struct {
		name string
		// markerRel is the marker path under the test's temp dir.
		markerRel     string
		corruptMarker bool
		// rebuildSeam makes every call after the first use a fresh seam instance over the same marker.
		rebuildSeam bool
		calls       []transition
		wantCommits int
		wantPushes  int
	}{
		{name: "RepeatedPairCommitsOnce", markerRel: "last-commit", calls: []transition{{"Run-Shed", "running"}, {"Run-Shed", "running"}}, wantCommits: 1, wantPushes: 1},
		{name: "SkipSurvivesARebuiltSeam", markerRel: "last-commit", rebuildSeam: true, calls: []transition{{"Run-Shed", "running"}, {"Run-Shed", "running"}}, wantCommits: 1, wantPushes: 1},
		{name: "ChangedPairCommitsAgain", markerRel: "last-commit", calls: []transition{{"Run-Shed", "running"}, {"Run-Shed", "paused"}}, wantCommits: 2, wantPushes: 2},
		{name: "MissingMarkerCommitsOnce", markerRel: filepath.Join("does-not-exist", "last-commit"), calls: []transition{{"Run-Shed", "running"}}, wantCommits: 1, wantPushes: 1},
		{name: "CorruptMarkerCommitsOnce", markerRel: "last-commit", corruptMarker: true, calls: []transition{{"Run-Shed", "running"}}, wantCommits: 1, wantPushes: 1},
		{name: "StartRowsCommitWithoutPush", markerRel: "last-commit", rebuildSeam: true, calls: []transition{
			{"Worktree-Create", "running"}, {"Seed-Child", "running"}, {"Open-Window", "running"}, {"Seed-IDE", "running"},
			{"Open-IDE", "running"}, {"Run-Shed", "running"}, {"Land", "running"}, {"Worktree-Teardown", "running"},
		}, wantCommits: 8, wantPushes: 3},
		{name: "SkippedGatedRowsCommitWithoutPush", markerRel: "last-commit", calls: []transition{{"Worktree-Create", "running"}, {"Seed-Child", "running"}, {"Run-Shed", "running"}}, wantCommits: 3, wantPushes: 1},
		{name: "ResumedStartRowPushes", markerRel: "last-commit", calls: []transition{{"Seed-Child", "paused"}, {"Seed-Child", "running"}, {"Open-Window", "running"}}, wantCommits: 3, wantPushes: 2},
		{name: "HaltedStartRowPushes", markerRel: "last-commit", calls: []transition{{"Worktree-Create", "running"}, {"Seed-Child", "running"}, {"Seed-Child", "failed"}}, wantCommits: 3, wantPushes: 2},
		{name: "StartRowWritesTheMarkerSoARepeatSkips", markerRel: "last-commit", calls: []transition{{"Worktree-Create", "running"}, {"Seed-Child", "running"}, {"Seed-Child", "running"}}, wantCommits: 2, wantPushes: 1},
		{name: "WorktreeCreateAfterAStartRowPushes", markerRel: "last-commit", calls: []transition{{"Seed-Child", "running"}, {"Worktree-Create", "running"}}, wantCommits: 2, wantPushes: 2},
		{name: "StartRowAfterRunShedRunningPushes", markerRel: "last-commit", calls: []transition{{"Run-Shed", "running"}, {"Seed-Child", "running"}}, wantCommits: 2, wantPushes: 2}}
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

			var commits, pushes, admits int
			var msg string
			newSeam := func() func(producer, state string) error {
				return newCommitStatusSeam(commitStatusDeps{
					MergeActive: func() (bool, error) { return false, nil },
					Commit:      func(m string) (bool, error) { commits++; msg = m; return true, nil },
					Push:        func() error { pushes++; return nil },
					Admit:       func() func() { admits++; return func() {} },
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
			if pushes != tt.wantPushes {
				t.Errorf("pushes = %d; want exactly %d", pushes, tt.wantPushes)
			}
			// A start row makes no network call, so it takes no gate admission: one admission per pushing transition.
			if admits != tt.wantPushes {
				t.Errorf("gate admissions = %d; want exactly %d, one per push", admits, tt.wantPushes)
			}
			last := tt.calls[len(tt.calls)-1]
			if want := "batten: " + last.producer + " -> " + last.state; msg != want {
				t.Errorf("Commit msg = %q; want %q", msg, want)
			}
		})
	}
}

// TestNewCommitStatusSeam_RetriesAFailedStartChainPush pins that a push that failed leaves the next start row pushing.
// The first push fails with an error that is not owed and halts the run at Worktree-Create;
// the resumed Seed-Child row pushes and lands, and the row after it commits without a push again.
func TestNewCommitStatusSeam_RetriesAFailedStartChainPush(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	markerPath := filepath.Join(t.TempDir(), "last-commit")
	var commits, pushes int
	seam := newCommitStatusSeam(commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(string) (bool, error) { commits++; return true, nil },
		Push: func() error {
			pushes++
			if pushes == 1 {
				return errors.New("push rejected")
			}
			return nil
		},
	}, markerPath, markerPath+".lock")

	if err := seam("Worktree-Create", "running"); err == nil {
		t.Fatal("seam(Worktree-Create, running) = nil; want the push failure")
	}
	for _, producer := range []string{"Seed-Child", "Open-Window"} {
		if err := seam(producer, "running"); err != nil {
			t.Fatalf("seam(%q, running) = %v; want nil", producer, err)
		}
	}

	if commits != 3 || pushes != 2 {
		t.Errorf("commits = %d, pushes = %d; want 3 and 2", commits, pushes)
	}
}

// TestNewCommitStatusSeam_WritesBoardStatusBeforeRunShed pins the seam's board write.
// A transition at a row before Run-Shed writes "<FSM state> · <row>" on the entry:
// a running transition reads starting, a blocked one halted and a paused one paused.
// A transition at Run-Shed or a later row writes nothing: the inner run writes the entry's status from its start on.
//
//testtiming:keep pins the board status text each producer and state maps to, and that Run-Shed and later rows write none
func TestNewCommitStatusSeam_WritesBoardStatusBeforeRunShed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	markerPath := filepath.Join(t.TempDir(), "last-commit")
	var written []string
	seam := newCommitStatusSeam(commitStatusDeps{
		MergeActive:    func() (bool, error) { return false, nil },
		Commit:         func(string) (bool, error) { return true, nil },
		Push:           func() error { return nil },
		SetBoardStatus: func(status string) error { written = append(written, status); return nil },
	}, markerPath, markerPath+".lock")

	for _, call := range []struct{ producer, state string }{
		{"Worktree-Create", "running"},
		{"Seed-Child", "blocked"},
		{"Open-IDE", "running"},
		{"Open-IDE", "paused"},
		{"Run-Shed", "running"},
		{"Land", "running"},
		{"Worktree-Teardown", "done"},
	} {
		if err := seam(call.producer, call.state); err != nil {
			t.Fatalf("seam(%q, %q) = %v; want nil", call.producer, call.state, err)
		}
	}

	want := []string{"starting · Worktree-Create", "halted · Seed-Child", "starting · Open-IDE", "paused · Open-IDE"}
	if !reflect.DeepEqual(written, want) {
		t.Errorf("board statuses written = %q; want %q", written, want)
	}
}

// TestNewCommitStatusSeam_BoardWriteFailureDoesNotStopTheCommit pins that a failing board write is only warned about.
// The write runs at a row before Run-Shed; the seam returns nil and still commits and pushes.
func TestNewCommitStatusSeam_BoardWriteFailureDoesNotStopTheCommit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	markerPath := filepath.Join(t.TempDir(), "last-commit")
	var commits, pushes int
	seam := newCommitStatusSeam(commitStatusDeps{
		MergeActive:    func() (bool, error) { return false, nil },
		Commit:         func(string) (bool, error) { commits++; return true, nil },
		Push:           func() error { pushes++; return nil },
		SetBoardStatus: func(string) error { return errors.New("board unreadable") },
	}, markerPath, markerPath+".lock")

	if err := seam("Worktree-Create", "running"); err != nil {
		t.Fatalf("seam(Worktree-Create, running) = %v; want nil", err)
	}

	if commits != 1 || pushes != 1 {
		t.Errorf("commits = %d, pushes = %d; want 1 and 1", commits, pushes)
	}
}

// TestBattenCommitStatusDeps_SetBoardStatusWritesTheRunsEntry pins that the real SetBoardStatus writes the status.
// The status lands on the board entry whose slug is the run id, and no status does once the run's filing mark stands.
// The test sets the board skip variables, which are process-global, so it does not call t.Parallel.
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

	if err := shedrun.WriteFilingMark(location, slug); err != nil {
		t.Fatalf("WriteFilingMark() = %v; want nil", err)
	}
	if err := battenCommitStatusDeps(location, slug).SetBoardStatus(boardengine.RunStatus("running", "Preflight")); err != nil {
		t.Fatalf("SetBoardStatus() under the filing mark = %v; want nil", err)
	}
	if task, _, err := board.GetTask(slug); err != nil || task.Status == nil || *task.Status != status {
		t.Errorf("board status under the filing mark = %v, %v; want %q left as it was", task.Status, err, status)
	}
}

// TestStatusPushSpawns_TellTheirLocationAndGateSlug pins what batten's cleanup push spawn asks fabric for:
// cleanup spawns for its location with no gate slug.
// It swaps the package's spawnStatusPush for a recorder, so it is serial:
// no parallel test may run while the variable is swapped.
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

	if err := fabricCleanupDeps(location).Spawn(); err != nil {
		t.Fatalf("cleanup Spawn() = %v; want nil", err)
	}
	if want := []spawnRequest{{location, ""}}; !reflect.DeepEqual(requests, want) {
		t.Errorf("cleanup requests = %v; want %v", requests, want)
	}
}

// TestBattenRunCommitPaths asserts a commit of the run's records stages each of these once it exists:
// the run's seed; dobby's run notice marker; the settle records; the child-recipe record;
// and each driver directory holding a file alongside its status.
// The status alone is committed while none of them exists.
// A run directory is durable, fabric-synced state:
// a status committed without its seed leaves a resumed machine knowing how far the run came but not what it is running.
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

	if err := shedrun.WriteFilingMark(loc, runID); err != nil {
		t.Fatalf("write filing mark: %v", err)
	}
	got = battenRunCommitPaths(loc, runID)
	if want = append(want, shedrun.FilingMarkRel(loc, runID)); !reflect.DeepEqual(got, want) {
		t.Errorf("battenRunCommitPaths with a filing mark on disk = %v; want %v", got, want)
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
// it commits and pushes; pushes nothing when nothing was committed;
// commits nothing while the pair is mid-merge; returns a failed commit, merge-state read or push;
// and never returns a push that is owed.
func TestCommitRunRecords(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	boom := errors.New("boom")
	owed := &fabricengine.PushFailedError{Record: fabricengine.FailedPush{Class: fabricengine.PushFailureTransport}, Err: boom}
	tests := []struct {
		name        string
		mergeActive bool
		mergeErr    error
		committed   bool
		commitErr   error
		pushErr     error
		wantCommits int
		wantPushes  int
		wantErr     error
	}{
		{name: "CommitsAndPushes", committed: true, wantCommits: 1, wantPushes: 1},
		{name: "NothingCommittedPushesNothing", wantCommits: 1},
		{name: "MidMergeCommitsNothing", mergeActive: true},
		{name: "FailedMergeStateReadIsReturned", mergeErr: boom, wantErr: boom},
		{name: "FailedCommitIsReturned", commitErr: boom, wantCommits: 1, wantErr: boom},
		{name: "OwedPushIsNotReturned", committed: true, pushErr: owed, wantCommits: 1, wantPushes: 1},
		{name: "FailedPushIsReturned", committed: true, pushErr: boom, wantCommits: 1, wantPushes: 1, wantErr: boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var commits, pushes int
			var msg string
			err := commitRunRecords(commitStatusDeps{
				MergeActive: func() (bool, error) { return tt.mergeActive, tt.mergeErr },
				Commit:      func(m string) (bool, error) { commits++; msg = m; return tt.committed, tt.commitErr },
				Push:        func() error { pushes++; return tt.pushErr },
			}, "a-slug")

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("commitRunRecords() = %v; want %v", err, tt.wantErr)
			}
			if commits != tt.wantCommits {
				t.Errorf("commits = %d; want %d", commits, tt.wantCommits)
			}
			if pushes != tt.wantPushes {
				t.Errorf("pushes = %d; want %d", pushes, tt.wantPushes)
			}
			if want := "batten: run records for a-slug"; commits > 0 && msg != want {
				t.Errorf("Commit msg = %q; want %q", msg, want)
			}
		})
	}
}
