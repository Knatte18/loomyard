// archive_test.go covers firstFreeArchivePath's same-second collision suffixing, archiveStateFile's
// absent-file no-op and rename/preserve behavior, and archiveReportsDir's recreate-after-archive
// and absent-dir-still-recreates behavior.
// Tier 1: no git, only t.TempDir() plus an injected now.

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// archiveFixedClock returns a func() time.Time that always returns t.
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

// archiveRecordGeom builds a Geometry over temp directories with a populated webster dir.
func archiveRecordGeom(t *testing.T) Geometry {
	t.Helper()
	root := t.TempDir()
	geom := Geometry{
		WebsterDir: filepath.Join(root, "webster"),
		ReportsDir: filepath.Join(root, "webster", "reports"),
		PromptsDir: filepath.Join(root, "scratch", "prompts"),
		ScratchDir: filepath.Join(root, "scratch"),
	}
	if err := SaveState(geom.WebsterDir, geom.ScratchDir, &State{RunGUID: "g1"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := os.MkdirAll(geom.ReportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(geom.ReportsDir, "01.yaml"), []byte("status: OK\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(geom.PromptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(geom.PromptsDir, "01.md"), []byte("p"), 0o644); err != nil {
		t.Fatal(err)
	}
	return geom
}

func TestArchiveRunRecord_MovesWholeRecordAndClearsPrompts(t *testing.T) {
	t.Parallel()

	geom := archiveRecordGeom(t)
	dest := filepath.Join(t.TempDir(), "dest")

	if err := ArchiveRunRecord(geom, dest); err != nil {
		t.Fatalf("ArchiveRunRecord() error = %v; want nil", err)
	}

	for _, rel := range []string{"state.json", "outcome.yaml", filepath.Join("reports", "01.yaml")} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			t.Errorf("archived %s missing: %v", rel, err)
		}
	}
	st, err := LoadState(geom.WebsterDir, geom.ScratchDir)
	if err != nil || st != nil {
		t.Errorf("LoadState after archive = (%v, %v); want (nil, nil)", st, err)
	}
	if _, err := os.Stat(geom.PromptsDir); !os.IsNotExist(err) {
		t.Errorf("prompts dir still present: %v", err)
	}
}

func TestArchiveRunRecord_SecondCallIsNoOp(t *testing.T) {
	t.Parallel()

	geom := archiveRecordGeom(t)
	dest := filepath.Join(t.TempDir(), "dest")

	if err := ArchiveRunRecord(geom, dest); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := ArchiveRunRecord(geom, dest); err != nil {
		t.Fatalf("second call error = %v; want nil", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "state.json")); err != nil {
		t.Errorf("archived state.json missing after second call: %v", err)
	}
}

func TestArchiveRunRecord_AbsentWebsterDirIsNoOp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	geom := Geometry{
		WebsterDir: filepath.Join(root, "webster"),
		PromptsDir: filepath.Join(root, "scratch", "prompts"),
		ScratchDir: filepath.Join(root, "scratch"),
	}
	dest := filepath.Join(root, "dest")

	if err := ArchiveRunRecord(geom, dest); err != nil {
		t.Fatalf("ArchiveRunRecord() error = %v; want nil", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("dest created for an absent webster dir: %v", err)
	}
}

func TestArchiveRunRecord_CollisionErrorsAndMovesNothing(t *testing.T) {
	t.Parallel()

	geom := archiveRecordGeom(t)
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "outcome.yaml"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ArchiveRunRecord(geom, dest)
	if err == nil {
		t.Fatal("ArchiveRunRecord() error = nil; want collision error")
	}
	if !strings.Contains(err.Error(), "outcome.yaml") || !strings.Contains(err.Error(), "way forward") {
		t.Errorf("error = %v; want it to name the entry and a way forward", err)
	}
	if _, statErr := os.Stat(filepath.Join(geom.WebsterDir, "state.json")); statErr != nil {
		t.Errorf("state.json moved despite collision: %v", statErr)
	}
}

func TestArchiveRunRecord_HeldRunLockReturnsErrRunBusy(t *testing.T) {
	t.Parallel()

	geom := archiveRecordGeom(t)
	held, locked, err := lock.TryAcquireWriteLock(filepath.Join(geom.ScratchDir, runLockName))
	if err != nil || !locked {
		t.Fatalf("hold run lock: locked=%v err=%v", locked, err)
	}
	defer held.Release()

	err = ArchiveRunRecord(geom, filepath.Join(t.TempDir(), "dest"))
	if !errors.Is(err, ErrRunBusy) {
		t.Errorf("ArchiveRunRecord() error = %v; want ErrRunBusy", err)
	}
}
