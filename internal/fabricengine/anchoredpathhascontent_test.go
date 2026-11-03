// anchoredpathhascontent_test.go covers AnchoredPathHasContent over a real records worktree:
// which directory states make git accept the path as a commit pathspec.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAnchoredPathHasContent asserts the probe is true only for a directory that git would accept as a commit pathspec:
// one holding a tracked file, even when deleted on disk, or an untracked file that is not ignored.
func TestAnchoredPathHasContent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	tests := []struct {
		name  string
		setup func(t *testing.T, weft string)
		want  bool
	}{
		{"TrackedFileDeletedOnDisk", func(t *testing.T, weft string) {
			gitkit.CommitFile(t, weft, "notes/halt.md", "halt\n", "track a note")
			if err := os.Remove(filepath.Join(weft, "notes", "halt.md")); err != nil {
				t.Fatalf("delete the tracked note: %v", err)
			}
		}, true},
		{"UntrackedFile", func(t *testing.T, weft string) {
			writeProbeFile(t, filepath.Join(weft, "notes", "new.md"))
		}, true},
		{"IgnoredOnlyDirectory", func(t *testing.T, weft string) {
			gitkit.CommitFile(t, weft, ".gitignore", "notes/\n", "ignore the notes directory")
			writeProbeFile(t, filepath.Join(weft, "notes", "ignored.md"))
		}, false},
		{"EmptyDirectory", func(t *testing.T, weft string) {
			if err := os.MkdirAll(filepath.Join(weft, "notes"), 0o755); err != nil {
				t.Fatalf("create the empty directory: %v", err)
			}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "task-slug", AnchorRel: "."}
			weft := fabricengine.RecordsWorktree(loc)
			if err := os.MkdirAll(weft, 0o755); err != nil {
				t.Fatalf("create the records worktree: %v", err)
			}
			gitkit.MustRun(t, weft, "git", "init")
			gitkit.CommitFile(t, weft, "README", "records\n", "seed the records worktree")
			tt.setup(t, weft)

			got, err := fabricengine.AnchoredPathHasContent(loc, "notes")
			if err != nil {
				t.Fatalf("AnchoredPathHasContent(loc, %q) error = %v; want nil", "notes", err)
			}
			if got != tt.want {
				t.Errorf("AnchoredPathHasContent(loc, %q) = %v; want %v", "notes", got, tt.want)
			}
		})
	}
}

// writeProbeFile writes a small file at path, creating its parent directories.
func writeProbeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("note\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
