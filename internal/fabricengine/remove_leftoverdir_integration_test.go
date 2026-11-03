// remove_leftoverdir_integration_test.go covers Remove finishing a pair worktree directory that `git worktree remove` deregistered but left on disk, on either side of the pair.
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
		// local builds the pair with AddLocal on a dead-origin hub, so its sibling goes through the local-only teardown.
		local bool
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
		{name: "a local pair's sibling worktree refilled after git's clean exit is removed", slug: "refilled-local", local: true, sibling: true, wantStep: fabricengine.RemoveStepSiblingWorktree},
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

			var h *hubforge.Hub
			if tt.local {
				h = addLocalPairOnDeadOrigin(t, tt.slug)
			} else {
				h = hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
				hubforge.AddPairWith(t, h, tt.slug, fabricengine.AddOptions{})
			}
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
