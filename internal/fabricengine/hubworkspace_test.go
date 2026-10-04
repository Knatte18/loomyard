package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// mkdirAll creates dir and fails the test on error.
func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

// TestHubWorkspacePathAndFolders pins the workspace file path and folder geometry for a root anchor and a nested one.
func TestHubWorkspacePathAndFolders(t *testing.T) {
	t.Parallel()

	const prime = "loomyard"

	for _, anchor := range []string{".", "wts/some-task"} {
		anchor := anchor
		t.Run(anchor, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			l := locationkit.Location(hub, prime, anchor)

			wantDirs := []string{
				filepath.Join(WorktreePath(l, prime), anchor),
				filepath.Join(hub, "_board"),
				filepath.Join(hub, "_portals", anchor),
			}
			for _, d := range wantDirs {
				mkdirAll(t, d)
			}

			file := HubWorkspacePath(l, prime)
			wantFile := filepath.Join(hub, "_launchers", anchor, prime+".code-workspace")
			if file != wantFile {
				t.Fatalf("HubWorkspacePath() = %q; want %q", file, wantFile)
			}

			folders, err := HubWorkspaceFolders(l, prime)
			if err != nil {
				t.Fatalf("HubWorkspaceFolders() error: %v", err)
			}
			wantNames := []string{prime, "_board", "_portals"}
			if len(folders) != len(wantNames) {
				t.Fatalf("got %d folders; want %d", len(folders), len(wantNames))
			}
			for i, f := range folders {
				if f.Name != wantNames[i] {
					t.Errorf("folder %d name = %q; want %q", i, f.Name, wantNames[i])
				}
				if strings.Contains(f.Path, `\`) {
					t.Errorf("folder %d path %q contains a backslash", i, f.Path)
				}
				got := filepath.Join(filepath.Dir(file), filepath.FromSlash(f.Path))
				if got != wantDirs[i] {
					t.Errorf("folder %d resolves to %q; want %q", i, got, wantDirs[i])
				}
			}
		})
	}
}

// TestHubWorkspaceFolders_OmitsMissingDirectories pins that a folder whose target directory does not exist is left out
// while the present ones keep their order.
func TestHubWorkspaceFolders_OmitsMissingDirectories(t *testing.T) {
	t.Parallel()

	const prime = "loomyard"
	hub := t.TempDir()
	l := locationkit.Location(hub, prime, filepath.Join("wts", "some-task"))
	mkdirAll(t, filepath.Join(WorktreePath(l, prime), l.AnchorRel))
	mkdirAll(t, filepath.Join(hub, "_board"))

	folders, err := HubWorkspaceFolders(l, prime)
	if err != nil {
		t.Fatalf("HubWorkspaceFolders() error: %v", err)
	}
	if len(folders) != 2 || folders[0].Name != prime || folders[1].Name != "_board" {
		t.Fatalf("HubWorkspaceFolders() = %+v; want the prime then _board, with the absent _portals anchor omitted", folders)
	}
}
