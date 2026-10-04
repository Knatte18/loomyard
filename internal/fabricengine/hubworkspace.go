package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// hubWorkspaceExt is the file extension of the prime's hub workspace file.
const hubWorkspaceExt = ".code-workspace"

// HubWorkspaceFolder is one folder entry of the prime's hub workspace file.
// Its fields and json tags match vscode.WorkspaceFolder so the orchestrator converts one to the other directly;
// fabricengine hands out names and paths as values and imports no vscode package.
type HubWorkspaceFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// HubWorkspacePath returns <hub>/_launchers/<AnchorRel>/<primeName>.code-workspace.
func HubWorkspacePath(l *lyxcwd.Location, primeName string) string {
	return filepath.Join(l.HubPath, launchersDirName, l.AnchorRel, primeName+hubWorkspaceExt)
}

// HubWorkspaceFolders returns the workspace file's folders in order: the prime, _board, then _portals.
// Only folders whose target directory exists are returned; a stat error other than not-exist propagates.
// Each Path is relative to the workspace file's directory and slash-separated.
// A filepath.Rel error propagates rather than collapsing to an empty path, because a wrong path written into the file is worse than a failed write.
func HubWorkspaceFolders(l *lyxcwd.Location, primeName string) ([]HubWorkspaceFolder, error) {
	fileDir := filepath.Dir(HubWorkspacePath(l, primeName))
	candidates := []HubWorkspaceFolder{
		{Name: primeName, Path: filepath.Join(WorktreePath(l, primeName), l.AnchorRel)},
		{Name: BoardDirName, Path: BoardDir(l.HubPath)},
		{Name: portalsDirName, Path: portalAnchorDir(l)},
	}
	folders := make([]HubWorkspaceFolder, 0, len(candidates))
	for _, f := range candidates {
		if _, err := os.Stat(f.Path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("stat workspace folder %s at %s: %w", f.Name, f.Path, err)
		}
		rel, err := filepath.Rel(fileDir, f.Path)
		if err != nil {
			return nil, fmt.Errorf("relate workspace dir %s to folder %s at %s: %w", fileDir, f.Name, f.Path, err)
		}
		folders = append(folders, HubWorkspaceFolder{Name: f.Name, Path: filepath.ToSlash(rel)})
	}
	return folders, nil
}
