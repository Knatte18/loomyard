// add_taskpathleftover_integration_test.go covers Add meeting a directory at the task worktree path that is no worktree.
// Output git ignores, such as a build leaves after a pair's removal, is deleted; anything else blocks the pair.
//
// It shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAdd_TaskPathLeftover asserts Add deletes a directory at the task worktree path whose every file git ignores.
// Add then makes the pair there.
// A directory holding a file git does not ignore, or a `.git` entry, is refused with *ErrTaskPathTaken.
// Such a directory is left on disk.
func TestAdd_TaskPathLeftover(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const buildOutput = "src/App/obj/Debug/App.AssemblyInfo.cs"
	tests := []struct {
		name string
		slug string
		// files are the paths, relative to the task worktree path, written there before Add.
		files []string
		// tracked are the paths in files that are also committed, at the same relative path, in the acting worktree.
		tracked     []string
		wantDeleted bool
	}{
		{name: "OnlyIgnoredOutputIsDeleted", slug: "ignored-only", files: []string{buildOutput}, wantDeleted: true},
		{name: "EmptyDirectoryIsDeleted", slug: "empty-dir", wantDeleted: true},
		{name: "FileGitDoesNotIgnoreKeepsIt", slug: "unignored-file", files: []string{buildOutput, "notes.txt"}},
		{name: "NothingIgnoredKeepsIt", slug: "nothing-ignored", files: []string{"notes.txt"}},
		{name: "GitEntryKeepsIt", slug: "git-entry", files: []string{buildOutput, "src/App/obj/.git"}},
		{
			name: "TrackedFileKeepsIt", slug: "tracked-file",
			files:   []string{buildOutput, "src/App/obj/tracked.cs"},
			tracked: []string{"src/App/obj/tracked.cs"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			// Tracked paths are committed before the ignore rule, which git add would otherwise refuse them under.
			for _, rel := range tt.tracked {
				gitkit.CommitFile(t, l.WorktreePath(), rel, "tracked\n", "track "+rel)
			}
			gitkit.CommitFile(t, l.WorktreePath(), "src/.gitignore", "obj/\n", "ignore build output")

			dir := fabricengine.WorktreePath(l, tt.slug)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("make %s: %v", dir, err)
			}
			for _, rel := range tt.files {
				path := filepath.Join(dir, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("make the directory of %s: %v", path, err)
				}
				if err := os.WriteFile(path, []byte("left behind\n"), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}

			res, err := h.Topology.Add(l, tt.slug, fabricengine.AddOptions{SkipPush: true})

			deleted := fabricengine.Mutation{Kind: fabricengine.KindPathRemoved, Target: tt.slug, Detail: "recursive"}
			if !tt.wantDeleted {
				var taken *fabricengine.ErrTaskPathTaken
				if !errors.As(err, &taken) || taken.Path != dir {
					t.Fatalf("Add(%q) error = %v; want *ErrTaskPathTaken for %s", tt.slug, err, dir)
				}
				for _, rel := range tt.files {
					if _, statErr := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel))); statErr != nil {
						t.Errorf("%s is gone after a refused Add (stat error = %v); want it kept", rel, statErr)
					}
				}
				if slices.Contains(res.Mutations.Entries(), deleted) {
					t.Errorf("mutation record = %+v; want no removal of %s", res.Mutations.Entries(), dir)
				}
				return
			}
			if err != nil {
				t.Fatalf("Add(%q) error = %v; want the leftover deleted and the pair made", tt.slug, err)
			}
			if _, statErr := os.Lstat(filepath.Join(dir, ".git")); statErr != nil {
				t.Errorf("%s holds no .git entry after Add (stat error = %v); want the task worktree made there", dir, statErr)
			}
			if _, statErr := os.Lstat(filepath.Join(dir, filepath.FromSlash(buildOutput))); !os.IsNotExist(statErr) {
				t.Errorf("%s still on disk after Add (stat error = %v); want it deleted", buildOutput, statErr)
			}
			if !slices.Contains(res.Mutations.Entries(), deleted) {
				t.Errorf("mutation record = %+v; want it to hold %+v", res.Mutations.Entries(), deleted)
			}
		})
	}
}

// writeIgnoredLeftover commits an ignore rule for obj/ in h's acting worktree.
// It then writes one ignored build-output file at the task worktree path of slug and returns the file's path.
func writeIgnoredLeftover(t *testing.T, h *hubforge.Hub, slug string) string {
	t.Helper()
	gitkit.CommitFile(t, h.Location.WorktreePath(), "src/.gitignore", "obj/\n", "ignore build output")
	file := filepath.Join(fabricengine.WorktreePath(h.Location, slug), "src", "App", "obj", "App.dll")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("make the directory of %s: %v", file, err)
	}
	if err := os.WriteFile(file, []byte("left behind\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
	return file
}

// TestAdd_TaskPathLeftoverKeptWhenPreflightRefuses asserts a refused Add leaves a directory of ignored output.
func TestAdd_TaskPathLeftoverKeptWhenPreflightRefuses(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "refused-add"
	file := writeIgnoredLeftover(t, h, slug)

	// An adopt-only add over a pair with no branch is refused by the pre-flight, after the task path is judged.
	_, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{SkipPush: true, AdoptLocal: true, AdoptOnly: true})

	if !errors.Is(err, fabricengine.ErrPairNotLive) {
		t.Fatalf("Add(%q) error = %v; want a refusal matching ErrPairNotLive", slug, err)
	}
	if _, statErr := os.Lstat(file); statErr != nil {
		t.Errorf("%s is gone after a refused Add (stat error = %v); want it kept", file, statErr)
	}
}

// TestAdd_TaskPathLeftoverDeleteFails asserts a delete that fails returns an error wrapping ErrStrayPathKept.
// The directory stays on disk.
func TestAdd_TaskPathLeftoverDeleteFails(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "delete-fails"
	file := writeIgnoredLeftover(t, h, slug)
	fabricengine.SetLeftoverSeamForTest(h.Topology, func(container, target string) (bool, bool, error) {
		return false, false, errors.New("delete refused")
	}, nil)

	_, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{SkipPush: true})

	if !errors.Is(err, fabricengine.ErrStrayPathKept) {
		t.Fatalf("Add(%q) error = %v; want an error wrapping ErrStrayPathKept", slug, err)
	}
	if _, statErr := os.Lstat(file); statErr != nil {
		t.Errorf("%s is gone after a failed delete (stat error = %v); want it kept", file, statErr)
	}
}
