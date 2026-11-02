// button_test.go pins the run directory the `button` verb hands a click: the anchor recorded now for the worktree holding the rendered directory.

package reedcli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestButtonRunDirectory renders a run directory, records an anchor marker after it, and asks where the click runs.
func TestButtonRunDirectory(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// marker is the anchor recorded after the render; empty records none.
		marker string
		// rendered is the run directory relative to the worktree root, "" for a directory in no worktree.
		rendered string
		// want is the run directory relative to the worktree root.
		want    string
		wantErr error
	}{
		{name: "NoMarkerRunsAtTheWorktreeRoot", rendered: ".", want: "."},
		{name: "SubpathMarkerRecordedAfterARootRenderRunsInTheSubpath", marker: "backend", rendered: ".", want: "backend"},
		{name: "RootMarkerRecordedAfterASubpathRenderRunsAtTheRoot", marker: ".", rendered: "backend", want: "."},
		{name: "RenderMatchingTheMarkerRunsWhereItWasRendered", marker: "backend", rendered: "backend", want: "backend"},
		{name: "DirectoryInNoWorktreeIsAnError", wantErr: lymcwd.ErrNotAGitRepo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatalf("resolve the hub directory: %v", err)
			}
			worktree := filepath.Join(hub, "task")
			if tt.rendered == "" {
				// A gitfile naming a missing git dir ends the walk up, so a temporary directory inside some repository still resolves to none.
				if err := os.MkdirAll(worktree, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", worktree, err)
				}
				if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+filepath.Join(hub, "missing")+"\n"), 0o644); err != nil {
					t.Fatalf("write the gitfile: %v", err)
				}
				if _, err := buttonRunDirectory(worktree); !errors.Is(err, tt.wantErr) {
					t.Errorf("buttonRunDirectory(%q) error = %v; want %v", worktree, err, tt.wantErr)
				}
				return
			}
			for _, dir := range []string{
				filepath.Join(worktree, ".git", "objects"),
				filepath.Join(worktree, ".git", "refs"),
				filepath.Join(worktree, "backend"),
				filepath.Join(hub, lymcwd.BoltDirName),
			} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}
			if err := os.WriteFile(filepath.Join(worktree, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
				t.Fatalf("write HEAD: %v", err)
			}
			if tt.marker != "" {
				marker := filepath.Join(hub, lymcwd.BoltDirName, lymcwd.AnchorFileName)
				if err := os.WriteFile(marker, []byte(tt.marker+"\n"), 0o644); err != nil {
					t.Fatalf("write the anchor marker: %v", err)
				}
			}

			rendered := filepath.Join(worktree, tt.rendered)
			got, err := buttonRunDirectory(rendered)
			if err != nil {
				t.Fatalf("buttonRunDirectory(%q) error = %v; want nil", rendered, err)
			}
			if want := filepath.Join(worktree, tt.want); got != want {
				t.Errorf("buttonRunDirectory(%q) = %q; want %q", rendered, got, want)
			}
		})
	}
}
