// retention_test.go exercises Sweep's age bound, count bound,
// grammar-scoping, and delete-failure tolerance over a t.TempDir() — pure
// filesystem logic, no git/exec spawns, per the Test Tier Purity Invariant.

package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const deadTestPID = 999999999

func traceTestFileName(ts time.Time, id string, pid int) string {
	return fmt.Sprintf("trace-%s-%s-%d.log", ts.UTC().Format(traceFileTimestampLayout), id, pid)
}

func writeTraceTestFile(t *testing.T, dir string, ts time.Time, id string, pid int) string {
	t.Helper()
	path := filepath.Join(dir, traceTestFileName(ts, id, pid))
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("write test trace file %s: %v", path, err)
	}
	return path
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

// TestSweep_AgeBound verifies files older than the age bound are deleted.
func TestSweep_AgeBound(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	oldFile := writeTraceTestFile(t, dir, now.Add(-15*24*time.Hour), hexID(1), deadTestPID)
	recentFile := writeTraceTestFile(t, dir, now.Add(-1*time.Hour), hexID(2), deadTestPID)

	if err := Sweep(dir); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertAbsent(t, oldFile)
	assertExists(t, recentFile)
}

// TestSweep_CountBound verifies only the newest files by filename timestamp are kept.
func TestSweep_CountBound(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Minute)

	var paths []string
	for i := 0; i <= retentionCountBound; i++ {
		ts := base.Add(-time.Duration(i) * time.Minute)
		paths = append(paths, writeTraceTestFile(t, dir, ts, hexID(i), deadTestPID))
	}

	oldestByName := paths[retentionCountBound]
	freshMtime := time.Now()
	if err := os.Chtimes(oldestByName, freshMtime, freshMtime); err != nil {
		t.Fatalf("Chtimes(%s) = %v", oldestByName, err)
	}

	if err := Sweep(dir); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	for i, path := range paths {
		if i < retentionCountBound {
			assertExists(t, path)
		} else {
			assertAbsent(t, path)
		}
	}
}

// TestSweep_GrammarScope verifies non-matching files are never deleted.
func TestSweep_GrammarScope(t *testing.T) {
	dir := t.TempDir()

	foreign := filepath.Join(dir, "tmux-server-1234.log")
	if err := os.WriteFile(foreign, []byte("test"), 0o644); err != nil {
		t.Fatalf("write foreign file: %v", err)
	}

	base := time.Now().Add(-time.Minute)
	for i := 0; i <= retentionCountBound; i++ {
		writeTraceTestFile(t, dir, base.Add(-time.Duration(i)*time.Minute), hexID(i), deadTestPID)
	}

	if err := Sweep(dir); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertExists(t, foreign)
}

// TestSweep_DeleteFailureTolerance verifies delete failures are silently tolerated.
func TestSweep_DeleteFailureTolerance(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permissions do not block unlink, cannot simulate a delete failure this way")
	}

	dir := t.TempDir()
	stale := writeTraceTestFile(t, dir, time.Now().Add(-15*24*time.Hour), hexID(1), deadTestPID)

	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("Chmod(%s) = %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if err := Sweep(dir); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil even when a delete fails", dir, err)
	}

	assertExists(t, stale)
}

// TestSweep_EmptyOrAbsentDirectory verifies empty or absent directories return nil.
func TestSweep_EmptyOrAbsentDirectory(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		dir := t.TempDir()
		if err := Sweep(dir); err != nil {
			t.Errorf("Sweep(%s) = %v; want nil", dir, err)
		}
	})

	t.Run("Absent", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "does-not-exist")
		if err := Sweep(dir); err != nil {
			t.Errorf("Sweep(%s) = %v; want nil", dir, err)
		}
	})
}

// TestSweep_LivenessSkipsSelfProcess verifies the current process's file is never deleted.
func TestSweep_LivenessSkipsSelfProcess(t *testing.T) {
	dir := t.TempDir()
	selfFile := writeTraceTestFile(t, dir, time.Now().Add(-15*24*time.Hour), hexID(1), os.Getpid())

	if err := Sweep(dir); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertExists(t, selfFile)
}

// TestSweep_LivenessDeletesDeadPID verifies dead PIDs are deleted normally.
func TestSweep_LivenessDeletesDeadPID(t *testing.T) {
	dir := t.TempDir()
	deadFile := writeTraceTestFile(t, dir, time.Now().Add(-15*24*time.Hour), hexID(1), deadTestPID)

	if err := Sweep(dir); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertAbsent(t, deadFile)
}

// TestSweep_LiveSkipDoesNotConsumeCountBudget verifies live files don't consume count budget slots.
func TestSweep_LiveSkipDoesNotConsumeCountBudget(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Minute)

	liveFile := writeTraceTestFile(t, dir, base.Add(time.Minute), hexID(0), os.Getpid())

	var deadFiles []string
	for i := 0; i < retentionCountBound; i++ {
		ts := base.Add(-time.Duration(i) * time.Minute)
		deadFiles = append(deadFiles, writeTraceTestFile(t, dir, ts, hexID(i+1), deadTestPID))
	}

	if err := Sweep(dir); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertExists(t, liveFile)
	for _, path := range deadFiles {
		assertExists(t, path)
	}
}
