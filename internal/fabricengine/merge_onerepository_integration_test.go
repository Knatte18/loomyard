// merge_onerepository_integration_test.go covers the fabric merge verbs in a one-repository hub.
// The hub is built by hubforge.NewOneRepoHub, so the receiving side's records sit on its code branch beside its code.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newOneRepositoryMergeHub returns a one-repository hub at "." whose prime tracks what the rows edit:
// the records file _lym/shared.txt, the code file code.txt and the anchor directories y/ and sub/.
func newOneRepositoryMergeHub(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.NewOneRepoHub(t, ".")
	prime := h.PrimeWorktree()
	gitkit.CommitFile(t, prime, "_lym/shared.txt", "base\n", "track the shared records file")
	gitkit.CommitFile(t, prime, "code.txt", "base\n", "track the code file")
	gitkit.CommitFile(t, prime, "y/.keep", "", "track y")
	gitkit.CommitFile(t, prime, "sub/.keep", "", "track sub")
	return h
}

// openOneRepositoryFabric opens the fabric handle on the worktree at path, fataling on error.
func openOneRepositoryFabric(t *testing.T, path string) *fabricengine.Fabric {
	t.Helper()

	loc, err := lymcwd.ResolveWorktree(path)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", path, err)
	}
	f, err := fabricengine.Open(loc)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	return f
}

// writeOneRepositoryFile writes content to the file rel under dir without committing it, fataling on error.
func writeOneRepositoryFile(t *testing.T, dir, rel, content string) {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the directory of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// requireRecordsHeldAt fails the test unless the worktree at dir holds the given records file content.
// The content must be in both index and disk.
// The worktree must also hold no task-only records file and show no change under its records paths.
func requireRecordsHeldAt(t *testing.T, dir string, want map[string]string) {
	t.Helper()

	for rel, content := range want {
		if got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))); err != nil || string(got) != content {
			t.Errorf("%s on disk = (%q, %v); want %q", rel, got, err, content)
		}
		if got := gitkit.Git(t, dir, "show", ":"+rel); got != strings.TrimSuffix(content, "\n") {
			t.Errorf("%s in the index = %q; want %q", rel, got, content)
		}
	}
	if status := gitkit.Git(t, dir, "status", "--porcelain", "--", "_lym", lymcwd.AnchorFileName); status != "" {
		t.Errorf("status of the records paths = %q; want none", status)
	}
	if tracked := gitkit.Git(t, dir, "ls-files", "--", "_lym/task-only.txt"); tracked != "" {
		t.Errorf("the index holds %s; want the task-only records file left out", tracked)
	}
	if _, err := os.Stat(filepath.Join(dir, "_lym", "task-only.txt")); !os.IsNotExist(err) {
		t.Errorf("the task-only records file is on disk (stat err %v); want it left out", err)
	}
}

func TestMerge_OneRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	for _, squash := range []bool{false, true} {
		name := "LocalMergeKeepsTheParentsRecords"
		if squash {
			name = "SquashMergeKeepsTheParentsRecords"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newOneRepositoryMergeHub(t)
			prime := h.PrimeWorktree()
			task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
			gitkit.CommitFile(t, task.Path, "_lym/shared.txt", "task\n", "task edit of the shared records")
			gitkit.CommitFile(t, task.Path, "_lym/task-only.txt", "task\n", "task-only records")
			gitkit.CommitFile(t, task.Path, "task-code.txt", "task\n", "task code")
			writeOneRepositoryFile(t, prime, "_lym/shared.txt", "parent\n")
			writeOneRepositoryFile(t, prime, "_lym/parent-only.txt", "parent\n")

			res, err := hubforge.OpenFabric(t, h).Merge(task.Branch, fabricengine.MergeOptions{Squash: squash})

			if err != nil || !res.Committed {
				t.Fatalf("Merge() = (committed %v, %v); want a committed merge", res.Committed, err)
			}
			if _, err := os.Stat(filepath.Join(prime, "task-code.txt")); err != nil {
				t.Errorf("the task's code file is missing after the merge: %v", err)
			}
			requireRecordsHeldAt(t, prime, map[string]string{"_lym/shared.txt": "parent\n", "_lym/parent-only.txt": "parent\n"})
		})
	}

	t.Run("MergeInKeepsTheTasksRecords", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryMergeHub(t)
		prime := h.PrimeWorktree()
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
		gitkit.CommitFile(t, prime, "code.txt", "parent\n", "parent code")
		gitkit.CommitFile(t, prime, "_lym/shared.txt", "parent\n", "parent edit of the shared records")
		gitkit.CommitFile(t, prime, "_lym/task-only.txt", "parent\n", "parent records file")
		writeOneRepositoryFile(t, task.Path, "_lym/shared.txt", "task\n")
		writeOneRepositoryFile(t, task.Path, "_lym/task-new.txt", "task\n")

		res, err := openOneRepositoryFabric(t, task.Path).MergeIn(gitkit.CurrentBranch(t, prime))

		if err != nil || !res.Committed {
			t.Fatalf("MergeIn() = (committed %v, %v); want a committed merge", res.Committed, err)
		}
		if got, err := os.ReadFile(filepath.Join(task.Path, "code.txt")); err != nil || string(got) != "parent\n" {
			t.Errorf("code.txt = (%q, %v); want the parent's code", got, err)
		}
		requireRecordsHeldAt(t, task.Path, map[string]string{"_lym/shared.txt": "task\n", "_lym/task-new.txt": "task\n"})
	})

	t.Run("FastForwardSourceMakesAMergeCommit", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryMergeHub(t)
		prime := h.PrimeWorktree()
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
		gitkit.CommitFile(t, task.Path, "task-code.txt", "task\n", "task code")

		res, err := hubforge.OpenFabric(t, h).Merge(task.Branch, fabricengine.MergeOptions{})

		if err != nil || !res.Committed {
			t.Fatalf("Merge() = (committed %v, %v); want a committed merge", res.Committed, err)
		}
		head := gitkit.RevParse(t, prime, "HEAD")
		if second := gitkit.RevParse(t, prime, "HEAD^2"); second != gitkit.RevParse(t, task.Path, "HEAD") {
			t.Errorf("second parent = %s; want the task's tip", second)
		}
		if message := gitkit.Git(t, prime, "log", "-1", "--format=%B"); !strings.Contains(message, fabricengine.MergeTrailerKey+": ") {
			t.Errorf("commit message = %q; want the %s trailer", message, fabricengine.MergeTrailerKey)
		}
		if isMerge, err := fabricengine.IsFabricMergeCommit(prime, head); err != nil || !isMerge {
			t.Errorf("IsFabricMergeCommit(%s) = (%v, %v); want true", head, isMerge, err)
		}
	})

	t.Run("AnchorChangeMergedIntoTheParentChangesNoCodePath", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryMergeHub(t)
		prime := h.PrimeWorktree()
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
		c, err := beginAnchorChange(t, h, "task", "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		completeAnchorChange(t, c)

		if _, err := hubforge.OpenFabric(t, h).Merge(task.Branch, fabricengine.MergeOptions{}); err != nil {
			t.Fatalf("Merge() error = %v; want the anchor change merged", err)
		}

		if changed := gitkit.Git(t, prime, "diff", "--name-only", "HEAD^1", "HEAD"); changed != "" {
			t.Errorf("the merge changes %q against the parent; want no path", changed)
		}
		if marker := gitkit.Git(t, prime, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "." {
			t.Errorf("parent's marker = %q; want . unchanged", marker)
		}
	})

	t.Run("TaskAddedAtAnotherAnchorMergesBack", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryMergeHub(t)
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true, Anchor: "y"})

		if _, err := hubforge.OpenFabric(t, h).Merge(task.Branch, fabricengine.MergeOptions{}); err != nil {
			t.Fatalf("Merge() error = %v; want the task merged without the marker refusal", err)
		}
	})

	t.Run("MergeInAfterABackMergeOfATaskAtAnotherAnchor", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryMergeHub(t)
		prime := h.PrimeWorktree()
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true, Anchor: "y"})
		if _, err := hubforge.OpenFabric(t, h).Merge(task.Branch, fabricengine.MergeOptions{}); err != nil {
			t.Fatalf("Merge() error = %v; want the task merged back", err)
		}

		res, err := openOneRepositoryFabric(t, task.Path).MergeIn(gitkit.CurrentBranch(t, prime))

		if err != nil || !res.Committed {
			t.Fatalf("MergeIn() = (committed %v, %v); want the merge-in to pass", res.Committed, err)
		}
		if marker := gitkit.Git(t, task.Path, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "y" {
			t.Errorf("task's marker = %q; want y unchanged", marker)
		}
	})

	t.Run("SourceMarkerEditRefuses", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryMergeHub(t)
		prime := h.PrimeWorktree()
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
		gitkit.CommitFile(t, task.Path, lymcwd.AnchorFileName, "sub\n", "edit the marker by hand")
		before := gitkit.RevParse(t, prime, "HEAD")
		f := hubforge.OpenFabric(t, h)

		_, err := f.Merge(task.Branch, fabricengine.MergeOptions{})

		var refusal *fabricengine.ErrMarkerChange
		if !errors.As(err, &refusal) || !strings.Contains(err.Error(), lymcwd.AnchorFileName) {
			t.Fatalf("Merge() error = %v; want an *ErrMarkerChange naming %s", err, lymcwd.AnchorFileName)
		}
		if got := gitkit.RevParse(t, prime, "HEAD"); got != before {
			t.Errorf("HEAD = %s; want %s unchanged by the refusal", got, before)
		}
		if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
			t.Errorf("merge record exists = (%v, %v); want none after the refusal", exists, err)
		}
	})

	t.Run("RevertedMarkerEditMerges", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryMergeHub(t)
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
		gitkit.CommitFile(t, task.Path, lymcwd.AnchorFileName, "sub\n", "edit the marker by hand")
		gitkit.CommitFile(t, task.Path, lymcwd.AnchorFileName, ".\n", "restore the marker")

		if _, err := hubforge.OpenFabric(t, h).Merge(task.Branch, fabricengine.MergeOptions{}); err != nil {
			t.Fatalf("Merge() error = %v; want the merge to pass once the marker is restored", err)
		}
	})

	t.Run("RecordsWriteBetweenStepTwoAndThreeRefuses", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryMergeHub(t)
		prime := h.PrimeWorktree()
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
		gitkit.CommitFile(t, task.Path, "_lym/task-only.txt", "task\n", "task-only records")
		gitkit.CommitFile(t, task.Path, "task-code.txt", "task\n", "task code")
		f := hubforge.OpenFabric(t, h)
		f.SetAfterMergeStartForTest(func() error {
			writeOneRepositoryFile(t, prime, "_lym/task-only.txt", "tampered\n")
			return nil
		})

		_, err := f.Merge(task.Branch, fabricengine.MergeOptions{})

		var refusal *fabricengine.MergeGuardError
		if !errors.As(err, &refusal) || !refusal.WorktreeDirty() || !slices.Contains(refusal.DirtyPaths, "_lym/task-only.txt") {
			t.Fatalf("Merge() error = %v; want the dirty refusal naming _lym/task-only.txt", err)
		}
		if !strings.Contains(err.Error(), "lym fabric merge --abort") {
			t.Errorf("refusal = %q; want it to name `lym fabric merge --abort`", err)
		}
		if st, found, err := fabricengine.LoadMergeStateForTest(f); err != nil || !found || !st.RecordsUnsettled {
			t.Errorf("merge record = (%+v, %v, %v); want it standing with RecordsUnsettled set", st, found, err)
		}
	})

	t.Run("MergeOverAParkedMergeRefusesAndKeepsItsResolution", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryMergeHub(t)
		prime := h.PrimeWorktree()
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
		gitkit.CommitFile(t, task.Path, "code.txt", "task\n", "task edit of the code file")
		gitkit.CommitFile(t, prime, "code.txt", "parent\n", "parent edit of the code file")
		parent := gitkit.CurrentBranch(t, prime)
		f := openOneRepositoryFabric(t, task.Path)
		res, err := f.MergeIn(parent)
		if err != nil || !slices.Equal(res.Conflicts, []string{"code.txt"}) {
			t.Fatalf("MergeIn() = (conflicts %v, %v); want the merge parked on code.txt", res.Conflicts, err)
		}
		writeOneRepositoryFile(t, task.Path, "code.txt", "resolved\n")
		gitkit.Git(t, task.Path, "add", "--", "code.txt")

		_, err = f.MergeIn(parent)

		var refusal *fabricengine.MergeGuardError
		if !errors.As(err, &refusal) || !refusal.AlreadyInProgress() {
			t.Fatalf("MergeIn() error = %v; want the already-in-progress refusal", err)
		}
		if got, err := os.ReadFile(filepath.Join(task.Path, "code.txt")); err != nil || string(got) != "resolved\n" {
			t.Errorf("code.txt on disk = (%q, %v); want the resolution kept", got, err)
		}
		if got := gitkit.Git(t, task.Path, "show", ":code.txt"); got != "resolved" {
			t.Errorf("code.txt in the index = %q; want the resolution kept", got)
		}
	})

	for _, squash := range []bool{false, true} {
		name := "MergeOverAParkedMergeRefusesBeforeAnyRecordsCommit"
		if squash {
			name = "SquashMergeOverAParkedMergeRefusesBeforeAnyRecordsCommit"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newOneRepositoryMergeHub(t)
			prime := h.PrimeWorktree()
			task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
			gitkit.CommitFile(t, task.Path, "code.txt", "task\n", "task edit of the code file")
			gitkit.CommitFile(t, prime, "code.txt", "parent\n", "parent edit of the code file")
			f := hubforge.OpenFabric(t, h)
			res, err := f.MergeIn(task.Branch)
			if err != nil || !slices.Equal(res.Conflicts, []string{"code.txt"}) {
				t.Fatalf("MergeIn() = (conflicts %v, %v); want the receiving side's merge parked on code.txt", res.Conflicts, err)
			}
			writeOneRepositoryFile(t, prime, "code.txt", "resolved\n")
			gitkit.Git(t, prime, "add", "--", "code.txt")
			writeOneRepositoryFile(t, prime, "_lym/late.txt", "late\n")
			before := gitkit.RevParse(t, prime, "HEAD")

			_, err = f.Merge(task.Branch, fabricengine.MergeOptions{Squash: squash})

			var refusal *fabricengine.MergeGuardError
			if !errors.As(err, &refusal) || !refusal.AlreadyInProgress() {
				t.Fatalf("Merge() error = %v; want the already-in-progress refusal", err)
			}
			if got := gitkit.RevParse(t, prime, "HEAD"); got != before {
				t.Errorf("HEAD = %s; want %s unchanged, with no records commit made", got, before)
			}
			if got, err := os.ReadFile(filepath.Join(prime, "code.txt")); err != nil || string(got) != "resolved\n" {
				t.Errorf("code.txt on disk = (%q, %v); want the resolution kept", got, err)
			}
			if got := gitkit.Git(t, prime, "show", ":code.txt"); got != "resolved" {
				t.Errorf("code.txt in the index = %q; want the resolution kept", got)
			}
		})
	}
}
