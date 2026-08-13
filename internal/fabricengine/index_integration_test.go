//go:build integration

// index_integration_test.go — integration tests for the fabric layer's git
// wiring around the correspondence index: gitdir resolution, the
// RecordCorrespondence/WeftSHAForWarpSHA round trip, and RebuildIndex's
// trailer scan. Package-internal (not fabricengine_test) because it asserts
// on weftGitDir, an unexported method. Uses gitkit.CopyWeft for the weft
// side and a minimal, locally-built plain git repo for the warp side —
// fabric's warp is just an ordinary warp repo, so these tests need none of
// CopyWeft's upstream-tracking setup or CopyPaired's junction/portal wiring
// on the warp side.

package fabricengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// commitWeftWithTrailer commits content into weftPath's tracked _lyx config
// file with a Warp-SHA trailer naming warpSHA — a hand-crafted stand-in for
// what CommitWeft (a later batch) produces — returning the new weft HEAD SHA.
func commitWeftWithTrailer(t *testing.T, weftPath, content, warpSHA string) string {
	t.Helper()

	configPath := filepath.Join(weftPath, "_lyx", "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, weftPath, "git", "add", ".")
	msg := appendWarpSHATrailer("weft sync", warpSHA)
	gitkit.MustRun(t, weftPath, "git", "commit", "-q", "-m", msg)
	return currentSHA(t, weftPath)
}

// TestWeftGitDir_ResolvesInsideWeftGitdir asserts that weftGitDir returns a path genuinely inside
// the weft worktree's own .git directory — the per-worktree gitdir the correspondence index is
// deliberately scoped to.
func TestWeftGitDir_ResolvesInsideWeftGitdir(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
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

// TestRecordAndLookupCorrespondence_RoundTrip asserts that a RecordCorrespondence call is visible
// to a subsequent WeftSHAForWarpSHA lookup, with WarpSeq computed from the warp repo's first-parent
// commit count.
func TestRecordAndLookupCorrespondence_RoundTrip(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
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

// TestWeftSHAForWarpSHA_NoEntryReturnsErrNoCorrespondence covers the miss path: a warp SHA with no
// recorded correspondence at all.
func TestWeftSHAForWarpSHA_NoEntryReturnsErrNoCorrespondence(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA := commitWarp(t, warpPath, "warp change, never synced")

	if _, err := f.WeftSHAForWarpSHA(warpSHA); !errors.Is(err, ErrNoCorrespondence) {
		t.Errorf("WeftSHAForWarpSHA() error = %v; want errors.Is(err, ErrNoCorrespondence)", err)
	}
}

// TestRebuildIndex_ReproducesTrailerHistory asserts that RebuildIndex, run against a weft branch
// carrying several hand-crafted Warp-SHA trailer commits, reconstructs an index whose lookups match
// what recording each correspondence incrementally would have produced — never having called
// RecordCorrespondence itself.
func TestRebuildIndex_ReproducesTrailerHistory(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
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
