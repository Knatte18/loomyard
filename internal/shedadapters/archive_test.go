package shedadapters

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixedClock(instant time.Time) func() time.Time {
	return func() time.Time { return instant }
}

func TestArchiveStaleOutputs_RenamesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.md")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	instant := time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)

	if err := archiveStaleOutputs([]string{path}, fixedClock(instant)); err != nil {
		t.Fatalf("archiveStaleOutputs = %v; want nil", err)
	}

	want := filepath.Join(dir, "review-20260816T151326Z.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected archived file %s to exist: %v", want, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("original path %s still exists after archive", path)
	}
}

func TestArchiveStaleOutputs_CollisionTakesNumericSuffix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.md")
	instant := time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)

	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := archiveStaleOutputs([]string{path}, fixedClock(instant)); err != nil {
		t.Fatalf("archiveStaleOutputs (first) = %v; want nil", err)
	}

	if err := os.WriteFile(path, []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := archiveStaleOutputs([]string{path}, fixedClock(instant)); err != nil {
		t.Fatalf("archiveStaleOutputs (second) = %v; want nil", err)
	}

	want := filepath.Join(dir, "review-20260816T151326Z-1.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected collision-suffixed archive file %s to exist: %v", want, err)
	}
}

func TestArchiveStaleOutputs_AbsentEntryIsNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.md")
	instant := time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)

	if err := archiveStaleOutputs([]string{path}, fixedClock(instant)); err != nil {
		t.Errorf("archiveStaleOutputs(absent) = %v; want nil", err)
	}
}

func TestArchiveRunDir_MovesEveryEntryAndRecreatesEmpty(t *testing.T) {
	parent := t.TempDir()
	runDir := filepath.Join(parent, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "round-1-review.md"), []byte("review"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "round-1-bouncer-verdict.md"), []byte("verdict"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	instant := time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)

	if err := archiveRunDir(runDir, fixedClock(instant)); err != nil {
		t.Fatalf("archiveRunDir = %v; want nil", err)
	}

	want := filepath.Join(parent, "run-20260816T151326Z")
	entries, err := os.ReadDir(want)
	if err != nil {
		t.Fatalf("expected archived sibling %s to exist: %v", want, err)
	}
	if len(entries) != 2 {
		t.Errorf("archived sibling %s has %d entries; want 2", want, len(entries))
	}

	freshEntries, err := os.ReadDir(runDir)
	if err != nil {
		t.Fatalf("expected recreated run dir %s to exist: %v", runDir, err)
	}
	if len(freshEntries) != 0 {
		t.Errorf("recreated run dir %s has %d entries; want 0", runDir, len(freshEntries))
	}
}

func TestArchiveRunDir_CollisionTakesNumericSuffix(t *testing.T) {
	parent := t.TempDir()
	runDir := filepath.Join(parent, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	instant := time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)

	if err := archiveRunDir(runDir, fixedClock(instant)); err != nil {
		t.Fatalf("archiveRunDir (first) = %v; want nil", err)
	}
	if err := archiveRunDir(runDir, fixedClock(instant)); err != nil {
		t.Fatalf("archiveRunDir (second) = %v; want nil", err)
	}

	want := filepath.Join(parent, "run-20260816T151326Z-1")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected collision-suffixed archive dir %s to exist: %v", want, err)
	}
}

func TestArchiveRunDir_RenameFailureReturnsError(t *testing.T) {
	parent := t.TempDir()
	runDir := filepath.Join(parent, "missing")
	instant := time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)

	if err := archiveRunDir(runDir, fixedClock(instant)); err == nil {
		t.Fatalf("archiveRunDir(absent run dir) = nil; want error")
	}
}

func TestArchiveStaleOutputs_MixedListArchivesOnlyExisting(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present.md")
	absent := filepath.Join(dir, "absent.md")
	instant := time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)

	if err := os.WriteFile(present, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := archiveStaleOutputs([]string{present, absent}, fixedClock(instant)); err != nil {
		t.Fatalf("archiveStaleOutputs(mixed) = %v; want nil", err)
	}

	wantArchived := filepath.Join(dir, "present-20260816T151326Z.md")
	if _, err := os.Stat(wantArchived); err != nil {
		t.Errorf("expected archived file %s to exist: %v", wantArchived, err)
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Errorf("absent entry %s unexpectedly exists after archive", absent)
	}
}
