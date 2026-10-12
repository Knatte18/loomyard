// cleanup_test.go drives cleanup over a synthetic prime location with stubbed records deps; it spawns no git and no process.

package battencli

import (
	"encoding/json"
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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
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
	committed  map[string][]byte
	commits    [][]string
	messages   []string
	pushes     int
	pushErr    error
	commitHook func()
}

func newCleanupStubs() *cleanupStubs {
	return &cleanupStubs{committed: map[string][]byte{}}
}

func (s *cleanupStubs) deps() cleanupDeps {
	return cleanupDeps{
		Committed: func(relPath string) ([]byte, bool, error) {
			data, found := s.committed[relPath]
			return data, found, nil
		},
		Commit: func(relPaths []string, msg string) (bool, error) {
			if s.commitHook != nil {
				s.commitHook()
			}
			s.commits = append(s.commits, relPaths)
			s.messages = append(s.messages, msg)
			return true, nil
		},
		Push: func() error {
			s.pushes++
			return s.pushErr
		},
		Now: func() time.Time { return cleanupNow },
	}
}

func statusJSON(t *testing.T, state string) []byte {
	t.Helper()
	data, err := json.Marshal(shedengine.Status{CurrentProducer: "Worktree-Teardown", State: shedengine.State(state), History: []shedengine.HistoryEntry{}})
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	return data
}

func writeCleanupFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// buildCleanupFixture lays fixture out under location and registers its committed state with stubs.
func buildCleanupFixture(t *testing.T, location *lyxcwd.Location, stubs *cleanupStubs, fixture cleanupFixture) {
	t.Helper()
	slug := fixture.slug

	if fixture.runState != "" || fixture.rawStatus != "" {
		status := []byte(fixture.rawStatus)
		if fixture.rawStatus == "" {
			status = statusJSON(t, fixture.runState)
		}
		writeCleanupFile(t, shedrun.StatusFile(location, slug), status)
		writeCleanupFile(t, shedrun.SeedFile(location, slug), []byte("{}"))
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

func newCleanupLocation(t *testing.T) *lyxcwd.Location {
	t.Helper()
	return locationkit.Location(t.TempDir(), "prime", "")
}

// treeListing returns every path under the prime worktree, in walk order.
func treeListing(t *testing.T, location *lyxcwd.Location) []string {
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
	t.Parallel()

	tests := []struct {
		name         string
		fixtures     []cleanupFixture
		holdRunLock  string
		denyRunsRoot bool
		// legacySelf adds a done run under a directory literally named "self", which no slug-addressed path reaches.
		legacySelf  bool
		pushErr     error
		wantRemoved []string
		wantEmptied []string
		wantSkipped map[string]string
		wantCommits []string
		wantPushErr bool
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
			wantSkipped: map[string]string{"r1": "run is running", "r2": "run is paused", "r3": "run is blocked", "r4": "run is awaiting", "r5": "run is failed"},
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
			wantSkipped: map[string]string{"half": "committed run is running"},
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
			name:        "push failure is reported with the commit made",
			fixtures:    []cleanupFixture{{slug: "alpha", runState: "done", scratchAge: time.Hour}},
			pushErr:     errors.New("remote refused"),
			wantRemoved: []string{"alpha"},
			wantEmptied: []string{"alpha"},
			wantCommits: []string{"alpha"},
			wantPushErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.denyRunsRoot && (runtime.GOOS == "windows" || os.Geteuid() == 0) {
				t.Skip("directory permissions are not enforced here")
			}

			location := newCleanupLocation(t)
			stubs := newCleanupStubs()
			stubs.pushErr = tt.pushErr
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
			wantPushes := 0
			if len(tt.wantCommits) > 0 {
				wantPushes = 1
			}
			if stubs.pushes != wantPushes {
				t.Errorf("pushes = %d, want %d", stubs.pushes, wantPushes)
			}
			if (report.PushError != "") != tt.wantPushErr {
				t.Errorf("PushError = %q, want error: %v", report.PushError, tt.wantPushErr)
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

func TestCleanup_DryRunListsExactlyWhatApplyRemovesAndChangesNothing(t *testing.T) {
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
	if len(dryStubs.commits) != 0 || dryStubs.pushes != 0 {
		t.Errorf("a dry run committed %v and pushed %d times", dryStubs.commits, dryStubs.pushes)
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
