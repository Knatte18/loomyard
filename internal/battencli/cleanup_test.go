// cleanup_test.go drives cleanup over a synthetic prime location with stubbed records deps;
// it spawns no git and no process.

package battencli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

var cleanupNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// cleanupFixture is one slug's on-disk and committed state.
type cleanupFixture struct {
	slug string
	// runState is the state of the records run directory's status; empty means no records run directory.
	runState string
	// rawStatus, when set, is written as the status file instead of a marshalled runState.
	rawStatus string
	// scratchAge, when positive, creates the scratch directory with every entry modified that long before cleanupNow.
	scratchAge time.Duration
	// committedStatus is the state the records HEAD holds for the slug; empty means none.
	committedStatus string
	// committedSeed reports whether the records HEAD holds the slug's seed.
	committedSeed bool
}

// cleanupStubs records what cleanup asked of its deps.
type cleanupStubs struct {
	committed map[string][]byte
	// committedErr, when set, is what the Committed stub returns.
	committedErr error
	// commitErr, when set, is what the Commit stub fails with, recording no commit.
	commitErr  error
	commits    [][]string
	messages   []string
	spawns     int
	spawnErr   error
	commitHook func()
}

func newCleanupStubs() *cleanupStubs {
	return &cleanupStubs{committed: map[string][]byte{}}
}

func (s *cleanupStubs) deps() cleanupDeps {
	return cleanupDeps{
		Committed: func(relPath string) ([]byte, bool, error) {
			data, found := s.committed[relPath]
			return data, found, s.committedErr
		},
		Commit: func(relPaths []string, msg string) (bool, error) {
			if s.commitHook != nil {
				s.commitHook()
			}
			if s.commitErr != nil {
				return false, s.commitErr
			}
			s.commits = append(s.commits, relPaths)
			s.messages = append(s.messages, msg)
			return true, nil
		},
		Spawn: func() error {
			s.spawns++
			return s.spawnErr
		},
		Now: func() time.Time { return cleanupNow },
	}
}

// buildCleanupFixture lays fixture out under location and registers its committed state with stubs.
func buildCleanupFixture(t *testing.T, location *lymcwd.Location, stubs *cleanupStubs, fixture cleanupFixture) {
	t.Helper()
	slug := fixture.slug

	if fixture.runState != "" || fixture.rawStatus != "" {
		status := []byte(fixture.rawStatus)
		if fixture.rawStatus == "" {
			status = statusJSON(t, fixture.runState)
		}
		writeCleanupFile(t, shedrun.StatusFile(location, slug), status)
		writeCleanupFile(t, shedrun.SeedFile(location, slug), []byte("{}"))
		stubs.committed[shedrun.SeedRel(location, slug)] = []byte("{}")
	}
	if fixture.committedStatus != "" {
		stubs.committed[shedrun.StatusRel(location, slug)] = statusJSON(t, fixture.committedStatus)
	}
	if fixture.committedSeed {
		stubs.committed[shedrun.SeedRel(location, slug)] = []byte("{}")
	}

	if fixture.scratchAge <= 0 {
		return
	}
	scratch := shedrun.ScratchDir(location, slug)
	for _, name := range []string{"run.lock", "status.json.lock", "last-commit", "steps/loop.lock", "steps/record.json", "steps/sub/note.txt"} {
		writeCleanupFile(t, filepath.Join(scratch, filepath.FromSlash(name)), []byte("x"))
	}
	modTime := cleanupNow.Add(-fixture.scratchAge)
	err := filepath.WalkDir(scratch, func(path string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Chtimes(path, modTime, modTime)
	})
	if err != nil {
		t.Fatalf("age scratch: %v", err)
	}
}

func newCleanupLocation(t *testing.T) *lymcwd.Location {
	t.Helper()
	return locationkit.Location(t.TempDir(), "prime", "")
}

// treeListing returns every path under the prime worktree, in walk order.
func treeListing(t *testing.T, location *lymcwd.Location) []string {
	t.Helper()
	var listing []string
	err := filepath.WalkDir(location.WorktreePath(), func(path string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		listing = append(listing, path)
		return nil
	})
	if err != nil {
		t.Fatalf("list tree: %v", err)
	}
	return listing
}

func entryExists(path string) bool {
	_, err := os.Lstat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

func TestCleanup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name         string
		fixtures     []cleanupFixture
		holdRunLock  string
		denyRunsRoot bool
		// legacySelf adds a done run under a directory named "self", which no slug-addressed path reaches.
		legacySelf   bool
		spawnErr     error
		wantRemoved  []string
		wantEmptied  []string
		wantSkipped  map[string]string
		wantCommits  []string
		wantSpawnErr bool
	}{
		{
			name:        "done run is removed, committed and its scratch emptied",
			fixtures:    []cleanupFixture{{slug: "alpha", runState: "done", scratchAge: time.Hour}},
			wantRemoved: []string{"alpha"},
			wantEmptied: []string{"alpha"},
			wantCommits: []string{"alpha"},
		},
		{
			name:        "done status with an unknown key is removed, committed and its scratch emptied",
			fixtures:    []cleanupFixture{{slug: "alpha", rawStatus: `{"current_producer":"Worktree-Teardown","state":"done","history":[],"routed_to":"x"}`, scratchAge: time.Hour}},
			wantRemoved: []string{"alpha"},
			wantEmptied: []string{"alpha"},
			wantCommits: []string{"alpha"},
		},
		{
			name: "runs that are not done are skipped with their state",
			fixtures: []cleanupFixture{
				{slug: "r1", runState: "running", scratchAge: time.Hour},
				{slug: "r2", runState: "paused", scratchAge: time.Hour},
				{slug: "r3", runState: "blocked", scratchAge: time.Hour},
				{slug: "r4", runState: "awaiting", scratchAge: time.Hour},
				{slug: "r5", runState: "failed", scratchAge: time.Hour},
			},
			wantSkipped: map[string]string{
				"r1": `run is running; way forward: "lym batten cancel r1" ends it`,
				"r2": `run is paused; way forward: "lym batten cancel r2" ends it`,
				"r3": `run is blocked; way forward: "lym batten cancel r3" ends it`,
				"r4": `run is awaiting; way forward: "lym batten cancel r4" ends it`,
				"r5": `run is failed; way forward: "lym batten cancel r5" ends it`,
			},
		},
		{
			name:        "undecodable status is skipped with the decode error",
			fixtures:    []cleanupFixture{{slug: "bad", rawStatus: `{"state":`, scratchAge: time.Hour}},
			wantSkipped: map[string]string{"bad": "decode status"},
		},
		{
			name:        "legacy self directory is skipped as reserved",
			legacySelf:  true,
			wantSkipped: map[string]string{shedrun.SelfRunID: "reserved"},
		},
		{
			name:        "held run lock skips the slug",
			fixtures:    []cleanupFixture{{slug: "busy", runState: "done", scratchAge: time.Hour}},
			holdRunLock: "busy",
			wantSkipped: map[string]string{"busy": "run lock held"},
		},
		{
			name:        "tracked-but-deleted done run is committed with nothing left to remove",
			fixtures:    []cleanupFixture{{slug: "gone", committedStatus: "done", scratchAge: time.Hour}},
			wantRemoved: []string{"gone"},
			wantEmptied: []string{"gone"},
			wantCommits: []string{"gone"},
		},
		{
			name:        "tracked-but-deleted run that is not done is skipped and keeps its scratch",
			fixtures:    []cleanupFixture{{slug: "half", committedStatus: "running", scratchAge: 48 * time.Hour}},
			wantSkipped: map[string]string{"half": `committed run is running; way forward: "lym batten cancel half" ends it, and a later cleanup removes it`},
		},
		{
			name:        "deleted run with a committed seed but no committed status is skipped and keeps its aged scratch",
			fixtures:    []cleanupFixture{{slug: "seeded", committedSeed: true, scratchAge: 48 * time.Hour}},
			wantSkipped: map[string]string{"seeded": "tracked run directory with no committed status"},
		},
		{
			name: "aged orphan scratch is emptied and a fresh one kept",
			fixtures: []cleanupFixture{
				{slug: "old", scratchAge: 48 * time.Hour},
				{slug: "new", scratchAge: time.Hour},
			},
			wantEmptied: []string{"old"},
			wantSkipped: map[string]string{"new": "newer than"},
		},
		{
			name:         "run directory that cannot be statted is skipped",
			fixtures:     []cleanupFixture{{slug: "locked", runState: "done", scratchAge: time.Hour}},
			denyRunsRoot: true,
			wantSkipped:  map[string]string{"locked": "stat run directory"},
		},
		{
			name:         "spawn failure is reported with the commit made",
			fixtures:     []cleanupFixture{{slug: "alpha", runState: "done", scratchAge: time.Hour}},
			spawnErr:     errors.New("remote refused"),
			wantRemoved:  []string{"alpha"},
			wantEmptied:  []string{"alpha"},
			wantCommits:  []string{"alpha"},
			wantSpawnErr: true,
		},
	}

	for _, tt := range tests {
		// Windows does not enforce directory permissions, so it generates no subtest for a denying row.
		if tt.denyRunsRoot && runtime.GOOS == "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.denyRunsRoot && os.Geteuid() == 0 {
				t.Skip("directory permissions are not enforced for root")
			}

			location := newCleanupLocation(t)
			stubs := newCleanupStubs()
			stubs.spawnErr = tt.spawnErr
			for _, fixture := range tt.fixtures {
				buildCleanupFixture(t, location, stubs, fixture)
			}
			if tt.legacySelf {
				writeCleanupFile(t, filepath.Join(shedrun.RunsRoot(location), shedrun.SelfRunID, "status.json"), statusJSON(t, "done"))
			}
			if tt.holdRunLock != "" {
				held, err := lock.AcquireWriteLock(shedrun.RunLock(location, tt.holdRunLock))
				if err != nil {
					t.Fatalf("hold run lock: %v", err)
				}
				t.Cleanup(func() { _ = held.Release() })
			}
			if tt.denyRunsRoot {
				root := shedrun.RunsRoot(location)
				if err := os.Chmod(root, 0o444); err != nil {
					t.Fatalf("chmod: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
			}

			report, err := cleanup(location, true, stubs.deps())
			if err != nil {
				t.Fatalf("cleanup: %v", err)
			}

			if !slices.Equal(report.Removed, tt.wantRemoved) {
				t.Errorf("Removed = %v, want %v", report.Removed, tt.wantRemoved)
			}
			if !slices.Equal(report.Emptied, tt.wantEmptied) {
				t.Errorf("Emptied = %v, want %v", report.Emptied, tt.wantEmptied)
			}
			if len(report.Skipped) != len(tt.wantSkipped) {
				t.Errorf("Skipped = %v, want slugs %v", report.Skipped, tt.wantSkipped)
			}
			for _, skipped := range report.Skipped {
				want, ok := tt.wantSkipped[skipped.Slug]
				if !ok || !strings.Contains(skipped.Reason, want) {
					t.Errorf("skip %+v, want reason containing %q", skipped, want)
				}
			}

			if len(stubs.commits) != len(tt.wantCommits) {
				t.Fatalf("commits = %v, want one per %v", stubs.commits, tt.wantCommits)
			}
			for i, slug := range tt.wantCommits {
				if want := []string{shedrun.RunDirRel(location, slug)}; !slices.Equal(stubs.commits[i], want) {
					t.Errorf("commit %d paths = %v, want %v", i, stubs.commits[i], want)
				}
				if !strings.Contains(stubs.messages[i], slug) {
					t.Errorf("commit %d message %q does not name %q", i, stubs.messages[i], slug)
				}
			}
			wantSpawns := 0
			if len(tt.wantCommits) > 0 {
				wantSpawns = 1
			}
			if stubs.spawns != wantSpawns {
				t.Errorf("spawns = %d, want %d", stubs.spawns, wantSpawns)
			}
			if (report.PushError != "") != tt.wantSpawnErr {
				t.Errorf("PushError = %q, want error: %v", report.PushError, tt.wantSpawnErr)
			}
			if wantSpawned := wantSpawns == 1 && !tt.wantSpawnErr; report.PushSpawned != wantSpawned {
				t.Errorf("PushSpawned = %v, want %v", report.PushSpawned, wantSpawned)
			}

			for _, fixture := range tt.fixtures {
				if fixture.scratchAge <= 0 {
					continue
				}
				scratch := shedrun.ScratchDir(location, fixture.slug)
				emptied := slices.Contains(tt.wantEmptied, fixture.slug)
				for _, name := range []string{"run.lock", "status.json.lock", "steps/loop.lock"} {
					if !entryExists(filepath.Join(scratch, filepath.FromSlash(name))) {
						t.Errorf("%s: lock file %s was removed", fixture.slug, name)
					}
				}
				for _, name := range []string{"last-commit", "steps/record.json", "steps/sub"} {
					if gone := !entryExists(filepath.Join(scratch, filepath.FromSlash(name))); gone != emptied {
						t.Errorf("%s: %s removed = %v, want %v", fixture.slug, name, gone, emptied)
					}
				}
				if fixture.runState != "" {
					removed := slices.Contains(tt.wantRemoved, fixture.slug)
					if gone := !entryExists(shedrun.RunDir(location, fixture.slug)); gone != removed {
						t.Errorf("%s: run directory removed = %v, want %v", fixture.slug, gone, removed)
					}
				}
			}
		})
	}
}

func TestRemoveRunRecords_CommitsTheRemovalOnlyWhileTheRecordsHeadTracksTheRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name          string
		onDisk        bool
		trackedRel    func(*lymcwd.Location, string) string
		wantCommitted bool
	}{
		{name: "a tracked run directory on disk is removed and committed", onDisk: true, trackedRel: shedrun.SeedRel, wantCommitted: true},
		{name: "a deleted run directory whose status is tracked commits its removal", trackedRel: shedrun.StatusRel, wantCommitted: true},
		{name: "a run directory already gone and untracked commits nothing and succeeds"},
		{name: "an untracked run directory on disk is removed with no commit", onDisk: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := newCleanupLocation(t)
			stubs := newCleanupStubs()
			const slug = "alpha"
			if tt.onDisk {
				writeCleanupFile(t, shedrun.SeedFile(location, slug), []byte("{}"))
			}
			if tt.trackedRel != nil {
				stubs.committed[tt.trackedRel(location, slug)] = []byte("{}")
			}
			scratchFile := filepath.Join(shedrun.ScratchDir(location, slug), "last-commit")
			writeCleanupFile(t, scratchFile, []byte("x"))

			committed, err := removeRunRecords(location, slug, "remove "+slug, stubs.deps())
			if err != nil {
				t.Fatalf("removeRunRecords() error = %v", err)
			}
			if committed != tt.wantCommitted {
				t.Errorf("removeRunRecords() committed = %v; want %v", committed, tt.wantCommitted)
			}
			wantCommits := 0
			if tt.wantCommitted {
				wantCommits = 1
			}
			if len(stubs.commits) != wantCommits {
				t.Errorf("commits = %v; want %d", stubs.commits, wantCommits)
			}
			if entryExists(shedrun.RunDir(location, slug)) {
				t.Errorf("run directory %s still exists; want it removed", shedrun.RunDir(location, slug))
			}
			if entryExists(scratchFile) {
				t.Errorf("scratch file %s still exists; want the scratch emptied", scratchFile)
			}
		})
	}
}

func TestRemoveRunRecords_ReturnsTheErrorOfAFailedRecordsHeadReadWithoutCommitting(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	location := newCleanupLocation(t)
	stubs := newCleanupStubs()
	stubs.committedErr = errors.New("records HEAD unreadable")
	const slug = "alpha"
	scratchFile := filepath.Join(shedrun.ScratchDir(location, slug), "last-commit")
	writeCleanupFile(t, scratchFile, []byte("x"))

	committed, err := removeRunRecords(location, slug, "remove "+slug, stubs.deps())
	if !errors.Is(err, stubs.committedErr) {
		t.Errorf("removeRunRecords() error = %v; want it to wrap %v", err, stubs.committedErr)
	}
	if committed || len(stubs.commits) != 0 {
		t.Errorf("removeRunRecords() committed = %v, commits = %v; want no commit", committed, stubs.commits)
	}
	if !entryExists(scratchFile) {
		t.Errorf("scratch file %s was removed; want the scratch left in place", scratchFile)
	}
}

func TestCleanup_DryRunListsExactlyWhatApplyRemovesAndChangesNothing(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	fixtures := []cleanupFixture{
		{slug: "done-one", runState: "done"},
		{slug: "done-two", runState: "done", scratchAge: time.Hour},
		{slug: "gone", committedStatus: "done", scratchAge: time.Hour},
		{slug: "old-orphan", scratchAge: 48 * time.Hour},
		{slug: "fresh-orphan", scratchAge: time.Hour},
		{slug: "running", runState: "running", scratchAge: time.Hour},
	}
	run := func(apply bool) (CleanupReport, *cleanupStubs, []string, []string) {
		location := newCleanupLocation(t)
		stubs := newCleanupStubs()
		for _, fixture := range fixtures {
			buildCleanupFixture(t, location, stubs, fixture)
		}
		before := treeListing(t, location)
		report, err := cleanup(location, apply, stubs.deps())
		if err != nil {
			t.Fatalf("cleanup(apply=%v): %v", apply, err)
		}
		return report, stubs, before, treeListing(t, location)
	}

	dryReport, dryStubs, dryBefore, dryAfter := run(false)
	applyReport, _, _, _ := run(true)

	if dryReport.Applied || !applyReport.Applied {
		t.Errorf("Applied = %v for the dry run and %v for apply", dryReport.Applied, applyReport.Applied)
	}
	if !slices.Equal(dryBefore, dryAfter) {
		t.Errorf("a dry run changed the tree:\nbefore %v\nafter  %v", dryBefore, dryAfter)
	}
	if len(dryStubs.commits) != 0 || dryStubs.spawns != 0 {
		t.Errorf("a dry run committed %v and spawned %d pushes", dryStubs.commits, dryStubs.spawns)
	}
	if want := []string{"done-one", "done-two", "gone"}; !slices.Equal(applyReport.Removed, want) {
		t.Errorf("apply Removed = %v, want %v", applyReport.Removed, want)
	}
	if want := []string{"done-two", "gone", "old-orphan"}; !slices.Equal(applyReport.Emptied, want) {
		t.Errorf("apply Emptied = %v, want %v", applyReport.Emptied, want)
	}
	if !slices.Equal(dryReport.Removed, applyReport.Removed) || !slices.Equal(dryReport.Emptied, applyReport.Emptied) {
		t.Errorf("dry run lists removed %v, emptied %v; apply did removed %v, emptied %v", dryReport.Removed, dryReport.Emptied, applyReport.Removed, applyReport.Emptied)
	}
	if !slices.Equal(dryReport.Skipped, applyReport.Skipped) {
		t.Errorf("dry run skipped %v; apply skipped %v", dryReport.Skipped, applyReport.Skipped)
	}
}

func TestCleanup_HoldsTheStatusLockUntilTheCommitAndLeavesTheSlugRunnable(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	location := newCleanupLocation(t)
	stubs := newCleanupStubs()
	slug := "alpha"
	buildCleanupFixture(t, location, stubs, cleanupFixture{slug: slug, runState: "done", scratchAge: time.Hour})

	statusLockPath := shedrun.StatusLock(location, slug)
	before, err := os.Stat(statusLockPath)
	if err != nil {
		t.Fatalf("stat status lock: %v", err)
	}
	stubs.commitHook = func() {
		other, acquired, err := lock.TryAcquireWriteLock(statusLockPath)
		if err != nil {
			t.Errorf("probe status lock: %v", err)
		}
		if acquired {
			_ = other.Release()
			t.Errorf("status lock was free while the removal was being committed")
		}
	}

	if _, err := cleanup(location, true, stubs.deps()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	after, err := os.Stat(statusLockPath)
	if err != nil {
		t.Fatalf("stat status lock after: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Errorf("the status lock file was replaced")
	}

	armed := &battenCLI{
		location: location,
		slug:     slug,
		shedPaths: shedbuild.ShedPaths{
			StatusPath:     shedrun.StatusFile(location, slug),
			StatusLockPath: statusLockPath,
		},
	}
	found, err := armed.checkRunStatus()
	if err != nil || found {
		t.Errorf("checkRunStatus = (%v, %v), want (false, nil) so the slug can run again", found, err)
	}
}

func TestClearMotifRun_RemovesOnlyADoneMotifOrArrasRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// runState is the state of the slug's status; empty means no status file.
		runState string
		// childRecipe is the child-recipe record; empty means none.
		childRecipe string
		// filingMark writes the run's filing mark, which a removal takes with the run.
		filingMark  bool
		holdRunLock bool
		// commitErr is what the records commit fails with; the run then stays and the error is returned.
		commitErr error
		// spawnErr is what the push spawn fails with; the run is still removed.
		spawnErr error
		// blockScratch makes a directory under the run's scratch unreadable, so emptying the scratch fails after the commit.
		blockScratch bool
		// untracked leaves the run out of the records HEAD, so its removal makes no commit.
		untracked   bool
		wantCleared bool
	}{
		{name: "DoneMotifRunIsRemovedCommittedAndPushed", runState: "done", childRecipe: shedrun.RecipeMotif, wantCleared: true},
		{name: "RecordsCommitFailureReturnsTheErrorAndKeepsTheRun", runState: "done", childRecipe: shedrun.RecipeMotif, commitErr: errors.New("records commit refused")},
		{name: "PushSpawnFailureStillClearsTheRun", runState: "done", childRecipe: shedrun.RecipeMotif, spawnErr: errors.New("remote refused"), wantCleared: true},
		{name: "ScratchEmptyFailureStillClearsTheRun", runState: "done", childRecipe: shedrun.RecipeMotif, blockScratch: true, wantCleared: true},
		{name: "DoneMotifRunTheRecordsHeadDoesNotTrackIsRemovedWithNoCommit", runState: "done", childRecipe: shedrun.RecipeMotif, untracked: true, wantCleared: true},
		{name: "DoneArrasRunIsRemovedWithItsFilingMark", runState: "done", childRecipe: shedrun.RecipeArras, filingMark: true, wantCleared: true},
		{name: "DoneDarnRunStays", runState: "done", childRecipe: shedrun.RecipeDarn},
		{name: "DoneRunWithNoChildRecipeRecordStays", runState: "done"},
		{name: "RunningMotifRunStays", runState: "running", childRecipe: shedrun.RecipeMotif},
		{name: "SlugWithNoStatusStays"},
		{name: "DoneMotifRunWhoseRunLockIsHeldStays", runState: "done", childRecipe: shedrun.RecipeMotif, holdRunLock: true},
	}
	for _, tt := range tests {
		// Windows does not enforce directory permissions, so it generates no subtest for a blocking row.
		if tt.blockScratch && runtime.GOOS == "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.blockScratch && os.Geteuid() == 0 {
				t.Skip("directory permissions are not enforced for root")
			}
			location := newCleanupLocation(t)
			stubs := newCleanupStubs()
			stubs.commitErr = tt.commitErr
			stubs.spawnErr = tt.spawnErr
			const slug = "codex-engine"
			if tt.runState != "" {
				writeCleanupFile(t, shedrun.StatusFile(location, slug), statusJSON(t, tt.runState))
				if !tt.untracked {
					stubs.committed[shedrun.StatusRel(location, slug)] = statusJSON(t, tt.runState)
				}
			}
			if tt.blockScratch {
				blocked := filepath.Join(shedrun.ScratchDir(location, slug), "steps")
				writeCleanupFile(t, filepath.Join(blocked, "record.json"), []byte("x"))
				if err := os.Chmod(blocked, 0o000); err != nil {
					t.Fatalf("chmod: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
			}
			if tt.childRecipe != "" {
				if err := shedrun.WriteChildRecipe(location, slug, tt.childRecipe); err != nil {
					t.Fatalf("write child recipe: %v", err)
				}
			}
			if tt.filingMark {
				if err := shedrun.WriteFilingMark(location, slug); err != nil {
					t.Fatalf("write filing mark: %v", err)
				}
			}
			if tt.holdRunLock {
				writeCleanupFile(t, shedrun.RunLock(location, slug), nil)
				held, err := lock.AcquireWriteLock(shedrun.RunLock(location, slug))
				if err != nil {
					t.Fatalf("hold run lock: %v", err)
				}
				t.Cleanup(func() { _ = held.Release() })
			}

			cleared, err := clearMotifRun(location, slug, stubs.deps())
			if !errors.Is(err, tt.commitErr) {
				t.Fatalf("clearMotifRun() error = %v; want it to wrap %v", err, tt.commitErr)
			}
			if cleared != tt.wantCleared {
				t.Errorf("clearMotifRun() cleared = %v; want %v", cleared, tt.wantCleared)
			}
			wantCommits, wantSpawns := 0, 0
			if tt.wantCleared && !tt.untracked {
				wantCommits, wantSpawns = 1, 1
			}
			if len(stubs.commits) != wantCommits {
				t.Fatalf("commits = %v; want %d", stubs.commits, wantCommits)
			}
			if wantCommits == 1 && !slices.Equal(stubs.commits[0], []string{shedrun.RunDirRel(location, slug)}) {
				t.Errorf("commit paths = %v; want the run directory %q", stubs.commits[0], shedrun.RunDirRel(location, slug))
			}
			if stubs.spawns != wantSpawns {
				t.Errorf("spawns = %d; want %d", stubs.spawns, wantSpawns)
			}
			if tt.runState != "" {
				if gone := !entryExists(shedrun.RunDir(location, slug)); gone != tt.wantCleared {
					t.Errorf("run directory removed = %v; want %v", gone, tt.wantCleared)
				}
			}
		})
	}
}

func TestClearMotifRun_FinishesTheRemovalOfARunWhoseRecordsCommitFailed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	location := newCleanupLocation(t)
	stubs := newCleanupStubs()
	const slug = "codex-engine"
	writeCleanupFile(t, shedrun.StatusFile(location, slug), statusJSON(t, "done"))
	stubs.committed[shedrun.StatusRel(location, slug)] = statusJSON(t, "done")
	if err := shedrun.WriteChildRecipe(location, slug, shedrun.RecipeArras); err != nil {
		t.Fatalf("write child recipe: %v", err)
	}
	if err := shedrun.WriteFilingMark(location, slug); err != nil {
		t.Fatalf("write filing mark: %v", err)
	}

	stubs.commitErr = errors.New("records commit refused")
	cleared, err := clearMotifRun(location, slug, stubs.deps())
	if !errors.Is(err, stubs.commitErr) || cleared {
		t.Fatalf("clearMotifRun() with a failing commit = (%v, %v); want false and an error wrapping %v", cleared, err, stubs.commitErr)
	}
	if marked, err := shedrun.FilingMarked(location, slug); err != nil || !marked {
		t.Errorf("FilingMarked() after the failed removal = %t, %v; want the mark written back", marked, err)
	}

	stubs.commitErr = nil
	cleared, err = clearMotifRun(location, slug, stubs.deps())
	if err != nil || !cleared {
		t.Fatalf("clearMotifRun() on the re-run = (%v, %v); want true and nil", cleared, err)
	}
	if len(stubs.commits) != 1 {
		t.Errorf("commits = %v; want the re-run's one removal commit", stubs.commits)
	}
	if entryExists(shedrun.RunDir(location, slug)) {
		t.Errorf("run directory %s still exists after the re-run; want it removed", shedrun.RunDir(location, slug))
	}
}
