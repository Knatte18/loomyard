// remove_leftoverdir_integration_test.go covers Remove finishing pair worktree directories left on disk.
// Either side of the pair can hold one, deregistered by git or refilled by a late writer after the removal.
// PairWorktreeStands never reads such a directory as a pair.
//
// It shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRemove_FinishesWorktreeDirGitLeftOnDisk asserts Remove finishes off a worktree directory left on disk.
// Rows set up a read-only directory git cannot delete, or a directory a late writer refills after git's clean exit.
func TestRemove_FinishesWorktreeDirGitLeftOnDisk(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const lockedDirName = "locked"
	tests := []struct {
		name string
		slug string
		// sibling puts the sibling worktree under test, else the task worktree.
		sibling bool
		// readOnly commits a file under a directory made read-only in the worktree under test.
		// Git then deregisters it and fails to delete it;
		// otherwise a late writer refills it after git's clean exit.
		readOnly bool
		// locked locks the worktree under test, so git refuses to remove it and it stays registered.
		locked bool
		// removeErr is what the seam's removal of the directory returns; nil keeps the removal working.
		removeErr error
		// wantErrContains is text the error of a Remove that must fail holds; empty means Remove succeeds.
		wantErrContains string
		// wantStep is the step Remove reports for the worktree under test.
		wantStep string
	}{
		{name: "a task worktree refilled after git's clean exit is removed", slug: "refilled-task", wantStep: fabricengine.RemoveStepTaskWorktree},
		{name: "a sibling worktree refilled after git's clean exit is removed", slug: "refilled-sibling", sibling: true, wantStep: fabricengine.RemoveStepSiblingWorktree},
		{name: "a sibling worktree git deregistered and failed to delete is removed", slug: "undeleted-sibling", sibling: true, readOnly: true, wantStep: fabricengine.RemoveStepSiblingWorktree},
		{name: "a refilled task worktree whose removal fails is returned and kept", slug: "refilled-kept", removeErr: fs.ErrPermission, wantErrContains: "delete it by hand; a pair that remains is then removed by running remove again"},
		{name: "a sibling worktree git deregistered whose removal fails is returned and kept", slug: "undeleted-kept", sibling: true, readOnly: true, removeErr: fs.ErrPermission, wantErrContains: "deregistered it and failed"},
		{name: "a locked sibling worktree git refused to remove is returned and kept", slug: "locked-sibling", sibling: true, locked: true, wantErrContains: "weft worktree remains"},
	}

	for _, tt := range tests {
		// A read-only directory stops git's delete only off Windows, so Windows generates no subtest for such a row.
		if tt.readOnly && runtime.GOOS == "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.readOnly && os.Geteuid() == 0 {
				t.Skip("a read-only directory stops git's delete only for a non-root user")
			}

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPairWith(t, h, tt.slug, fabricengine.AddOptions{})
			l := h.Location
			dir := fabricengine.WorktreePath(l, tt.slug)
			if tt.sibling {
				dir = fabricengine.RecordsWorktreePath(l, tt.slug)
			}
			lockedDir := filepath.Join(dir, lockedDirName)
			if tt.readOnly {
				gitkit.CommitFile(t, dir, lockedDirName+"/held.txt", "committed\n", "add a file under the directory made read-only")
				if err := os.Chmod(lockedDir, 0o555); err != nil {
					t.Fatalf("make %s read-only: %v", lockedDir, err)
				}
				t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o755) })
			}
			if tt.locked {
				gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "worktree", "lock", dir)
			}

			refilled := false
			fabricengine.SetLeftoverRefillForTest(h.Topology, func(target string) {
				if target != dir {
					return
				}
				refilled = true
				output := filepath.Join(dir, "src", "App", "obj", "project.assets.json")
				if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
					t.Errorf("refill %s: %v", dir, err)
					return
				}
				if err := os.WriteFile(output, []byte("{}\n"), 0o644); err != nil {
					t.Errorf("refill %s: %v", dir, err)
				}
			})
			var remove func(container, target string) (bool, bool, error)
			switch {
			case tt.removeErr != nil:
				remove = func(container, target string) (bool, bool, error) {
					return false, false, &os.PathError{Op: "unlinkat", Path: target, Err: tt.removeErr}
				}
			case tt.readOnly:
				remove = func(container, target string) (bool, bool, error) {
					if err := os.Chmod(lockedDir, 0o755); err != nil {
						return false, false, err
					}
					return true, true, os.RemoveAll(target)
				}
			}
			fabricengine.SetLeftoverSeamForTest(h.Topology, remove, nil)

			res, err := h.Topology.Remove(l, tt.slug, false, false)

			rel, relErr := filepath.Rel(l.HubPath, dir)
			if relErr != nil {
				t.Fatalf("relate %s to the hub: %v", dir, relErr)
			}
			want := fabricengine.Mutation{Kind: fabricengine.KindPathRemoved, Target: filepath.ToSlash(rel), Detail: "recursive"}

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("Remove error = nil; want a failure holding %q", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("Remove error = %q; want it to hold %q", err.Error(), tt.wantErrContains)
				}
				if tt.removeErr != nil && !errors.Is(err, tt.removeErr) {
					t.Errorf("Remove error = %v; want it to wrap %v", err, tt.removeErr)
				}
				if _, statErr := os.Lstat(dir); statErr != nil {
					t.Errorf("%s is gone after a failed finish (stat error = %v); want it on disk", dir, statErr)
				}
				if tt.locked {
					if _, statErr := os.Lstat(filepath.Join(dir, ".git")); statErr != nil {
						t.Errorf("%s lost its content though git refused to remove it (stat error = %v)", dir, statErr)
					}
				}
				if slices.Contains(res.Mutations.Entries(), want) {
					t.Errorf("mutation record = %+v; want no removal of %s", res.Mutations.Entries(), dir)
				}
				return
			}
			if err != nil {
				t.Fatalf("Remove error = %v; want the directory left on disk finished", err)
			}
			if !tt.readOnly && !refilled {
				t.Fatalf("no late writer refilled %s; want the refill to run after git's clean exit", dir)
			}
			if _, statErr := os.Lstat(dir); !os.IsNotExist(statErr) {
				t.Errorf("%s still on disk after Remove (stat error = %v)", dir, statErr)
			}
			if !slices.Contains(res.Mutations.Entries(), want) {
				t.Errorf("mutation record = %+v; want it to hold %+v", res.Mutations.Entries(), want)
			}
			if !slices.Contains(res.Steps, tt.wantStep) {
				t.Errorf("Steps = %v; want it to hold %q", res.Steps, tt.wantStep)
			}
		})
	}
}

// TestRemove_DeletesTaskWorktreeDirRefilledAfterItsRemoval asserts Remove deletes the refilled directory.
// The refill at the task worktree's path lands once that removal is over, and Remove reports no StrayPath.
// It sets the process-global teardown hook, so it is not parallel.
func TestRemove_DeletesTaskWorktreeDirRefilledAfterItsRemoval(t *testing.T) {
	tiergate.Require(t, 1)

	const slug = "refilled-late"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	l := h.Location
	dir := fabricengine.WorktreePath(l, slug)
	refilled := false
	fabricengine.SetTeardownHooksForTest(t, nil, func() { refilled = refillWithLockFile(t, dir) })

	res, err := h.Topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove error = %v; want the refilled directory deleted", err)
	}
	if !refilled {
		t.Fatalf("no late writer refilled %s; want the refill after the teardown", dir)
	}
	if _, statErr := os.Lstat(dir); !os.IsNotExist(statErr) {
		t.Errorf("%s still on disk after Remove (stat error = %v)", dir, statErr)
	}
	if res.StrayPath != "" {
		t.Errorf("StrayPath = %q; want empty once the refilled directory is deleted", res.StrayPath)
	}
	rel, relErr := filepath.Rel(l.HubPath, dir)
	if relErr != nil {
		t.Fatalf("relate %s to the hub: %v", dir, relErr)
	}
	want := fabricengine.Mutation{Kind: fabricengine.KindPathRemoved, Target: filepath.ToSlash(rel), Detail: "recursive"}
	if !slices.Contains(res.Mutations.Entries(), want) {
		t.Errorf("mutation record = %+v; want it to hold %+v", res.Mutations.Entries(), want)
	}
}

// TestRemove_FailsOnTaskWorktreeDirRefilledAfterItsRemovalThatCannotBeDeleted asserts that Remove keeps the directory:
// a late writer refills the task worktree's path, and the directory cannot be deleted.
// Remove then returns an error wrapping ErrStrayPathKept that names the directory and reports its path in StrayPath.
// It sets the process-global teardown hook, so it is not parallel.
func TestRemove_FailsOnTaskWorktreeDirRefilledAfterItsRemovalThatCannotBeDeleted(t *testing.T) {
	tiergate.Require(t, 1)

	const slug = "refilled-late-kept"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	l := h.Location
	dir := fabricengine.WorktreePath(l, slug)
	refilled := false
	fabricengine.SetTeardownHooksForTest(t, nil, func() { refilled = refillWithLockFile(t, dir) })
	fabricengine.SetLeftoverSeamForTest(h.Topology,
		func(container, target string) (bool, bool, error) {
			return false, false, &os.PathError{Op: "unlinkat", Path: target, Err: fs.ErrPermission}
		},
		func(time.Duration) {})

	res, err := h.Topology.Remove(l, slug, false, false)
	if !errors.Is(err, fabricengine.ErrStrayPathKept) || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Remove error = %v; want it to wrap ErrStrayPathKept and the delete's failure", err)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("Remove error = %q; want it to name %s", err.Error(), dir)
	}
	if !refilled {
		t.Fatalf("no late writer refilled %s; want the refill after the teardown", dir)
	}
	if res.StrayPath != dir {
		t.Errorf("StrayPath = %q; want %q", res.StrayPath, dir)
	}
	if _, statErr := os.Lstat(dir); statErr != nil {
		t.Errorf("%s is gone after a failed delete (stat error = %v); want it on disk", dir, statErr)
	}
	rel, relErr := filepath.Rel(l.HubPath, dir)
	if relErr != nil {
		t.Fatalf("relate %s to the hub: %v", dir, relErr)
	}
	removed := fabricengine.Mutation{Kind: fabricengine.KindPathRemoved, Target: filepath.ToSlash(rel), Detail: "recursive"}
	if slices.Contains(res.Mutations.Entries(), removed) {
		t.Errorf("mutation record = %+v; want no removal of %s", res.Mutations.Entries(), dir)
	}
}

// TestRemove_ReturnsTheRemoteStepFailureOverAKeptDirectory asserts that Remove returns the remote failure:
// when a remote step and a kept refilled directory both fail, and it still reports the directory in StrayPath.
// It sets process-global hooks, so it is not parallel.
func TestRemove_ReturnsTheRemoteStepFailureOverAKeptDirectory(t *testing.T) {
	tiergate.Require(t, 1)

	const slug = "refilled-late-remote"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	l := h.Location
	dir := fabricengine.WorktreePath(l, slug)
	gitkit.CommitFile(t, h.PairRecordsSibling(slug), "_lym/record.txt", "run record\n", "record")
	fabricengine.SetTeardownHooksForTest(t, nil, func() { refillWithLockFile(t, dir) })
	fabricengine.SetLeftoverSeamForTest(h.Topology,
		func(container, target string) (bool, bool, error) {
			return false, false, &os.PathError{Op: "unlinkat", Path: target, Err: fs.ErrPermission}
		},
		func(time.Duration) {})
	fabricengine.SetArchiveSleepForTest(h.Topology, func(time.Duration) {})
	fabricengine.SetArchiveTagPushHookForTest(t, func(tagRef string) error {
		return &gitexec.GitError{Args: []string{"push"}, ExitCode: 1, Stderr: " ! [remote rejected] " + tagRef + " -> " + tagRef + " (pre-receive hook declined)\n"}
	})

	res, err := h.Topology.Remove(l, slug, false, false)
	if err == nil {
		t.Fatal("Remove error = nil; want the remote step's failure")
	}
	if errors.Is(err, fabricengine.ErrStrayPathKept) {
		t.Errorf("Remove error = %v; want the remote step's failure, not the kept directory's", err)
	}
	if res.StrayPath != dir {
		t.Errorf("StrayPath = %q; want %q", res.StrayPath, dir)
	}
}

// refillWithLockFile plays a late writer: it writes a lock file under dir, recreating dir, and reports whether it did.
func refillWithLockFile(t *testing.T, dir string) bool {
	t.Helper()

	lockFile := filepath.Join(dir, ".lym", "reed.lock")
	if err := os.MkdirAll(filepath.Dir(lockFile), 0o755); err != nil {
		t.Errorf("refill %s: %v", dir, err)
		return false
	}
	if err := os.WriteFile(lockFile, nil, 0o644); err != nil {
		t.Errorf("refill %s: %v", dir, err)
		return false
	}
	return true
}

// TestRemoveStrayTaskPath asserts how Remove treats what stands at a pair's task worktree path:
// a plain directory is deleted, and an absent path is no error.
// A registered worktree, a directory holding a .git entry, an invalid slug and the prime's slug are refused and kept.
// A failed delete wraps ErrStrayPathKept and keeps the directory.
func TestRemoveStrayTaskPath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// slug is the slug asked for; empty means the default stray slug.
		slug string
		// primeSlug asks for the prime worktree's own name instead of slug.
		primeSlug bool
		// plant prepares the hub for the slug asked for; nil plants nothing.
		plant func(t *testing.T, h *hubforge.Hub, slug string)
		// removeErr is what the seam's removal returns; nil keeps the removal working.
		removeErr   error
		wantRemoved bool
		// wantErrIs is a sentinel the error must wrap, and wantErrContains text it must hold; both empty means success.
		wantErrIs       error
		wantErrContains string
		// wantGone says the path is absent afterwards.
		wantGone bool
	}{
		{
			name:        "PlainDirectoryWithBuildOutputIsDeleted",
			plant:       plantStrayDir("obj"),
			wantRemoved: true,
			wantGone:    true,
		},
		{
			name:     "AbsentPathIsNoError",
			wantGone: true,
		},
		{
			name: "RegisteredWorktreeIsRefused",
			plant: func(t *testing.T, h *hubforge.Hub, slug string) {
				hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
			},
			wantErrContains: "is a registered worktree",
		},
		{
			name:            "DirectoryHoldingGitIsRefused",
			plant:           plantStrayDir(".git"),
			wantErrContains: "holds a .git entry",
		},
		{
			name:      "FailedDeleteKeepsTheDirectory",
			plant:     plantStrayDir("obj"),
			removeErr: fs.ErrPermission,
			wantErrIs: fabricengine.ErrStrayPathKept,
		},
		{
			name:            "InvalidSlugIsRefusedAndNothingIsDeleted",
			slug:            "nested/slug",
			plant:           plantStrayDir("obj"),
			wantErrContains: "invalid slug",
		},
		{
			name:            "PrimeSlugIsRefusedAndThePrimeIsKept",
			primeSlug:       true,
			wantErrContains: "prime worktree",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			slug := "stray-left"
			if tt.slug != "" {
				slug = tt.slug
			}
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			if tt.primeSlug {
				primeName, err := fabricengine.PrimeName(h.Location)
				if err != nil {
					t.Fatalf("read the prime worktree's name: %v", err)
				}
				slug = primeName
			}
			if tt.plant != nil {
				tt.plant(t, h, slug)
			}
			if tt.removeErr != nil {
				fabricengine.SetLeftoverSeamForTest(h.Topology,
					func(container, target string) (bool, bool, error) {
						return false, false, &os.PathError{Op: "unlinkat", Path: target, Err: tt.removeErr}
					},
					func(time.Duration) {})
			}
			dir := fabricengine.WorktreePath(h.Location, slug)

			removed, err := h.Topology.RemoveStrayTaskPath(h.Location, slug)

			switch {
			case tt.wantErrIs != nil:
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("RemoveStrayTaskPath error = %v; want it to wrap %v", err, tt.wantErrIs)
				}
			case tt.wantErrContains != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("RemoveStrayTaskPath error = %v; want it to hold %q", err, tt.wantErrContains)
				}
			case err != nil:
				t.Errorf("RemoveStrayTaskPath error = %v; want nil", err)
			}
			if removed != tt.wantRemoved {
				t.Errorf("RemoveStrayTaskPath removed = %v; want %v", removed, tt.wantRemoved)
			}
			_, statErr := os.Lstat(dir)
			if gone := os.IsNotExist(statErr); gone != tt.wantGone {
				t.Errorf("%s gone = %v (stat error = %v); want %v", dir, gone, statErr, tt.wantGone)
			}
		})
	}
}

// plantStrayDir returns a plant that creates a plain directory at the slug's task worktree path.
// The directory holds one empty entry, named child.
func plantStrayDir(child string) func(t *testing.T, h *hubforge.Hub, slug string) {
	return func(t *testing.T, h *hubforge.Hub, slug string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(fabricengine.WorktreePath(h.Location, slug), child), 0o755); err != nil {
			t.Fatalf("plant a stray directory for %s: %v", slug, err)
		}
	}
}

// TestPairWorktreeStands_ReadsOnlyARegisteredWorktree asserts that a registered task worktree reads as standing:
// a plain directory at a pair's path and an absent path do not.
func TestPairWorktreeStands_ReadsOnlyARegisteredWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "standing", fabricengine.AddOptions{})
	l := h.Location
	if err := os.MkdirAll(filepath.Join(fabricengine.WorktreePath(l, "stray"), "obj"), 0o755); err != nil {
		t.Fatalf("create a plain directory at a pair's path: %v", err)
	}

	tests := []struct {
		name string
		slug string
		want bool
	}{
		{"RegisteredWorktree", "standing", true},
		{"PlainDirectory", "stray", false},
		{"AbsentPath", "absent", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fabricengine.PairWorktreeStands(l, tt.slug); got != tt.want {
				t.Errorf("PairWorktreeStands(%q) = %v; want %v", tt.slug, got, tt.want)
			}
		})
	}
}
