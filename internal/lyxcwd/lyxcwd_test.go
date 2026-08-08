//go:build integration

// lyxcwd_test.go covers Location resolution, the geometry accessors, and the
// ErrNotAGitRepo path for directories outside a git repo.

package lyxcwd_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestResolve_FromWorktreeRoot verifies that Resolve from the worktree root yields AnchorRel "."
// and correct other fields.
func TestResolve_FromWorktreeRoot(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	layout, err := lyxcwd.Resolve(hub)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	if layout == nil {
		t.Fatal("Resolve() returned nil layout")
	}

	// AnchorRel should be "." when no anchor is recorded, regardless of cwd.
	if layout.AnchorRel != "." {
		t.Errorf("layout.AnchorRel = %q; want %q", layout.AnchorRel, ".")
	}

	// WorktreePath() should be the hub (worktree root)
	if layout.WorktreePath() != filepath.Clean(hub) {
		t.Errorf("layout.WorktreePath() = %q; want %q", layout.WorktreePath(), filepath.Clean(hub))
	}

	// HubPath should be the parent of WorktreePath()
	expectedContainer := filepath.Dir(hub)
	if layout.HubPath != expectedContainer {
		t.Errorf("layout.HubPath = %q; want %q", layout.HubPath, expectedContainer)
	}

	// RepoName is derived by trimming HubSuffix off the container directory's base
	// name — this fixture's container has no "-HUB" suffix, so RepoName is simply
	// its base name unchanged.
	wantRepoName := strings.TrimSuffix(filepath.Base(layout.HubPath), fabricengine.HubSuffix)
	if layout.RepoName != wantRepoName {
		t.Errorf("layout.RepoName = %q; want %q", layout.RepoName, wantRepoName)
	}
}

// TestResolve_FromSubdirectory verifies that, for an unanchored repo, Resolve from a subdirectory
// errors under the strict cwd gate: with no anchor recorded, AnchorRel is only ever ".", so cwd is
// only ever accepted at the worktree root itself, never in a subdirectory.
func TestResolve_FromSubdirectory(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	// Create a subdirectory structure
	subDir := filepath.Join(hub, "subdir", "nested")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	layout, err := lyxcwd.Resolve(subDir)
	if layout != nil {
		t.Errorf("Resolve(%q) returned non-nil layout; want nil", subDir)
	}
	if !errors.Is(err, lyxcwd.ErrCwdOutsideAnchor) {
		t.Errorf("Resolve(%q) error = %v; want wrapped ErrCwdOutsideAnchor", subDir, err)
	}
}

// TestResolve_ForwardSlashNormalization verifies that forward-slash output from --show-toplevel is
// reconciled with backslash cwd on Windows.
func TestResolve_ForwardSlashNormalization(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	// Call Resolve normally; both cwd and --show-toplevel output get normalized
	layout, err := lyxcwd.Resolve(hub)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	// Verify paths are clean and use the platform's separator
	if layout.WorktreePath() != filepath.Clean(hub) {
		t.Errorf("layout.WorktreePath() = %q; want %q", layout.WorktreePath(), filepath.Clean(hub))
	}
}

// TestResolve_NotAGitRepo verifies that Resolve in a non-git temp directory returns ErrNotAGitRepo.
func TestResolve_NotAGitRepo(t *testing.T) {
	t.Parallel()

	nonGitDir := t.TempDir()

	layout, err := lyxcwd.Resolve(nonGitDir)

	if layout != nil {
		t.Errorf("Resolve() returned non-nil layout in non-git dir: %v", layout)
	}

	if !errors.Is(err, lyxcwd.ErrNotAGitRepo) {
		t.Errorf("Resolve() error = %v; want wrapped ErrNotAGitRepo", err)
	}

	// Pin the bare-sentinel behavior: git's raw stderr must never leak into the
	// error text, and no other content may be appended to the sentinel message.
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Resolve() error = %q; must not contain raw git stderr (\"fatal:\")", err.Error())
	}
	if err.Error() != lyxcwd.ErrNotAGitRepo.Error() {
		t.Errorf("Resolve() error = %q; want exactly %q", err.Error(), lyxcwd.ErrNotAGitRepo.Error())
	}
}

// TestIsReservedHubName_PatternNoLongerReserved pins the inverse of the old truth: _pattern has
// collapsed into _lyx (see fabricengine/junctionnames_test.go's TestIsReservedHubName for the full
// current table of what remains reserved -- _lyx, _board, _portals, _launchers), so a worktree slug
// named "_pattern" is no longer refused on reserved-name grounds.
// nil is passed for junctionNames rather than a fixture naming "_pattern": with pathspec empty the
// production call site supplies no junction names at all, and injecting "_pattern" through
// junctionNames would make the assertion trivially reserved again and prove nothing.
// IsReservedHubName only ranges over that parameter, so nil and an empty slice are interchangeable
// here; nil is the simpler literal.
func TestIsReservedHubName_PatternNoLongerReserved(t *testing.T) {
	t.Parallel()

	if got := fabricengine.IsReservedHubName("_pattern", nil); got {
		t.Errorf("IsReservedHubName(%q, nil) = %v; want false", "_pattern", got)
	}
}

// TestIsReservedHubName_RaddleNoLongerReserved pins the same convergence for _raddle: raddle has
// converged on an anchor-level `_lyx/raddle/` design with no hub-level presence, so the name is no
// longer reserved and a worktree slug named "_raddle" is accepted.
func TestIsReservedHubName_RaddleNoLongerReserved(t *testing.T) {
	t.Parallel()

	if got := fabricengine.IsReservedHubName("_raddle", nil); got {
		t.Errorf("IsReservedHubName(%q, nil) = %v; want false", "_raddle", got)
	}
}
