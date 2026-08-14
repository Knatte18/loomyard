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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

// TestContainedWorktreeAdd_FailsClosedOnStagingLeafSwap reproduces R6's create-side observation escape:
// an observer that watches the container for the random staging parent to appear and plants a symlink at
// the predictable staging LEAF (target's base name) before git writes there, so git follows the link and
// writes the worktree OUTSIDE the container. R5's design placed the worktree via os.Root.Rename — which
// refuses a symlink at the rename DESTINATION but renames a symlink at the SOURCE's own final component
// as a link — so before R6 this produced an out-of-hub worktree with the target left a dangling symlink,
// all reported as a nil error (false success). The fail-closed containment checks R6 added must make
// containedWorktreeAdd refuse instead: never a nil error while the worktree escaped, never a dangling
// symlink at target, never staging debris left in the container, and the escaped worktree cleaned up.
//
// The planter reliably wins because git's subprocess spawn latency vastly exceeds a busy poll's, but the
// assertions do not depend on winning every attempt: the loop asserts the never-false-success invariant
// on every attempt and requires at least one attempt where the planter won and the add failed closed, so
// a run that never triggered the guarded path fails loudly rather than passing vacuously.
func TestContainedWorktreeAdd_FailsClosedOnStagingLeafSwap(t *testing.T) {
	t.Parallel()

	repoDir := newPlainWarpRepo(t)
	base := t.TempDir()
	container := filepath.Join(base, "container")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatalf("mkdir container: %v", err)
	}

	sawFailClosed := false
	deadline := time.Now().Add(20 * time.Second)
	for attempt := 0; attempt < 60 && !sawFailClosed && time.Now().Before(deadline); attempt++ {
		slug := fmt.Sprintf("slug%d", attempt)
		target := filepath.Join(container, slug)
		if err := os.RemoveAll(outside); err != nil {
			t.Fatalf("reset outside: %v", err)
		}
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatalf("mkdir outside: %v", err)
		}

		// Planter: watch the container for the staging parent, then plant <parent>/<slug> -> outside so
		// git follows it. It plants exactly ONCE and then idles until stop — the single-plant shape the
		// documented repro uses (an observer racing one add), rather than a relentless re-planter that
		// would fight containedWorktreeAdd's own cleanup and leave staging debris that is a test artifact,
		// not a fabric integrity failure.
		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				entries, _ := os.ReadDir(container)
				for _, e := range entries {
					if strings.HasPrefix(e.Name(), ".fabric-wt-staging-") {
						leaf := filepath.Join(container, e.Name(), slug)
						if os.Symlink(outside, leaf) == nil {
							<-stop // planted once; stop touching the staging area so cleanup can proceed
							return
						}
					}
				}
			}
		}()

		err := containedWorktreeAdd(repoDir, container, target, func(worktreePath string) []string {
			return []string{"worktree", "add", "-b", slug, worktreePath}
		})
		close(stop)
		wg.Wait()

		if err == nil {
			// A nil error is only legitimate when target is a real directory and nothing escaped.
			info, statErr := os.Lstat(target)
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				t.Fatalf("attempt %d: false success — nil error but target is not a real directory (%v)", attempt, info)
			}
			if entries, _ := os.ReadDir(outside); len(entries) != 0 {
				t.Fatalf("attempt %d: false success — nil error but a worktree escaped to %s: %v", attempt, outside, entries)
			}
			continue
		}

		// The planter won and the add failed closed. Assert nothing escaped and the container is clean.
		sawFailClosed = true
		if info, statErr := os.Lstat(target); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("attempt %d: fail-closed left target a dangling symlink pointing outside the container", attempt)
		}
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Fatalf("attempt %d: fail-closed did not clean up the escaped worktree in %s: %v", attempt, outside, entries)
		}
		leftovers, _ := os.ReadDir(container)
		for _, e := range leftovers {
			if strings.HasPrefix(e.Name(), ".fabric-wt-staging-") {
				t.Fatalf("attempt %d: fail-closed left staging debris in the container: %q", attempt, e.Name())
			}
		}
	}

	if !sawFailClosed {
		t.Fatal("the planter never won the staging-leaf race, so the fail-closed path was never exercised; this test is not proving the fix (raise the attempt budget or check git spawn latency)")
	}
}
