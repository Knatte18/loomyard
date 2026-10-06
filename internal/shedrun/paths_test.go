package shedrun

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// worktreeName is the worktree name of every syntheticLocation, the run-id "self" resolves to.
const worktreeName = "worktree"

// syntheticLocation returns a *lyxcwd.Location anchored at a t.TempDir()-rooted worktree, with no
// git or filesystem I/O of its own -- every constructor under test is a plain filepath.Join, so this
// only needs to carry an AnchorPath() that resolves predictably.
func syntheticLocation(t *testing.T) *lyxcwd.Location {
	t.Helper()
	hub := t.TempDir()
	return &lyxcwd.Location{
		RepoName:     "repo",
		HubPath:      hub,
		WorktreeName: worktreeName,
		AnchorRel:    ".",
	}
}

// TestPathConstructors pins every path constructor's layout under the anchor: durable files under _lyx/shed/<run>,
// ephemeral ones under .lyx/shed/<run>.
func TestPathConstructors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		got  func(l *lyxcwd.Location) string
		want func(l *lyxcwd.Location) string
	}{
		{"RunDir", func(l *lyxcwd.Location) string { return RunDir(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join(l.AnchorPath(), "_lyx", "shed", worktreeName) }},
		{"SeedFile", func(l *lyxcwd.Location) string { return SeedFile(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join(RunDir(l, "self"), "seed.json") }},
		{"StatusFile", func(l *lyxcwd.Location) string { return StatusFile(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join(RunDir(l, "self"), "status.json") }},
		{"StepsDir", func(l *lyxcwd.Location) string { return StepsDir(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "steps") }},
		{"ScratchDir", func(l *lyxcwd.Location) string { return ScratchDir(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join(l.AnchorPath(), ".lyx", "shed", worktreeName) }},
		{"RunLock", func(l *lyxcwd.Location) string { return RunLock(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "run.lock") }},
		{"StatusLock", func(l *lyxcwd.Location) string { return StatusLock(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "status.json.lock") }},
		{"LastCommitMarker", func(l *lyxcwd.Location) string { return LastCommitMarker(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "last-commit") }},
		{"DriveReportsDir", func(l *lyxcwd.Location) string { return DriveReportsDir(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join(RunDir(l, "self"), "drive-reports") }},
		{"PrimeRunLock", PrimeRunLock,
			func(l *lyxcwd.Location) string { return filepath.Join(l.AnchorPath(), ".lyx", "shed", "run.lock") }},
		{"SeedRel", func(l *lyxcwd.Location) string { return SeedRel(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join("_lyx", "shed", worktreeName, "seed.json") }},
		{"StatusRel", func(l *lyxcwd.Location) string { return StatusRel(l, "self") },
			func(l *lyxcwd.Location) string { return filepath.Join("_lyx", "shed", worktreeName, "status.json") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := syntheticLocation(t)
			if got, want := tt.got(l), tt.want(l); got != want {
				t.Errorf("%s = %q; want %q", tt.name, got, want)
			}
		})
	}
}

func TestParkMarker(t *testing.T) {
	t.Parallel()
	l := syntheticLocation(t)
	for _, runID := range []string{"worktree", "self"} {
		t.Run(runID, func(t *testing.T) {
			t.Parallel()
			got := ParkMarker(l, runID)
			want := filepath.Join(ScratchDir(l, runID), ParkMarkerFileName)
			if got != want {
				t.Errorf("ParkMarker(l, %q) = %q; want %q", runID, got, want)
			}
			rel, err := filepath.Rel(filepath.Join(l.AnchorPath(), ".lyx"), got)
			if err != nil || strings.HasPrefix(rel, "..") {
				t.Errorf("ParkMarker(l, %q) = %q; want it under the .lyx tree", runID, got)
			}
		})
	}
}

func TestRunsRootRel(t *testing.T) {
	t.Parallel()
	l := syntheticLocation(t)
	root := RunsRootRel()
	if want := filepath.Join(lyxdirs.LyxDirName, "shed"); root != want {
		t.Errorf("RunsRootRel() = %q; want %q", root, want)
	}
	prefix := root + string(filepath.Separator)
	for name, got := range map[string]string{
		"SeedRel":   SeedRel(l, "x"),
		"StatusRel": StatusRel(l, "x"),
	} {
		if !strings.HasPrefix(got, prefix) {
			t.Errorf("%s(l, %q) = %q; want it strictly under %q", name, "x", got, root)
		}
	}
}

func TestDriveReportsRel(t *testing.T) {
	t.Parallel()
	anchored := syntheticLocation(t)
	anchored.AnchorRel = "sub"
	for name, l := range map[string]*lyxcwd.Location{
		"unanchored": syntheticLocation(t),
		"anchored":   anchored,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := DriveReportsRel(l, "self")
			want, err := filepath.Rel(l.AnchorPath(), DriveReportsDir(l, "self"))
			if err != nil {
				t.Fatalf("filepath.Rel: %v", err)
			}
			if got != want {
				t.Errorf("DriveReportsRel = %q; want %q", got, want)
			}
		})
	}
}

// TestLocksAreDistinct pins that a run's RunLock and StatusLock never share a file, which shedengine.Shed's
// own validation rejects outright (a shared file would hang on the first persist rather than fail),
// and that PrimeRunLock is a distinct hub-scoped lock from any single run-id's locks.
//
//testtiming:keep pins that RunLock, StatusLock and PrimeRunLock never share a file, which its covering tests do not
func TestLocksAreDistinct(t *testing.T) {
	t.Parallel()
	l := syntheticLocation(t)
	prime := PrimeRunLock(l)
	for _, runID := range []string{"self", "abc123", "run-two", "another-run-id"} {
		t.Run(runID, func(t *testing.T) {
			t.Parallel()
			if got := RunLock(l, runID); got == StatusLock(l, runID) {
				t.Errorf("RunLock(l, %q) == StatusLock(l, %q) = %q; must differ", runID, runID, got)
			}
			if prime == RunLock(l, runID) {
				t.Errorf("PrimeRunLock(l) == RunLock(l, %q) = %q; must differ", runID, prime)
			}
			if prime == StatusLock(l, runID) {
				t.Errorf("PrimeRunLock(l) == StatusLock(l, %q) = %q; must differ", runID, prime)
			}
		})
	}
}
