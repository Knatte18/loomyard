// archive_test.go covers firstFreeArchivePath's same-second collision
// suffixing, archiveStateFile's absent-file no-op and rename/preserve
// behavior, and archiveReportsDir's recreate-after-archive and
// absent-dir-still-recreates behavior. Tier 1: no git, only t.TempDir()
// plus an injected now.

package websterengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// archiveFixedClock returns a func() time.Time that always returns t,
// letting a test pin an archive helper's timestamp deterministically
// instead of racing the real clock.
func archiveFixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestFirstFreeArchivePath_ReturnsBareCandidateWhenFree(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	candidate := func(suffix string) string { return filepath.Join(dir, "target"+suffix+".txt") }

	got, err := firstFreeArchivePath(candidate)
	if err != nil {
		t.Fatalf("firstFreeArchivePath() error = %v; want nil", err)
	}
	want := filepath.Join(dir, "target.txt")
	if got != want {
		t.Errorf("firstFreeArchivePath() = %q; want %q", got, want)
	}
}

func TestFirstFreeArchivePath_CollisionAppendsSuffix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	candidate := func(suffix string) string { return filepath.Join(dir, "target"+suffix+".txt") }

	// Occupy the bare candidate and its first numeric suffix so the search
	// must skip both before finding a free path.
	if err := os.WriteFile(candidate(""), []byte("taken"), 0o644); err != nil {
		t.Fatalf("write %s: %v", candidate(""), err)
	}
	if err := os.WriteFile(candidate("-1"), []byte("also taken"), 0o644); err != nil {
		t.Fatalf("write %s: %v", candidate("-1"), err)
	}

	got, err := firstFreeArchivePath(candidate)
	if err != nil {
		t.Fatalf("firstFreeArchivePath() error = %v; want nil", err)
	}
	want := candidate("-2")
	if got != want {
		t.Errorf("firstFreeArchivePath() = %q; want %q", got, want)
	}
}

func TestArchiveStateFile_AbsentFileIsNoOp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	clk := archiveFixedClock(time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC))

	got, err := archiveStateFile(dir, clk)
	if err != nil {
		t.Fatalf("archiveStateFile() error = %v; want nil", err)
	}
	if got != "" {
		t.Errorf("archiveStateFile() = %q; want \"\" for an absent state.json", got)
	}
}

func TestArchiveStateFile_RenamesAndPreservesContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	original := filepath.Join(dir, "state.json")
	content := `{"runGuid":"abc123"}`
	if err := os.WriteFile(original, []byte(content), 0o644); err != nil {
		t.Fatalf("write state.json: %v", err)
	}

	clk := archiveFixedClock(time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC))
	got, err := archiveStateFile(dir, clk)
	if err != nil {
		t.Fatalf("archiveStateFile() error = %v; want nil", err)
	}

	want := filepath.Join(dir, "state-20260711T134500Z.json")
	if got != want {
		t.Errorf("archiveStateFile() = %q; want %q", got, want)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Errorf("original state.json still exists after archiving; want it renamed away")
	}

	archived, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", got, err)
	}
	if string(archived) != content {
		t.Errorf("archived content = %q; want %q", archived, content)
	}
}

func TestArchiveReportsDir_AbsentDirStillRecreatesEmpty(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	reportsDir := filepath.Join(root, "reports")
	clk := archiveFixedClock(time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC))

	if err := archiveReportsDir(reportsDir, clk); err != nil {
		t.Fatalf("archiveReportsDir() error = %v; want nil", err)
	}

	info, err := os.Stat(reportsDir)
	if err != nil {
		t.Fatalf("Stat(reportsDir) after archiveReportsDir() on an absent dir: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("reportsDir = %q; want a directory", reportsDir)
	}

	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		t.Fatalf("ReadDir(reportsDir): %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("recreated reportsDir has %d entries; want empty", len(entries))
	}
}

func TestArchiveReportsDir_ArchivesExistingContentAndRecreatesEmpty(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	reportsDir := filepath.Join(root, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(reportsDir): %v", err)
	}
	reportPath := filepath.Join(reportsDir, "01-first.yaml")
	if err := os.WriteFile(reportPath, []byte("status: done\n"), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	clk := archiveFixedClock(time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC))
	if err := archiveReportsDir(reportsDir, clk); err != nil {
		t.Fatalf("archiveReportsDir() error = %v; want nil", err)
	}

	// The old reports dir must have been renamed wholesale, carrying its
	// content along, and a fresh empty reportsDir recreated in its place.
	archivedDir := filepath.Join(root, "reports-20260711T134500Z")
	archivedReport := filepath.Join(archivedDir, "01-first.yaml")
	content, err := os.ReadFile(archivedReport)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v; want the archived report preserved", archivedReport, err)
	}
	if string(content) != "status: done\n" {
		t.Errorf("archived report content = %q; want %q", content, "status: done\n")
	}

	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		t.Fatalf("ReadDir(reportsDir) after archive: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("recreated reportsDir has %d entries; want empty", len(entries))
	}
}
