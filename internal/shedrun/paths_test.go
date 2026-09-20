package shedrun

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// syntheticLocation returns a *lyxcwd.Location anchored at a t.TempDir()-rooted worktree, with no
// git or filesystem I/O of its own -- every constructor under test is a plain filepath.Join, so this
// only needs to carry an AnchorPath() that resolves predictably.
func syntheticLocation(t *testing.T) *lyxcwd.Location {
	t.Helper()
	hub := t.TempDir()
	return &lyxcwd.Location{
		RepoName:     "repo",
		HubPath:      hub,
		WorktreeName: "worktree",
		AnchorRel:    ".",
	}
}

func TestRunDir(t *testing.T) {
	l := syntheticLocation(t)
	got := RunDir(l, "self")
	want := filepath.Join(l.AnchorPath(), "_lyx", "shed", "self")
	if got != want {
		t.Errorf("RunDir(l, %q) = %q; want %q", "self", got, want)
	}
}

func TestSeedFile(t *testing.T) {
	l := syntheticLocation(t)
	got := SeedFile(l, "self")
	want := filepath.Join(RunDir(l, "self"), "seed.json")
	if got != want {
		t.Errorf("SeedFile(l, %q) = %q; want %q", "self", got, want)
	}
}

func TestStatusFile(t *testing.T) {
	l := syntheticLocation(t)
	got := StatusFile(l, "self")
	want := filepath.Join(RunDir(l, "self"), "status.json")
	if got != want {
		t.Errorf("StatusFile(l, %q) = %q; want %q", "self", got, want)
	}
}

func TestScratchDir(t *testing.T) {
	l := syntheticLocation(t)
	got := ScratchDir(l, "self")
	want := filepath.Join(l.AnchorPath(), ".lyx", "shed", "self")
	if got != want {
		t.Errorf("ScratchDir(l, %q) = %q; want %q", "self", got, want)
	}
}

func TestRunLock(t *testing.T) {
	l := syntheticLocation(t)
	got := RunLock(l, "self")
	want := filepath.Join(ScratchDir(l, "self"), "run.lock")
	if got != want {
		t.Errorf("RunLock(l, %q) = %q; want %q", "self", got, want)
	}
}

func TestStatusLock(t *testing.T) {
	l := syntheticLocation(t)
	got := StatusLock(l, "self")
	want := filepath.Join(ScratchDir(l, "self"), "status.json.lock")
	if got != want {
		t.Errorf("StatusLock(l, %q) = %q; want %q", "self", got, want)
	}
}

func TestLastCommitMarker(t *testing.T) {
	l := syntheticLocation(t)
	got := LastCommitMarker(l, "self")
	want := filepath.Join(ScratchDir(l, "self"), "last-commit")
	if got != want {
		t.Errorf("LastCommitMarker(l, %q) = %q; want %q", "self", got, want)
	}
}

func TestSeedRel(t *testing.T) {
	got := SeedRel("self")
	want := filepath.Join("_lyx", "shed", "self", "seed.json")
	if got != want {
		t.Errorf("SeedRel(%q) = %q; want %q", "self", got, want)
	}
}

func TestStatusRel(t *testing.T) {
	got := StatusRel("self")
	want := filepath.Join("_lyx", "shed", "self", "status.json")
	if got != want {
		t.Errorf("StatusRel(%q) = %q; want %q", "self", got, want)
	}
}

func TestPrimeRunLock(t *testing.T) {
	l := syntheticLocation(t)
	got := PrimeRunLock(l)
	want := filepath.Join(l.AnchorPath(), ".lyx", "shed", "run.lock")
	if got != want {
		t.Errorf("PrimeRunLock(l) = %q; want %q", got, want)
	}
}

// TestRunLock_NeverEqualsStatusLock pins shedengine.Shed's own validation, which rejects
// LockPath == StatusLockPath outright: a shared file would hang on the first persist rather than
// fail.
func TestRunLock_NeverEqualsStatusLock(t *testing.T) {
	l := syntheticLocation(t)
	for _, runID := range []string{"self", "abc123", "run-two", "another-run-id"} {
		t.Run(runID, func(t *testing.T) {
			if got := RunLock(l, runID); got == StatusLock(l, runID) {
				t.Errorf("RunLock(l, %q) == StatusLock(l, %q) = %q; must differ", runID, runID, got)
			}
		})
	}
}

// TestPrimeRunLock_NeverEqualsRunOrStatusLock pins PrimeRunLock as a distinct hub-scoped lock from
// any single run-id's own RunLock or StatusLock.
func TestPrimeRunLock_NeverEqualsRunOrStatusLock(t *testing.T) {
	l := syntheticLocation(t)
	prime := PrimeRunLock(l)
	for _, runID := range []string{"self", "abc123", "run-two", "another-run-id"} {
		t.Run(runID, func(t *testing.T) {
			if prime == RunLock(l, runID) {
				t.Errorf("PrimeRunLock(l) == RunLock(l, %q) = %q; must differ", runID, prime)
			}
			if prime == StatusLock(l, runID) {
				t.Errorf("PrimeRunLock(l) == StatusLock(l, %q) = %q; must differ", runID, prime)
			}
		})
	}
}
