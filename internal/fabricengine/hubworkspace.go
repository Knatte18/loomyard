package fabricengine

import (
	"fmt"
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
// Each Path is relative to the workspace file's directory and slash-separated.
// A filepath.Rel error propagates rather than collapsing to an empty path, because a wrong path written into the file is worse than a failed write.
func HubWorkspaceFolders(l *lyxcwd.Location, primeName string) ([]HubWorkspaceFolder, error) {
	fileDir := filepath.Dir(HubWorkspacePath(l, primeName))
	targets := []HubWorkspaceFolder{
		{Name: primeName, Path: filepath.Join(l.HubPath, primeName, l.AnchorRel)},
		{Name: BoardDirName, Path: BoardDir(l.HubPath)},
		{Name: portalsDirName, Path: filepath.Join(PortalsDir(l), l.AnchorRel)},
	}
	for i, f := range targets {
		rel, err := filepath.Rel(fileDir, f.Path)
		if err != nil {
			return nil, fmt.Errorf("relate workspace dir %s to folder %s at %s: %w", fileDir, f.Name, f.Path, err)
		}
		targets[i].Path = filepath.ToSlash(rel)
	}
	return targets, nil
}
