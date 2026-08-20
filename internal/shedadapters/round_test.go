package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// reportName is a small test convention naming a stand-in round-report artifact, independent of
// any real round producer's own naming.
func reportName(round int) string {
	return fmt.Sprintf("round-%d-report.md", round)
}

func writeReport(t *testing.T, dir string, round int) {
	t.Helper()
	path := filepath.Join(dir, reportName(round))
	if err := os.WriteFile(path, []byte("report"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

func TestResolveRound_EmptyRunDirReturnsZero(t *testing.T) {
	dir := t.TempDir()

	got, err := ResolveRound(dir, reportName)
	if err != nil {
		t.Fatalf("ResolveRound(%q, ...) = _, %v; want nil error", dir, err)
	}
	if got != 0 {
		t.Errorf("ResolveRound(%q, ...) = %d; want 0", dir, got)
	}
}

func TestResolveRound_OnlyRoundOnePresent(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, 1)

	got, err := ResolveRound(dir, reportName)
	if err != nil {
		t.Fatalf("ResolveRound(%q, ...) = _, %v; want nil error", dir, err)
	}
	if got != 1 {
		t.Errorf("ResolveRound(%q, ...) = %d; want 1", dir, got)
	}
}

func TestResolveRound_RoundsOneThroughThreePresent(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, 1)
	writeReport(t, dir, 2)
	writeReport(t, dir, 3)

	got, err := ResolveRound(dir, reportName)
	if err != nil {
		t.Fatalf("ResolveRound(%q, ...) = _, %v; want nil error", dir, err)
	}
	if got != 3 {
		t.Errorf("ResolveRound(%q, ...) = %d; want 3", dir, got)
	}
}

func TestResolveRound_GapStopsScanBeforeLaterRounds(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, 1)
	writeReport(t, dir, 3)

	got, err := ResolveRound(dir, reportName)
	if err != nil {
		t.Fatalf("ResolveRound(%q, ...) = _, %v; want nil error", dir, err)
	}
	if got != 1 {
		t.Errorf("ResolveRound(%q, ...) = %d; want 1, not 3 -- round 2's absence must stop the scan", dir, got)
	}
}

func TestResolveRound_MissingRunDirReturnsError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")

	got, err := ResolveRound(dir, reportName)
	if err == nil {
		t.Fatalf("ResolveRound(%q, ...) = %d, nil; want a non-nil error", dir, got)
	}
}

func TestResolveRound_NonDirectoryReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}

	got, err := ResolveRound(path, reportName)
	if err == nil {
		t.Fatalf("ResolveRound(%q, ...) = %d, nil; want a non-nil error", path, got)
	}
}

// TestResolveRound_NonNotExistStatErrorIsReturned arranges a stat failure
// that is not fs.ErrNotExist (a permission failure from an unsearchable run
// dir), asserting that it surfaces as an error rather than being read as
// absence -- which would otherwise truncate the scan or re-seed an
// already-judged segment.
// Skipped when running as root (root bypasses directory permission checks)
// or on a platform where chmod cannot be used to arrange this.
func TestResolveRound_NonNotExistStatErrorIsReturned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission denial is not portable to windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permission checks")
	}

	parent := t.TempDir()
	dir := filepath.Join(parent, "run")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) = %v; want nil", dir, err)
	}
	writeReport(t, dir, 1)

	// Remove the directory's search (execute) bit so stat-ing a file inside
	// it fails with a permission error rather than not-exist.
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("Chmod(%q, 0o000) = %v; want nil", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
	})

	got, err := ResolveRound(dir, reportName)
	if err == nil {
		t.Fatalf("ResolveRound(%q, ...) = %d, nil; want a non-nil error from the permission-denied stat", dir, got)
	}
}

// TestResolveRound_BothDerivedReadings pins the two readings a caller derives
// from ResolveRound against a single disk state: with reports for rounds 1
// and 2 on disk, the round to judge is ResolveRound's own return value, and
// the round to write is that value plus one.
func TestResolveRound_BothDerivedReadings(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, 1)
	writeReport(t, dir, 2)

	roundToJudge, err := ResolveRound(dir, reportName)
	if err != nil {
		t.Fatalf("ResolveRound(%q, ...) = _, %v; want nil error", dir, err)
	}
	if roundToJudge != 2 {
		t.Errorf("round to judge = %d; want 2", roundToJudge)
	}
	roundToWrite := roundToJudge + 1
	if roundToWrite != 3 {
		t.Errorf("round to write = %d; want 3", roundToWrite)
	}
}

func TestRoundPathHelpers_PinExactFilenameSpellings(t *testing.T) {
	tests := []struct {
		name string
		fn   func(runDir string, round int) string
		want string
	}{
		{name: "verdictPath", fn: verdictPath, want: "round-5-bouncer-verdict.md"},
		{name: "ledgerPath", fn: ledgerPath, want: "round-5-bouncer-ledger.md"},
		{name: "focusPath", fn: focusPath, want: "round-5-focus.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn("/run", 5)
			want := filepath.Join("/run", tt.want)
			if got != want {
				t.Errorf("%s(%q, 5) = %q; want %q", tt.name, "/run", got, want)
			}
		})
	}
}
