// retention_test.go exercises Sweep's trace grouping, activity ranking, age bound, count bound, grammar-scoping, and delete-failure tolerance over a t.TempDir() — pure filesystem logic, no git/exec spawns, per the Test Tier Purity Invariant.

package logger

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const deadTestPID = 999999999

func traceTestFileName(ts time.Time, id string, pid int) string {
	return fmt.Sprintf("trace-%s-%s-%d.log", ts.UTC().Format(traceFileTimestampLayout), id, pid)
}

// writeTraceTestFile writes a trace file named for ts and sets its mtime to ts, so ranking by activity is deterministic.
func writeTraceTestFile(t *testing.T, dir string, ts time.Time, id string, pid int) string {
	t.Helper()
	path := filepath.Join(dir, traceTestFileName(ts, id, pid))
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("write test trace file %s: %v", path, err)
	}
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatalf("Chtimes(%s) = %v", path, err)
	}
	return path
}

func setMtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("Chtimes(%s) = %v", path, err)
	}
}

func hexID(n int) string {
	return fmt.Sprintf("%016x", n)
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to exist, got: %v", path, err)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted, stat err = %v", path, err)
	}
}

var testBounds = RetentionBounds{Count: 3, MaxAge: 14 * 24 * time.Hour}

// TestDefaultRetentionBounds pins the compiled-in defaults.
func TestDefaultRetentionBounds(t *testing.T) {
	t.Parallel()
	got := DefaultRetentionBounds()
	want := RetentionBounds{Count: 200, MaxAge: 14 * 24 * time.Hour}
	if got != want {
		t.Errorf("DefaultRetentionBounds() = %+v; want %+v", got, want)
	}
}

// TestSweep_AgeBound verifies groups with no recent activity are deleted.
func TestSweep_AgeBound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Now()

	oldFile := writeTraceTestFile(t, dir, now.Add(-15*24*time.Hour), hexID(1), deadTestPID)
	recentFile := writeTraceTestFile(t, dir, now.Add(-1*time.Hour), hexID(2), deadTestPID)

	if err := Sweep(dir, testBounds); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertAbsent(t, oldFile)
	assertExists(t, recentFile)
}

// TestSweep_CountBoundKeepsNewestGroupsWhole verifies the newest Count groups by activity survive with every file,
// and every file of each older group is deleted.
func TestSweep_CountBoundKeepsNewestGroupsWhole(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := time.Now().Add(-time.Minute)

	const groups = 5
	files := make([][]string, groups)
	for i := 0; i < groups; i++ {
		ts := base.Add(-time.Duration(i) * time.Hour)
		files[i] = append(files[i], writeTraceTestFile(t, dir, ts, hexID(i), deadTestPID))
		// A second file in the group, one pid apart, starting a little later.
		files[i] = append(files[i], writeTraceTestFile(t, dir, ts.Add(time.Second), hexID(i), deadTestPID+1))
	}

	if err := Sweep(dir, testBounds); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	for i, paths := range files {
		for _, path := range paths {
			if i < testBounds.Count {
				assertExists(t, path)
			} else {
				assertAbsent(t, path)
			}
		}
	}
}

// TestSweep_LiveGroupKeptRegardlessOfAgeAndBudget verifies a group holding a live-pid file keeps all its files past the age bound and does not consume count budget.
func TestSweep_LiveGroupKeptRegardlessOfAgeAndBudget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Now()

	liveOld := writeTraceTestFile(t, dir, now.Add(-30*24*time.Hour), hexID(0), os.Getpid())
	liveSibling := writeTraceTestFile(t, dir, now.Add(-30*24*time.Hour+time.Second), hexID(0), deadTestPID)

	var deadFiles []string
	for i := 0; i < testBounds.Count; i++ {
		ts := now.Add(-time.Duration(i+1) * time.Hour)
		deadFiles = append(deadFiles, writeTraceTestFile(t, dir, ts, hexID(i+1), deadTestPID))
	}

	if err := Sweep(dir, testBounds); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertExists(t, liveOld)
	assertExists(t, liveSibling)
	for _, path := range deadFiles {
		assertExists(t, path)
	}
}

// TestSweep_LongRunningStepSurvivesByMtime verifies a group whose filename timestamp is the oldest but whose mtime is the newest survives a count-bound sweep that deletes groups with later filename timestamps and older mtimes.
func TestSweep_LongRunningStepSurvivesByMtime(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Now()

	long := writeTraceTestFile(t, dir, now.Add(-10*24*time.Hour), hexID(0), deadTestPID)
	setMtime(t, long, now.Add(-time.Second))

	var others []string
	for i := 0; i < testBounds.Count+1; i++ {
		ts := now.Add(-time.Duration(i+1) * time.Hour)
		others = append(others, writeTraceTestFile(t, dir, ts, hexID(i+1), deadTestPID))
	}

	if err := Sweep(dir, testBounds); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertExists(t, long)
	for i, path := range others {
		// The long-running group takes one of the Count slots, so one fewer of the others survives.
		if i < testBounds.Count-1 {
			assertExists(t, path)
		} else {
			assertAbsent(t, path)
		}
	}
}

// TestSweep_AgeBoundOverridesCountBudgetButNotLiveness verifies a non-live group past MaxAge is deleted though Count would keep it,
// and a live group past MaxAge is kept.
func TestSweep_AgeBoundOverridesCountBudgetButNotLiveness(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	old := time.Now().Add(-20 * 24 * time.Hour)

	stale := writeTraceTestFile(t, dir, old, hexID(1), deadTestPID)
	live := writeTraceTestFile(t, dir, old, hexID(2), os.Getpid())

	bounds := RetentionBounds{Count: 100, MaxAge: 14 * 24 * time.Hour}
	if err := Sweep(dir, bounds); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertAbsent(t, stale)
	assertExists(t, live)
}

// TestSweep_StatFailureFallsBackToFilenameTimestamp verifies a file whose stat fails ranks by its filename timestamp,
// and the sweep still returns nil and processes the rest.
func TestSweep_StatFailureFallsBackToFilenameTimestamp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Now()

	// Old by filename, but with a fresh mtime; the stat failure hides the mtime.
	failing := writeTraceTestFile(t, dir, now.Add(-20*24*time.Hour), hexID(1), deadTestPID)
	setMtime(t, failing, now)
	recent := writeTraceTestFile(t, dir, now.Add(-time.Hour), hexID(2), deadTestPID)
	stale := writeTraceTestFile(t, dir, now.Add(-21*24*time.Hour), hexID(3), deadTestPID)

	stat := func(path string) (fs.FileInfo, error) {
		if path == failing {
			return nil, errors.New("stat failed")
		}
		return os.Stat(path)
	}
	if err := sweep(dir, testBounds, now, stat); err != nil {
		t.Fatalf("sweep(%s) = %v; want nil", dir, err)
	}

	assertAbsent(t, failing)
	assertAbsent(t, stale)
	assertExists(t, recent)
}

// TestSweep_GrammarScope verifies non-matching files and subdirectories are never deleted.
func TestSweep_GrammarScope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	foreign := filepath.Join(dir, "tmux-server-1234.log")
	if err := os.WriteFile(foreign, []byte("test"), 0o644); err != nil {
		t.Fatalf("write foreign file: %v", err)
	}
	setMtime(t, foreign, time.Now().Add(-100*24*time.Hour))

	subdir := filepath.Join(dir, traceTestFileName(time.Now().Add(-100*24*time.Hour), hexID(9), deadTestPID))
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", subdir, err)
	}

	base := time.Now().Add(-time.Minute)
	for i := 0; i <= testBounds.Count; i++ {
		writeTraceTestFile(t, dir, base.Add(-time.Duration(i)*time.Minute), hexID(i), deadTestPID)
	}

	if err := Sweep(dir, testBounds); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertExists(t, foreign)
	assertExists(t, subdir)
}

// TestSweep_DeleteFailureTolerance verifies delete failures are silently tolerated.
func TestSweep_DeleteFailureTolerance(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permissions do not block unlink, cannot simulate a delete failure this way")
	}

	dir := t.TempDir()
	stale := writeTraceTestFile(t, dir, time.Now().Add(-15*24*time.Hour), hexID(1), deadTestPID)

	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("Chmod(%s) = %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if err := Sweep(dir, testBounds); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil even when a delete fails", dir, err)
	}

	assertExists(t, stale)
}

// TestSweep_EmptyOrAbsentDirectory verifies empty or absent directories return nil.
func TestSweep_EmptyOrAbsentDirectory(t *testing.T) {
	t.Parallel()
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := Sweep(dir, testBounds); err != nil {
			t.Errorf("Sweep(%s) = %v; want nil", dir, err)
		}
	})

	t.Run("Absent", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "does-not-exist")
		if err := Sweep(dir, testBounds); err != nil {
			t.Errorf("Sweep(%s) = %v; want nil", dir, err)
		}
	})
}
