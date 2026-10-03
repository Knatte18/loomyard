package fabricengine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// TestHubWorkspacePathAndFolders pins the workspace file path and folder geometry for a root anchor and a nested one.
func TestHubWorkspacePathAndFolders(t *testing.T) {
	t.Parallel()

	hub := filepath.Join(string(filepath.Separator), "repos", "loomyard-LYXHUB")
	const prime = "loomyard"

	for _, anchor := range []string{".", "wts/some-task"} {
		anchor := anchor
		t.Run(anchor, func(t *testing.T) {
			t.Parallel()
			l := locationkit.Location(hub, prime, anchor)

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
			wantDirs := []string{
				filepath.Join(hub, prime, anchor),
				filepath.Join(hub, "_board"),
				filepath.Join(hub, "_portals", anchor),
			}
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
