//go:build integration

// create_containment_toctou_integration_test.go guards the act-time containment binding that closed
// R5's CREATE-side symlink-directed-write escape: containedWorktreeAdd must never let `git worktree
// add` write a worktree OUTSIDE its container through a symlink standing at the target path, must still
// place a legitimate worktree at target with its branch intact, and must name git's internal admin
// directory after the slug rather than the staging token.
//
// It is the create-side twin of destroy_toctou_test.go's delete-side guard. The original defect
// (reproduced live in R5) escaped because git resolves its destination-path argument itself and follows
// a symlink there, carrying a whole worktree out of the hub during Add's wide check-then-act window.
// The fix makes git's WRITE target only a collision-free staging path inside the container and moves the
// result into place with os.Root.Rename, which refuses to follow a symlink at target — so pinning that
// refusal with a symlink standing at target at act time is exactly what a regression back to a nominal
// `git worktree add target` would fail. Because containedWorktreeAdd spawns git, this test is
// integration-tagged (the untagged tier spawns nothing).

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"
)

// TestContainedWorktreeAdd_RefusesSymlinkedTarget plants a symlink at the target path pointing OUTSIDE
// the container and asserts containedWorktreeAdd never writes a worktree through it: the outside
// directory stays empty and the call fails at placement rather than following the link.
func TestContainedWorktreeAdd_RefusesSymlinkedTarget(t *testing.T) {
	t.Parallel()

	repoDir := newPlainWarpRepo(t)
	base := t.TempDir()
	container := filepath.Join(base, "container")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatalf("mkdir container: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	// The target path is a symlink escaping the container — the live-at-act-time shape a toggle race
	// leaves behind exactly when git resolves its destination argument.
	target := filepath.Join(container, "escaped")
	if err := os.Symlink(outside, target); err != nil {
		t.Fatalf("plant escaping target symlink: %v", err)
	}

	err := containedWorktreeAdd(repoDir, container, target, func(worktreePath string) []string {
		return []string{"worktree", "add", "-b", "feature", worktreePath}
	})
	if err == nil {
		t.Fatalf("containedWorktreeAdd must refuse to place a worktree through a symlinked target, got nil error")
	}

	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatalf("read outside dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("a worktree was written OUTSIDE the container through the escaping target symlink: %v", entries)
	}
}

// TestContainedWorktreeAdd_PlacesWorktreeAtTarget asserts the ordinary success path: the worktree lands
// at the nominal target as a real directory on the requested branch, and git's admin directory is named
// after the slug (the target base), never the internal staging token.
func TestContainedWorktreeAdd_PlacesWorktreeAtTarget(t *testing.T) {
	t.Parallel()

	repoDir := newPlainWarpRepo(t)
	base := t.TempDir()
	container := filepath.Join(base, "container")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatalf("mkdir container: %v", err)
	}
	target := filepath.Join(container, "myslug")

	err := containedWorktreeAdd(repoDir, container, target, func(worktreePath string) []string {
		return []string{"worktree", "add", "-b", "feature", worktreePath}
	})
	if err != nil {
		t.Fatalf("containedWorktreeAdd(%q): %v", target, err)
	}

	info, statErr := os.Lstat(target)
	if statErr != nil {
		t.Fatalf("target not created: %v", statErr)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		t.Fatalf("target is not a real directory: mode=%v", info.Mode())
	}

	// No staging debris survives in the container.
	entries, readErr := os.ReadDir(container)
	if readErr != nil {
		t.Fatalf("read container: %v", readErr)
	}
	for _, e := range entries {
		if e.Name() != "myslug" {
			t.Fatalf("unexpected leftover in container after placement: %q", e.Name())
		}
	}

	// git's admin directory is named after the slug, not the random staging token.
	if _, statErr := os.Stat(filepath.Join(repoDir, ".git", "worktrees", "myslug")); statErr != nil {
		t.Fatalf("git admin dir not named after the slug: %v", statErr)
	}
}
