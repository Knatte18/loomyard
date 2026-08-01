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

// deadTestPID is a PID far above any value a real platform would ever
// assign (Linux's pid_max tops out well under this), used by tests that
// need a "dead" candidate without spawning and killing a real process —
// spawning is banned in this untagged file by the Test Tier Purity
// Invariant's banned-substring guard.
const deadTestPID = 999999999

// traceTestFileName renders a trace-file name matching traceFilePattern for
// the given filename timestamp, hex ID, and pid, mirroring the
// trace-<UTC>-<16-hex>-<pid>.log grammar Sweep parses.
func traceTestFileName(ts time.Time, id string, pid int) string {
	return fmt.Sprintf("trace-%s-%s-%d.log", ts.UTC().Format(traceFileTimestampLayout), id, pid)
}

// writeTraceTestFile creates an empty file named per traceTestFileName
// inside dir and returns its full path.
func writeTraceTestFile(t *testing.T, dir string, ts time.Time, id string, pid int) string {
	t.Helper()
	path := filepath.Join(dir, traceTestFileName(ts, id, pid))
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("write test trace file %s: %v", path, err)
	}
	return path
}

// hexID renders n as a 16-lowercase-hex-character string, matching the
// trace-file grammar's ID segment shape (the actual value is irrelevant to
// retention — only its shape needs to match traceFilePattern).
func hexID(n int) string {
	return fmt.Sprintf("%016x", n)
}

// assertExists fails the test unless path exists.
func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to exist, got: %v", path, err)
	}
}

// assertAbsent fails the test unless path does not exist.
func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted, stat err = %v", path, err)
	}
}

// TestSweep_AgeBound covers the retention decision's age bound: a candidate
// whose filename timestamp is older than the 14-day bound is deleted, and
// one within the bound is kept.
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

// TestSweep_CountBound covers the retention decision's count bound: when
// more than retentionCountBound in-age-bound candidates survive the age
// pass, only the newest retentionCountBound by filename timestamp are
// kept — ranked by filename, never mtime. One candidate is seeded with an
// old filename timestamp but a fresh mtime to pin that the ranking key is
// the filename segment.
func TestSweep_CountBound(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Minute)

	// Seed retentionCountBound+1 candidates with distinct, descending
	// filename timestamps: index 0 is newest, index retentionCountBound is
	// oldest by filename.
	var paths []string
	for i := 0; i <= retentionCountBound; i++ {
		ts := base.Add(-time.Duration(i) * time.Minute)
		paths = append(paths, writeTraceTestFile(t, dir, ts, hexID(i), deadTestPID))
	}

	// Pin the ranking key: give the oldest-by-filename candidate a fresh
	// mtime. If Sweep ranked by mtime instead of filename, this file would
	// look newest and survive; it must not.
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

// TestSweep_GrammarScope covers the retention decision's sweep-scope rule:
// a file that does not match the trace-file grammar is never deleted and
// never counted toward the count bound.
func TestSweep_GrammarScope(t *testing.T) {
	dir := t.TempDir()

	foreign := filepath.Join(dir, "tmux-server-1234.log")
	if err := os.WriteFile(foreign, []byte("test"), 0o644); err != nil {
		t.Fatalf("write foreign file: %v", err)
	}

	// Seed more than the count bound of legitimate, in-bound candidates so
	// that if the foreign file were mistakenly treated as a candidate (and,
	// worse, ranked as "newest" by some accidental zero-value timestamp),
	// it would still have to survive for this test to distinguish "ignored"
	// from "accidentally kept by ranking".
	base := time.Now().Add(-time.Minute)
	for i := 0; i <= retentionCountBound; i++ {
		writeTraceTestFile(t, dir, base.Add(-time.Duration(i)*time.Minute), hexID(i), deadTestPID)
	}

	if err := Sweep(dir); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil", dir, err)
	}

	assertExists(t, foreign)
}

// TestSweep_DeleteFailureTolerance covers the retention decision's
// delete-failure tolerance: a per-file delete failure (simulated here via a
// read-only directory permission, the non-Windows equivalent of a locked
// file) is skipped rather than propagated — Sweep still returns nil.
func TestSweep_DeleteFailureTolerance(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permissions do not block unlink, cannot simulate a delete failure this way")
	}

	dir := t.TempDir()
	stale := writeTraceTestFile(t, dir, time.Now().Add(-15*24*time.Hour), hexID(1), deadTestPID)

	// Remove write permission on dir itself: unlink requires write access
	// to the containing directory, not the file, so this reliably makes
	// os.Remove fail without touching the file's own mode.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("Chmod(%s) = %v", dir, err)
	}
	// Restore write permission before t.TempDir()'s own cleanup runs, or
	// the test framework cannot remove the directory either.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if err := Sweep(dir); err != nil {
		t.Fatalf("Sweep(%s) = %v; want nil even when a delete fails", dir, err)
	}

	assertExists(t, stale)
}

// TestSweep_EmptyOrAbsentDirectory covers the retention decision's
// empty/absent-directory no-op: Sweep on a directory with nothing to sweep,
// or on a path that does not exist at all, returns nil with no panic.
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
