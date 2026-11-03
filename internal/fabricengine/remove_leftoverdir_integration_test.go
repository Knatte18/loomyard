// remove_leftoverdir_integration_test.go covers Remove finishing a pair worktree directory that `git worktree remove` deregistered but left on disk, on either side of the pair,
// or that a late writer refilled after the removal, and PairWorktreeStands never reading such a directory as a pair.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

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
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRemove_FinishesWorktreeDirGitLeftOnDisk asserts Remove deletes a worktree directory a late writer refilled with git-ignored build output after git's clean exit, and one git deregistered and failed to delete.
func TestRemove_FinishesWorktreeDirGitLeftOnDisk(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const lockedDirName = "locked"
	tests := []struct {
		name string
		slug string
		// sibling puts the sibling worktree under test, else the task worktree.
		sibling bool
		// readOnly commits a file under a directory made read-only in the worktree under test, so git deregisters it and fails to delete it;
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

// TestRemove_DeletesTaskWorktreeDirRefilledAfterItsRemoval asserts Remove deletes a directory a late writer refills at the task worktree's path once the task worktree's removal is over, and reports no StrayPath.
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

// TestRemove_KeepsAndReportsTaskWorktreeDirRefilledAfterItsRemovalWhenItCannotBeDeleted asserts Remove still succeeds when the directory a late writer refilled at the task worktree's path cannot be deleted, leaves it on disk and reports its path in StrayPath.
// It sets the process-global teardown hook, so it is not parallel.
func TestRemove_KeepsAndReportsTaskWorktreeDirRefilledAfterItsRemovalWhenItCannotBeDeleted(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Remove error = %v; want success, since the pair is gone", err)
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

// TestPairWorktreeStands_ReadsOnlyARegisteredWorktree asserts a registered task worktree reads as standing, while a plain directory at a pair's path and an absent path do not.
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
