//go:build integration

// index_integration_test.go — integration tests for the fabric layer's git
// wiring around the correspondence index: gitdir resolution, the
// RecordCorrespondence/WeftSHAForWarpSHA round trip, and RebuildIndex's
// trailer scan. Package-internal (not fabricengine_test) because it asserts
// on weftGitDir, an unexported method. Uses lyxtest.CopyWeft for the weft
// side and a minimal, locally-built plain git repo for the warp side —
// fabric's warp is just an ordinary host repo, so these tests need none of
// CopyWeft's upstream-tracking setup or CopyPaired's junction/portal wiring
// on the warp side.

package fabricengine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// newPlainWarpRepo creates a minimal, isolated git repo at t.TempDir() on
// branch main with one commit — everything RecordCorrespondence/warpSeq
// needs from a warp repo, without any of fabric's own topology wiring
// (junctions, weft pairing), which these index tests do not exercise.
func newPlainWarpRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	lyxtest.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("warp"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, dir, "git", "add", ".")
	lyxtest.MustRun(t, dir, "git", "commit", "-q", "-m", "init")
	return dir
}

// currentSHA returns dir's HEAD commit SHA.
func currentSHA(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

// commitWarp creates a new commit in warpPath carrying content, returning
// the new HEAD SHA.
func commitWarp(t *testing.T, warpPath, content string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(warpPath, "README"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, warpPath, "git", "add", ".")
	lyxtest.MustRun(t, warpPath, "git", "commit", "-q", "-m", content)
	return currentSHA(t, warpPath)
}

// commitWeftWithTrailer commits content into weftPath's tracked _lyx config
// file with a Warp-SHA trailer naming warpSHA — a hand-crafted stand-in for
// what CommitWeft (a later batch) produces — returning the new weft HEAD SHA.
func commitWeftWithTrailer(t *testing.T, weftPath, content, warpSHA string) string {
	t.Helper()

	configPath := filepath.Join(weftPath, "_lyx", "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, weftPath, "git", "add", ".")
	msg := appendWarpSHATrailer("weft sync", warpSHA)
	lyxtest.MustRun(t, weftPath, "git", "commit", "-q", "-m", msg)
	return currentSHA(t, weftPath)
}

// newFabric wraps New, failing the test on error rather than returning it —
// every test in this file expects a valid pair.
func newFabric(t *testing.T, warpPath, weftPath string) *Fabric {
	t.Helper()

	f, err := New(warpPath, weftPath)
	if err != nil {
		t.Fatalf("New(%q, %q) error = %v", warpPath, weftPath, err)
	}
	return f
}

// TestWeftGitDir_ResolvesInsideWeftGitdir asserts that weftGitDir returns a
// path genuinely inside the weft worktree's own .git directory — the
// per-worktree gitdir the correspondence index is deliberately scoped to.
func TestWeftGitDir_ResolvesInsideWeftGitdir(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	gitDir, err := f.weftGitDir()
	if err != nil {
		t.Fatalf("weftGitDir() error = %v", err)
	}
	wantPrefix := filepath.Join(weftFixture.WeftPath, ".git")
	if !strings.HasPrefix(gitDir, wantPrefix) {
		t.Errorf("weftGitDir() = %q; want it under %q", gitDir, wantPrefix)
	}
}

// TestRecordAndLookupCorrespondence_RoundTrip asserts that a
// RecordCorrespondence call is visible to a subsequent WeftSHAForWarpSHA
// lookup, with WarpSeq computed from the warp repo's first-parent commit
// count.
func TestRecordAndLookupCorrespondence_RoundTrip(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA := commitWarp(t, warpPath, "warp change 1")
	weftSHA := commitWeftWithTrailer(t, weftFixture.WeftPath, "weft change 1", warpSHA)

	if err := f.RecordCorrespondence(warpSHA, weftSHA); err != nil {
		t.Fatalf("RecordCorrespondence() error = %v", err)
	}

	got, err := f.WeftSHAForWarpSHA(warpSHA)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA() error = %v", err)
	}
	if got != weftSHA {
		t.Errorf("WeftSHAForWarpSHA(%q) = %q; want %q", warpSHA, got, weftSHA)
	}
}

// TestWeftSHAForWarpSHA_NoEntryReturnsErrNoCorrespondence covers the miss
// path: a warp SHA with no recorded correspondence at all.
func TestWeftSHAForWarpSHA_NoEntryReturnsErrNoCorrespondence(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA := commitWarp(t, warpPath, "warp change, never synced")

	if _, err := f.WeftSHAForWarpSHA(warpSHA); !errors.Is(err, ErrNoCorrespondence) {
		t.Errorf("WeftSHAForWarpSHA() error = %v; want errors.Is(err, ErrNoCorrespondence)", err)
	}
}

// TestRebuildIndex_ReproducesTrailerHistory asserts that RebuildIndex, run
// against a weft branch carrying several hand-crafted Warp-SHA trailer
// commits, reconstructs an index whose lookups match what recording each
// correspondence incrementally would have produced — never having called
// RecordCorrespondence itself.
func TestRebuildIndex_ReproducesTrailerHistory(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA1 := commitWarp(t, warpPath, "warp change 1")
	weftSHA1 := commitWeftWithTrailer(t, weftFixture.WeftPath, "weft change 1", warpSHA1)
	warpSHA2 := commitWarp(t, warpPath, "warp change 2")
	weftSHA2 := commitWeftWithTrailer(t, weftFixture.WeftPath, "weft change 2", warpSHA2)

	if err := f.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}

	wantByWarpSHA := map[string]string{warpSHA1: weftSHA1, warpSHA2: weftSHA2}
	for warpSHA, wantWeftSHA := range wantByWarpSHA {
		got, err := f.WeftSHAForWarpSHA(warpSHA)
		if err != nil {
			t.Fatalf("WeftSHAForWarpSHA(%q) error = %v", warpSHA, err)
		}
		if got != wantWeftSHA {
			t.Errorf("WeftSHAForWarpSHA(%q) = %q; want %q", warpSHA, got, wantWeftSHA)
		}
	}
}
