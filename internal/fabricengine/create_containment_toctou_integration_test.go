// create_containment_toctou_integration_test.go tests that containedWorktreeAdd places a git worktree at its target.
// It also covers a symlink planted at the target or at the staging leaf, where the call must fail closed.
// Each test requires tiergate tier 1, because containedWorktreeAdd spawns git.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestContainedWorktreeAdd_RefusesSymlinkedTarget plants a symlink at the target path, pointing outside the container.
// It asserts that containedWorktreeAdd fails at placement and writes no worktree through the link:
// the outside directory stays empty.
func TestContainedWorktreeAdd_RefusesSymlinkedTarget(t *testing.T) {
	tiergate.Require(t, 1)
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

	// The target path is a symlink that escapes the container.
	// A toggle race leaves this state while git resolves its destination argument.
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

// TestContainedWorktreeAdd_PlacesWorktreeAtTarget asserts the success path.
// The worktree lands at target as a real directory on the requested branch.
// git's admin directory is named after the slug, the base name of target, not the staging token.
func TestContainedWorktreeAdd_PlacesWorktreeAtTarget(t *testing.T) {
	tiergate.Require(t, 1)
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

// TestContainedWorktreeAdd_FailsClosedOnStagingLeafSwap checks the fail-closed path of containedWorktreeAdd.
// An observer waits for the random staging parent to appear, then plants a symlink at the predictable staging leaf.
// The leaf is the target's base name, so git follows the link and writes the worktree outside the container.
// The add must fail closed: no nil error while the worktree escaped, and no dangling symlink at target.
// The container must hold no staging debris, and the escaped worktree must be cleaned up.
//
// The planter reliably wins because git's subprocess spawn latency exceeds a busy poll's,
// but the assertions do not depend on winning every attempt.
// The loop asserts the never-false-success invariant on every attempt.
// It also requires at least one attempt where the planter won and the add failed closed,
// so a run that never triggered the guarded path fails loudly rather than passing vacuously.
func TestContainedWorktreeAdd_FailsClosedOnStagingLeafSwap(t *testing.T) {
	tiergate.Require(t, 1)
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

		// Planter: watch the container for the staging parent, then plant <parent>/<slug> -> outside so git follows it.
		// It plants exactly once and then idles until stop, so one observer races one add.
		// A relentless re-planter would fight containedWorktreeAdd's own cleanup.
		// The staging debris it leaves would be a test artifact, not a fabric integrity failure.
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

		// A non-nil error means the planter won and the add failed closed.
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
